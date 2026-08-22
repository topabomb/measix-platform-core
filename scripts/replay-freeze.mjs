#!/usr/bin/env node
/**
 * CAP-C7-002 — Clean-environment replay from freeze manifest.
 *
 * Per architecture §14 CAP-C7-002:
 *   "clean-environment replay from manifest"
 *
 * Two-phase freeze flow:
 *   1. freeze-manifest.mjs generates a candidate manifest with CAP-C7-002=NOT_EXECUTED
 *   2. This script (replay-freeze.mjs) performs a real clean-environment replay:
 *      a. Verify exact current checkout matches manifest
 *      b. Verify production admin build hash matches manifest (using same algorithm as freeze-manifest)
 *      c. Verify deterministic adapter version matches manifest
 *      d. Spin up a fresh temp environment (fresh DB, Hub, Relay, Adapter)
 *      e. Apply migrations to fresh DB
 *      f. Bootstrap admin
 *      g. Start Hub + Relay + deterministic Adapter
 *      h. Verify Hub/Relay health
 *      i. Run a real smoke test: admin login + system status
 *      j. Run the deterministic T4.1 Golden Path + Test Client four capabilities + Usage closure
 *      k. Generate a replay artifact with the result
 *      l. Update the manifest: CAP-C7-002=PASS + replay artifact hash
 *
 * Usage:
 *   node scripts/replay-freeze.mjs
 */
import { createHash } from 'node:crypto'
import { existsSync, readFileSync, writeFileSync, mkdirSync, mkdtempSync, rmSync, readdirSync, statSync } from 'node:fs'
import { join, resolve, relative, extname } from 'node:path'
import { execSync, spawn } from 'node:child_process'
import { tmpdir } from 'node:os'
import { randomFillSync } from 'node:crypto'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const net = require('node:net')
const http = require('node:http')
const { createServer } = http
const { request: httpRequest } = http

const ROOT = resolve(import.meta.dirname, '..')
const ARTIFACTS_DIR = join(ROOT, '.artifacts')
const ARCH_REPO = resolve(ROOT, '..', 'measix-architecture')
const ADAPTER_SOURCE = join(ROOT, 'backend', 'test', 'system', 'adapter', 'adapter.go')
const ADAPTER_TEST = join(ROOT, 'backend', 'test', 'system', 'adapter', 'adapter_test.go')
const CLIENT_SOURCE = join(ROOT, 'backend', 'test', 'system', 'client', 'client.go')

function log(msg) {
  console.log(`[replay] ${msg}`)
}

function gitCommit(cwd) {
  try { return execSync('git rev-parse HEAD', { cwd, encoding: 'utf-8' }).trim() }
  catch { return 'unknown' }
}

function gitDirty(cwd) {
  try { return execSync('git status --porcelain', { cwd, encoding: 'utf-8' }).trim().length > 0 }
  catch { return true }
}

// Use the SAME algorithm as freeze-manifest.mjs for build hash
function collectFiles(dir) {
  const out = []
  for (const entry of readdirSync(dir).sort()) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) out.push(...collectFiles(full))
    else out.push(full)
  }
  return out
}

