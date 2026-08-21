#!/usr/bin/env node
/**
 * S0.1 T4.1 Clean-Environment Harness
 *
 * Per measix-s0-capability-delivery-system-testing-spec.md §3:
 *   "独立 temp directory/SQLite/ports/identity"
 *   "真实 SQLite + migrations"
 *   "真实 TCP/HTTP streaming/binary/multipart"
 *   "Hub/Relay T3/T4.1 使用真实 process/binary"
 *   "Admin T4.1 使用 production dist/spa"
 *
 * This script:
 *   1. Creates a clean temp directory with unique ports and identity
 *   2. Generates crypto material (master key, Ed25519 seed, relay service token)
 *   3. Applies migrations to a fresh SQLite DB via devmigrate
 *   4. Bootstraps an admin user via control-hub bootstrap-admin
 *   5. Builds and starts the Control Hub process
 *   6. Builds and starts the Runtime Relay process
 *   7. Starts a deterministic upstream Adapter (Node implementation)
 *   8. Starts a same-origin SPA proxy (serves dist/spa + proxies API to Hub)
 *   9. Runs Playwright E2E tests
 *  10. Collects evidence/artifacts and tears down
 *
 * Usage:
 *   node scripts/e2e-harness.mjs [--keep] [--timeout=600000]
 *
 * Environment variables:
 *   MEASIX_E2E_TIMEOUT  — max time for the harness (default 600000ms = 10min)
 */
import { spawn, execSync } from 'node:child_process'
import {
  existsSync,
  mkdirSync,
  rmSync,
  writeFileSync,
  readFileSync,
  statSync,
} from 'node:fs'
import { join, resolve, extname } from 'node:path'
import { tmpdir } from 'node:os'
import { createServer } from 'node:http'
import { randomFillSync, createHash } from 'node:crypto'
import { createRequire } from 'node:module'
import { request as httpRequest } from 'node:http'

const require = createRequire(import.meta.url)
const net = require('node:net')

const ROOT = resolve(import.meta.dirname, '..')
const ARCH_REPO = resolve(ROOT, '..', 'measix-architecture')
const KEEP = process.argv.includes('--keep')
const TIMEOUT = parseInt(process.env.MEASIX_E2E_TIMEOUT || '600000', 10)

const processes = []
const servers = []
let envRoot = ''

function log(msg) {
  console.log(`[harness] ${msg}`)
}

function cleanup() {
  if (KEEP) {
    log(`Keeping temp dir: ${envRoot}`)
    return
  }
  for (const p of processes) {
    try { p.kill('SIGTERM') } catch {}
  }
  for (const s of servers) {
    try { s.close() } catch {}
  }
  setTimeout(() => {
    for (const p of processes) {
      try { p.kill('SIGKILL') } catch {}
    }
    if (envRoot) {
      try { rmSync(envRoot, { recursive: true, force: true }) } catch {}
    }
  }, 3000)
}

process.on('SIGINT', () => { cleanup(); process.exit(1) })
process.on('SIGTERM', () => { cleanup(); process.exit(1) })
process.on('exit', () => { cleanup() })

// --- Allocate free ports ---

function freePort() {
  return new Promise((resolveP, rejectP) => {
    const srv = net.createServer()
    srv.listen(0, '127.0.0.1', () => {
      const port = srv.address().port
      srv.close(() => resolveP(port))
    })
    srv.on('error', rejectP)
  })
}

// --- Generate random bytes ---

function randomBytes(n) {
  const buf = Buffer.alloc(n)
  randomFillSync(buf)
  return buf
}

// --- Setup ---

envRoot = join(tmpdir(), `measix-e2e-${Date.now()}`)
mkdirSync(envRoot, { recursive: true })

const hubDB = join(envRoot, 'hub.db')
const masterKeyFile = join(envRoot, 'master.key')
const jwtKeyFile = join(envRoot, 'jwt-ed25519.seed')
const relayTokenFile = join(envRoot, 'relay-service.token')
const spoolPath = join(envRoot, 'relay-spool.db')
const pwFile = join(envRoot, 'admin-password.txt')

const hubPort = await freePort()
const hubInternalPort = await freePort()
const relayPubPort = await freePort()
const relayIntPort = await freePort()
const adapterPort = await freePort()
const spaPort = await freePort()

