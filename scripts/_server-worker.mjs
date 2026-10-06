/**
 * Worker thread that runs HTTP servers (SPA proxy + Adapter) in a separate
 * thread so the main thread can use execSync without blocking the event loop.
 */
import { parentPort, workerData } from 'node:worker_threads'
import { once } from 'node:events'
import { join } from 'node:path'
import { resolveWithin } from './lib/harness.mjs'
import { existsSync } from 'node:fs'
import http from 'node:http'
import { readFileSync, statSync } from 'node:fs'

const { spaPort, spaDir, adapterPort, hubPort, relayPort, captureStarterRequests = false } = workerData
let starterCapture = null
let mcpMode = 'initial'

// --- MIME types ---
const MIME_TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'application/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
}

// --- Deterministic Adapter ---
const ttsBytes = Buffer.from([
  0x49, 0x44, 0x33, 0x03, 0x00, 0x00, 0x00, 0x00,
  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
])

const adapterServer = http.createServer((req, res) => {
  const url = new URL(req.url, `http://127.0.0.1:${adapterPort}`)
  const bodyChunks = []
  req.on('data', (chunk) => bodyChunks.push(chunk))
  req.on('end', () => {
    const body = Buffer.concat(bodyChunks)
    let bodyJSON = null
    try {
      if (req.headers['content-type']?.includes('application/json') && body.length > 0) {
        bodyJSON = JSON.parse(body.toString())
      }
    } catch {}

    const path = url.pathname
    // Isolated deterministic adapter control; never mounted by production Hub.
    if (path === '/__mcp_mode' && req.method === 'POST') {
      if (!['initial', 'added', 'drift', 'deleted', 'failure', 'large', 'dense'].includes(bodyJSON?.mode)) { res.writeHead(400); res.end(); return }
      mcpMode = bodyJSON.mode
      res.writeHead(204); res.end(); return
    }
    if (path === '/v1/chat/completions') {
      if (captureStarterRequests && starterCapture && Array.isArray(bodyJSON?.messages)) {
        const userTexts = bodyJSON.messages.filter(message => message.role === 'user').flatMap(message =>
          typeof message.content === 'string' ? [message.content] : Array.isArray(message.content) ? message.content.filter(part => typeof part.text === 'string').map(part => part.text) : [])
        if (userTexts.some(text => text === starterCapture.prompt || text.endsWith('\n' + starterCapture.prompt))) {
          if (starterCapture.requests.length >= 16) starterCapture.overflow = true
          else starterCapture.requests.push({ model: bodyJSON.model, messages: bodyJSON.messages.map(message => ({ role: message.role, content: message.content })) })
        }
      }
      const streaming = bodyJSON?.stream === true
      if (streaming) {
        res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' })
        const chunks = [
          `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"}}]}`,
          `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"content":"hel"}}]}`,
          `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"content":"lo"}}]}`,
          `data: {"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
          `data: [DONE]`,
        ]
        for (const c of chunks) res.write(c + '\n\n')
        res.end()
      } else {
        res.writeHead(200, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({
          id: '1', object: 'chat.completion',
          choices: [{ index: 0, message: { role: 'assistant', content: 'hello' }, finish_reason: 'stop' }],
        }))
      }
      return
    }
    if (path === '/v1/audio/speech') {
      res.writeHead(200, { 'Content-Type': 'audio/mpeg' })
      res.end(ttsBytes)
      return
    }
    if (path === '/v1/audio/transcriptions') {
      res.writeHead(200, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ text: 'transcribed' }))
      return
    }
    if (path === '/v1/images/generations') {
      res.writeHead(200, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ created: 1789833600, data: [{ b64_json: 'iVBORw0KGgo=' }] }))
      return
    }
    if (path === '/api/v1/services/aigc/multimodal-generation/generation') {
      const valid = bodyJSON?.model === 'wan2.7-image'
        && bodyJSON?.input?.messages?.length === 1
        && bodyJSON.input.messages[0]?.role === 'user'
        && typeof bodyJSON.input.messages[0]?.content?.[0]?.text === 'string'
        && bodyJSON?.parameters?.size === '1024*1024'
        && Number.isInteger(bodyJSON?.parameters?.n)
        && bodyJSON?.parameters?.watermark === false
      if (!valid) {
        res.writeHead(400, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ code: 'InvalidParameter', message: 'invalid DashScope image request' }))
        return
      }
      res.writeHead(200, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ output: { choices: [{ message: { content: [{ image: 'https://images.example/generated.png' }] } }] } }))
      return
    }
    if (path === '/mcp') {
      if (mcpMode === 'failure') { res.writeHead(503); res.end('private adapter error'); return }
      if (req.method !== 'POST') { res.writeHead(405); res.end(); return }
      if (bodyJSON?.jsonrpc !== '2.0' || typeof bodyJSON.method !== 'string') {
        res.writeHead(400, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ jsonrpc: '2.0', id: null, error: { code: -32600, message: 'Invalid Request' } }))
        return
      }
      if (!Object.hasOwn(bodyJSON, 'id')) { res.writeHead(202); res.end(); return }
      const result = bodyJSON.method === 'initialize'
        ? { protocolVersion: '2025-06-18', capabilities: { tools: {} }, serverInfo: { name: 'measix-test-adapter', version: '1.0.0' } }
        : bodyJSON.method === 'tools/list'
          ? { tools: mcpMode === 'dense' ? Array.from({ length: 27 }, (_, index) => ({
            name: index === 0 ? 'firecrawl_agent' : index === 26 ? `research_${'very_long_tool_name_'.repeat(12)}26` : `research_${String(index).padStart(2, '0')}`,
            description: index === 0
              ? 'Structured website research: search public websites, read relevant pages and return evidence for the requested fields.\n' + 'Provide a research question and the fields you need. Results may combine several pages, with source links and structured records. '.repeat(6) + 'Complete definition end.'
              : `Research catalog item ${index}: find source documents and summarize their contents. ` + 'Use the supplied filters to narrow the results and return sources for review. '.repeat(6),
            inputSchema: { type: 'object', properties: { prompt: { type: 'string' } } },
          })) : mcpMode === 'deleted' ? [] : [
            { name: 'tool-a', description: mcpMode === 'drift' ? 'Changed read contract' : 'Read enterprise records', inputSchema: { type: 'object' }, annotations: { readOnlyHint: true } },
            ...(mcpMode === 'added' || mcpMode === 'drift' ? [{ name: 'tool-new', description: 'Newly discovered tool', inputSchema: { type: 'object' } }] : []),
            ...(mcpMode === 'large' ? Array.from({ length: 63 }, (_, index) => ({ name: `tool-${String(index).padStart(3, '0')}`, description: `Enterprise catalog item ${index}`, inputSchema: { type: 'object' } })) : []),
          ] }
          : bodyJSON.method === 'tools/call'
            ? { content: [{ type: 'text', text: 'tool-a executed' }] }
            : null
      res.writeHead(200, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify(result === null
        ? { jsonrpc: '2.0', id: bodyJSON.id, error: { code: -32601, message: 'Method not found' } }
        : { jsonrpc: '2.0', id: bodyJSON.id, result }))
      return
    }
    if (path.startsWith('/v1/errors/')) {
      const code = parseInt(path.split('/').pop(), 10) || 400
      res.writeHead(code, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ error: { code } }))
      return
    }
    res.writeHead(404, { 'Content-Type': 'application/json' })
    res.end(JSON.stringify({ error: 'not found' }))
  })
})
adapterServer.keepAliveTimeout = 30000
adapterServer.headersTimeout = 35000
adapterServer.listen(adapterPort, '127.0.0.1')

// --- SPA Proxy ---
function serveStaticFile(res, fullPath, name) {
  const data = readFileSync(fullPath)
  if (name === 'index.html') {
    res.setHeader('Cache-Control', 'no-cache')
  } else if (name.startsWith('assets/')) {
    res.setHeader('Cache-Control', 'public, max-age=31536000, immutable')
  }
  const ext = name.slice(name.lastIndexOf('.'))
  const ct = MIME_TYPES[ext] || 'application/octet-stream'
  res.setHeader('Content-Type', ct)
  res.setHeader('Content-Length', data.length)
  res.writeHead(200)
  res.end(data)
}

const spaServer = http.createServer((req, res) => {
  const url = new URL(req.url, `http://127.0.0.1:${spaPort}`)
  const path = url.pathname

  // One public origin: Discovery/control to Hub, Runtime to Relay.
  const runtime = path.startsWith('/runtime/v1/')
  if (runtime || path === '/.well-known/measix' || path.startsWith('/api/') || path === '/live' || path === '/ready') {
    const proxyReq = http.request({
      hostname: '127.0.0.1',
      port: runtime ? relayPort : hubPort,
      path: req.url,
      method: req.method,
      headers: req.headers,
    }, (proxyResp) => {
      res.writeHead(proxyResp.statusCode, proxyResp.headers)
      proxyResp.pipe(res)
    })
    proxyReq.on('error', (e) => {
      if (!res.headersSent) {
        res.writeHead(502, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ error: 'proxy error', detail: e.message }))
      }
    })
    req.pipe(proxyReq)
    return
  }

  // Serve SPA static files under /admin
  if (path === '/admin' || path.startsWith('/admin/')) {
    let filePath = path.replace(/^\/admin\/?/, '')
    if (!filePath) filePath = 'index.html'
    const full = resolveWithin(spaDir, filePath)
    if (full && existsSync(full) && statSync(full).isFile()) {
      serveStaticFile(res, full, filePath)
      return
    }
    // SPA fallback
    if (!filePath.startsWith('assets/') && !filePath.includes('.')) {
      const index = join(spaDir, 'index.html')
      if (existsSync(index)) {
        serveStaticFile(res, index, 'index.html')
        return
      }
    }
    res.writeHead(404)
    res.end('not found')
    return
  }

  if (path === '/' || path === '') {
    res.writeHead(302, { Location: '/admin' })
    res.end()
    return
  }

  res.writeHead(404, { 'Content-Type': 'application/json' })
  res.end(JSON.stringify({ error: 'not found' }))
})
spaServer.keepAliveTimeout = 30000
spaServer.headersTimeout = 35000
spaServer.listen(spaPort, '127.0.0.1')

await Promise.all([once(adapterServer, 'listening'), once(spaServer, 'listening')])
parentPort.postMessage({ ready: true })

parentPort.on('message', (msg) => {
  if (captureStarterRequests && msg.starterCapture === 'start') {
    starterCapture = { prompt: msg.prompt, requests: [], overflow: false }
    parentPort.postMessage({ starterCaptureId: msg.id, started: true })
  }
  if (captureStarterRequests && msg.starterCapture === 'stop') {
    parentPort.postMessage({ starterCaptureId: msg.id, requests: starterCapture?.requests ?? [], overflow: starterCapture?.overflow ?? false })
    starterCapture = null
  }
  if (msg.shutdown) {
    adapterServer.close()
    spaServer.close()
    process.exit(0)
  }
})
