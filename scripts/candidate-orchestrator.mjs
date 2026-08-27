#!/usr/bin/env node
/**
 * S0.1 Candidate Orchestrator
 *
 * Per audit P0-2 and P0-3: establishes a single, fixed execution order for
 * the C6 Golden Path, ensuring the four-capability runtime traffic runs
 * BETWEEN the authoring/publish and usage/system phases, and that the
 * Adapter remains alive throughout the entire pipeline.
 *
 * Fixed execution order:
 *   1. Browser Admin (authoring/publish): login → user/enrollment → upstream
 *      test/apply → resources/policy → validate/review/publish
 *   2. Test Client (same clean environment): Model/TTS/ASR/MCP four-capability
 *      runtime traffic
 *   3. Wait for Usage ingestion (>= 4 requests)
 *   4. Browser Admin (usage/system): Usage/System/refresh/session/
 *      candidate-active review
 *   5. Cleanup
 *
 * Usage:
 *   node scripts/candidate-orchestrator.mjs
 *
 * Environment variables:
 *   MEASIX_E2E_BASE_URL      — SPA proxy base URL
 *   MEASIX_E2E_HUB_BASE_URL  — Hub public base URL
 *   MEASIX_E2E_ADAPTER_URL   — deterministic adapter base URL
 *   MEASIX_E2E_ADMIN_PASSWORD — admin password
 */
import { existsSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { execSync } from 'node:child_process'
import { randomUUID } from 'node:crypto'

import {
  resolveRoot,
  startDeterministicAdapter,
  startSpaProxy,
  waitFor,
  createFreshEnvironment,
  startHubAndRelay,
  cleanupEnvironment,
  writeMetaJson,
} from './lib/harness.mjs'

const ROOT = resolveRoot(import.meta.dirname)
const ARTIFACTS_DIR = join(ROOT, '.artifacts')
const ARCH_REPO = join(ROOT, '..', 'measix-architecture')

function log(msg) {
  console.log(`[orchestrator] ${msg}`)
}

async function main() {
  // --- 1. Create fresh environment ---
  log('Creating fresh environment...')
  const env = await createFreshEnvironment(ROOT, {
    prefix: 'measix-candidate',
    deploymentName: 'CANDIDATE-TEST',
    displayName: 'Candidate Admin',
    adminPasswordPrefix: 'candidate',
  })

  const { adminPassword } = env

  // --- 2. Start Hub and Relay ---
  log('Starting Control Hub and Runtime Relay...')
  const { hubProc, relayProc } = startHubAndRelay(env, { stdio: 'pipe', log })
  const processes = [hubProc, relayProc]

  const hubReady = await waitFor(`${env.hubBaseURL}/live`, 'Hub', 30000, log)
  const relayReady = await waitFor(`${env.relayIntBaseURL}/live`, 'Relay', 30000, log)
  if (!hubReady || !relayReady) {
    log('ERROR: Hub or Relay not ready')
    cleanupEnvironment(processes, [], env.envRoot, false, log)
    process.exit(1)
  }

  // --- 3. Start Adapter (covers entire pipeline per P0-3) ---
  log('Starting deterministic Adapter (covers entire pipeline)...')
  const adapterPort = await (await import('./lib/harness.mjs')).freePort()
  const adapterServer = startDeterministicAdapter(adapterPort)
  const adapterBaseURL = `http://127.0.0.1:${adapterPort}`
  const servers = [adapterServer]

  // --- 4. Start SPA proxy ---
  log('Starting SPA proxy...')
  const spaPort = await (await import('./lib/harness.mjs')).freePort()
  const spaDir = join(ROOT, 'console', 'dist', 'spa')
  if (!existsSync(spaDir)) {
    log('ERROR: SPA build not found. Run "make console-build" first.')
    cleanupEnvironment(processes, servers, env.envRoot, false, log)
    process.exit(1)
  }

  const spaServer = startSpaProxy(spaPort, spaDir, env.hubPort)
  servers.push(spaServer)
  const spaBaseURL = `http://127.0.0.1:${spaPort}`

  const spaReady = await waitFor(spaBaseURL, 'SPA', 30000, log)
  if (!spaReady) {
    log('ERROR: SPA proxy not ready')
    cleanupEnvironment(processes, servers, env.envRoot, false, log)
    process.exit(1)
  }

  // --- 5. Common environment for Playwright ---
  const e2eEnv = {
    ...process.env,
    MEASIX_E2E_BASE_URL: spaBaseURL,
    MEASIX_E2E_HUB_BASE_URL: env.hubBaseURL,
    MEASIX_E2E_ADAPTER_URL: adapterBaseURL,
    MEASIX_E2E_ADMIN_PASSWORD: adminPassword,
    PLAYWRIGHT_BASE_URL: spaBaseURL,
  }

  if (!existsSync(ARTIFACTS_DIR)) {
    mkdirSync(ARTIFACTS_DIR, { recursive: true })
  }

  let exitCode = 1
  try {
    // ================================================================
    // Phase A: Browser Admin (authoring/publish)
    // Per P0-2: First phase — configure and publish the snapshot
    // ================================================================
    log('--- Phase A: Browser Admin (authoring/publish) ---')
    execSync(
      'npx playwright test --reporter=line golden-path-authoring.spec.ts',
      { cwd: join(ROOT, 'console'), stdio: 'inherit', env: e2eEnv, timeout: 600000 },
    )
    log('Phase A PASSED')

    // ================================================================
    // Phase B: Four-capability runtime traffic (same clean environment)
    // Per P0-2: Second phase — generate usage data via Test Client
    // The Adapter MUST still be running (P0-3 fix)
    // ================================================================
    log('--- Phase B: Four-capability runtime traffic ---')
    const fourCapResult = await runFourCapabilityTraffic(env, adminPassword)
    if (!fourCapResult.success) {
      log(`Phase B FAILED: ${fourCapResult.error}`)
      throw new Error(`Four-capability traffic failed: ${fourCapResult.error}`)
    }
    log('Phase B PASSED')

    // ================================================================
    // Phase C: Wait for Usage ingestion
    // ================================================================
    log('--- Phase C: Waiting for Usage ingestion ---')
    await waitForUsageIngestion(env, adminPassword, 4, 30)
    log('Phase C PASSED')

    // ================================================================
    // Phase D: Browser Admin (usage/system verification)
    // Per P0-2: Third phase — verify usage data and system health
    // ================================================================
    log('--- Phase D: Browser Admin (usage/system verification) ---')
    execSync(
      'npx playwright test --reporter=line golden-path-usage.spec.ts',
      { cwd: join(ROOT, 'console'), stdio: 'inherit', env: e2eEnv, timeout: 300000 },
    )
    log('Phase D PASSED')

    exitCode = 0
    log('All phases PASSED')
    writeMetaJson(ARTIFACTS_DIR, 'candidate-orchestrator.json', ROOT, ARCH_REPO, 'node scripts/candidate-orchestrator.mjs', 0)
  } catch (e) {
    exitCode = e.status ?? 1
    log(`Pipeline FAILED (exit=${exitCode}): ${e.message}`)
    writeMetaJson(ARTIFACTS_DIR, 'candidate-orchestrator.json', ROOT, ARCH_REPO, 'node scripts/candidate-orchestrator.mjs', exitCode)
  } finally {
    // Per P0-3: Adapter closed here — AFTER all phases complete
    log('Cleaning up (Adapter remains alive until now per P0-3)...')
    cleanupEnvironment(processes, servers, env.envRoot, false, log)
  }

  process.exit(exitCode)
}

/**
 * Run four-capability runtime traffic against the SAME clean environment.
 * Per P0-2: This uses the public topology via Relay, not a separate client.
 */
async function runFourCapabilityTraffic(env, adminPassword) {
  try {
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
      headers: {
        'Content-Type': 'application/json',
        'Cookie': cookie,
        'X-CSRF-Token': csrfToken,
      },
      body: JSON.stringify({ username: 'orchestrator-user-' + Date.now(), displayName: 'Orchestrator User', role: 'MEMBER' }),
    })
    if (!userResp.ok) throw new Error(`create user failed: ${userResp.status}`)
    const userJson = await userResp.json()
    const managedUserId = userJson.userId

    // Create enrollment
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
        appVersion: 'orchestrator-1.0',
      }),
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
    const headers = {
      'Authorization': `Bearer ${clientToken}`,
      'X-Measix-Managed-Generation': String(generation),
      'Content-Type': 'application/json',
    }

    // 1. Model streaming
    const modelResp = await fetch(`${relayUrl}/runtime/v1/resources/${modelId}/v1/chat/completions`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ model: 'gpt-test', stream: true, messages: [{ role: 'user', content: 'Say hello' }] }),
    })
    if (!modelResp.ok) throw new Error(`model request failed: ${modelResp.status}`)
    // Consume the stream to ensure complete
    await modelResp.text()

    // 2. TTS
    const ttsResp = await fetch(`${relayUrl}/runtime/v1/resources/${ttsId}/v1/audio/speech`, {
      method: 'POST',
      headers,
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
      headers: { 'Authorization': `Bearer ${clientToken}`, 'X-Measix-Managed-Generation': String(generation) },
      body: asrFormData,
    })
    if (!asrResp.ok) throw new Error(`asr request failed: ${asrResp.status}`)
    await asrResp.text()

    // 4. MCP
    const mcpResp = await fetch(`${relayUrl}/runtime/v1/resources/${mcpId}/mcp`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'orchestrator-client', version: '1.0' } } }),
    })
    if (!mcpResp.ok) throw new Error(`mcp request failed: ${mcpResp.status}`)
    await mcpResp.text()

    log('Four-capability runtime requests sent successfully')
    return { success: true }
  } catch (e) {
    return { success: false, error: e.message }
  }
}