function adminBuildHash() {
  const distDir = join(ROOT, 'console', 'dist', 'spa')
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

function deterministicAdapterVersion() {
  const hash = createHash('sha256')
  if (existsSync(ADAPTER_SOURCE)) hash.update(readFileSync(ADAPTER_SOURCE).toString('utf-8').replace(/\r\n/g, '\n'))
  hash.update('\0')
  if (existsSync(ADAPTER_TEST)) hash.update(readFileSync(ADAPTER_TEST).toString('utf-8').replace(/\r\n/g, '\n'))
  hash.update('\0')
  if (existsSync(CLIENT_SOURCE)) hash.update(readFileSync(CLIENT_SOURCE).toString('utf-8').replace(/\r\n/g, '\n'))
  return 'sha256:' + hash.digest('hex')
}

// --- Read manifest ---
const manifestPath = join(ROOT, 'docs', 's0-freeze-manifest.json')
if (!existsSync(manifestPath)) {
  console.error('ERROR: docs/s0-freeze-manifest.json does not exist. Generate it first.')
  process.exit(1)
}

const manifest = JSON.parse(readFileSync(manifestPath, 'utf-8'))
const errors = []

log(`manifest: ${manifest.manifest}`)
log(`platform core commit: ${manifest.platformCoreCommit}`)
log(`architecture commit: ${manifest.architectureCommit}`)
log(`admin build hash: ${manifest.adminBuildHash}`)

// --- 1. Verify exact current checkout ---
const currentCommit = gitCommit(ROOT)
if (manifest.platformCoreCommit !== currentCommit) {
  errors.push(`platformCoreCommit mismatch: manifest=${manifest.platformCoreCommit} current=${currentCommit}`)
}

// --- 2. Verify architecture commit ---
const archCommit = gitCommit(ARCH_REPO)
if (manifest.architectureCommit !== archCommit) {
  errors.push(`architectureCommit mismatch: manifest=${manifest.architectureCommit} current=${archCommit}`)
}

// --- 3. Verify production admin build hash (using same algorithm as freeze-manifest) ---
const buildHash = adminBuildHash()
if (buildHash === 'not-built') {
  errors.push('Admin production build not found. Run "make console-build" first.')
} else if (manifest.adminBuildHash !== buildHash) {
  errors.push(`adminBuildHash mismatch: manifest=${manifest.adminBuildHash} current=${buildHash}`)
}

// --- 4. Verify deterministic adapter version ---
const adapterVersion = deterministicAdapterVersion()
if (manifest.deterministicAdapterVersion && manifest.deterministicAdapterVersion !== adapterVersion) {
  errors.push(`deterministicAdapterVersion mismatch: manifest=${manifest.deterministicAdapterVersion} current=${adapterVersion}`)
}

// --- 5. Verify all required scenarios are PASS, EXCEPT CAP-C7-002 ---
// CAP-C7-002 is allowed to be NOT_EXECUTED in the candidate manifest.
// This replay script will set it to PASS after successful replay.
const notPass = manifest.scenarioResults.filter(s => s.required && s.result !== 'PASS' && s.id !== 'CAP-C7-002')
if (notPass.length > 0) {
  errors.push(`${notPass.length} required scenarios are not PASS (excluding CAP-C7-002):`)
  for (const s of notPass) errors.push(`  ${s.id} ${s.name}: ${s.result}`)
}

// --- 6. Verify adapter qualification ---
if (manifest.realAdapterQualificationStatus !== 'VERIFIED') {
  errors.push(`realAdapterQualificationStatus is ${manifest.realAdapterQualificationStatus}, expected VERIFIED`)
}

// --- 7. Verify resource baseline ---
if (manifest.resourceBaselineStatus !== 'GREEN') {
  errors.push(`resourceBaselineStatus is ${manifest.resourceBaselineStatus}, expected GREEN`)
}

if (errors.length > 0) {
  console.error('ERROR: Clean replay validation failed:')
  for (const e of errors) console.error(`  ${e}`)
  process.exit(1)
}

// --- 8. Fresh environment replay ---
// Spin up a fresh Hub + Relay + Adapter to verify the system actually works
// from a clean environment — not just that the manifest is valid.
log('Starting fresh environment replay...')

const envRoot = mkdtempSync(join(tmpdir(), 'measix-replay-'))
const hubDB = join(envRoot, 'hub.db')
const masterKeyFile = join(envRoot, 'master.key')
const jwtKeyFile = join(envRoot, 'jwt-ed25519.seed')
const relayTokenFile = join(envRoot, 'relay-service.token')
const spoolPath = join(envRoot, 'relay-spool.db')
const pwFile = join(envRoot, 'admin-password.txt')

function randomBytes(n) {
  const buf = Buffer.alloc(n)
  randomFillSync(buf)
  return buf
}

writeFileSync(masterKeyFile, randomBytes(32), { mode: 0o600 })
writeFileSync(jwtKeyFile, randomBytes(32), { mode: 0o600 })
const tokenBytes = randomBytes(32)
writeFileSync(relayTokenFile, Buffer.from(tokenBytes.toString('hex') + '\n'), { mode: 0o600 })
const adminPassword = 'replay-' + tokenBytes.toString('hex').slice(0, 8)
writeFileSync(pwFile, adminPassword + '\n', { mode: 0o600 })

// Allocate free ports
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

const hubPort = await freePort()
const hubInternalPort = await freePort()
const relayPubPort = await freePort()
const relayIntPort = await freePort()

const hubBaseURL = `http://127.0.0.1:${hubPort}`
const hubInternalBaseURL = `http://127.0.0.1:${hubInternalPort}`
const relayPubBaseURL = `http://127.0.0.1:${relayPubPort}`
const relayIntBaseURL = `http://127.0.0.1:${relayIntPort}`

const backendDir = join(ROOT, 'backend')
const hubBin = join(envRoot, process.platform === 'win32' ? 'control-hub.exe' : 'control-hub')
const relayBin = join(envRoot, process.platform === 'win32' ? 'runtime-relay.exe' : 'runtime-relay')

try {
  log('Building control-hub and runtime-relay binaries...')
  execSync(`go build -o "${hubBin}" ./cmd/control-hub`, { cwd: backendDir, stdio: 'pipe' })
  execSync(`go build -o "${relayBin}" ./cmd/runtime-relay`, { cwd: backendDir, stdio: 'pipe' })
} catch (e) {
  console.error('Build failed:', e.message)
  rmSync(envRoot, { recursive: true, force: true })
  process.exit(1)
}

try {
  log('Applying migrations to fresh DB...')
  execSync(`go run ./cmd/devmigrate --db "${hubDB}"`, { cwd: backendDir, stdio: 'pipe' })
} catch (e) {
  console.error('Migration failed:', e.message)
  rmSync(envRoot, { recursive: true, force: true })
  process.exit(1)
}

try {
  log('Bootstrapping admin...')
  execSync(
    `go run ./cmd/control-hub bootstrap-admin` +
    ` --db "${hubDB}"` +
    ` --master-key-file "${masterKeyFile}"` +
    ` --jwt-private-key-file "${jwtKeyFile}"` +
    ` --deployment-name "REPLAY-TEST"` +
    ` --username "admin"` +
    ` --display-name "Replay Admin"` +
    ` --password-file "${pwFile}"`,
    { cwd: backendDir, stdio: 'pipe' },
  )
} catch (e) {
  console.error('Bootstrap failed:', e.message)
  rmSync(envRoot, { recursive: true, force: true })
  process.exit(1)
}

// Start Hub
log('Starting Control Hub...')
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
], { cwd: envRoot, stdio: ['ignore', 'pipe', 'pipe'] })

