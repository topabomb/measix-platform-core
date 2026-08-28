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
import { createHash, randomUUID } from 'node:crypto'
import { existsSync, readFileSync, writeFileSync, mkdirSync, rmSync } from 'node:fs'
import { join, relative } from 'node:path'
import { execSync } from 'node:child_process'
import { Worker } from 'node:worker_threads'

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

// Per audit P0-3: Adapter lifecycle covers Browser config/Publish, Test Client
// traffic, AND Usage wait. All phases go inside try/finally; only close the
// Adapter after all phases complete.

// Start a deterministic adapter on a free port (covers entire pipeline)
const adapterPort = await freePort()
const spaPort = await freePort()
const adapterBaseURL = `http://127.0.0.1:${adapterPort}`
const spaBaseURL = `http://127.0.0.1:${spaPort}`

// Per e2e-harness.mjs pattern: SPA proxy + Adapter must run in a Worker thread
// so the main thread can use execSync (for Playwright) without blocking the
// HTTP event loop. Without a Worker thread, the SPA proxy cannot serve
// pages while execSync is running, causing Playwright navigation timeouts.
const spaDir = join(ROOT, 'console', 'dist', 'spa')
const worker = new Worker(join(ROOT, 'scripts', '_server-worker.mjs'), {
  workerData: { spaPort, spaDir, adapterPort, hubPort: env.hubPort },
})
await new Promise((resolve, reject) => {
  worker.on('message', (msg) => { if (msg.ready) resolve() })
  worker.on('error', reject)
})
const servers = [] // Worker-managed servers are closed via worker.terminate()

