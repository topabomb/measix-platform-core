<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch, createCandidateId } from '../api/client'
import { useDraftStore, ttsTransport, asrTransport, isRealtimeAsr, type AsrProtocol, type ImageGenerationProtocol, type TtsProtocol, type ManagedResourceKind } from '../stores/draft'
import { useSessionStore } from '../stores/session'
import { useActivationStore } from '../stores/activation'
import ManagedExperienceEditor from '../components/ManagedExperienceEditor.vue'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import ConfigurationSectionNav, { type ConfigurationSection } from '../components/ConfigurationSectionNav.vue'
import { useResourceDiff } from '../composables/useResourceDiff'
import PagedEntityPicker from '../components/PagedEntityPicker.vue'
import { fetchUpstreamPickerPage, resolveUpstreamPickerOption } from '../api/entityPickerSources'

type Activation = components['schemas']['Activation']
type ModelDefinition = components['schemas']['ModelDefinition']
type TtsDefinition = components['schemas']['TtsDefinition']
type AsrDefinition = components['schemas']['AsrDefinition']
type McpDefinition = components['schemas']['McpDefinition']
type ProviderDefinition = components['schemas']['ProviderDefinition']
type ManagedPolicy = components['schemas']['ManagedPolicy']
type PolicyFlagKey = 'allowLocalProviders' | 'allowLocalTts' | 'allowLocalAsr' | 'allowLocalMcp' | 'allowLocalAssistants'
type PolicyDefaultKey = 'defaultModelId' | 'defaultImageGenerationId' | 'defaultTtsId' | 'defaultAsrId' | 'defaultAssistantId'
type Upstream = components['schemas']['Upstream']
type UpstreamPage = components['schemas']['UpstreamPage']
type RuntimeBindingDefinition = components['schemas']['RuntimeBindingDefinition']
type TransportPolicy = RuntimeBindingDefinition['transportPolicy']
type DraftPreviewResponse = components['schemas']['DraftPreviewResponse']
type ReleaseDiffKind = components['schemas']['ReleaseDiffKind']
type ResourceDiff = components['schemas']['ResourceDiff']
type ValidationIssue = components['schemas']['ValidationIssue']

const { t: $t } = useI18n()
const draft = useDraftStore()
const session = useSessionStore()
const activation = useActivationStore()
const { reviewWarnings, hasBlockingErrors, validationIssuesFor } = useResourceDiff(draft)
const error = ref<unknown>()
const publishing = ref(false)
const previewing = ref(false)
const preview = ref<DraftPreviewResponse>()
const previewOpen = ref(false)
const reviewOpen = ref(false)
const reviewing = ref(false)
const upstreams = ref<Upstream[]>([])
const upstreamsLoading = ref(false)
const upstreamError = ref<unknown>()
const activeTab = ref<'overview' | 'models' | 'image-generation' | 'tts' | 'asr' | 'mcp' | 'assistants' | 'policy'>('overview')
const canMutate = computed(() => Boolean(session.csrfToken))
const reviewTotalChanges = computed(() => {
  const summary = preview.value?.diffSummary
  return summary ? summary.added + summary.changed + summary.removed : 0
})
const unchangedPublishedDraft = computed(() =>
  reviewTotalChanges.value === 0 && (preview.value?.publishedGeneration ?? 0) > 0,
)
function previewModelName(id: string | undefined): string {
  return previewReferenceName('model', id)
}
function previewReferenceName(kind: 'model' | 'image' | 'tts' | 'asr' | 'provider' | 'assistant', id: string | undefined): string {
  if (!id) return $t('resources.preview.notSelected')
  const snapshot = preview.value
  const resource = kind === 'model' ? snapshot?.models.find(item => item.modelId === id)
    : kind === 'image' ? snapshot?.imageGenerators?.find(item => item.imageId === id)
      : kind === 'tts' ? snapshot?.tts.find(item => item.ttsId === id)
      : kind === 'asr' ? snapshot?.asr.find(item => item.asrId === id)
        : kind === 'assistant' ? snapshot?.assistants.find(item => item.assistantDefinitionId === id)
          : snapshot?.providers.find(item => item.providerId === id)
  return resource?.displayName ?? $t('resources.preview.unavailableResource', { id })
}
function previewMcpNames(ids: string[]): string {
  return ids.map(id => preview.value?.mcp.find(server => server.mcpServerId === id)?.displayName ?? $t('common.unknown')).join(', ') || $t('common.none')
}
function previewAssistantName(id: string): string {
  return preview.value?.assistants.find(assistant => assistant.assistantDefinitionId === id)?.displayName ?? $t('common.unknown')
}
function reviewDiffFor(kind: ReleaseDiffKind): ResourceDiff {
  return preview.value?.diffSummary.details?.find(detail => detail.kind === kind)
    ?? { kind, added: 0, changed: 0, removed: 0 }
}
const routingImpact = computed(() => reviewDiffFor('BINDING'))
const routingChanged = computed(() => {
  const impact = routingImpact.value
  return impact.added + impact.changed + impact.removed > 0
})
const reviewResourceRows = computed(() => [
  { kind: $t('resources.preview.providers'), diff: reviewDiffFor('PROVIDER') },
  { kind: $t('resources.tabs.models'), diff: reviewDiffFor('MODEL') },
  { kind: $t('resources.tabs.imageGeneration'), diff: reviewDiffFor('IMAGE_GENERATION') },
  { kind: 'TTS', diff: reviewDiffFor('TTS') },
  { kind: 'ASR', diff: reviewDiffFor('ASR') },
  { kind: 'MCP', diff: reviewDiffFor('MCP') },
  { kind: $t('experience.tab'), diff: reviewDiffFor('ASSISTANT') },
  { kind: $t('experience.starters'), diff: reviewDiffFor('STARTER') },
].filter(row => row.diff.added + row.diff.changed + row.diff.removed > 0))
const policyChanged = computed(() => {
  const value = reviewDiffFor('POLICY')
  return value.added + value.changed + value.removed > 0
})

const configurationSections = computed<ConfigurationSection[]>(() => {
  const content = draft.localContent
  const totalResources = (content?.models.length ?? 0) + (content?.imageGenerators?.length ?? 0) + (content?.tts.length ?? 0)
    + (content?.asr.length ?? 0) + (content?.mcp.length ?? 0)
  const enabledLocal = content?.policy
    ? [content.policy.allowLocalProviders, content.policy.allowLocalTts, content.policy.allowLocalAsr,
      content.policy.allowLocalMcp, content.policy.allowLocalAssistants].filter(Boolean).length
    : 0
  return [
    { id: 'overview', label: $t('resources.navigation.overview'), description: $t('resources.navigation.overviewHint'), icon: 'account_tree', badge: totalResources },
    { id: 'models', label: $t('resources.tabs.models'), description: $t('resources.navigation.modelsHint'), icon: 'smart_toy', badge: content?.models.length ?? 0 },
    { id: 'image-generation', label: $t('resources.tabs.imageGeneration'), description: $t('resources.navigation.imageGenerationHint'), icon: 'image', badge: content?.imageGenerators?.length ?? 0 },
    { id: 'tts', label: $t('resources.tabs.tts'), description: $t('resources.navigation.ttsHint'), icon: 'record_voice_over', badge: content?.tts.length ?? 0 },
    { id: 'asr', label: $t('resources.tabs.asr'), description: $t('resources.navigation.asrHint'), icon: 'hearing', badge: content?.asr.length ?? 0 },
    { id: 'mcp', label: $t('resources.tabs.mcp'), description: $t('resources.navigation.mcpHint'), icon: 'hub', badge: content?.mcp.length ?? 0 },
    { id: 'assistants', label: $t('experience.tab'), description: $t('resources.navigation.assistantsHint'), icon: 'assistant', badge: content?.assistants.length ?? 0 },
    { id: 'policy', label: $t('resources.tabs.policy'), description: $t('resources.navigation.policyHint'), icon: 'policy', badge: $t('resources.navigation.localAllowedCount', { count: enabledLocal }) },
  ]
})
const policySettings = computed((): { key: PolicyFlagKey; label: string; hint: string; cy: string }[] => [
  { key: 'allowLocalProviders', label: $t('resources.policy.allowLocalModels'), hint: $t('resources.policy.allowLocalModelsHint'), cy: 'policy-allow-local-models' },
  { key: 'allowLocalTts', label: $t('resources.policy.allowLocalTts'), hint: $t('resources.policy.allowLocalTtsHint'), cy: 'policy-allow-local-tts' },
  { key: 'allowLocalAsr', label: $t('resources.policy.allowLocalAsr'), hint: $t('resources.policy.allowLocalAsrHint'), cy: 'policy-allow-local-asr' },
  { key: 'allowLocalMcp', label: $t('resources.policy.allowLocalMcp'), hint: $t('resources.policy.allowLocalMcpHint'), cy: 'policy-allow-local-mcp' },
  { key: 'allowLocalAssistants', label: $t('resources.policy.allowLocalAssistants'), hint: $t('resources.policy.allowLocalAssistantsHint'), cy: 'policy-allow-local-assistants' },
])

function setPolicyFlag(key: PolicyFlagKey, value: boolean) {
  if (!draft.localContent) return
  draft.localContent.policy[key] = value
  draft.markDirty()
}

// Selected resource for editor/detail mode
const selectedResourceId = ref<string>()
const collectionQuery = ref('')

function matchesCollection(value: string, id: string): boolean {
  const term = collectionQuery.value.trim().toLocaleLowerCase()
  return !term || value.toLocaleLowerCase().includes(term) || id.toLocaleLowerCase().includes(term)
}

const filteredModels = computed(() => draft.localContent?.models.filter(item => matchesCollection(item.displayName, item.modelId)) ?? [])
const filteredImageGenerators = computed(() => draft.localContent?.imageGenerators?.filter(item => matchesCollection(item.displayName, item.imageId)) ?? [])
const filteredTts = computed(() => draft.localContent?.tts.filter(item => matchesCollection(item.displayName, item.ttsId)) ?? [])
const filteredAsr = computed(() => draft.localContent?.asr.filter(item => matchesCollection(item.displayName, item.asrId)) ?? [])
const filteredMcp = computed(() => draft.localContent?.mcp.filter(item => matchesCollection(item.displayName, item.mcpServerId)) ?? [])

const INPUT_MODS = ['TEXT', 'IMAGE'] as const
const OUTPUT_MODS = ['TEXT'] as const
const MODEL_CAPS = ['TOOL', 'REASONING'] as const
const AUTH_OWNERSHIPS = ['ENTERPRISE_MANAGED', 'NONE'] as const
const IMAGE_SIZE_OPTIONS = ['auto', '256x256', '512x512', '1024x1024', '1024x1536', '1536x1024']
const RELATIONSHIP_KINDS = ['Model', 'Image Generation', 'TTS', 'ASR', 'MCP'] as const
const relationshipKindFilter = ref<string>('all')

const up = (id?: string) => upstreams.value.find((u) => u.upstreamId === id)
const upstreamLabel = (id?: string) => (id ? up(id)?.name ?? id : '—')
const upstreamStatus = (id?: string) => up(id)?.status

/** Selected model for the Models tab editor. */
const selectedModel = computed(() =>
  draft.localContent?.models.find((m) => m.modelId === selectedResourceId.value),
)
const selectedImageGeneration = computed(() =>
  draft.localContent?.imageGenerators?.find(item => item.imageId === selectedResourceId.value),
)
const imageGenerationProtocols = computed<{ label: string; value: ImageGenerationProtocol }[]>(() => [
  { label: $t('resources.imageGeneration.openAiProtocol'), value: 'OPENAI_IMAGES_GENERATIONS' },
  { label: $t('resources.imageGeneration.dashScopeProtocol'), value: 'DASHSCOPE_MULTIMODAL_GENERATION' },
])
const imageSizeOptions = computed(() => selectedImageGeneration.value?.clientProtocol === 'DASHSCOPE_MULTIMODAL_GENERATION'
  ? IMAGE_SIZE_OPTIONS.filter(size => size !== 'auto')
  : IMAGE_SIZE_OPTIONS)
const imageGenerationTransportHint = computed(() => selectedImageGeneration.value?.clientProtocol === 'DASHSCOPE_MULTIMODAL_GENERATION'
  ? $t('resources.imageGeneration.dashScopeTransport')
  : $t('resources.imageGeneration.openAiTransport'))