// Start Relay
log('Starting Runtime Relay...')
const relayProc = spawn(relayBin, [
  '--public-listen', `127.0.0.1:${relayPubPort}`,
  '--internal-listen', `127.0.0.1:${relayIntPort}`,
  '--spool', spoolPath,
  '--hub-usage-url', `${hubInternalBaseURL}/internal/v1/usage/request-events:batch`,
  '--hub-service-token-file', relayTokenFile,
], { cwd: envRoot, stdio: ['ignore', 'pipe', 'pipe'] })

// Wait for Hub and Relay to be ready
async function waitFor(url, label, maxWait = 30000) {
  const start = Date.now()
  while (Date.now() - start < maxWait) {
    try {
      const resp = await fetch(url)
      if (resp.ok || resp.status === 401 || resp.status === 404) {
        log(`${label} ready`)
        return true
      }
    } catch {}
    await new Promise(r => setTimeout(r, 500))
  }
  return false
}

const hubReady = await waitFor(`${hubBaseURL}/live`, 'Hub')
const relayReady = await waitFor(`${relayIntBaseURL}/live`, 'Relay')

if (!hubReady || !relayReady) {
  console.error('ERROR: Hub or Relay not ready in fresh environment')
  try { hubProc.kill('SIGTERM') } catch {}
  try { relayProc.kill('SIGTERM') } catch {}
  rmSync(envRoot, { recursive: true, force: true })
  process.exit(1)
}

