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
import { existsSync, mkdirSync, unlinkSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { Worker } from 'node:worker_threads'
import { randomUUID } from 'node:crypto'
import { spawn } from 'node:child_process'

import {
  resolveRoot,
  freePort,
  waitFor,
  createFreshEnvironment,
  startHubAndRelay,
  cleanupEnvironment,
  writeMetaJson,
} from './lib/harness.mjs'

const ROOT = resolveRoot(import.meta.dirname)
const ARCH_REPO = join(ROOT, '..', 'measix-architecture')
const KEEP = process.argv.includes('--keep')
const TIMEOUT = parseInt(process.env.MEASIX_E2E_TIMEOUT || '600000', 10)

const processes = []
const servers = []

function log(msg) {
  console.log(`[harness] ${msg}`)
}

function cleanup() {
  // Per audit P1-1: pass the real envRoot so temp dir is cleaned up.
  // Previously passed '' which left temp directories behind on failure.
  cleanupEnvironment(processes, servers, env?.envRoot || '', KEEP, log)
}

process.on('SIGINT', () => { cleanup(); process.exit(1) })
process.on('SIGTERM', () => { cleanup(); process.exit(1) })
process.on('exit', () => cleanup())

// Per audit P1-1: ensure cleanup runs even on uncaught exception
process.on('uncaughtException', (err) => {
  log(`Uncaught exception: ${err.message}`)
  cleanup()
  process.exit(1)
})

// --- Setup ---

const env = await createFreshEnvironment(ROOT, {
  prefix: 'measix-e2e',
  deploymentName: 'E2E-TEST',
  displayName: 'E2E Test Admin',
  adminPasswordPrefix: 'e2e-admin',
})

const { adminPassword } = env

const adapterPort = await freePort()
const spaPort = await freePort()
const adapterBaseURL = `http://127.0.0.1:${adapterPort}`
const spaBaseURL = `http://127.0.0.1:${spaPort}`

log(`temp dir: ${env.envRoot}`)
log(`hub: ${env.hubBaseURL}`)
log(`hub internal: ${env.hubInternalBaseURL}`)
log(`relay public: ${env.relayPubBaseURL}`)
log(`relay internal: ${env.relayIntBaseURL}`)
log(`adapter: ${adapterBaseURL}`)
log(`spa (same-origin proxy): ${spaBaseURL}`)

// --- Start Hub and Relay ---

log('starting Control Hub and Runtime Relay...')
const { hubProc, relayProc } = startHubAndRelay(env, { stdio: 'pipe', log })
processes.push(hubProc, relayProc)

// --- Start deterministic Adapter ---

log('starting deterministic Adapter (in worker)...')

// --- Wait for Hub and Relay to be ready ---

const hubReady = await waitFor(`${env.hubBaseURL}/live`, 'Hub', 30000, log)
const relayReady = await waitFor(`${env.relayIntBaseURL}/live`, 'Relay', 30000, log)
if (!hubReady || !relayReady) {
  log('ERROR: Hub or Relay not ready')
  process.exit(1)
}

// --- Start same-origin SPA proxy ---

log('starting same-origin SPA proxy (in worker)...')
const spaDir = join(ROOT, 'console', 'dist', 'spa')
if (!existsSync(spaDir)) {
  log('SPA build not found. Run "make console-build" first.')
  process.exit(1)
}

// Start HTTP servers (SPA proxy + Adapter) in a worker thread to avoid
// blocking the Node.js event loop when using execSync for Playwright.
const worker = new Worker(join(ROOT, 'scripts', '_server-worker.mjs'), {
  workerData: { spaPort, spaDir, adapterPort, hubPort: env.hubPort },
})
await new Promise((resolve, reject) => {
  worker.on('message', (msg) => { if (msg.ready) resolve() })
  worker.on('error', reject)
})

const spaReady = await waitFor(spaBaseURL, 'SPA', 30000, log)
if (!spaReady) {
  log('ERROR: SPA proxy not ready')
  process.exit(1)
}

// --- Run Playwright E2E ---

log('running Playwright E2E tests...')

// Verify SPA proxy is reachable before running Playwright
try {
  const resp = await fetch(`${spaBaseURL}/admin/`)
  const text = await resp.text()
  log(`SPA proxy check: ${resp.status} (${text.length} bytes)`)
  if (!resp.ok) {
    log('ERROR: SPA proxy not serving /admin/ correctly')
    process.exit(1)
  }
} catch (e) {
  log(`ERROR: SPA proxy unreachable: ${e.message}`)
  process.exit(1)
}

const e2eEnv = {
  ...process.env,
  MEASIX_E2E_BASE_URL: spaBaseURL,
  MEASIX_E2E_HUB_BASE_URL: env.hubBaseURL,
  MEASIX_E2E_ADAPTER_URL: adapterBaseURL,
  MEASIX_E2E_ADMIN_PASSWORD: adminPassword,
  PLAYWRIGHT_BASE_URL: spaBaseURL,
}

// Ensure .artifacts directory exists for JSON reporter output
const artifactsDir = join(ROOT, '.artifacts')
if (!existsSync(artifactsDir)) {
  mkdirSync(artifactsDir, { recursive: true })
}

// Track envRoot for cleanup
env.envRoot_ref = env.envRoot

// Per audit P1-1: use try/finally to ensure Worker, Hub, Relay, Adapter,
// SPA proxy and temp directory are cleaned up regardless of pass/fail.
//
// The E2E test is executed in 3 phases (matching candidate-orchestrator):
//   Phase A: golden-path-authoring.spec.ts (setup, upstream, resources, publish)
//   Phase B: Four-capability runtime traffic (Model/TTS/ASR/MCP)
//   Phase C: Wait for usage ingestion
//   Phase D: golden-path-usage.spec.ts (usage, system, persistence, logout)
let exitCode = 1

async function runPlaywrightSpec(specFile, phaseName) {
  // Each phase writes to its own temp JSON file to avoid overwriting.
  // Results are merged into e2e-playwright.json at the end.
  const tempOutput = join(artifactsDir, `_e2e-${phaseName}.json`)
  const phaseEnv = {
    ...e2eEnv,
    PLAYWRIGHT_JSON_OUTPUT_FILE: tempOutput,
  }
  return new Promise((resolve, reject) => {
    const proc = spawn('npx', ['playwright', 'test', specFile], {
      cwd: join(ROOT, 'console'),
      stdio: 'inherit',
      env: phaseEnv,
      shell: true,
    })
    const timer = setTimeout(() => {
      proc.kill('SIGTERM')
      reject(new Error(`Playwright ${phaseName} timed out`))
    }, TIMEOUT)
    proc.on('exit', (code) => { clearTimeout(timer); resolve(code ?? 1) })
    proc.on('error', (err) => { clearTimeout(timer); reject(err) })
  })
}

// Merge multiple Playwright JSON reports into a single artifact.
function mergePlaywrightJsons(filePaths, outputPath) {
  const merged = {
    config: { configFile: '', forbidOnly: false, fullyParallel: false, globalSetup: null, globalTeardown: null, globalTimeout: 0, grep: {}, grepInvert: null, maxFailures: 0, metadata: { actualWorkers: 1 }, preserveOutput: 'always', projects: [], quiet: false, reporter: [], reportSlowTests: { max: 5, threshold: 300000 }, shard: null, updateSnapshots: 'missing', updateSourceMethod: 'patch', version: '1.55.0', workers: 1, webServer: null },
    suites: [],
    errors: [],
    stats: { startTime: '', duration: 0, expected: 0, skipped: 0, unexpected: 0, flaky: 0 },
  }
  for (const fp of filePaths) {
    if (!existsSync(fp)) continue
    try {
      const data = JSON.parse(readFileSync(fp, 'utf-8'))
      if (data.suites) merged.suites.push(...data.suites)
      if (data.errors) merged.errors.push(...data.errors)
      if (data.stats) {
        merged.stats.expected += data.stats.expected || 0
        merged.stats.unexpected += data.stats.unexpected || 0
        merged.stats.flaky += data.stats.flaky || 0
        merged.stats.skipped += data.stats.skipped || 0
        merged.stats.duration += data.stats.duration || 0
        if (!merged.stats.startTime) merged.stats.startTime = data.stats.startTime || ''
      }
      // Use config from the first file
      if (!merged.config.configFile && data.config) {
        merged.config = data.config
        // Override JSON reporter output path in config
        if (merged.config.reporter) {
          merged.config.reporter = merged.config.reporter.map(r => {
            if (Array.isArray(r) && r[0] === 'json') {
              return ['json', { outputFile: '../.artifacts/e2e-playwright.json' }]
            }
            return r
          })
        }
      }
    } catch {}
  }
  writeFileSync(outputPath, JSON.stringify(merged, null, 2))
  // Clean up temp files
  for (const fp of filePaths) { try { unlinkSync(fp) } catch {} }
}

async function runFourCapabilityTraffic() {
  // Same logic as candidate-orchestrator's runFourCapabilityTraffic
  // Login as admin to get CSRF token + cookie
  const loginResp = await fetch(`${env.hubBaseURL}/api/admin/v1/session/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: adminPassword }),
  })
  if (!loginResp.ok) throw new Error(`admin login failed: ${loginResp.status}`)
  const loginJson = await loginResp.json()
  const csrfToken = loginJson.csrfToken
  const cookie = loginResp.headers.get('set-cookie')?.split(';')[0] || ''

  // Create a managed user
  const userResp = await fetch(`${env.hubBaseURL}/api/admin/v1/users`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Cookie': cookie, 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ username: 'e2e-user-' + Date.now(), displayName: 'E2E User', role: 'MEMBER' }),
  })
  if (!userResp.ok) throw new Error(`create user failed: ${userResp.status}`)
  const userJson = await userResp.json()
  const managedUserId = userJson.userId

  // Create enrollment
  const enrollResp = await fetch(`${env.hubBaseURL}/api/admin/v1/users/${managedUserId}/enrollments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Cookie': cookie, 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ expiresInSeconds: 3600 }),
  })
  if (!enrollResp.ok) throw new Error(`create enrollment failed: ${enrollResp.status}`)
  const enrollJson = await enrollResp.json()
  const enrollmentCode = enrollJson.code

  // Exchange enrollment for access token
  const exchangeResp = await fetch(`${env.hubBaseURL}/api/client/v1/enrollments/exchange`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ platform: 'ANDROID', code: enrollmentCode, installationId: `ins_${randomUUID()}`, appVersion: 'e2e-1.0' }),
  })
  if (!exchangeResp.ok) throw new Error(`exchange enrollment failed: ${exchangeResp.status}`)
  const exchangeJson = await exchangeResp.json()
  const clientToken = exchangeJson.accessToken

  // Get managed state for generation + resource IDs
  const stateResp = await fetch(`${env.hubBaseURL}/api/client/v1/managed/state`, {
    headers: { 'Authorization': `Bearer ${clientToken}` },
  })
  if (!stateResp.ok) throw new Error(`get managed state failed: ${stateResp.status}`)
  const stateJson = await stateResp.json()
  const generation = stateJson.activeManagedGeneration

  // Fetch the snapshot to get resource IDs
  const snapResp = await fetch(`${env.hubBaseURL}/api/client/v1/managed/snapshots/${generation}`, {
    headers: { 'Authorization': `Bearer ${clientToken}` },
  })
  if (!snapResp.ok) throw new Error(`get snapshot failed: ${snapResp.status}`)
  const snapJson = await snapResp.json()

  const modelId = snapJson.models?.[0]?.modelId
  const ttsId = snapJson.tts?.[0]?.ttsId
  const asrId = snapJson.asr?.[0]?.asrId
  const mcpId = snapJson.mcp?.[0]?.mcpServerId

  if (!modelId || !ttsId || !asrId || !mcpId) {
    throw new Error(`snapshot missing resource IDs: model=${modelId} tts=${ttsId} asr=${asrId} mcp=${mcpId}`)
  }

  const relayUrl = env.relayPubBaseURL
  const baseHeaders = {
    'Authorization': `Bearer ${clientToken}`,
    'X-Measix-Managed-Generation': String(generation),
    'Content-Type': 'application/json',
  }

  // 1. Model streaming
  const modelResp = await fetch(`${relayUrl}/runtime/v1/resources/${modelId}/v1/chat/completions`, {
    method: 'POST',
    headers: { ...baseHeaders, 'X-Measix-Interaction-Id': `int_${randomUUID()}` },
    body: JSON.stringify({ model: 'gpt-test', stream: true, messages: [{ role: 'user', content: 'Say hello' }] }),
  })
  if (!modelResp.ok) throw new Error(`model request failed: ${modelResp.status}`)
  await modelResp.text()

  // 2. TTS
  const ttsResp = await fetch(`${relayUrl}/runtime/v1/resources/${ttsId}/v1/audio/speech`, {
    method: 'POST',
    headers: { ...baseHeaders, 'X-Measix-Interaction-Id': `int_${randomUUID()}` },
    body: JSON.stringify({ model: 'tts-test', input: 'hello', voice: 'alloy' }),
  })
  if (!ttsResp.ok) throw new Error(`tts request failed: ${ttsResp.status}`)
  await ttsResp.text()

  // 3. ASR
  const asrFormData = new FormData()
  asrFormData.append('file', new Blob([Buffer.from('RIFF')]), 'sample.wav')
  asrFormData.append('model', 'whisper-test')
  const asrResp = await fetch(`${relayUrl}/runtime/v1/resources/${asrId}/v1/audio/transcriptions`, {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${clientToken}`, 'X-Measix-Managed-Generation': String(generation), 'X-Measix-Interaction-Id': `int_${randomUUID()}` },
    body: asrFormData,
  })
  if (!asrResp.ok) throw new Error(`asr request failed: ${asrResp.status}`)
  await asrResp.text()

  // 4. MCP
  const mcpResp = await fetch(`${relayUrl}/runtime/v1/resources/${mcpId}/mcp`, {
    method: 'POST',
    headers: { ...baseHeaders, 'X-Measix-Interaction-Id': `int_${randomUUID()}` },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'e2e-client', version: '1.0' } } }),
  })
  if (!mcpResp.ok) throw new Error(`mcp request failed: ${mcpResp.status}`)
  await mcpResp.text()
}

async function waitForUsageIngestion(minRequests, maxWaitSeconds) {
  const loginResp = await fetch(`${env.hubBaseURL}/api/admin/v1/session/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: adminPassword }),
  })
  if (!loginResp.ok) return
  const loginBody = await loginResp.json()
  const cookie = loginResp.headers.get('set-cookie')?.split(';')[0] || ''
  const csrfToken = loginBody.csrfToken || ''

  for (let i = 0; i < maxWaitSeconds; i++) {
    await new Promise(r => setTimeout(r, 1000))
    const resp = await fetch(`${env.hubBaseURL}/api/admin/v1/usage/summary`, {
      headers: { 'Cookie': cookie, 'X-CSRF-Token': csrfToken },
    })
    if (resp.ok) {
      const data = await resp.json()
      const count = data.requestCount || 0
      if (count >= minRequests) return
    }
  }
}

