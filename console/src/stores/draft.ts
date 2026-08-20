import { ref } from 'vue'
import { defineStore } from 'pinia'
import type { components } from '../api/generated'
import { ApiProblem, apiFetch, createCandidateId } from '../api/client'

type Draft = components['schemas']['Draft']
type ManagedDraftContent = components['schemas']['ManagedDraftContent']
type ValidateDraftResponse = components['schemas']['ValidateDraftResponse']

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
    baselineRevision.value = draft.draftRevision
    baselineContent.value = structuredClone(draft.content)
    localContent.value = structuredClone(draft.content)
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
      language: '',
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
    load, save, validate, addModel, addTts, addAsr, addMcp, markDirty,
  }
})
