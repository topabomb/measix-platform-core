<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { components } from '../api/generated'
import { apiFetch, createCandidateId } from '../api/client'
import { useDraftStore } from '../stores/draft'
import { useSessionStore } from '../stores/session'
import { useActivationStore } from '../stores/activation'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'

type Activation = components['schemas']['Activation']
type ManagedDraftContent = components['schemas']['ManagedDraftContent']
type ModelDefinition = components['schemas']['ModelDefinition']
type TtsDefinition = components['schemas']['TtsDefinition']
type AsrDefinition = components['schemas']['AsrDefinition']
type McpDefinition = components['schemas']['McpDefinition']
type ProviderDefinition = components['schemas']['ProviderDefinition']
type ManagedPolicy = components['schemas']['ManagedPolicy']
type Upstream = components['schemas']['Upstream']
type UpstreamPage = components['schemas']['UpstreamPage']
type RuntimeBindingDefinition = components['schemas']['RuntimeBindingDefinition']
type TransportPolicy = RuntimeBindingDefinition['transportPolicy']
type DraftPreviewResponse = components['schemas']['DraftPreviewResponse']

const draft = useDraftStore()
const session = useSessionStore()
const activation = useActivationStore()
const error = ref<unknown>()
const publishing = ref(false)
const previewing = ref(false)
const preview = ref<DraftPreviewResponse>()
const previewOpen = ref(false)
const reviewOpen = ref(false)
const reviewing = ref(false)
const upstreams = ref<Upstream[]>([])
const activeTab = ref<'overview' | 'models' | 'tts' | 'asr' | 'mcp' | 'policy'>('overview')
const canMutate = computed(() => Boolean(session.csrfToken))

// Selected resource for editor/detail mode
const selectedResourceId = ref<string>()

const INPUT_MODS = ['TEXT', 'IMAGE'] as const
const OUTPUT_MODS = ['TEXT'] as const
const MODEL_CAPS = ['TOOL', 'REASONING'] as const
const AUTH_OWNERSHIPS = ['ENTERPRISE_MANAGED', 'NONE'] as const
const RELATIONSHIP_KINDS = ['Model', 'TTS', 'ASR', 'MCP'] as const
const relationshipKindFilter = ref<string>('all')

const up = (id?: string) => upstreams.value.find((u) => u.upstreamId === id)
const upstreamLabel = (id?: string) => (id ? up(id)?.name ?? id : '—')
const upstreamStatus = (id?: string) => up(id)?.status

/** Upstream picker options showing Name + status, not just bare ID. */
const upstreamOptions = computed(() =>
  upstreams.value
    .slice()
    .sort((a, b) => (a.status === 'ACTIVE' ? -1 : 0) - (b.status === 'ACTIVE' ? -1 : 0))
    .map((u) => ({
      label: `${u.name} (${u.status})`,
      value: u.upstreamId,
      status: u.status,
    })),
)

/** Selected model for the Models tab editor. */
const selectedModel = computed(() =>
  draft.localContent?.models.find((m) => m.modelId === selectedResourceId.value),
)
const selectedTts = computed(() =>
  draft.localContent?.tts.find((t) => t.ttsId === selectedResourceId.value),
)
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
  for (const t of c.tts) {
    const b = draft.bindingFor(t.ttsId)
    rows.push({
      resourceId: t.ttsId, kind: 'TTS', displayName: t.displayName,
      upstreamId: b?.upstreamId, upstreamName: upstreamLabel(b?.upstreamId),
      upstreamStatus: upstreamStatus(b?.upstreamId),
      enabled: t.enabled, runtimePath: t.runtimePath,
      transport: b?.transportPolicy,
      bindingState: !b ? 'missing' : !b.upstreamId ? 'missing' : 'bound',
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
    label: `${m.displayName} (${m.modelId})`,
    value: m.modelId,
  })) ?? [],
)
const enabledTts = computed(() =>
  draft.localContent?.tts.filter((t) => t.enabled).map((t) => ({
    label: `${t.displayName} (${t.ttsId})`,
    value: t.ttsId,
  })) ?? [],
)
const enabledAsr = computed(() =>
  draft.localContent?.asr.filter((a) => a.enabled).map((a) => ({
    label: `${a.displayName} (${a.asrId})`,
    value: a.asrId,
  })) ?? [],
)

async function refresh() {
  error.value = undefined
  try {
    await Promise.all([draft.load(), loadUpstreams()])
  } catch (cause) {
    error.value = cause
  }
}

