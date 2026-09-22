import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer,
  QCard, QCardSection, QCardActions, QInput, QBtn, QBanner,
  QSelect, QToggle, QDialog, QSeparator, QTab, QTabs, QBadge, QChip,
  QList, QItem, QItemSection, QItemLabel, QMarkupTable, QSpinner, QIcon,
  QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBtnToggle, QExpansionItem,
  ClosePopup,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory, RouterView } from 'vue-router'
import { h } from 'vue'
import ResourcesPage from './ResourcesPage.vue'
import { useSessionStore } from '../stores/session'
import { useDraftStore } from '../stores/draft'
import { useActivationStore } from '../stores/activation'
import * as client from '../api/client'
import type { components } from '../api/generated'

type Draft = components['schemas']['Draft']

function mountResourcesPage() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: ResourcesPage }],
  })
  const wrapper = mount(
    {
      render() {
        return h(QLayout, {}, () => [
          h(QPageContainer, {}, () => [h(RouterView)]),
        ])
      },
    },
    {
      global: {
        plugins: [[Quasar, {
          components: {
            QLayout, QPage, QPageContainer, QCard, QCardSection, QCardActions,
            QInput, QBtn, QBanner, QSelect, QToggle, QDialog, QSeparator, QTab, QTabs,
            QList, QItem, QItemSection, QItemLabel, QMarkupTable, QChip, QBadge,
            QSpinner, QIcon, QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBtnToggle,
            QExpansionItem,
          },
          directives: { ClosePopup },
        }], pinia, router],
      },
    },
  )
  return { wrapper, pinia }
}

function setupSession(pinia: ReturnType<typeof createPinia>) {
  const session = useSessionStore(pinia)
  session.session = {
    user: { userId: 'usr_001', displayName: 'Admin', role: 'ADMIN' as const },
    csrfToken: 'test-csrf',
    expiresAt: '2026-12-31T23:59:59Z',
  }
  return session
}

const EMPTY_DRAFT: Draft = {
  draftId: 'dft_00000000-0000-4000-8000-000000000001',
  draftRevision: 1,
  content: {
    providers: [],
    models: [],
    imageGenerators: [],
    tts: [],
    asr: [],
    mcp: [],
    bindings: [],
    assistants: [],
    starters: [],
    policy: {
      policyId: 'pol_draft',
      allowLocalProviders: true,
      allowLocalTts: true,
      allowLocalAsr: true,
      allowLocalMcp: true,
      allowLocalAssistants: true,
    },
  },
}

function findAddBtn(wrapper: ReturnType<typeof mount>, label: string) {
  return wrapper.findAllComponents(QBtn).find((b) => String(b.props('label') ?? '').includes(label))
}

async function switchTab(wrapper: ReturnType<typeof mount>, name: string) {
  const tab = wrapper.find(`[data-cy="config-section-${name}"]`)
  expect(tab.exists()).toBe(true)
  await tab.trigger('click')
  await flushPromises()
}

