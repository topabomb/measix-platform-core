/**
 * Shared harness utilities for measix S0.1 scripts.
 *
 * Used by:
 *   - scripts/e2e-harness.mjs
 *   - scripts/replay-freeze.mjs
 *   - scripts/collect-adapter-qualification.mjs
 *   - scripts/freeze-manifest.mjs (partial)
 *
 * This module eliminates ~400-500 lines of duplicated utility code across
 * the three harness scripts.
 */
import { createHash, randomFillSync } from 'node:crypto'
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { join, resolve, relative, extname } from 'node:path'
import { execFileSync, spawn } from 'node:child_process'
import { tmpdir } from 'node:os'
import { mkdtempSync, rmSync, writeFileSync, mkdirSync } from 'node:fs'
import { createServer, request as httpRequest } from 'node:http'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const net = require('node:net')

// --- Path constants (derived from caller's script location) ---

/**
 * Resolve the repository root from a script's __dirname.
 * Scripts live in scripts/ or scripts/lib/, so ROOT is always 1-2 levels up.
 * @param {string} scriptDir - import.meta.dirname of the calling script
 * @returns {string} absolute path to repo root
 */
export function resolveRoot(scriptDir) {
  // If the script is in scripts/, parent is root.
  // If the script is in scripts/lib/, grandparent is root.
  const parent = resolve(scriptDir, '..')
  const grandparent = resolve(scriptDir, '../..')
  // Check if parent has a 'backend' dir → it's the root
  if (existsSync(join(parent, 'backend'))) return parent
  if (existsSync(join(grandparent, 'backend'))) return grandparent
  // Fallback: assume scripts/ structure
  return parent
}

// --- Git utilities ---

export function gitCommit(cwd) {
  try {
    return execFileSync('git', ['rev-parse', 'HEAD'], { cwd, encoding: 'utf-8' }).trim()
  } catch {
    return 'unknown'
  }
}

export function gitDirty(cwd) {
  try {
    return execFileSync('git', ['status', '--porcelain'], { cwd, encoding: 'utf-8' }).trim().length > 0
  } catch {
    return true
  }
}

// --- File hashing utilities ---

export function sha256File(path) {
  return 'sha256:' + createHash('sha256').update(readFileSync(path).toString('utf-8').replace(/\r\n/g, '\n')).digest('hex')
}

export function collectFiles(dir) {
  const out = []
  for (const entry of readdirSync(dir).sort()) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) out.push(...collectFiles(full))
    else out.push(full)
  }
  return out
}

/**
 * Compute a deterministic hash of the production Admin Console SPA build.
 * @param {string} root - repo root path
 * @returns {string} 'sha256:...' or 'not-built'
 */
export function adminBuildHash(root) {
  const distDir = join(root, 'console', 'dist', 'spa')
  if (!existsSync(distDir)) return 'not-built'
  const hash = createHash('sha256')
  for (const file of collectFiles(distDir).sort()) {
    hash.update(relative(distDir, file).replace(/\\/g, '/'))
    hash.update('\0')
    hash.update(readFileSync(file))
    hash.update('\0')
  }
  return 'sha256:' + hash.digest('hex')
}

/**
 * Compute a deterministic hash of the deterministic adapter + client source.
 * @param {string} root - repo root path
 * @returns {string} 'sha256:...'
 */
export function deterministicAdapterVersion(root) {
  const ADAPTER_SOURCE = join(root, 'backend', 'test', 'system', 'adapter', 'adapter.go')
  const ADAPTER_TEST = join(root, 'backend', 'test', 'system', 'adapter', 'adapter_test.go')
  const CLIENT_SOURCE = join(root, 'backend', 'test', 'system', 'client', 'client.go')
  const hash = createHash('sha256')
  if (existsSync(ADAPTER_SOURCE)) hash.update(readFileSync(ADAPTER_SOURCE).toString('utf-8').replace(/\r\n/g, '\n'))
  hash.update('\0')
  if (existsSync(ADAPTER_TEST)) hash.update(readFileSync(ADAPTER_TEST).toString('utf-8').replace(/\r\n/g, '\n'))
  hash.update('\0')
  if (existsSync(CLIENT_SOURCE)) hash.update(readFileSync(CLIENT_SOURCE).toString('utf-8').replace(/\r\n/g, '\n'))
  return 'sha256:' + hash.digest('hex')
}