const hubBaseURL = `http://127.0.0.1:${hubPort}`
const hubInternalBaseURL = `http://127.0.0.1:${hubInternalPort}`
const relayPubBaseURL = `http://127.0.0.1:${relayPubPort}`
const relayIntBaseURL = `http://127.0.0.1:${relayIntPort}`
const adapterBaseURL = `http://127.0.0.1:${adapterPort}`
const spaBaseURL = `http://127.0.0.1:${spaPort}`

log(`temp dir: ${envRoot}`)
log(`hub: ${hubBaseURL}`)
log(`hub internal: ${hubInternalBaseURL}`)
log(`relay public: ${relayPubBaseURL}`)
log(`relay internal: ${relayIntBaseURL}`)
log(`adapter: ${adapterBaseURL}`)
log(`spa (same-origin proxy): ${spaBaseURL}`)

// --- 1. Generate crypto material ---

log('generating crypto material...')
writeFileSync(masterKeyFile, randomBytes(32), { mode: 0o600 })
writeFileSync(jwtKeyFile, randomBytes(32), { mode: 0o600 })
const tokenBytes = randomBytes(32)
writeFileSync(relayTokenFile, Buffer.from(tokenBytes.toString('hex') + '\n'), { mode: 0o600 })

// --- 2. Generate admin password ---

const adminPassword = 'e2e-admin-' + tokenBytes.toString('hex').slice(0, 8)
writeFileSync(pwFile, adminPassword + '\n', { mode: 0o600 })

// --- 3. Build binaries ---

log('building control-hub and runtime-relay binaries...')
const backendDir = join(ROOT, 'backend')
const hubBin = join(envRoot, process.platform === 'win32' ? 'control-hub.exe' : 'control-hub')
const relayBin = join(envRoot, process.platform === 'win32' ? 'runtime-relay.exe' : 'runtime-relay')

try {
  execSync(`go build -o "${hubBin}" ./cmd/control-hub`, { cwd: backendDir, stdio: 'inherit' })
  execSync(`go build -o "${relayBin}" ./cmd/runtime-relay`, { cwd: backendDir, stdio: 'inherit' })
} catch (e) {
  log(`build failed: ${e.message}`)
  process.exit(1)
}

// --- 4. Apply migrations ---

log('applying migrations...')
try {
  execSync(`go run ./cmd/devmigrate --db "${hubDB}"`, { cwd: backendDir, stdio: 'inherit' })
} catch (e) {
  log(`migration failed: ${e.message}`)
  process.exit(1)
}

// --- 5. Bootstrap admin ---

log('bootstrapping admin...')
try {
  execSync(
    `go run ./cmd/control-hub bootstrap-admin` +
    ` --db "${hubDB}"` +
    ` --master-key-file "${masterKeyFile}"` +
    ` --jwt-private-key-file "${jwtKeyFile}"` +
    ` --deployment-name "E2E-TEST"` +
    ` --username "admin"` +
    ` --display-name "E2E Test Admin"` +
    ` --password-file "${pwFile}"`,
    { cwd: backendDir, stdio: 'inherit' },
  )
} catch (e) {
  log(`bootstrap-admin failed: ${e.message}`)
  process.exit(1)
}

// --- 6. Start Control Hub ---

log('starting Control Hub...')
const hubProc = spawn(hubBin, [
  'run',
  '--listen', `127.0.0.1:${hubPort}`,
  '--internal-listen', `127.0.0.1:${hubInternalPort}`,
  '--public-base-url', hubBaseURL,
  '--runtime-api-base', relayPubBaseURL,
  '--db', hubDB,
  '--master-key-file', masterKeyFile,
  '--jwt-private-key-file', jwtKeyFile,
  '--relay-internal-url', relayIntBaseURL,
  '--relay-service-token-file', relayTokenFile,
  '--reconcile-interval', '2s',
], {
  cwd: envRoot,
  stdio: ['ignore', 'pipe', 'pipe'],
})
processes.push(hubProc)
hubProc.stdout.on('data', (d) => process.stdout.write(`[hub] ${d}`))
hubProc.stderr.on('data', (d) => process.stderr.write(`[hub] ${d}`))

// --- 7. Start Runtime Relay ---