// --- 9. Run real replay tests ---
// Per architecture CAP-C7-002: clean replay must run against the SAME
// fresh environment. This means:
//   a. Production Playwright Browser Golden Path against this fresh Hub/Relay
//   b. Deterministic Go candidate system tests (these start their own HubEnv,
//      proving the system works from a completely clean environment)
//   c. Topology security + admin login + system status smoke tests
//
// The Playwright test runs against the SAME fresh environment we just started.
// The Go candidate tests start their own independent HubEnv, which proves
// the system can be deployed from scratch (not just that our running instance works).

let browserGoldenPathPassed = false
let browserGoldenPathOutput = ''
let replayTestsPassed = false
let replayTestOutput = ''

// --- 9a. Run Playwright Browser Golden Path against this fresh environment ---
// We need a SPA proxy and adapter for the browser test.
// Start a deterministic adapter on a free port.
const adapterPort = await freePort()
const spaPort = await freePort()
const adapterBaseURL = `http://127.0.0.1:${adapterPort}`
const spaBaseURL = `http://127.0.0.1:${spaPort}`

// Start a simple deterministic adapter (inline HTTP server)
const adapterServer = createRequireAdapter(adapterPort)

// Start SPA proxy (same-origin as browser expects)
const spaDir = join(ROOT, 'console', 'dist', 'spa')
if (existsSync(spaDir)) {
  const spaServer = createSpaProxy(spaPort, spaDir, hubPort)
  await waitFor(spaBaseURL, 'SPA')

  try {
    log('Running Playwright Browser Golden Path against fresh environment...')
    const e2eEnv = {
      ...process.env,
      MEASIX_E2E_BASE_URL: spaBaseURL,
      MEASIX_E2E_HUB_BASE_URL: hubBaseURL,
      MEASIX_E2E_ADAPTER_URL: adapterBaseURL,
      MEASIX_E2E_ADMIN_PASSWORD: adminPassword,
      PLAYWRIGHT_BASE_URL: spaBaseURL,
    }
    browserGoldenPathOutput = execSync(
      'npx playwright test --reporter=line',
      { cwd: join(ROOT, 'console'), encoding: 'utf-8', stdio: ['pipe', 'pipe', 'pipe'], env: e2eEnv, timeout: 600000 },
    )
    browserGoldenPathPassed = true
    log('Browser Golden Path PASSED')
  } catch (e) {
    browserGoldenPathOutput = (e.stdout || '') + (e.stderr || '')
    log('Browser Golden Path FAILED')
    console.error('Browser test failure:', browserGoldenPathOutput.slice(0, 2000))
  }

  try { spaServer.close() } catch {}
} else {
  log('WARNING: SPA build not found at console/dist/spa — skipping Browser Golden Path.')
  log('Run "make console-build" before clean replay to include browser tests.')
  browserGoldenPathPassed = false
}

try { adapterServer.close() } catch {}

// --- 9b. Run deterministic Go candidate tests ---
// These tests start their OWN HubEnv (independent of the one we started above).
// They prove the system can be deployed from a completely clean environment.
try {
  log('Running deterministic T4.1 replay tests (Golden Path + Test Client + Usage closure)...')
  replayTestOutput = execSync(
    `go test -tags=candidate -run "TestCAPC6001GoldenPath|TestCAPC6002TestClientFourCapabilities|TestCAPC6003UsageClosure" -v -timeout 10m ./test/system/scenarios/`,
    { cwd: backendDir, encoding: 'utf-8', stdio: ['pipe', 'pipe', 'pipe'] },
  )
  replayTestsPassed = true
  log('Deterministic replay tests PASSED')
} catch (e) {
  log('Deterministic replay tests FAILED')
  replayTestOutput = (e.stdout || '') + (e.stderr || '')
  console.error('Replay test failure:', replayTestOutput.slice(0, 2000))
}