// --- Port and crypto utilities ---

export function freePort() {
  return new Promise((resolveP, rejectP) => {
    const srv = net.createServer()
    srv.listen(0, '127.0.0.1', () => {
      const port = srv.address().port
      srv.close(() => resolveP(port))
    })
    srv.on('error', rejectP)
  })
}

export function randomBytes(n) {
  const buf = Buffer.alloc(n)
  randomFillSync(buf)
  return buf
}

// --- Process health check ---

/**
 * Wait for an HTTP endpoint to become ready.
 * @param {string} url - URL to poll
 * @param {string} label - human-readable label for logging
 * @param {number} maxWait - max wait time in ms (default 30000)
 * @param {function} [logFn] - optional log function (default console.log)
 * @returns {Promise<boolean>} true if ready, false if timed out
 */
export async function waitFor(url, label, maxWait = 30000, logFn = null) {
  const log = logFn || (msg => console.log(msg))
  const start = Date.now()
  let last = ''
  while (Date.now() - start < maxWait) {
    try {
      const resp = await fetch(url)
      if (resp.ok || resp.status === 401 || resp.status === 404) {
        log(`${label} ready`)
        return true
      }
      last = `status=${resp.status}`
    } catch (e) {
      last = e.message
    }
    await new Promise(r => setTimeout(r, 500))
  }
  log(`${label} not ready after ${maxWait}ms (${last})`)
  return false
}

// --- Fresh environment setup ---

/**
 * Create a fresh temp environment with crypto material and admin password.
 * @param {string} prefix - temp dir name prefix (e.g. 'measix-e2e', 'measix-replay')
 * @param {string} deploymentName - deployment name for bootstrap-admin
 * @param {string} displayName - admin display name
 * @param {string} [adminPasswordOverride] - optional password override
 * @returns {Promise<object>} env config object with all paths, ports, URLs, and processes
 */