const selectedTts = computed(() =>
  draft.localContent?.tts.find((t) => t.ttsId === selectedResourceId.value),
)
const modelProtocols = [
  { label: 'OpenAI Chat Completions', value: 'OPENAI_CHAT_COMPLETIONS' },
  { label: 'OpenAI Responses', value: 'OPENAI_RESPONSES' },
  { label: 'Google Gemini', value: 'GOOGLE_GENERATE_CONTENT' },
  { label: 'Anthropic Claude', value: 'ANTHROPIC_MESSAGES' },
]
const asrProtocols = computed(() => [
  { label: $t('resources.asr.httpService'), value: 'OPENAI_AUDIO_TRANSCRIPTIONS' },
  { label: $t('resources.asr.dashscopeHttpService'), value: 'DASHSCOPE_HTTP_ASR' },
  { label: 'OpenAI Realtime', value: 'OPENAI_REALTIME_TRANSCRIPTION' },
  { label: 'DashScope Realtime', value: 'DASHSCOPE_REALTIME_ASR' },
])
const ttsProtocols = computed(() => [
  { label: 'OpenAI Speech', value: 'OPENAI_AUDIO_SPEECH' },
  { label: 'Gemini TTS', value: 'GEMINI_GENERATE_CONTENT_TTS' },
  { label: 'MiMo TTS', value: 'MIMO_CHAT_COMPLETIONS_TTS' },
  { label: $t('resources.tts.system'), value: 'SYSTEM_TTS' },
])
function isVoiceDesign(tts: TtsDefinition): boolean {
  return tts.clientProtocol === 'MIMO_CHAT_COMPLETIONS_TTS' && !!tts.upstreamModelKey?.toLowerCase().includes('voicedesign')
}
function ttsSummary(tts: TtsDefinition): string {
  if (tts.clientProtocol === 'SYSTEM_TTS') return $t('resources.tts.systemSummary', { rate: tts.speechRate, pitch: tts.pitch })
  if (isVoiceDesign(tts)) return tts.voiceDesignPrompt || $t('resources.tts.designPromptRequired')
  return tts.voice || $t('resources.tts.noVoice')
}
function updateTtsModel() {
  if (selectedTts.value && isVoiceDesign(selectedTts.value)) delete selectedTts.value.voice
  draft.markDirty()
}
function updateVoiceDesignPrompt(value: string | number | null) {
  if (!selectedTts.value) return
  if (value) selectedTts.value.voiceDesignPrompt = String(value)
  else delete selectedTts.value.voiceDesignPrompt
  draft.markDirty()
}
const selectedAsr = computed(() =>
  draft.localContent?.asr.find((a) => a.asrId === selectedResourceId.value),
)
const selectedMcp = computed(() =>
  draft.localContent?.mcp.find((m) => m.mcpServerId === selectedResourceId.value),
)

/** Resource → upstream relationship projection for the Overview tab. */
const relationshipRows = computed(() => {
  const rows: { resourceId: string; kind: string; displayName: string; upstreamId?: string; upstreamName: string; upstreamStatus?: string; enabled: boolean; runtimePath?: string; transport?: string; bindingState: string }[] = []
  const c = draft.localContent
  if (!c) return rows
  for (const m of c.models) {
    const b = draft.bindingFor(m.modelId)
    rows.push({
      resourceId: m.modelId, kind: 'Model', displayName: m.displayName,
      upstreamId: b?.upstreamId, upstreamName: upstreamLabel(b?.upstreamId),
      upstreamStatus: upstreamStatus(b?.upstreamId),
      enabled: m.enabled, runtimePath: m.runtimePath,
      transport: b?.transportPolicy,
      bindingState: !b ? 'missing' : !b.upstreamId ? 'missing' : 'bound',
    })
  }
  for (const image of c.imageGenerators ?? []) {
    const binding = draft.bindingFor(image.imageId)
    rows.push({
      resourceId: image.imageId, kind: 'Image Generation', displayName: image.displayName,
      upstreamId: binding?.upstreamId, upstreamName: upstreamLabel(binding?.upstreamId),
      upstreamStatus: upstreamStatus(binding?.upstreamId), enabled: image.enabled,
      runtimePath: image.runtimePath, transport: binding?.transportPolicy,
      bindingState: !binding?.upstreamId ? 'missing' : 'bound',
    })
  }
  for (const t of c.tts) {
    const b = draft.bindingFor(t.ttsId)
    const local = t.clientProtocol === 'SYSTEM_TTS'
    rows.push({
      resourceId: t.ttsId, kind: 'TTS', displayName: t.displayName,
      upstreamId: b?.upstreamId, upstreamName: local ? $t('resources.tts.system') : upstreamLabel(b?.upstreamId),
      upstreamStatus: upstreamStatus(b?.upstreamId),
      enabled: t.enabled, runtimePath: t.runtimePath,
      transport: b?.transportPolicy,
      bindingState: local ? 'device' : !b ? 'missing' : !b.upstreamId ? 'missing' : 'bound',
    })
  }
  for (const a of c.asr) {
    const b = draft.bindingFor(a.asrId)
    rows.push({
      resourceId: a.asrId, kind: 'ASR', displayName: a.displayName,
      upstreamId: b?.upstreamId, upstreamName: upstreamLabel(b?.upstreamId),
      upstreamStatus: upstreamStatus(b?.upstreamId),
      enabled: a.enabled, runtimePath: a.runtimePath,
      transport: b?.transportPolicy,
      bindingState: !b ? 'missing' : !b.upstreamId ? 'missing' : 'bound',
    })
  }
  for (const m of c.mcp) {
    const b = draft.bindingFor(m.mcpServerId)
    rows.push({
      resourceId: m.mcpServerId, kind: 'MCP', displayName: m.displayName,
      upstreamId: b?.upstreamId, upstreamName: upstreamLabel(b?.upstreamId),
      upstreamStatus: upstreamStatus(b?.upstreamId),
      enabled: m.enabled, runtimePath: m.runtimePath,
      transport: b?.transportPolicy,
      bindingState: !b ? 'missing' : !b.upstreamId ? 'missing' : 'bound',
    })
  }
  return rows
})

const filteredRelationshipRows = computed(() => {
  if (relationshipKindFilter.value === 'all') return relationshipRows.value
  return relationshipRows.value.filter((r) => r.kind === relationshipKindFilter.value)
})

/** Enabled models for Policy default picker. */
const enabledModels = computed(() =>
  draft.localContent?.models.filter((m) => m.enabled).map((m) => ({
    label: m.displayName,
    value: m.modelId,
  })) ?? [],
)
const enabledImageGenerators = computed(() =>
  draft.localContent?.imageGenerators?.filter(item => item.enabled).map(item => ({
    label: item.displayName,
    value: item.imageId,
  })) ?? [],
)

function setPolicyDefault(key: PolicyDefaultKey, value: string | null) {
  if (!draft.localContent) return
  if (value) draft.localContent.policy[key] = value
  else delete draft.localContent.policy[key]
  draft.markDirty()
}
const enabledTts = computed(() =>
  draft.localContent?.tts.filter((t) => t.enabled).map((t) => ({
    label: t.displayName,
    value: t.ttsId,
  })) ?? [],
)
const enabledAsr = computed(() =>
  draft.localContent?.asr.filter((a) => a.enabled).map((a) => ({
    label: a.displayName,
    value: a.asrId,
  })) ?? [],
)
const enabledAssistants = computed(() =>
  draft.localContent?.assistants.filter((a) => a.enabled).map((a) => ({
    label: a.displayName,
    value: a.assistantDefinitionId,
  })) ?? [],
)

function confirmDiscard(): boolean {
  return !draft.dirty || window.confirm($t('resources.discardChanges'))
}

async function refresh(force = false) {
  if (!force && !confirmDiscard()) return
  error.value = undefined
  try {
    await draft.load()
    await loadUpstreams()
  } catch (cause) {
    error.value = cause
  }
}

async function loadUpstreams() {
  upstreamsLoading.value = true
  upstreamError.value = undefined
  try {
    const page = await apiFetch<UpstreamPage>('/api/admin/v1/upstreams?limit=50')
    const known = new Map(page.items.map(item => [item.upstreamId, item]))
    const referenced = new Set(draft.localContent?.bindings.map(binding => binding.upstreamId).filter(Boolean) ?? [])
    await Promise.all([...referenced].filter(id => !known.has(id)).map(async id => {
      known.set(id, await apiFetch<Upstream>(`/api/admin/v1/upstreams/${encodeURIComponent(id)}`))
    }))
    upstreams.value = [...known.values()]
  } catch (cause) {
    upstreamError.value = cause
  } finally {
    upstreamsLoading.value = false
  }
}

async function save() {
  if (!session.csrfToken) return
  error.value = undefined
  try { await draft.save(session.csrfToken) } catch (cause) { error.value = cause }
}

async function validate() {
  if (!session.csrfToken) return
  error.value = undefined
  try { await draft.validate(session.csrfToken) } catch (cause) { error.value = cause }
}

async function openReview() {
  if (!session.csrfToken) return
  if (draft.baselineRevision === undefined) return
  reviewing.value = true
  error.value = undefined
  try {
    // Re-validate before opening the review dialog to ensure
    // acknowledgedWarningCodes includes all current warnings.
    // The backend re-runs validation on publish and rejects if
    // any warning code is not in acknowledgedWarningCodes.
    await draft.validate(session.csrfToken)
    // Ensure we have a fresh preview before the review dialog
    if (!preview.value || preview.value.draftRevision !== draft.baselineRevision) {
      preview.value = await apiFetch<DraftPreviewResponse>(
        '/api/admin/v1/draft:preview',
        { method: 'POST', body: JSON.stringify({ expectedDraftRevision: draft.baselineRevision }) },
        session.csrfToken,
      )
    }
    reviewOpen.value = true
  } catch (cause) {
    error.value = cause
  } finally {
    reviewing.value = false
  }
}

async function publish() {
  if (!session.csrfToken) return
  if (draft.baselineRevision === undefined) return
  const warnings = draft.validationResult?.warnings ?? []
  publishing.value = true
  const payload = JSON.stringify({
    expectedDraftRevision: draft.baselineRevision,
    acknowledgedWarningCodes: warnings.map((w) => w.code).sort(),
  })
  const key = activation.beginCommand('PUBLISH', 'draft:' + payload)
  error.value = undefined
  try {
    const result = await apiFetch<Activation>('/api/admin/v1/draft:publish', {
      method: 'POST',
      headers: { 'Idempotency-Key': key },
      body: payload,
    }, session.csrfToken)
    activation.accept(result)
    if (result.state === 'APPLYING' || result.state === 'UNKNOWN') {
      await activation.pollUntilSettled(result.activationId, { timeoutMs: 60_000 })
    }
    reviewOpen.value = false
    await refresh(true)
  } catch (cause) {
    error.value = cause
  } finally {
    publishing.value = false
  }
}

async function previewSnapshot() {
  if (!session.csrfToken) return
  if (draft.baselineRevision === undefined) return
  previewing.value = true
  error.value = undefined
  try {
    preview.value = await apiFetch<DraftPreviewResponse>(
      '/api/admin/v1/draft:preview',
      { method: 'POST', body: JSON.stringify({ expectedDraftRevision: draft.baselineRevision }) },
      session.csrfToken,
    )
    previewOpen.value = true
  } catch (cause) {
    error.value = cause
  } finally {
    previewing.value = false
  }
}

function modelCountForProvider(providerId: string): number {
  return draft.localContent?.models.filter((m) => m.providerId === providerId).length ?? 0
}

function addModel() {
  const content = draft.localContent
  if (!content) return
  if (!content.providers.length) {
    error.value = new Error($t('resources.addProviderBeforeModels'))
    return
  }
  const id = draft.addModel(content.providers[0].providerId)
  selectedResourceId.value = id
  activeTab.value = 'models'
}

function addProvider() {
  const content = draft.localContent
  if (!content) return
  const providerId = createCandidateId('prv')
  content.providers.push({
    providerId,
    displayName: $t('resources.newProvider'),
    clientProtocol: 'OPENAI_CHAT_COMPLETIONS',
    enabled: true,
  })
  draft.markDirty()
  return providerId
}

function removeProvider(providerId: string) {
  const content = draft.localContent
  if (!content) return
  const referenced = content.models.some((m) => m.providerId === providerId)
  if (referenced) {
    error.value = new Error($t('resources.providerReferenced', { id: providerId }))
    return
  }
  const provider = content.providers.find(item => item.providerId === providerId)
  if (!window.confirm($t('resources.removeConfirm', { name: provider?.displayName ?? providerId }))) return
  content.providers = content.providers.filter((p) => p.providerId !== providerId)
  draft.markDirty()
}

function removeResource(kind: ManagedResourceKind, resourceId: string, displayName: string) {
  if (!window.confirm($t('resources.removeConfirm', { name: displayName }))) return
  const result = draft.removeResource(kind, resourceId)
  if (!result.removed) {
    error.value = new Error($t('resources.resourceReferenced', { references: result.references.join(', ') }))
    return
  }
  if (selectedResourceId.value === resourceId) selectedResourceId.value = undefined
}