// --- 9a. Run Playwright Browser Golden Path against this fresh environment ---
// SPA proxy is already running in the Worker thread (started above)
if (existsSync(spaDir)) {
  const spaReady = await waitFor(spaBaseURL, 'SPA', 30000, log)

  // Per e2e-harness.mjs: verify /admin/ actually serves content
  try {
    const resp = await fetch(`${spaBaseURL}/admin/`)
    const text = await resp.text()
    log(`SPA proxy check: ${resp.status} (${text.length} bytes)`)
    if (!resp.ok) {
      log('ERROR: SPA proxy not serving /admin/ correctly')
    }
  } catch (e) {
    log(`ERROR: SPA proxy unreachable: ${e.message}`)
  }

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
      // Per audit P0-2: use orchestrator for split authoring/usage phases
      browserGoldenPathOutput = execSync(
        'npx playwright test --reporter=line golden-path-authoring.spec.ts',
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

} else {
  log('WARNING: SPA build not found at console/dist/spa — skipping Browser Golden Path.')
  log('Run "make console-build" before clean replay to include browser tests.')
  browserGoldenPathPassed = false
}

// NOTE: adapterServer is NOT closed here — it must remain alive for the
// four-capability traffic phase (per audit P0-3). It is closed in cleanup.

// --- 9b. Run four-capability runtime traffic against SAME fresh environment ---
// Per architecture CAP-C7-002: the clean replay must prove a complete product
// loop in the SAME deployment: Browser config/Publish → Test Client four
// capabilities → Usage → Browser Usage/System. The Browser Golden Path already
// configured and published the snapshot. Now we send four runtime requests
// through the SAME Relay to generate usage data.
// Per audit P0-3: Adapter is still running — this phase would fail if Adapter
// were closed prematurely.
let fourCapabilityPassed = false
let fourCapabilityOutput = ''
if (browserGoldenPathPassed) {
  try {
    log('Running four-capability runtime traffic against same fresh environment...')

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

    // Create a managed user (userId must be usr_* format)
    const userResp = await fetch(`${env.hubBaseURL}/api/admin/v1/users`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Cookie': cookie,
        'X-CSRF-Token': csrfToken,
      },
      body: JSON.stringify({ username: 'replay-user-' + Date.now(), displayName: 'Replay User', role: 'MEMBER' }),
    })
    if (!userResp.ok) throw new Error(`create user failed: ${userResp.status}`)
    const userJson = await userResp.json()
    const managedUserId = userJson.userId

    // Create enrollment for the managed user
    const enrollResp = await fetch(`${env.hubBaseURL}/api/admin/v1/users/${managedUserId}/enrollments`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Cookie': cookie,
        'X-CSRF-Token': csrfToken,
      },
      body: JSON.stringify({ expiresInSeconds: 3600 }),
    })
    if (!enrollResp.ok) throw new Error(`create enrollment failed: ${enrollResp.status}`)
    const enrollJson = await enrollResp.json()
    const enrollmentCode = enrollJson.code

    // Exchange enrollment for access token
    const exchangeResp = await fetch(`${env.hubBaseURL}/api/client/v1/enrollments/exchange`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        platform: 'ANDROID',
        code: enrollmentCode,
        installationId: `ins_${randomUUID()}`,
        appVersion: 'replay-1.0',
      }),
    })
    if (!exchangeResp.ok) throw new Error(`exchange enrollment failed: ${exchangeResp.status}`)
    const exchangeJson = await exchangeResp.json()
    const clientToken = exchangeJson.accessToken

    // Get managed state for generation + resource IDs
    const stateResp = await fetch(`${env.hubBaseURL}/api/client/v1/managed/state`, {
      headers: { 'Authorization': `Bearer ${clientToken}` },
    })
    const stateJson = await stateResp.json()
    const generation = stateJson.activeManagedGeneration

    // Fetch the snapshot to get resource IDs
    const snapResp = await fetch(`${env.hubBaseURL}/api/client/v1/managed/snapshots/${generation}`, {
      headers: { 'Authorization': `Bearer ${clientToken}` },
    })
    const snapJson = await snapResp.json()

    // Extract resource IDs from snapshot
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

    // 2. TTS
    const ttsResp = await fetch(`${relayUrl}/runtime/v1/resources/${ttsId}/v1/audio/speech`, {
      method: 'POST',
      headers: { ...baseHeaders, 'X-Measix-Interaction-Id': `int_${randomUUID()}` },
      body: JSON.stringify({ model: 'tts-test', input: 'hello', voice: 'alloy' }),
    })
    if (!ttsResp.ok) throw new Error(`tts request failed: ${ttsResp.status}`)

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

    // 4. MCP
    const mcpResp = await fetch(`${relayUrl}/runtime/v1/resources/${mcpId}/mcp`, {
      method: 'POST',
      headers: { ...baseHeaders, 'X-Measix-Interaction-Id': `int_${randomUUID()}` },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'replay-client', version: '1.0' } } }),
    })
    if (!mcpResp.ok) throw new Error(`mcp request failed: ${mcpResp.status}`)

    log('Four-capability runtime requests sent successfully')

    // Wait for usage to be recorded (bounded polling)
    let usageRecorded = false
    for (let i = 0; i < 30; i++) {
      const usageResp = await fetch(`${env.hubBaseURL}/api/admin/v1/usage/summary`, {
        headers: { 'Cookie': cookie, 'X-CSRF-Token': csrfToken },
      })
      if (usageResp.ok) {
        const usageJson = await usageResp.json()
        if (usageJson.requestCount && usageJson.requestCount >= 4) {
          usageRecorded = true
          log(`Usage recorded: ${usageJson.requestCount} requests`)
          break
        }
      }
      await new Promise(r => setTimeout(r, 1000))
    }
    if (!usageRecorded) {
      throw new Error('usage not recorded within 30s (expected >= 4 requests)')
    }

    fourCapabilityPassed = true
    fourCapabilityOutput = 'four capabilities: PASS, usage recorded'
  } catch (e) {
    fourCapabilityOutput = `four capabilities: FAIL — ${e.message}`
    log(`Four-capability runtime traffic FAILED: ${e.message}`)
  }
} else {
  log('Skipping four-capability traffic — Browser Golden Path did not pass')
}

