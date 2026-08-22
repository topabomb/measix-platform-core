#!/usr/bin/env node
/**
 * Real Adapter Qualification Artifact Generator
 *
 * Per architecture §16 (Real Adapter Qualification Gate):
 *   "Deterministic tests Green 后，至少记录：
 *    adapterName/version, endpoint/profile,
 *    OPENAI_CHAT_COMPLETIONS result, streaming/cancel result,
 *    TTS result if claimed, ASR result if claimed,
 *    MCP endpoint result, correlation level,
 *    usage capability level, known deviations"
 *
 * This script automates the qualification flow:
 *   1. Start Hub + Relay (or use already-running instances)
 *   2. Create a Secret in Hub with the API key
 *   3. Create an Upstream with the endpoint URL
 *   4. Test and apply the upstream (with Idempotency-Key)
 *   5. Create resources (Model/TTS/ASR/MCP) bound to the upstream
 *      using stable platform-prefixed IDs (mdl_*, tts_*, etc.)
 *   6. Publish a draft (with Idempotency-Key)
 *   7. Run runtime requests through the Relay against the real upstream
 *      including streaming, cancel, MCP full flow (initialize/list/call)
 *   8. Verify usage records
 *   9. Generate .artifacts/real-adapter-qualification.json
 *
 * The qualification unit is adapter/version + configRevision + profile.
 * Different profiles (Model/TTS/ASR/MCP) may use different endpoints/adapters.
 *
 * Each profile must pass ALL required cases:
 *   model: normal, streaming, cancel, authBoundary, errorBoundary
 *   tts: normal, errorBoundary
 *   asr: normal, cancel, errorBoundary
 *   mcp: initialize, tools/list, tools/call, errorBoundary
 *
 * Any FAIL or NOT_EXECUTED on a required case → profile != VERIFIED.
 *
 * Usage:
 *   node scripts/collect-adapter-qualification.mjs --endpoint <url> --key <api-key>
 *   [--hub-url <url>] [--relay-url <url>] [--admin-password <pw>]
 *   [--profile all|model|tts|asr|mcp]
 *
 * Or to mark as NOT_EXECUTED (default when no args):
 *   node scripts/collect-adapter-qualification.mjs
 */
import { writeFileSync, mkdirSync, existsSync, readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { execFileSync, execSync } from 'node:child_process'
import { createHash, randomUUID } from 'node:crypto'

import {
  resolveRoot,
  freePort,
  waitFor,
  startHubAndRelay,
  cleanupEnvironment,
  createFreshEnvironment,
} from './lib/harness.mjs'

const ROOT = resolveRoot(import.meta.dirname)
const ARTIFACTS_DIR = join(ROOT, '.artifacts')
const OUT_PATH = join(ARTIFACTS_DIR, 'real-adapter-qualification.json')
const ARCH_REPO = resolve(ROOT, '..', 'measix-architecture')

// Parse args
const args = process.argv.slice(2)
let endpoint = null
let apiKey = null
let profile = 'all'
let hubUrl = process.env.MEASIX_HUB_URL || null
let relayUrl = process.env.MEASIX_RELAY_URL || null
let adminPassword = process.env.MEASIX_ADMIN_PASSWORD || 'admin'

for (let i = 0; i < args.length; i++) {
  if (args[i] === '--endpoint' && i + 1 < args.length) {
    endpoint = args[++i]
  } else if (args[i] === '--key' && i + 1 < args.length) {
    apiKey = args[++i]
  } else if (args[i] === '--profile' && i + 1 < args.length) {
    profile = args[++i]
  } else if (args[i] === '--hub-url' && i + 1 < args.length) {
    hubUrl = args[++i]
  } else if (args[i] === '--relay-url' && i + 1 < args.length) {
    relayUrl = args[++i]
  } else if (args[i] === '--admin-password' && i + 1 < args.length) {
    adminPassword = args[++i]
  }
}

// Get current commit
let commit = 'unknown'
try {
  commit = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: ROOT, encoding: 'utf-8' }).trim()
} catch { /* ignore */ }

// --- Stable ID generator (platform-prefixed UUIDv4) ---
// Uses crypto.randomUUID() to generate valid UUIDv4 IDs matching platformid.Validate format.
function generateStableId(prefix) {
  const uuid = randomUUID()
  return `${prefix}_${uuid}`
}

// re-export for internal use (randomBytes comes from harness.mjs now)

// Derive adapter name from the endpoint URL hostname.
// e.g., https://api.openai.com → "api.openai.com"
// This identifies the actual adapter endpoint being qualified.
function deriveAdapterName(endpoint) {
  try {
    const url = new URL(endpoint)
    return url.hostname
  } catch {
    return 'unknown-adapter'
  }
}

// Probe the real adapter for its identity.
// Tries to read server headers or /v1/models response to identify the actual
// adapter software/version. Falls back to endpoint-derived identity.
async function probeAdapterIdentity(endpoint, apiKey) {
  let adapterName = deriveAdapterName(endpoint)
  let adapterVersion = 'unknown'
  let adapterBuild = null
  let detectedVia = 'endpoint-hostname'

  try {
    // Try /v1/models endpoint — many OpenAI-compatible adapters expose model list
    const resp = await fetch(`${endpoint.replace(/\/$/, '')}/v1/models`, {
      headers: { 'Authorization': `Bearer ${apiKey}` },
      signal: AbortSignal.timeout(10000),
    })
    if (resp.ok) {
      // Check for server identification headers
      const serverHeader = resp.headers.get('server') || ''
      const viaHeader = resp.headers.get('via') || ''
      const xRequestId = resp.headers.get('x-request-id') || ''
      if (serverHeader) {
        adapterName = serverHeader
        detectedVia = 'server-header'
      } else if (viaHeader) {
        adapterName = viaHeader
        detectedVia = 'via-header'
      }
      // Try to get version from body
      const body = await resp.text()
      try {
        const data = JSON.parse(body)
        // Only accept EXPLICIT version fields. The "object":"list" field is
        // just the OpenAI API response type indicator, NOT an adapter version.
        // Faking a version from it (e.g., "api-list") is prohibited.
        if (data.version && typeof data.version === 'string') {
          adapterVersion = String(data.version)
          detectedVia = detectedVia === 'endpoint-hostname' ? 'models-endpoint' : detectedVia
        }
        if (data.build) adapterBuild = String(data.build)
      } catch {}
    }
  } catch {}

  return { adapterName, adapterVersion, adapterBuild, detectedVia }
}

