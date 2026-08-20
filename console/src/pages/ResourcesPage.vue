<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { components } from '../api/generated'
import { apiFetch, createCandidateId } from '../api/client'
import { useDraftStore } from '../stores/draft'
import { useSessionStore } from '../stores/session'
import { useActivationStore } from '../stores/activation'
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

const draft = useDraftStore()
const session = useSessionStore()
const activation = useActivationStore()
const error = ref<unknown>()
const publishing = ref(false)
const previewing = ref(false)
const preview = ref<components['schemas']['DraftPreviewResponse']>()
const previewOpen = ref(false)
const canMutate = computed(() => Boolean(session.csrfToken))

const INPUT_MODS = ['TEXT', 'IMAGE'] as const
const OUTPUT_MODS = ['TEXT'] as const
const MODEL_CAPS = ['TOOL', 'REASONING'] as const
const AUTH_OWNERSHIPS = ['ENTERPRISE_MANAGED', 'NONE'] as const

async function refresh() {
  error.value = undefined
  try { await draft.load() } catch (cause) { error.value = cause }
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
  // The executable contract requires expectedDraftRevision and an explicit
  // acknowledgement of any outstanding warning codes before publishing.
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

/** Count models referencing a given providerId */
function modelCountForProvider(providerId: string): number {
  return draft.localContent?.models.filter((m) => m.providerId === providerId).length ?? 0
}

/** Add a model bound to the first real provider. Never fabricate a placeholder
 * provider (architecture testing spec forbids prv_placeholder bindings). */
function addModel() {
  const content = draft.localContent
  if (!content) return
  if (!content.providers.length) {
    error.value = new Error('Add a provider before adding models — every model must bind to a real provider.')
    return
  }
  draft.addModel(content.providers[0].providerId)
  draft.markDirty()
}

/** Add a real provider (candidate id) — never a placeholder binding. */
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

/** Delete a provider only when no models reference it. */
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
    <div class="row items-center justify-between q-mb-lg">
      <div>
        <div class="text-h5 text-weight-bold">Draft</div>
        <div class="text-body2 text-grey-7">Edit managed configuration before publishing as an immutable release.</div>
      </div>
      <div class="q-gutter-sm">
        <q-btn flat icon="refresh" @click="refresh" />
        <q-btn outline color="primary" label="Validate" :disable="!canMutate || draft.loading" @click="validate" />
        <q-btn outline color="primary" label="Save" :disable="!canMutate || !draft.dirty" :loading="draft.saving" @click="save" />
        <q-btn outline color="secondary" label="Preview" :disable="!canMutate" :loading="previewing" @click="previewSnapshot" />
        <q-btn color="positive" icon="rocket_launch" label="Publish" :disable="!canMutate" :loading="publishing" @click="publish" />
      </div>
    </div>
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
      <q-card flat bordered class="q-mb-md">
        <q-card-section>
          <div class="row items-center justify-between">
            <div class="text-subtitle1">Revision {{ draft.baselineRevision }}<q-badge v-if="draft.dirty" color="orange" label="unsaved" class="q-ml-sm" /></div>
          </div>
        </q-card-section>
      </q-card>

      <!-- Providers + Models -->
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

      <div class="row q-gutter-md">
        <!-- Models -->
        <q-card flat bordered class="col">
          <q-card-section>
            <div class="row items-center justify-between">
              <div class="text-subtitle2">Models</div>
              <q-btn flat dense icon="add" label="Add model" size="sm" :disable="!draft.localContent.providers.length" @click="addModel()" />
            </div>
            <q-list dense class="q-mt-sm">
              <q-item v-for="(model, idx) in draft.localContent.models" :key="model.modelId">
                <q-item-section>
                  <q-input v-model="model.displayName" dense outlined label="Name" @update:model-value="draft.markDirty()" />
                  <q-input v-model="model.upstreamModelKey" dense outlined label="Upstream model key" class="q-mt-xs" @update:model-value="draft.markDirty()" />
                  <div class="text-caption text-grey-7">{{ model.modelId }} · {{ model.providerId }}</div>
                </q-item-section>
                <q-item-section side>
                  <q-toggle v-model="model.enabled" @update:model-value="draft.markDirty()" />
                  <q-btn flat dense color="negative" icon="delete" size="sm" @click="draft.localContent!.models.splice(idx, 1); draft.markDirty()" />
                </q-item-section>
              </q-item>
              <q-item v-if="!draft.localContent.models.length"><q-item-section class="text-grey-7">No models.</q-item-section></q-item>
            </q-list>
          </q-card-section>
        </q-card>

        <!-- TTS -->
        <q-card flat bordered class="col">
          <q-card-section>
            <div class="row items-center justify-between">
              <div class="text-subtitle2">TTS</div>
              <q-btn flat dense icon="add" label="Add TTS" size="sm" @click="draft.addTts()" />
            </div>
            <q-list dense class="q-mt-sm">
              <q-item v-for="(tts, idx) in draft.localContent.tts" :key="tts.ttsId">
                <q-item-section>
                  <q-input v-model="tts.displayName" dense outlined label="Name" @update:model-value="draft.markDirty()" />
                  <q-input v-model="tts.voice" dense outlined label="Voice" class="q-mt-xs" @update:model-value="draft.markDirty()" />
                  <div class="text-caption text-grey-7">{{ tts.ttsId }} · {{ tts.clientProtocol }}</div>
                </q-item-section>
                <q-item-section side>
                  <q-toggle v-model="tts.enabled" @update:model-value="draft.markDirty()" />
                  <q-btn flat dense color="negative" icon="delete" size="sm" @click="draft.localContent!.tts.splice(idx, 1); draft.markDirty()" />
                </q-item-section>
              </q-item>
              <q-item v-if="!draft.localContent.tts.length"><q-item-section class="text-grey-7">No TTS.</q-item-section></q-item>
            </q-list>
          </q-card-section>
        </q-card>
      </div>

      <div class="row q-gutter-md q-mt-md">
        <!-- ASR -->
        <q-card flat bordered class="col">
          <q-card-section>
            <div class="row items-center justify-between">
              <div class="text-subtitle2">ASR</div>
              <q-btn flat dense icon="add" label="Add ASR" size="sm" @click="draft.addAsr()" />
            </div>
            <q-list dense class="q-mt-sm">
              <q-item v-for="(asr, idx) in draft.localContent.asr" :key="asr.asrId">
                <q-item-section>
                  <q-input v-model="asr.displayName" dense outlined label="Name" @update:model-value="draft.markDirty()" />
                  <q-input v-model="asr.language" dense outlined label="Language" class="q-mt-xs" @update:model-value="draft.markDirty()" />
                  <div class="text-caption text-grey-7">{{ asr.asrId }} · {{ asr.clientProtocol }}</div>
                </q-item-section>
                <q-item-section side>
                  <q-toggle v-model="asr.enabled" @update:model-value="draft.markDirty()" />
                  <q-btn flat dense color="negative" icon="delete" size="sm" @click="draft.localContent!.asr.splice(idx, 1); draft.markDirty()" />
                </q-item-section>
              </q-item>
              <q-item v-if="!draft.localContent.asr.length"><q-item-section class="text-grey-7">No ASR.</q-item-section></q-item>
            </q-list>
          </q-card-section>
        </q-card>

        <!-- MCP -->
        <q-card flat bordered class="col">
          <q-card-section>
            <div class="row items-center justify-between">
              <div class="text-subtitle2">MCP</div>
              <q-btn flat dense icon="add" label="Add MCP" size="sm" @click="draft.addMcp()" />
            </div>
            <q-list dense class="q-mt-sm">
              <q-item v-for="(mcp, idx) in draft.localContent.mcp" :key="mcp.mcpServerId">
                <q-item-section>
                  <q-input v-model="mcp.displayName" dense outlined label="Name" @update:model-value="draft.markDirty()" />
                  <div class="text-caption text-grey-7">{{ mcp.mcpServerId }} · {{ mcp.clientProtocol }} · {{ mcp.authOwnership }}</div>
                </q-item-section>
                <q-item-section side>
                  <q-toggle v-model="mcp.enabled" @update:model-value="draft.markDirty()" />
                  <q-btn flat dense color="negative" icon="delete" size="sm" @click="draft.localContent!.mcp.splice(idx, 1); draft.markDirty()" />
                </q-item-section>
              </q-item>
              <q-item v-if="!draft.localContent.mcp.length"><q-item-section class="text-grey-7">No MCP.</q-item-section></q-item>
            </q-list>
          </q-card-section>
        </q-card>
      </div>

      <!-- Policy -->
      <q-card flat bordered class="q-mt-md">
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

      <!-- Validation -->
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
                <q-item-section>{{ e.path }}: {{ e.code }} — {{ e.detail }}</q-item-section>
              </q-item>
              <q-item v-for="w in draft.validationResult.warnings" :key="w.path + w.code">
                <q-item-section avatar><q-icon name="warning" color="amber-8" /></q-item-section>
                <q-item-section>{{ w.path }}: {{ w.code }} — {{ w.detail }}</q-item-section>
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
