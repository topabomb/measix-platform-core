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
type TransportPolicy = components['schemas']['RuntimeBindingDefinition']['transportPolicy']

const draft = useDraftStore()
const session = useSessionStore()
const activation = useActivationStore()
const error = ref<unknown>()
const publishing = ref(false)
const previewing = ref(false)
const preview = ref<components['schemas']['DraftPreviewResponse']>()
const previewOpen = ref(false)
const upstreams = ref<Upstream[]>([])
const activeTab = ref<'overview' | 'models' | 'tts' | 'asr' | 'mcp' | 'policy'>('overview')
const canMutate = computed(() => Boolean(session.csrfToken))

const INPUT_MODS = ['TEXT', 'IMAGE'] as const
const OUTPUT_MODS = ['TEXT'] as const
const MODEL_CAPS = ['TOOL', 'REASONING'] as const
const AUTH_OWNERSHIPS = ['ENTERPRISE_MANAGED', 'NONE'] as const
const TRANSPORT_POLICIES: TransportPolicy[] = [
  'HTTP_REQUEST_RESPONSE', 'HTTP_STREAMING_SSE', 'HTTP_BINARY_STREAM', 'HTTP_MULTIPART',
]

const up = (id?: string) => upstreams.value.find((u) => u.upstreamId === id)
const upstreamLabel = (id?: string) => (id ? up(id)?.name ?? id : '—')
const upstreamStatus = (id?: string) => up(id)?.status

/** Resource → upstream relationship projection for the Overview tab. */
const relationshipRows = computed(() => {
  const rows: { resourceId: string; kind: string; displayName: string; upstreamId?: string; upstreamName: string; upstreamStatus?: string; enabled: boolean }[] = []
  const c = draft.localContent
  if (!c) return rows
  for (const m of c.models) rows.push({ resourceId: m.modelId, kind: 'Model', displayName: m.displayName, upstreamId: draft.bindingFor(m.modelId)?.upstreamId, upstreamName: upstreamLabel(draft.bindingFor(m.modelId)?.upstreamId), upstreamStatus: upstreamStatus(draft.bindingFor(m.modelId)?.upstreamId), enabled: m.enabled })
  for (const t of c.tts) rows.push({ resourceId: t.ttsId, kind: 'TTS', displayName: t.displayName, upstreamId: draft.bindingFor(t.ttsId)?.upstreamId, upstreamName: upstreamLabel(draft.bindingFor(t.ttsId)?.upstreamId), upstreamStatus: upstreamStatus(draft.bindingFor(t.ttsId)?.upstreamId), enabled: t.enabled })
  for (const a of c.asr) rows.push({ resourceId: a.asrId, kind: 'ASR', displayName: a.displayName, upstreamId: draft.bindingFor(a.asrId)?.upstreamId, upstreamName: upstreamLabel(draft.bindingFor(a.asrId)?.upstreamId), upstreamStatus: upstreamStatus(draft.bindingFor(a.asrId)?.upstreamId), enabled: a.enabled })
  for (const m of c.mcp) rows.push({ resourceId: m.mcpServerId, kind: 'MCP', displayName: m.displayName, upstreamId: draft.bindingFor(m.mcpServerId)?.upstreamId, upstreamName: upstreamLabel(draft.bindingFor(m.mcpServerId)?.upstreamId), upstreamStatus: upstreamStatus(draft.bindingFor(m.mcpServerId)?.upstreamId), enabled: m.enabled })
  return rows
})

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

async function publish() {
  if (!session.csrfToken) return
  if (draft.baselineRevision === undefined) return
  const warnings = draft.validationResult?.warnings ?? []
  if (warnings.length) {
    const ok = window.confirm(
      `Publish with ${warnings.length} acknowledged warning(s)?\n${warnings.map((w) => `${w.code} (${w.path})`).join('\n')}`,
    )
    if (!ok) return
  }
  if (!window.confirm('Publish current draft as a new Release? This creates an immutable staged release.')) return
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
    await refresh()
  } catch (cause) {
    error.value = cause
  } finally {
    publishing.value = false
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
  draft.addModel(content.providers[0].providerId)
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
}