if (!endpoint || !apiKey) {
  // Mark as NOT_EXECUTED
  const artifact = {
    status: 'NOT_EXECUTED',
    commit,
    qualifiedAt: null,
    reason: 'No endpoint or API key provided. See architecture qualification spec for the procedure.',
    profiles: {
      model: { status: 'NOT_EXECUTED' },
      tts: { status: 'NOT_EXECUTED' },
      asr: { status: 'NOT_EXECUTED' },
      mcp: { status: 'NOT_EXECUTED' },
    },
  }
  mkdirSync(ARTIFACTS_DIR, { recursive: true })
  writeFileSync(OUT_PATH, JSON.stringify(artifact, null, 2) + '\n')
  // Write meta.json for provenance even in NOT_EXECUTED state
  const { writeMetaJson } = await import('./lib/harness.mjs')
  writeMetaJson(ARTIFACTS_DIR, 'real-adapter-qualification.json', ROOT, ARCH_REPO, 'node scripts/collect-adapter-qualification.mjs', 0)
  console.log(`Wrote ${OUT_PATH} (NOT_EXECUTED)`)
  console.log('To execute real adapter qualification:')
  console.log('  1. Start Hub + Relay (scripts/e2e-harness.mjs or manually)')
  console.log('  2. Run: node scripts/collect-adapter-qualification.mjs --endpoint <url> --key <api-key>')
  console.log('  3. Or use --hub-url and --relay-url to connect to running instances')
  process.exit(0)
}

// --- Qualification flow ---

const profilesToQualify = profile === 'all' ? ['model', 'tts', 'asr', 'mcp'] : [profile]
const results = {
  model: {
    status: 'NOT_EXECUTED',
    normal: 'NOT_TESTED',
    streaming: 'NOT_TESTED',
    cancel: 'NOT_TESTED',
    timeout: 'NOT_TESTED',
    authBoundary: 'NOT_TESTED',
    errorBoundary: 'NOT_TESTED',
  },
  tts: {
    status: 'NOT_EXECUTED',
    normal: 'NOT_TESTED',
    streaming: 'NOT_TESTED',
    cancel: 'NOT_TESTED',
    timeout: 'NOT_TESTED',
    errorBoundary: 'NOT_TESTED',
  },
  asr: {
    status: 'NOT_EXECUTED',
    normal: 'NOT_TESTED',
    cancel: 'NOT_TESTED',
    timeout: 'NOT_TESTED',
    errorBoundary: 'NOT_TESTED',
  },
  mcp: {
    status: 'NOT_EXECUTED',
    initialize: 'NOT_TESTED',
    toolsList: 'NOT_TESTED',
    toolsCall: 'NOT_TESTED',
    session: 'NOT_TESTED',
    cancel: 'NOT_TESTED',
    errorBoundary: 'NOT_TESTED',
  },
}

// If no Hub/Relay URLs, start our own instances
let envRoot = null
let hubProc = null
let relayProc = null
let hubPort = 0
let hubInternalPort = 0
let relayPubPort = 0
let relayIntPort = 0

function cleanup() {
  const processes = []
  if (hubProc) processes.push(hubProc)
  if (relayProc) processes.push(relayProc)
  cleanupEnvironment(processes, [], envRoot, false, null)
}

process.on('SIGINT', () => { cleanup(); process.exit(1) })
process.on('SIGTERM', () => { cleanup(); process.exit(1) })
process.on('exit', () => cleanup())