/** Toggle the enabled flag of a resource row. */
function toggleEnabled(kind: string, resourceId: string, enabled: boolean) {
  const content = draft.localContent
  if (!content) return
  const item = kind === 'Model' ? content.models.find(value => value.modelId === resourceId)
    : kind === 'Image Generation' ? content.imageGenerators?.find(value => value.imageId === resourceId)
      : kind === 'TTS' ? content.tts.find(value => value.ttsId === resourceId)
        : kind === 'ASR' ? content.asr.find(value => value.asrId === resourceId)
          : content.mcp.find(value => value.mcpServerId === resourceId)
  if (item) {
    item.enabled = enabled
    draft.markDirty()
  }
}

/** Navigate to the editor for a specific resource. */
function goToResource(kind: string, resourceId: string) {
  selectedResourceId.value = resourceId
  const tab = ({ Model: 'models', 'Image Generation': 'image-generation', TTS: 'tts', ASR: 'asr', MCP: 'mcp' } as Record<string, string>)[kind]
  if (tab) activeTab.value = tab as typeof activeTab.value
}

/** Select a model in the editor. */
function selectModel(id: string) { selectedResourceId.value = id }
function selectImageGeneration(id: string) { selectedResourceId.value = id }
function selectTts(id: string) { selectedResourceId.value = id }
function selectAsr(id: string) { selectedResourceId.value = id }
function selectMcp(id: string) { selectedResourceId.value = id }

function beforeUnload(event: BeforeUnloadEvent) {
  if (!draft.dirty) return
  event.preventDefault()
  event.returnValue = ''
}

async function goToValidationIssue(issue: ValidationIssue) {
  const section = issue.resourceKind === 'MODEL' ? 'models'
    : issue.resourceKind === 'IMAGE_GENERATION' ? 'image-generation'
      : issue.resourceKind === 'TTS' ? 'tts'
        : issue.resourceKind === 'ASR' ? 'asr'
          : issue.resourceKind === 'MCP' ? 'mcp'
            : issue.resourceKind === 'ASSISTANT' || issue.resourceKind === 'STARTER' ? 'assistants'
              : issue.resourceKind === 'POLICY' ? 'policy'
                : issue.resourceKind === 'PROVIDER' ? 'overview' : undefined
  if (!section) return
  activeTab.value = section
  if (issue.resourceId && ['MODEL', 'IMAGE_GENERATION', 'TTS', 'ASR', 'MCP'].includes(issue.resourceKind ?? '')) {
    selectedResourceId.value = issue.resourceId
  }
  await nextTick()
  if (issue.field) {
    const element = document.querySelector<HTMLElement>(`[data-field="${issue.field}"]`)
    element?.scrollIntoView({ block: 'center' })
    element?.querySelector<HTMLElement>('input, textarea, [tabindex]')?.focus()
  }
}

onBeforeRouteLeave(() => confirmDiscard())
onMounted(() => {
  window.addEventListener('beforeunload', beforeUnload)
  void refresh(true)
})
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
</script>

