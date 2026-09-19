#!/usr/bin/env node
/**
 * Apply the repeatable "real device" enterprise configuration to the local
 * device-demo Hub.  Credentials are read only from the ignored supplier-keys
 * file; the published Snapshot contains resource IDs and never API keys.
 *
 * This is intentionally a whole-draft preset.  It owns the isolated
 * `.data/device-real` database and must not be pointed at an administrator's
 * shared deployment database.
 */
import { existsSync, readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { randomUUID } from 'node:crypto'
import { dirname, resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')
const origin = process.env.MEASIX_REAL_DEVICE_ORIGIN
// The variable carries a path, never the secret itself, so it is named _FILE.
const passwordPath = process.env.MEASIX_REAL_DEVICE_ADMIN_PASSWORD_FILE || resolve(root, '.secrets/device-real-admin-password.txt')
const keyPath = process.env.MEASIX_REAL_DEVICE_SUPPLIER_KEYS || resolve(root, '.secrets/supplier-keys.env')
const statePath = process.env.MEASIX_REAL_DEVICE_STATE || resolve(root, '.data/device-real/preset-state.json')

if (!origin) throw new Error('MEASIX_REAL_DEVICE_ORIGIN is required (for example http://192.168.100.138:9100)')
if (!existsSync(passwordPath)) throw new Error(`Admin password file is missing: ${passwordPath}`)
if (!existsSync(keyPath)) throw new Error(`Supplier key file is missing: ${keyPath}`)

function readEnv(path) {
  const rows = readFileSync(path, 'utf8').split(/\r?\n/)
  const values = {}
  for (const row of rows) {
    const line = row.trim()
    if (!line || line.startsWith('#')) continue
    const equal = line.indexOf('=')
    if (equal <= 0) throw new Error(`Invalid key entry in ${path}`)
    values[line.slice(0, equal)] = line.slice(equal + 1)
  }
  return values
}

function loadState() {
  if (!existsSync(statePath)) return { upstreams: {} }
  const value = JSON.parse(readFileSync(statePath, 'utf8'))
  if (!value || typeof value !== 'object' || !value.upstreams || typeof value.upstreams !== 'object') {
    throw new Error(`Invalid device preset state: ${statePath}`)
  }
  return value
}

const keys = readEnv(keyPath)
for (const name of ['DEEPSEEK_API_KEY', 'MIMO_API_KEY', 'FIRECRAWL_API_KEY', 'DASHSCOPE_API_KEY', 'ALIBABA_TOKEN_PLAN_BASE_URL']) {
  if (!keys[name]) throw new Error(`Missing ${name} in ${keyPath}`)
}
const password = readFileSync(passwordPath, 'utf8').trim()
if (!password) throw new Error(`Admin password is empty: ${passwordPath}`)
const state = loadState()
const saveState = () => {
  mkdirSync(dirname(statePath), { recursive: true })
  writeFileSync(statePath, JSON.stringify(state, null, 2) + '\n', { mode: 0o600 })
}

const login = await fetch(`${origin}/api/admin/v1/session/login`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: 'admin', password }),
})
if (!login.ok) throw new Error(`Admin login failed: HTTP ${login.status}`)
const cookie = login.headers.get('set-cookie')?.match(/measix_admin_session=[^;]+/)?.[0]
const csrf = (await login.json()).csrfToken
if (!cookie || !csrf) throw new Error('Admin login did not return a usable session')