// Also verify topology security: /internal/* not reachable from public listener
let topologySecurityPassed = false
try {
  log('Verifying topology security: /internal/* not reachable from public listener...')
  const internalResp = await fetch(`${hubBaseURL}/internal/v1/usage/request-events:batch`)
  if (internalResp.status === 404) {
    topologySecurityPassed = true
    log('Topology security: PASS (/internal/* returns 404 on public listener)')
  } else {
    log(`Topology security: FAIL (/internal/* returned ${internalResp.status} on public listener)`)
  }
} catch (e) {
  log(`Topology security check error: ${e.message}`)
}

// Smoke test: login as admin
log('Smoke test: admin login...')
let loginOk = false
try {
  const loginResp = await fetch(`${hubBaseURL}/api/admin/v1/session/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: adminPassword }),
  })
  if (loginResp.ok) {
    const result = await loginResp.json()
    if (result.csrfToken) {
      loginOk = true
      log('Admin login: OK')
    }
  }
} catch (e) {
  log(`Admin login error: ${e.message}`)
}

// Smoke test: system status
log('Smoke test: system status...')
let statusOk = false
try {
  const setCookie = (await fetch(`${hubBaseURL}/api/admin/v1/session/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: adminPassword }),
  })).headers.get('set-cookie') || ''
  const cookieMatch = setCookie.match(/measix-admin-session=([^;]+)/)
  const cookie = cookieMatch ? `measix-admin-session=${cookieMatch[1]}` : ''
  const csrfMatch = setCookie.match(/measix-csrf=([^;]+)/)
  const csrfToken = csrfMatch ? csrfMatch[1] : ''

  const statusResp = await fetch(`${hubBaseURL}/api/admin/v1/system/status`, {
    headers: { 'Cookie': cookie, 'X-CSRF-Token': csrfToken },
  })
  if (statusResp.ok) {
    const status = await statusResp.json()
    log(`System status: runtime=${status.runtimeStatus} relay=${status.relayStatus}`)
    statusOk = true
  }
} catch (e) {
  log(`System status error: ${e.message}`)
}

// Cleanup
log('Cleaning up fresh environment...')
try { hubProc.kill('SIGTERM') } catch {}
try { relayProc.kill('SIGTERM') } catch {}
setTimeout(() => {
  try { hubProc.kill('SIGKILL') } catch {}
  try { relayProc.kill('SIGKILL') } catch {}
  rmSync(envRoot, { recursive: true, force: true })
}, 3000)

if (!browserGoldenPathPassed || !replayTestsPassed || !loginOk || !statusOk || !topologySecurityPassed) {
  console.error('ERROR: Clean replay failed:')
  if (!browserGoldenPathPassed) console.error('  - Browser Golden Path did not pass')
  if (!replayTestsPassed) console.error('  - Deterministic replay tests did not pass')
  if (!loginOk) console.error('  - Admin login failed in fresh environment')
  if (!statusOk) console.error('  - System status check failed')
  if (!topologySecurityPassed) console.error('  - Topology security check failed')
  process.exit(1)
}

// --- 10. Generate replay artifact ---
const replayArtifact = {
  status: 'PASS',
  replayedAt: new Date().toISOString(),
  platformCoreCommit: currentCommit,
  architectureCommit: archCommit,
  adminBuildHash: buildHash,
  deterministicAdapterVersion: adapterVersion,
  freshEnvironment: {
    hubBaseURL,
    hubInternalBaseURL,
    relayPubBaseURL,
    dbPath: hubDB,
    hubReady,
    relayReady,
    adminLoginOk: loginOk,
    systemStatusOk: statusOk,
    topologySecurityPassed,
    spaBaseURL,
    adapterBaseURL,
  },
  browserGoldenPath: {
    passed: browserGoldenPathPassed,
    outputHash: 'sha256:' + createHash('sha256').update(browserGoldenPathOutput).digest('hex'),
  },
  replayTests: {
    passed: replayTestsPassed,
    testsRun: [
      'TestCAPC6001GoldenPath',
      'TestCAPC6002TestClientFourCapabilities',
      'TestCAPC6003UsageClosure',
    ],
    outputHash: 'sha256:' + createHash('sha256').update(replayTestOutput).digest('hex'),
  },
  manifestScenarioResults: manifest.scenarioResults.filter(s => s.required),
}