<template>
  <q-page class="admin-page" data-cy="resources-page">
    <PageHeader :title="$t('nav.resources')" :subtitle="$t('resources.subtitle')">
      <template #actions>
        <q-btn flat icon="refresh" :aria-label="$t('common.refresh')" :loading="draft.loading" @click="refresh()" />
        <q-btn outline color="primary" :label="$t('common.save')" :disable="!canMutate || !draft.dirty" :loading="draft.saving" @click="save" data-cy="draft-save-btn" />
        <q-btn outline color="primary" :label="$t('resources.draft.validate')" :disable="!canMutate || draft.loading || draft.dirty" @click="validate" data-cy="draft-validate-btn" />
        <q-btn outline color="secondary" :label="$t('resources.draft.preview')" :disable="!canMutate || draft.dirty" :loading="previewing" @click="previewSnapshot" data-cy="draft-preview-btn" />
        <q-btn color="positive" icon="rocket_launch" :label="$t('resources.draft.review')" :disable="!canMutate || draft.dirty" :loading="reviewing" @click="openReview" data-cy="draft-review-btn" />
      </template>
    </PageHeader>

    <ProblemBanner :error="error" class="q-mb-xs" />
    <q-banner v-if="upstreamError" class="bg-orange-1 q-mb-xs rounded-borders" data-cy="upstream-load-error">
      <div class="text-weight-medium">{{ $t('resources.upstreamLoadFailed') }}</div>
      <div class="text-body2">{{ $t('resources.upstreamLoadFailedHint') }}</div>
      <template #action><q-btn flat :label="$t('common.retry')" :loading="upstreamsLoading" @click="loadUpstreams" /></template>
    </q-banner>
    <q-banner v-if="draft.conflictRevision !== undefined" class="bg-orange-1 q-mb-xs rounded-borders">
      <div class="text-weight-medium">{{ $t('resources.staleDraft', { rev: draft.baselineRevision }) }}</div>
      <div class="text-body2">{{ $t('resources.staleHint', { rev: draft.conflictRevision }) }}</div>
      <template #action><q-btn flat :label="$t('resources.reload')" @click="refresh()" /></template>
    </q-banner>
    <q-banner v-if="activation.activation" :class="activation.succeeded ? 'bg-green-1' : 'bg-orange-1'" class="q-mb-xs rounded-borders">
      <div class="row items-center justify-between">
        <span>{{ $t('resources.draft.latestOperation') }}</span>
        <StatusChip :value="activation.activation.state" />
      </div>
      <details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ activation.activation.activationId }} · {{ activation.activation.kind }}</details>
      <div v-if="activation.activation.errorCode" class="text-caption text-negative">{{ activation.activation.errorCode }}</div>
      <div v-if="activation.pending" class="text-caption text-grey-7 q-mt-xs">{{ $t('upstreams.activationRecoveryHint') }}</div>
    </q-banner>

    <LoadingState v-if="draft.loading" />
    <template v-else-if="draft.localContent">
      <!-- Draft identity + status -->
      <q-card flat bordered class="q-mb-xs">
        <q-card-section class="row items-center justify-between">
          <div class="text-subtitle1">
            {{ $t('resources.draft.revision') }} {{ draft.baselineRevision }}
            <q-badge v-if="draft.dirty" color="orange" :label="$t('common.dirty').toLowerCase()" class="q-ml-xs" />
          </div>
          <div class="row items-center q-gutter-xs">
            <q-chip v-if="draft.validationResult?.errors.length" color="negative" outline dense icon="error">
              {{ draft.validationResult.errors.length }} {{ $t('common.error') }}(s)
            </q-chip>
            <q-chip v-else-if="draft.validationResult?.warnings.length" color="amber" outline dense icon="warning">
              {{ draft.validationResult.warnings.length }} {{ $t('common.warning') }}(s)
            </q-chip>
            <q-chip v-else-if="draft.validationResult" color="positive" outline dense icon="check_circle">
              {{ $t('resources.valid').toLowerCase() }}
            </q-chip>
          </div>
        </q-card-section>
      </q-card>

      <div v-if="!reviewOpen && !previewOpen" class="configuration-workbench">
        <ConfigurationSectionNav
          v-model="activeTab"
          :title="$t('resources.navigation.title')"
          :subtitle="$t('resources.navigation.subtitle')"
          :items="configurationSections"
        />
        <section class="configuration-workbench__detail">

      <!-- ===== Overview: relationship view + providers ===== -->
      <template v-if="activeTab === 'overview'">
        <!-- Providers section -->
        <q-card flat bordered class="q-mb-xs">
          <q-card-section>
            <div class="row items-center justify-between">
              <div class="text-subtitle2">{{ $t('resources.overview.providers') }}</div>
                <q-btn flat dense icon="add" :label="$t('resources.overview.addProvider')" size="sm" data-cy="add-provider-btn" @click="addProvider()" />
            </div>
            <q-list dense class="q-mt-xs">
              <q-item v-for="provider in draft.localContent.providers" :key="provider.providerId">
                <q-item-section>
                  <q-input v-model="provider.displayName" dense outlined :label="$t('users.displayName')" @update:model-value="draft.markDirty()" />
                  <q-select v-model="provider.clientProtocol" dense outlined class="q-mt-xs" :label="$t('resources.model.protocolLabel')" :hint="$t('resources.model.protocolHint')" :options="modelProtocols" emit-value map-options data-cy="provider-protocol" @update:model-value="draft.markDirty()" />
                  <div class="text-caption text-grey-7">{{ $t('resources.overview.modelCount', { count: modelCountForProvider(provider.providerId) }) }}</div>
                  <details class="text-caption text-grey-7"><summary class="cursor-pointer">{{ $t('resources.review.technicalDetails') }}</summary>{{ provider.providerId }} · {{ provider.clientProtocol }}</details>
                </q-item-section>
                <q-item-section side>
                  <q-toggle :label="$t('common.enabled')" v-model="provider.enabled" @update:model-value="draft.markDirty()" />
                  <q-btn flat dense color="negative" icon="delete" size="sm" :aria-label="$t('resources.removeNamed', { name: provider.displayName })" :disable="modelCountForProvider(provider.providerId) > 0" @click="removeProvider(provider.providerId)" />
                </q-item-section>
              </q-item>
              <q-item v-if="!draft.localContent.providers.length"><q-item-section class="text-grey-7">{{ $t('resources.overview.noProviders') }}</q-item-section></q-item>
            </q-list>
          </q-card-section>
        </q-card>

        <!-- Relationship view with kind filter -->
        <details data-cy="resource-relationships-details" class="resource-relationships q-mb-xs">
          <summary class="cursor-pointer q-pa-sm text-subtitle2">{{ $t('resources.overview.relationships') }} ({{ relationshipRows.length }})</summary>
        <q-card flat bordered>
          <q-card-section>
            <div class="row items-center justify-between q-mb-xs">
              <q-btn-toggle
                v-model="relationshipKindFilter"
                dense flat
                :options="[
                  { label: $t('common.all'), value: 'all' },
                  { label: $t('resources.tabs.models'), value: 'Model' },
                  { label: $t('resources.tabs.tts'), value: 'TTS' },
                  { label: $t('resources.tabs.asr'), value: 'ASR' },
                  { label: $t('resources.tabs.mcp'), value: 'MCP' },
                ]"
              />
            </div>
            <q-markup-table flat dense>
              <thead>
                <tr>
                  <th>{{ $t('resources.overview.kind') }}</th>
                  <th>{{ $t('resources.overview.resource') }}</th>
                  <th>{{ $t('resources.overview.upstream') }}</th>
                  <th>{{ $t('resources.overview.runtimePath') }}</th>
                  <th>{{ $t('resources.overview.transport') }}</th>
                  <th class="text-right">{{ $t('common.status') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in filteredRelationshipRows" :key="row.resourceId" clickable @click="goToResource(row.kind, row.resourceId)" style="cursor: pointer">
                  <td><q-chip dense :color="row.kind === 'Model' ? 'primary' : row.kind === 'TTS' ? 'teal' : row.kind === 'ASR' ? 'indigo' : 'deep-purple'" text-color="white">{{ row.kind }}</q-chip></td>
                  <td>
                    <div>{{ row.displayName }}</div>
                    <div class="text-caption text-grey-7">{{ row.resourceId }}</div>
                  </td>
                  <td>
                    <template v-if="row.upstreamId">
                      <div>{{ row.upstreamName }}</div>
                      <div class="text-caption text-grey-7">{{ row.upstreamId }}</div>
                      <q-badge v-if="row.upstreamStatus" :color="row.upstreamStatus === 'ACTIVE' ? 'green' : row.upstreamStatus === 'DEGRADED' ? 'orange' : 'grey'" :label="row.upstreamStatus.toLowerCase()" class="q-mt-xs" />
                    </template>
                    <template v-else>
                      <q-badge color="red" :label="$t('resources.overview.missingBinding')" />
                      <div class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.overview.clickToBind') }}</div>
                    </template>
                  </td>
                  <td><code class="text-caption">{{ row.runtimePath || '—' }}</code></td>
                  <td><span class="text-caption">{{ row.transport || '—' }}</span></td>
                  <td class="text-right">
                    <div class="row items-center justify-end q-gutter-xs">
                      <q-badge v-if="!row.enabled" color="grey" :label="$t('common.disabled').toLowerCase()" />
                      <q-badge v-else-if="row.bindingState === 'missing'" color="red" :label="$t('resources.overview.noBinding')" />
                      <q-badge v-else-if="row.bindingState === 'device'" color="teal" :label="$t('resources.tts.system')" />
                      <q-badge v-else-if="row.upstreamStatus && row.upstreamStatus !== 'ACTIVE'" color="orange" :label="$t('resources.overview.degradedUpstream')" />
                      <q-badge v-else color="green" :label="$t('resources.overview.bound')" />
                      <q-toggle data-testid="relationship-enable-toggle" :model-value="row.enabled" :label="row.enabled ? $t('resources.overview.on') : $t('resources.overview.off')" @update:model-value="(v: boolean) => toggleEnabled(row.kind, row.resourceId, v)" @click.stop />
                    </div>
                  </td>
                </tr>
                <tr v-if="!filteredRelationshipRows.length"><td colspan="6" class="text-grey-7">{{ $t('resources.overview.noResources') }}</td></tr>
              </tbody>
            </q-markup-table>
          </q-card-section>
        </q-card>
        </details>
      </template>

      <!-- ===== Models: Collection → Editor ===== -->
      <template v-if="activeTab === 'models'">
        <div class="row q-col-gutter-xs resource-split" :class="{ 'resource-split--selected': Boolean(selectedModel) }">
          <!-- Collection -->
          <div class="col-12 col-md-4 resource-split__collection">
            <q-card flat bordered>
              <q-card-section class="row items-center justify-between">
                <div class="text-subtitle2">{{ $t('resources.tabs.models') }}</div>
                <q-btn flat dense icon="add" :label="$t('common.add')" size="sm" data-cy="add-model-btn" :disable="!draft.localContent.providers.length" @click="addModel()" />
              </q-card-section>
              <q-card-section class="q-pa-xs"><q-input v-model="collectionQuery" dense outlined clearable :label="$t('common.search')"><template #prepend><q-icon name="search" /></template></q-input></q-card-section>
              <q-list separator>
                <q-item v-for="model in filteredModels" :key="model.modelId"
                  :active="selectedResourceId === model.modelId" clickable @click="selectModel(model.modelId)">
                  <q-item-section>
                    <q-item-label>{{ model.displayName }}</q-item-label>
                    <q-item-label caption>
                      <q-badge v-for="m in model.inputModalities" :key="m" dense color="primary" :label="$t(`resources.model.${m.toLowerCase()}`)" class="q-mr-xs" />
                      · {{ model.upstreamModelKey || $t('resources.model.noKey') }}
                    </q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-xs">
                      <q-badge v-if="!draft.bindingFor(model.modelId)?.upstreamId" color="red" :label="$t('resources.overview.noBinding')" />
                      <q-badge v-if="!model.enabled" color="grey" :label="$t('resources.overview.off')" />
                      <q-btn flat dense color="negative" icon="delete" size="sm" :aria-label="$t('resources.removeNamed', { name: model.displayName })" @click.stop="removeResource('MODEL', model.modelId, model.displayName)" />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="!draft.localContent.models.length"><q-item-section class="text-grey-7">{{ $t('resources.model.noModels') }}</q-item-section></q-item>
              </q-list>
            </q-card>
          </div>

          <!-- Model Editor -->
          <div class="col-12 col-md-8 resource-split__detail">
            <q-card v-if="selectedModel" flat bordered>
              <!-- Header: Identity -->
              <q-card-section class="row items-start justify-between">
                <q-btn class="resource-detail-back" flat dense no-caps icon="arrow_back" :label="$t('resources.backToList')" @click="selectedResourceId = undefined" />
                <div>
                  <div class="text-h6">{{ selectedModel.displayName }}</div>
                  <div class="text-caption">{{ modelProtocols.find(option => option.value === draft.localContent!.providers.find(p => p.providerId === selectedModel!.providerId)?.clientProtocol)?.label }}</div>
                  <details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ selectedModel.modelId }}</details>
                </div>
                <div class="row items-center q-gutter-xs">
                  <q-badge v-if="draft.dirty" color="orange" :label="$t('common.dirty').toLowerCase()" />
                  <q-toggle v-model="selectedModel.enabled" :label="$t('common.enabled')" @update:model-value="draft.markDirty()" />
                </div>
              </q-card-section>
              <q-separator />

              <!-- Identity -->
              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.model.identity') }}</div>
                <div class="row q-gutter-xs">
                  <q-input v-model="selectedModel.displayName" dense outlined :label="$t('resources.model.displayName')" class="col" data-cy="model-display-name" data-field="displayName" @update:model-value="draft.markDirty()" />
                  <q-select v-model="selectedModel.providerId" dense outlined :label="$t('resources.model.provider')" :options="draft.localContent.providers.map((p) => ({ label: p.displayName, value: p.providerId }))" emit-value map-options class="col" data-cy="model-provider-select" @update:model-value="draft.markDirty()" />
                </div>
              </q-card-section>
              <q-separator />

              <!-- Capability -->
              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.model.capability') }}</div>
                <div class="row q-gutter-xs q-mb-xs">
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">{{ $t('resources.model.upstreamModelKey') }}</div>
                    <q-input v-model="selectedModel.upstreamModelKey" dense outlined :label="$t('resources.model.upstreamModelKey')" :hint="$t('resources.model.upstreamModelKeyHint')" data-cy="model-upstream-key" data-field="upstreamModelKey" @update:model-value="draft.markDirty()" />
                  </div>
                </div>
                <div class="row q-gutter-xs">
                  <q-select v-model="selectedModel.inputModalities" dense outlined :label="$t('resources.model.inputModalities')" multiple :options="INPUT_MODS.map(value => ({ label: $t(`resources.model.${value.toLowerCase()}`), value }))" class="col" data-field="inputModalities" emit-value map-options @update:model-value="draft.markDirty()" />
                  <q-select v-model="selectedModel.outputModalities" dense outlined :label="$t('resources.model.outputModalities')" multiple :options="OUTPUT_MODS.map(value => ({ label: $t(`resources.model.${value.toLowerCase()}`), value }))" class="col" data-field="outputModalities" emit-value map-options @update:model-value="draft.markDirty()" />
                  <q-select v-model="selectedModel.capabilities" dense outlined :label="$t('resources.model.capabilities')" multiple :options="MODEL_CAPS.map(value => ({ label: $t(`resources.model.${value.toLowerCase()}`), value }))" class="col" emit-value map-options @update:model-value="draft.markDirty()" />
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.model.capabilityHint') }}</div>
              </q-card-section>
              <q-separator />

              <!-- Execution -->
              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.model.execution') }}</div>
                <div class="row q-gutter-xs">
                  <PagedEntityPicker :model-value="draft.bindingFor(selectedModel.modelId)?.upstreamId" :label="$t('resources.model.upstream')" :empty-label="$t('resources.overview.noBinding')" :fetch-page="fetchUpstreamPickerPage" :resolve-option="resolveUpstreamPickerOption" :disabled="upstreamsLoading || !!upstreamError" :clearable="false" class="col" data-cy="model-upstream-select" data-field="upstreamId" @update:model-value="(v) => v && draft.setBinding(selectedModel!.modelId, v, 'HTTP_STREAMING_SSE')" />
                  <q-input :model-value="selectedModel.runtimePath" dense outlined :label="$t('resources.model.runtimePath')" :hint="$t('resources.model.runtimePathHint')" class="col" data-cy="model-runtime-path" @update:model-value="v => draft.setRuntimePath(selectedModel!.modelId, String(v ?? ''))" />
                </div>
                <div v-if="draft.localContent.providers.find(provider => provider.providerId === selectedModel!.providerId)?.clientProtocol === 'GOOGLE_GENERATE_CONTENT'" class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.model.geminiPathHint') }}</div>
                <div class="text-caption text-grey-7 q-mt-xs">
                  {{ $t('resources.model.transportSummary') }}
                </div>
                <q-banner v-if="!draft.bindingFor(selectedModel.modelId)?.upstreamId" class="bg-red-1 q-mt-xs rounded-borders">
                  <div class="text-body2 text-negative">{{ $t('resources.model.missingUpstream') }}</div>
                </q-banner>
              </q-card-section>
              <q-separator />

              <!-- Validation -->
              <q-card-section v-if="draft.validationResult">
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.model.validation') }}</div>
                <q-list dense>
                  <q-item v-for="e in validationIssuesFor(selectedModel.modelId).errors" :key="e.path + e.code">
                    <q-item-section avatar><q-icon name="error" color="negative" /></q-item-section>
                    <q-item-section><span class="text-negative">{{ e.code }} — {{ e.message }}</span> <span class="text-caption text-grey-7">{{ e.path }}</span></q-item-section>
                  </q-item>
                  <q-item v-for="w in validationIssuesFor(selectedModel.modelId).warnings" :key="w.path + w.code">
                    <q-item-section avatar><q-icon name="warning" color="amber-8" /></q-item-section>
                    <q-item-section><span class="text-amber-8">{{ w.code }} — {{ w.message }}</span> <span class="text-caption text-grey-7">{{ w.path }}</span></q-item-section>
                  </q-item>
                  <q-item v-if="!validationIssuesFor(selectedModel.modelId).errors.length && !validationIssuesFor(selectedModel.modelId).warnings.length">
                    <q-item-section class="text-positive">{{ $t('resources.model.noIssues') }}</q-item-section>
                  </q-item>
                </q-list>
              </q-card-section>
            </q-card>
            <q-card v-else flat bordered>
              <q-card-section class="text-grey-7 text-center">
                <q-icon name="smart_toy" size="3rem" />
                <div class="text-body2 q-mt-xs">{{ $t('resources.model.selectOrAdd') }}</div>
              </q-card-section>
            </q-card>
          </div>
        </div>
      </template>

      <!-- ===== Image Generation: Collection → Editor ===== -->
      <template v-if="activeTab === 'image-generation'">
        <div class="row q-col-gutter-xs resource-split" :class="{ 'resource-split--selected': Boolean(selectedImageGeneration) }">
          <div class="col-12 col-md-4 resource-split__collection">
            <q-card flat bordered>
              <q-card-section class="row items-center justify-between">
                <div class="text-subtitle2">{{ $t('resources.tabs.imageGeneration') }}</div>
                <q-btn flat dense icon="add" :label="$t('common.add')" size="sm" data-cy="add-image-generation-btn" @click="selectedResourceId = draft.addImageGeneration()" />
              </q-card-section>
              <q-card-section class="q-pa-xs"><q-input v-model="collectionQuery" dense outlined clearable :label="$t('common.search')"><template #prepend><q-icon name="search" /></template></q-input></q-card-section>
              <q-list separator>
                <q-item v-for="image in filteredImageGenerators" :key="image.imageId" :active="selectedResourceId === image.imageId" clickable @click="selectImageGeneration(image.imageId)">
                  <q-item-section>
                    <q-item-label>{{ image.displayName }}</q-item-label>
                    <q-item-label caption>{{ image.upstreamModelKey || $t('resources.imageGeneration.noKey') }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-xs">
                      <q-badge v-if="!draft.bindingFor(image.imageId)?.upstreamId" color="red" :label="$t('resources.overview.noBinding')" />
                      <q-btn flat dense color="negative" icon="delete" size="sm" :aria-label="$t('resources.removeNamed', { name: image.displayName })" @click.stop="removeResource('IMAGE_GENERATION', image.imageId, image.displayName)" />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="!draft.localContent.imageGenerators?.length"><q-item-section class="text-grey-7">{{ $t('resources.imageGeneration.empty') }}</q-item-section></q-item>
              </q-list>
            </q-card>
          </div>
          <div class="col-12 col-md-8 resource-split__detail">
            <q-btn flat dense icon="arrow_back" class="resource-detail-back q-mb-xs" :label="$t('resources.backToList')" @click="selectedResourceId = undefined" />
            <q-card v-if="selectedImageGeneration" flat bordered>
              <q-card-section class="row items-start justify-between">
                <div>
                  <div class="text-subtitle1 text-weight-medium">{{ selectedImageGeneration.displayName }}</div>
                  <details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ selectedImageGeneration.imageId }} · {{ selectedImageGeneration.clientProtocol }}</details>
                </div>
                <q-toggle v-model="selectedImageGeneration.enabled" :label="$t('common.enabled')" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />
              <q-card-section class="q-gutter-xs">
                <div class="text-subtitle2">{{ $t('resources.imageGeneration.identity') }}</div>
                <q-input v-model="selectedImageGeneration.displayName" dense outlined :label="$t('resources.imageGeneration.displayName')" data-cy="image-generation-display-name" @update:model-value="draft.markDirty()" />
                <q-select :model-value="selectedImageGeneration.clientProtocol" :options="imageGenerationProtocols" emit-value map-options dense outlined :label="$t('resources.imageGeneration.protocol')" data-cy="image-generation-protocol" @update:model-value="value => draft.setImageGenerationProtocol(selectedImageGeneration!.imageId, value as ImageGenerationProtocol)" />
                <div class="text-caption text-grey-7">{{ $t('resources.imageGeneration.protocolHint') }}</div>
                <q-input v-model="selectedImageGeneration.upstreamModelKey" dense outlined :label="$t('resources.imageGeneration.modelKey')" data-cy="image-generation-model-key" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />
              <q-card-section class="q-gutter-xs">
                <div class="text-subtitle2">{{ $t('resources.imageGeneration.requestLimits') }}</div>
                <q-input v-model.number="selectedImageGeneration.maxImagesPerRequest" type="number" min="1" max="6" dense outlined :label="$t('resources.imageGeneration.maxImages')" data-cy="image-generation-max-images" :rules="[(value: number) => Number.isInteger(value) && value >= 1 && value <= 6 || $t('resources.imageGeneration.maxImagesInvalid')]" @update:model-value="draft.markDirty()" />
                <q-select v-model="selectedImageGeneration.allowedSizes" multiple use-chips use-input new-value-mode="add-unique" dense outlined :options="imageSizeOptions" :label="$t('resources.imageGeneration.allowedSizes')" data-cy="image-generation-allowed-sizes" :rules="[(value: string[]) => value.length > 0 || $t('resources.imageGeneration.allowedSizesRequired')]" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />
              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.imageGeneration.execution') }}</div>
                <div class="row q-gutter-xs">
                  <PagedEntityPicker :model-value="draft.bindingFor(selectedImageGeneration.imageId)?.upstreamId" :label="$t('resources.model.upstream')" :empty-label="$t('resources.overview.noBinding')" :fetch-page="fetchUpstreamPickerPage" :resolve-option="resolveUpstreamPickerOption" :disabled="upstreamsLoading || !!upstreamError" :clearable="false" class="col" data-cy="image-generation-upstream-select" @update:model-value="value => value && draft.setBinding(selectedImageGeneration!.imageId, value, 'HTTP_REQUEST_RESPONSE')" />
                  <q-input :model-value="selectedImageGeneration.runtimePath" dense outlined :label="$t('resources.model.runtimePath')" class="col" data-cy="image-generation-runtime-path" @update:model-value="value => draft.setRuntimePath(selectedImageGeneration!.imageId, String(value ?? ''))" />
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">{{ imageGenerationTransportHint }}</div>
              </q-card-section>
            </q-card>
            <q-card v-else flat bordered><q-card-section class="text-grey-7 text-center"><q-icon name="image" size="3rem" /><div>{{ $t('resources.imageGeneration.selectOrAdd') }}</div></q-card-section></q-card>
          </div>
        </div>
      </template>

      <!-- ===== TTS: Collection → Editor ===== -->
      <template v-if="activeTab === 'tts'">
        <div class="row q-col-gutter-xs resource-split" :class="{ 'resource-split--selected': Boolean(selectedTts) }">
          <div class="col-12 col-md-4 resource-split__collection">
            <q-card flat bordered>
              <q-card-section class="row items-center justify-between">
                <div class="text-subtitle2">{{ $t('resources.tabs.tts') }}</div>
                <q-btn flat dense icon="add" :label="$t('common.add')" size="sm" data-cy="add-tts-btn" @click="draft.addTts(); selectedResourceId = draft.localContent?.tts[draft.localContent.tts.length - 1]?.ttsId" />
              </q-card-section>
              <q-card-section class="q-pa-xs"><q-input v-model="collectionQuery" dense outlined clearable :label="$t('common.search')"><template #prepend><q-icon name="search" /></template></q-input></q-card-section>
              <q-list separator>
                <q-item v-for="tts in filteredTts" :key="tts.ttsId"
                  :active="selectedResourceId === tts.ttsId" clickable @click="selectTts(tts.ttsId)">
                  <q-item-section>
                    <q-item-label>{{ tts.displayName }}</q-item-label>
                    <q-item-label caption>{{ ttsSummary(tts) }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-xs">
                      <q-badge v-if="tts.clientProtocol !== 'SYSTEM_TTS' && !draft.bindingFor(tts.ttsId)?.upstreamId" color="red" :label="$t('resources.overview.noBinding')" />
                      <q-badge v-if="tts.clientProtocol !== 'SYSTEM_TTS' && !isVoiceDesign(tts) && !tts.voice" color="red" :label="$t('resources.tts.noVoice')" />
                      <q-btn flat dense color="negative" icon="delete" size="sm" :aria-label="$t('resources.removeNamed', { name: tts.displayName })" @click.stop="removeResource('TTS', tts.ttsId, tts.displayName)" />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="!draft.localContent.tts.length"><q-item-section class="text-grey-7">{{ $t('resources.tts.noTts') }}</q-item-section></q-item>
              </q-list>
            </q-card>
          </div>

          <div class="col-12 col-md-8 resource-split__detail">
            <q-card v-if="selectedTts" flat bordered>
              <q-card-section class="row items-start justify-between">
                <q-btn class="resource-detail-back" flat dense no-caps icon="arrow_back" :label="$t('resources.backToList')" @click="selectedResourceId = undefined" />
                <div>
                  <div class="text-h6">{{ selectedTts.displayName }}</div>
                  <details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ selectedTts.ttsId }} · {{ selectedTts.clientProtocol }}</details>
                </div>
                <q-toggle v-model="selectedTts.enabled" :label="$t('common.enabled')" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.tts.identity') }}</div>
                <q-input v-model="selectedTts.displayName" dense outlined :label="$t('resources.tts.displayName')" data-cy="tts-display-name" data-field="displayName" @update:model-value="draft.markDirty()" />
                <q-select :model-value="selectedTts.clientProtocol" :options="ttsProtocols" emit-value map-options dense outlined class="q-mt-xs" :label="$t('resources.tts.protocol')" :hint="$t('resources.tts.protocolHint')" data-cy="tts-protocol" @update:model-value="(value: TtsProtocol) => draft.setTtsProtocol(selectedTts!.ttsId, value)" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.tts.speechProfile') }}</div>
                <div v-if="selectedTts.clientProtocol === 'SYSTEM_TTS'" class="row q-gutter-xs">
                  <q-input v-model.number="selectedTts.speechRate" type="number" min="0.1" step="0.1" dense outlined :label="$t('resources.tts.speechRate')" class="col" data-cy="tts-speech-rate" @update:model-value="draft.markDirty()" />
                  <q-input v-model.number="selectedTts.pitch" type="number" min="0.1" step="0.1" dense outlined :label="$t('resources.tts.pitch')" class="col" data-cy="tts-pitch" @update:model-value="draft.markDirty()" />
                </div>
                <template v-else>
                <div class="row q-gutter-xs q-mb-xs">
                  <q-input v-model="selectedTts.upstreamModelKey" dense outlined :label="$t('resources.tts.modelKey')" :hint="$t('resources.model.upstreamModelKeyHint')" class="col" data-cy="tts-model-key" @update:model-value="updateTtsModel" />
                  <q-input v-if="!isVoiceDesign(selectedTts)" v-model="selectedTts.voice" dense outlined :label="$t('resources.tts.voice')" :hint="$t('resources.tts.voiceHint')" class="col" data-cy="tts-voice"
                    :rules="[(v: string) => !!v || $t('resources.tts.voiceRequired')]"
                    @update:model-value="draft.markDirty()" />
                </div>
                <q-input v-if="selectedTts.clientProtocol === 'MIMO_CHAT_COMPLETIONS_TTS'" :model-value="selectedTts.voiceDesignPrompt" type="textarea" autogrow dense outlined :label="$t('resources.tts.voiceDesignPrompt')" :hint="$t(isVoiceDesign(selectedTts) ? 'resources.tts.designPromptRequired' : 'resources.tts.stylePromptOptional')" data-cy="tts-voice-design" @update:model-value="updateVoiceDesignPrompt" />
                <div class="row q-gutter-xs items-center">
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">{{ $t('resources.tts.outputBaseline') }}</div>
                    <q-badge color="grey" :label="$t(selectedTts.clientProtocol === 'OPENAI_AUDIO_SPEECH' ? 'resources.tts.mp3BaselineBadge' : 'resources.tts.pcmOutput')" />
                  </div>
                </div>
                <q-banner v-if="!isVoiceDesign(selectedTts) && !selectedTts.voice" class="bg-red-1 q-mt-xs rounded-borders">
                  <div class="text-body2 text-negative">{{ $t('resources.tts.voiceRequired') }}</div>
                </q-banner>
                </template>
                <div v-if="selectedTts.clientProtocol === 'SYSTEM_TTS'" class="text-caption q-mt-xs">{{ $t('resources.tts.systemHint') }}</div>
              </q-card-section>
              <q-separator />

              <q-card-section v-if="selectedTts.clientProtocol !== 'SYSTEM_TTS'">
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.tts.execution') }}</div>
                <div class="row q-gutter-xs">
                  <PagedEntityPicker :model-value="draft.bindingFor(selectedTts.ttsId)?.upstreamId" :label="$t('resources.model.upstream')" :empty-label="$t('resources.overview.noBinding')" :fetch-page="fetchUpstreamPickerPage" :resolve-option="resolveUpstreamPickerOption" :disabled="upstreamsLoading || !!upstreamError" :clearable="false" class="col" data-cy="tts-upstream-select" @update:model-value="(v) => v && draft.setBinding(selectedTts!.ttsId, v, ttsTransport(selectedTts!.clientProtocol))" />
                  <q-input :model-value="selectedTts.runtimePath" dense outlined :label="$t('resources.model.runtimePath')" :hint="$t('resources.model.runtimePathHint')" class="col" data-cy="tts-runtime-path" @update:model-value="v => draft.setRuntimePath(selectedTts!.ttsId, String(v ?? ''))" />
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.tts.cloudHint') }}</div>
              </q-card-section>
              <q-separator />

              <q-card-section v-if="draft.validationResult">
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.tts.validation') }}</div>
                <q-list dense>
                  <q-item v-for="e in validationIssuesFor(selectedTts.ttsId).errors" :key="e.path + e.code">
                    <q-item-section avatar><q-icon name="error" color="negative" /></q-item-section>
                    <q-item-section><span class="text-negative">{{ e.code }} — {{ e.message }}</span> <span class="text-caption text-grey-7">{{ e.path }}</span></q-item-section>
                  </q-item>
                  <q-item v-for="w in validationIssuesFor(selectedTts.ttsId).warnings" :key="w.path + w.code">
                    <q-item-section avatar><q-icon name="warning" color="amber-8" /></q-item-section>
                    <q-item-section><span class="text-amber-8">{{ w.code }} — {{ w.message }}</span> <span class="text-caption text-grey-7">{{ w.path }}</span></q-item-section>
                  </q-item>
                  <q-item v-if="!validationIssuesFor(selectedTts.ttsId).errors.length && !validationIssuesFor(selectedTts.ttsId).warnings.length">
                    <q-item-section class="text-positive">{{ $t('resources.tts.noIssues') }}</q-item-section>
                  </q-item>
                </q-list>
              </q-card-section>
            </q-card>
            <q-card v-else flat bordered>
              <q-card-section class="text-grey-7 text-center">
                <q-icon name="record_voice_over" size="3rem" />
                <div class="text-body2 q-mt-xs">{{ $t('resources.tts.selectOrAdd') }}</div>
              </q-card-section>
            </q-card>
          </div>
        </div>
      </template>

      <!-- ===== ASR: Collection → Editor ===== -->
      <template v-if="activeTab === 'asr'">
        <div class="row q-col-gutter-xs resource-split" :class="{ 'resource-split--selected': Boolean(selectedAsr) }">
          <div class="col-12 col-md-4 resource-split__collection">
            <q-card flat bordered>
              <q-card-section class="row items-center justify-between">
                <div class="text-subtitle2">{{ $t('resources.tabs.asr') }}</div>
                <q-btn flat dense icon="add" :label="$t('common.add')" size="sm" data-cy="add-asr-btn" @click="draft.addAsr(); selectedResourceId = draft.localContent?.asr[draft.localContent.asr.length - 1]?.asrId" />
              </q-card-section>
              <q-card-section class="q-pa-xs"><q-input v-model="collectionQuery" dense outlined clearable :label="$t('common.search')"><template #prepend><q-icon name="search" /></template></q-input></q-card-section>
              <q-list separator>
                <q-item v-for="asr in filteredAsr" :key="asr.asrId"
                  :active="selectedResourceId === asr.asrId" clickable @click="selectAsr(asr.asrId)">
                  <q-item-section>
                    <q-item-label>{{ asr.displayName }}</q-item-label>
                    <q-item-label caption>{{ asr.language || $t('resources.asr.anyLanguage') }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-xs">
                      <q-badge v-if="!draft.bindingFor(asr.asrId)?.upstreamId" color="red" :label="$t('resources.overview.noBinding')" />
                      <q-btn flat dense color="negative" icon="delete" size="sm" :aria-label="$t('resources.removeNamed', { name: asr.displayName })" @click.stop="removeResource('ASR', asr.asrId, asr.displayName)" />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="!draft.localContent.asr.length"><q-item-section class="text-grey-7">{{ $t('resources.asr.noAsr') }}</q-item-section></q-item>
              </q-list>
            </q-card>
          </div>

          <div class="col-12 col-md-8 resource-split__detail">
            <q-card v-if="selectedAsr" flat bordered>
              <q-card-section class="row items-start justify-between">
                <q-btn class="resource-detail-back" flat dense no-caps icon="arrow_back" :label="$t('resources.backToList')" @click="selectedResourceId = undefined" />
                <div>
                  <div class="text-h6">{{ selectedAsr.displayName }}</div>
                  <details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ selectedAsr.asrId }} · {{ selectedAsr.clientProtocol }}</details>
                </div>
                <q-toggle v-model="selectedAsr.enabled" :label="$t('common.enabled')" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.asr.identity') }}</div>
                <q-input v-model="selectedAsr.displayName" dense outlined :label="$t('resources.asr.displayName')" data-cy="asr-display-name" data-field="displayName" @update:model-value="draft.markDirty()" />
                <q-select :model-value="selectedAsr.clientProtocol" :options="asrProtocols" emit-value map-options dense outlined class="q-mt-xs" :label="$t('resources.asr.service')" :hint="$t('resources.asr.switchHint')" data-cy="asr-protocol" @update:model-value="(v: AsrProtocol) => draft.setAsrProtocol(selectedAsr!.asrId, v)" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.asr.transcriptionProfile') }}</div>
                <div class="row q-gutter-xs q-mb-xs">
                  <q-input v-model="selectedAsr.upstreamModelKey" dense outlined :label="$t('resources.asr.modelKey')" :hint="$t('resources.model.upstreamModelKeyHint')" class="col" data-cy="asr-model-key" @update:model-value="draft.markDirty()" />
                  <q-input :model-value="selectedAsr.language" dense outlined :label="$t('resources.asr.optionalLanguage')" :hint="$t('resources.asr.optionalLanguageHint')" class="col" @update:model-value="value => { if (selectedAsr) { if (value) selectedAsr.language = String(value); else delete selectedAsr.language; draft.markDirty() } }" />
                </div>
                <div class="row q-gutter-xs items-center">
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">{{ $t('upstreams.transport') }}</div>
                    <q-badge color="grey" :label="selectedAsr.clientProtocol === 'OPENAI_AUDIO_TRANSCRIPTIONS' ? $t('resources.asr.transportBadge') : selectedAsr.clientProtocol === 'DASHSCOPE_HTTP_ASR' ? $t('resources.asr.jsonTransportBadge') : 'WebSocket'" />
                  </div>
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">{{ $t(isRealtimeAsr(selectedAsr.clientProtocol) ? 'resources.asr.realtimeHint' : selectedAsr.clientProtocol === 'DASHSCOPE_HTTP_ASR' ? 'resources.asr.dashscopeHttpHint' : 'resources.asr.notRealtimeHint') }}</div>
                <q-expansion-item v-if="isRealtimeAsr(selectedAsr.clientProtocol)" :label="$t('resources.asr.audioSettings')" class="q-mt-xs" data-cy="asr-audio-settings">
                  <div class="q-pa-sm q-gutter-xs">
                    <q-select v-model="selectedAsr.sampleRate" :options="selectedAsr.clientProtocol === 'OPENAI_REALTIME_TRANSCRIPTION' ? [24000] : [16000, 8000]" dense outlined :label="$t('resources.asr.sampleRate')" data-cy="asr-sample-rate" @update:model-value="draft.markDirty()" />
                    <q-input v-model.number="selectedAsr.vadThreshold" type="number" min="0" max="1" step="0.1" dense outlined :label="$t('resources.asr.vadThreshold')" @update:model-value="draft.markDirty()" />
                    <q-input v-model.number="selectedAsr.silenceDurationMs" type="number" min="1" step="100" dense outlined :label="$t('resources.asr.silenceDuration')" @update:model-value="draft.markDirty()" />
                    <template v-if="selectedAsr.clientProtocol === 'OPENAI_REALTIME_TRANSCRIPTION'">
                      <q-input v-model.number="selectedAsr.prefixPaddingMs" type="number" min="0" step="100" dense outlined :label="$t('resources.asr.prefixPadding')" @update:model-value="draft.markDirty()" />
                      <q-input v-model="selectedAsr.prompt" type="textarea" dense outlined :label="$t('resources.asr.prompt')" @update:model-value="(v) => { if (!v) delete selectedAsr!.prompt; draft.markDirty() }" />
                    </template>
                  </div>
                </q-expansion-item>
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.asr.execution') }}</div>
                <div class="row q-gutter-xs">
                  <PagedEntityPicker :model-value="draft.bindingFor(selectedAsr.asrId)?.upstreamId" :label="$t('resources.model.upstream')" :empty-label="$t('resources.overview.noBinding')" :fetch-page="fetchUpstreamPickerPage" :resolve-option="resolveUpstreamPickerOption" :disabled="upstreamsLoading || !!upstreamError" :clearable="false" class="col" data-cy="asr-upstream-select" @update:model-value="(v) => v && draft.setBinding(selectedAsr!.asrId, v, asrTransport(selectedAsr!.clientProtocol))" />
                  <q-input :model-value="selectedAsr.runtimePath" dense outlined :label="$t('resources.model.runtimePath')" :hint="$t('resources.model.runtimePathHint')" class="col" data-cy="asr-runtime-path" @update:model-value="v => draft.setRuntimePath(selectedAsr!.asrId, String(v ?? ''))" />
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.asr.bindingHint') }}</div>
              </q-card-section>
              <q-separator />

              <q-card-section v-if="draft.validationResult">
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.asr.validation') }}</div>
                <q-list dense>
                  <q-item v-for="e in validationIssuesFor(selectedAsr.asrId).errors" :key="e.path + e.code">
                    <q-item-section avatar><q-icon name="error" color="negative" /></q-item-section>
                    <q-item-section><span class="text-negative">{{ e.code }} — {{ e.message }}</span> <span class="text-caption text-grey-7">{{ e.path }}</span></q-item-section>
                  </q-item>
                  <q-item v-for="w in validationIssuesFor(selectedAsr.asrId).warnings" :key="w.path + w.code">
                    <q-item-section avatar><q-icon name="warning" color="amber-8" /></q-item-section>
                    <q-item-section><span class="text-amber-8">{{ w.code }} — {{ w.message }}</span> <span class="text-caption text-grey-7">{{ w.path }}</span></q-item-section>
                  </q-item>
                  <q-item v-if="!validationIssuesFor(selectedAsr.asrId).errors.length && !validationIssuesFor(selectedAsr.asrId).warnings.length">
                    <q-item-section class="text-positive">{{ $t('resources.asr.noIssues') }}</q-item-section>
                  </q-item>
                </q-list>
              </q-card-section>
            </q-card>
            <q-card v-else flat bordered>
              <q-card-section class="text-grey-7 text-center">
                <q-icon name="hearing" size="3rem" />
                <div class="text-body2 q-mt-xs">{{ $t('resources.asr.selectOrAdd') }}</div>
              </q-card-section>
            </q-card>
          </div>
        </div>
      </template>

      <!-- ===== MCP: Collection → Editor ===== -->
      <template v-if="activeTab === 'mcp'">
        <div class="row q-col-gutter-xs resource-split" :class="{ 'resource-split--selected': Boolean(selectedMcp) }">
          <div class="col-12 col-md-4 resource-split__collection">
            <q-card flat bordered>
              <q-card-section class="row items-center justify-between">
                <div class="text-subtitle2">{{ $t('resources.tabs.mcp') }} <span class="text-caption text-grey-7">· {{ $t('resources.mcp.transport') }}</span></div>
                <q-btn flat dense icon="add" :label="$t('common.add')" size="sm" data-cy="add-mcp-btn" @click="draft.addMcp(); selectedResourceId = draft.localContent?.mcp[draft.localContent.mcp.length - 1]?.mcpServerId" />
              </q-card-section>
              <q-card-section class="q-pa-xs"><q-input v-model="collectionQuery" dense outlined clearable :label="$t('common.search')"><template #prepend><q-icon name="search" /></template></q-input></q-card-section>
              <q-list separator>
                <q-item v-for="mcp in filteredMcp" :key="mcp.mcpServerId"
                  :active="selectedResourceId === mcp.mcpServerId" clickable @click="selectMcp(mcp.mcpServerId)">
                  <q-item-section>
                    <q-item-label>{{ mcp.displayName }}</q-item-label>
                    <q-item-label caption>{{ $t(`authOwnership.${mcp.authOwnership}`) }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-xs">
                      <q-badge v-if="!draft.bindingFor(mcp.mcpServerId)?.upstreamId" color="red" :label="$t('resources.overview.noBinding')" />
                      <q-btn flat dense color="negative" icon="delete" size="sm" :aria-label="$t('resources.removeNamed', { name: mcp.displayName })" @click.stop="removeResource('MCP', mcp.mcpServerId, mcp.displayName)" />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="!draft.localContent.mcp.length"><q-item-section class="text-grey-7">{{ $t('resources.mcp.noMcp') }}</q-item-section></q-item>
              </q-list>
            </q-card>
          </div>

          <div class="col-12 col-md-8 resource-split__detail">
            <q-card v-if="selectedMcp" flat bordered>
              <q-card-section class="row items-start justify-between">
                <q-btn class="resource-detail-back" flat dense no-caps icon="arrow_back" :label="$t('resources.backToList')" @click="selectedResourceId = undefined" />
                <div>
                  <div class="text-h6">{{ selectedMcp.displayName }}</div>
                  <details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ selectedMcp.mcpServerId }} · {{ $t('resources.mcp.protocolBadge') }}</details>
                </div>
                <q-toggle v-model="selectedMcp.enabled" :label="$t('common.enabled')" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.mcp.identity') }}</div>
                <q-input v-model="selectedMcp.displayName" dense outlined :label="$t('resources.mcp.displayName')" data-cy="mcp-display-name" data-field="displayName" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.mcp.mcpProfile') }}</div>
                <div class="row q-gutter-xs items-center q-mb-xs">
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">{{ $t('resources.mcp.authOwnership') }}</div>
                    <q-select v-model="selectedMcp.authOwnership" dense outlined :label="$t('resources.mcp.authOwnership')" :options="AUTH_OWNERSHIPS.map(value => ({ label: $t(`authOwnership.${value}`), value }))" emit-value map-options @update:model-value="draft.markDirty()" />
                  </div>
                </div>
                <q-banner v-if="selectedMcp.authOwnership === 'ENTERPRISE_MANAGED'" class="bg-blue-1 q-mt-xs rounded-borders">
                  <div class="text-body2 text-info">{{ $t('resources.mcp.enterpriseManagedHint') }}</div>
                </q-banner>
                <q-banner v-else class="bg-grey-1 q-mt-xs rounded-borders">
                  <div class="text-body2 text-grey-7">{{ $t('resources.mcp.noneHint') }}</div>
                </q-banner>
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.mcp.execution') }}</div>
                <div class="row q-gutter-xs">
                  <PagedEntityPicker :model-value="draft.bindingFor(selectedMcp.mcpServerId)?.upstreamId" :label="$t('resources.model.upstream')" :empty-label="$t('resources.overview.noBinding')" :fetch-page="fetchUpstreamPickerPage" :resolve-option="resolveUpstreamPickerOption" :disabled="upstreamsLoading || !!upstreamError" :clearable="false" class="col" data-cy="mcp-upstream-select" @update:model-value="(v) => v && draft.setBinding(selectedMcp!.mcpServerId, v, 'HTTP_REQUEST_RESPONSE')" />
                  <q-input :model-value="selectedMcp.runtimePath" dense outlined :label="$t('resources.model.runtimePath')" :hint="$t('resources.model.runtimePathHint')" class="col" data-cy="mcp-runtime-path" @update:model-value="v => draft.setRuntimePath(selectedMcp!.mcpServerId, String(v ?? ''))" />
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.mcp.transportSummary') }}</div>
              </q-card-section>
              <q-separator />

              <q-card-section v-if="draft.validationResult">
                <div class="text-subtitle2 q-mb-xs">{{ $t('resources.mcp.validation') }}</div>
                <q-list dense>
                  <q-item v-for="e in validationIssuesFor(selectedMcp.mcpServerId).errors" :key="e.path + e.code">
                    <q-item-section avatar><q-icon name="error" color="negative" /></q-item-section>
                    <q-item-section><span class="text-negative">{{ e.code }} — {{ e.message }}</span> <span class="text-caption text-grey-7">{{ e.path }}</span></q-item-section>
                  </q-item>
                  <q-item v-for="w in validationIssuesFor(selectedMcp.mcpServerId).warnings" :key="w.path + w.code">
                    <q-item-section avatar><q-icon name="warning" color="amber-8" /></q-item-section>
                    <q-item-section><span class="text-amber-8">{{ w.code }} — {{ w.message }}</span> <span class="text-caption text-grey-7">{{ w.path }}</span></q-item-section>
                  </q-item>
                  <q-item v-if="!validationIssuesFor(selectedMcp.mcpServerId).errors.length && !validationIssuesFor(selectedMcp.mcpServerId).warnings.length">
                    <q-item-section class="text-positive">{{ $t('resources.mcp.noIssues') }}</q-item-section>
                  </q-item>
                </q-list>
              </q-card-section>
            </q-card>
            <q-card v-else flat bordered>
              <q-card-section class="text-grey-7 text-center">
                <q-icon name="link" size="3rem" />
                <div class="text-body2 q-mt-xs">{{ $t('resources.mcp.selectOrAdd') }}</div>
              </q-card-section>
            </q-card>
          </div>
        </div>
      </template>

      <!-- ===== Policy Editor ===== -->
      <ManagedExperienceEditor v-if="activeTab === 'assistants'" :disabled="!canMutate || draft.saving || publishing" />
      <template v-if="activeTab === 'policy'">
        <q-card flat bordered>
          <q-card-section class="row items-center justify-between">
            <div>
              <div class="text-subtitle2">{{ $t('resources.tabs.policy') }}</div>
              <details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ $t('resources.policy.policyId') }}: {{ draft.localContent.policy.policyId }}</details>
            </div>
            <q-badge v-if="draft.dirty" color="orange" :label="$t('common.dirty').toLowerCase()" />
          </q-card-section>
          <q-separator />

          <q-card-section>
            <div class="text-subtitle2 q-mb-xs">{{ $t('resources.policy.localCoexistence') }}</div>
            <div class="text-body2 text-grey-7 q-mb-xs">{{ $t('resources.policy.coexistenceHint') }}</div>
            <q-list bordered separator class="settings-list">
              <q-item v-for="setting in policySettings" :key="setting.key" tag="label" data-cy="policy-setting-row">
                <q-item-section>
                  <q-item-label>{{ setting.label }}</q-item-label>
                  <q-item-label caption>{{ setting.hint }}</q-item-label>
                </q-item-section>
                <q-item-section side>
                  <q-toggle
                    :model-value="draft.localContent.policy[setting.key]"
                    :aria-label="setting.label"
                    :data-cy="setting.cy"
                    @update:model-value="(value: boolean) => setPolicyFlag(setting.key, value)"
                  />
                </q-item-section>
              </q-item>
            </q-list>
          </q-card-section>
          <q-separator />

          <q-card-section>
            <div class="text-subtitle2 q-mb-xs">{{ $t('resources.policy.defaults') }}</div>
            <div class="text-body2 text-grey-7 q-mb-xs">{{ $t('resources.policy.defaultsHint') }}</div>
            <div class="row q-col-gutter-xs">
              <div class="col-12 col-md-3">
                <q-select :model-value="draft.localContent.policy.defaultModelId" dense outlined :label="$t('resources.policy.defaultModel')" :options="enabledModels" emit-value map-options clearable data-cy="policy-default-model" @update:model-value="value => setPolicyDefault('defaultModelId', value)" />
                <div class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.policy.enabledModelsHint') }}</div>
              </div>
              <div class="col-12 col-md-3">
                <q-select :model-value="draft.localContent.policy.defaultImageGenerationId" dense outlined :label="$t('resources.policy.defaultImageGeneration')" :options="enabledImageGenerators" emit-value map-options clearable data-cy="policy-default-image-generation" @update:model-value="value => setPolicyDefault('defaultImageGenerationId', value)" />
                <div class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.policy.enabledImageGenerationHint') }}</div>
              </div>
              <div class="col-12 col-md-3">
                <q-select :model-value="draft.localContent.policy.defaultTtsId" dense outlined :label="$t('resources.policy.defaultTts')" :options="enabledTts" emit-value map-options clearable data-cy="policy-default-tts" @update:model-value="value => setPolicyDefault('defaultTtsId', value)" />
                <div class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.policy.enabledTtsHint') }}</div>
              </div>
              <div class="col-12 col-md-3">
                <q-select :model-value="draft.localContent.policy.defaultAsrId" dense outlined :label="$t('resources.policy.defaultAsr')" :options="enabledAsr" emit-value map-options clearable data-cy="policy-default-asr" @update:model-value="value => setPolicyDefault('defaultAsrId', value)" />
                <div class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.policy.enabledAsrHint') }}</div>
              </div>
              <div class="col-12 col-md-3">
                <q-select :model-value="draft.localContent.policy.defaultAssistantId" dense outlined :label="$t('resources.policy.defaultAssistant')" :options="enabledAssistants" emit-value map-options clearable data-cy="policy-default-assistant" @update:model-value="value => setPolicyDefault('defaultAssistantId', value)" />
                <div class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.policy.enabledAssistantsHint') }}</div>
              </div>
            </div>
          </q-card-section>
        </q-card>
      </template>

      <!-- Validation only occupies space after the operator explicitly runs it. -->
      <q-card v-if="draft.validationResult" flat bordered class="q-mt-xs" data-cy="draft-validation-summary">
        <q-card-section class="q-py-xs">
          <div class="text-subtitle2">{{ $t('resources.model.validation') }}</div>
          <div>
            <q-banner v-if="draft.validationResult.errors.length === 0 && draft.validationResult.warnings.length === 0" class="bg-green-1 q-mt-xs rounded-borders">
              {{ $t('resources.valid') }}
            </q-banner>
            <q-banner v-else class="bg-orange-1 q-mt-xs rounded-borders">
              <div>{{ $t('resources.errorsWarnings', { errors: draft.validationResult.errors.length, warnings: draft.validationResult.warnings.length }) }}</div>
            </q-banner>
            <q-list dense class="q-mt-xs">
              <q-item v-for="e in draft.validationResult.errors" :key="e.path + e.code" :clickable="!!e.resourceKind" @click="goToValidationIssue(e)">
                <q-item-section avatar><q-icon name="error" color="negative" /></q-item-section>
                <q-item-section>{{ e.path }}: {{ e.code }} — {{ e.message }}</q-item-section>
              </q-item>
              <q-item v-for="w in draft.validationResult.warnings" :key="w.path + w.code" :clickable="!!w.resourceKind" @click="goToValidationIssue(w)">
                <q-item-section avatar><q-icon name="warning" color="amber-8" /></q-item-section>
                <q-item-section>{{ w.path }}: {{ w.code }} — {{ w.message }}</q-item-section>
              </q-item>
            </q-list>
          </div>
        </q-card-section>
      </q-card>
        </section>
      </div>

      <!-- Review is a first-class work surface, not a modal confirmation. -->
        <q-card v-if="reviewOpen && !previewOpen" flat bordered data-cy="publish-review-surface">
          <q-card-section class="row items-center justify-between">
            <div>
              <div class="text-h6">{{ $t('resources.review.title') }}</div>
              <div class="text-caption text-grey-7">
                {{ preview?.publishedGeneration ? $t('resources.review.baseline', { generation: preview.publishedGeneration, revision: draft.baselineRevision }) : $t('resources.review.firstPublication', { revision: draft.baselineRevision }) }}
              </div>
            </div>
            <q-badge v-if="reviewTotalChanges === 0" color="grey" :label="$t('resources.review.noChanges')" data-cy="review-change-count" />
            <q-badge v-else color="primary" :label="$t('resources.review.changes', { count: reviewTotalChanges })" data-cy="review-change-count" />
          </q-card-section>
          <q-separator />

          <q-card-section v-if="preview">
            <q-banner v-if="unchangedPublishedDraft" data-cy="review-no-changes" class="bg-blue-1 q-mb-xs rounded-borders">
              {{ $t('resources.review.alreadyPublished') }}
            </q-banner>
            <!-- Blocking errors -->
            <q-banner v-if="hasBlockingErrors" class="bg-red-1 q-mb-xs rounded-borders">
              <div class="text-weight-medium text-negative">{{ $t('resources.review.blockingErrors', { count: draft.validationResult!.errors.length }) }}</div>
              <q-list dense class="q-mt-xs">
                <q-item v-for="e in draft.validationResult!.errors" :key="e.path + e.code" :clickable="!!e.resourceKind" @click="goToValidationIssue(e)">
                  <q-item-section avatar><q-icon name="error" color="negative" /></q-item-section>
                  <q-item-section>
                    <span class="text-negative">{{ e.code }} — {{ e.message }}</span>
                    <span class="text-caption text-grey-7">{{ e.path }}</span>
                  </q-item-section>
                </q-item>
              </q-list>
            </q-banner>

            <!-- Resource changes: Added / Changed / Removed -->
            <div v-if="reviewResourceRows.length" class="text-subtitle2 q-mb-xs">{{ $t('resources.review.resourceChanges') }}</div>
            <q-markup-table v-if="reviewResourceRows.length" flat dense class="q-mb-xs" data-cy="review-resource-table">
              <thead>
                <tr>
                  <th>{{ $t('resources.relationship.kind') }}</th>
                  <th class="text-right text-positive">{{ $t('resources.review.added') }}</th>
                  <th class="text-right text-warning">{{ $t('resources.review.changed') }}</th>
                  <th class="text-right text-negative">{{ $t('resources.review.removed') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in reviewResourceRows" :key="row.kind">
                  <td>{{ row.kind }}</td>
                  <td class="text-right text-positive">{{ row.diff.added > 0 ? '+' + row.diff.added : '—' }}</td>
                  <td class="text-right text-warning">{{ row.diff.changed > 0 ? '~' + row.diff.changed : '—' }}</td>
                  <td class="text-right text-negative">{{ row.diff.removed > 0 ? '-' + row.diff.removed : '—' }}</td>
                </tr>
              </tbody>
            </q-markup-table>

            <!-- Policy changes -->
            <div v-if="policyChanged" class="text-subtitle2 q-mb-xs">{{ $t('resources.review.policyChanges') }}</div>
            <q-banner v-if="policyChanged" class="bg-blue-1 q-mb-xs rounded-borders">
              <div class="text-body2">{{ $t('resources.review.policyChangedHint') }}</div>
            </q-banner>

            <!-- Runtime routing impact -->
            <div v-if="routingChanged" class="text-subtitle2 q-mb-xs">{{ $t('resources.review.runtimeImpact') }}</div>
            <q-markup-table v-if="routingChanged" flat dense class="q-mb-xs">
              <tbody>
                <tr><td class="text-grey-7">{{ $t('resources.review.bindingsAdded') }}</td><td class="text-positive">{{ routingImpact.added > 0 ? '+' + routingImpact.added : '—' }}</td></tr>
                <tr><td class="text-grey-7">{{ $t('resources.review.bindingsChanged') }}</td><td class="text-warning">{{ routingImpact.changed > 0 ? '~' + routingImpact.changed : '—' }}</td></tr>
                <tr><td class="text-grey-7">{{ $t('resources.review.bindingsRemoved') }}</td><td class="text-negative">{{ routingImpact.removed > 0 ? '-' + routingImpact.removed : '—' }}</td></tr>
              </tbody>
            </q-markup-table>

            <!-- Warnings -->
            <div v-if="reviewWarnings.length" class="text-subtitle2 q-mb-xs">{{ $t('resources.review.warningsAck') }}</div>
            <q-list v-if="reviewWarnings.length" dense class="q-mb-xs">
              <q-item v-for="w in reviewWarnings" :key="w.path + w.code">
                <q-item-section avatar><q-icon name="warning" color="amber-8" /></q-item-section>
                <q-item-section>
                  <span class="text-amber-8">{{ w.code }} — {{ w.message }}</span>
                  <span class="text-caption text-grey-7">{{ w.path }}</span>
                </q-item-section>
              </q-item>
            </q-list>

            <!-- Snapshot hash -->
            <details class="text-caption text-grey-7"><summary class="cursor-pointer">{{ $t('resources.review.technicalDetails') }}</summary>
              {{ $t('resources.review.projectionHash') }}: <code style="overflow-wrap: anywhere">{{ preview.projectionHash }}</code>
            </details>
          </q-card-section>

          <q-separator />
          <q-card-actions align="right">
            <q-btn flat no-caps color="primary" :label="$t('resources.review.inspectFinalContent')" @click="previewOpen = true" data-cy="review-preview-btn" />
            <q-btn flat icon="arrow_back" :label="$t('common.cancel')" :disable="publishing" @click="reviewOpen = false" />
            <q-btn
              color="positive"
              icon="rocket_launch"
              :label="reviewWarnings.length ? $t('resources.review.publishWithWarnings', { count: reviewWarnings.length }) : $t('resources.draft.publish')"
              :disable="hasBlockingErrors || unchangedPublishedDraft || publishing"
              :loading="publishing"
              @click="publish"
              data-cy="draft-publish-btn"
            />
          </q-card-actions>
        </q-card>

      <!-- Canonical client preview replaces the editor while open. -->
        <q-card v-if="previewOpen" flat bordered data-cy="snapshot-preview-surface">
          <q-card-section class="text-h6">{{ $t('resources.preview.title') }}</q-card-section>
          <q-card-section v-if="preview">
            <div class="text-body2 q-mb-xs">{{ $t('resources.preview.intro') }}</div>
            <details class="text-caption text-grey-7 q-mb-xs"><summary class="cursor-pointer">{{ $t('resources.review.technicalDetails') }}</summary>
              {{ $t('resources.preview.hash') }}: <code style="overflow-wrap: anywhere">{{ preview.projectionHash }}</code><br>
              {{ $t('resources.draft.revision') }} {{ preview.draftRevision }}
            <div class="q-mt-xs">
              <div class="text-body2"><b>{{ $t('resources.preview.clientReceives') }}:</b> {{ $t('resources.preview.clientReceivesList') }}</div>
              <div class="text-body2 text-negative"><b>{{ $t('resources.preview.clientNeverReceives') }}:</b> {{ $t('resources.preview.clientNeverReceivesList') }}</div>
            </div>
            </details>

            <q-expansion-item dense group="preview" :label="`${$t('resources.preview.providers')} (${preview.providers.length})`" icon="domain">
              <q-markup-table flat dense>
                <thead><tr><th>{{ $t('resources.preview.displayName') }}</th><th>{{ $t('common.status') }}</th></tr></thead>
                <tbody>
                  <tr v-for="p in preview.providers" :key="p.providerId">
                    <td>{{ p.displayName }}<details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ p.providerId }} · {{ p.clientProtocol }}</details></td>
                    <td>{{ p.enabled ? $t('common.enabled') : $t('common.disabled') }}</td>
                  </tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>

            <q-expansion-item dense group="preview" :label="`${$t('resources.preview.models')} (${preview.models.length})`" icon="smart_toy">
              <q-markup-table flat dense>
                <thead><tr><th>{{ $t('resources.preview.displayName') }}</th><th>{{ $t('resources.preview.provider') }}</th><th>{{ $t('resources.preview.input') }}</th><th>{{ $t('resources.preview.caps') }}</th><th>{{ $t('common.status') }}</th></tr></thead>
                <tbody>
                  <tr v-for="m in preview.models" :key="m.modelId" data-cy="preview-model-summary">
                    <td>{{ m.displayName }}<details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ m.modelId }} · {{ m.upstreamModelKey }}</details></td>
                    <td>{{ previewReferenceName('provider', m.providerId) }}</td>
                    <td>{{ m.inputModalities.map(value => $t(`resources.model.${value.toLowerCase()}`)).join(', ') }}</td>
                    <td>{{ m.capabilities.map(value => $t(`resources.model.${value.toLowerCase()}`)).join(', ') || $t('common.none') }}</td>
                    <td>{{ m.enabled ? $t('common.enabled') : $t('common.disabled') }}</td>
                  </tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>

            <q-expansion-item dense group="preview" :label="`${$t('resources.tabs.imageGeneration')} (${preview.imageGenerators?.length ?? 0})`" icon="image">
              <q-markup-table flat dense>
                <thead><tr><th>{{ $t('resources.preview.displayName') }}</th><th>{{ $t('resources.imageGeneration.allowedSizes') }}</th><th>{{ $t('common.status') }}</th></tr></thead>
                <tbody>
                  <tr v-for="image in preview.imageGenerators ?? []" :key="image.imageId">
                    <td>{{ image.displayName }}<details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ image.imageId }} · {{ image.clientProtocol }} · {{ image.upstreamModelKey }}</details></td>
                    <td>{{ image.allowedSizes.join(', ') }} · {{ $t('resources.imageGeneration.maxImagesValue', { count: image.maxImagesPerRequest }) }}</td>
                    <td>{{ image.enabled ? $t('common.enabled') : $t('common.disabled') }}</td>
                  </tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>

            <q-expansion-item dense group="preview" :label="`${$t('resources.preview.tts')} (${preview.tts.length})`" icon="record_voice_over">
              <q-markup-table flat dense>
                <thead><tr><th>{{ $t('resources.preview.displayName') }}</th><th>{{ $t('resources.tts.speechProfile') }}</th><th>{{ $t('common.status') }}</th></tr></thead>
                <tbody>
                  <tr v-for="t in preview.tts" :key="t.ttsId">
                    <td>{{ t.displayName }}<details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ t.ttsId }} · {{ t.upstreamModelKey }}</details></td>
                    <td><div>{{ ttsProtocols.find(option => option.value === t.clientProtocol)?.label }}</div><div class="text-caption">{{ ttsSummary(t) }}</div></td><td>{{ t.enabled ? $t('common.enabled') : $t('common.disabled') }}</td>
                  </tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>

            <q-expansion-item dense group="preview" :label="`${$t('resources.preview.asr')} (${preview.asr.length})`" icon="hearing">
              <q-markup-table flat dense>
                <thead><tr><th>{{ $t('resources.preview.displayName') }}</th><th>{{ $t('resources.preview.language') }}</th><th>{{ $t('common.status') }}</th></tr></thead>
                <tbody>
                  <tr v-for="a in preview.asr" :key="a.asrId">
                    <td>{{ a.displayName }}<div class="text-caption">{{ asrProtocols.find(protocol => protocol.value === a.clientProtocol)?.label }}</div><details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ a.asrId }} · {{ a.upstreamModelKey }}<template v-if="a.sampleRate"><br>{{ $t('resources.asr.sampleRate') }}: {{ a.sampleRate }} · {{ $t('resources.asr.vadThreshold') }}: {{ a.vadThreshold }} · {{ $t('resources.asr.silenceDuration') }}: {{ a.silenceDurationMs }}</template><template v-if="a.prefixPaddingMs !== undefined"><br>{{ $t('resources.asr.prefixPadding') }}: {{ a.prefixPaddingMs }}</template><template v-if="a.prompt"><br>{{ $t('resources.asr.prompt') }}: {{ a.prompt }}</template></details></td>
                    <td>{{ a.language || $t('resources.preview.auto') }}</td><td>{{ a.enabled ? $t('common.enabled') : $t('common.disabled') }}</td>
                  </tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>

            <q-expansion-item dense group="preview" :label="`${$t('resources.preview.mcp')} (${preview.mcp.length})`" icon="link">
              <q-markup-table flat dense>
                <thead><tr><th>{{ $t('resources.preview.displayName') }}</th><th>{{ $t('resources.preview.auth') }}</th><th>{{ $t('common.status') }}</th></tr></thead>
                <tbody>
                  <tr v-for="m in preview.mcp" :key="m.mcpServerId">
                    <td>{{ m.displayName }}<details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ m.mcpServerId }}</details></td>
                    <td>{{ $t(`authOwnership.${m.authOwnership}`) }}</td><td>{{ m.enabled ? $t('common.enabled') : $t('common.disabled') }}</td>
                  </tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>


            <q-expansion-item dense group="preview" :label="`${$t('experience.tab')} (${preview.assistants.length})`" icon="assistant">
              <q-list>
                <q-item v-for="a in preview.assistants" :key="a.assistantDefinitionId">
                  <q-item-section>
                    <q-item-label>{{ a.displayName }} <q-badge class="q-ml-xs" :color="a.enabled ? 'positive' : 'grey'" :label="a.enabled ? $t('common.enabled') : $t('common.disabled')" /></q-item-label>
                    <div v-if="a.description" data-cy="preview-assistant-description" class="text-body2 q-mt-xs">{{ a.description }}</div>
                    <q-item-label caption data-cy="preview-assistant-summary">{{ $t('resources.preview.usesModel') }}: {{ previewModelName(a.modelId) }} · MCP: {{ previewMcpNames(a.mcpServerIds) }}</q-item-label>
                    <div class="text-caption text-grey-7 q-mt-xs">{{ $t('resources.preview.instructions') }}</div>
                    <p class="q-my-xs" style="white-space: pre-wrap">{{ a.systemPrompt }}</p>
                    <div v-if="a.memorySeed.length" class="text-caption text-grey-7">{{ $t('resources.preview.memorySeeds') }}</div>
                    <ol class="q-my-xs"><li v-for="(seed, i) in a.memorySeed" :key="i" style="white-space: pre-wrap">{{ seed }}</li></ol>
                    <details class="text-caption text-grey-7"><summary class="cursor-pointer">{{ $t('resources.review.technicalDetails') }}</summary>{{ a.assistantDefinitionId }}</details>
                  </q-item-section>
                </q-item>
              </q-list>
            </q-expansion-item>
            <q-expansion-item dense group="preview" :label="`${$t('experience.starters')} (${preview.starters.length})`" icon="forum">
              <q-list>
                <q-item v-for="s in preview.starters.toSorted((a, b) => a.sortOrder - b.sortOrder || a.starterId.localeCompare(b.starterId))" :key="s.starterId">
                  <q-item-section>
                    <q-item-label>{{ s.title }} <q-badge class="q-ml-xs" :color="s.enabled ? 'positive' : 'grey'" :label="s.enabled ? $t('common.enabled') : $t('common.disabled')" /></q-item-label>
                    <div v-if="s.description" data-cy="preview-starter-description" class="text-body2 q-mt-xs">{{ s.description }}</div>
                    <q-item-label caption>{{ $t('resources.preview.forAssistant') }}: {{ previewAssistantName(s.assistantDefinitionId) }}</q-item-label>
                    <p class="q-my-xs" style="white-space: pre-wrap">{{ s.prompt }}</p>
                    <details class="text-caption text-grey-7"><summary class="cursor-pointer">{{ $t('resources.review.technicalDetails') }}</summary>{{ s.starterId }} · {{ s.sortOrder }}</details>
                  </q-item-section>
                </q-item>
              </q-list>
            </q-expansion-item>
            <q-expansion-item dense group="preview" :label="$t('resources.preview.policy')" icon="policy">
              <q-markup-table flat dense data-cy="preview-policy-summary">
                <tbody>
                  <tr v-for="setting in policySettings" :key="setting.key"><td class="text-grey-7">{{ setting.label }}</td><td>{{ preview.policy[setting.key] ? $t('resources.preview.allowed') : $t('resources.preview.notAllowed') }}</td></tr>
                  <tr><td class="text-grey-7">{{ $t('resources.preview.defaultModel') }}</td><td>{{ previewReferenceName('model', preview.policy.defaultModelId) }}</td></tr>
                  <tr><td class="text-grey-7">{{ $t('resources.preview.defaultImageGeneration') }}</td><td>{{ previewReferenceName('image', preview.policy.defaultImageGenerationId) }}</td></tr>
                  <tr><td class="text-grey-7">{{ $t('resources.preview.defaultTts') }}</td><td>{{ previewReferenceName('tts', preview.policy.defaultTtsId) }}</td></tr>
                  <tr><td class="text-grey-7">{{ $t('resources.preview.defaultAsr') }}</td><td>{{ previewReferenceName('asr', preview.policy.defaultAsrId) }}</td></tr>
                  <tr><td class="text-grey-7">{{ $t('resources.preview.defaultAssistant') }}</td><td>{{ previewReferenceName('assistant', preview.policy.defaultAssistantId) }}</td></tr>
                </tbody>
              </q-markup-table>
              <details class="text-caption text-grey-7 q-pa-sm"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ $t('resources.policy.policyId') }}: {{ preview.policy.policyId }}</details>
            </q-expansion-item>
          </q-card-section>
          <q-card-actions align="right">
            <q-btn flat icon="arrow_back" :label="$t('common.close')" @click="previewOpen = false" />
          </q-card-actions>
        </q-card>
    </template>
    <div v-else class="text-body2 text-grey-7">{{ $t('resources.noDraft') }}</div>
  </q-page>
</template>

<style scoped>
.configuration-workbench {
  display: grid;
  grid-template-columns: minmax(196px, 232px) minmax(0, 1fr);
  gap: 4px;
  align-items: start;
}

.configuration-workbench__detail {
  min-width: 0;
}

.settings-list {
  border-radius: 4px;
  overflow: hidden;
}

.settings-list .q-item {
  min-height: 52px;
}

.resource-detail-back {
  display: none;
}

@media (max-width: 899px) {
  .configuration-workbench {
    grid-template-columns: minmax(0, 1fr);
  }

  .resource-split:not(.resource-split--selected) .resource-split__detail,
  .resource-split--selected .resource-split__collection {
    display: none;
  }

  .resource-detail-back {
    display: inline-flex;
  }
}

@media (max-width: 599px) {
  .configuration-workbench {
    gap: 4px;
  }

  .configuration-workbench__detail :deep(.row.q-gutter-xs),
  .configuration-workbench__detail :deep(.row.q-gutter-xs) {
    align-items: stretch;
  }

  .configuration-workbench__detail :deep(.row.q-gutter-xs > .col),
  .configuration-workbench__detail :deep(.row.q-gutter-xs > .col) {
    flex: 1 0 100%;
  }
}
</style>