async function api(method, path, body, idempotency = false) {
  const response = await fetch(`${origin}/api/admin/v1${path}`, {
    method,
    headers: {
      Cookie: cookie,
      'X-CSRF-Token': csrf,
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      ...(idempotency ? { 'Idempotency-Key': `idem_${randomUUID()}` } : {}),
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  })
  const result = await response.json().catch(() => undefined)
  if (!response.ok) {
    throw new Error(`${method} ${path} failed: HTTP ${response.status}; ${result?.code || result?.errorCode || 'unknown'}`)
  }
  return result
}

async function getOrCreateUpstream(kind, config, secretName, secretValue) {
  const known = state.upstreams[kind]
  if (known?.upstreamId) {
    try {
      const upstream = await api('GET', `/upstreams/${known.upstreamId}`)
      if (upstream.status === 'ACTIVE') return upstream
    } catch {
      delete state.upstreams[kind]
      saveState()
    }
  }

  const secret = await api('POST', '/secrets', { name: secretName, value: secretValue })
  const upstream = await api('POST', '/upstreams', { config: {
    ...config,
    auth: {
      ...config.auth,
      secretRef: { secretId: secret.secretId, secretVersion: secret.secretVersion },
    },
  } })
  state.upstreams[kind] = { upstreamId: upstream.upstreamId, secretId: secret.secretId, secretVersion: secret.secretVersion }
  saveState()
  const probe = await api('POST', `/upstreams/${upstream.upstreamId}:test`, {})
  if (!probe.reachable) throw new Error(`${kind} upstream is not reachable from this host`)
  await api('POST', `/upstreams/${upstream.upstreamId}:apply`, {}, true)
  for (let attempt = 0; attempt < 30; attempt++) {
    const current = await api('GET', `/upstreams/${upstream.upstreamId}`)
    if (current.status === 'ACTIVE') return current
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
  throw new Error(`${kind} upstream did not become ACTIVE`)
}

const upstreamSpecs = {
  deepseek: {
    secretName: '真机联调 DeepSeek API key', key: keys.DEEPSEEK_API_KEY,
    config: { name: '真机联调 · DeepSeek', baseUrl: 'https://api.deepseek.com',
      transportCapabilities: ['HTTP_REQUEST_RESPONSE', 'HTTP_STREAMING_SSE'], auth: { type: 'BEARER' },
      correlationMode: 'NONE', usageCapabilityLevel: 'LEVEL_0',
      timeoutDefaults: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 } },
  },
  mimo: {
    secretName: '真机联调 MiMo API key', key: keys.MIMO_API_KEY,
    config: { name: '真机联调 · MiMo TTS', baseUrl: 'https://api.xiaomimimo.com',
      transportCapabilities: ['HTTP_REQUEST_RESPONSE', 'HTTP_STREAMING_SSE'], auth: { type: 'STATIC_HEADER', headerName: 'api-key' },
      correlationMode: 'NONE', usageCapabilityLevel: 'LEVEL_0',
      timeoutDefaults: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 } },
  },
  firecrawl: {
    secretName: '真机联调 Firecrawl API key', key: keys.FIRECRAWL_API_KEY,
    config: { name: '真机联调 · Firecrawl MCP', baseUrl: 'https://mcp.firecrawl.dev/v2',
      transportCapabilities: ['HTTP_REQUEST_RESPONSE', 'HTTP_STREAMING_SSE'], auth: { type: 'BEARER' },
      correlationMode: 'NONE', usageCapabilityLevel: 'LEVEL_0',
      timeoutDefaults: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 } },
  },
  alibaba: {
    secretName: '真机联调 百炼 Token Plan 实验密钥', key: keys.DASHSCOPE_API_KEY,
    config: { name: '真机联调 · 百炼新加坡实验', baseUrl: keys.ALIBABA_TOKEN_PLAN_BASE_URL.replace(/\/compatible-mode\/v1\/?$/, ''),
      transportCapabilities: ['HTTP_REQUEST_RESPONSE', 'HTTP_STREAMING_SSE'], auth: { type: 'BEARER' },
      correlationMode: 'NONE', usageCapabilityLevel: 'LEVEL_0',
      timeoutDefaults: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 } },
  },
}
const upstreams = {}
for (const [kind, spec] of Object.entries(upstreamSpecs)) {
  upstreams[kind] = await getOrCreateUpstream(kind, spec.config, spec.secretName, spec.key)
  console.log(`upstream ${kind}: ACTIVE`)
}