export async function createFreshEnvironment(root, opts = {}) {
  const {
    prefix = 'measix-env',
    deploymentName = 'TEST',
    displayName = 'Test Admin',
    adminPasswordPrefix = 'test',
    adminPasswordOverride = null,
    buildStdio = 'inherit',
    migrateStdio = 'inherit',
    bootstrapStdio = 'inherit',
  } = opts

  const { execSync } = await import('node:child_process')
  const envRoot = mkdtempSync(join(tmpdir(), `${prefix}-`))
  const hubDB = join(envRoot, 'hub.db')
  const masterKeyFile = join(envRoot, 'master.key')
  const jwtKeyFile = join(envRoot, 'jwt-ed25519.seed')
  const relayTokenFile = join(envRoot, 'relay-service.token')
  const spoolPath = join(envRoot, 'relay-spool.db')
  const pwFile = join(envRoot, 'admin-password.txt')

  // Generate crypto material
  writeFileSync(masterKeyFile, randomBytes(32), { mode: 0o600 })
  writeFileSync(jwtKeyFile, randomBytes(32), { mode: 0o600 })
  const tokenBytes = randomBytes(32)
  writeFileSync(relayTokenFile, Buffer.from(tokenBytes.toString('hex') + '\n'), { mode: 0o600 })
  const adminPassword = adminPasswordOverride || (adminPasswordPrefix + '-' + tokenBytes.toString('hex').slice(0, 8))
  writeFileSync(pwFile, adminPassword + '\n', { mode: 0o600 })

  // Allocate ports
  const hubPort = await freePort()
  const hubInternalPort = await freePort()
  const relayPubPort = await freePort()
  const relayIntPort = await freePort()

  const hubBaseURL = `http://127.0.0.1:${hubPort}`
  const hubInternalBaseURL = `http://127.0.0.1:${hubInternalPort}`
  const relayPubBaseURL = `http://127.0.0.1:${relayPubPort}`
  const relayIntBaseURL = `http://127.0.0.1:${relayIntPort}`

  const backendDir = join(root, 'backend')
  const hubBin = join(envRoot, process.platform === 'win32' ? 'control-hub.exe' : 'control-hub')
  const relayBin = join(envRoot, process.platform === 'win32' ? 'runtime-relay.exe' : 'runtime-relay')

  const env = {
    envRoot, hubDB, masterKeyFile, jwtKeyFile, relayTokenFile, spoolPath, pwFile,
    adminPassword,
    hubPort, hubInternalPort, relayPubPort, relayIntPort,
    hubBaseURL, hubInternalBaseURL, relayPubBaseURL, relayIntBaseURL,
    backendDir, hubBin, relayBin,
    tokenBytes,
  }

  // Build binaries
  try {
    execSync(`go build -o "${hubBin}" ./cmd/control-hub`, { cwd: backendDir, stdio: buildStdio })
    execSync(`go build -o "${relayBin}" ./cmd/runtime-relay`, { cwd: backendDir, stdio: buildStdio })
  } catch (e) {
    rmSync(envRoot, { recursive: true, force: true })
    throw new Error(`Build failed: ${e.message}`)
  }

  // Apply migrations
  try {
    execSync(`go run ./cmd/devmigrate --db "${hubDB}"`, { cwd: backendDir, stdio: migrateStdio })
  } catch (e) {
    rmSync(envRoot, { recursive: true, force: true })
    throw new Error(`Migration failed: ${e.message}`)
  }

  // Bootstrap admin
  try {
    execSync(
      `go run ./cmd/control-hub bootstrap-admin` +
      ` --db "${hubDB}"` +
      ` --master-key-file "${masterKeyFile}"` +
      ` --jwt-private-key-file "${jwtKeyFile}"` +
      ` --deployment-name "${deploymentName}"` +
      ` --username "admin"` +
      ` --display-name "${displayName}"` +
      ` --password-file "${pwFile}"`,
      { cwd: backendDir, stdio: bootstrapStdio },
    )
  } catch (e) {
    rmSync(envRoot, { recursive: true, force: true })
    throw new Error(`Bootstrap failed: ${e.message}`)
  }

  return env
}

/**
 * Start Control Hub and Runtime Relay processes.
 * @param {object} env - environment object from createFreshEnvironment()
 * @param {object} [opts] - optional { stdio: 'pipe'|'inherit', log: fn }
 * @returns {{hubProc: object, relayProc: object}} spawned processes
 */
export function startHubAndRelay(env, opts = {}) {
  const { stdio = 'pipe', log = null } = opts
  const stdioOpt = stdio === 'inherit' ? 'inherit' : ['ignore', 'pipe', 'pipe']
  const logFn = log || (() => {})

  const hubProc = spawn(env.hubBin, [
    'run',
    '--listen', `127.0.0.1:${env.hubPort}`,
    '--internal-listen', `127.0.0.1:${env.hubInternalPort}`,
    '--public-base-url', env.hubBaseURL,
    '--runtime-api-base', env.relayPubBaseURL,
    '--db', env.hubDB,
    '--master-key-file', env.masterKeyFile,
    '--jwt-private-key-file', env.jwtKeyFile,
    '--relay-internal-url', env.relayIntBaseURL,
    '--relay-service-token-file', env.relayTokenFile,
    '--reconcile-interval', '2s',
  ], { cwd: env.envRoot, stdio: stdioOpt })

  const relayProc = spawn(env.relayBin, [
    '--public-listen', `127.0.0.1:${env.relayPubPort}`,
    '--internal-listen', `127.0.0.1:${env.relayIntPort}`,
    '--spool', env.spoolPath,
    '--hub-usage-url', `${env.hubInternalBaseURL}/internal/v1/usage/request-events:batch`,
    '--hub-service-token-file', env.relayTokenFile,
  ], { cwd: env.envRoot, stdio: stdioOpt })

  if (stdio === 'pipe') {
    hubProc.stdout.on('data', (d) => process.stdout.write(`[hub] ${d}`))
    hubProc.stderr.on('data', (d) => process.stderr.write(`[hub] ${d}`))
    relayProc.stdout.on('data', (d) => process.stdout.write(`[relay] ${d}`))
    relayProc.stderr.on('data', (d) => process.stderr.write(`[relay] ${d}`))
  }

  return { hubProc, relayProc }
}