// --- 9c. Run Browser Usage/System verification (Phase 9-13) ---
// Per audit P0-2: usage/system verification runs AFTER four-capability traffic.
// Per audit P0-3: SPA proxy and Adapter are still alive for this phase.
let browserUsagePassed = false
let browserUsageOutput = ''
if (fourCapabilityPassed) {
  try {
    log('Starting SPA proxy for usage/system verification phase...')
    const usageSpaPort = await freePort()
    // Start a second Worker for the usage phase SPA proxy
    const usageWorker = new Worker(join(ROOT, 'scripts', '_server-worker.mjs'), {
      workerData: { spaPort: usageSpaPort, spaDir, adapterPort, hubPort: env.hubPort },
    })
    await new Promise((resolve, reject) => {
      usageWorker.on('message', (msg) => { if (msg.ready) resolve() })
      usageWorker.on('error', reject)
    })
    const usageSpaBaseURL = `http://127.0.0.1:${usageSpaPort}`
    const usageSpaReady = await waitFor(usageSpaBaseURL, 'usage SPA', 30000, log)

    if (usageSpaReady) {
      const usageE2eEnv = {
        ...process.env,
        MEASIX_E2E_BASE_URL: usageSpaBaseURL,
        MEASIX_E2E_HUB_BASE_URL: env.hubBaseURL,
        MEASIX_E2E_ADAPTER_URL: adapterBaseURL,
        MEASIX_E2E_ADMIN_PASSWORD: adminPassword,
        PLAYWRIGHT_BASE_URL: usageSpaBaseURL,
      }
      browserUsageOutput = execSync(
        'npx playwright test --reporter=line golden-path-usage.spec.ts',
        { cwd: join(ROOT, 'console'), encoding: 'utf-8', stdio: ['pipe', 'pipe', 'pipe'], env: usageE2eEnv, timeout: 300000 },
      )
      browserUsagePassed = true
      log('Browser Usage/System verification PASSED')
    } else {
      browserUsageOutput = 'SPA proxy not ready for usage phase'
      log('WARNING: SPA proxy not ready — skipping Browser Usage/System verification.')
    }
    try { await usageWorker.terminate() } catch {}
  } catch (e) {
    browserUsageOutput = (e.stdout || '') + (e.stderr || '')
    log('Browser Usage/System verification FAILED')
    console.error('Browser usage failure:', browserUsageOutput.slice(0, 2000))
  }
} else {
  log('Skipping Browser Usage/System verification — four-capability traffic did not pass')
}

// Per audit P0-3: adapterServer is closed AFTER all phases complete (in cleanup).

// --- 9d. Run deterministic Go candidate tests ---
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
  // Login first to get session cookie + CSRF token
  const loginResp = await fetch(`${env.hubBaseURL}/api/admin/v1/session/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: adminPassword }),
  })
  const loginJson = await loginResp.json()
  const csrfToken = loginJson.csrfToken || ''
  // Extract session cookie from set-cookie header
  // Hub uses underscore in cookie name: measix_admin_session
  const setCookie = loginResp.headers.get('set-cookie') || ''
  const cookieMatch = setCookie.match(/measix_admin_session=([^;]+)/)
  const cookie = cookieMatch ? `measix_admin_session=${cookieMatch[1]}` : ''

  const statusResp = await fetch(`${env.hubBaseURL}/api/admin/v1/system/status`, {
    headers: { 'Cookie': cookie, 'X-CSRF-Token': csrfToken },
  })
  if (statusResp.ok) {
    const status = await statusResp.json()
    log(`System status: runtime=${status.runtimeStatus} relay=${status.relayStatus}`)
    statusOk = true
  } else {
    log(`System status: HTTP ${statusResp.status}`)
  }
} catch (e) {
  log(`System status error: ${e.message}`)
}

// Cleanup
log('Cleaning up fresh environment...')
// Terminate the Worker that runs SPA proxy + Adapter
try { await worker.terminate() } catch {}
cleanupEnvironment(processes, servers, env.envRoot, false, log)

if (!browserGoldenPathPassed || !fourCapabilityPassed || !browserUsagePassed || !replayTestsPassed || !loginOk || !statusOk || !topologySecurityPassed) {
  console.error('ERROR: Clean replay failed:')
  if (!browserGoldenPathPassed) console.error('  - Browser Golden Path (authoring/publish) did not pass')
  if (!fourCapabilityPassed) console.error('  - Four-capability runtime traffic did not pass')
  if (!browserUsagePassed) console.error('  - Browser Usage/System verification did not pass')
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
    phase: 'authoring/publish (golden-path-authoring.spec.ts)',
    passed: browserGoldenPathPassed,
    outputHash: 'sha256:' + createHash('sha256').update(browserGoldenPathOutput).digest('hex'),
  },
  fourCapabilityTraffic: {
    passed: fourCapabilityPassed,
    description: 'Model/TTS/ASR/MCP runtime traffic against same fresh environment',
    output: fourCapabilityOutput,
  },
  browserUsageSystem: {
    phase: 'usage/system verification (golden-path-usage.spec.ts)',
    passed: browserUsagePassed,
    outputHash: 'sha256:' + createHash('sha256').update(browserUsageOutput).digest('hex'),
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
  // Per audit P0-2: execution order is now documented in the artifact
  executionOrder: [
    '1. Browser Admin (authoring/publish): golden-path-authoring.spec.ts',
    '2. Four-capability runtime traffic: Model/TTS/ASR/MCP',
    '3. Wait for Usage ingestion (>= 4 requests)',
    '4. Browser Admin (usage/system): golden-path-usage.spec.ts',
    '5. Deterministic Go candidate tests',
    '6. Topology security + smoke tests',
  ],
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