const id = {
  deepseekProvider: 'prv_3d005a2a-7e91-4abd-a1a4-5c7d4a2e1111',
  qwenProvider: 'prv_3d005a2a-7e91-4abd-a1a4-5c7d4a2e2222',
  deepseekModel: 'mdl_3d005a2a-7e91-4abd-a1a4-5c7d4a2e1111',
  qwenModel: 'mdl_3d005a2a-7e91-4abd-a1a4-5c7d4a2e2222',
  mimoTts: 'tts_3d005a2a-7e91-4abd-a1a4-5c7d4a2e1111',
  systemTts: 'tts_3d005a2a-7e91-4abd-a1a4-5c7d4a2e2222',
  dashscopeAsr: 'asr_3d005a2a-7e91-4abd-a1a4-5c7d4a2e1111',
  firecrawl: 'mcp_3d005a2a-7e91-4abd-a1a4-5c7d4a2e1111',
  workAssistant: 'asd_3d005a2a-7e91-4abd-a1a4-5c7d4a2e1111',
  qwenAssistant: 'asd_3d005a2a-7e91-4abd-a1a4-5c7d4a2e2222',
  workStarter: 'str_3d005a2a-7e91-4abd-a1a4-5c7d4a2e1111',
  webStarter: 'str_3d005a2a-7e91-4abd-a1a4-5c7d4a2e2222',
  qwenStarter: 'str_3d005a2a-7e91-4abd-a1a4-5c7d4a2e3333',
  deepseekRoute: 'rte_3d005a2a-7e91-4abd-a1a4-5c7d4a2e1111',
  qwenRoute: 'rte_3d005a2a-7e91-4abd-a1a4-5c7d4a2e2222',
  mimoRoute: 'rte_3d005a2a-7e91-4abd-a1a4-5c7d4a2e3333',
  asrRoute: 'rte_3d005a2a-7e91-4abd-a1a4-5c7d4a2e4444',
  mcpRoute: 'rte_3d005a2a-7e91-4abd-a1a4-5c7d4a2e5555',
}
const draft = await api('GET', '/draft')
const policy = {
  ...draft.content.policy,
  allowLocalProviders: true, allowLocalTts: true, allowLocalAsr: true, allowLocalMcp: true, allowLocalAssistants: true,
  defaultModelId: id.deepseekModel, defaultTtsId: id.mimoTts, defaultAsrId: id.dashscopeAsr, defaultAssistantId: id.workAssistant,
}
const content = {
  providers: [
    { providerId: id.deepseekProvider, displayName: 'DeepSeek', clientProtocol: 'OPENAI_CHAT_COMPLETIONS', enabled: true },
    { providerId: id.qwenProvider, displayName: '百炼 Qwen（本机实验）', clientProtocol: 'OPENAI_CHAT_COMPLETIONS', enabled: true },
  ],
  models: [
    { modelId: id.deepseekModel, providerId: id.deepseekProvider, displayName: 'DeepSeek Flash（文字、图片、工具）', upstreamModelKey: 'deepseek-flash', runtimePath: '/chat/completions', inputModalities: ['TEXT', 'IMAGE'], outputModalities: ['TEXT'], capabilities: ['TOOL'], enabled: true },
    { modelId: id.qwenModel, providerId: id.qwenProvider, displayName: 'Qwen 3.8 Flash（文字、图片、工具；本机实验）', upstreamModelKey: 'qwen3.8-flash', runtimePath: '/compatible-mode/v1/chat/completions', inputModalities: ['TEXT', 'IMAGE'], outputModalities: ['TEXT'], capabilities: ['TOOL'], enabled: true },
  ],
  tts: [
    { ttsId: id.mimoTts, displayName: 'MiMo 云端朗读', clientProtocol: 'MIMO_CHAT_COMPLETIONS_TTS', upstreamModelKey: 'mimo-v2.5-tts', voice: 'mimo_default', runtimePath: '/v1/chat/completions', enabled: true },
    { ttsId: id.systemTts, displayName: '设备本地朗读', clientProtocol: 'SYSTEM_TTS', speechRate: 1, pitch: 1, enabled: true },
  ],
  asr: [
    { asrId: id.dashscopeAsr, displayName: '百炼云端录音转写（本机实验）', clientProtocol: 'DASHSCOPE_HTTP_ASR', upstreamModelKey: 'qwen-audio-3.0-asr-flash', runtimePath: '/api/v1/services/aigc/multimodal-generation/generation', enabled: true },
  ],
  mcp: [
    { mcpServerId: id.firecrawl, displayName: 'Firecrawl 网页读取', clientProtocol: 'MCP_STREAMABLE_HTTP', runtimePath: '/mcp', authOwnership: 'NONE', enabled: true },
  ],
  assistants: [
    { assistantDefinitionId: id.workAssistant, displayName: '企业工作助手', description: '日常问答、工作梳理和公开网页资料查阅。', systemPrompt: '你是企业工作助手。用简明中文回答；需要读取公开网页时使用已授权的 Firecrawl 工具。不要声称知道未提供的企业内部事实。', modelId: id.deepseekModel, memorySeed: [], mcpServerIds: [id.firecrawl], enabled: true },
    { assistantDefinitionId: id.qwenAssistant, displayName: '百炼问答助手（本机实验）', description: '使用百炼 Qwen 模型回答日常问题。', systemPrompt: '你是企业问答助手，使用简明中文，区分事实和推测。', modelId: id.qwenModel, memorySeed: [], mcpServerIds: [id.firecrawl], enabled: true },
  ],
  starters: [
    { starterId: id.workStarter, assistantDefinitionId: id.workAssistant, title: '梳理今天的工作', description: '先整理重点，再形成待办清单。', prompt: '请帮我整理今天的工作安排。先问我今天最重要的目标和截止时间。', sortOrder: 0, enabled: true },
    { starterId: id.webStarter, assistantDefinitionId: id.workAssistant, title: '阅读公开网页', description: '提供网址后，概括内容与依据。', prompt: '请帮我阅读一个公开网页。我接下来会提供网址，请先确认网址再开始。', sortOrder: 1, enabled: true },
    { starterId: id.qwenStarter, assistantDefinitionId: id.qwenAssistant, title: '使用百炼梳理问题', description: '切换到百炼问答助手并发起对话。', prompt: '请帮我把这个问题分解为可执行的步骤。', sortOrder: 2, enabled: true },
  ],
  bindings: [
    { runtimeRouteId: id.deepseekRoute, resourceId: id.deepseekModel, upstreamId: upstreams.deepseek.upstreamId, allowedMethods: ['POST'], allowedPathPrefixes: ['/chat/completions'], transportPolicy: 'HTTP_STREAMING_SSE', timeoutPolicy: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 } },
    { runtimeRouteId: id.qwenRoute, resourceId: id.qwenModel, upstreamId: upstreams.alibaba.upstreamId, allowedMethods: ['POST'], allowedPathPrefixes: ['/compatible-mode/v1/chat/completions'], transportPolicy: 'HTTP_STREAMING_SSE', timeoutPolicy: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 } },
    { runtimeRouteId: id.mimoRoute, resourceId: id.mimoTts, upstreamId: upstreams.mimo.upstreamId, allowedMethods: ['POST'], allowedPathPrefixes: ['/v1/chat/completions'], transportPolicy: 'HTTP_STREAMING_SSE', timeoutPolicy: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 } },
    { runtimeRouteId: id.asrRoute, resourceId: id.dashscopeAsr, upstreamId: upstreams.alibaba.upstreamId, allowedMethods: ['POST'], allowedPathPrefixes: ['/api/v1/services/aigc/multimodal-generation/generation'], transportPolicy: 'HTTP_REQUEST_RESPONSE', timeoutPolicy: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 } },
    { runtimeRouteId: id.mcpRoute, resourceId: id.firecrawl, upstreamId: upstreams.firecrawl.upstreamId, allowedMethods: ['POST', 'GET', 'DELETE'], allowedPathPrefixes: ['/mcp'], transportPolicy: 'HTTP_STREAMING_SSE', timeoutPolicy: { connectMs: 5000, responseHeaderMs: 30000, idleMs: 60000 } },
  ],
  policy,
}
const saved = await api('PUT', '/draft', { expectedDraftRevision: draft.draftRevision, content })
const validation = await api('POST', '/draft:validate', { expectedDraftRevision: saved.draftRevision })
if (!validation.valid) {
  throw new Error(`Preset validation failed: ${validation.errors.map(item => `${item.code}:${item.fieldPath}`).join(', ')}`)
}
const preview = await api('POST', '/draft:preview', { expectedDraftRevision: saved.draftRevision })
const changed = preview.diffSummary.added + preview.diffSummary.changed + preview.diffSummary.removed
let activationState = 'already-active'
if (changed > 0 || !preview.publishedGeneration) {
  const activation = await api('POST', '/draft:publish', {
    expectedDraftRevision: saved.draftRevision,
    acknowledgedWarningCodes: [...new Set(validation.warnings.map(item => item.code))],
  }, true)
  activationState = 'pending'
  for (let attempt = 0; attempt < 60; attempt++) {
    const current = await api('GET', `/activations/${activation.activationId}`)
    if (current.state === 'COMPLETED') { activationState = 'completed'; break }
    if (current.state === 'FAILED') throw new Error(`Preset activation failed: ${current.errorCode || 'unknown'}`)
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
  if (activationState !== 'completed') throw new Error('Preset activation did not finish within 60 seconds')
}
console.log(JSON.stringify({
  origin,
  activationState,
  publishedGeneration: preview.publishedGeneration || 'new-release',
  resources: { models: content.models.length, tts: content.tts.length, asr: content.asr.length, mcp: content.mcp.length, assistants: content.assistants.length, starters: content.starters.length },
  usageCapabilityLevel: 'LEVEL_0',
}, null, 2))
