/**
 * Worker thread that runs HTTP servers (SPA proxy + Adapter) in a separate
 * thread so the main thread can use execSync without blocking the event loop.
 */
import { parentPort, workerData } from 'node:worker_threads'
import { join } from 'node:path'
import { existsSync } from 'node:fs'
import http from 'node:http'
import { readFileSync, statSync } from 'node:fs'

const { spaPort, spaDir, adapterPort, hubPort } = workerData

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
    if (path === '/v1/chat/completions') {
      const streaming = bodyJSON?.stream === true
      if (streaming) {
        res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' })
        const chunks = [
          `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"}}]}`,
          `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"content":"hel"}}]}`,
          `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"content":"lo"}}]}`,
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
    if (path === '/mcp') {
      res.writeHead(200, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ jsonrpc: '2.0', id: 1, result: { tools: [{ name: 'tool-a' }] } }))
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

  // Proxy API requests to Hub
  if (path.startsWith('/api/') || path === '/live' || path === '/ready') {
    const proxyReq = http.request({
      hostname: '127.0.0.1',
      port: hubPort,
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
    const full = join(spaDir, filePath)
    if (existsSync(full) && statSync(full).isFile()) {
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

parentPort.postMessage({ ready: true })

parentPort.on('message', (msg) => {
  if (msg.shutdown) {
    adapterServer.close()
    spaServer.close()
    process.exit(0)
  }
})