/**
 * Clean up processes and temp environment.
 * @param {object[]} processes - spawned processes to kill
 * @param {object[]} servers - HTTP servers to close
 * @param {string} envRoot - temp dir to remove
 * @param {boolean} keep - if true, don't clean up
 * @param {function} [log] - optional log function
 */
export function cleanupEnvironment(processes, servers, envRoot, keep, log = null) {
  if (keep) {
    if (log) log(`Keeping temp dir: ${envRoot}`)
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

// --- Deterministic Adapter (Node HTTP server) ---

/**
 * Start a deterministic upstream Adapter HTTP server.
 * This is the Node implementation of backend/test/system/adapter.
 * @param {number} port - port to listen on
 * @returns {object} HTTP server instance
 */
export function startDeterministicAdapter(port) {
  const ttsBytes = Buffer.from([
    0x49, 0x44, 0x33, 0x03, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
  ])

  const server = createServer((req, res) => {
    const url = new URL(req.url, `http://127.0.0.1:${port}`)

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

// --- Same-origin SPA proxy ---

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

/**
 * Start a same-origin SPA proxy that serves production dist/spa at /admin/*
 * and proxies /api/*, /live, /ready to Hub.
 * Per architecture, /internal/* MUST NOT be reachable from the public origin.
 * @param {number} port - port to listen on
 * @param {string} spaDir - path to console/dist/spa
 * @param {number} hubPort - Hub's public port to proxy API requests to
 * @returns {object} HTTP server instance
 */
export function startSpaProxy(port, spaDir, hubPort) {
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

  // Ensure keep-alive connections don't hang — Chromium headless uses
  // keep-alive aggressively and the default Node.js timeouts (5s keep-alive,
  // 60s headers) can cause races where the server closes the connection
  // while Chromium still has pending requests, resulting in ERR_ABORTED.
  server.keepAliveTimeout = 30000
  server.headersTimeout = 35000

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
  const ct = MIME_TYPES[ext] || 'application/octet-stream'
  res.setHeader('Content-Type', ct)
  res.setHeader('Content-Length', data.length)
  res.writeHead(200)
  res.end(data)
}

// --- Artifact meta.json writer ---

/**
 * Write a .meta.json provenance envelope for an artifact.
 * @param {string} artifactsDir - .artifacts/ directory path
 * @param {string} artifactName - artifact filename (e.g. 'backend-test.json')
 * @param {string} root - repo root for git commit
 * @param {string} archRepo - architecture repo path
 * @param {string} command - command that generated the artifact
 * @param {number} exitCode - exit code of the generating command
 */
export function writeMetaJson(artifactsDir, artifactName, root, archRepo, command, exitCode) {
  const artifactPath = join(artifactsDir, artifactName)
  let artifactSha = 'missing'
  if (existsSync(artifactPath)) {
    artifactSha = 'sha256:' + createHash('sha256').update(readFileSync(artifactPath)).digest('hex')
  }
  const now = new Date().toISOString()
  const meta = {
    platformCoreCommit: gitCommit(root),
    architectureCommit: gitCommit(archRepo),
    command,
    artifactSha256: artifactSha,
    startedAt: now,
    completedAt: now,
    exitCode,
  }
  mkdirSync(artifactsDir, { recursive: true })
  writeFileSync(join(artifactsDir, artifactName + '.meta.json'), JSON.stringify(meta, null, 2) + '\n')
}