async function loadUpstreams() {
  try {
    const page = await apiFetch<UpstreamPage>('/api/admin/v1/upstreams?limit=200')
    upstreams.value = page.items
  } catch {
    // Upstream list is a convenience for binding; failure does not block draft editing.
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
  activation.resetCommand()
  const key = activation.beginCommand('PUBLISH')
  error.value = undefined
  try {
    const result = await apiFetch<Activation>('/api/admin/v1/draft:publish', {
      method: 'POST',
      headers: { 'Idempotency-Key': key },
      body: JSON.stringify({
        expectedDraftRevision: draft.baselineRevision,
        acknowledgedWarningCodes: warnings.map((w) => w.code),
      }),
    }, session.csrfToken)
    activation.accept(result)
    if (result.state === 'APPLYING' || result.state === 'UNKNOWN') {
      await activation.poll(result.activationId)
    }
    reviewOpen.value = false
    await refresh()
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
    error.value = new Error('Add a provider before adding models — every model must bind to a real provider.')
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
    displayName: 'New provider',
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
    error.value = new Error(`Provider ${providerId} is referenced by models and cannot be removed.`)
    return
  }
  content.providers = content.providers.filter((p) => p.providerId !== providerId)
  draft.markDirty()
}

function removeResource(list: string, index: number, resourceId: string) {
  const content = draft.localContent
  if (!content) return
  content[list as keyof ManagedDraftContent] = (content[list as keyof ManagedDraftContent] as unknown[]).filter((_, i) => i !== index) as never
  draft.removeBinding(resourceId)
  if (selectedResourceId.value === resourceId) selectedResourceId.value = undefined
}

/** Toggle the enabled flag of a resource row. */
function toggleEnabled(kind: string, resourceId: string, enabled: boolean) {
  const content = draft.localContent
  if (!content) return
  const list = ({ Model: 'models', TTS: 'tts', ASR: 'asr', MCP: 'mcp' } as Record<string, string>)[kind]
  const arr = content[list as keyof ManagedDraftContent] as { enabled: boolean }[]
  const item = arr.find((r) => (r as unknown as { [k: string]: string }).modelId === resourceId
    || (r as unknown as { [k: string]: string }).ttsId === resourceId
    || (r as unknown as { [k: string]: string }).asrId === resourceId
    || (r as unknown as { [k: string]: string }).mcpServerId === resourceId)
  if (item) {
    item.enabled = enabled
    draft.markDirty()
  }
}

/** Navigate to the editor for a specific resource. */
function goToResource(kind: string, resourceId: string) {
  selectedResourceId.value = resourceId
  const tab = ({ Model: 'models', TTS: 'tts', ASR: 'asr', MCP: 'mcp' } as Record<string, string>)[kind]
  if (tab) activeTab.value = tab as typeof activeTab.value
}

/** Select a model in the editor. */
function selectModel(id: string) { selectedResourceId.value = id }
function selectTts(id: string) { selectedResourceId.value = id }
function selectAsr(id: string) { selectedResourceId.value = id }
function selectMcp(id: string) { selectedResourceId.value = id }

/** Validation issue path for a resource. */
function validationIssuesFor(resourceId: string) {
  const errors = draft.validationResult?.errors.filter((e) => e.path.includes(resourceId)) ?? []
  const warnings = draft.validationResult?.warnings.filter((w) => w.path.includes(resourceId)) ?? []
  return { errors, warnings }
}

/** Structured diff between baseline and local draft content for the Review workspace. */
const reviewDiff = computed(() => {
  const base = draft.baselineContent
  const local = draft.localContent
  if (!base || !local) return null

  function diffList<T extends { modelId?: string; ttsId?: string; asrId?: string; mcpServerId?: string; providerId?: string }>(
    baseArr: T[], localArr: T[], idFn: (item: T) => string,
  ): { added: T[]; changed: { item: T; baseItem?: T }[]; removed: T[] } {
    const baseMap = new Map(baseArr.map((item) => [idFn(item), item]))
    const localMap = new Map(localArr.map((item) => [idFn(item), item]))
    const added: T[] = []
    const changed: { item: T; baseItem?: T }[] = []
    const removed: T[] = []
    for (const item of localArr) {
      const id = idFn(item)
      if (!baseMap.has(id)) added.push(item)
      else changed.push({ item, baseItem: baseMap.get(id) })
    }
    for (const item of baseArr) {
      const id = idFn(item)
      if (!localMap.has(id)) removed.push(item)
    }
    return { added, changed, removed }
  }

  const providers = diffList(base.providers, local.providers, (p) => p.providerId)
  const models = diffList(base.models, local.models, (m) => m.modelId)
  const tts = diffList(base.tts, local.tts, (t) => t.ttsId)
  const asr = diffList(base.asr, local.asr, (a) => a.asrId)
  const mcp = diffList(base.mcp, local.mcp, (m) => m.mcpServerId)
  const bindings = diffList(base.bindings, local.bindings, (b) => b.resourceId)

  // Policy diff
  const policyChanged = JSON.stringify(base.policy) !== JSON.stringify(local.policy)

  return { providers, models, tts, asr, mcp, bindings, policyChanged }
})

/** Runtime routing impact summary for the Review workspace. */
const routingImpact = computed(() => {
  if (!reviewDiff.value) return { added: 0, changed: 0, removed: 0 }
  const b = reviewDiff.value.bindings
  return { added: b.added.length, changed: b.changed.length, removed: b.removed.length }
})

/** Warnings from validation that need acknowledgment before publish. */
const reviewWarnings = computed(() => draft.validationResult?.warnings ?? [])

/** Total resource changes count for review summary. */
const reviewTotalChanges = computed(() => {
  if (!reviewDiff.value) return 0
  const d = reviewDiff.value
  return d.providers.added.length + d.providers.changed.length + d.providers.removed.length
    + d.models.added.length + d.models.changed.length + d.models.removed.length
    + d.tts.added.length + d.tts.changed.length + d.tts.removed.length
    + d.asr.added.length + d.asr.changed.length + d.asr.removed.length
    + d.mcp.added.length + d.mcp.changed.length + d.mcp.removed.length
    + (d.policyChanged ? 1 : 0)
})

/** Has blocking errors that prevent publish. */
const hasBlockingErrors = computed(() => (draft.validationResult?.errors.length ?? 0) > 0)

onMounted(refresh)
</script>

<template>
  <q-page padding>
    <PageHeader title="Resources" subtitle="Edit the managed capability draft: models, TTS, ASR, MCP, policy and upstream bindings.">
      <template #primary>
        <q-btn color="positive" icon="rocket_launch" label="Review & Publish" :disable="!canMutate || draft.dirty" :loading="reviewing" @click="openReview" />
      </template>
      <template #actions>
        <q-btn flat icon="refresh" :loading="draft.loading" @click="refresh" />
        <q-btn outline color="secondary" label="Preview" :disable="!canMutate" :loading="previewing" @click="previewSnapshot" />
        <q-btn outline color="primary" label="Validate" :disable="!canMutate || draft.loading" @click="validate" />
        <q-btn outline color="primary" label="Save" :disable="!canMutate || !draft.dirty" :loading="draft.saving" @click="save" />
      </template>
    </PageHeader>

    <ProblemBanner :error="error" class="q-mb-md" />
    <q-banner v-if="draft.conflictRevision !== undefined" class="bg-orange-1 q-mb-md rounded-borders">
      <div class="text-weight-medium">Stale draft (revision {{ draft.baselineRevision }})</div>
      <div class="text-body2">Server is at revision {{ draft.conflictRevision }}. Reload to pick up the latest changes.</div>
      <template #action><q-btn flat label="Reload" @click="refresh" /></template>
    </q-banner>
    <q-banner v-if="activation.activation" :class="activation.succeeded ? 'bg-green-1' : 'bg-orange-1'" class="q-mb-md rounded-borders">
      <div class="row items-center justify-between">
        <span>Activation {{ activation.activation.activationId }} ({{ activation.activation.kind }})</span>
        <StatusChip :value="activation.activation.state" />
      </div>
      <div v-if="activation.activation.errorCode" class="text-caption text-negative">{{ activation.activation.errorCode }}</div>
      <!-- Publish progress stages -->
      <div v-if="activation.activation.kind === 'PUBLISH' && activation.publishStages.length" class="row items-center q-gutter-xs q-mt-sm">
        <template v-for="stage in activation.publishStages" :key="stage.key">
          <div class="column items-center" style="min-width: 80px">
            <q-icon
              :name="stage.icon"
              :color="stage.status === 'done' ? 'positive' : stage.status === 'active' ? 'primary' : stage.status === 'failed' ? 'negative' : 'grey'"
              :size="stage.status === 'active' ? '2rem' : '1.5rem'"
            />
            <div class="text-caption" :class="stage.status === 'pending' ? 'text-grey-6' : ''">{{ stage.label }}</div>
          </div>
          <q-separator v-if="stage !== activation.publishStages[activation.publishStages.length - 1]" vertical class="q-mx-sm" style="height: 20px" />
        </template>
      </div>
      <div class="text-caption text-grey-7 q-mt-xs">Recovery: refresh the page to recover this activation — no duplicate command will be sent.</div>
    </q-banner>

    <LoadingState v-if="draft.loading" />
    <template v-else-if="draft.localContent">
      <!-- Draft identity + status -->
      <q-card flat bordered class="q-mb-md">
        <q-card-section class="row items-center justify-between">
          <div class="text-subtitle1">
            Revision {{ draft.baselineRevision }}
            <q-badge v-if="draft.dirty" color="orange" label="unsaved" class="q-ml-sm" />
          </div>
          <div class="row items-center q-gutter-sm">
            <q-chip v-if="draft.validationResult?.errors.length" color="negative" outline dense icon="error">
              {{ draft.validationResult.errors.length }} error(s)
            </q-chip>
            <q-chip v-else-if="draft.validationResult?.warnings.length" color="amber" outline dense icon="warning">
              {{ draft.validationResult.warnings.length }} warning(s)
            </q-chip>
            <q-chip v-else-if="draft.validationResult" color="positive" outline dense icon="check_circle">
              valid
            </q-chip>
          </div>
        </q-card-section>
      </q-card>

      <!-- Tabbed resource editors -->
      <q-tabs v-model="activeTab" class="q-mb-md" dense align="left">
        <q-tab name="overview" label="Overview" icon="account_tree" />
        <q-tab name="models" label="Models" icon="smart_toy">
          <q-badge v-if="draft.localContent.models.length" color="primary" rounded floating :label="draft.localContent.models.length" />
        </q-tab>
        <q-tab name="tts" label="TTS" icon="record_voice_over">
          <q-badge v-if="draft.localContent.tts.length" color="teal" rounded floating :label="draft.localContent.tts.length" />
        </q-tab>
        <q-tab name="asr" label="ASR" icon="hearing">
          <q-badge v-if="draft.localContent.asr.length" color="indigo" rounded floating :label="draft.localContent.asr.length" />
        </q-tab>
        <q-tab name="mcp" label="MCP" icon="link">
          <q-badge v-if="draft.localContent.mcp.length" color="deep-purple" rounded floating :label="draft.localContent.mcp.length" />
        </q-tab>
        <q-tab name="policy" label="Policy" icon="policy" />
      </q-tabs>

      <!-- ===== Overview: relationship view + providers ===== -->
      <template v-if="activeTab === 'overview'">
        <!-- Providers section -->
        <q-card flat bordered class="q-mb-md">
          <q-card-section>
            <div class="row items-center justify-between">
              <div class="text-subtitle2">Providers</div>
              <q-btn flat dense icon="add" label="Add provider" size="sm" @click="addProvider()" />
            </div>
            <q-list dense class="q-mt-sm">
              <q-item v-for="provider in draft.localContent.providers" :key="provider.providerId">
                <q-item-section>
                  <q-input v-model="provider.displayName" dense outlined label="Display name" @update:model-value="draft.markDirty()" />
                  <div class="text-caption text-grey-7">{{ provider.providerId }} · {{ provider.clientProtocol }} · {{ modelCountForProvider(provider.providerId) }} model(s)</div>
                </q-item-section>
                <q-item-section side>
                  <q-toggle v-model="provider.enabled" @update:model-value="draft.markDirty()" />
                  <q-btn flat dense color="negative" icon="delete" size="sm" :disable="modelCountForProvider(provider.providerId) > 0" @click="removeProvider(provider.providerId)" />
                </q-item-section>
              </q-item>
              <q-item v-if="!draft.localContent.providers.length"><q-item-section class="text-grey-7">No providers. Add one before creating models.</q-item-section></q-item>
            </q-list>
          </q-card-section>
        </q-card>

        <!-- Relationship view with kind filter -->
        <q-card flat bordered>
          <q-card-section>
            <div class="row items-center justify-between q-mb-sm">
              <div class="text-subtitle2">Resource → Upstream relationships</div>
              <q-btn-toggle
                v-model="relationshipKindFilter"
                dense flat
                :options="[
                  { label: 'All', value: 'all' },
                  { label: 'Model', value: 'Model' },
                  { label: 'TTS', value: 'TTS' },
                  { label: 'ASR', value: 'ASR' },
                  { label: 'MCP', value: 'MCP' },
                ]"
              />
            </div>
            <q-markup-table flat dense>
              <thead>
                <tr>
                  <th>Kind</th>
                  <th>Resource</th>
                  <th>Upstream</th>
                  <th>Runtime Path</th>
                  <th>Transport</th>
                  <th class="text-right">Status</th>
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
                      <q-badge color="red" label="missing binding" />
                      <div class="text-caption text-grey-7 q-mt-xs">Click to bind upstream</div>
                    </template>
                  </td>
                  <td><code class="text-caption">{{ row.runtimePath || '—' }}</code></td>
                  <td><span class="text-caption">{{ row.transport || '—' }}</span></td>
                  <td class="text-right">
                    <div class="row items-center justify-end q-gutter-xs">
                      <q-badge v-if="!row.enabled" color="grey" label="disabled" />
                      <q-badge v-else-if="row.bindingState === 'missing'" color="red" label="no binding" />
                      <q-badge v-else-if="row.upstreamStatus && row.upstreamStatus !== 'ACTIVE'" color="orange" label="degraded upstream" />
                      <q-badge v-else color="green" label="bound" />
                      <q-toggle data-testid="relationship-enable-toggle" :model-value="row.enabled" :label="row.enabled ? 'on' : 'off'" @update:model-value="(v: boolean) => toggleEnabled(row.kind, row.resourceId, v)" @click.stop />
                    </div>
                  </td>
                </tr>
                <tr v-if="!filteredRelationshipRows.length"><td colspan="6" class="text-grey-7">No managed resources of this kind yet.</td></tr>
              </tbody>
            </q-markup-table>
          </q-card-section>
        </q-card>
      </template>

      <!-- ===== Models: Collection → Editor ===== -->
      <template v-if="activeTab === 'models'">
        <div class="row q-col-gutter-md">
          <!-- Collection -->
          <div class="col-12 col-md-4">
            <q-card flat bordered>
              <q-card-section class="row items-center justify-between">
                <div class="text-subtitle2">Models</div>
                <q-btn flat dense icon="add" label="Add" size="sm" :disable="!draft.localContent.providers.length" @click="addModel()" />
              </q-card-section>
              <q-list separator>
                <q-item v-for="(model, idx) in draft.localContent.models" :key="model.modelId"
                  :active="selectedResourceId === model.modelId" clickable @click="selectModel(model.modelId)">
                  <q-item-section>
                    <q-item-label>{{ model.displayName }}</q-item-label>
                    <q-item-label caption>
                      {{ model.modelId }}
                      · <q-badge v-for="m in model.inputModalities" :key="m" dense color="primary" :label="m" class="q-mr-xs" />
                      · {{ model.upstreamModelKey || 'no key' }}
                    </q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-xs">
                      <q-badge v-if="!draft.bindingFor(model.modelId)?.upstreamId" color="red" label="no binding" />
                      <q-badge v-if="!model.enabled" color="grey" label="off" />
                      <q-btn flat dense color="negative" icon="delete" size="sm" @click.stop="removeResource('models', idx, model.modelId)" />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="!draft.localContent.models.length"><q-item-section class="text-grey-7">No models. Add one to get started.</q-item-section></q-item>
              </q-list>
            </q-card>
          </div>

          <!-- Model Editor -->
          <div class="col-12 col-md-8">
            <q-card v-if="selectedModel" flat bordered>
              <!-- Header: Identity -->
              <q-card-section class="row items-start justify-between">
                <div>
                  <div class="text-h6">{{ selectedModel.displayName }}</div>
                  <div class="text-caption text-grey-7">{{ selectedModel.modelId }} (read-only system identity)</div>
                </div>
                <div class="row items-center q-gutter-sm">
                  <q-badge v-if="draft.dirty" color="orange" label="unsaved" />
                  <q-toggle v-model="selectedModel.enabled" label="Enabled" @update:model-value="draft.markDirty()" />
                </div>
              </q-card-section>
              <q-separator />

              <!-- Identity -->
              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">Identity</div>
                <div class="row q-gutter-sm">
                  <q-input v-model="selectedModel.displayName" dense outlined label="Display Name" class="col" @update:model-value="draft.markDirty()" />
                  <q-select v-model="selectedModel.providerId" dense outlined label="Provider" :options="draft.localContent.providers.map((p) => ({ label: p.displayName, value: p.providerId }))" emit-value map-options class="col" @update:model-value="draft.markDirty()" />
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">{{ selectedModel.modelId }} (logical mdl_* identity — created automatically)</div>
              </q-card-section>
              <q-separator />

              <!-- Capability -->
              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">Capability</div>
                <div class="row q-gutter-sm q-mb-sm">
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">Protocol</div>
                    <q-badge color="primary" label="OPENAI_CHAT_COMPLETIONS" />
                  </div>
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">Upstream Model Key</div>
                    <q-input v-model="selectedModel.upstreamModelKey" dense outlined label="Upstream model key" hint="The upstream provider's model identifier" @update:model-value="draft.markDirty()" />
                  </div>
                </div>
                <div class="row q-gutter-sm">
                  <q-select v-model="selectedModel.inputModalities" dense outlined label="Input Modalities" multiple :options="[...INPUT_MODS]" class="col" emit-value map-options @update:model-value="draft.markDirty()" />
                  <q-select v-model="selectedModel.outputModalities" dense outlined label="Output Modalities" multiple :options="[...OUTPUT_MODS]" class="col" emit-value map-options @update:model-value="draft.markDirty()" />
                  <q-select v-model="selectedModel.capabilities" dense outlined label="Capabilities" multiple :options="[...MODEL_CAPS]" class="col" emit-value map-options @update:model-value="draft.markDirty()" />
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">Use checkbox/chip controls for capabilities — no comma-separated strings.</div>
              </q-card-section>
              <q-separator />

              <!-- Execution -->
              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">Execution / Binding</div>
                <div class="row q-gutter-sm">
                  <q-select :model-value="draft.bindingFor(selectedModel.modelId)?.upstreamId ?? ''" dense outlined label="Upstream" :options="upstreamOptions" emit-value map-options class="col" @update:model-value="(v: string) => draft.setBinding(selectedModel!.modelId, v, 'HTTP_STREAMING_SSE')" />
                  <q-input v-model="selectedModel.runtimePath" dense outlined label="Runtime Path" class="col" @update:model-value="draft.markDirty()" />
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">
                  Transport summary: HTTP + SSE (streaming chat completions)
                </div>
                <q-banner v-if="!draft.bindingFor(selectedModel.modelId)?.upstreamId" class="bg-red-1 q-mt-sm rounded-borders">
                  <div class="text-body2 text-negative">Missing upstream binding — this resource cannot be published when enabled.</div>
                </q-banner>
              </q-card-section>
              <q-separator />

              <!-- Validation -->
              <q-card-section v-if="draft.validationResult">
                <div class="text-subtitle2 q-mb-sm">Validation</div>
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
                    <q-item-section class="text-positive">No issues for this resource.</q-item-section>
                  </q-item>
                </q-list>
              </q-card-section>
            </q-card>
            <q-card v-else flat bordered>
              <q-card-section class="text-grey-7 text-center">
                <q-icon name="smart_toy" size="3rem" />
                <div class="text-body2 q-mt-sm">Select a model from the list or add a new one.</div>
              </q-card-section>
            </q-card>
          </div>
        </div>
      </template>

      <!-- ===== TTS: Collection → Editor ===== -->
      <template v-if="activeTab === 'tts'">
        <div class="row q-col-gutter-md">
          <div class="col-12 col-md-4">
            <q-card flat bordered>
              <q-card-section class="row items-center justify-between">
                <div class="text-subtitle2">TTS <span class="text-caption text-grey-7">· OpenAI Audio Speech</span></div>
                <q-btn flat dense icon="add" label="Add" size="sm" @click="draft.addTts(); selectedResourceId = draft.localContent?.tts[draft.localContent.tts.length - 1]?.ttsId" />
              </q-card-section>
              <q-list separator>
                <q-item v-for="(tts, idx) in draft.localContent.tts" :key="tts.ttsId"
                  :active="selectedResourceId === tts.ttsId" clickable @click="selectTts(tts.ttsId)">
                  <q-item-section>
                    <q-item-label>{{ tts.displayName }}</q-item-label>
                    <q-item-label caption>{{ tts.ttsId }} · {{ tts.voice || 'no voice' }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-xs">
                      <q-badge v-if="!draft.bindingFor(tts.ttsId)?.upstreamId" color="red" label="no binding" />
                      <q-badge v-if="!tts.voice" color="red" label="no voice" />
                      <q-btn flat dense color="negative" icon="delete" size="sm" @click.stop="removeResource('tts', idx, tts.ttsId)" />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="!draft.localContent.tts.length"><q-item-section class="text-grey-7">No TTS. Add one to get started.</q-item-section></q-item>
              </q-list>
            </q-card>
          </div>

          <div class="col-12 col-md-8">
            <q-card v-if="selectedTts" flat bordered>
              <q-card-section class="row items-start justify-between">
                <div>
                  <div class="text-h6">{{ selectedTts.displayName }}</div>
                  <div class="text-caption text-grey-7">{{ selectedTts.ttsId }}</div>
                </div>
                <q-toggle v-model="selectedTts.enabled" label="Enabled" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">Identity</div>
                <q-input v-model="selectedTts.displayName" dense outlined label="Display Name" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">Speech Profile</div>
                <div class="row q-gutter-sm q-mb-sm">
                  <q-input v-model="selectedTts.upstreamModelKey" dense outlined label="Model Key" hint="Upstream TTS model identifier" class="col" @update:model-value="draft.markDirty()" />
                  <q-input v-model="selectedTts.voice" dense outlined label="Voice" hint="Required field — e.g. alloy, echo, nova" class="col"
                    :rules="[(v: string) => !!v || 'Voice is required']"
                    @update:model-value="draft.markDirty()" />
                </div>
                <div class="row q-gutter-sm items-center">
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">Protocol</div>
                    <q-badge color="teal" label="OPENAI_AUDIO_SPEECH" />
                  </div>
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">Output baseline</div>
                    <q-badge color="grey" label="MP3 (binary response)" />
                  </div>
                </div>
                <q-banner v-if="!selectedTts.voice" class="bg-red-1 q-mt-sm rounded-borders">
                  <div class="text-body2 text-negative">Voice is required for enabled TTS.</div>
                </q-banner>
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">Execution / Binding</div>
                <div class="row q-gutter-sm">
                  <q-select :model-value="draft.bindingFor(selectedTts.ttsId)?.upstreamId ?? ''" dense outlined label="Upstream" :options="upstreamOptions" emit-value map-options class="col" @update:model-value="(v: string) => draft.setBinding(selectedTts!.ttsId, v, 'HTTP_BINARY_STREAM')" />
                  <q-input v-model="selectedTts.runtimePath" dense outlined label="Runtime Path" class="col" @update:model-value="draft.markDirty()" />
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">Transport summary: HTTP request → binary audio response</div>
              </q-card-section>
              <q-separator />

              <q-card-section v-if="draft.validationResult">
                <div class="text-subtitle2 q-mb-sm">Validation</div>
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
                    <q-item-section class="text-positive">No issues for this resource.</q-item-section>
                  </q-item>
                </q-list>
              </q-card-section>
            </q-card>
            <q-card v-else flat bordered>
              <q-card-section class="text-grey-7 text-center">
                <q-icon name="record_voice_over" size="3rem" />
                <div class="text-body2 q-mt-sm">Select a TTS from the list or add a new one.</div>
              </q-card-section>
            </q-card>
          </div>
        </div>
      </template>

      <!-- ===== ASR: Collection → Editor ===== -->
      <template v-if="activeTab === 'asr'">
        <div class="row q-col-gutter-md">
          <div class="col-12 col-md-4">
            <q-card flat bordered>
              <q-card-section class="row items-center justify-between">
                <div class="text-subtitle2">ASR <span class="text-caption text-grey-7">· OpenAI Audio Transcriptions (HTTP)</span></div>
                <q-btn flat dense icon="add" label="Add" size="sm" @click="draft.addAsr(); selectedResourceId = draft.localContent?.asr[draft.localContent.asr.length - 1]?.asrId" />
              </q-card-section>
              <q-list separator>
                <q-item v-for="(asr, idx) in draft.localContent.asr" :key="asr.asrId"
                  :active="selectedResourceId === asr.asrId" clickable @click="selectAsr(asr.asrId)">
                  <q-item-section>
                    <q-item-label>{{ asr.displayName }}</q-item-label>
                    <q-item-label caption>{{ asr.asrId }} · {{ asr.language || 'any language' }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-xs">
                      <q-badge v-if="!draft.bindingFor(asr.asrId)?.upstreamId" color="red" label="no binding" />
                      <q-btn flat dense color="negative" icon="delete" size="sm" @click.stop="removeResource('asr', idx, asr.asrId)" />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="!draft.localContent.asr.length"><q-item-section class="text-grey-7">No ASR. Add one to get started.</q-item-section></q-item>
              </q-list>
            </q-card>
          </div>

          <div class="col-12 col-md-8">
            <q-card v-if="selectedAsr" flat bordered>
              <q-card-section class="row items-start justify-between">
                <div>
                  <div class="text-h6">{{ selectedAsr.displayName }}</div>
                  <div class="text-caption text-grey-7">{{ selectedAsr.asrId }}</div>
                </div>
                <q-toggle v-model="selectedAsr.enabled" label="Enabled" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">Identity</div>
                <q-input v-model="selectedAsr.displayName" dense outlined label="Display Name" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">Transcription Profile</div>
                <div class="row q-gutter-sm q-mb-sm">
                  <q-input v-model="selectedAsr.upstreamModelKey" dense outlined label="Model Key" hint="Upstream ASR model identifier" class="col" @update:model-value="draft.markDirty()" />
                  <q-input v-model="selectedAsr.language" dense outlined label="Optional Language" hint="Leave empty for auto-detect" class="col" @update:model-value="draft.markDirty()" />
                </div>
                <div class="row q-gutter-sm items-center">
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">Protocol</div>
                    <q-badge color="indigo" label="OPENAI_AUDIO_TRANSCRIPTIONS" />
                  </div>
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">Transport</div>
                    <q-badge color="grey" label="HTTP multipart transcription" />
                  </div>
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">No realtime/WebSocket/VAD/streaming ASR fields — not supported in S0.1.</div>
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">Execution / Binding</div>
                <div class="row q-gutter-sm">
                  <q-select :model-value="draft.bindingFor(selectedAsr.asrId)?.upstreamId ?? ''" dense outlined label="Upstream" :options="upstreamOptions" emit-value map-options class="col" @update:model-value="(v: string) => draft.setBinding(selectedAsr!.asrId, v, 'HTTP_MULTIPART')" />
                  <q-input v-model="selectedAsr.runtimePath" dense outlined label="Runtime Path" class="col" @update:model-value="draft.markDirty()" />
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">Transport summary: multipart HTTP transcription</div>
              </q-card-section>
              <q-separator />

              <q-card-section v-if="draft.validationResult">
                <div class="text-subtitle2 q-mb-sm">Validation</div>
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
                    <q-item-section class="text-positive">No issues for this resource.</q-item-section>
                  </q-item>
                </q-list>
              </q-card-section>
            </q-card>
            <q-card v-else flat bordered>
              <q-card-section class="text-grey-7 text-center">
                <q-icon name="hearing" size="3rem" />
                <div class="text-body2 q-mt-sm">Select an ASR from the list or add a new one.</div>
              </q-card-section>
            </q-card>
          </div>
        </div>
      </template>

      <!-- ===== MCP: Collection → Editor ===== -->
      <template v-if="activeTab === 'mcp'">
        <div class="row q-col-gutter-md">
          <div class="col-12 col-md-4">
            <q-card flat bordered>
              <q-card-section class="row items-center justify-between">
                <div class="text-subtitle2">MCP <span class="text-caption text-grey-7">· MCP Streamable HTTP</span></div>
                <q-btn flat dense icon="add" label="Add" size="sm" @click="draft.addMcp(); selectedResourceId = draft.localContent?.mcp[draft.localContent.mcp.length - 1]?.mcpServerId" />
              </q-card-section>
              <q-list separator>
                <q-item v-for="(mcp, idx) in draft.localContent.mcp" :key="mcp.mcpServerId"
                  :active="selectedResourceId === mcp.mcpServerId" clickable @click="selectMcp(mcp.mcpServerId)">
                  <q-item-section>
                    <q-item-label>{{ mcp.displayName }}</q-item-label>
                    <q-item-label caption>{{ mcp.mcpServerId }} · {{ mcp.authOwnership }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-xs">
                      <q-badge v-if="!draft.bindingFor(mcp.mcpServerId)?.upstreamId" color="red" label="no binding" />
                      <q-btn flat dense color="negative" icon="delete" size="sm" @click.stop="removeResource('mcp', idx, mcp.mcpServerId)" />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="!draft.localContent.mcp.length"><q-item-section class="text-grey-7">No MCP. Add one to get started.</q-item-section></q-item>
              </q-list>
            </q-card>
          </div>

          <div class="col-12 col-md-8">
            <q-card v-if="selectedMcp" flat bordered>
              <q-card-section class="row items-start justify-between">
                <div>
                  <div class="text-h6">{{ selectedMcp.displayName }}</div>
                  <div class="text-caption text-grey-7">{{ selectedMcp.mcpServerId }}</div>
                </div>
                <q-toggle v-model="selectedMcp.enabled" label="Enabled" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">Identity</div>
                <q-input v-model="selectedMcp.displayName" dense outlined label="Display Name" @update:model-value="draft.markDirty()" />
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">MCP Profile</div>
                <div class="row q-gutter-sm items-center q-mb-sm">
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">Protocol</div>
                    <q-badge color="deep-purple" label="MCP_STREAMABLE_HTTP" />
                  </div>
                  <div class="col">
                    <div class="text-caption text-grey-7 q-mb-xs">Auth Ownership</div>
                    <q-select v-model="selectedMcp.authOwnership" dense outlined label="Auth ownership" :options="[...AUTH_OWNERSHIPS]" @update:model-value="draft.markDirty()" />
                  </div>
                </div>
                <q-banner v-if="selectedMcp.authOwnership === 'ENTERPRISE_MANAGED'" class="bg-blue-1 q-mt-sm rounded-borders">
                  <div class="text-body2 text-info">Credentials are managed server-side via the Upstream/Secret binding. The client never receives the Secret.</div>
                </q-banner>
                <q-banner v-else class="bg-grey-1 q-mt-sm rounded-borders">
                  <div class="text-body2 text-grey-7">No authentication required for this MCP server.</div>
                </q-banner>
              </q-card-section>
              <q-separator />

              <q-card-section>
                <div class="text-subtitle2 q-mb-sm">Execution / Binding</div>
                <div class="row q-gutter-sm">
                  <q-select :model-value="draft.bindingFor(selectedMcp.mcpServerId)?.upstreamId ?? ''" dense outlined label="Upstream" :options="upstreamOptions" emit-value map-options class="col" @update:model-value="(v: string) => draft.setBinding(selectedMcp!.mcpServerId, v, 'HTTP_REQUEST_RESPONSE')" />
                  <q-input v-model="selectedMcp.runtimePath" dense outlined label="Runtime Path" class="col" @update:model-value="draft.markDirty()" />
                </div>
                <div class="text-caption text-grey-7 q-mt-xs">Transport summary: Streamable HTTP</div>
              </q-card-section>
              <q-separator />

              <q-card-section v-if="draft.validationResult">
                <div class="text-subtitle2 q-mb-sm">Validation</div>
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
                    <q-item-section class="text-positive">No issues for this resource.</q-item-section>
                  </q-item>
                </q-list>
              </q-card-section>
            </q-card>
            <q-card v-else flat bordered>
              <q-card-section class="text-grey-7 text-center">
                <q-icon name="link" size="3rem" />
                <div class="text-body2 q-mt-sm">Select an MCP from the list or add a new one.</div>
              </q-card-section>
            </q-card>
          </div>
        </div>
      </template>

      <!-- ===== Policy Editor ===== -->
      <template v-if="activeTab === 'policy'">
        <q-card flat bordered>
          <q-card-section class="row items-center justify-between">
            <div>
              <div class="text-subtitle2">Policy</div>
              <div class="text-caption text-grey-7">Policy ID: {{ draft.localContent.policy.policyId }}</div>
            </div>
            <q-badge v-if="draft.dirty" color="orange" label="unsaved" />
          </q-card-section>
          <q-separator />

          <q-card-section>
            <div class="text-subtitle2 q-mb-sm">Local Coexistence</div>
            <div class="text-body2 text-grey-7 q-mb-md">Controls whether client local capabilities can coexist with managed capabilities.</div>
            <div class="row q-gutter-md">
              <q-toggle v-model="draft.localContent.policy.allowLocalProviders" label="Allow Local Models" @update:model-value="draft.markDirty()" />
              <q-toggle v-model="draft.localContent.policy.allowLocalTts" label="Allow Local TTS" @update:model-value="draft.markDirty()" />
              <q-toggle v-model="draft.localContent.policy.allowLocalAsr" label="Allow Local ASR" @update:model-value="draft.markDirty()" />
              <q-toggle v-model="draft.localContent.policy.allowLocalMcp" label="Allow Local MCP" @update:model-value="draft.markDirty()" />
            </div>
          </q-card-section>
          <q-separator />

          <q-card-section>
            <div class="text-subtitle2 q-mb-sm">Defaults</div>
            <div class="text-body2 text-grey-7 q-mb-md">Only enabled and valid resources can be set as defaults.</div>
            <div class="row q-col-gutter-md">
              <div class="col-12 col-md-4">
                <q-select v-model="draft.localContent.policy.defaultModelId" dense outlined label="Default Model" :options="enabledModels" emit-value map-options clearable @update:model-value="draft.markDirty()" />
                <div class="text-caption text-grey-7 q-mt-xs">Only enabled models appear here.</div>
              </div>
              <div class="col-12 col-md-4">
                <q-select v-model="draft.localContent.policy.defaultTtsId" dense outlined label="Default TTS" :options="enabledTts" emit-value map-options clearable @update:model-value="draft.markDirty()" />
                <div class="text-caption text-grey-7 q-mt-xs">Only enabled TTS appear here.</div>
              </div>
              <div class="col-12 col-md-4">
                <q-select v-model="draft.localContent.policy.defaultAsrId" dense outlined label="Default ASR" :options="enabledAsr" emit-value map-options clearable @update:model-value="draft.markDirty()" />
                <div class="text-caption text-grey-7 q-mt-xs">Only enabled ASR appear here.</div>
              </div>
            </div>
          </q-card-section>
        </q-card>
      </template>

      <!-- Shared validation summary (below all tabs) -->
      <q-card flat bordered class="q-mt-md">
        <q-card-section>
          <div class="text-subtitle2">Validation</div>
          <div v-if="!draft.validationResult" class="text-body2 text-grey-7 q-mt-sm">Click Validate to check the draft.</div>
          <div v-else>
            <q-banner v-if="draft.validationResult.errors.length === 0 && draft.validationResult.warnings.length === 0" class="bg-green-1 q-mt-sm rounded-borders">
              Draft is valid.
            </q-banner>
            <q-banner v-else class="bg-orange-1 q-mt-sm rounded-borders">
              <div>{{ draft.validationResult.errors.length }} errors · {{ draft.validationResult.warnings.length }} warnings</div>
            </q-banner>
            <q-list dense class="q-mt-sm">
              <q-item v-for="e in draft.validationResult.errors" :key="e.path + e.code">
                <q-item-section avatar><q-icon name="error" color="negative" /></q-item-section>
                <q-item-section>{{ e.path }}: {{ e.code }} — {{ e.message }}</q-item-section>
              </q-item>
              <q-item v-for="w in draft.validationResult.warnings" :key="w.path + w.code">
                <q-item-section avatar><q-icon name="warning" color="amber-8" /></q-item-section>
                <q-item-section>{{ w.path }}: {{ w.code }} — {{ w.message }}</q-item-section>
              </q-item>
            </q-list>
          </div>
        </q-card-section>
      </q-card>

      <!-- Review & Publish Dialog (structured diff, not a simple confirm) -->
      <q-dialog v-model="reviewOpen" persistent>
        <q-card style="min-width: 800px; max-width: 95vw">
          <q-card-section class="row items-center justify-between">
            <div>
              <div class="text-h6">Review & Publish</div>
              <div class="text-caption text-grey-7">Revision {{ draft.baselineRevision }} → new immutable release</div>
            </div>
            <q-badge v-if="reviewTotalChanges === 0" color="grey" label="no changes" />
            <q-badge v-else color="primary" :label="`${reviewTotalChanges} change(s)`" />
          </q-card-section>
          <q-separator />

          <q-card-section v-if="reviewDiff" style="max-height: 60vh; overflow-y: auto">
            <!-- Blocking errors -->
            <q-banner v-if="hasBlockingErrors" class="bg-red-1 q-mb-md rounded-borders">
              <div class="text-weight-medium text-negative">{{ draft.validationResult!.errors.length }} blocking error(s) — cannot publish.</div>
              <q-list dense class="q-mt-sm">
                <q-item v-for="e in draft.validationResult!.errors" :key="e.path + e.code">
                  <q-item-section avatar><q-icon name="error" color="negative" /></q-item-section>
                  <q-item-section>
                    <span class="text-negative">{{ e.code }} — {{ e.message }}</span>
                    <span class="text-caption text-grey-7">{{ e.path }}</span>
                  </q-item-section>
                </q-item>
              </q-list>
            </q-banner>

            <!-- Resource changes: Added / Changed / Removed -->
            <div class="text-subtitle2 q-mb-sm">Resource changes</div>
            <q-markup-table flat dense class="q-mb-md">
              <thead>
                <tr>
                  <th>Kind</th>
                  <th class="text-right text-positive">Added</th>
                  <th class="text-right text-warning">Changed</th>
                  <th class="text-right text-negative">Removed</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in [
                  { kind: 'Providers', d: reviewDiff.providers },
                  { kind: 'Models', d: reviewDiff.models },
                  { kind: 'TTS', d: reviewDiff.tts },
                  { kind: 'ASR', d: reviewDiff.asr },
                  { kind: 'MCP', d: reviewDiff.mcp },
                ]" :key="row.kind">
                  <td>{{ row.kind }}</td>
                  <td class="text-right text-positive">{{ row.d.added.length > 0 ? '+' + row.d.added.length : '—' }}</td>
                  <td class="text-right text-warning">{{ row.d.changed.length > 0 ? '~' + row.d.changed.length : '—' }}</td>
                  <td class="text-right text-negative">{{ row.d.removed.length > 0 ? '-' + row.d.removed.length : '—' }}</td>
                </tr>
                <tr v-if="reviewDiff.policyChanged">
                  <td>Policy</td>
                  <td class="text-right">—</td>
                  <td class="text-right text-warning">~1</td>
                  <td class="text-right">—</td>
                </tr>
              </tbody>
            </q-markup-table>

            <!-- Added resource details -->
            <template v-if="reviewDiff.providers.added.length || reviewDiff.models.added.length || reviewDiff.tts.added.length || reviewDiff.asr.added.length || reviewDiff.mcp.added.length">
              <div class="text-subtitle2 q-mb-sm text-positive">Added resources</div>
              <q-list dense class="q-mb-md">
                <q-item v-for="p in reviewDiff.providers.added" :key="p.providerId">
                  <q-item-section avatar><q-icon name="add_circle" color="positive" /></q-item-section>
                  <q-item-section>Provider: {{ p.displayName }} ({{ p.providerId }})</q-item-section>
                </q-item>
                <q-item v-for="m in reviewDiff.models.added" :key="m.modelId">
                  <q-item-section avatar><q-icon name="add_circle" color="positive" /></q-item-section>
                  <q-item-section>Model: {{ m.displayName }} ({{ m.modelId }})</q-item-section>
                </q-item>
                <q-item v-for="t in reviewDiff.tts.added" :key="t.ttsId">
                  <q-item-section avatar><q-icon name="add_circle" color="positive" /></q-item-section>
                  <q-item-section>TTS: {{ t.displayName }} ({{ t.ttsId }})</q-item-section>
                </q-item>
                <q-item v-for="a in reviewDiff.asr.added" :key="a.asrId">
                  <q-item-section avatar><q-icon name="add_circle" color="positive" /></q-item-section>
                  <q-item-section>ASR: {{ a.displayName }} ({{ a.asrId }})</q-item-section>
                </q-item>
                <q-item v-for="m in reviewDiff.mcp.added" :key="m.mcpServerId">
                  <q-item-section avatar><q-icon name="add_circle" color="positive" /></q-item-section>
                  <q-item-section>MCP: {{ m.displayName }} ({{ m.mcpServerId }})</q-item-section>
                </q-item>
              </q-list>
            </template>

            <!-- Removed resource details -->
            <template v-if="reviewDiff.providers.removed.length || reviewDiff.models.removed.length || reviewDiff.tts.removed.length || reviewDiff.asr.removed.length || reviewDiff.mcp.removed.length">
              <div class="text-subtitle2 q-mb-sm text-negative">Removed resources</div>
              <q-list dense class="q-mb-md">
                <q-item v-for="p in reviewDiff.providers.removed" :key="p.providerId">
                  <q-item-section avatar><q-icon name="remove_circle" color="negative" /></q-item-section>
                  <q-item-section>Provider: {{ p.displayName }} ({{ p.providerId }})</q-item-section>
                </q-item>
                <q-item v-for="m in reviewDiff.models.removed" :key="m.modelId">
                  <q-item-section avatar><q-icon name="remove_circle" color="negative" /></q-item-section>
                  <q-item-section>Model: {{ m.displayName }} ({{ m.modelId }})</q-item-section>
                </q-item>
                <q-item v-for="t in reviewDiff.tts.removed" :key="t.ttsId">
                  <q-item-section avatar><q-icon name="remove_circle" color="negative" /></q-item-section>
                  <q-item-section>TTS: {{ t.displayName }} ({{ t.ttsId }})</q-item-section>
                </q-item>
                <q-item v-for="a in reviewDiff.asr.removed" :key="a.asrId">
                  <q-item-section avatar><q-icon name="remove_circle" color="negative" /></q-item-section>
                  <q-item-section>ASR: {{ a.displayName }} ({{ a.asrId }})</q-item-section>
                </q-item>
                <q-item v-for="m in reviewDiff.mcp.removed" :key="m.mcpServerId">
                  <q-item-section avatar><q-icon name="remove_circle" color="negative" /></q-item-section>
                  <q-item-section>MCP: {{ m.displayName }} ({{ m.mcpServerId }})</q-item-section>
                </q-item>
              </q-list>
            </template>

            <!-- Policy changes -->
            <div v-if="reviewDiff.policyChanged" class="text-subtitle2 q-mb-sm">Policy changes</div>
            <q-banner v-if="reviewDiff.policyChanged" class="bg-blue-1 q-mb-md rounded-borders">
              <div class="text-body2">Policy has been modified. Review local coexistence toggles and default resource selections.</div>
            </q-banner>

            <!-- Runtime routing impact -->
            <div class="text-subtitle2 q-mb-sm">Runtime routing impact</div>
            <q-markup-table flat dense class="q-mb-md">
              <tbody>
                <tr><td class="text-grey-7">Bindings added</td><td class="text-positive">{{ routingImpact.added > 0 ? '+' + routingImpact.added : '—' }}</td></tr>
                <tr><td class="text-grey-7">Bindings changed</td><td class="text-warning">{{ routingImpact.changed > 0 ? '~' + routingImpact.changed : '—' }}</td></tr>
                <tr><td class="text-grey-7">Bindings removed</td><td class="text-negative">{{ routingImpact.removed > 0 ? '-' + routingImpact.removed : '—' }}</td></tr>
              </tbody>
            </q-markup-table>

            <!-- Warnings -->
            <div v-if="reviewWarnings.length" class="text-subtitle2 q-mb-sm">Warnings (acknowledged on publish)</div>
            <q-list v-if="reviewWarnings.length" dense class="q-mb-md">
              <q-item v-for="w in reviewWarnings" :key="w.path + w.code">
                <q-item-section avatar><q-icon name="warning" color="amber-8" /></q-item-section>
                <q-item-section>
                  <span class="text-amber-8">{{ w.code }} — {{ w.message }}</span>
                  <span class="text-caption text-grey-7">{{ w.path }}</span>
                </q-item-section>
              </q-item>
            </q-list>

            <!-- Snapshot hash -->
            <div v-if="preview" class="text-caption text-grey-7">
              Snapshot hash: <code>{{ preview.snapshotHash }}</code>
            </div>
          </q-card-section>

          <q-separator />
          <q-card-actions align="right">
            <q-btn flat label="Cancel" v-close-popup :disable="publishing" />
            <q-btn
              color="positive"
              icon="rocket_launch"
              :label="reviewWarnings.length ? `Publish with ${reviewWarnings.length} warning(s)` : 'Publish'"
              :disable="hasBlockingErrors || publishing"
              :loading="publishing"
              @click="publish"
            />
          </q-card-actions>
        </q-card>
      </q-dialog>

      <!-- Snapshot Preview Dialog -->
      <q-dialog v-model="previewOpen">
        <q-card style="min-width: 700px; max-width: 95vw">
          <q-card-section class="text-h6">Client Snapshot Preview</q-card-section>
          <q-card-section v-if="preview">
            <div class="text-subtitle2 q-mb-sm">Hash: {{ preview.snapshotHash }}</div>
            <div class="text-caption text-grey-7 q-mb-md">Revision {{ preview.draftRevision }}</div>

            <q-banner class="bg-blue-1 q-mb-md rounded-borders">
              <div class="text-body2"><b>Client receives:</b> Providers, Models, TTS, ASR, MCP, Policy</div>
              <div class="text-body2 text-negative"><b>Client never receives:</b> Upstream/base URL, Secret, runtimeRouteId, Runtime Binding, Pricing</div>
            </q-banner>

            <q-expansion-item dense group="preview" label="Providers ({{ preview.providers.length }})" icon="domain">
              <q-markup-table flat dense>
                <thead><tr><th>Display Name</th><th>Protocol</th><th>Enabled</th></tr></thead>
                <tbody>
                  <tr v-for="p in preview.providers" :key="p.providerId">
                    <td>{{ p.displayName }}</td><td>{{ p.clientProtocol }}</td><td>{{ p.enabled }}</td>
                  </tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>

            <q-expansion-item dense group="preview" label="Models ({{ preview.models.length }})" icon="smart_toy">
              <q-markup-table flat dense>
                <thead><tr><th>Display Name</th><th>Model ID</th><th>Provider</th><th>Key</th><th>Input</th><th>Caps</th></tr></thead>
                <tbody>
                  <tr v-for="m in preview.models" :key="m.modelId">
                    <td>{{ m.displayName }}</td><td>{{ m.modelId }}</td><td>{{ m.providerId }}</td>
                    <td>{{ m.upstreamModelKey }}</td><td>{{ m.inputModalities.join(', ') }}</td>
                    <td>{{ m.capabilities.join(', ') }}</td>
                  </tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>

            <q-expansion-item dense group="preview" label="TTS ({{ preview.tts.length }})" icon="record_voice_over">
              <q-markup-table flat dense>
                <thead><tr><th>Display Name</th><th>TTS ID</th><th>Voice</th><th>Key</th></tr></thead>
                <tbody>
                  <tr v-for="t in preview.tts" :key="t.ttsId">
                    <td>{{ t.displayName }}</td><td>{{ t.ttsId }}</td><td>{{ t.voice }}</td><td>{{ t.upstreamModelKey }}</td>
                  </tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>

            <q-expansion-item dense group="preview" label="ASR ({{ preview.asr.length }})" icon="hearing">
              <q-markup-table flat dense>
                <thead><tr><th>Display Name</th><th>ASR ID</th><th>Language</th><th>Key</th></tr></thead>
                <tbody>
                  <tr v-for="a in preview.asr" :key="a.asrId">
                    <td>{{ a.displayName }}</td><td>{{ a.asrId }}</td><td>{{ a.language || 'auto' }}</td><td>{{ a.upstreamModelKey }}</td>
                  </tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>

            <q-expansion-item dense group="preview" label="MCP ({{ preview.mcp.length }})" icon="link">
              <q-markup-table flat dense>
                <thead><tr><th>Display Name</th><th>MCP ID</th><th>Auth</th></tr></thead>
                <tbody>
                  <tr v-for="m in preview.mcp" :key="m.mcpServerId">
                    <td>{{ m.displayName }}</td><td>{{ m.mcpServerId }}</td><td>{{ m.authOwnership }}</td>
                  </tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>

            <q-expansion-item dense group="preview" label="Policy" icon="policy">
              <q-markup-table flat dense>
                <tbody>
                  <tr><td class="text-grey-7">Policy ID</td><td>{{ preview.policy.policyId }}</td></tr>
                  <tr><td class="text-grey-7">Allow local providers</td><td>{{ preview.policy.allowLocalProviders }}</td></tr>
                  <tr><td class="text-grey-7">Allow local TTS</td><td>{{ preview.policy.allowLocalTts }}</td></tr>
                  <tr><td class="text-grey-7">Allow local ASR</td><td>{{ preview.policy.allowLocalAsr }}</td></tr>
                  <tr><td class="text-grey-7">Allow local MCP</td><td>{{ preview.policy.allowLocalMcp }}</td></tr>
                  <tr><td class="text-grey-7">Default Model</td><td>{{ preview.policy.defaultModelId ?? '—' }}</td></tr>
                  <tr><td class="text-grey-7">Default TTS</td><td>{{ preview.policy.defaultTtsId ?? '—' }}</td></tr>
                  <tr><td class="text-grey-7">Default ASR</td><td>{{ preview.policy.defaultAsrId ?? '—' }}</td></tr>
                </tbody>
              </q-markup-table>
            </q-expansion-item>
          </q-card-section>
          <q-card-actions align="right">
            <q-btn flat label="Close" v-close-popup />
          </q-card-actions>
        </q-card>
      </q-dialog>
    </template>
    <div v-else class="text-body2 text-grey-7">No draft loaded.</div>
  </q-page>
</template>