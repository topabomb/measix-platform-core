<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import { useSessionStore } from '../stores/session'
import ProblemBanner from './ProblemBanner.vue'

const props = defineProps<{ selection: string[] }>()
const emit = defineEmits<{ cleaned: [] }>()
const { t } = useI18n()
const session = useSessionStore()
type Selection = components['schemas']['ReleaseCleanupSelection']
type Preview = components['schemas']['ReleaseCleanupPreview']
type Retention = components['schemas']['ReleaseRetention']
type Audit = components['schemas']['ReleaseHistoryAuditPage']
const cleanupOpen = ref(false), ruleOpen = ref(false), busy = ref(false)
const error = ref<unknown>()
const mode = ref('selected'), ids = ref<string[]>([])
const before = ref(''), after = ref('')
const preview = ref<Preview>(), previewSelection = ref<Selection>()
const policy = ref<Retention>(), audits = ref<Audit>()
const enabled = ref(false), keepLast = ref<number | string | null>(10), keepDays = ref<number | string | null>(30)
const modes = [ { label: t('releaseHistory.selected'), value: 'selected' }, { label: t('releaseHistory.period'), value: 'period' }, { label: t('releaseHistory.savedRule'), value: 'rule' } ]
watch([mode, ids, before, after], () => { preview.value = undefined; previewSelection.value = undefined }, { deep: true })

function openCleanup(selected = props.selection) {
  ids.value = [...selected]; mode.value = selected.length ? 'selected' : 'period'
  before.value = new Date().toISOString().slice(0, 16); after.value = ''
  preview.value = undefined; previewSelection.value = undefined; error.value = undefined; cleanupOpen.value = true
}
defineExpose({ openCleanup })

async function loadPolicy() {
  policy.value = await apiFetch<Retention>('/api/admin/v1/releases/retention')
  enabled.value = policy.value.rule.enabled
  keepLast.value = policy.value.rule.keepLast ?? null; keepDays.value = policy.value.rule.keepDays ?? null
  audits.value = await apiFetch<Audit>('/api/admin/v1/releases/cleanup/audit')
}
async function openRule() {
  error.value = undefined; busy.value = true; ruleOpen.value = true
  try { await loadPolicy() } catch (cause) { error.value = cause } finally { busy.value = false }
}
async function saveRule() {
  if (!policy.value || !session.csrfToken) return
  error.value = undefined; busy.value = true
  try {
    await apiFetch<Retention>('/api/admin/v1/releases/retention', { method: 'PUT', body: JSON.stringify({ expectedRevision: policy.value.revision, rule: { enabled: enabled.value, keepLast: keepLast.value === '' || keepLast.value == null ? undefined : Number(keepLast.value), keepDays: keepDays.value === '' || keepDays.value == null ? undefined : Number(keepDays.value) } }) }, session.csrfToken)
    await loadPolicy()
  } catch (cause) { error.value = cause; policy.value = undefined } finally { busy.value = false }
}
async function makePreview() {
  if (!session.csrfToken) return
  preview.value = undefined; error.value = undefined; busy.value = true
  try {
    // Time inputs are explicitly UTC; the server uses [after, before).
    const selection: Selection = mode.value === 'selected' ? { releaseIds: [...ids.value] } : mode.value === 'rule' ? { useRetentionRule: true } : { publishedBefore: new Date(before.value + 'Z').toISOString(), publishedAfter: after.value ? new Date(after.value + 'Z').toISOString() : undefined }
    preview.value = await apiFetch<Preview>('/api/admin/v1/releases/cleanup:preview', { method: 'POST', body: JSON.stringify(selection) }, session.csrfToken)
    previewSelection.value = selection
  } catch (cause) { error.value = cause } finally { busy.value = false }
}
async function execute() {
  if (!preview.value || !previewSelection.value || preview.value.activationBlocked || !session.csrfToken) return
  busy.value = true; error.value = undefined
  const request = { selection: previewSelection.value, previewHash: preview.value.previewHash }
  // A lost response or conflict consumes this preview. Refresh and preview
  // again instead of silently retrying a destructive command.
  preview.value = undefined; previewSelection.value = undefined
  try {
    await apiFetch('/api/admin/v1/releases/cleanup', { method: 'POST', body: JSON.stringify(request) }, session.csrfToken)
    cleanupOpen.value = false; emit('cleaned')
  } catch (cause) { error.value = cause; emit('cleaned') } finally { busy.value = false }
}
</script>