log('starting Runtime Relay...')
const relayProc = spawn(relayBin, [
  '--public-listen', `127.0.0.1:${relayPubPort}`,
  '--internal-listen', `127.0.0.1:${relayIntPort}`,
  '--spool', spoolPath,
  '--hub-usage-url', `${hubInternalBaseURL}/internal/v1/usage/request-events:batch`,
  '--hub-service-token-file', relayTokenFile,
], {
  cwd: envRoot,
  stdio: ['ignore', 'pipe', 'pipe'],
})
processes.push(relayProc)
relayProc.stdout.on('data', (d) => process.stdout.write(`[relay] ${d}`))
relayProc.stderr.on('data', (d) => process.stderr.write(`[relay] ${d}`))

// --- 8. Start deterministic Adapter ---

log('starting deterministic Adapter...')
const adapterServer = startAdapter(adapterPort)
servers.push(adapterServer)

// --- 9. Wait for Hub and Relay to be ready ---

async function waitFor(url, label, maxWait = 30000) {
  const start = Date.now()
  let last = ''
  while (Date.now() - start < maxWait) {
    try {
      const resp = await fetch(url)
      if (resp.ok || resp.status === 401 || resp.status === 404) {
        log(`${label} ready`)
        return
      }
      last = `status=${resp.status}`
    } catch (e) {
      last = e.message
    }
    await new Promise(r => setTimeout(r, 500))
  }
  throw new Error(`${label} not ready after ${maxWait}ms (${last})`)
}

await waitFor(`${hubBaseURL}/live`, 'Hub')
await waitFor(`${relayIntBaseURL}/live`, 'Relay')

// --- 10. Start same-origin SPA proxy ---

log('starting same-origin SPA proxy...')
const spaDir = join(ROOT, 'console', 'dist', 'spa')
if (!existsSync(spaDir)) {
  log('SPA build not found. Run "make console-build" first.')
  process.exit(1)
}

const spaServer = startSpaProxy(spaPort, spaDir, hubPort)
servers.push(spaServer)

await waitFor(spaBaseURL, 'SPA')

// --- 11. Run Playwright E2E ---

log('running Playwright E2E tests...')
const e2eEnv = {
  ...process.env,
  MEASIX_E2E_BASE_URL: spaBaseURL,
  MEASIX_E2E_HUB_BASE_URL: hubBaseURL,
  MEASIX_E2E_ADAPTER_URL: adapterBaseURL,
  MEASIX_E2E_ADMIN_PASSWORD: adminPassword,
  PLAYWRIGHT_BASE_URL: spaBaseURL,
}

// Ensure .artifacts directory exists for JSON reporter output
const artifactsDir = join(ROOT, '.artifacts')
if (!existsSync(artifactsDir)) {
  mkdirSync(artifactsDir, { recursive: true })
}

try {
  // Use the Playwright config which includes the JSON reporter
  // outputting to ../.artifacts/e2e-playwright.json
  // Do NOT pass --reporter=list — it would override the config's reporters
  // and prevent the JSON artifact from being generated.
  execSync('npx playwright test', {
    cwd: join(ROOT, 'console'),
    stdio: 'inherit',
    env: e2eEnv,
    timeout: TIMEOUT,
  })
  log('E2E tests PASSED')

  // Write artifact metadata envelope for provenance validation
  const playwrightArtifactPath = join(artifactsDir, 'e2e-playwright.json')
  if (existsSync(playwrightArtifactPath)) {
    const artifactSha = 'sha256:' + createHash('sha256').update(readFileSync(playwrightArtifactPath)).digest('hex')
    const now = new Date().toISOString()
    const meta = {
      platformCoreCommit: execSync('git rev-parse HEAD', { cwd: ROOT, encoding: 'utf-8' }).trim(),
      architectureCommit: (() => {
        try { return execSync('git rev-parse HEAD', { cwd: ARCH_REPO, encoding: 'utf-8' }).trim() }
        catch { return 'unknown' }
      })(),
      command: 'npx playwright test',
      artifactSha256: artifactSha,
      startedAt: now,
      completedAt: now,
      exitCode: 0,
    }
    writeFileSync(join(artifactsDir, 'e2e-playwright.json.meta.json'), JSON.stringify(meta, null, 2) + '\n')
    log(`wrote e2e-playwright.json.meta.json (sha=${artifactSha.slice(0, 20)}...)`)
  }
} catch (e) {
  log(`E2E tests FAILED: ${e.message}`)

  // Still write meta.json even on failure for provenance
  const playwrightArtifactPath = join(artifactsDir, 'e2e-playwright.json')
  if (existsSync(playwrightArtifactPath)) {
    const artifactSha = 'sha256:' + createHash('sha256').update(readFileSync(playwrightArtifactPath)).digest('hex')
    const now = new Date().toISOString()
    const meta = {
      platformCoreCommit: execSync('git rev-parse HEAD', { cwd: ROOT, encoding: 'utf-8' }).trim(),
      architectureCommit: (() => {
        try { return execSync('git rev-parse HEAD', { cwd: ARCH_REPO, encoding: 'utf-8' }).trim() }
        catch { return 'unknown' }
      })(),
      command: 'npx playwright test',
      artifactSha256: artifactSha,
      startedAt: now,
      completedAt: now,
      exitCode: 1,
    }
    writeFileSync(join(artifactsDir, 'e2e-playwright.json.meta.json'), JSON.stringify(meta, null, 2) + '\n')
  }
  process.exit(1)
}