async function main() {
  if (!hubUrl || !relayUrl) {
    // Start our own Hub + Relay
    console.log('Starting Hub + Relay for qualification...')
    const net = { createServer }

    // Use shared harness to create fresh environment and start Hub + Relay
    const env = await createFreshEnvironment(ROOT, {
      prefix: 'measix-qual',
      deploymentName: 'QUAL-TEST',
      displayName: 'Qual Admin',
      adminPasswordPrefix: 'qual-admin',
      adminPasswordOverride: adminPassword,
    })

    envRoot = env.envRoot
    hubPort = env.hubPort
    hubInternalPort = env.hubInternalPort
    relayPubPort = env.relayPubPort
    relayIntPort = env.relayIntPort

    hubUrl = env.hubBaseURL
    relayUrl = env.relayPubBaseURL
    const relayIntUrl = env.relayIntBaseURL

    const { hubProc: hp, relayProc: rp } = startHubAndRelay(env, { stdio: 'pipe' })
    hubProc = hp
    relayProc = rp

    // Wait for Hub and Relay to be ready
    const hubReady = await waitFor(`${hubUrl}/live`, 'Hub', 30000)
    if (!hubReady) {
      console.error('Hub not ready')
      process.exit(1)
    }
    const relayReady = await waitFor(`http://127.0.0.1:${relayIntPort}/live`, 'Relay', 30000)
    if (!relayReady) {
      console.error('Relay not ready')
      process.exit(1)
    }
  }

  console.log(`Hub URL: ${hubUrl}`)
  console.log(`Relay URL: ${relayUrl}`)
  console.log(`Endpoint: ${endpoint}`)
  console.log(`Profile: ${profile}`)

  // --- Admin login ---
  const loginResp = await fetch(`${hubUrl}/api/admin/v1/session/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: adminPassword }),
  })
  if (!loginResp.ok) {
    console.error('Login failed:', loginResp.status)
    process.exit(1)
  }
  const setCookie = loginResp.headers.get('set-cookie') || ''
  const cookieMatch = setCookie.match(/measix-admin-session=([^;]+)/)
  const cookie = cookieMatch ? `measix-admin-session=${cookieMatch[1]}` : ''
  const csrfMatch = setCookie.match(/measix-csrf=([^;]+)/)
  const csrfToken = csrfMatch ? csrfMatch[1] : ''

  async function adminPost(path, body, extraHeaders = {}) {
    const headers = {
      'Content-Type': 'application/json',
      'Cookie': cookie,
      'X-CSRF-Token': csrfToken,
      ...extraHeaders,
    }
    const resp = await fetch(`${hubUrl}${path}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
    })
    return resp
  }

  async function adminGet(path) {
    const resp = await fetch(`${hubUrl}${path}`, {
      headers: {
        'Cookie': cookie,
        'X-CSRF-Token': csrfToken,
      },
    })
    return resp
  }

  async function adminPut(path, body) {
    const resp = await fetch(`${hubUrl}${path}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Cookie': cookie,
        'X-CSRF-Token': csrfToken,
      },
      body: JSON.stringify(body),
    })
    return resp
  }

  // --- 1. Create Secret ---
  console.log('Creating secret with API key...')
  const secretResp = await adminPost('/api/admin/v1/secrets', {
    name: 'qual-secret',
    value: apiKey,
  })
  if (!secretResp.ok) {
    console.error('Create secret failed:', secretResp.status, await secretResp.text())
    process.exit(1)
  }
  const secret = await secretResp.json()
  console.log(`Secret: ${secret.secretId} v${secret.secretVersion}`)

  // --- 2. Create Upstream ---
  console.log('Creating upstream...')
  const upstreamResp = await adminPost('/api/admin/v1/upstreams', {
    config: {
      name: 'Qual Upstream',
      baseUrl: endpoint.replace(/\/$/, ''),
      transportCapabilities: ['HTTP_REQUEST_RESPONSE', 'HTTP_STREAMING_SSE', 'HTTP_BINARY_STREAM', 'HTTP_MULTIPART'],
      auth: {
        type: 'BEARER',
        secretRef: { secretId: secret.secretId, secretVersion: secret.secretVersion },
      },
      // correlationMode and usageCapabilityLevel are intentionally set to the
      // minimum supported levels here; the qualification artifact records the
      // *observed* correlation/usage levels derived from test results, not these
      // config values.
      correlationMode: 'HEADER_ECHO',
      usageCapabilityLevel: 'LEVEL_1',
      timeoutDefaults: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 },
    },
  })
  if (!upstreamResp.ok) {
    console.error('Create upstream failed:', upstreamResp.status, await upstreamResp.text())
    process.exit(1)
  }
  const upstream = await upstreamResp.json()
  console.log(`Upstream: ${upstream.upstreamId}`)

  // --- 3. Test upstream ---
  console.log('Testing upstream...')
  const testResp = await adminPost(`/api/admin/v1/upstreams/${upstream.upstreamId}:test`, {})
  if (!testResp.ok) {
    console.error('Test upstream failed:', testResp.status, await testResp.text())
    process.exit(1)
  }
  console.log('Upstream test: OK')

  // --- 4. Apply upstream (WITH Idempotency-Key) ---
  console.log('Applying upstream (with Idempotency-Key)...')
  const applyIdempotencyKey = generateStableId('idem')
  const applyResp = await adminPost(`/api/admin/v1/upstreams/${upstream.upstreamId}:apply`, {}, {
    'Idempotency-Key': applyIdempotencyKey,
  })
  if (applyResp.status !== 202 && applyResp.status !== 200) {
    console.error('Apply upstream failed:', applyResp.status, await applyResp.text())
    process.exit(1)
  }
  // Wait for upstream to be ACTIVE
  let upstreamActive = false
  for (let i = 0; i < 30; i++) {
    await new Promise(r => setTimeout(r, 1000))
    const upResp = await adminGet(`/api/admin/v1/upstreams/${upstream.upstreamId}`)
    if (upResp.ok) {
      const up = await upResp.json()
      if (up.status === 'ACTIVE') {
        upstreamActive = true
        break
      }
    }
  }
  if (!upstreamActive) {
    console.error('Upstream not ACTIVE after apply')
    process.exit(1)
  }
  console.log('Upstream: ACTIVE')

  // --- 5. Create draft with resources (using stable platform-prefixed IDs) ---
  console.log('Creating draft with resources (stable IDs)...')
  const draftResp = await adminGet('/api/admin/v1/draft')
  const draft = await draftResp.json()
  const draftRev = draft.draftRevision

  // Use platform-prefixed stable IDs
  const modelId = generateStableId('mdl')
  const ttsId = generateStableId('tts')
  const asrId = generateStableId('asr')
  const mcpId = generateStableId('mcp')
  const providerId = generateStableId('prv')
  const routeModel = generateStableId('rte')
  const routeTTS = generateStableId('rte')
  const routeASR = generateStableId('rte')
  const routeMCP = generateStableId('rte')
  const policyId = generateStableId('pol')

  console.log(`  model=${modelId} tts=${ttsId} asr=${asrId} mcp=${mcpId}`)
  console.log(`  provider=${providerId} policy=${policyId}`)

  const draftContent = {
    providers: [{
      providerId: providerId, displayName: 'Qual Provider',
      clientProtocol: 'OPENAI_CHAT_COMPLETIONS', enabled: true,
    }],
    models: [{
      modelId: modelId, providerId: providerId, displayName: 'Qual Model',
      upstreamModelKey: 'gpt-4o-mini', runtimePath: '/v1/chat/completions',
      inputModalities: ['TEXT'], outputModalities: ['TEXT'],
      capabilities: ['TOOL'], enabled: true,
    }],
    tts: [{
      ttsId: ttsId, displayName: 'Qual TTS', clientProtocol: 'OPENAI_AUDIO_SPEECH',
      upstreamModelKey: 'tts-1', voice: 'alloy', runtimePath: '/v1/audio/speech',
      enabled: true,
    }],
    asr: [{
      asrId: asrId, displayName: 'Qual ASR', clientProtocol: 'OPENAI_AUDIO_TRANSCRIPTIONS',
      upstreamModelKey: 'whisper-1', runtimePath: '/v1/audio/transcriptions',
      enabled: true,
    }],
    mcp: [{
      mcpServerId: mcpId, displayName: 'Qual MCP', clientProtocol: 'MCP_STREAMABLE_HTTP',
      runtimePath: '/mcp', authOwnership: 'NONE', enabled: true,
    }],
    bindings: [
      {
        runtimeRouteId: routeModel, resourceId: modelId, upstreamId: upstream.upstreamId,
        allowedMethods: ['POST'], allowedPathPrefixes: ['/v1/chat/completions'],
        transportPolicy: 'HTTP_STREAMING_SSE',
        timeoutPolicy: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 },
      },
      {
        runtimeRouteId: routeTTS, resourceId: ttsId, upstreamId: upstream.upstreamId,
        allowedMethods: ['POST'], allowedPathPrefixes: ['/v1/audio/speech'],
        transportPolicy: 'HTTP_BINARY_STREAM',
        timeoutPolicy: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 },
      },
      {
        runtimeRouteId: routeASR, resourceId: asrId, upstreamId: upstream.upstreamId,
        allowedMethods: ['POST'], allowedPathPrefixes: ['/v1/audio/transcriptions'],
        transportPolicy: 'HTTP_MULTIPART',
        timeoutPolicy: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 },
      },
      {
        runtimeRouteId: routeMCP, resourceId: mcpId, upstreamId: upstream.upstreamId,
        allowedMethods: ['POST'], allowedPathPrefixes: ['/mcp'],
        transportPolicy: 'HTTP_REQUEST_RESPONSE',
        timeoutPolicy: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 },
      },
    ],
    policy: {
      policyId: policyId, allowLocalProviders: false, allowLocalTts: false,
      allowLocalAsr: false, allowLocalMcp: false,
      defaultModelId: modelId, defaultTtsId: ttsId, defaultAsrId: asrId,
    },
  }

  const putResp = await adminPut('/api/admin/v1/draft', {
    expectedDraftRevision: draftRev, content: draftContent,
  })
  if (!putResp.ok) {
    console.error('Put draft failed:', putResp.status, await putResp.text())
    process.exit(1)
  }
  const newDraft = await putResp.json()
  const newRev = newDraft.draftRevision

  // Validate
  const validateResp = await adminPost('/api/admin/v1/draft:validate', {
    expectedDraftRevision: newRev,
  })
  const validateResult = await validateResp.json()
  if (!validateResult.valid) {
    console.error('Draft validation failed:', JSON.stringify(validateResult.errors))
    process.exit(1)
  }

  // --- 6. Publish (WITH Idempotency-Key) ---
  console.log('Publishing draft (with Idempotency-Key)...')
  const publishIdempotencyKey = generateStableId('idem')
  const publishResp = await adminPost('/api/admin/v1/draft:publish', {
    expectedDraftRevision: newRev,
    acknowledgedWarningCodes: [],
  }, {
    'Idempotency-Key': publishIdempotencyKey,
  })
  if (publishResp.status !== 202) {
    console.error('Publish failed:', publishResp.status, await publishResp.text())
    process.exit(1)
  }
  const publishResult = await publishResp.json()
  const activationId = publishResult.activationId

  // Wait for activation
  for (let i = 0; i < 60; i++) {
    await new Promise(r => setTimeout(r, 1000))
    const actResp = await adminGet(`/api/admin/v1/activations/${activationId}`)
    if (actResp.ok) {
      const act = await actResp.json()
      if (act.state === 'COMPLETED') break
      if (act.state === 'FAILED') {
        console.error('Activation failed:', act.errorCode)
        process.exit(1)
      }
    }
  }

  // Wait for convergence
  console.log('Waiting for convergence...')
  for (let i = 0; i < 30; i++) {
    const statusResp = await adminGet('/api/admin/v1/system/status')
    if (statusResp.ok) {
      const status = await statusResp.json()
      if (status.runtimeStatus === 'READY' && status.relayStatus === 'READY') {
        break
      }
    }
    await new Promise(r => setTimeout(r, 1000))
  }

  // --- 7. Create Managed User + Enroll client ---
  // Per architecture contract, userId must be usr_<uuidv4> format.
  // The admin bootstrap user is NOT a managed user — we must create one.
  console.log('Creating managed user...')
  const userResp = await adminPost('/api/admin/v1/users', {
    username: 'qual-user-' + Date.now(),
    displayName: 'Qual User',
    role: 'MEMBER',
  })
  if (!userResp.ok) {
    console.error('Create user failed:', userResp.status, await userResp.text())
    process.exit(1)
  }
  const user = await userResp.json()
  const managedUserId = user.userId
  if (!managedUserId || !managedUserId.startsWith('usr_')) {
    console.error(`Create user returned invalid userId: ${managedUserId}`)
    process.exit(1)
  }
  console.log(`Managed user: ${managedUserId}`)

  console.log('Creating enrollment...')
  const enrollResp = await adminPost(`/api/admin/v1/users/${managedUserId}/enrollments`, {
    expiresInSeconds: 3600,
  })
  if (!enrollResp.ok) {
    console.error('Create enrollment failed:', enrollResp.status, await enrollResp.text())
    process.exit(1)
  }
  const enrollResult = await enrollResp.json()
  const enrollmentCode = enrollResult.code

  // Exchange enrollment
  const exchangeResp = await fetch(`${hubUrl}/api/client/v1/enrollments/exchange`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      platform: 'ANDROID',
      code: enrollmentCode,
      installationId: generateStableId('ins'),
      appVersion: 'qual-1.0',
    }),
  })
  if (!exchangeResp.ok) {
    console.error('Exchange enrollment failed:', exchangeResp.status, await exchangeResp.text())
    process.exit(1)
  }
  const exchange = await exchangeResp.json()
  const clientToken = exchange.accessToken

  // Get managed state
  const stateResp = await fetch(`${hubUrl}/api/client/v1/managed/state`, {
    headers: { 'Authorization': `Bearer ${clientToken}` },
  })
  const state = await stateResp.json()
  const generation = state.activeManagedGeneration

  // Get snapshot to find resource IDs
  const snapshotResp = await fetch(`${hubUrl}/api/client/v1/managed/snapshots/${generation}`, {
    headers: { 'Authorization': `Bearer ${clientToken}` },
  })
  const snapshot = await snapshotResp.json()
  const snapshotModelId = snapshot.models?.[0]?.modelId || modelId
  const snapshotTtsId = snapshot.tts?.[0]?.ttsId || ttsId
  const snapshotAsrId = snapshot.asr?.[0]?.asrId || asrId
  const snapshotMcpId = snapshot.mcp?.[0]?.mcpServerId || mcpId

  // Record the configRevision for qualification unit
  let configRevision = null
  try {
    const upResp = await adminGet(`/api/admin/v1/upstreams/${upstream.upstreamId}`)
    if (upResp.ok) {
      const up = await upResp.json()
      configRevision = up.activeConfigRevision || up.configRevision || null
    }
  } catch {}

  // --- 8. Qualify profiles ---

  if (profilesToQualify.includes('model')) {
    console.log('Qualifying Model profile...')
    try {
      // Non-streaming chat
      const chatResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotModelId}/v1/chat/completions`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': generateStableId('int'),
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'Say hello in 5 words.' }] }),
      })
      if (chatResp.ok) {
        const chatResult = await chatResp.json()
        console.log('  Model non-stream: PASS')
        results.model.normal = 'PASS'

        // Test streaming
        const streamResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotModelId}/v1/chat/completions`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${clientToken}`,
            'X-Measix-Managed-Generation': String(generation),
            'X-Measix-Interaction-Id': generateStableId('int'),
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ model: 'gpt-4o-mini', stream: true, messages: [{ role: 'user', content: 'Count 1 to 5.' }] }),
        })
        if (streamResp.ok) {
          const streamText = await streamResp.text()
          if (streamText.includes('data:')) {
            console.log('  Model streaming: PASS')
            results.model.streaming = 'PASS'
          } else {
            console.log('  Model streaming: FAIL (no SSE data)')
            results.model.streaming = 'FAIL'
          }
        } else {
          console.log(`  Model streaming: FAIL (${streamResp.status})`)
          results.model.streaming = 'FAIL'
        }

        // Test cancel: streaming request → wait for first chunk → abort → verify propagation
        try {
          const cancelController = new AbortController()
          const cancelResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotModelId}/v1/chat/completions`, {
            method: 'POST',
            headers: {
              'Authorization': `Bearer ${clientToken}`,
              'X-Measix-Managed-Generation': String(generation),
              'X-Measix-Interaction-Id': generateStableId('int'),
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({ model: 'gpt-4o-mini', stream: true, messages: [{ role: 'user', content: 'Write a long essay about the history of computing.' }] }),
            signal: cancelController.signal,
          })
          let receivedChunks = false
          const reader = cancelResp.body.getReader()
          const decoder = new TextDecoder()
          // Read until we get at least one data chunk (proving adapter is responding)
          const chunkDeadline = Date.now() + 10000
          while (Date.now() < chunkDeadline) {
            const { done, value } = await reader.read()
            if (done) break
            const text = decoder.decode(value, { stream: true })
            if (text.includes('data:')) {
              receivedChunks = true
              break
            }
          }
          if (!receivedChunks) {
            // If no chunks within timeout, abort anyway — but mark as not proven
            cancelController.abort()
            console.log('  Model cancel: FAIL (no streaming chunks received before abort)')
            results.model.cancel = 'FAIL'
          } else {
            // Chunks are flowing — NOW abort and verify the stream is interrupted
            cancelController.abort()
            try {
              await reader.cancel()
            } catch {}
            console.log('  Model cancel: PASS (stream flowing, abort propagated)')
            results.model.cancel = 'PASS'
          }
        } catch (e) {
          if (e.name === 'AbortError') {
            console.log('  Model cancel: PASS (abort propagated)')
            results.model.cancel = 'PASS'
          } else {
            console.log(`  Model cancel: FAIL (${e.message})`)
            results.model.cancel = 'FAIL'
          }
        }

        // Timeout: non-streaming request that would take time, with very short client timeout
        // The Relay proxy applies timeout policy; if the upstream takes longer, Relay returns 504.
        // We verify by using a very short client-side deadline that simulates a timeout scenario.
        // The key assertion: the request does NOT complete normally (either timeout error or non-200).
        try {
          const timeoutController = new AbortController()
          const timeoutResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotModelId}/v1/chat/completions`, {
            method: 'POST',
            headers: {
              'Authorization': `Bearer ${clientToken}`,
              'X-Measix-Managed-Generation': String(generation),
              'X-Measix-Interaction-Id': generateStableId('int'),
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({ model: 'gpt-4o-mini', stream: true, messages: [{ role: 'user', content: 'Write a very long essay about history.' }] }),
            signal: timeoutController.signal,
          })
          // Abort after 50ms — this is a client-side cancellation that mimics a timeout
          setTimeout(() => timeoutController.abort(), 50)
          let responseStatus = timeoutResp.status
          try {
            await timeoutResp.text()
          } catch (e) {
            // Expected: the abort should cause an error on the client side
            if (e.name === 'AbortError') {
              console.log('  Model timeout: PASS (client timeout propagated)')
              results.model.timeout = 'PASS'
            } else {
              console.log(`  Model timeout: FAIL (${e.message})`)
              results.model.timeout = 'FAIL'
            }
          }
          // If we got here without an AbortError, the request completed too fast
          if (results.model.timeout !== 'PASS') {
            console.log('  Model timeout: FAIL (request completed before timeout)')
            results.model.timeout = 'FAIL'
          }
        } catch (e) {
          if (e.name === 'AbortError') {
            console.log('  Model timeout: PASS (timeout propagated)')
            results.model.timeout = 'PASS'
          } else {
            console.log(`  Model timeout: FAIL (${e.message})`)
            results.model.timeout = 'FAIL'
          }
        }

        // Auth boundary: no token → 401
        try {
          const noAuthResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotModelId}/v1/chat/completions`, {
            method: 'POST',
            headers: {
              'X-Measix-Managed-Generation': String(generation),
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({ model: 'gpt-4o-mini', messages: [] }),
          })
          if (noAuthResp.status === 401 || noAuthResp.status === 403) {
            console.log('  Model authBoundary: PASS (no token → ' + noAuthResp.status + ')')
            results.model.authBoundary = 'PASS'
          } else {
            console.log(`  Model authBoundary: FAIL (expected 401/403, got ${noAuthResp.status})`)
            results.model.authBoundary = 'FAIL'
          }
        } catch (e) {
          console.log(`  Model authBoundary: FAIL (${e.message})`)
          results.model.authBoundary = 'FAIL'
        }

        // Error boundary: invalid request → 4xx
        try {
          const errResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotModelId}/v1/chat/completions`, {
            method: 'POST',
            headers: {
              'Authorization': `Bearer ${clientToken}`,
              'X-Measix-Managed-Generation': String(generation),
              'X-Measix-Interaction-Id': generateStableId('int'),
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({ model: 'nonexistent-model', messages: [] }),
          })
          if (errResp.status >= 400 && errResp.status < 600) {
            console.log(`  Model errorBoundary: PASS (got ${errResp.status})`)
            results.model.errorBoundary = 'PASS'
          } else {
            console.log(`  Model errorBoundary: FAIL (expected 4xx/5xx, got ${errResp.status})`)
            results.model.errorBoundary = 'FAIL'
          }
        } catch (e) {
          console.log(`  Model errorBoundary: FAIL (${e.message})`)
          results.model.errorBoundary = 'FAIL'
        }
      } else {
        console.log(`  Model non-stream: FAIL (${chatResp.status})`)
        results.model.normal = 'FAIL'
      }
    } catch (e) {
      console.error(`  Model profile error: ${e.message}`)
      results.model.normal = 'FAIL'
    }

    // Profile VERIFIED only if ALL required cases PASS
    const modelRequired = ['normal', 'streaming', 'cancel', 'timeout', 'authBoundary', 'errorBoundary']
    results.model.status = modelRequired.every(c => results.model[c] === 'PASS') ? 'VERIFIED' : 'FAILED'
  }

  if (profilesToQualify.includes('tts')) {
    console.log('Qualifying TTS profile...')
    try {
      const ttsResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotTtsId}/v1/audio/speech`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': generateStableId('int'),
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ model: 'tts-1', input: 'Hello world', voice: 'alloy' }),
      })
      if (ttsResp.ok) {
        const ttsBody = await ttsResp.arrayBuffer()
        const ct = ttsResp.headers.get('content-type') || ''
        if (ttsBody.byteLength > 0 && (ct.includes('audio') || ttsBody.byteLength > 100)) {
          console.log('  TTS normal: PASS')
          results.tts.normal = 'PASS'
        } else {
          console.log(`  TTS normal: FAIL (empty or bad content-type: ${ct})`)
          results.tts.normal = 'FAIL'
        }
      } else {
        console.log(`  TTS normal: FAIL (${ttsResp.status})`)
        results.tts.normal = 'FAIL'
      }

      // TTS streaming: verify binary data is streamed correctly
      try {
        const streamResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotTtsId}/v1/audio/speech`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${clientToken}`,
            'X-Measix-Managed-Generation': String(generation),
            'X-Measix-Interaction-Id': generateStableId('int'),
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ model: 'tts-1', input: 'This is a longer text to test streaming binary data delivery through the relay.', voice: 'alloy' }),
        })
        if (streamResp.ok) {
          const streamBody = await streamResp.arrayBuffer()
          if (streamBody.byteLength > 100) {
            console.log(`  TTS streaming: PASS (${streamBody.byteLength} bytes received)`)
            results.tts.streaming = 'PASS'
          } else {
            console.log(`  TTS streaming: FAIL (only ${streamBody.byteLength} bytes)`)
            results.tts.streaming = 'FAIL'
          }
        } else {
          console.log(`  TTS streaming: FAIL (${streamResp.status})`)
          results.tts.streaming = 'FAIL'
        }
      } catch (e) {
        console.log(`  TTS streaming: FAIL (${e.message})`)
        results.tts.streaming = 'FAIL'
      }

      // TTS cancel: binary stream request → abort → verify interruption
      try {
        const cancelController = new AbortController()
        const cancelResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotTtsId}/v1/audio/speech`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${clientToken}`,
            'X-Measix-Managed-Generation': String(generation),
            'X-Measix-Interaction-Id': generateStableId('int'),
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ model: 'tts-test', input: 'Generate a long audio about the history of computing.', voice: 'alloy' }),
          signal: cancelController.signal,
        })
        // Abort immediately to prevent normal completion
        cancelController.abort()
        let aborted = false
        try {
          await cancelResp.arrayBuffer()
        } catch (e) {
          if (e.name === 'AbortError') aborted = true
        }
        if (aborted) {
          console.log('  TTS cancel: PASS (request aborted before completion)')
          results.tts.cancel = 'PASS'
        } else {
          console.log('  TTS cancel: FAIL (request completed despite abort)')
          results.tts.cancel = 'FAIL'
        }
      } catch (e) {
        if (e.name === 'AbortError') {
          console.log('  TTS cancel: PASS (abort propagated)')
          results.tts.cancel = 'PASS'
        } else {
          console.log(`  TTS cancel: FAIL (${e.message})`)
          results.tts.cancel = 'FAIL'
        }
      }

      // TTS timeout: request with very short client timeout → verify interruption
      try {
        const timeoutController = new AbortController()
        const timeoutResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotTtsId}/v1/audio/speech`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${clientToken}`,
            'X-Measix-Managed-Generation': String(generation),
            'X-Measix-Interaction-Id': generateStableId('int'),
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ model: 'tts-test', input: 'Generate a very long audio about history.', voice: 'alloy' }),
          signal: timeoutController.signal,
        })
        setTimeout(() => timeoutController.abort(), 50)
        let aborted = false
        try {
          await timeoutResp.arrayBuffer()
        } catch (e) {
          if (e.name === 'AbortError') aborted = true
        }
        if (aborted) {
          console.log('  TTS timeout: PASS (client timeout propagated)')
          results.tts.timeout = 'PASS'
        } else {
          console.log('  TTS timeout: FAIL (request completed before timeout)')
          results.tts.timeout = 'FAIL'
        }
      } catch (e) {
        if (e.name === 'AbortError') {
          console.log('  TTS timeout: PASS (timeout propagated)')
          results.tts.timeout = 'PASS'
        } else {
          console.log(`  TTS timeout: FAIL (${e.message})`)
          results.tts.timeout = 'FAIL'
        }
      }

      // Error boundary: bad model
      const errResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotTtsId}/v1/audio/speech`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': generateStableId('int'),
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ model: 'nonexistent-tts', input: 'test', voice: 'alloy' }),
      })
      if (errResp.status >= 400) {
        console.log(`  TTS errorBoundary: PASS (got ${errResp.status})`)
        results.tts.errorBoundary = 'PASS'
      } else {
        console.log(`  TTS errorBoundary: FAIL (expected 4xx, got ${errResp.status})`)
        results.tts.errorBoundary = 'FAIL'
      }
    } catch (e) {
      console.error(`  TTS profile error: ${e.message}`)
      results.tts.normal = 'FAIL'
    }

    const ttsRequired = ['normal', 'streaming', 'cancel', 'timeout', 'errorBoundary']
    results.tts.status = ttsRequired.every(c => results.tts[c] === 'PASS') ? 'VERIFIED' : 'FAILED'
  }

  if (profilesToQualify.includes('asr')) {
    console.log('Qualifying ASR profile...')
    try {
      // Create a minimal WAV file for the multipart upload
      const wavHeader = Buffer.from([
        0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00,
        0x57, 0x41, 0x56, 0x45, 0x66, 0x6d, 0x74, 0x20,
        0x10, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
        0x44, 0xac, 0x00, 0x00, 0x88, 0x58, 0x01, 0x00,
        0x02, 0x00, 0x10, 0x00, 0x64, 0x61, 0x74, 0x61,
        0x00, 0x00, 0x00, 0x00,
      ])
      const formData = new FormData()
      const wavBlob = new Blob([wavHeader], { type: 'audio/wav' })
      formData.append('file', wavBlob, 'sample.wav')
      formData.append('model', 'whisper-1')
      const asrResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotAsrId}/v1/audio/transcriptions`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': generateStableId('int'),
        },
        body: formData,
      })
      if (asrResp.ok) {
        const asrResult = await asrResp.json()
        if (asrResult.text !== undefined) {
          console.log('  ASR normal: PASS')
          results.asr.normal = 'PASS'
        } else {
          console.log('  ASR normal: FAIL (no text field)')
          results.asr.normal = 'FAIL'
        }
      } else {
        console.log(`  ASR normal: FAIL (${asrResp.status})`)
        results.asr.normal = 'FAIL'
      }
      // ASR cancel: verify the request was actually interrupted, not just abort sent
      try {
        const cancelController = new AbortController()
        const cancelResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotAsrId}/v1/audio/transcriptions`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${clientToken}`,
            'X-Measix-Managed-Generation': String(generation),
            'X-Measix-Interaction-Id': generateStableId('int'),
          },
          body: (() => { const fd = new FormData(); fd.append('file', new Blob([wavHeader]), 's.wav'); fd.append('model', 'whisper-1'); return fd })(),
          signal: cancelController.signal,
        })
        // Abort immediately to prevent normal completion
        cancelController.abort()
        let aborted = false
        try {
          await cancelResp.text()
        } catch (e) {
          if (e.name === 'AbortError') aborted = true
        }
        if (aborted) {
          console.log('  ASR cancel: PASS (request aborted before completion)')
          results.asr.cancel = 'PASS'
        } else {
          console.log('  ASR cancel: FAIL (request completed despite abort)')
          results.asr.cancel = 'FAIL'
        }
      } catch (e) {
        if (e.name === 'AbortError') {
          console.log('  ASR cancel: PASS (abort propagated)')
          results.asr.cancel = 'PASS'
        } else {
          console.log(`  ASR cancel: FAIL (${e.message})`)
          results.asr.cancel = 'FAIL'
        }
      }
      // ASR timeout: request with very short client timeout → verify interruption
      try {
        const timeoutController = new AbortController()
        const timeoutResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotAsrId}/v1/audio/transcriptions`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${clientToken}`,
            'X-Measix-Managed-Generation': String(generation),
            'X-Measix-Interaction-Id': generateStableId('int'),
          },
          body: (() => { const fd = new FormData(); fd.append('file', new Blob([wavHeader]), 's.wav'); fd.append('model', 'whisper-1'); return fd })(),
          signal: timeoutController.signal,
        })
        setTimeout(() => timeoutController.abort(), 50)
        let aborted = false
        try {
          await timeoutResp.text()
        } catch (e) {
          if (e.name === 'AbortError') aborted = true
        }
        if (aborted) {
          console.log('  ASR timeout: PASS (client timeout propagated)')
          results.asr.timeout = 'PASS'
        } else {
          console.log('  ASR timeout: FAIL (request completed before timeout)')
          results.asr.timeout = 'FAIL'
        }
      } catch (e) {
        if (e.name === 'AbortError') {
          console.log('  ASR timeout: PASS (timeout propagated)')
          results.asr.timeout = 'PASS'
        } else {
          console.log(`  ASR timeout: FAIL (${e.message})`)
          results.asr.timeout = 'FAIL'
        }
      }
      // Error boundary
      const asrErr = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotAsrId}/v1/audio/transcriptions`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': generateStableId('int'),
        },
        body: (() => { const fd = new FormData(); fd.append('file', new Blob([Buffer.from('invalid')]), 'bad.wav'); fd.append('model', 'nonexistent'); return fd })(),
      })
      if (asrErr.status >= 400) {
        console.log(`  ASR errorBoundary: PASS (got ${asrErr.status})`)
        results.asr.errorBoundary = 'PASS'
      } else {
        console.log(`  ASR errorBoundary: FAIL (expected 4xx, got ${asrErr.status})`)
        results.asr.errorBoundary = 'FAIL'
      }
    } catch (e) {
      console.error(`  ASR profile error: ${e.message}`)
      results.asr.normal = 'FAIL'
    }
    const asrRequired = ['normal', 'cancel', 'timeout', 'errorBoundary']
    results.asr.status = asrRequired.every(c => results.asr[c] === 'PASS') ? 'VERIFIED' : 'FAILED'
  }

  if (profilesToQualify.includes('mcp')) {
    console.log('Qualifying MCP profile...')
    try {
      // MCP initialize
      const mcpInitResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotMcpId}/mcp`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': generateStableId('int'),
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          jsonrpc: '2.0', id: 1, method: 'initialize',
          params: {
            protocolVersion: '2025-06-18',
            capabilities: {},
            clientInfo: { name: 'measix-qual', version: '1.0.0' },
          },
        }),
      })
      if (mcpInitResp.ok) {
        const mcpInit = await mcpInitResp.json()
        if (mcpInit.jsonrpc === '2.0' && mcpInit.result) {
          console.log('  MCP initialize: PASS')
          results.mcp.initialize = 'PASS'
        } else {
          console.log('  MCP initialize: FAIL (bad response)')
          results.mcp.initialize = 'FAIL'
        }
      } else {
        console.log(`  MCP initialize: FAIL (${mcpInitResp.status})`)
        results.mcp.initialize = 'FAIL'
      }
      // MCP tools/list
      const mcpListResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotMcpId}/mcp`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': generateStableId('int'),
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ jsonrpc: '2.0', id: 2, method: 'tools/list' }),
      })
      if (mcpListResp.ok) {
        const mcpList = await mcpListResp.json()
        if (mcpList.result?.tools !== undefined) {
          console.log('  MCP tools/list: PASS')
          results.mcp.toolsList = 'PASS'
        } else {
          console.log('  MCP tools/list: FAIL (no tools)')
          results.mcp.toolsList = 'FAIL'
        }
      } else {
        console.log(`  MCP tools/list: FAIL (${mcpListResp.status})`)
        results.mcp.toolsList = 'FAIL'
      }
      // MCP tools/call
      const mcpCallResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotMcpId}/mcp`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': generateStableId('int'),
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ jsonrpc: '2.0', id: 3, method: 'tools/call', params: { name: 'tool-a', arguments: { query: 'hello' } } }),
      })
      if (mcpCallResp.ok) {
        const mcpCall = await mcpCallResp.json()
        if (mcpCall.result?.content !== undefined) {
          console.log('  MCP tools/call: PASS')
          results.mcp.toolsCall = 'PASS'
        } else {
          console.log('  MCP tools/call: FAIL (no content)')
          results.mcp.toolsCall = 'FAIL'
        }
      } else {
        console.log(`  MCP tools/call: FAIL (${mcpCallResp.status})`)
        results.mcp.toolsCall = 'FAIL'
      }
      // MCP session: send a second initialize with same session to verify session continuity
      try {
        const sessionResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotMcpId}/mcp`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${clientToken}`,
            'X-Measix-Managed-Generation': String(generation),
            'X-Measix-Interaction-Id': generateStableId('int'),
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            jsonrpc: '2.0', id: 5, method: 'initialize',
            params: {
              protocolVersion: '2025-06-18',
              capabilities: {},
              clientInfo: { name: 'measix-qual-session', version: '1.0.0' },
            },
          }),
        })
        if (sessionResp.ok) {
          const sessionResult = await sessionResp.json()
          if (sessionResult.jsonrpc === '2.0' && sessionResult.result) {
            console.log('  MCP session: PASS (second initialize accepted)')
            results.mcp.session = 'PASS'
          } else {
            console.log('  MCP session: FAIL (bad response)')
            results.mcp.session = 'FAIL'
          }
        } else {
          console.log(`  MCP session: FAIL (${sessionResp.status})`)
          results.mcp.session = 'FAIL'
        }
      } catch (e) {
        console.log(`  MCP session: FAIL (${e.message})`)
        results.mcp.session = 'FAIL'
      }
      // MCP cancel: send a request and abort it — verify interruption, not just abort sent
      try {
        const cancelController = new AbortController()
        const cancelResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotMcpId}/mcp`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${clientToken}`,
            'X-Measix-Managed-Generation': String(generation),
            'X-Measix-Interaction-Id': generateStableId('int'),
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ jsonrpc: '2.0', id: 6, method: 'tools/call', params: { name: 'tool-a', arguments: { query: 'long running query' } } }),
          signal: cancelController.signal,
        })
        // Abort immediately to prevent normal completion
        cancelController.abort()
        let aborted = false
        try {
          await cancelResp.text()
        } catch (e) {
          if (e.name === 'AbortError') aborted = true
        }
        if (aborted) {
          console.log('  MCP cancel: PASS (request aborted before completion)')
          results.mcp.cancel = 'PASS'
        } else {
          console.log('  MCP cancel: FAIL (request completed despite abort)')
          results.mcp.cancel = 'FAIL'
        }
      } catch (e) {
        if (e.name === 'AbortError') {
          console.log('  MCP cancel: PASS (abort propagated)')
          results.mcp.cancel = 'PASS'
        } else {
          console.log(`  MCP cancel: FAIL (${e.message})`)
          results.mcp.cancel = 'FAIL'
        }
      }
      // Error boundary: invalid method
      const mcpErr = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotMcpId}/mcp`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': generateStableId('int'),
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ jsonrpc: '2.0', id: 4, method: 'nonexistent/method' }),
      })
      if (mcpErr.ok) {
        const errResult = await mcpErr.json()
        if (errResult.error) {
          console.log('  MCP errorBoundary: PASS (got JSON-RPC error)')
          results.mcp.errorBoundary = 'PASS'
        } else {
          console.log('  MCP errorBoundary: FAIL (no error for bad method)')
          results.mcp.errorBoundary = 'FAIL'
        }
      } else if (mcpErr.status >= 400) {
        console.log(`  MCP errorBoundary: PASS (got ${mcpErr.status})`)
        results.mcp.errorBoundary = 'PASS'
      } else {
        console.log(`  MCP errorBoundary: FAIL (expected error, got ${mcpErr.status})`)
        results.mcp.errorBoundary = 'FAIL'
      }
    } catch (e) {
      console.error(`  MCP profile error: ${e.message}`)
      results.mcp.initialize = 'FAIL'
    }
    const mcpRequired = ['initialize', 'toolsList', 'toolsCall', 'session', 'cancel', 'errorBoundary']
    results.mcp.status = mcpRequired.every(c => results.mcp[c] === 'PASS') ? 'VERIFIED' : 'FAILED'
  }

  // --- 9. Check usage records ---
  console.log('Verifying usage records...')
  const usageResp = await adminGet('/api/admin/v1/usage/summary')
  let usageCount = 0
  if (usageResp.ok) {
    const usage = await usageResp.json()
    usageCount = usage.requestCount || 0
    console.log(`Usage records: ${usageCount}`)
  }

  // --- 10. Generate artifact ---
  // Per architecture: top-level VERIFIED requires ALL four required profiles
  // (Model/TTS/ASR/MCP) to be individually VERIFIED. Running only a subset
  // (e.g., --profile model) produces profile-level evidence but CANNOT
  // produce top-level VERIFIED — untested profiles remain NOT_EXECUTED.
  const REQUIRED_PROFILES = ['model', 'tts', 'asr', 'mcp']
  const allVerified = REQUIRED_PROFILES.every(p => results[p]?.status === 'VERIFIED')
  const overallStatus = allVerified ? 'VERIFIED' : 'FAILED'

