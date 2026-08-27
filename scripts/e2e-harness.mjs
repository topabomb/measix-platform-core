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

// Use execSync to run Playwright — this is safe because the HTTP servers
// (SPA proxy + Adapter) run in a worker thread, so execSync's event loop
// blocking does not prevent Chromium from accessing the HTTP servers.
//
// Per audit P1-1: use try/finally to ensure Worker, Hub, Relay, Adapter,
// SPA proxy and temp directory are cleaned up regardless of pass/fail.
let exitCode = 1
try {
  execSync('npx playwright test', {
    cwd: join(ROOT, 'console'),
    stdio: 'inherit',
    env: e2eEnv,
    timeout: TIMEOUT,
  })
  exitCode = 0
} catch (e) {
  exitCode = e.status ?? 1
} finally {
  // Shutdown the worker — must happen even on failure
  try { worker.postMessage({ shutdown: true }) } catch {}
  await new Promise(r => setTimeout(r, 500))
  try { worker.terminate() } catch {}

  // Write meta.json for provenance regardless of pass/fail
  writeMetaJson(artifactsDir, 'e2e-playwright.json', ROOT, ARCH_REPO, 'npx playwright test', exitCode)

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
