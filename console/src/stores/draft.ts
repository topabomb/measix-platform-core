import { ref } from 'vue'
import { defineStore } from 'pinia'
import type { components } from '../api/generated'
import { ApiProblem, apiFetch, createCandidateId } from '../api/client'

type Draft = components['schemas']['Draft']
type ManagedDraftContent = components['schemas']['ManagedDraftContent']
type ValidateDraftResponse = components['schemas']['ValidateDraftResponse']
type RuntimeBindingDefinition = components['schemas']['RuntimeBindingDefinition']
type TransportPolicy = RuntimeBindingDefinition['transportPolicy']
export type ManagedResourceKind = 'MODEL' | 'IMAGE_GENERATION' | 'TTS' | 'ASR' | 'MCP'
export type TtsProtocol = components['schemas']['TtsDefinition']['clientProtocol']
export type AsrProtocol = components['schemas']['AsrDefinition']['clientProtocol']
export type ImageGenerationProtocol = components['schemas']['ImageGenerationDefinition']['clientProtocol']
export function isRealtimeAsr(protocol: AsrProtocol): boolean {
  return protocol === 'OPENAI_REALTIME_TRANSCRIPTION' || protocol === 'DASHSCOPE_REALTIME_ASR'
}
export function asrTransport(protocol: AsrProtocol): TransportPolicy {
  return protocol === 'OPENAI_AUDIO_TRANSCRIPTIONS' ? 'HTTP_MULTIPART'
    : protocol === 'DASHSCOPE_HTTP_ASR' ? 'HTTP_REQUEST_RESPONSE' : 'WEBSOCKET'
}
export function ttsTransport(protocol: TtsProtocol): TransportPolicy {
  return protocol === 'MIMO_CHAT_COMPLETIONS_TTS' ? 'HTTP_STREAMING_SSE'
    : protocol === 'GEMINI_GENERATE_CONTENT_TTS' ? 'HTTP_REQUEST_RESPONSE' : 'HTTP_BINARY_STREAM'
}