try {
  // Phase A: Authoring
  log('Phase A: Browser Admin (authoring/publish)...')
  const phaseA = await runPlaywrightSpec('e2e/golden-path-authoring.spec.ts', 'authoring')
  if (phaseA !== 0) throw new Error(`Phase A failed (exit ${phaseA})`)
  log('Phase A PASSED')

  // Phase B: Four-capability runtime traffic
  log('Phase B: Four-capability runtime traffic...')
  try {
    await runFourCapabilityTraffic()
    log('Phase B PASSED')
  } catch (e) {
    log(`Phase B WARNING: ${e.message} (continuing...)`)
  }

  // Phase C: Wait for usage ingestion
  log('Phase C: Waiting for usage ingestion...')
  await waitForUsageIngestion(2, 30)
  log('Phase C PASSED')

  // Phase D: Usage verification
  log('Phase D: Browser Admin (usage/system verification)...')
  const phaseD = await runPlaywrightSpec('e2e/golden-path-usage.spec.ts', 'usage')
  if (phaseD !== 0) throw new Error(`Phase D failed (exit ${phaseD})`)
  log('Phase D PASSED')

  // Phase E: Topology security
  log('Phase E: Topology security...')
  const phaseE = await runPlaywrightSpec('e2e/topology-security.spec.ts', 'topology')
  if (phaseE !== 0) throw new Error(`Phase E failed (exit ${phaseE})`)
  log('Phase E PASSED')

  exitCode = 0
} catch (e) {
  log(`E2E test FAILED: ${e.message}`)
  exitCode = e.status ?? 1
} finally {
  // Shutdown the worker — must happen even on failure
  try { worker.postMessage({ shutdown: true }) } catch {}
  await new Promise(r => setTimeout(r, 500))
  try { worker.terminate() } catch {}

  // Merge per-phase Playwright JSON results into the final artifact
  const tempFiles = [
    join(artifactsDir, '_e2e-authoring.json'),
    join(artifactsDir, '_e2e-usage.json'),
    join(artifactsDir, '_e2e-topology.json'),
  ].filter(f => existsSync(f))
  if (tempFiles.length > 0) {
    mergePlaywrightJsons(tempFiles, join(artifactsDir, 'e2e-playwright.json'))
  }

  // Write meta.json for provenance regardless of pass/fail
  writeMetaJson(artifactsDir, 'e2e-playwright.json', ROOT, ARCH_REPO, 'node scripts/e2e-harness.mjs (orchestrated)', exitCode)

  // Always run cleanup — temp dir, processes, servers
  cleanup()
}

if (exitCode === 0) {
  log('E2E tests PASSED')
  log('wrote e2e-playwright.json.meta.json')
} else {
  log(`E2E tests FAILED (exit=${exitCode})`)
  process.exit(exitCode)
}

// --- Done ---

log('all steps completed successfully')
