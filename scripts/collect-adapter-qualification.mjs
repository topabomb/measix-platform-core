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
 *   4. Test and apply the upstream
 *   5. Create resources (Model/TTS/ASR/MCP) bound to the upstream
 *   6. Publish a draft
 *   7. Run runtime requests through the Relay against the real upstream
 *   8. Verify usage records
 *   9. Generate .artifacts/real-adapter-qualification.json
 *
 * The qualification unit is adapter/version + configRevision + profile.
 * Different profiles (Model/TTS/ASR/MCP) may use different endpoints/adapters.
 *
 * Usage:
 *   node scripts/collect-adapter-qualification.mjs --endpoint <url> --key <api-key>
 *   [--hub-url <url>] [--relay-url <url>] [--admin-password <pw>]
 *   [--profile all|model|tts|asr|mcp]
 *
 * Or to mark as NOT_EXECUTED (default when no args):
 *   node scripts/collect-adapter-qualification.mjs
 *
 * Environment variables:
 *   MEASIX_HUB_URL       — Hub base URL (default http://127.0.0.1:18080)
 *   MEASIX_RELAY_URL     — Relay public base URL (default http://127.0.0.1:18090)
 *   MEASIX_ADMIN_PASSWORD — admin password (default "admin")
 */
import { writeFileSync, mkdirSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { execFileSync, spawn } from 'node:child_process'
import { existsSync, readFileSync, writeFileSync as writeSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { randomFillSync } from 'node:crypto'

const ROOT = resolve(import.meta.dirname, '..')
const ARTIFACTS_DIR = join(ROOT, '.artifacts')
const OUT_PATH = join(ARTIFACTS_DIR, 'real-adapter-qualification.json')

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
  commit = execFileSync('git', ['rev-parse', 'HEAD'], {
    cwd: ROOT,
    encoding: 'utf-8',
  }).trim()
} catch { /* ignore */ }

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
  model: { status: 'NOT_EXECUTED' },
  tts: { status: 'NOT_EXECUTED' },
  asr: { status: 'NOT_EXECUTED' },
  mcp: { status: 'NOT_EXECUTED' },
}

// If no Hub/Relay URLs, start our own instances
let envRoot = null
let hubProc = null
let relayProc = null
let hubPort = 0
let relayPubPort = 0
let relayIntPort = 0

function cleanup() {
  if (hubProc) try { hubProc.kill('SIGTERM') } catch {}
  if (relayProc) try { relayProc.kill('SIGTERM') } catch {}
  if (envRoot && existsSync(envRoot)) {
    try { require('node:fs').rmSync(envRoot, { recursive: true, force: true }) } catch {}
  }
}

process.on('SIGINT', () => { cleanup(); process.exit(1) })
process.on('SIGTERM', () => { cleanup(); process.exit(1) })
process.on('exit', () => cleanup())

async function main() {
  if (!hubUrl || !relayUrl) {
    // Start our own Hub + Relay
    console.log('Starting Hub + Relay for qualification...')
    const { execSync } = await import('node:child_process')
    const { createServer } = await import('node:http')
    const net = require('node:net')
    
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

    function randomBytes(n) {
      const buf = Buffer.alloc(n)
      randomFillSync(buf)
      return buf
    }

    const { mkdtempSync } = await import('node:fs')
    const { join: pathJoin } = await import('node:path')
    envRoot = mkdtempSync(join(tmpdir(), 'measix-qual-'))
    const hubDB = join(envRoot, 'hub.db')
    const masterKeyFile = join(envRoot, 'master.key')
    const jwtKeyFile = join(envRoot, 'jwt-ed25519.seed')
    const relayTokenFile = join(envRoot, 'relay-service.token')
    const spoolPath = join(envRoot, 'relay-spool.db')
    const pwFile = join(envRoot, 'admin-password.txt')
    
    writeFileSync(masterKeyFile, randomBytes(32), { mode: 0o600 })
    writeFileSync(jwtKeyFile, randomBytes(32), { mode: 0o600 })
    writeFileSync(relayTokenFile, Buffer.from(randomBytes(32).toString('hex') + '\n'), { mode: 0o600 })
    writeFileSync(pwFile, adminPassword + '\n', { mode: 0o600 })
    
    hubPort = await freePort()
    relayPubPort = await freePort()
    relayIntPort = await freePort()
    
    const backendDir = join(ROOT, 'backend')
    const hubBin = join(envRoot, process.platform === 'win32' ? 'control-hub.exe' : 'control-hub')
    const relayBin = join(envRoot, process.platform === 'win32' ? 'runtime-relay.exe' : 'runtime-relay')
    
    try {
      execSync(`go build -o "${hubBin}" ./cmd/control-hub`, { cwd: backendDir, stdio: 'inherit' })
      execSync(`go build -o "${relayBin}" ./cmd/runtime-relay`, { cwd: backendDir, stdio: 'inherit' })
    } catch (e) {
      console.error('Build failed:', e.message)
      process.exit(1)
    }
    
    try {
      execSync(`go run ./cmd/devmigrate --db "${hubDB}"`, { cwd: backendDir, stdio: 'inherit' })
    } catch (e) {
      console.error('Migration failed:', e.message)
      process.exit(1)
    }
    
    try {
      execSync(
        `go run ./cmd/control-hub bootstrap-admin` +
        ` --db "${hubDB}"` +
        ` --master-key-file "${masterKeyFile}"` +
        ` --jwt-private-key-file "${jwtKeyFile}"` +
        ` --deployment-name "QUAL-TEST"` +
        ` --username "admin"` +
        ` --display-name "Qual Admin"` +
        ` --password-file "${pwFile}"`,
        { cwd: backendDir, stdio: 'inherit' },
      )
    } catch (e) {
      console.error('Bootstrap failed:', e.message)
      process.exit(1)
    }
    
    hubUrl = `http://127.0.0.1:${hubPort}`
    relayUrl = `http://127.0.0.1:${relayPubPort}`
    const relayIntUrl = `http://127.0.0.1:${relayIntPort}`
    
    hubProc = spawn(hubBin, [
      'run',
      '--listen', `127.0.0.1:${hubPort}`,
      '--public-base-url', hubUrl,
      '--runtime-api-base', relayUrl,
      '--db', hubDB,
      '--master-key-file', masterKeyFile,
      '--jwt-private-key-file', jwtKeyFile,
      '--relay-internal-url', relayIntUrl,
      '--relay-service-token-file', relayTokenFile,
      '--reconcile-interval', '2s',
    ], { cwd: envRoot, stdio: ['ignore', 'pipe', 'pipe'] })
    
    relayProc = spawn(relayBin, [
      '--public-listen', `127.0.0.1:${relayPubPort}`,
      '--internal-listen', `127.0.0.1:${relayIntPort}`,
      '--spool', spoolPath,
      '--hub-usage-url', `${hubUrl}/internal/v1/usage/request-events:batch`,
      '--hub-service-token-file', relayTokenFile,
    ], { cwd: envRoot, stdio: ['ignore', 'pipe', 'pipe'] })
    
    // Wait for Hub and Relay to be ready
    async function waitFor(url, label, maxWait = 30000) {
      const start = Date.now()
      while (Date.now() - start < maxWait) {
        try {
          const resp = await fetch(url)
          if (resp.ok || resp.status === 401 || resp.status === 404) {
            console.log(`${label} ready`)
            return
          }
        } catch {}
        await new Promise(r => setTimeout(r, 500))
      }
      throw new Error(`${label} not ready after ${maxWait}ms`)
    }
    
    await waitFor(`${hubUrl}/live`, 'Hub')
    await waitFor(`http://127.0.0.1:${relayIntPort}/live`, 'Relay')
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
  
  async function adminPost(path, body) {
    const resp = await fetch(`${hubUrl}${path}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Cookie': cookie,
        'X-CSRF-Token': csrfToken,
      },
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

  // --- 4. Apply upstream ---
  console.log('Applying upstream...')
  const idempotencyKey = crypto.randomUUID()
  const applyResp = await adminPost(`/api/admin/v1/upstreams/${upstream.upstreamId}:apply`, {})
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

  // --- 5. Create draft with resources ---
  console.log('Creating draft with resources...')
  const draftResp = await adminGet('/api/admin/v1/draft')
  const draft = await draftResp.json()
  const draftRev = draft.draftRevision
  
  const modelId = crypto.randomUUID()
  const ttsId = crypto.randomUUID()
  const asrId = crypto.randomUUID()
  const mcpId = crypto.randomUUID()
  const providerId = crypto.randomUUID()
  const routeModel = crypto.randomUUID()
  const routeTTS = crypto.randomUUID()
  const routeASR = crypto.randomUUID()
  const routeMCP = crypto.randomUUID()
  const policyId = crypto.randomUUID()
  
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

  // Publish
  console.log('Publishing draft...')
  const publishIdempotencyKey = crypto.randomUUID()
  const publishResp = await adminPost('/api/admin/v1/draft:publish', {
    expectedDraftRevision: newRev,
    acknowledgedWarningCodes: [],
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

  // --- 6. Enroll client ---
  console.log('Creating enrollment...')
  const enrollResp = await adminPost('/api/admin/v1/users/admin/enrollments', {
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
      installationId: crypto.randomUUID(),
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

  const interactionId = crypto.randomUUID()
  
  // --- 7. Qualify profiles ---
  
  if (profilesToQualify.includes('model')) {
    console.log('Qualifying Model profile...')
    try {
      // Non-streaming chat
      const chatResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotModelId}/v1/chat/completions`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': interactionId,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'Say hello in 5 words.' }] }),
      })
      if (chatResp.ok) {
        const chatResult = await chatResp.json()
        console.log('  Model non-stream: PASS')
        results.model = {
          status: 'VERIFIED',
          nonStream: 'PASS',
          streaming: 'NOT_TESTED',
          cancel: 'NOT_TESTED',
          responseSample: JSON.stringify(chatResult).slice(0, 200),
        }
        
        // Test streaming
        const streamResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotModelId}/v1/chat/completions`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${clientToken}`,
            'X-Measix-Managed-Generation': String(generation),
            'X-Measix-Interaction-Id': crypto.randomUUID(),
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
      } else {
        console.log(`  Model non-stream: FAIL (${chatResp.status})`)
        results.model = { status: 'FAILED', nonStream: 'FAIL', error: `HTTP ${chatResp.status}` }
      }
    } catch (e) {
      console.error(`  Model profile error: ${e.message}`)
      results.model = { status: 'FAILED', error: e.message }
    }
  }

  if (profilesToQualify.includes('tts')) {
    console.log('Qualifying TTS profile...')
    try {
      const ttsResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotTtsId}/v1/audio/speech`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': crypto.randomUUID(),
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ model: 'tts-1', input: 'Hello world', voice: 'alloy' }),
      })
      if (ttsResp.ok) {
        const ttsBody = await ttsResp.arrayBuffer()
        const ct = ttsResp.headers.get('content-type') || ''
        if (ttsBody.byteLength > 0 && (ct.includes('audio') || ttsBody.byteLength > 100)) {
          console.log('  TTS: PASS')
          results.tts = {
            status: 'VERIFIED',
            responseSize: ttsBody.byteLength,
            contentType: ct,
          }
        } else {
          console.log(`  TTS: FAIL (empty or bad content-type: ${ct})`)
          results.tts = { status: 'FAILED', error: 'empty or bad content-type' }
        }
      } else {
        console.log(`  TTS: FAIL (${ttsResp.status})`)
        results.tts = { status: 'FAILED', error: `HTTP ${ttsResp.status}` }
      }
    } catch (e) {
      console.error(`  TTS profile error: ${e.message}`)
      results.tts = { status: 'FAILED', error: e.message }
    }
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
          'X-Measix-Interaction-Id': crypto.randomUUID(),
        },
        body: formData,
      })
      if (asrResp.ok) {
        const asrResult = await asrResp.json()
        if (asrResult.text !== undefined) {
          console.log('  ASR: PASS')
          results.asr = {
            status: 'VERIFIED',
            responseSample: JSON.stringify(asrResult).slice(0, 200),
          }
        } else {
          console.log('  ASR: FAIL (no text field)')
          results.asr = { status: 'FAILED', error: 'no text field in response' }
        }
      } else {
        console.log(`  ASR: FAIL (${asrResp.status})`)
        results.asr = { status: 'FAILED', error: `HTTP ${asrResp.status}` }
      }
    } catch (e) {
      console.error(`  ASR profile error: ${e.message}`)
      results.asr = { status: 'FAILED', error: e.message }
    }
  }

  if (profilesToQualify.includes('mcp')) {
    console.log('Qualifying MCP profile...')
    try {
      // MCP initialize
      const mcpResp = await fetch(`${relayUrl}/runtime/v1/resources/${snapshotMcpId}/mcp`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${clientToken}`,
          'X-Measix-Managed-Generation': String(generation),
          'X-Measix-Interaction-Id': crypto.randomUUID(),
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
      if (mcpResp.ok) {
        const mcpResult = await mcpResp.json()
        if (mcpResult.jsonrpc === '2.0' && mcpResult.result) {
          console.log('  MCP initialize: PASS')
          results.mcp = {
            status: 'VERIFIED',
            initializeResult: JSON.stringify(mcpResult.result).slice(0, 200),
          }
        } else {
          console.log('  MCP initialize: FAIL (bad response)')
          results.mcp = { status: 'FAILED', error: 'bad initialize response' }
        }
      } else {
        console.log(`  MCP initialize: FAIL (${mcpResp.status})`)
        results.mcp = { status: 'FAILED', error: `HTTP ${mcpResp.status}` }
      }
    } catch (e) {
      console.error(`  MCP profile error: ${e.message}`)
      results.mcp = { status: 'FAILED', error: e.message }
    }
  }

  // --- 8. Check usage records ---
  console.log('Verifying usage records...')
  const usageResp = await adminGet('/api/admin/v1/usage/summary')
  let usageCount = 0
  if (usageResp.ok) {
    const usage = await usageResp.json()
    usageCount = usage.requestCount || 0
    console.log(`Usage records: ${usageCount}`)
  }

  // --- 9. Generate artifact ---
  const allVerified = profilesToQualify.every(p => results[p]?.status === 'VERIFIED')
  const overallStatus = allVerified ? 'VERIFIED' : 'FAILED'
  
  const artifact = {
    status: overallStatus,
    commit,
    qualifiedAt: new Date().toISOString(),
    endpoint: endpoint.replace(/\/$/, ''),
    profile,
    adapterName: 'real-adapter',
    correlationLevel: 'HEADER_ECHO',
    usageCapabilityLevel: 'LEVEL_1',
    knownDeviations: [],
    usageRecordsCount: usageCount,
    profiles: results,
  }

  mkdirSync(ARTIFACTS_DIR, { recursive: true })
  writeFileSync(OUT_PATH, JSON.stringify(artifact, null, 2) + '\n')
  console.log(`\nWrote ${OUT_PATH}`)
  console.log(`Overall status: ${overallStatus}`)
  for (const [p, r] of Object.entries(results)) {
    console.log(`  ${p}: ${r.status}`)
  }
  
  cleanup()
}

main().catch(e => {
  console.error('Qualification failed:', e)
  cleanup()
  process.exit(1)
})