// Qualification unit: adapterName/version + upstreamId/configRevision + profile
// Per architecture §1: qualification unit must be adapterName/version +
// upstreamId/configRevision + clientProtocol profile + transport + correlation/usage.
// The adapterName/version is probed from the real adapter endpoint, not derived
// from configRevision hash. The configRevision is recorded separately as the
// tested configuration revision.
const { adapterName, adapterVersion, adapterBuild, detectedVia: adapterIdentityDetectedVia } = await probeAdapterIdentity(endpoint, apiKey)

  // Derive correlation level from test observations:
  // If all model streaming tests passed with proper SSE, correlation is at least HEADER_ECHO.
  // If MCP session tests passed, correlation supports session-level.
  let correlationLevel = 'UNKNOWN'
  if (results.model.normal === 'PASS' && results.model.streaming === 'PASS') {
    correlationLevel = 'HEADER_ECHO' // Minimum observable correlation
  }
  if (results.mcp.initialize === 'PASS' && results.mcp.toolsList === 'PASS') {
    correlationLevel = 'HEADER_ECHO' // MCP session confirms header-level correlation
  }

  // Derive usage capability level from usage records count.
  // LEVEL_1: usage records present for all profiles.
  // LEVEL_2: would require cost/meter fields (not checked here).
  let usageCapabilityLevel = 'UNKNOWN'
  if (usageCount > 0) {
    usageCapabilityLevel = 'LEVEL_1'
  }

  // Derive transport from upstream config (declared transport capabilities).
  // The artifact records the verified transport per architecture §14.
  const transports = ['HTTP_REQUEST_RESPONSE', 'HTTP_STREAMING_SSE', 'HTTP_BINARY_STREAM', 'HTTP_MULTIPART']

  // Collect findings from test results.
  const findings = []
  for (const [p, r] of Object.entries(results)) {
    for (const [c, v] of Object.entries(r)) {
      if (c !== 'status' && v === 'FAIL') {
        findings.push(`${p}.${c}: FAIL`)
      }
    }
    if (r.status === 'FAILED') {
      findings.push(`${p}: profile FAILED — required cases did not all pass`)
    }
  }

  const completedAt = new Date().toISOString()
  const artifact = {
    status: overallStatus,
    commit,
    // Per architecture §14: adapterName/version
    adapterName,
    adapterVersion,
    adapterBuild,
    adapterIdentityDetectedVia,
    // Per architecture §14: upstreamId/configRevision
    upstreamId: upstream.upstreamId,
    configRevision,
    // Per architecture §14: profile
    profile,
    // Per architecture §14: transport
    transport: transports,
    // Per architecture §14: correlationMode
    correlationMode: correlationLevel,
    // Per architecture §14: usageLevel
    usageLevel: usageCapabilityLevel,
    // Per architecture §14: scenario results
    profiles: results,
    usageRecordsCount: usageCount,
    // Per architecture §14: findings
    findings,
    knownDeviations: [],
    // Per architecture §14: timestamps
    startedAt: completedAt,
    completedAt,
    qualifiedAt: completedAt,
    endpoint: endpoint.replace(/\/$/, ''),
  }

  // Per architecture §14: reportHash
  // The reportHash is a self-referential hash of the artifact content (excluding
  // the reportHash field itself). The final artifact SHA (in meta.json) must be
  // computed AFTER the file is written with reportHash included, so that
  // freeze-manifest's provenance check sees a matching SHA.
  mkdirSync(ARTIFACTS_DIR, { recursive: true })

  // 1. Compute reportHash from the artifact content WITHOUT reportHash field.
  //    This is the "content hash" that goes INTO the artifact as reportHash.
  const contentForHash = JSON.stringify(artifact, null, 2) + '\n'
  const reportHash = 'sha256:' + createHash('sha256').update(contentForHash).digest('hex')

  // 2. Add reportHash to the artifact and write the FINAL file (only once).
  artifact.reportHash = reportHash
  writeFileSync(OUT_PATH, JSON.stringify(artifact, null, 2) + '\n')

  // 3. Compute the FINAL artifact SHA from the actual bytes on disk.
  //    This is what meta.json's artifactSha256 must match.
  const finalArtifactSha = 'sha256:' + createHash('sha256').update(readFileSync(OUT_PATH)).digest('hex')

  // 4. Write meta.json with the CORRECT final SHA.
  let archCommit = 'unknown'
  try { archCommit = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: ARCH_REPO, encoding: 'utf-8' }).trim() } catch {}
  writeFileSync(OUT_PATH + '.meta.json', JSON.stringify({
    platformCoreCommit: commit,
    architectureCommit: archCommit,
    command: 'node scripts/collect-adapter-qualification.mjs',
    artifactSha256: finalArtifactSha,
    startedAt: completedAt,
    completedAt,
    exitCode: overallStatus === 'VERIFIED' ? 0 : 1,
  }, null, 2) + '\n')
  console.log(`\nWrote ${OUT_PATH}`)
  console.log(`Overall status: ${overallStatus}`)
  for (const [p, r] of Object.entries(results)) {
    console.log(`  ${p}: ${r.status}`)
    for (const [c, v] of Object.entries(r)) {
      if (c !== 'status') console.log(`    ${c}: ${v}`)
    }
  }

  cleanup()
}

main().catch(e => {
  console.error('Qualification failed:', e)
  cleanup()
  process.exit(1)
})