// --- 12. Done ---

log('all steps completed successfully')
log(`evidence dir: ${envRoot}`)
cleanup()

// ---------------------------------------------------------------------------
// Deterministic Adapter (Node implementation of backend/test/system/adapter)
// ---------------------------------------------------------------------------

function startAdapter(port) {
  const ttsBytes = Buffer.from([
    0x49, 0x44, 0x33, 0x03, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
  ])

  const server = createServer((req, res) => {
    const url = new URL(req.url, `http://127.0.0.1:${port}`)

    // Gather body for JSON parsing
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
          res.writeHead(200, {
            'Content-Type': 'text/event-stream',
            'Cache-Control': 'no-cache',
          })
          const chunks = [
            `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"}}]}`,
            `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"content":"hel"}}]}`,
            `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"content":"lo"}}]}`,
            `data: [DONE]`,
          ]
          for (const c of chunks) {
            res.write(c + '\n\n')
          }
          res.end()
        } else {
          res.writeHead(200, { 'Content-Type': 'application/json' })
          res.end(JSON.stringify({
            id: '1',
            object: 'chat.completion',
            choices: [{
              index: 0,
              message: { role: 'assistant', content: 'hello' },
              finish_reason: 'stop',
            }],
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
        res.end(JSON.stringify({
          jsonrpc: '2.0',
          id: 1,
          result: { tools: [{ name: 'tool-a' }] },
        }))
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

  server.listen(port, '127.0.0.1')
  return server
}

// ---------------------------------------------------------------------------
// Same-origin SPA proxy
// Serves production dist/spa at /admin/* and proxies /api/*, /live, /ready
// to Hub. Per architecture, /internal/* MUST NOT be reachable from the
// public origin — it must only exist on the Hub's private listener.
// The Relay internal listener is on a separate port/address.
// ---------------------------------------------------------------------------

function startSpaProxy(port, spaDir, hubPort) {
  const server = createServer((req, res) => {
    const url = new URL(req.url, `http://127.0.0.1:${port}`)
    const path = url.pathname

    // Proxy API requests to Hub via raw HTTP pipeline.
    // /internal/* is intentionally NOT proxied — architecture requires
    // management/internal endpoints to be unreachable from public origin.
    if (path.startsWith('/api/') || path === '/live' || path === '/ready') {
      const proxyReq = httpRequest({
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

      // SPA fallback: serve index.html for client-side routing (non-asset, no extension)
      if (!filePath.startsWith('assets/') && !extname(filePath)) {
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

    // Root redirect to /admin
    if (path === '/' || path === '') {
      res.writeHead(302, { Location: '/admin' })
      res.end()
      return
    }

    res.writeHead(404, { 'Content-Type': 'application/json' })
    res.end(JSON.stringify({ error: 'not found' }))
  })

  server.listen(port, '127.0.0.1')
  return server
}

function serveStaticFile(res, fullPath, name) {
  const data = readFileSync(fullPath)

  if (name === 'index.html') {
    res.setHeader('Cache-Control', 'no-cache')
  } else if (name.startsWith('assets/')) {
    res.setHeader('Cache-Control', 'public, max-age=31536000, immutable')
  }

  const ext = extname(name)
  const mimeTypes = {
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
  const ct = mimeTypes[ext] || 'application/octet-stream'
  res.setHeader('Content-Type', ct)
  res.writeHead(200)
  res.end(data)
}