<template>
  <div class="row q-gutter-xs">
    <q-btn flat :label="t('releaseHistory.cleanup')" data-cy="release-cleanup-open" :disable="!session.csrfToken" @click="openCleanup()" />
    <q-btn flat :label="t('releaseHistory.rule')" data-cy="release-retention-open" :disable="!session.csrfToken" @click="openRule" />
  </div>
  <q-dialog v-model="cleanupOpen" :persistent="busy">
    <q-card class="app-dialog">
      <q-card-section class="text-h6">{{ t('releaseHistory.cleanup') }}</q-card-section>
      <q-card-section class="app-dialog__body q-gutter-sm">
        <ProblemBanner :error="error" />
        <q-banner class="bg-orange-1 rounded-borders">{{ t('releaseHistory.consequence') }}</q-banner>
        <q-select v-model="mode" outlined emit-value map-options :options="modes" :label="t('releaseHistory.scope')" :disable="busy" />
        <div v-if="mode === 'selected'">{{ t('releaseHistory.selectedCount', { count: ids.length }) }}</div>
        <template v-if="mode === 'period'">
          <q-input v-model="after" outlined type="datetime-local" :label="t('releaseHistory.after')" :disable="busy" />
          <q-input v-model="before" outlined type="datetime-local" :label="t('releaseHistory.before')" :disable="busy" />
        </template>
        <div v-if="mode === 'rule'" class="text-caption">{{ t('releaseHistory.ruleHint') }}</div>
        <q-btn outline color="primary" :label="t('releaseHistory.preview')" data-cy="release-cleanup-preview" :loading="busy" :disable="busy || mode === 'selected' && !ids.length || mode === 'period' && !before" @click="makePreview" />
        <template v-if="preview">
          <div data-cy="release-cleanup-summary">{{ t('releaseHistory.summary', { count: preview.candidates.length, size: (preview.reclaimableBytes / 1024).toFixed(1) }) }}</div>
          <div class="text-caption">{{ preview.candidates.map(r => t('releases.versionLabel', { generation: r.managedGeneration })).join('、') }}</div>
          <q-banner v-if="preview.activationBlocked" class="bg-orange-1">{{ t('releaseHistory.busy') }}</q-banner>
          <div v-if="preview.protected.length" data-cy="release-cleanup-protected">
            <div class="text-subtitle2">{{ t('releaseHistory.protected') }}</div>
            <div v-for="r in preview.protected" :key="r.releaseId" class="text-caption">{{ r.managedGeneration ? t('releases.versionLabel', { generation: r.managedGeneration }) : t('releaseHistory.missing') }} · {{ t('releaseHistory.reasons.' + r.reason) }}</div>
          </div>
          <div v-if="preview.releasedUpstreamIds.length">{{ t('releaseHistory.upstreams', { count: preview.releasedUpstreamIds.length }) }}</div>
          <div v-if="preview.hasMore" class="text-caption">{{ t('releaseHistory.more') }}</div>
        </template>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat :label="t('common.close')" :disable="busy" @click="cleanupOpen = false" />
        <q-btn color="negative" :label="t('releaseHistory.execute')" data-cy="release-cleanup-execute" :loading="busy" :disable="busy || !preview?.candidates.length || preview.activationBlocked" @click="execute" />
      </q-card-actions>
    </q-card>
  </q-dialog>
  <q-dialog v-model="ruleOpen" :persistent="busy">
    <q-card class="app-dialog">
      <q-card-section class="text-h6">{{ t('releaseHistory.rule') }}</q-card-section>
      <q-card-section class="app-dialog__body q-gutter-sm">
        <ProblemBanner :error="error" />
        <q-banner class="bg-blue-1">{{ t('releaseHistory.ruleHint') }}</q-banner>
        <template v-if="policy">
          <q-toggle v-model="enabled" :label="t('releaseHistory.enabled')" :disable="busy" />
          <q-input v-model="keepLast" outlined type="number" min="1" max="1000" clearable :label="t('releaseHistory.keepLast')" :disable="busy" />
          <q-input v-model="keepDays" outlined type="number" min="1" max="36500" clearable :label="t('releaseHistory.keepDays')" :disable="busy" />
          <div v-if="enabled" class="text-negative text-caption">{{ t('releaseHistory.enableHint') }}</div>
        </template>
        <div class="text-subtitle2">{{ t('releaseHistory.audit') }}</div>
        <div v-for="a in audits?.items" :key="a.auditId" class="text-caption">
          {{ new Date(a.createdAt).toLocaleString() }} · {{ t('releaseHistory.kinds.' + a.kind) }}
          <span v-if="a.releaseGenerations.length"> · {{ a.releaseGenerations.join('、') }} · {{ (a.reclaimedBytes / 1024).toFixed(1) }} KiB</span>
        </div>
        <div v-if="audits && !audits.items.length" class="text-caption">{{ t('common.noData') }}</div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat icon="refresh" :label="t('common.refresh')" :disable="busy" @click="openRule" />
        <q-btn flat :label="t('common.close')" :disable="busy" @click="ruleOpen = false" />
        <q-btn color="primary" :label="t('common.save')" :loading="busy" :disable="busy || !policy || !(keepLast || keepDays)" @click="saveRule" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>
