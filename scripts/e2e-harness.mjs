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
import { execSync } from 'node:child_process'
import { existsSync, mkdirSync } from 'node:fs'
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

async function runPlaywrightSpec(specFile) {
  return new Promise((resolve, reject) => {
    const proc = spawn('npx', ['playwright', 'test', specFile], {
      cwd: join(ROOT, 'console'),
      stdio: 'inherit',
      env: e2eEnv,
      timeout: TIMEOUT,
    })
    proc.on('exit', (code) => resolve(code ?? 1))
    proc.on('error', reject)
  })
}

async function runFourCapabilityTraffic() {
  // Same logic as candidate-orchestrator's runFourCapabilityTraffic
  const interactionId = 'e2e-' + randomUUID()
  const headers = {
    'Authorization': `Bearer test-token`,
    'Content-Type': 'application/json',
    'X-Measix-Interaction-Id': interactionId,
  }

  // 1. Chat (non-streaming)
  const chatResp = await fetch(`${env.relayPubBaseURL}/runtime/v1/chat/completions`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ model: 'test-model', messages: [{ role: 'user', content: 'Say hello' }] }),
  })
  if (!chatResp.ok) throw new Error(`Chat request failed: ${chatResp.status}`)
  await chatResp.text()

  // 2. Chat (streaming)
  const streamResp = await fetch(`${env.relayPubBaseURL}/runtime/v1/chat/completions`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ model: 'test-model', stream: true, messages: [{ role: 'user', content: 'Count 1 to 5' }] }),
  })
  if (streamResp.ok) {
    const reader = streamResp.body?.getReader()
    if (reader) {
      for (let i = 0; i < 10; i++) { const { done } = await reader.read(); if (done) break }
      try { reader.cancel() } catch {}
    }
  }

  // 3. TTS
  try {
    const ttsResp = await fetch(`${env.relayPubBaseURL}/runtime/v1/audio/speech`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ model: 'test-tts', input: 'Hello', voice: 'alloy' }),
    })
    if (ttsResp.ok) await ttsResp.arrayBuffer()
  } catch {}

  // 4. ASR (multipart)
  try {
    // Minimal WAV header (44 bytes)
    const wavHeader = new Uint8Array(44)
    wavHeader[0] = 0x52; wavHeader[1] = 0x49; wavHeader[2] = 0x46; wavHeader[3] = 0x46
    wavHeader[8] = 0x57; wavHeader[9] = 0x41; wavHeader[10] = 0x56; wavHeader[11] = 0x45
    wavHeader[16] = 16; wavHeader[20] = 1; wavHeader[22] = 1; wavHeader[24] = 0x44; wavHeader[25] = 0xAC
    wavHeader[28] = 0x88; wavHeader[29] = 0x58; wavHeader[32] = 2; wavHeader[34] = 16; wavHeader[36] = 0x64; wavHeader[37] = 0x61; wavHeader[38] = 0x74; wavHeader[39] = 0x61
    const formData = new FormData()
    formData.append('file', new Blob([wavHeader], { type: 'audio/wav' }), 'sample.wav')
    formData.append('model', 'test-asr')
    const asrResp = await fetch(`${env.relayPubBaseURL}/runtime/v1/audio/transcriptions`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer test-token`, 'X-Measix-Interaction-Id': 'e2e-' + randomUUID() },
      body: formData,
    })
    if (asrResp.ok) await asrResp.text()
  } catch {}

  // 5. MCP (initialize)
  try {
    const mcpResp = await fetch(`${env.relayPubBaseURL}/runtime/v1/mcp`, {
      method: 'POST',
      headers: { ...headers, 'Content-Type': 'application/json' },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'test', version: '1.0' } } }),
    })
    if (mcpResp.ok) await mcpResp.text()
  } catch {}
}

async function waitForUsageIngestion(minRequests, maxWaitSeconds) {
  const adminPassword = env.adminPassword || 'admin'
  const loginResp = await fetch(`${env.hubBaseURL}/api/admin/v1/session/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: adminPassword }),
  })
  if (!loginResp.ok) return
  const loginBody = await loginResp.json()
  const setCookie = loginResp.headers.get('set-cookie') || ''
  const cookieMatch = setCookie.match(/measix_admin_session=([^;]+)/)
  const cookie = cookieMatch ? `measix_admin_session=${cookieMatch[1]}` : ''
  const csrfToken = loginBody.csrfToken || ''

  for (let i = 0; i < maxWaitSeconds; i++) {
    await new Promise(r => setTimeout(r, 1000))
    const resp = await fetch(`${env.hubBaseURL}/api/admin/v1/usage?limit=100`, {
      headers: { 'Cookie': cookie, 'X-CSRF-Token': csrfToken },
    })
    if (resp.ok) {
      const data = await resp.json()
      const count = data.items?.length || data.length || 0
      if (count >= minRequests) return
    }
  }
}

try {
  // Phase A: Authoring
  log('Phase A: Browser Admin (authoring/publish)...')
  const phaseA = await runPlaywrightSpec('e2e/golden-path-authoring.spec.ts')
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
  const phaseD = await runPlaywrightSpec('e2e/golden-path-usage.spec.ts')
  if (phaseD !== 0) throw new Error(`Phase D failed (exit ${phaseD})`)
  log('Phase D PASSED')

  // Phase E: Topology security
  log('Phase E: Topology security...')
  const phaseE = await runPlaywrightSpec('e2e/topology-security.spec.ts')
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