mkdirSync(ARTIFACTS_DIR, { recursive: true })
const replayPath = join(ARTIFACTS_DIR, 'replay-artifact.json')
writeFileSync(replayPath, JSON.stringify(replayArtifact, null, 2) + '\n')
log(`wrote ${replayPath}`)

// --- 11. Update manifest: set CAP-C7-002=PASS and record replay artifact hash ---
const replayArtifactHash = 'sha256:' + createHash('sha256').update(readFileSync(replayPath)).digest('hex')

manifest.scenarioResults = manifest.scenarioResults.map(s => {
  if (s.id === 'CAP-C7-002') {
    return { ...s, result: 'PASS' }
  }
  return s
})
manifest.replayArtifactRef = '.artifacts/replay-artifact.json'
manifest.replayArtifactHash = replayArtifactHash
manifest.replayCompletedAt = new Date().toISOString()

writeFileSync(manifestPath, JSON.stringify(manifest, null, 2) + '\n')
log(`updated ${relative(ROOT, manifestPath)}: CAP-C7-002=PASS`)

log('Clean replay: PASS')
log('CAP-C7-002: PASS')

// --- Helper functions for browser golden path replay ---

function createRequireAdapter(port) {
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
          res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' })
          const chunks = [
            `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"}}]}`,
            `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"content":"hel"}}]}`,
            `data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"content":"lo"}}]}`,
            `data: [DONE]`,
          ]
          for (const c of chunks) { res.write(c + '\n\n') }
          res.end()
        } else {
          res.writeHead(200, { 'Content-Type': 'application/json' })
          res.end(JSON.stringify({ id: '1', object: 'chat.completion', choices: [{ index: 0, message: { role: 'assistant', content: 'hello' }, finish_reason: 'stop' }] }))
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
      res.writeHead(404, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ error: 'not found' }))
    })
  })
  server.listen(port, '127.0.0.1')
  return server
}

function createSpaProxy(port, spaDir, hubPort) {
  const server = createServer((req, res) => {
    const url = new URL(req.url, `http://127.0.0.1:${port}`)
    const path = url.pathname
    if (path.startsWith('/api/') || path === '/live' || path === '/ready') {
      const proxyReq = httpRequest({
        hostname: '127.0.0.1', port: hubPort, path: req.url, method: req.method, headers: req.headers,
      }, (proxyResp) => {
        res.writeHead(proxyResp.statusCode, proxyResp.headers)
        proxyResp.pipe(res)
      })
      proxyReq.on('error', () => {
        if (!res.headersSent) { res.writeHead(502, { 'Content-Type': 'application/json' }); res.end(JSON.stringify({ error: 'proxy error' })) }
      })
      req.pipe(proxyReq)
      return
    }
    if (path === '/admin' || path.startsWith('/admin/')) {
      let filePath = path.replace(/^\/admin\/?/, '')
      if (!filePath) filePath = 'index.html'
      const full = join(spaDir, filePath)
      if (existsSync(full) && statSync(full).isFile()) {
        const data = readFileSync(full)
        const ext = extname(filePath)
        const mimeTypes = {
          '.html': 'text/html; charset=utf-8', '.js': 'application/javascript; charset=utf-8',
          '.css': 'text/css; charset=utf-8', '.json': 'application/json; charset=utf-8',
          '.svg': 'image/svg+xml', '.png': 'image/png', '.jpg': 'image/jpeg',
          '.ico': 'image/x-icon', '.woff': 'font/woff', '.woff2': 'font/woff2', '.ttf': 'font/ttf',
        }
        res.setHeader('Content-Type', mimeTypes[ext] || 'application/octet-stream')
        res.writeHead(200)
        res.end(data)
        return
      }
      if (!filePath.startsWith('assets/') && !extname(filePath)) {
        const index = join(spaDir, 'index.html')
        if (existsSync(index)) {
          const data = readFileSync(index)
          res.setHeader('Content-Type', 'text/html; charset=utf-8')
          res.writeHead(200)
          res.end(data)
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
  server.listen(port, '127.0.0.1')
  return server
}
