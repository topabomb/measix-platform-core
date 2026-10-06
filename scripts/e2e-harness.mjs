#!/usr/bin/env node
/**
 * S0.1 T4.1 Clean-Environment Harness
 *
 * Per measix-s0-capability-delivery-system-testing-spec.md §3:
 *   "独立 temp directory/SQLite/ports/identity"
 *   "真实 SQLite + current schema initialization"
 *   "真实 TCP/HTTP streaming/binary/multipart"
 *   "Hub/Relay T3/T4.1 使用真实 process/binary"
 *   "Admin T4.1 使用 production dist/spa"
 *
 * This script:
 *   1. Creates a clean temp directory with unique ports and identity
 *   2. Generates crypto material (master key, Ed25519 seed, relay service token)
 *   3. Initializes a fresh SQLite DB with the current schema via devmigrate
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
 *   MEASIX_E2E_ANDROID_SERIAL — opt-in isolated emulator with debug/test APKs already installed
 *   ADB — optional adb binary path; Android lane never installs or clears app data
 */
import { existsSync, mkdirSync, renameSync, unlinkSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { Worker } from 'node:worker_threads'
import { randomUUID } from 'node:crypto'
import { spawn } from 'node:child_process'
import { assertInstrumentationPassed, validateAndroidSerial, verifyStarterRequest } from './lib/android-starter.mjs'

import {
  resolveRoot,
  freePort,
  waitFor,
  createFreshEnvironment,
  startHubAndRelay,
  cleanupEnvironment,
  writeMetaJson,
  snapshotAdminBuild,
} from './lib/harness.mjs'

const ROOT = resolveRoot(import.meta.dirname)
const ARCH_REPO = join(ROOT, '..', 'measix-architecture')
const KEEP = process.argv.includes('--keep')
const ANDROID_SERIAL = process.env.MEASIX_E2E_ANDROID_SERIAL ? validateAndroidSerial(process.env.MEASIX_E2E_ANDROID_SERIAL) : undefined
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
const { hubProc, relayProc } = startHubAndRelay(env, { stdio: 'pipe', log, publicOrigin: spaBaseURL })
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
const adminBuild = snapshotAdminBuild(ROOT, env.envRoot)
const spaDir = adminBuild.directory
log(`Pinned production Admin build: ${adminBuild.buildHash}`)

// Start HTTP servers (SPA proxy + Adapter) in a worker thread to avoid
// blocking the Node.js event loop when using execSync for Playwright.
const worker = new Worker(join(ROOT, 'scripts', '_server-worker.mjs'), {
  workerData: { spaPort, spaDir, adapterPort, hubPort: env.hubPort, relayPort: env.relayPubPort, captureStarterRequests: Boolean(ANDROID_SERIAL) },
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

// Per audit P1-1: use try/finally to ensure Worker, Hub, Relay, Adapter,
// SPA proxy and temp directory are cleaned up regardless of pass/fail.
//
// This is the single browser candidate entry:
//   Phase A: golden-path-authoring.spec.ts (setup, upstream, resources, publish)
//   Phase B: Five-capability runtime traffic (Model/Image/TTS/ASR/MCP)
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
    } catch (error) {
      // An unreadable report must not be silently dropped: the merged artifact
      // would then under-report and look complete.
      throw new Error(`Cannot merge Playwright report ${fp}: ${error.message}`)
    }
  }
  writeFileSync(outputPath, JSON.stringify(merged, null, 2))
  // Clean up temp files
  for (const fp of filePaths) { try { unlinkSync(fp) } catch {} }
}