/** Toggle the enabled flag of a resource row (used by the Overview relationship view). */
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

/** Select options for upstream binding, showing active upstreams first. */
const upstreamOptions = computed(() =>
  upstreams.value
    .slice()
    .sort((a, b) => (a.status === 'ACTIVE' ? -1 : 0) - (b.status === 'ACTIVE' ? -1 : 0))
    .map((u) => ({ label: `${u.name} (${u.status})`, value: u.upstreamId })),
)

async function previewSnapshot() {
  if (!session.csrfToken) return
  if (draft.baselineRevision === undefined) return
  previewing.value = true
  error.value = undefined
  try {
    preview.value = await apiFetch<components['schemas']['DraftPreviewResponse']>(
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

onMounted(refresh)
</script>

<template>
  <q-page padding>
    <PageHeader title="Resources" subtitle="Edit the managed capability draft: models, TTS, ASR, MCP, policy and upstream bindings.">
      <template #primary>
        <q-btn color="positive" icon="rocket_launch" label="Publish" :disable="!canMutate" :loading="publishing" @click="publish" />
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
          <q-chip v-if="draft.validationResult?.errors.length" color="negative" outline dense icon="error">
            {{ draft.validationResult.errors.length }} error(s)
          </q-chip>
          <q-chip v-else-if="draft.validationResult?.warnings.length" color="amber" outline dense icon="warning">
            {{ draft.validationResult.warnings.length }} warning(s)
          </q-chip>
        </q-card-section>
      </q-card>

      <!-- Tabbed resource editors (product §8): Overview | Models | TTS | ASR | MCP | Policy -->
      <q-tabs v-model="activeTab" class="q-mb-md" dense align="left">
        <q-tab name="overview" label="Overview" icon="account_tree" />
        <q-tab name="models" label="Models" icon="smart_toy" />
        <q-tab name="tts" label="TTS" icon="record_voice_over" />
        <q-tab name="asr" label="ASR" icon="hearing" />
        <q-tab name="mcp" label="MCP" icon="link" />
        <q-tab name="policy" label="Policy" icon="policy" />
      </q-tabs>

      <!-- ===== Overview: relationship view + providers ===== -->
      <template v-if="activeTab === 'overview'">
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

        <q-card flat bordered>
          <q-card-section>
            <div class="text-subtitle2 q-mb-sm">Resource → Upstream relationships</div>
            <q-markup-table flat dense>
              <thead>
                <tr><th>Kind</th><th>Resource</th><th>Upstream</th><th class="text-right">Status</th></tr>
              </thead>
              <tbody>
                <tr v-for="row in relationshipRows" :key="row.resourceId">
                  <td><q-chip dense>{{ row.kind }}</q-chip></td>
                  <td>
                    <div>{{ row.displayName }}</div>
                    <div class="text-caption text-grey-7">{{ row.resourceId }}</div>
                  </td>
                  <td>
                    <template v-if="row.upstreamId">
                      <div>{{ row.upstreamName }}</div>
                      <div class="text-caption text-grey-7">{{ row.upstreamId }}</div>
                    </template>
                    <span v-else class="text-grey-7">Not bound</span>
                  </td>
                  <td class="text-right">
                    <div class="row items-center justify-end q-gutter-xs">
                      <q-badge v-if="!row.upstreamId" color="orange" label="no binding" />
                      <q-badge v-else-if="row.upstreamStatus" :color="row.upstreamStatus === 'ACTIVE' ? 'green' : 'orange'" :label="row.upstreamStatus.toLowerCase()" />
                      <q-toggle data-testid="relationship-enable-toggle" :model-value="row.enabled" :label="row.enabled ? 'on' : 'off'" @update:model-value="(v: boolean) => toggleEnabled(row.kind, row.resourceId, v)" />
                    </div>
                  </td>
                </tr>
                <tr v-if="!relationshipRows.length"><td colspan="4" class="text-grey-7">No managed resources yet.</td></tr>
              </tbody>
            </q-markup-table>
          </q-card-section>
        </q-card>
      </template>

      <!-- ===== Models ===== -->
      <q-card v-if="activeTab === 'models'" flat bordered>
        <q-card-section>
          <div class="row items-center justify-between">
            <div class="text-subtitle2">Models</div>
            <q-btn flat dense icon="add" label="Add model" size="sm" :disable="!draft.localContent.providers.length" @click="addModel()" />
          </div>
          <q-list dense class="q-mt-sm">
            <q-item v-for="(model, idx) in draft.localContent.models" :key="model.modelId">
              <q-item-section>
                <div class="row q-gutter-sm">
                  <q-input v-model="model.displayName" dense outlined label="Display name" class="col" @update:model-value="draft.markDirty()" />
                  <q-input v-model="model.upstreamModelKey" dense outlined label="Upstream model key" class="col" @update:model-value="draft.markDirty()" />
                </div>
                <div class="row q-gutter-sm q-mt-xs">
                  <q-select v-model="model.providerId" dense outlined label="Provider" :options="draft.localContent.providers.map((p) => ({ label: p.displayName, value: p.providerId }))" emit-value map-options class="col" @update:model-value="draft.markDirty()" />
                  <q-input v-model="model.runtimePath" dense outlined label="Runtime path" class="col" @update:model-value="draft.markDirty()" />
                </div>
                <div class="row q-gutter-sm q-mt-xs">
                  <q-select v-model="model.inputModalities" dense outlined label="Input" multiple :options="[...INPUT_MODS]" class="col" emit-value map-options @update:model-value="draft.markDirty()" />
                  <q-select v-model="model.outputModalities" dense outlined label="Output" multiple :options="[...OUTPUT_MODS]" class="col" emit-value map-options @update:model-value="draft.markDirty()" />
                  <q-select v-model="model.capabilities" dense outlined label="Capabilities" multiple :options="[...MODEL_CAPS]" class="col" emit-value map-options @update:model-value="draft.markDirty()" />
                </div>
                <div class="row q-gutter-sm q-mt-xs items-center">
                  <q-select :model-value="draft.bindingFor(model.modelId)?.upstreamId ?? ''" dense outlined label="Upstream" :options="upstreamOptions" emit-value map-options class="col" @update:model-value="(v: string) => draft.setBinding(model.modelId, v, 'HTTP_STREAMING_SSE')" />
                  <q-toggle v-model="model.enabled" label="Enabled" @update:model-value="draft.markDirty()" />
                  <div class="text-caption text-grey-7">{{ model.modelId }}</div>
                  <q-btn flat dense color="negative" icon="delete" size="sm" @click="removeResource('models', idx, model.modelId)" />
                </div>
              </q-item-section>
            </q-item>
            <q-item v-if="!draft.localContent.models.length"><q-item-section class="text-grey-7">No models.</q-item-section></q-item>
          </q-list>
        </q-card-section>
      </q-card>

      <!-- ===== TTS ===== -->
      <q-card v-if="activeTab === 'tts'" flat bordered>
        <q-card-section>
          <div class="row items-center justify-between">
            <div class="text-subtitle2">TTS <span class="text-caption text-grey-7">· OpenAI Audio Speech</span></div>
            <q-btn flat dense icon="add" label="Add TTS" size="sm" @click="draft.addTts()" />
          </div>
          <q-list dense class="q-mt-sm">
            <q-item v-for="(tts, idx) in draft.localContent.tts" :key="tts.ttsId">
              <q-item-section>
                <div class="row q-gutter-sm">
                  <q-input v-model="tts.displayName" dense outlined label="Display name" class="col" @update:model-value="draft.markDirty()" />
                  <q-input v-model="tts.upstreamModelKey" dense outlined label="Model" class="col" @update:model-value="draft.markDirty()" />
                  <q-input v-model="tts.voice" dense outlined label="Voice" class="col" @update:model-value="draft.markDirty()" />
                </div>
                <div class="row q-gutter-sm q-mt-xs items-center">
                  <q-input v-model="tts.runtimePath" dense outlined label="Runtime path" class="col" @update:model-value="draft.markDirty()" />
                  <q-select :model-value="draft.bindingFor(tts.ttsId)?.upstreamId ?? ''" dense outlined label="Upstream" :options="upstreamOptions" emit-value map-options class="col" @update:model-value="(v: string) => draft.setBinding(tts.ttsId, v, 'HTTP_BINARY_STREAM')" />
                  <q-toggle v-model="tts.enabled" label="Enabled" @update:model-value="draft.markDirty()" />
                  <div class="text-caption text-grey-7">{{ tts.ttsId }}</div>
                  <q-btn flat dense color="negative" icon="delete" size="sm" @click="removeResource('tts', idx, tts.ttsId)" />
                </div>
              </q-item-section>
            </q-item>
            <q-item v-if="!draft.localContent.tts.length"><q-item-section class="text-grey-7">No TTS.</q-item-section></q-item>
          </q-list>
        </q-card-section>
      </q-card>

      <!-- ===== ASR ===== -->
      <q-card v-if="activeTab === 'asr'" flat bordered>
        <q-card-section>
          <div class="row items-center justify-between">
            <div class="text-subtitle2">ASR <span class="text-caption text-grey-7">· OpenAI Audio Transcriptions (HTTP)</span></div>
            <q-btn flat dense icon="add" label="Add ASR" size="sm" @click="draft.addAsr()" />
          </div>
          <q-list dense class="q-mt-sm">
            <q-item v-for="(asr, idx) in draft.localContent.asr" :key="asr.asrId">
              <q-item-section>
                <div class="row q-gutter-sm">
                  <q-input v-model="asr.displayName" dense outlined label="Display name" class="col" @update:model-value="draft.markDirty()" />
                  <q-input v-model="asr.upstreamModelKey" dense outlined label="Model" class="col" @update:model-value="draft.markDirty()" />
                  <q-input v-model="asr.language" dense outlined label="Optional language" class="col" @update:model-value="draft.markDirty()" />
                </div>
                <div class="row q-gutter-sm q-mt-xs items-center">
                  <q-input v-model="asr.runtimePath" dense outlined label="Runtime path" class="col" @update:model-value="draft.markDirty()" />
                  <q-select :model-value="draft.bindingFor(asr.asrId)?.upstreamId ?? ''" dense outlined label="Upstream" :options="upstreamOptions" emit-value map-options class="col" @update:model-value="(v: string) => draft.setBinding(asr.asrId, v, 'HTTP_MULTIPART')" />
                  <q-toggle v-model="asr.enabled" label="Enabled" @update:model-value="draft.markDirty()" />
                  <div class="text-caption text-grey-7">{{ asr.asrId }}</div>
                  <q-btn flat dense color="negative" icon="delete" size="sm" @click="removeResource('asr', idx, asr.asrId)" />
                </div>
              </q-item-section>
            </q-item>
            <q-item v-if="!draft.localContent.asr.length"><q-item-section class="text-grey-7">No ASR.</q-item-section></q-item>
          </q-list>
        </q-card-section>
      </q-card>

      <!-- ===== MCP ===== -->
      <q-card v-if="activeTab === 'mcp'" flat bordered>
        <q-card-section>
          <div class="row items-center justify-between">
            <div class="text-subtitle2">MCP <span class="text-caption text-grey-7">· MCP Streamable HTTP</span></div>
            <q-btn flat dense icon="add" label="Add MCP" size="sm" @click="draft.addMcp()" />
          </div>
          <q-list dense class="q-mt-sm">
            <q-item v-for="(mcp, idx) in draft.localContent.mcp" :key="mcp.mcpServerId">
              <q-item-section>
                <div class="row q-gutter-sm">
                  <q-input v-model="mcp.displayName" dense outlined label="Display name" class="col" @update:model-value="draft.markDirty()" />
                  <q-input v-model="mcp.runtimePath" dense outlined label="Runtime path" class="col" @update:model-value="draft.markDirty()" />
                  <q-select v-model="mcp.authOwnership" dense outlined label="Auth ownership" :options="[...AUTH_OWNERSHIPS]" class="col" @update:model-value="draft.markDirty()" />
                </div>
                <div class="row q-gutter-sm q-mt-xs items-center">
                  <q-select :model-value="draft.bindingFor(mcp.mcpServerId)?.upstreamId ?? ''" dense outlined label="Upstream" :options="upstreamOptions" emit-value map-options class="col" @update:model-value="(v: string) => draft.setBinding(mcp.mcpServerId, v, 'HTTP_REQUEST_RESPONSE')" />
                  <q-toggle v-model="mcp.enabled" label="Enabled" @update:model-value="draft.markDirty()" />
                  <div class="text-caption text-grey-7">{{ mcp.mcpServerId }}</div>
                  <q-btn flat dense color="negative" icon="delete" size="sm" @click="removeResource('mcp', idx, mcp.mcpServerId)" />
                </div>
              </q-item-section>
            </q-item>
            <q-item v-if="!draft.localContent.mcp.length"><q-item-section class="text-grey-7">No MCP.</q-item-section></q-item>
          </q-list>
        </q-card-section>
      </q-card>

      <!-- ===== Policy ===== -->
      <q-card v-if="activeTab === 'policy'" flat bordered>
        <q-card-section>
          <div class="text-subtitle2 q-mb-sm">Policy</div>
          <div class="row q-gutter-md">
            <q-toggle v-model="draft.localContent.policy.allowLocalProviders" label="Allow local providers" @update:model-value="draft.markDirty()" />
            <q-toggle v-model="draft.localContent.policy.allowLocalTts" label="Allow local TTS" @update:model-value="draft.markDirty()" />
            <q-toggle v-model="draft.localContent.policy.allowLocalAsr" label="Allow local ASR" @update:model-value="draft.markDirty()" />
            <q-toggle v-model="draft.localContent.policy.allowLocalMcp" label="Allow local MCP" @update:model-value="draft.markDirty()" />
          </div>
          <div class="text-caption text-grey-7 q-mt-sm">Policy ID: {{ draft.localContent.policy.policyId }}</div>
        </q-card-section>
      </q-card>

      <!-- Validation (shared, below all tabs) -->
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

      <!-- Snapshot Preview Dialog -->
      <q-dialog v-model="previewOpen">
        <q-card style="min-width: 600px; max-width: 95vw">
          <q-card-section class="text-h6">Snapshot Preview</q-card-section>
          <q-card-section v-if="preview">
            <div class="text-subtitle2 q-mb-sm">Hash: {{ preview.snapshotHash }}</div>
            <div class="text-caption text-grey-7 q-mb-md">Revision {{ preview.draftRevision }} · {{ preview.providers.length }} providers · {{ preview.models.length }} models · {{ preview.tts.length }} TTS · {{ preview.asr.length }} ASR · {{ preview.mcp.length }} MCP</div>
            <q-markup-table flat dense>
              <tbody>
                <tr><td class="text-grey-7">Providers</td><td>{{ preview.providers.length }}</td></tr>
                <tr><td class="text-grey-7">Models</td><td>{{ preview.models.length }}</td></tr>
                <tr><td class="text-grey-7">TTS</td><td>{{ preview.tts.length }}</td></tr>
                <tr><td class="text-grey-7">ASR</td><td>{{ preview.asr.length }}</td></tr>
                <tr><td class="text-grey-7">MCP</td><td>{{ preview.mcp.length }}</td></tr>
                <tr><td class="text-grey-7">Policy ID</td><td>{{ preview.policy.policyId }}</td></tr>
              </tbody>
            </q-markup-table>
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
