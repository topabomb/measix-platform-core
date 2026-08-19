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
      inputModalities: ['text'],
      outputModalities: ['text'],
      capabilities: [],
      enabled: true,
    })
    markDirty()
    return modelId
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
    load, save, validate, addModel, markDirty,
  }
})