export const useDraftStore = defineStore('draft', () => {
  const baselineContent = ref<ManagedDraftContent>()
  const baselineRevision = ref<number>()
  const localContent = ref<ManagedDraftContent>()
  const dirty = ref(false)
  const loading = ref(false)
  const saving = ref(false)
  const validationResult = ref<ValidateDraftResponse>()
  const conflictRevision = ref<number>()

  function accept(draft: Draft) {
    const content = structuredClone(draft.content)
    content.imageGenerators ??= []
    baselineRevision.value = draft.draftRevision
    baselineContent.value = structuredClone(content)
    localContent.value = content
    dirty.value = false
    conflictRevision.value = undefined
    validationResult.value = undefined
  }

  async function load() {
    loading.value = true
    try {
      const draft = await apiFetch<Draft>('/api/admin/v1/draft')
      accept(draft)
      return draft
    } finally {
      loading.value = false
    }
  }

  function requireContent(): ManagedDraftContent {
    if (!localContent.value) throw new Error('draft is not loaded')
    return localContent.value
  }

  function markDirty() {
    dirty.value = true
    validationResult.value = undefined
    conflictRevision.value = undefined
  }

  function addModel(providerId: string): string {
    const modelId = createCandidateId('mdl')
    requireContent().models.push({
      modelId,
      providerId,
      displayName: 'New model',
      upstreamModelKey: '',
      runtimePath: '/',
      inputModalities: ['TEXT'],
      outputModalities: ['TEXT'],
      capabilities: [],
      enabled: true,
    })
    markDirty()
    return modelId
  }

  function addTts(): string {
    const ttsId = createCandidateId('tts')
    requireContent().tts.push({
      ttsId,
      displayName: 'New TTS',
      clientProtocol: 'OPENAI_AUDIO_SPEECH',
      upstreamModelKey: '',
      voice: 'alloy',
      runtimePath: '/v1/audio/speech',
      enabled: true,
    })
    markDirty()
    return ttsId
  }

  function addAsr(): string {
    const asrId = createCandidateId('asr')
    requireContent().asr.push({
      asrId,
      displayName: 'New ASR',
      clientProtocol: 'OPENAI_AUDIO_TRANSCRIPTIONS',
      upstreamModelKey: '',
      runtimePath: '/v1/audio/transcriptions',
      enabled: true,
    })
    markDirty()
    return asrId
  }

  function addMcp(): string {
    const mcpServerId = createCandidateId('mcp')
    requireContent().mcp.push({
      mcpServerId,
      displayName: 'New MCP Server',
      clientProtocol: 'MCP_STREAMABLE_HTTP',
      runtimePath: '/mcp',
      authOwnership: 'NONE',
      enabled: true,
    })
    markDirty()
    return mcpServerId
  }


  function addAssistant(displayName: string): string {
    const content = requireContent()
    const assistantDefinitionId = createCandidateId('asd')
    content.assistants.push({
      assistantDefinitionId, displayName, systemPrompt: '', modelId: '',
      memorySeed: [], mcpServerIds: [], enabled: true,
    })
    markDirty()
    return assistantDefinitionId
  }

  function removeAssistant(id: string) {
    const content = requireContent()
    content.assistants = content.assistants.filter(a => a.assistantDefinitionId !== id)
    content.starters = content.starters.filter(s => s.assistantDefinitionId !== id)
    if (content.policy.defaultAssistantId === id) delete content.policy.defaultAssistantId
    markDirty()
  }

  function addImageGeneration(): string {
    const imageId = createCandidateId('img')
    requireContent().imageGenerators ??= []
    requireContent().imageGenerators!.push({
      imageId,
      displayName: 'New image generator',
      clientProtocol: 'OPENAI_IMAGES_GENERATIONS',
      upstreamModelKey: '',
      runtimePath: '/v1/images/generations',
      maxImagesPerRequest: 1,
      allowedSizes: ['auto'],
      enabled: true,
    })
    markDirty()
    return imageId
  }

  function addStarter(assistantDefinitionId: string, title: string): string {
    const content = requireContent()
    if (!content.assistants.some(a => a.assistantDefinitionId === assistantDefinitionId)) throw new Error('assistant not found')
    const starterId = createCandidateId('str')
    const nextSortOrder = content.starters
      .filter(starter => starter.assistantDefinitionId === assistantDefinitionId)
      .reduce((highest, starter) => Math.max(highest, starter.sortOrder), -1) + 1
    content.starters.push({ starterId, assistantDefinitionId, title, prompt: '', sortOrder: nextSortOrder, enabled: true })
    markDirty()
    return starterId
  }

  function removeStarter(id: string) {
    const content = requireContent()
    content.starters = content.starters.filter(s => s.starterId !== id)
    markDirty()
  }

  /** Existing binding for a resource, if any. */
  function bindingFor(resourceId: string): RuntimeBindingDefinition | undefined {
    return requireContent().bindings.find((b) => b.resourceId === resourceId)
  }

  function runtimeResourceFor(resourceId: string) {
    const content = requireContent()
    return [...content.models, ...(content.imageGenerators ?? []), ...content.tts, ...content.asr, ...content.mcp]
      .find(resource => ('modelId' in resource ? resource.modelId
        : 'imageId' in resource ? resource.imageId
          : 'ttsId' in resource ? resource.ttsId
          : 'asrId' in resource ? resource.asrId : resource.mcpServerId) === resourceId)
  }

  function setRuntimePath(resourceId: string, runtimePath: string) {
    const resource = runtimeResourceFor(resourceId)
    if (!resource) throw new Error('runtime resource not found')
    resource.runtimePath = runtimePath
    const binding = bindingFor(resourceId)
    if (binding) binding.allowedPathPrefixes = [runtimePath]
    markDirty()
  }

  /**
   * Upsert a runtime binding for an enabled resource. Reuses an existing
   * runtimeRouteId so candidate IDs stay stable across edits; a binding that
   * references no upstream is removed (a resource without an upstream has no
   * valid binding).
   */
  function setBinding(resourceId: string, upstreamId: string, transportPolicy: TransportPolicy) {
    if (!upstreamId) {
      removeBinding(resourceId)
      return
    }
    const content = requireContent()
    const resource = runtimeResourceFor(resourceId)
    if (!resource?.runtimePath) throw new Error('runtime path is required before binding an upstream')
    const existing = content.bindings.find((b) => b.resourceId === resourceId)
    const runtimeRouteId = existing?.runtimeRouteId ?? createCandidateId('rte')
    const defaultMethods = transportPolicy === 'WEBSOCKET' ? ['GET'] : content.mcp.some(mcp => mcp.mcpServerId === resourceId) ? ['POST', 'GET', 'DELETE'] : ['POST']
    const next: RuntimeBindingDefinition = {
      runtimeRouteId,
      resourceId,
      upstreamId,
      allowedMethods: defaultMethods,
      allowedPathPrefixes: [resource.runtimePath],
      transportPolicy,
    }
    if (existing) {
      Object.assign(existing, next)
    } else {
      content.bindings.push(next)
    }
    markDirty()
  }

  /** Remove the runtime binding for a resource (e.g. when it is deleted or unbound). */
  function removeBinding(resourceId: string) {
    const content = requireContent()
    const before = content.bindings.length
    content.bindings = content.bindings.filter((b) => b.resourceId !== resourceId)
    if (content.bindings.length !== before) markDirty()
  }

  function setTtsProtocol(id: string, protocol: TtsProtocol) {
    const content = requireContent()
    const index = content.tts.findIndex(tts => tts.ttsId === id)
    const previous = content.tts[index]
    if (!previous || previous.clientProtocol === protocol) return
    const settings = protocol === 'SYSTEM_TTS' ? { speechRate: 1, pitch: 1 }
      : protocol === 'MIMO_CHAT_COMPLETIONS_TTS' ? { upstreamModelKey: 'mimo-v2.5-tts', voice: 'mimo_default', runtimePath: '/v1/chat/completions' }
        : protocol === 'GEMINI_GENERATE_CONTENT_TTS' ? { upstreamModelKey: 'gemini-2.5-flash-preview-tts', voice: 'Kore', runtimePath: '/v1beta/models/gemini-2.5-flash-preview-tts:generateContent' }
          : { upstreamModelKey: 'gpt-4o-mini-tts', voice: 'alloy', runtimePath: '/v1/audio/speech' }
    content.tts[index] = { ttsId: id, displayName: previous.displayName, enabled: previous.enabled, clientProtocol: protocol, ...settings }
    const binding = bindingFor(id)
    if (protocol === 'SYSTEM_TTS') removeBinding(id)
    else if (binding) setBinding(id, binding.upstreamId, ttsTransport(protocol))
    markDirty()
  }

  function setImageGenerationProtocol(id: string, protocol: ImageGenerationProtocol) {
    const content = requireContent()
    const image = content.imageGenerators?.find(item => item.imageId === id)
    if (!image || image.clientProtocol === protocol) return
    image.clientProtocol = protocol
    image.runtimePath = protocol === 'DASHSCOPE_MULTIMODAL_GENERATION'
      ? '/api/v1/services/aigc/multimodal-generation/generation'
      : '/v1/images/generations'
    image.allowedSizes = protocol === 'DASHSCOPE_MULTIMODAL_GENERATION' ? ['1024x1024'] : ['auto']
    const binding = bindingFor(id)
    if (binding) {
      binding.allowedMethods = ['POST']
      binding.allowedPathPrefixes = [image.runtimePath]
      binding.transportPolicy = 'HTTP_REQUEST_RESPONSE'
    }
    markDirty()
  }

  function setAsrProtocol(id: string, protocol: AsrProtocol) {
    const content = requireContent()
    const index = content.asr.findIndex(asr => asr.asrId === id)
    const previous = content.asr[index]
    if (!previous || previous.clientProtocol === protocol) return
    const settings = protocol === 'OPENAI_REALTIME_TRANSCRIPTION'
      ? { upstreamModelKey: 'gpt-4o-transcribe', runtimePath: '/v1/realtime', sampleRate: 24000 as const, vadThreshold: 0.5, silenceDurationMs: 500, prefixPaddingMs: 300 }
      : protocol === 'DASHSCOPE_REALTIME_ASR'
        ? { upstreamModelKey: 'qwen3-asr-flash-realtime', runtimePath: '/api-ws/v1/realtime', sampleRate: 16000 as const, vadThreshold: 0, silenceDurationMs: 400 }
        : protocol === 'DASHSCOPE_HTTP_ASR'
          ? { upstreamModelKey: 'qwen-audio-3.0-asr-flash', runtimePath: '/api/v1/services/aigc/multimodal-generation/generation' }
        : { upstreamModelKey: 'whisper-1', runtimePath: '/v1/audio/transcriptions' }
    content.asr[index] = { asrId: id, displayName: previous.displayName, enabled: previous.enabled, clientProtocol: protocol, ...settings }
    if (previous.language) content.asr[index]!.language = previous.language
    const binding = bindingFor(id)
    if (binding) {
      binding.allowedMethods = isRealtimeAsr(protocol) ? ['GET'] : ['POST']
      binding.allowedPathPrefixes = [settings.runtimePath]
      binding.transportPolicy = asrTransport(protocol)
    }
    markDirty()
  }

  function resourceReferences(kind: ManagedResourceKind, resourceId: string): string[] {
    const content = requireContent()
    const references: string[] = []
    if (kind === 'MODEL') {
      if (content.policy.defaultModelId === resourceId) references.push('policy.defaultModelId')
      for (const assistant of content.assistants) {
        if (assistant.modelId === resourceId) references.push(`assistant:${assistant.assistantDefinitionId}.modelId`)
      }
    } else if (kind === 'IMAGE_GENERATION' && content.policy.defaultImageGenerationId === resourceId) {
      references.push('policy.defaultImageGenerationId')
    } else if (kind === 'TTS' && content.policy.defaultTtsId === resourceId) {
      references.push('policy.defaultTtsId')
    } else if (kind === 'ASR' && content.policy.defaultAsrId === resourceId) {
      references.push('policy.defaultAsrId')
    } else if (kind === 'MCP') {
      for (const assistant of content.assistants) {
        if (assistant.mcpServerIds.includes(resourceId)) references.push(`assistant:${assistant.assistantDefinitionId}.mcpServerIds`)
      }
    }
    return references
  }

  function removeResource(kind: ManagedResourceKind, resourceId: string): { removed: boolean; references: string[] } {
    const references = resourceReferences(kind, resourceId)
    if (references.length) return { removed: false, references }
    const content = requireContent()
    let removed = false
    if (kind === 'MODEL') {
      const before = content.models.length
      content.models = content.models.filter(item => item.modelId !== resourceId)
      removed = content.models.length !== before
    } else if (kind === 'IMAGE_GENERATION') {
      const before = (content.imageGenerators ?? []).length
      content.imageGenerators = (content.imageGenerators ?? []).filter(item => item.imageId !== resourceId)
      removed = content.imageGenerators.length !== before
    } else if (kind === 'TTS') {
      const before = content.tts.length
      content.tts = content.tts.filter(item => item.ttsId !== resourceId)
      removed = content.tts.length !== before
    } else if (kind === 'ASR') {
      const before = content.asr.length
      content.asr = content.asr.filter(item => item.asrId !== resourceId)
      removed = content.asr.length !== before
    } else {
      const before = content.mcp.length
      content.mcp = content.mcp.filter(item => item.mcpServerId !== resourceId)
      removed = content.mcp.length !== before
    }
    if (removed) {
      removeBinding(resourceId)
      markDirty()
    }
    return { removed, references: [] }
  }

  async function save(csrfToken: string) {
    if (baselineRevision.value === undefined) throw new Error('draft is not loaded')
    saving.value = true
    try {
      const draft = await apiFetch<Draft>('/api/admin/v1/draft', {
        method: 'PUT',
        body: JSON.stringify({ expectedDraftRevision: baselineRevision.value, content: requireContent() }),
      }, csrfToken)
      accept(draft)
      return draft
    } catch (error) {
      if (error instanceof ApiProblem && error.status === 409) conflictRevision.value = error.currentDraftRevision
      throw error
    } finally {
      saving.value = false
    }
  }

  async function validate(csrfToken: string) {
    if (baselineRevision.value === undefined) throw new Error('draft is not loaded')
    validationResult.value = await apiFetch<ValidateDraftResponse>('/api/admin/v1/draft:validate', {
      method: 'POST',
      body: JSON.stringify({ expectedDraftRevision: baselineRevision.value }),
    }, csrfToken)
    return validationResult.value
  }

  return {
    baselineContent, baselineRevision, localContent, dirty, loading, saving, validationResult, conflictRevision,
    load, save, validate, addModel, addImageGeneration, addTts, addAsr, addMcp, addAssistant, removeAssistant, addStarter, removeStarter, markDirty,
    bindingFor, setBinding, setRuntimePath, removeBinding, resourceReferences, removeResource, setImageGenerationProtocol, setTtsProtocol, setAsrProtocol,
  }
})
