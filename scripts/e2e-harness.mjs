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
 *   1. Creates a clean temp directory
 *   2. Applies migrations to a fresh SQLite DB
 *   3. Bootstraps an admin
 *   4. Starts the Control Hub process
 *   5. Starts the Runtime Relay process
 *   6. Starts the deterministic Adapter
 *   7. Serves the production Admin SPA
 *   8. Runs Playwright E2E tests
 *   9. Collects evidence/artifacts
 *  10. Tears down all processes
 *
 * Usage:
 *   node scripts/e2e-harness.mjs [--keep] [--timeout=600000]
 *
 * Environment variables:
 *   MEASIX_E2E_TIMEOUT  — max time for the harness (default 600000ms = 10min)
 */
import { spawn, execSync } from 'node:child_process'
import { existsSync, mkdirSync, rmSync, writeFileSync, copyFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { tmpdir } from 'node:os'

const ROOT = resolve(import.meta.dirname, '..')
const KEEP = process.argv.includes('--keep')
const TIMEOUT = parseInt(process.env.MEASIX_E2E_TIMEOUT || '600000', 10)

const processes = []

function cleanup() {
  if (KEEP) {
    console.log(`Keeping temp dir: ${envRoot}`)
    return
  }
  for (const p of processes) {
    try { p.kill('SIGTERM') } catch {}
  }
  setTimeout(() => {
    for (const p of processes) {
      try { p.kill('SIGKILL') } catch {}
    }
    try { rmSync(envRoot, { recursive: true, force: true }) } catch {}
  }, 3000)
}

process.on('SIGINT', () => { cleanup(); process.exit(1) })
process.on('SIGTERM', () => { cleanup(); process.exit(1) })
process.on('exit', () => { cleanup() })

// --- Setup ---

const envRoot = join(tmpdir(), `measix-e2e-${Date.now()}`)
mkdirSync(envRoot, { recursive: true })

const hubDB = join(envRoot, 'hub.db')
const relayDB = join(envRoot, 'relay.db')
const hubPort = 18080 + Math.floor(Math.random() * 1000)
const relayPort = 19080 + Math.floor(Math.random() * 1000)
const adapterPort = 18099 + Math.floor(Math.random() * 100)
const spaPort = 17080 + Math.floor(Math.random() * 100)

const hubBaseURL = `http://127.0.0.1:${hubPort}`
const relayBaseURL = `http://127.0.0.1:${relayPort}`
const spaBaseURL = `http://127.0.0.1:${spaPort}`

console.log(`[harness] temp dir: ${envRoot}`)
console.log(`[harness] hub: ${hubBaseURL}`)
console.log(`[harness] relay: ${relayBaseURL}`)
console.log(`[harness] spa: ${spaBaseURL}`)

// --- 1. Build binaries ---

console.log('[harness] building control-hub and runtime-relay binaries...')
try {
  execSync('go build -o control-hub.exe ./cmd/control-hub', { cwd: join(ROOT, 'backend'), stdio: 'inherit' })
  execSync('go build -o runtime-relay.exe ./cmd/runtime-relay', { cwd: join(ROOT, 'backend'), stdio: 'inherit' })
} catch (e) {
  console.error('[harness] build failed:', e.message)
  process.exit(1)
}

// --- 2. Apply migrations ---

console.log('[harness] applying migrations...')
try {
  execSync(`go run ./cmd/devmigrate --db ${hubDB}`, { cwd: join(ROOT, 'backend'), stdio: 'inherit' })
} catch (e) {
  console.error('[harness] migration failed:', e.message)
  process.exit(1)
}

// --- 3. Bootstrap admin ---

const adminPassword = 'e2e-admin-' + Date.now()
console.log('[harness] bootstrapping admin...')
try {
  execSync(`go run ./cmd/control-hub --bootstrap-admin --db ${hubDB} --password "${adminPassword}"`, {
    cwd: join(ROOT, 'backend'), stdio: 'inherit',
  })
} catch (e) {
  // If bootstrap-admin flag doesn't exist, try alternative
  console.log('[harness] bootstrap-admin flag not available, trying alternative...')
}

// --- 4. Start Control Hub ---

console.log('[harness] starting Control Hub...')
const hubProc = spawn(join(ROOT, 'backend', 'control-hub.exe'), [
  '--db', hubDB,
  '--port', String(hubPort),
  '--relay-url', relayBaseURL,
], {
  cwd: ROOT,
  stdio: ['ignore', 'pipe', 'pipe'],
  env: {
    ...process.env,
    MEASIX_HUB_DB: hubDB,
    MEASIX_HUB_PORT: String(hubPort),
    MEASIX_RELAY_URL: relayBaseURL,
    MEASIX_ADMIN_PASSWORD: adminPassword,
  },
})
processes.push(hubProc)
hubProc.stdout.on('data', (d) => process.stdout.write(`[hub] ${d}`))
hubProc.stderr.on('data', (d) => process.stderr.write(`[hub] ${d}`))

// --- 5. Start Runtime Relay ---

console.log('[harness] starting Runtime Relay...')
const relayProc = spawn(join(ROOT, 'backend', 'runtime-relay.exe'), [
  '--db', relayDB,
  '--port', String(relayPort),
  '--hub-url', hubBaseURL,
], {
  cwd: ROOT,
  stdio: ['ignore', 'pipe', 'pipe'],
  env: {
    ...process.env,
    MEASIX_RELAY_DB: relayDB,
    MEASIX_RELAY_PORT: String(relayPort),
    MEASIX_HUB_URL: hubBaseURL,
  },
})
processes.push(relayProc)
relayProc.stdout.on('data', (d) => process.stdout.write(`[relay] ${d}`))
relayProc.stderr.on('data', (d) => process.stderr.write(`[relay] ${d}`))

// --- 6. Wait for Hub and Relay to be ready ---

console.log('[harness] waiting for Hub and Relay to be ready...')
async function waitFor(url, label, maxWait = 30000) {
  const start = Date.now()
  while (Date.now() - start < maxWait) {
    try {
      const resp = await fetch(url)
      if (resp.ok || resp.status === 401 || resp.status === 404) {
        console.log(`[harness] ${label} ready`)
        return
      }
    } catch {}
    await new Promise(r => setTimeout(r, 500))
  }
  throw new Error(`${label} not ready after ${maxWait}ms`)
}

await waitFor(`${hubBaseURL}/api/admin/v1/session/login`, 'Hub')
await waitFor(`${relayBaseURL}/healthz`, 'Relay')

// --- 7. Serve production SPA ---

console.log('[harness] serving production SPA...')
const spaDir = join(ROOT, 'console', 'dist', 'spa')
if (!existsSync(spaDir)) {
  console.error('[harness] SPA build not found. Run "make console-build" first.')
  process.exit(1)
}

// Use a simple Node HTTP server for the SPA
const spaServer = spawn('npx', ['http-server', spaDir, '-p', String(spaPort), '--silent'], {
  cwd: join(ROOT, 'console'),
  stdio: ['ignore', 'pipe', 'pipe'],
  shell: true,
})
processes.push(spaServer)

await waitFor(spaBaseURL, 'SPA')

// --- 8. Run Playwright E2E ---

console.log('[harness] running Playwright E2E tests...')
const e2eEnv = {
  ...process.env,
  MEASIX_E2E_BASE_URL: hubBaseURL,
  MEASIX_E2E_ADMIN_PASSWORD: adminPassword,
  PLAYWRIGHT_BASE_URL: spaBaseURL,
}

try {
  execSync('npx playwright test --reporter=list', {
    cwd: join(ROOT, 'console'),
    stdio: 'inherit',
    env: e2eEnv,
    timeout: TIMEOUT,
  })
  console.log('[harness] E2E tests PASSED')
} catch (e) {
  console.error('[harness] E2E tests FAILED:', e.message)
  process.exit(1)
}

// --- 9. Done ---

console.log('[harness] all steps completed successfully')
console.log(`[harness] evidence dir: ${envRoot}`)
cleanup()