async function runFiveCapabilityTraffic() {
  const discoveryResponse = await fetch(`${spaBaseURL}/.well-known/measix`)
  if (!discoveryResponse.ok) throw new Error(`Discovery through public origin failed: ${discoveryResponse.status}`)
  const discovery = await discoveryResponse.json()
  const runtimeBase = new URL(discovery.runtimeApiBase, spaBaseURL)
  if (runtimeBase.origin !== spaBaseURL || runtimeBase.pathname !== '/runtime/v1') throw new Error('Discovery did not provide the current same-origin Runtime base')
  const clientBase = new URL(discovery.clientApiBase, spaBaseURL)
  if (clientBase.origin !== spaBaseURL || clientBase.pathname !== '/api/client/v1') throw new Error('Discovery did not provide the current same-origin Client base')
  // Login as admin to get CSRF token + cookie
  const loginResp = await fetch(`${spaBaseURL}/api/admin/v1/session/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: adminPassword }),
  })
  if (!loginResp.ok) throw new Error(`admin login failed: ${loginResp.status}`)
  const loginJson = await loginResp.json()
  const csrfToken = loginJson.csrfToken
  const cookie = loginResp.headers.get('set-cookie')?.split(';')[0] || ''

  // Create a managed user
  const userResp = await fetch(`${spaBaseURL}/api/admin/v1/users`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Cookie': cookie, 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ username: 'e2e-user-' + Date.now(), displayName: 'E2E User', role: 'MEMBER' }),
  })
  if (!userResp.ok) throw new Error(`create user failed: ${userResp.status}`)
  const userJson = await userResp.json()
  const managedUserId = userJson.userId

  // Create enrollment
  const enrollResp = await fetch(`${spaBaseURL}/api/admin/v1/users/${managedUserId}/enrollments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Cookie': cookie, 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ expiresInSeconds: 3600 }),
  })
  if (!enrollResp.ok) throw new Error(`create enrollment failed: ${enrollResp.status}`)
  const enrollJson = await enrollResp.json()
  const enrollmentCode = enrollJson.code

  // Exchange enrollment for access token
  const exchangeResp = await fetch(`${clientBase.href}/enrollments/exchange`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ platform: 'ANDROID', deviceName: 'Test device', code: enrollmentCode, installationId: `ins_${randomUUID()}`, appVersion: 'e2e-1.0' }),
  })
  if (!exchangeResp.ok) throw new Error(`exchange enrollment failed: ${exchangeResp.status}`)
  const exchangeJson = await exchangeResp.json()
  const clientToken = exchangeJson.accessToken

  // Get managed state for generation + resource IDs
  const stateResp = await fetch(`${clientBase.href}/managed/state`, {
    headers: { 'Authorization': `Bearer ${clientToken}` },
  })
  if (!stateResp.ok) throw new Error(`get managed state failed: ${stateResp.status}`)
  const stateJson = await stateResp.json()
  const generation = stateJson.activeManagedGeneration

  // Fetch the snapshot to get resource IDs
  const snapResp = await fetch(`${clientBase.href}/managed/snapshots/${generation}`, {
    headers: { 'Authorization': `Bearer ${clientToken}` },
  })
  if (!snapResp.ok) throw new Error(`get snapshot failed: ${snapResp.status}`)
  const snapJson = await snapResp.json()

  const modelId = snapJson.models?.[0]?.modelId
  const imageId = snapJson.imageGenerators?.[0]?.imageId
  const ttsId = snapJson.tts?.[0]?.ttsId
  const asrId = snapJson.asr?.[0]?.asrId
  const mcpId = snapJson.mcp?.[0]?.mcpServerId

  if (!modelId || !imageId || !ttsId || !asrId || !mcpId) {
    throw new Error(`snapshot missing resource IDs: model=${modelId} image=${imageId} tts=${ttsId} asr=${asrId} mcp=${mcpId}`)
  }

  const runtimeURL = (id, path) => `${runtimeBase.href}/resources/${id}${path}`
  const baseHeaders = {
    'Authorization': `Bearer ${clientToken}`,
    'X-Measix-Managed-Generation': String(generation),
    'Content-Type': 'application/json',
  }

  // 1. Model streaming
  const modelResp = await fetch(runtimeURL(modelId, snapJson.models[0].runtimePath), {
    method: 'POST',
    headers: { ...baseHeaders, 'X-Measix-Interaction-Id': `int_${randomUUID()}` },
    body: JSON.stringify({ model: snapJson.models[0].upstreamModelKey, stream: true, messages: [{ role: 'user', content: 'Say hello' }] }),
  })
  if (!modelResp.ok) throw new Error(`model request failed: ${modelResp.status}`)
  await modelResp.text()

  // 2. Image Generation
  const imageDefinition = snapJson.imageGenerators[0]
  const imageRequest = imageDefinition.clientProtocol === 'DASHSCOPE_MULTIMODAL_GENERATION'
    ? {
        model: imageDefinition.upstreamModelKey,
        input: { messages: [{ role: 'user', content: [{ text: 'Draw a blue square' }] }] },
        parameters: { size: imageDefinition.allowedSizes[0].replace('x', '*'), n: 1, watermark: false },
      }
    : { model: imageDefinition.upstreamModelKey, prompt: 'Draw a blue square', n: 1, size: imageDefinition.allowedSizes[0] }
  const imageResp = await fetch(runtimeURL(imageId, imageDefinition.runtimePath), {
    method: 'POST',
    headers: { ...baseHeaders, 'X-Measix-Interaction-Id': `int_${randomUUID()}` },
    body: JSON.stringify(imageRequest),
  })
  if (!imageResp.ok) throw new Error(`image generation request failed: ${imageResp.status} ${await imageResp.text()}`)
  const imageBody = await imageResp.json()
  if (imageDefinition.clientProtocol === 'DASHSCOPE_MULTIMODAL_GENERATION') {
    if (imageBody.output?.choices?.[0]?.message?.content?.[0]?.image !== 'https://images.example/generated.png') throw new Error('DashScope image generation response was changed')
  } else if (imageBody.data?.[0]?.b64_json !== 'iVBORw0KGgo=') throw new Error('image generation response was changed')

  // 3. TTS
  const ttsResp = await fetch(runtimeURL(ttsId, snapJson.tts[0].runtimePath), {
    method: 'POST',
    headers: { ...baseHeaders, 'X-Measix-Interaction-Id': `int_${randomUUID()}` },
    body: JSON.stringify({ model: snapJson.tts[0].upstreamModelKey, input: 'hello', voice: snapJson.tts[0].voice }),
  })
  if (!ttsResp.ok) throw new Error(`tts request failed: ${ttsResp.status}`)
  await ttsResp.text()

  // 4. ASR
  const asrFormData = new FormData()
  asrFormData.append('file', new Blob([Buffer.from('RIFF')]), 'sample.wav')
  asrFormData.append('model', snapJson.asr[0].upstreamModelKey)
  const asrResp = await fetch(runtimeURL(asrId, snapJson.asr[0].runtimePath), {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${clientToken}`, 'X-Measix-Managed-Generation': String(generation), 'X-Measix-Interaction-Id': `int_${randomUUID()}` },
    body: asrFormData,
  })
  if (!asrResp.ok) throw new Error(`asr request failed: ${asrResp.status}`)
  await asrResp.text()

  // 5. MCP
  const mcpResp = await fetch(runtimeURL(mcpId, snapJson.mcp[0].runtimePath), {
    method: 'POST',
    headers: { ...baseHeaders, 'X-Measix-Interaction-Id': `int_${randomUUID()}` },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'e2e-client', version: '1.0' } } }),
  })
  if (!mcpResp.ok) throw new Error(`mcp request failed: ${mcpResp.status}`)
  await mcpResp.text()
  return snapJson
}

function starterCapture(command, prompt) {
  const id = randomUUID()
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => { worker.off('message', listener); reject(new Error('Adapter Starter capture timed out')) }, 5000)
    function listener(message) {
      if (message.starterCaptureId !== id) return
      clearTimeout(timer)
      worker.off('message', listener)
      resolve(message)
    }
    worker.on('message', listener)
    worker.postMessage({ starterCapture: command, id, prompt })
  })
}

async function runAndroidStarter(snapshot) {
  if (!ANDROID_SERIAL) return
  if (snapshot.schemaVersion !== 5) throw new Error('Android lane requires the actual published v5 Snapshot')
  const starter = snapshot.starters.find(item => item.enabled && item.openingSnapshot?.initialContexts?.length >= 2
    && snapshot.assistants.some(assistant => assistant.assistantDefinitionId === item.assistantDefinitionId && assistant.enabled))
  if (!starter) throw new Error('Published Snapshot has no enabled Starter with two backgrounds; author it through Admin')
  const outputDir = join(artifactsDir, 'starter-v5', 'core-android')
  // Keep each run's outcome independent; adb pull otherwise nests into an existing directory.
  if (existsSync(outputDir)) renameSync(outputDir, `${outputDir}-${Date.now()}`)
  mkdirSync(outputDir, { recursive: true })
  const adbPath = process.env.ADB || 'D:/Android/platform-tools/adb.exe'
  const remoteDir = '/sdcard/Android/data/net.weero.measix.pilot.debug/files'
  const appPackage = 'net.weero.measix.pilot.debug'
  const inputName = `core-starter-${randomUUID()}.json`
  let remoteInput
  let code = '', reversed = false, captureStarted = false, failure
  const redact = text => code ? text.replaceAll(code, '[REDACTED_ENROLLMENT]') : text
  function adb(args, timeoutMs = 30000, input) {
    return new Promise((resolve, reject) => {
      const proc = spawn(adbPath, ['-s', ANDROID_SERIAL, ...args], { windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] })
      let output = ''
      const timer = setTimeout(() => { proc.kill(); reject(new Error('ADB operation timed out')) }, timeoutMs)
      proc.stdout.on('data', chunk => { output += chunk.toString() })
      proc.stderr.on('data', chunk => { output += chunk.toString() })
      proc.stdin.on('error', cause => { clearTimeout(timer); reject(cause) })
      proc.stdin.end(input)
      proc.on('error', cause => { clearTimeout(timer); reject(cause) })
      proc.on('close', exit => {
        clearTimeout(timer)
        if (args[0] === 'shell' && args[1] === 'am') writeFileSync(join(outputDir, 'android-instrumentation.log'), redact(output))
        if (exit !== 0) reject(new Error(`ADB operation failed (${exit}): ${redact(output)}`))
        else resolve(output)
      })
    })
  }
  try {
    if ((await adb(['get-state'])).trim() !== 'device') throw new Error('Selected Android emulator is unavailable')
    const appRoot = (await adb(['shell', 'run-as', appPackage, 'pwd'])).trim()
    if (!/^\/data\/(?:user\/\d+|data)\/net\.weero\.measix\.pilot\.debug$/.test(appRoot)) throw new Error('Unexpected private directory for Android debug app')
    remoteInput = `${appRoot}/cache/${inputName}`
    await adb(['reverse', `tcp:${spaPort}`, `tcp:${spaPort}`])
    reversed = true
    const login = await fetch(`${spaBaseURL}/api/admin/v1/session/login`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: 'admin', password: adminPassword }),
    })
    if (!login.ok) throw new Error(`Android test Admin login failed: HTTP ${login.status}`)
    const session = await login.json()
    const headers = { 'Content-Type': 'application/json', Cookie: login.headers.get('set-cookie')?.split(';')[0] || '', 'X-CSRF-Token': session.csrfToken }
    const userResponse = await fetch(`${spaBaseURL}/api/admin/v1/users`, {
      method: 'POST', headers, body: JSON.stringify({ username: `android-starter-${Date.now()}`, displayName: 'Android Starter isolated E2E', role: 'MEMBER' }),
    })
    if (!userResponse.ok) throw new Error(`Android test user creation failed: HTTP ${userResponse.status}`)
    const user = await userResponse.json()
    const enrollmentResponse = await fetch(`${spaBaseURL}/api/admin/v1/users/${user.userId}/enrollments`, { method: 'POST', headers, body: '{}' })
    if (!enrollmentResponse.ok) throw new Error(`Android test enrollment failed: HTTP ${enrollmentResponse.status}`)
    const enrollment = await enrollmentResponse.json()
    code = enrollment.code
    if (enrollment.platformUrl !== spaBaseURL) throw new Error('Enrollment origin differs from the isolated harness public origin')
    const material = { formatVersion: 1, kind: 'PLATFORM_ENROLLMENT', platformUrl: enrollment.platformUrl, code, expiresAt: enrollment.expiresAt }
    const input = JSON.stringify({ enrollment: material, snapshot, expectedAnswer: 'hello', starterId: starter.starterId })
    // Only the app UID creates its private input. Credentials travel over stdin,
    // never through shell arguments or an externally owned application directory.
    await adb(['shell', '-T', `run-as ${appPackage} sh -c 'umask 077; mkdir -p cache; cat > cache/${inputName}'`], 30000, input)
    await starterCapture('start', starter.prompt)
    captureStarted = true
    const result = await adb(['shell', 'am', 'instrument', '-w', '-r', '-e', 'class',
      'net.weero.measix.pilot.ui.pages.chat.StarterV5ChatFlowAndroidTest#syncedCoreV5CardSurvivesFirstSendDetailsRoomReadbackAndActivityReopen',
      '-e', 'starterV5CoreLive', 'true', '-e', 'coreStarterInput', remoteInput,
      'net.weero.measix.pilot.debug.test/androidx.test.runner.AndroidJUnitRunner'], 240000)
    assertInstrumentationPassed(result)
    const captured = await starterCapture('stop')
    captureStarted = false
    if (captured.overflow) throw new Error('Adapter Starter request capture exceeded its bound')
    writeFileSync(join(outputDir, 'adapter-requests.json'), JSON.stringify(captured.requests, null, 2))
    const assistant = snapshot.assistants.find(item => item.assistantDefinitionId === starter.assistantDefinitionId)
    const verified = verifyStarterRequest(captured.requests, starter, assistant.systemPrompt)
    writeFileSync(join(outputDir, 'verification.json'), JSON.stringify({
      schemaVersion: snapshot.schemaVersion, releaseId: snapshot.releaseId, snapshotHash: snapshot.snapshotHash,
      managedGeneration: snapshot.managedGeneration, starterId: verified.starterId, contextIds: verified.contextIds,
      instrumentation: '1 passed', adapterRequestVerified: true,
    }, null, 2))
  } catch (cause) {
    failure = cause
  } finally {
    const cleanupFailures = []
    async function cleanupAction(action) { try { await action() } catch (cause) { cleanupFailures.push(cause) } }
    if (captureStarted) await cleanupAction(async () => {
      const captured = await starterCapture('stop')
      writeFileSync(join(outputDir, 'adapter-requests.json'), JSON.stringify(captured.requests, null, 2))
    })
    if (remoteInput) await cleanupAction(() => adb(['shell', 'run-as', appPackage, 'rm', '-f', remoteInput]))
    await cleanupAction(() => adb(['pull', `${remoteDir}/starter-v5-chat-evidence`, join(outputDir, 'device-evidence')]))
    if (reversed) await cleanupAction(() => adb(['reverse', '--remove', `tcp:${spaPort}`]))
    if (failure) {
      if (cleanupFailures.length) log(`Android test cleanup also failed: ${cleanupFailures.map(item => redact(item.message)).join('; ')}`)
      throw failure
    }
    if (cleanupFailures.length) throw new AggregateError(cleanupFailures, 'Android test evidence/cleanup failed')
  }
}

async function waitForUsageIngestion(minRequests, maxWaitSeconds) {
  const loginResp = await fetch(`${env.hubBaseURL}/api/admin/v1/session/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: adminPassword }),
  })
  // An unreachable or unauthenticated Admin API means usage cannot be
  // verified; returning silently would let a broken run look complete.
  if (!loginResp.ok) throw new Error(`usage ingestion check could not authenticate: HTTP ${loginResp.status}`)
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
  throw new Error(`usage ingestion did not reach ${minRequests} requests within ${maxWaitSeconds}s`)
}