/**
 * Wait for usage ingestion to record at least `minRequests` requests.
 */
async function waitForUsageIngestion(env, adminPassword, minRequests, maxWaitSeconds) {
  // Login to get credentials
  const loginResp = await fetch(`${env.hubBaseURL}/api/admin/v1/session/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: adminPassword }),
  })
  if (!loginResp.ok) throw new Error(`login failed for usage check: ${loginResp.status}`)
  const cookie = loginResp.headers.get('set-cookie')?.split(';')[0] || ''
  const loginJson = await loginResp.json()
  const csrfToken = loginJson.csrfToken

  for (let i = 0; i < maxWaitSeconds; i++) {
    const usageResp = await fetch(`${env.hubBaseURL}/api/admin/v1/usage/summary`, {
      headers: { 'Cookie': cookie, 'X-CSRF-Token': csrfToken },
    })
    if (usageResp.ok) {
      const usageJson = await usageResp.json()
      if (usageJson.requestCount && usageJson.requestCount >= minRequests) {
        log(`Usage recorded: ${usageJson.requestCount} requests`)
        return
      }
    }
    await new Promise(r => setTimeout(r, 1000))
  }
  throw new Error(`usage not recorded within ${maxWaitSeconds}s (expected >= ${minRequests} requests)`)
}

main().catch(e => {
  console.error(`[orchestrator] FATAL: ${e.message}`)
  process.exit(1)
})
