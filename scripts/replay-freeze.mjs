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
import { existsSync, readFileSync, writeFileSync, mkdirSync, rmSync } from 'node:fs'
import { join, relative } from 'node:path'
import { execSync } from 'node:child_process'

import {
  resolveRoot,
  freePort,
  startDeterministicAdapter,
  startSpaProxy,
  waitFor,
  createFreshEnvironment,
  startHubAndRelay,
  cleanupEnvironment,
  gitCommit,
  adminBuildHash,
  deterministicAdapterVersion,
} from './lib/harness.mjs'

const ROOT = resolveRoot(import.meta.dirname)
const ARTIFACTS_DIR = join(ROOT, '.artifacts')
const ARCH_REPO = join(ROOT, '..', 'measix-architecture')

function log(msg) {
  console.log(`[replay] ${msg}`)
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
const buildHash = adminBuildHash(ROOT)
if (buildHash === 'not-built') {
  errors.push('Admin production build not found. Run "make console-build" first.')
} else if (manifest.adminBuildHash !== buildHash) {
  errors.push(`adminBuildHash mismatch: manifest=${manifest.adminBuildHash} current=${buildHash}`)
}

// --- 4. Verify deterministic adapter version ---
const adapterVersion = deterministicAdapterVersion(ROOT)
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

const env = await createFreshEnvironment(ROOT, {
  prefix: 'measix-replay',
  deploymentName: 'REPLAY-TEST',
  displayName: 'Replay Admin',
  adminPasswordPrefix: 'replay',
  buildStdio: 'pipe',
  migrateStdio: 'pipe',
  bootstrapStdio: 'pipe',
})

const { adminPassword } = env
const backendDir = env.backendDir

// Start Hub and Relay (pipe stdio for quieter operation)
const { hubProc, relayProc } = startHubAndRelay(env, { stdio: 'pipe', log })
const processes = [hubProc, relayProc]

const hubReady = await waitFor(`${env.hubBaseURL}/live`, 'Hub', 30000, log)
const relayReady = await waitFor(`${env.relayIntBaseURL}/live`, 'Relay', 30000, log)

if (!hubReady || !relayReady) {
  console.error('ERROR: Hub or Relay not ready in fresh environment')
  try { hubProc.kill('SIGTERM') } catch {}
  try { relayProc.kill('SIGTERM') } catch {}
  rmSync(env.envRoot, { recursive: true, force: true })
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
const adapterServer = startDeterministicAdapter(adapterPort)
const servers = [adapterServer]

// Start SPA proxy (same-origin as browser expects)
const spaDir = join(ROOT, 'console', 'dist', 'spa')
if (existsSync(spaDir)) {
  const spaServer = startSpaProxy(spaPort, spaDir, env.hubPort)
  servers.push(spaServer)
  const spaReady = await waitFor(spaBaseURL, 'SPA', 30000, log)

  if (spaReady) {
    try {
      log('Running Playwright Browser Golden Path against fresh environment...')
      const e2eEnv = {
        ...process.env,
        MEASIX_E2E_BASE_URL: spaBaseURL,
        MEASIX_E2E_HUB_BASE_URL: env.hubBaseURL,
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
  } else {
    log('WARNING: SPA proxy not ready — skipping Browser Golden Path.')
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
  const internalResp = await fetch(`${env.hubBaseURL}/internal/v1/usage/request-events:batch`)
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
  const loginResp = await fetch(`${env.hubBaseURL}/api/admin/v1/session/login`, {
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
  const setCookie = (await fetch(`${env.hubBaseURL}/api/admin/v1/session/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: adminPassword }),
  })).headers.get('set-cookie') || ''
  const cookieMatch = setCookie.match(/measix-admin-session=([^;]+)/)
  const cookie = cookieMatch ? `measix-admin-session=${cookieMatch[1]}` : ''
  const csrfMatch = setCookie.match(/measix-csrf=([^;]+)/)
  const csrfToken = csrfMatch ? csrfMatch[1] : ''

  const statusResp = await fetch(`${env.hubBaseURL}/api/admin/v1/system/status`, {
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
cleanupEnvironment(processes, servers, env.envRoot, false, log)

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
    hubBaseURL: env.hubBaseURL,
    hubInternalBaseURL: env.hubInternalBaseURL,
    relayPubBaseURL: env.relayPubBaseURL,
    dbPath: env.hubDB,
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