describe('ResourcesPage', () => {
  it('switches ASR settings and binding transport together', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    await switchTab(wrapper, 'asr')
    await wrapper.get('[data-cy="add-asr-btn"]').trigger('click')
    const draft = useDraftStore(pinia)
    const id = draft.localContent!.asr[0]!.asrId
    draft.setBinding(id, 'ups_test', 'HTTP_MULTIPART')
    expect(draft.bindingFor(id)?.allowedPathPrefixes).toEqual(['/v1/audio/transcriptions'])
    await wrapper.get('[data-cy="asr-runtime-path"]').setValue('/custom/transcribe')
    expect(draft.bindingFor(id)?.allowedPathPrefixes).toEqual(['/custom/transcribe'])
    const selector = wrapper.findAllComponents(QSelect).find(select => select.props('options')?.some(option => option?.value === 'OPENAI_REALTIME_TRANSCRIPTION'))
    expect(selector).toBeDefined()
    selector!.vm.$emit('update:modelValue', 'OPENAI_REALTIME_TRANSCRIPTION')
    await flushPromises()
    expect(draft.localContent!.asr[0]).toMatchObject({ sampleRate: 24000, prefixPaddingMs: 300, runtimePath: '/v1/realtime' })
    expect(draft.bindingFor(id)).toMatchObject({ transportPolicy: 'WEBSOCKET', allowedMethods: ['GET'], allowedPathPrefixes: ['/v1/realtime'] })
    expect(wrapper.find('[data-cy="asr-sample-rate"]').exists()).toBe(true)
    selector!.vm.$emit('update:modelValue', 'DASHSCOPE_REALTIME_ASR')
    await flushPromises()
    expect(draft.localContent!.asr[0]!.sampleRate).toBe(16000)
    expect(draft.localContent!.asr[0]).not.toHaveProperty('prefixPaddingMs')
    selector!.vm.$emit('update:modelValue', 'OPENAI_AUDIO_TRANSCRIPTIONS')
    await flushPromises()
    expect(draft.localContent!.asr[0]).not.toHaveProperty('sampleRate')
    expect(draft.bindingFor(id)).toMatchObject({ transportPolicy: 'HTTP_MULTIPART', allowedMethods: ['POST'] })
    selector!.vm.$emit('update:modelValue', 'DASHSCOPE_HTTP_ASR')
    await flushPromises()
    expect(draft.localContent!.asr[0]).toMatchObject({ upstreamModelKey: 'qwen-audio-3.0-asr-flash', runtimePath: '/api/v1/services/aigc/multimodal-generation/generation' })
    expect(draft.bindingFor(id)).toMatchObject({ transportPolicy: 'HTTP_REQUEST_RESPONSE', allowedMethods: ['POST'],
      allowedPathPrefixes: ['/api/v1/services/aigc/multimodal-generation/generation'] })
    expect(wrapper.find('[data-cy="asr-sample-rate"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('creates MCP bindings with the Streamable HTTP session methods', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    const draft = useDraftStore(pinia)
    const id = draft.addMcp()
    draft.setBinding(id, 'ups_test', 'HTTP_REQUEST_RESPONSE')
    expect(draft.bindingFor(id)?.allowedMethods).toEqual(['POST', 'GET', 'DELETE'])
    expect(draft.bindingFor(id)?.allowedPathPrefixes).toEqual(['/mcp'])
    const model = draft.addModel('prv_test')
    draft.setRuntimePath(model, '/v1/chat/completions')
    draft.setBinding(model, 'ups_test', 'HTTP_STREAMING_SSE')
    expect(draft.bindingFor(model)?.allowedMethods).toEqual(['POST'])
    expect(draft.bindingFor(model)?.allowedPathPrefixes).toEqual(['/v1/chat/completions'])
    wrapper.unmount()
  })
  it('authors system and MiMo TTS without retaining fields from the other execution type', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    await switchTab(wrapper, 'tts')
    await wrapper.get('[data-cy="add-tts-btn"]').trigger('click')
    await flushPromises()
    const draft = useDraftStore(pinia)
    const id = draft.localContent!.tts[0]!.ttsId
    draft.setBinding(id, 'ups_test', 'HTTP_BINARY_STREAM')
    const selector = wrapper.findAllComponents(QSelect).find(select => select.props('options')?.some(option => option?.value === 'SYSTEM_TTS'))
    expect(selector).toBeDefined()
    selector!.vm.$emit('update:modelValue', 'SYSTEM_TTS')
    await flushPromises()
    expect(draft.localContent!.tts[0]).toMatchObject({clientProtocol: 'SYSTEM_TTS', speechRate: 1, pitch: 1})
    expect(draft.localContent!.tts[0]).not.toHaveProperty('runtimePath')
    expect(draft.bindingFor(id)).toBeUndefined()
    expect(wrapper.find('[data-cy="tts-upstream-select"]').exists()).toBe(false)
    expect(wrapper.find('[data-cy="tts-model-key"]').exists()).toBe(false)
    expect(wrapper.find('[data-cy="tts-speech-rate"]').exists()).toBe(true)
    selector!.vm.$emit('update:modelValue', 'MIMO_CHAT_COMPLETIONS_TTS')
    await flushPromises()
    expect(draft.localContent!.tts[0]).not.toHaveProperty('speechRate')
    expect(draft.localContent!.tts[0]!.runtimePath).toBe('/v1/chat/completions')
    await wrapper.get('[data-cy="tts-model-key"]').setValue('mimo-v2.5-tts-voicedesign')
    expect(draft.localContent!.tts[0]).not.toHaveProperty('voice')
    expect(wrapper.find('[data-cy="tts-voice"]').exists()).toBe(false)
    expect(wrapper.find('[data-cy="tts-voice-design"]').exists()).toBe(true)
    wrapper.unmount()
  })
  it.each(['OPENAI_RESPONSES', 'GOOGLE_GENERATE_CONTENT', 'ANTHROPIC_MESSAGES'] as const)('lets administrators select %s explicitly without guessing from a provider name', async (protocol) => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    await wrapper.get('[data-cy="add-provider-btn"]').trigger('click')
    await flushPromises()
    const selector = wrapper.findAllComponents(QSelect).find(select =>
      select.props('options')?.some(option => option?.value === protocol),
    )
    expect(selector).toBeDefined()
    selector!.vm.$emit('update:modelValue', protocol)
    await flushPromises()
    const draft = useDraftStore(pinia)
    expect(draft.localContent!.providers[0]!.clientProtocol).toBe(protocol)
    expect(draft.dirty).toBe(true)
    if (protocol === 'GOOGLE_GENERATE_CONTENT') {
      await wrapper.get('[data-cy="config-section-models"]').trigger('click')
      await wrapper.get('[data-cy="add-model-btn"]').trigger('click')
      await flushPromises()
      expect(wrapper.text()).toContain('Gemini includes the model in its endpoint path')
    }
    wrapper.unmount()
  })
  it('shows the effective downstream model key and keeps the provider key explicit', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    await wrapper.get('[data-cy="add-provider-btn"]').trigger('click')
    await switchTab(wrapper, 'models')
    await wrapper.get('[data-cy="add-model-btn"]').trigger('click')
    await flushPromises()

    const draft = useDraftStore(pinia)
    await wrapper.get('[data-cy="model-upstream-key"]').setValue('provider-model-v1')
    expect(wrapper.text()).toContain('Device uses provider-model-v1 → Core forwards provider-model-v1')
    await wrapper.get('[data-cy="model-published-key"]').setValue('workshop-assistant')
    expect(draft.localContent!.models[0]).toMatchObject({
      publishedModelKey: 'workshop-assistant',
      upstreamModelKey: 'provider-model-v1',
    })
    expect(wrapper.text()).toContain('Device uses workshop-assistant → Core forwards provider-model-v1')
    wrapper.unmount()
  })
  beforeEach(() => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/draft') return structuredClone(EMPTY_DRAFT)
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      return {}
    })
  })

  it('renders all resource and experience tabs', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    for (const expected of ['overview', 'models', 'image-generation', 'tts', 'asr', 'mcp', 'assistants', 'policy']) {
      expect(wrapper.find(`[data-cy="config-section-${expected}"]`).exists()).toBe(true)
    }
  })

  it('localizes validation issues and opens the exact assistant field', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    draft.localContent!.assistants.push({
      assistantDefinitionId: 'asd_helper',
      displayName: 'Workshop helper',
      description: '',
      systemPrompt: 'Help the operator.',
      memorySeed: [],
      modelId: 'mdl_missing',
      mcpServerIds: [],
      enabled: true,
    })
    draft.validationResult = {
      valid: false,
      errors: [{
        code: 'invalid_model_ref',
        path: 'assistants[0].modelId',
        message: 'assistant references an unknown or disabled model',
        severity: 'ERROR',
        resourceKind: 'ASSISTANT',
        resourceId: 'asd_helper',
        field: 'modelId',
      }],
      warnings: [],
    }
    await flushPromises()

    const issue = wrapper.get('[data-validation-code="invalid_model_ref"]')
    expect(issue.text()).toContain('Select an enabled model. The current model is missing or disabled.')
    expect(issue.text()).not.toContain('assistant references an unknown or disabled model')

    await issue.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-field="modelId"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Workshop helper')
  })

  it('presents Android-familiar configuration sections with current summaries', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    expect(wrapper.find('[data-cy="configuration-section-nav"]').exists()).toBe(true)
    for (const section of ['overview', 'models', 'image-generation', 'tts', 'asr', 'mcp', 'assistants', 'policy']) {
      expect(wrapper.find(`[data-cy="config-section-${section}"]`).exists()).toBe(true)
    }
    expect(wrapper.get('[data-cy="config-section-models"]').text()).toContain('0')
    expect(wrapper.get('[data-cy="config-section-policy"]').text()).toContain('Local allowed 5/5')
  })

  it('exposes one shared list/detail state for narrow resource navigation', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    await wrapper.get('[data-cy="add-provider-btn"]').trigger('click')
    await flushPromises()
    await switchTab(wrapper, 'models')
    await wrapper.get('[data-cy="add-model-btn"]').trigger('click')
    await flushPromises()
    const split = wrapper.get('.resource-split')
    expect(split.classes()).toContain('resource-split--selected')
    const back = wrapper.get('.resource-detail-back')
    expect(back.text()).toContain('Back to list')
    await back.trigger('click')
    expect(split.classes()).not.toContain('resource-split--selected')
  })

  it('keeps draft editing available and retries when upstream discovery fails', async () => {
    let upstreamAttempts = 0
    vi.mocked(client.apiFetch).mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/draft') return structuredClone(EMPTY_DRAFT)
      if (path.startsWith('/api/admin/v1/upstreams')) {
        upstreamAttempts++
        if (upstreamAttempts === 1) throw new Error('upstream discovery unavailable')
        return { items: [], nextCursor: undefined }
      }
      return {}
    })
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const banner = wrapper.get('[data-cy="upstream-load-error"]')
    expect(banner.text()).toContain('Upstream')
    expect(wrapper.find('[data-cy="configuration-section-nav"]').exists()).toBe(true)

    const retry = banner.findComponent(QBtn)
    await retry.trigger('click')
    await flushPromises()
    expect(upstreamAttempts).toBe(2)
    expect(wrapper.find('[data-cy="upstream-load-error"]').exists()).toBe(false)
  })

  it('authors assistant seeds and starters in the shared draft workflow', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    const draft = useDraftStore(pinia)
    await switchTab(wrapper, 'assistants')
    await wrapper.get('[data-cy="assistant-add"]').trigger('click')
    const assistant = draft.localContent!.assistants[0]!
    expect(assistant.assistantDefinitionId).toMatch(/^asd_/)
    const identity = wrapper.get('[data-cy="assistant-identity"]')
    expect(identity.text()).toContain(assistant.assistantDefinitionId)
    expect(identity.attributes('open')).toBeUndefined()
    expect(assistant.memorySeed).toEqual([])
    await wrapper.get('[data-cy="assistant-name"]').setValue('Inspector')
    await wrapper.get('[data-cy="assistant-section-memory"]').trigger('click')
    await wrapper.get('[data-cy="seed-add"]').trigger('click')
    await wrapper.get('[data-cy="seed-input-0"]').setValue('first memory')
    await wrapper.get('[data-cy="seed-add"]').trigger('click')
    await wrapper.get('[data-cy="seed-input-1"]').setValue('second memory')
    await wrapper.get('[data-cy="seed-up-1"]').trigger('click')
    expect(assistant.memorySeed).toEqual(['second memory', 'first memory'])
    await wrapper.get('[data-cy="assistant-section-starters"]').trigger('click')
    await wrapper.get('[data-cy="starter-add"]').trigger('click')
    expect(draft.localContent!.starters[0]!.starterId).toMatch(/^str_/)
    expect(draft.localContent!.starters[0]!.assistantDefinitionId).toBe(assistant.assistantDefinitionId)
    expect(draft.dirty).toBe(true)
    wrapper.unmount()
  })

  it('appends a new starter after the existing starter in Android display order', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    const draft = useDraftStore(pinia)
    await switchTab(wrapper, 'assistants')
    await wrapper.get('[data-cy="assistant-add"]').trigger('click')
    await wrapper.get('[data-cy="assistant-section-starters"]').trigger('click')
    await wrapper.get('[data-cy="starter-add"]').trigger('click')
    await wrapper.get('[data-cy="starter-add"]').trigger('click')

    expect(draft.localContent!.starters.map(starter => starter.sortOrder)).toEqual([0, 1])
    wrapper.unmount()
  })

  it('shows recovery guidance only while an activation is pending', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    const activation = useActivationStore(pinia)
    const base = {
      activationId: 'act_00000000-0000-4000-8000-000000000001',
      kind: 'PUBLISH' as const,
      desiredControlRevision: 1,
      createdAt: '2026-09-18T00:00:00Z',
      updatedAt: '2026-09-18T00:00:00Z',
    }
    activation.accept({ ...base, state: 'APPLYING' })
    await flushPromises()
    expect(wrapper.text()).toContain('Recovery: refresh the page')
    expect(wrapper.text()).not.toContain('Staging Release')
    expect(wrapper.text()).not.toContain('Applying Runtime')

    activation.accept({ ...base, state: 'FAILED', errorCode: 'RUNTIME_UNAVAILABLE' })
    await flushPromises()
    expect(wrapper.text()).toContain('RUNTIME_UNAVAILABLE')
    expect(wrapper.text()).not.toContain('Staging Release')

    activation.accept({ ...base, state: 'COMPLETED' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('Recovery: refresh the page')
    wrapper.unmount()
  })

  it('organizes assistant settings into Android-familiar detail sections', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    await switchTab(wrapper, 'assistants')
    await wrapper.get('[data-cy="assistant-add"]').trigger('click')

    for (const section of ['basic', 'prompt', 'memory', 'connections', 'starters']) {
      expect(wrapper.find(`[data-cy="assistant-section-${section}"]`).exists()).toBe(true)
    }
    expect(wrapper.find('[data-cy="assistant-name"]').exists()).toBe(true)
    expect(wrapper.find('[data-cy="assistant-prompt"]').exists()).toBe(false)
    await wrapper.get('[data-cy="assistant-section-prompt"]').trigger('click')
    expect(wrapper.find('[data-cy="assistant-prompt"]').exists()).toBe(true)
  })

  it('can add a TTS resource through the Add button', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    expect(draft.localContent?.tts).toHaveLength(0)

    await switchTab(wrapper, 'tts')
    const addTtsBtn = findAddBtn(wrapper, 'Add')
    expect(addTtsBtn).toBeTruthy()
    await addTtsBtn!.trigger('click')
    await flushPromises()

    expect(draft.localContent?.tts).toHaveLength(1)
    expect(draft.localContent?.tts[0].ttsId).toMatch(/^tts_/)
    expect(draft.localContent?.tts[0].clientProtocol).toBe('OPENAI_AUDIO_SPEECH')
    expect(draft.dirty).toBe(true)
  })

  it('authors standalone image generation and leaves its policy default unset until selected', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    const draft = useDraftStore(pinia)

    await switchTab(wrapper, 'image-generation')
    await wrapper.get('[data-cy="add-image-generation-btn"]').trigger('click')
    await flushPromises()

    expect(draft.localContent?.imageGenerators).toHaveLength(1)
    expect(draft.localContent?.imageGenerators?.[0]).toMatchObject({
      imageId: expect.stringMatching(/^img_/),
      clientProtocol: 'OPENAI_IMAGES_GENERATIONS',
      runtimePath: '/v1/images/generations',
      maxImagesPerRequest: 1,
      allowedSizes: ['auto'],
      enabled: true,
    })
    expect(draft.localContent?.policy.defaultImageGenerationId).toBeUndefined()
    expect(wrapper.find('[data-cy="image-generation-upstream-select"]').exists()).toBe(true)

    const imageId = draft.localContent!.imageGenerators![0]!.imageId
    draft.setBinding(imageId, 'ups_test', 'HTTP_REQUEST_RESPONSE')
    draft.setImageGenerationProtocol(imageId, 'DASHSCOPE_MULTIMODAL_GENERATION')
    expect(draft.localContent!.imageGenerators![0]).toMatchObject({
      clientProtocol: 'DASHSCOPE_MULTIMODAL_GENERATION',
      runtimePath: '/api/v1/services/aigc/multimodal-generation/generation',
      allowedSizes: ['1024x1024'],
    })
    expect(draft.bindingFor(imageId)).toMatchObject({
      upstreamId: 'ups_test',
      transportPolicy: 'HTTP_REQUEST_RESPONSE',
      allowedPathPrefixes: ['/api/v1/services/aigc/multimodal-generation/generation'],
    })
    expect(wrapper.find('[data-cy="image-generation-protocol"]').exists()).toBe(true)

    await switchTab(wrapper, 'policy')
    const selector = wrapper.get('[data-cy="policy-default-image-generation"]')
    expect(selector.attributes('modelvalue')).toBeUndefined()
  })

  it('normalizes a draft with the additive image collection missing and writes only the current shape', async () => {
    const stored = structuredClone(EMPTY_DRAFT)
    delete stored.content.imageGenerators
    vi.mocked(client.apiFetch).mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/draft') return structuredClone(stored)
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [] }
      return {}
    })
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    const draft = useDraftStore(pinia)
    expect(draft.localContent?.imageGenerators).toEqual([])
    expect(() => draft.addImageGeneration()).not.toThrow()
    wrapper.unmount()
  })

  it('keeps every optional policy default explicitly unset instead of selecting the first resource', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    await switchTab(wrapper, 'policy')
    const draft = useDraftStore(pinia)
    const defaults = [
      ['Default Model', 'defaultModelId'],
      ['Fast task model', 'defaultFastModelId'],
      ['Title generation model', 'defaultTitleModelId'],
      ['Attachment inspection model', 'defaultAttachmentInspectionModelId'],
      ['Suggestion model', 'defaultSuggestionModelId'],
      ['Context compaction model', 'defaultCompressModelId'],
      ['Default Image Generation', 'defaultImageGenerationId'],
      ['Default TTS', 'defaultTtsId'],
      ['Default ASR', 'defaultAsrId'],
      ['Assistant for first use', 'defaultAssistantId'],
    ] as const
    for (const [label, key] of defaults) {
      const selector = wrapper.findAllComponents(QSelect).find(item => item.props('label') === label)!
      expect(selector).toBeDefined()
      expect(selector.props('modelValue')).toBeUndefined()
      selector.vm.$emit('update:modelValue', 'temporary_id')
      await flushPromises()
      selector.vm.$emit('update:modelValue', null)
      await flushPromises()
      expect(Object.hasOwn(draft.localContent!.policy, key)).toBe(false)
    }
  })

  it('offers only image-capable enabled models for attachment inspection', async () => {
    const stored = structuredClone(EMPTY_DRAFT)
    stored.content.models = [
      { modelId: 'mdl_text', providerId: 'prv_1', displayName: 'Text only', upstreamModelKey: 'text', runtimePath: '/chat', inputModalities: ['TEXT'], outputModalities: ['TEXT'], capabilities: [], enabled: true },
      { modelId: 'mdl_vision', providerId: 'prv_1', displayName: 'Vision', upstreamModelKey: 'vision', runtimePath: '/chat', inputModalities: ['TEXT', 'IMAGE'], outputModalities: ['TEXT'], capabilities: [], enabled: true },
      { modelId: 'mdl_disabled', providerId: 'prv_1', displayName: 'Disabled vision', upstreamModelKey: 'disabled', runtimePath: '/chat', inputModalities: ['IMAGE'], outputModalities: ['TEXT'], capabilities: [], enabled: false },
    ]
    vi.mocked(client.apiFetch).mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/draft') return structuredClone(stored)
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [] }
      return {}
    })
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    await switchTab(wrapper, 'policy')

    const selector = wrapper.findAllComponents(QSelect).find(item => item.props('label') === 'Attachment inspection model')!
    expect(selector).toBeDefined()
    expect(selector.props('options')).toEqual([{ label: 'Vision', value: 'mdl_vision' }])
  })

  it('can add an ASR resource through the Add button', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    expect(draft.localContent?.asr).toHaveLength(0)

    await switchTab(wrapper, 'asr')
    const addAsrBtn = findAddBtn(wrapper, 'Add')
    expect(addAsrBtn).toBeTruthy()
    await addAsrBtn!.trigger('click')
    await flushPromises()

    expect(draft.localContent?.asr).toHaveLength(1)
    expect(draft.localContent?.asr[0].asrId).toMatch(/^asr_/)
    expect(draft.localContent?.asr[0].clientProtocol).toBe('OPENAI_AUDIO_TRANSCRIPTIONS')
    expect(draft.dirty).toBe(true)
  })

  it('can add an MCP server through the Add button', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    expect(draft.localContent?.mcp).toHaveLength(0)

    await switchTab(wrapper, 'mcp')
    const addMcpBtn = findAddBtn(wrapper, 'Add')
    expect(addMcpBtn).toBeTruthy()
    await addMcpBtn!.trigger('click')
    await flushPromises()

    expect(draft.localContent?.mcp).toHaveLength(1)
    expect(draft.localContent?.mcp[0].mcpServerId).toMatch(/^mcp_/)
    expect(draft.localContent?.mcp[0].clientProtocol).toBe('MCP_STREAMABLE_HTTP')
    expect(draft.dirty).toBe(true)
  })

  it('renders Policy editor with toggles for local allow flags', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    await switchTab(wrapper, 'policy')
    const toggles = wrapper.findAllComponents(QToggle)
    expect(toggles.length).toBeGreaterThanOrEqual(4)

    const draft = useDraftStore(pinia)
    expect(draft.localContent?.policy?.allowLocalProviders).toBe(true)
    expect(wrapper.findAll('[data-cy="policy-setting-row"]').length).toBe(5)
    expect(wrapper.get('[data-cy="policy-setting-row"]').text()).toContain('enterprise')
  })

  it('shows relationship rows in the Overview tab for each resource to its upstream', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    draft.localContent!.providers.push({
      providerId: 'prv_openai',
      displayName: 'OpenAI',
      clientProtocol: 'OPENAI_CHAT_COMPLETIONS',
      enabled: true,
    })
    const modelId = draft.addModel('prv_openai')
    draft.setBinding(modelId, 'ups_test', 'HTTP_STREAMING_SSE')
    await flushPromises()

    const html = wrapper.html()
    expect(html).toContain('Resource → Upstream relationships')
    expect(wrapper.get('[data-cy="resource-relationships-details"]').element).not.toHaveProperty('open', true)
    expect(html).toContain('ups_test')
  })

  it('renders an enable toggle for each resource in the relationship view', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    draft.localContent!.providers.push({
      providerId: 'prv_openai', displayName: 'OpenAI', clientProtocol: 'OPENAI_CHAT_COMPLETIONS', enabled: true,
    })
    const modelId = draft.addModel('prv_openai')
    expect(draft.localContent!.models[0].enabled).toBe(true)
    await flushPromises()

    // The Overview relationship view renders an enable toggle per resource row.
    const html = wrapper.html()
    expect(html).toContain('relationship-enable-toggle')
    expect(modelId).toBeDefined()
  })

  it('adds a model only when a real provider exists, never a placeholder', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    await switchTab(wrapper, 'models')
    const addModelBtn = findAddBtn(wrapper, 'Add')
    expect(addModelBtn!.props('disable')).toBe(true)

    // Add a real provider, then the model button becomes usable and binds to it.
    draft.localContent!.providers.push({
      providerId: 'prv_openai',
      displayName: 'OpenAI',
      clientProtocol: 'OPENAI_CHAT_COMPLETIONS',
      enabled: true,
    })
    draft.markDirty()
    await flushPromises()

    const enabledBtn = findAddBtn(wrapper, 'Add')
    await enabledBtn!.trigger('click')
    await flushPromises()

    expect(draft.localContent!.models).toHaveLength(1)
    expect(draft.localContent!.models[0].providerId).toBe('prv_openai')
    expect(draft.localContent!.models[0].providerId).not.toMatch(/placeholder/)
  })

  it('publishes with expectedDraftRevision and acknowledged warning codes', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    const draftWithWarnings = structuredClone(EMPTY_DRAFT)
    draftWithWarnings.content.providers = [{
      providerId: 'prv_openai', displayName: 'OpenAI', clientProtocol: 'OPENAI_CHAT_COMPLETIONS', enabled: true,
    }]
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === '/api/admin/v1/draft') return draftWithWarnings
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      if (path === '/api/admin/v1/draft:validate') {
        return {
          valid: true,
          errors: [],
          warnings: [{ code: 'WARN_UNKNOWN_COST', path: '$', message: 'cost unknown', severity: 'WARNING' }],
        }
      }
      if (path === '/api/admin/v1/draft:preview') {
        return {
          draftRevision: 1,
          projectionHash: 'sha256:preview',
          providers: draftWithWarnings.content.providers,
          models: [],
          tts: [],
          asr: [],
          mcp: [],
          policy: { policyId: 'pol_draft', allowLocalProviders: true, allowLocalTts: true, allowLocalAsr: true, allowLocalMcp: true, allowLocalAssistants: true },
          assistants: [],
          starters: [],
          publishedGeneration: 3,
          diffSummary: { added: 1, changed: 0, removed: 0, details: [{ kind: 'PROVIDER', added: 1, changed: 0, removed: 0 }] },
        }
      }
      if (path === '/api/admin/v1/draft:publish') {
        return { activationId: 'act_001', kind: 'PUBLISH', state: 'COMPLETED', desiredControlRevision: 1, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() }
      }
      if (path.startsWith('/api/admin/v1/activations/')) {
        return { activationId: 'act_001', kind: 'PUBLISH', state: 'COMPLETED', desiredControlRevision: 1, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() }
      }
      return {}
    })

    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    draft.validationResult = {
      valid: true,
      errors: [],
      warnings: [{ code: 'WARN_UNKNOWN_COST', path: '$', message: 'cost unknown', severity: 'WARNING' }],
    }
    await flushPromises()

    // Click "Review & Publish" to open the structured review workspace.
    const reviewBtn = findAddBtn(wrapper, 'Review & Publish')
    expect(reviewBtn).toBeTruthy()
    await reviewBtn!.trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-cy="review-change-count"]').text()).toContain('1')
    expect(wrapper.get('[data-cy="review-resource-table"]').text()).toContain('Provider')
    expect(wrapper.get('[data-cy="review-resource-table"]').text()).not.toContain('Model')
    expect(wrapper.find('[data-cy="review-preview-btn"]').exists()).toBe(true)

    // The review workspace opens; find the actual Publish button inside.
    const publishBtn = wrapper.findAllComponents(QBtn).find(
      (b) => String(b.props('label') ?? '').startsWith('Publish'),
    )
    expect(publishBtn).toBeTruthy()
    await publishBtn!.trigger('click')
    await flushPromises()

    const publishCall = fetchSpy.mock.calls.find((c) => c[0] === '/api/admin/v1/draft:publish')
    expect(publishCall).toBeDefined()
    const body = JSON.parse((publishCall![1] as RequestInit).body as string)
    expect(body.expectedDraftRevision).toBe(1)
    expect(body.acknowledgedWarningCodes).toEqual(['WARN_UNKNOWN_COST'])
  })

  it('explains an unchanged published draft instead of offering another Publish', async () => {
    vi.mocked(client.apiFetch).mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/draft') return structuredClone(EMPTY_DRAFT)
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      if (path === '/api/admin/v1/draft:validate') return { valid: true, errors: [], warnings: [] }
      if (path === '/api/admin/v1/draft:preview') return {
        draftRevision: 1, projectionHash: 'sha256:unchanged', publishedGeneration: 3,
        providers: [], models: [], tts: [], asr: [], mcp: [], assistants: [], starters: [],
        policy: EMPTY_DRAFT.content.policy,
        diffSummary: { added: 0, changed: 0, removed: 0, details: [] },
      }
      return {}
    })
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()
    await wrapper.get('[data-cy="draft-review-btn"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-cy="review-no-changes"]').text()).toContain('already published')
    const publishButton = wrapper.findAllComponents(QBtn).find(button => button.attributes('data-cy') === 'draft-publish-btn')
    expect(publishButton?.props('disable')).toBe(true)
    wrapper.unmount()
  })

  it('can preview snapshot and shows hash and resource counts', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === '/api/admin/v1/draft') return structuredClone(EMPTY_DRAFT)
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      if (path === '/api/admin/v1/draft:preview' && init?.method === 'POST') {
        return {
          draftRevision: 1,
          projectionHash: 'sha256:abc123',
          providers: [{ providerId: 'prv_1', displayName: 'Friendly Provider', clientProtocol: 'OPENAI_CHAT_COMPLETIONS', enabled: true }],
          models: [{ modelId: 'mdl_1', displayName: 'Friendly Model', providerId: 'prv_1', upstreamModelKey: 'model', inputModalities: ['TEXT'], capabilities: ['TOOL'], enabled: false }],
          tts: [{ ttsId: 'tts_1', displayName: 'Friendly Voice', upstreamModelKey: 'speech', voice: 'alloy', enabled: true }],
          asr: [],
          mcp: [{ mcpServerId: 'mcp_1', displayName: 'Tools', authOwnership: 'ENTERPRISE_MANAGED', enabled: true }],
          policy: { policyId: 'pol_draft', allowLocalProviders: true, allowLocalTts: false, allowLocalAsr: true, allowLocalMcp: true, allowLocalAssistants: true, defaultModelId: 'mdl_missing', defaultTtsId: 'tts_1' },
          assistants: [{ assistantDefinitionId: 'asd_1', displayName: 'Field Helper', description: 'Helps field engineers', enabled: true, modelId: 'mdl_1', mcpServerIds: ['mcp_1'], systemPrompt: 'Help safely', memorySeed: ['Check safety'] }],
          starters: [{ starterId: 'str_1', assistantDefinitionId: 'asd_1', title: 'Inspect device', description: 'Start a safety inspection', prompt: 'Please inspect', sortOrder: 0, enabled: true }],
          diffSummary: { added: 0, changed: 0, removed: 0 },
        }
      }
      return {}
    })

    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const btns = wrapper.findAllComponents(QBtn)
    const previewBtn = btns.find((b) => String(b.props('label') ?? '').includes('Snapshot Preview'))
    expect(previewBtn).toBeTruthy()
    await previewBtn!.trigger('click')
    await flushPromises()

    const previewCallAfter = fetchSpy.mock.calls.find(
      (c) => c[0] === '/api/admin/v1/draft:preview' && (c[1] as RequestInit)?.method === 'POST',
    )
    expect(previewCallAfter).toBeDefined()

    const body = wrapper.get('[data-cy="snapshot-preview-surface"]').text()
    expect(body).toContain('sha256:abc123')
    expect(body).toContain('Snapshot Preview')
    expect(body).toContain('Policy ID')
    expect(body).toContain('Assistants, Memory seeds, Starters')
    expect(body).toContain('Assistants (1)')
    expect(body).toContain('Starters (1)')
    const assistantsSection = wrapper.findAllComponents(QExpansionItem).find(item => String(item.props('label')).startsWith('Assistants'))!
    await assistantsSection.trigger('click')
    await flushPromises()
    const assistantSummary = wrapper.get('[data-cy="preview-assistant-summary"]')
    expect(assistantSummary.text()).toContain('Friendly Model')
    expect(assistantSummary.text()).toContain('Tools')
    expect(assistantSummary.text()).not.toContain('mdl_1')
    expect(wrapper.get('[data-cy="preview-assistant-description"]').text()).toContain('Helps field engineers')
    const sections = wrapper.findAllComponents(QExpansionItem)
    await sections.find(item => item.props('label') === 'Policy')!.trigger('click')
    await flushPromises()
    const policy = wrapper.get('[data-cy="preview-policy-summary"]')
    expect(policy.text()).toContain('Allowed')
    expect(policy.text()).toContain('Not allowed')
    expect(policy.text()).toContain('Friendly Voice')
    expect(policy.text()).toContain('Not selected')
    expect(policy.text()).toContain('Unavailable resource: mdl_missing')
    expect(policy.text()).not.toMatch(/true|false|pol_draft|tts_1/)
    await sections.find(item => String(item.props('label')).startsWith('Models ('))!.trigger('click')
    await flushPromises()
    const model = wrapper.get('[data-cy="preview-model-summary"]')
    expect(model.text()).toContain('Friendly Provider')
    expect(model.text()).toContain('Disabled')
    await sections.find(item => String(item.props('label')).startsWith('Starters ('))!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-cy="preview-starter-description"]').text()).toContain('Start a safety inspection')
    wrapper.unmount()
  })
})