try {
  // Phase A: Authoring
  log('Phase A: Browser Admin (authoring/publish)...')
  const phaseA = await runPlaywrightSpec('e2e/golden-path-authoring.spec.ts', 'authoring')
  if (phaseA !== 0) throw new Error(`Phase A failed (exit ${phaseA})`)
  log('Phase A PASSED')

  if (process.argv.includes('--manual')) {
    writeFileSync(join(env.envRoot, 'manual-browser.json'), JSON.stringify({ spaBaseURL, adapterBaseURL, adminPassword }, null, 2))
    log(`Manual browser verification is ready. Local test credentials: ${join(env.envRoot, 'manual-browser.json')}`)
    log('Press Enter in this harness to continue runtime verification and cleanup.')
    process.stdin.resume()
    await new Promise(resolve => process.stdin.once('data', resolve))
    process.stdin.pause()
  }

  log('Phase A MCP: explicit tool grants, assistant bindings and drift recovery...')
  const phaseMcp = await runPlaywrightSpec('e2e/mcp-tool-governance.spec.ts', 'mcp')
  if (phaseMcp !== 0) throw new Error(`MCP browser verification failed (exit ${phaseMcp})`)
  log('Phase A MCP PASSED')

  // Phase B: Five-capability runtime traffic. A failure here is a gate
  // failure, not a warning: the complete capability path is the point of the run.
  log('Phase B: Five-capability runtime traffic...')
  try {
    const publishedSnapshot = await runFiveCapabilityTraffic()
    log('Phase B PASSED')
    if (ANDROID_SERIAL) {
      log('Phase B Android: actual published Starter through native UI...')
      await runAndroidStarter(publishedSnapshot)
      log('Phase B Android PASSED')
    }
  } catch (e) {
    throw new Error(`Phase B failed: ${e.message}`)
  }

  // Phase C: Wait for usage ingestion
  log('Phase C: Waiting for usage ingestion...')
  await waitForUsageIngestion(5, 30)
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

  log('Phase F: all Admin routes, responsive layout and input guards...')
  const phaseF = await runPlaywrightSpec('e2e/admin-console-review.spec.ts', 'admin-review')
  if (phaseF !== 0) throw new Error(`Admin review failed (exit ${phaseF})`)
  log('Phase F PASSED')

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
    join(artifactsDir, '_e2e-mcp.json'),
    join(artifactsDir, '_e2e-usage.json'),
    join(artifactsDir, '_e2e-topology.json'),
    join(artifactsDir, '_e2e-admin-review.json'),
  ].filter(f => existsSync(f))
  if (tempFiles.length === 0) {
    // Writing meta without a refreshed artifact would let a stale
    // e2e-playwright.json be presented as evidence for this run.
    log('ERROR: no per-phase Playwright reports were produced; e2e-playwright.json was not refreshed')
    if (exitCode === 0) exitCode = 1
  } else {
    try {
      mergePlaywrightJsons(tempFiles, join(artifactsDir, 'e2e-playwright.json'))
    } catch (error) {
      // Inside finally: record the failure without masking the original error.
      log(`ERROR: ${error.message}`)
      if (exitCode === 0) exitCode = 1
    }
  }

  // Write meta.json for provenance regardless of pass/fail
  writeMetaJson(artifactsDir, 'e2e-playwright.json', ROOT, ARCH_REPO, 'node scripts/e2e-harness.mjs (orchestrated)', exitCode)
  writeFileSync(join(artifactsDir, 'e2e-admin-build.json'), JSON.stringify({ ...adminBuild, capturedAt: new Date().toISOString() }, null, 2) + '\n')
  writeMetaJson(artifactsDir, 'e2e-admin-build.json', ROOT, ARCH_REPO, 'node scripts/e2e-harness.mjs (served build snapshot)', exitCode)

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
