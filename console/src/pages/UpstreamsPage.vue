<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, toRaw, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch, ApiProblem } from '../api/client'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import DetailWorkspace from '../components/DetailWorkspace.vue'
import CursorPager from '../components/CursorPager.vue'
import PagedEntityPicker, { type EntityPickerOption } from '../components/PagedEntityPicker.vue'
import { useCursorPager } from '../composables/useCursorPager'
import { fetchSecretPickerPage, resolveSecretPickerOption } from '../api/entityPickerSources'
import { useActivationStore } from '../stores/activation'
import { useSessionStore } from '../stores/session'

const { t: $t } = useI18n()

type Upstream = components['schemas']['Upstream']
type UpstreamPage = components['schemas']['UpstreamPage']
type Activation = components['schemas']['Activation']
type UpstreamConfig = components['schemas']['UpstreamConfig']
type UpstreamTestResult = components['schemas']['UpstreamTestResult']
type Secret = components['schemas']['Secret']

const session = useSessionStore()
const activation = useActivationStore()
const listPath = ref('/api/admin/v1/upstreams?limit=50')
const search = ref('')
const {
  items: upstreams,
  nextCursor,
  pageNumber,
  loading,
  error,
  reset: resetUpstreams,
  nextPage,
  previousPage,
} = useCursorPager<Upstream, UpstreamPage>(listPath, path => apiFetch<UpstreamPage>(path))

const selected = ref<Upstream>()
const createOpen = ref(false)
const testing = ref(false)
const applyConfirmOpen = ref(false)
const applying = ref(false)
const testResult = ref<UpstreamTestResult>()
const canMutate = computed(() => Boolean(session.csrfToken))

// Editing state for existing upstream candidate
const editMode = ref(false)
const editForm = ref<UpstreamConfig>(emptyConfig())
const editUpstreamId = ref<string>()
const editExpectedRevision = ref<number>()
const editDirty = ref(false)
const saving = ref(false)
const conflictRevision = ref<number>()

// Inline Secret creation/replace
const secretOpen = ref(false)
const secretMode = ref<'create' | 'replace'>('create')
const secretName = ref('')
const secretValue = ref('')
const replacingSecret = ref(false)
const replaceSecretId = ref<string>()
const replaceExpectedVersion = ref<number>()

const selectedSecretId = ref<string>()
const selectedSecret = ref<Secret>()
const justCreatedSecret = ref<Secret>()
const secretNames = ref<Record<string, string>>({})
function secretDisplayName(id: string | undefined): string {
  if (!id) return '—'
  return secretNames.value[id] ?? id
}

function secretFromOption(option: EntityPickerOption | undefined): Secret | undefined {
  const version = Number(option?.metadata?.secretVersion)
  if (!option || !Number.isInteger(version) || version < 1) return undefined
  return { secretId: option.value, name: option.label, secretVersion: version }
}

const selectedSecretOption = computed<EntityPickerOption | undefined>(() => selectedSecret.value ? {
  value: selectedSecret.value.secretId,
  label: selectedSecret.value.name,
  caption: `v${selectedSecret.value.secretVersion}`,
  metadata: { secretVersion: selectedSecret.value.secretVersion },
} : undefined)

function selectCreateSecret(option: EntityPickerOption | undefined) {
  selectedSecret.value = secretFromOption(option)
  if (selectedSecret.value) secretNames.value[selectedSecret.value.secretId] = selectedSecret.value.name
}

const AUTH_TYPES = ['NONE', 'BEARER', 'STATIC_HEADER', 'BASIC'] as const
const authLabel = (type: string) => $t(`upstreams.authTypes.${type}`)
const authOptions = computed(() => AUTH_TYPES.map(value => ({ label: authLabel(value), value })))
const USAGE_LEVELS = ['LEVEL_0', 'LEVEL_1', 'LEVEL_2'] as const
const CORRELATION_MODES = ['HEADER_ECHO', 'VIRTUAL_KEY', 'REQUEST_LOG_ID', 'USAGE_API', 'WEBHOOK', 'NONE'] as const
const TRANSPORT_CAPS = ['HTTP_REQUEST_RESPONSE', 'HTTP_STREAMING_SSE', 'HTTP_BINARY_STREAM', 'HTTP_MULTIPART', 'WEBSOCKET'] as const
const usageLabel = (value: string) => $t(`upstreams.usageLevels.${value}`)
const correlationLabel = (value: string) => $t(`upstreams.correlationModes.${value}`)
const transportLabel = (value: string) => $t(`upstreams.transportTypes.${value}`)
const usageOptions = computed(() => USAGE_LEVELS.map(value => ({ label: usageLabel(value), value })))
const correlationOptions = computed(() => CORRELATION_MODES.map(value => ({ label: correlationLabel(value), value })))
const transportOptions = computed(() => TRANSPORT_CAPS.map(value => ({ label: transportLabel(value), value })))

function emptyConfig(): UpstreamConfig {
  return {
    name: '',
    baseUrl: '',
    transportCapabilities: ['HTTP_STREAMING_SSE'],
    auth: { type: 'NONE' },
    correlationMode: 'NONE',
    usageCapabilityLevel: 'LEVEL_0',
    timeoutDefaults: { connectMs: 1000, responseHeaderMs: 5000, idleMs: 30000 },
  }
}

const createForm = ref<UpstreamConfig>(emptyConfig())
const headerName = ref('')
const username = ref('')

async function refresh() {
  const query = new URLSearchParams({ limit: '50' })
  if (search.value.trim()) query.set('query', search.value.trim())
  listPath.value = `/api/admin/v1/upstreams?${query.toString()}`
  await resetUpstreams()
}

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { void refresh() }, 300)
})

function inlineSecretRef(): { secretId: string; secretVersion: number } | undefined {
  if (!selectedSecret.value) return undefined
  return { secretId: selectedSecret.value.secretId, secretVersion: selectedSecret.value.secretVersion }
}

function buildAuth(): UpstreamConfig['auth'] {
  const authType = createForm.value.auth.type
  if (authType === 'NONE') return { type: 'NONE' }
  const ref = inlineSecretRef()
  if (authType === 'BEARER') {
    return ref ? { type: 'BEARER', secretRef: ref } : { type: 'BEARER' }
  }
  if (authType === 'STATIC_HEADER') {
    return headerName.value.trim()
      ? { type: 'STATIC_HEADER', headerName: headerName.value.trim(), ...(ref ? { secretRef: ref } : {}) }
      : { type: 'STATIC_HEADER', ...(ref ? { secretRef: ref } : {}) }
  }
  if (authType === 'BASIC') {
    return username.value.trim()
      ? { type: 'BASIC', username: username.value.trim(), ...(ref ? { passwordSecretRef: ref } : {}) }
      : { type: 'BASIC', ...(ref ? { passwordSecretRef: ref } : {}) }
  }
  return { type: authType }
}

async function createUpstream() {
  if (!session.csrfToken) return
  error.value = undefined
  try {
    const config: UpstreamConfig = {
      ...createForm.value,
      auth: buildAuth(),
    }
    await apiFetch<Upstream>('/api/admin/v1/upstreams', {
      method: 'POST',
      body: JSON.stringify({ config }),
    }, session.csrfToken)
    createOpen.value = false
    createForm.value = emptyConfig()
    selectedSecretId.value = undefined
    selectedSecret.value = undefined
    justCreatedSecret.value = undefined
    headerName.value = ''
    username.value = ''
    await refresh()
  } catch (cause) {
    error.value = cause
  }
}

async function createSecret() {
  if (!session.csrfToken) return
  if (!secretName.value.trim() || !secretValue.value) return
  replacingSecret.value = true
  error.value = undefined
  try {
    const created = await apiFetch<Secret>('/api/admin/v1/secrets', {
      method: 'POST',
      body: JSON.stringify({ name: secretName.value.trim(), value: secretValue.value }),
    }, session.csrfToken)
    selectedSecretId.value = created.secretId
    selectedSecret.value = created
    secretNames.value[created.secretId] = created.name
    justCreatedSecret.value = created
    secretOpen.value = false
    secretName.value = ''
    secretValue.value = ''
  } catch (cause) {
    error.value = cause
  } finally {
    replacingSecret.value = false
  }
}

// --- Existing candidate edit flow ---

function openUpstream(upstream: Upstream) {
  selected.value = upstream
  testResult.value = undefined
  editMode.value = false
  conflictRevision.value = undefined
  const auth = upstream.config?.auth
  if (!auth) return
  const secretId = auth.type === 'BASIC' ? auth.passwordSecretRef?.secretId : ('secretRef' in auth ? auth.secretRef?.secretId : undefined)
  if (secretId && !secretNames.value[secretId]) {
    void resolveSecretPickerOption(secretId).then(option => {
      if (option) secretNames.value[secretId] = option.label
    }).catch(cause => { error.value = cause })
  }
}

function startEdit() {
  if (!selected.value?.config) return
  editUpstreamId.value = selected.value.upstreamId
  editExpectedRevision.value = selected.value.configRevision
  editForm.value = structuredClone(toRaw(selected.value.config))
  editDirty.value = false
  editMode.value = true
  conflictRevision.value = undefined
}

function markEditDirty() {
  editDirty.value = true
  conflictRevision.value = undefined
}

const editSecretId = computed(() => {
  const auth = editForm.value.auth
  if (auth.type === 'BASIC') return auth.passwordSecretRef?.secretId ?? ''
  if (auth.type === 'BEARER' || auth.type === 'STATIC_HEADER') return auth.secretRef?.secretId ?? ''
  return ''
})

function selectEditSecret(option: EntityPickerOption | undefined) {
  const secret = secretFromOption(option)
  if (!secret) return
  const ref = { secretId: secret.secretId, secretVersion: secret.secretVersion }
  const auth = editForm.value.auth
  if (auth.type === 'BASIC') auth.passwordSecretRef = ref
  else if (auth.type === 'BEARER' || auth.type === 'STATIC_HEADER') auth.secretRef = ref
  else return
  markEditDirty()
}

function discardEdit() {
  if (!selected.value?.config) return
  editForm.value = structuredClone(toRaw(selected.value.config))
  editExpectedRevision.value = selected.value.configRevision
  editDirty.value = false
  conflictRevision.value = undefined
}

async function saveEdit() {
  if (!session.csrfToken || !editUpstreamId.value || editExpectedRevision.value === undefined) return
  saving.value = true
  error.value = undefined
  try {
    const updated = await apiFetch<Upstream>(`/api/admin/v1/upstreams/${encodeURIComponent(editUpstreamId.value)}`, {
      method: 'PUT',
      body: JSON.stringify({
        expectedConfigRevision: editExpectedRevision.value,
        config: editForm.value,
      }),
    }, session.csrfToken)
    selected.value = updated
    editExpectedRevision.value = updated.configRevision
    editDirty.value = false
    conflictRevision.value = undefined
    await refresh()
    const refreshed = upstreams.value.find((u) => u.upstreamId === updated.upstreamId)
    if (refreshed) selected.value = refreshed
  } catch (cause) {
    if (cause instanceof ApiProblem && cause.status === 409) {
      conflictRevision.value = cause.currentConfigRevision
    }
    error.value = cause
  } finally {
    saving.value = false
  }
}

function openReplaceSecret(secretIdVal: string, currentVersion: number) {
  secretMode.value = 'replace'
  replaceSecretId.value = secretIdVal
  replaceExpectedVersion.value = currentVersion
  secretName.value = ''
  secretValue.value = ''
  secretOpen.value = true
}

async function replaceSecret() {
  if (!session.csrfToken || !replaceSecretId.value || !replaceExpectedVersion.value || !secretValue.value) return
  replacingSecret.value = true
  error.value = undefined
  try {
    const updated = await apiFetch<Secret>(`/api/admin/v1/secrets/${encodeURIComponent(replaceSecretId.value)}:replace`, {
      method: 'POST',
      body: JSON.stringify({
        expectedSecretVersion: replaceExpectedVersion.value,
        value: secretValue.value,
      }),
    }, session.csrfToken)
    secretNames.value[updated.secretId] = updated.name
    if (selectedSecret.value?.secretId === updated.secretId) selectedSecret.value = updated
    const auth = editForm.value.auth
    if (auth && 'secretRef' in auth && auth.secretRef?.secretId === updated.secretId) {
      auth.secretRef = { secretId: updated.secretId, secretVersion: updated.secretVersion }
      markEditDirty()
    }
    if (auth && 'passwordSecretRef' in auth && auth.passwordSecretRef?.secretId === updated.secretId) {
      auth.passwordSecretRef = { secretId: updated.secretId, secretVersion: updated.secretVersion }
      markEditDirty()
    }
    secretOpen.value = false
    secretValue.value = ''
    secretName.value = ''
  } catch (cause) {
    error.value = cause
  } finally {
    replacingSecret.value = false
  }
}

function handleSecretAction() {
  if (secretMode.value === 'create') {
    return createSecret()
  }
  return replaceSecret()
}

async function testUpstream() {
  if (!selected.value || !session.csrfToken) return
  testing.value = true
  error.value = undefined
  testResult.value = undefined
  try {
    const result = await apiFetch<UpstreamTestResult>(`/api/admin/v1/upstreams/${encodeURIComponent(selected.value.upstreamId)}:test`, {
      method: 'POST',
    }, session.csrfToken)
    testResult.value = result
  } catch (cause) {
    error.value = cause
  } finally {
    testing.value = false
  }
}

async function applyUpstream() {
  if (!selected.value || !session.csrfToken || applying.value) return
  applyConfirmOpen.value = false
  applying.value = true
  const key = activation.beginCommand('RUNTIME_CONFIG', `${selected.value.upstreamId}:${selected.value.configRevision}`)
  error.value = undefined
  try {
    const result = await apiFetch<Activation>(
      `/api/admin/v1/upstreams/${encodeURIComponent(selected.value.upstreamId)}:apply`,
      { method: 'POST', headers: { 'Idempotency-Key': key } },
      session.csrfToken,
    )
    activation.accept(result)
    if (result.state === 'APPLYING' || result.state === 'UNKNOWN') {
      await activation.pollUntilSettled(result.activationId, { timeoutMs: 60_000 })
    }
    await refresh()
    if (selected.value) {
      const updated = upstreams.value.find((u) => u.upstreamId === selected.value?.upstreamId)
      if (updated) selected.value = updated
    }
  } catch (cause) {
    error.value = cause
  } finally {
    applying.value = false
  }
}

// Candidate vs active display
const candidateVsActive = computed(() => {
  if (!selected.value) return null
  const candidate = selected.value.configRevision
  const active = selected.value.activeConfigRevision
  if (active === undefined || active === null) return { candidate, active: null, pending: true }
  if (candidate !== active) return { candidate, active, pending: true }
  return { candidate, active, pending: false }
})

onMounted(refresh)
onBeforeUnmount(() => {
  if (searchTimer) clearTimeout(searchTimer)
})
</script>

<template>
  <q-page class="admin-page" data-cy="upstreams-page">
    <PageHeader :title="$t('upstreams.title')" :subtitle="$t('upstreams.subtitle')">
      <template #actions>
        <q-btn flat icon="refresh" :aria-label="$t('common.refresh')" :loading="loading" @click="refresh" />
        <q-btn outline color="secondary" icon="key" :label="$t('upstreams.createSecret')" :disable="!canMutate" @click="secretMode = 'create'; secretName = ''; secretValue = ''; secretOpen = true" />
        <q-btn color="primary" icon="cloud_queue" :label="$t('upstreams.createUpstream')" :disable="!canMutate" @click="createOpen = true" data-cy="create-upstream-btn" />
      </template>
    </PageHeader>
    <ProblemBanner :error="error" class="q-mb-xs" />
    <q-banner v-if="justCreatedSecret && !createOpen" data-cy="created-secret-notice" class="bg-green-1 q-mb-xs rounded-borders">
      {{ $t('upstreams.createdSecretNotice', { name: justCreatedSecret.name, version: justCreatedSecret.secretVersion }) }}
      <!-- The notice describes a one-time step; without a way to dismiss it, it
           stays on the page for the rest of the session. -->
      <template #action>
        <q-btn flat dense icon="close" :aria-label="$t('common.close')" @click="justCreatedSecret = undefined" />
      </template>
    </q-banner>
    <q-banner v-if="activation.activation" :class="activation.succeeded ? 'bg-green-1' : 'bg-orange-1'" class="q-mb-xs rounded-borders">
      <div class="row items-center justify-between">
        <span>{{ $t('resources.draft.latestOperation') }}</span>
        <StatusChip :value="activation.activation.state" />
      </div>
      <details class="text-caption text-grey-7" data-cy="upstream-activation-details"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ activation.activation.activationId }} · {{ activation.activation.kind }}</details>
      <div v-if="activation.activation.errorCode" class="text-caption text-negative">{{ activation.activation.errorCode }}</div>
      <div v-if="activation.pending" class="text-caption text-grey-7">{{ $t('upstreams.activationRecoveryHint') }}</div>
    </q-banner>
    <DetailWorkspace :detail-open="Boolean(selected)" list-width="minmax(300px, 400px)">
      <template #list>
    <LoadingState v-if="loading && !upstreams.length" />
    <q-card v-else flat bordered>
      <q-card-section class="row items-center justify-between q-py-xs">
        <div class="text-subtitle2">{{ $t('upstreams.title') }}</div>
        <q-input v-model="search" outlined dense clearable :label="$t('common.search')" data-cy="upstream-search" style="width: 100%; max-width: 260px">
          <template #prepend><q-icon name="search" /></template>
        </q-input>
      </q-card-section>
      <q-separator />
      <q-list separator>
        <q-item v-for="upstream in upstreams" :key="upstream.upstreamId" clickable data-cy="upstream-row" @click="openUpstream(upstream)">
          <q-item-section>
            <q-item-label>{{ upstream.name }}</q-item-label>
            <q-item-label caption>
              {{ upstream.activeConfigRevision == null
                ? $t('upstreams.notAppliedHint')
                : upstream.activeConfigRevision !== upstream.configRevision
                  ? $t('upstreams.unappliedChangesHint')
                  : $t('upstreams.appliedHint') }}
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-xs">
              <q-badge v-if="upstream.activeConfigRevision !== undefined && upstream.activeConfigRevision !== upstream.configRevision" color="orange" :label="$t('common.pending')" />
              <StatusChip :value="upstream.status" />
            </div>
          </q-item-section>
        </q-item>
        <q-item v-if="!upstreams.length"><q-item-section class="text-grey-7">{{ $t('upstreams.noUpstreams') }}</q-item-section></q-item>
      </q-list>
      <q-separator />
      <CursorPager :page="pageNumber" :count="upstreams.length" :has-next="Boolean(nextCursor)" :loading="loading" @previous="previousPage" @next="nextPage" />
    </q-card>

    <!-- Create upstream dialog -->
    <q-dialog v-model="createOpen">
      <q-card class="app-dialog">
        <q-card-section class="text-h6">{{ $t('upstreams.createUpstream') }}</q-card-section>
        <q-card-section class="q-gutter-xs app-dialog__body">
          <q-banner class="bg-blue-1 rounded-borders">{{ $t('upstreams.createFlowHint') }}</q-banner>
          <q-input v-model="createForm.name" outlined :label="$t('upstreams.name')" data-cy="upstream-form-name" />
          <q-input v-model="createForm.baseUrl" outlined :label="$t('upstreams.baseUrl')" placeholder="https://api.example.com" data-cy="upstream-form-base-url" />

          <div class="text-subtitle2">{{ $t('upstreams.auth') }}</div>
          <q-select v-model="createForm.auth.type" outlined emit-value map-options :label="$t('upstreams.authMode')" :options="authOptions" />
          <template v-if="createForm.auth.type !== 'NONE'">
            <PagedEntityPicker
              v-model="selectedSecretId"
              :label="$t('upstreams.existingSecret')"
              :empty-label="$t('upstreams.noSecretBound')"
              :fetch-page="fetchSecretPickerPage"
              :resolve-option="resolveSecretPickerOption"
              :selected-option="selectedSecretOption"
              data-cy="create-secret-picker"
              @selected="selectCreateSecret"
            />
            <div v-if="selectedSecret" class="row items-center q-gutter-xs">
              <q-icon name="vpn_key" color="positive" />
              <div class="col">
                <div class="text-body2">{{ selectedSecret.name }}</div>
                <div class="text-caption text-grey-7">{{ $t('upstreams.secretBound', { version: selectedSecret.secretVersion }) }}</div>
              </div>
              <q-btn flat dense color="warning" icon="refresh" :label="$t('upstreams.replaceSecret')" @click="secretMode = 'create'; secretName = ''; secretValue = ''; secretOpen = true" />
            </div>
            <q-banner v-else class="bg-grey-1 rounded-borders">
              <div class="row items-center justify-between">
                <span class="text-body2 text-grey-7">{{ $t('upstreams.noSecretBound') }}</span>
                <q-btn outline color="primary" icon="add_circle" :label="$t('upstreams.createSecret')" size="sm" @click="secretMode = 'create'; secretName = ''; secretValue = ''; secretOpen = true" />
              </div>
            </q-banner>
            <q-input v-if="createForm.auth.type === 'STATIC_HEADER'" v-model="headerName" outlined :label="$t('upstreams.headerName')" placeholder="X-Api-Key" />
            <q-input v-if="createForm.auth.type === 'BASIC'" v-model="username" outlined :label="$t('upstreams.username')" />
          </template>

          <details data-cy="upstream-create-advanced">
            <summary class="text-primary cursor-pointer q-mb-xs">{{ $t('upstreams.advancedSettings') }}</summary>
            <div class="q-gutter-xs">
              <q-select v-model="createForm.transportCapabilities" outlined emit-value map-options :label="$t('upstreams.transportCapabilities')" multiple :options="transportOptions" />
              <q-select v-model="createForm.correlationMode" outlined emit-value map-options :label="$t('upstreams.correlationMode')" :options="correlationOptions" />
              <q-select v-model="createForm.usageCapabilityLevel" outlined emit-value map-options :label="$t('upstreams.usageCapabilityLevel')" :options="usageOptions" />
              <div class="text-subtitle2">{{ $t('upstreams.timeoutDefaults') }} (ms)</div>
              <div class="row q-gutter-xs">
                <q-input v-model.number="createForm.timeoutDefaults.connectMs" type="number" outlined :label="$t('upstreams.connect')" class="col" />
                <q-input v-model.number="createForm.timeoutDefaults.responseHeaderMs" type="number" outlined :label="$t('upstreams.responseHeader')" class="col" />
                <q-input v-model.number="createForm.timeoutDefaults.idleMs" type="number" outlined :label="$t('upstreams.idle')" class="col" />
              </div>
            </div>
          </details>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.cancel')" v-close-popup />
          <q-btn color="primary" :label="$t('common.create')" :disable="!createForm.name.trim() || !createForm.baseUrl.trim()" @click="createUpstream" data-cy="upstream-form-submit" />
        </q-card-actions>
      </q-card>
    </q-dialog>

      </template>
      <template #detail>
      <q-card v-if="selected" flat bordered data-cy="upstream-detail">
        <q-card-section class="row items-start justify-between">
          <div>
            <div class="text-h6">{{ selected.name }}</div>
            <details class="text-caption text-grey-7"><summary class="cursor-pointer">{{ $t('resources.review.technicalDetails') }}</summary>{{ selected.upstreamId }}</details>
          </div>
          <div class="text-right">
            <StatusChip :value="selected.status" />
            <div v-if="candidateVsActive && !candidateVsActive.pending" class="text-caption q-mt-xs">{{ $t('upstreams.appliedHint') }}</div>
          </div>
        </q-card-section>
        <q-separator />

        <q-card-section>
          <!-- Candidate vs Active banner -->
          <q-banner v-if="candidateVsActive?.pending" class="bg-orange-1 q-mb-xs rounded-borders">
            <div class="text-body2">{{ candidateVsActive.active === null ? $t('upstreams.notAppliedHint') : $t('upstreams.unappliedChangesHint') }}</div>
            <div class="text-caption text-grey-7">{{ $t('upstreams.applyHint') }}</div>
          </q-banner>

          <!-- Stale revision banner -->
          <q-banner v-if="conflictRevision !== undefined" class="bg-orange-1 q-mb-xs rounded-borders">
            <div class="text-weight-medium">{{ $t('upstreams.staleCandidate', { rev: editExpectedRevision }) }}</div>
            <div class="text-body2">{{ $t('upstreams.staleHint', { rev: conflictRevision }) }}</div>
            <template #action><q-btn flat :label="$t('upstreams.discardLocal')" @click="discardEdit" /></template>
          </q-banner>

          <!-- Action buttons -->
          <div class="row q-gutter-xs q-mb-xs">
            <q-btn v-if="!editMode" outline color="primary" :label="$t('upstreams.editCandidate')" @click="startEdit" />
            <template v-else>
              <q-btn outline color="primary" :label="$t('upstreams.saveCandidate')" :loading="saving" :disable="!editDirty" @click="saveEdit" />
              <q-btn flat :label="$t('common.discard')" :disable="!editDirty" @click="discardEdit" />
            </template>
            <q-btn outline color="secondary" :label="$t('upstreams.test')" :loading="testing" @click="testUpstream" data-cy="upstream-test-btn" />
            <q-btn outline color="positive" :loading="applying" :label="$t(candidateVsActive?.pending ? 'upstreams.apply' : 'upstreams.reapply')" @click="applyConfirmOpen = true" data-cy="upstream-apply-btn" />
          </div>

          <!-- Read-only or editable config -->
          <template v-if="!editMode">
            <q-markup-table flat dense>
              <tbody>
                <tr v-if="selected.config"><td class="text-grey-7">{{ $t('upstreams.baseUrl') }}</td><td>{{ selected.config.baseUrl ?? '—' }}</td></tr>
                <tr v-if="selected.config"><td class="text-grey-7">{{ $t('upstreams.authMode') }}</td><td>{{ selected.config.auth ? authLabel(selected.config.auth.type) : '—' }}</td></tr>
                <tr v-if="selected.config?.auth?.type === 'STATIC_HEADER'"><td class="text-grey-7">{{ $t('upstreams.headerName') }}</td><td>{{ selected.config.auth.headerName ?? '—' }}</td></tr>
                <tr v-if="selected.config?.auth?.type === 'BASIC'"><td class="text-grey-7">{{ $t('upstreams.username') }}</td><td>{{ selected.config.auth.username ?? '—' }}</td></tr>
                <tr v-if="selected.config?.auth && 'secretRef' in selected.config.auth"><td class="text-grey-7">{{ $t('upstreams.secretRef') }}</td><td>{{ secretDisplayName(selected.config.auth.secretRef?.secretId) }} v{{ selected.config.auth.secretRef?.secretVersion ?? '—' }}</td></tr>
                <tr v-if="selected.config?.auth && 'passwordSecretRef' in selected.config.auth"><td class="text-grey-7">{{ $t('upstreams.passwordSecret') }}</td><td>{{ secretDisplayName(selected.config.auth.passwordSecretRef?.secretId) }} v{{ selected.config.auth.passwordSecretRef?.secretVersion ?? '—' }}</td></tr>
              </tbody>
            </q-markup-table>
            <details data-cy="upstream-detail-advanced" class="q-mt-xs">
              <summary class="text-primary cursor-pointer">{{ $t('upstreams.detailAdvanced') }}</summary>
              <q-markup-table flat dense>
              <tbody>
                <tr><td class="text-grey-7">{{ $t('upstreams.candidateRevision') }}</td><td>{{ selected.configRevision }}</td></tr>
                <tr><td class="text-grey-7">{{ $t('upstreams.activeRevision') }}</td><td>{{ selected.activeConfigRevision ?? '—' }}</td></tr>
                <tr v-if="selected.config"><td class="text-grey-7">{{ $t('upstreams.correlationMode') }}</td><td>{{ selected.config.correlationMode ? correlationLabel(selected.config.correlationMode) : '—' }}</td></tr>
                <tr v-if="selected.config"><td class="text-grey-7">{{ $t('upstreams.usageCapabilityLevel') }}</td><td>{{ selected.config.usageCapabilityLevel ? usageLabel(selected.config.usageCapabilityLevel) : '—' }}</td></tr>
                <tr v-if="selected.config"><td class="text-grey-7">{{ $t('upstreams.transport') }}</td><td>{{ selected.config.transportCapabilities?.map(transportLabel).join(', ') ?? '—' }}</td></tr>
                <tr v-if="selected.config?.timeoutDefaults"><td class="text-grey-7">{{ $t('upstreams.timeoutDefaults') }}</td><td>{{ $t('upstreams.connect') }} {{ selected.config.timeoutDefaults.connectMs }}ms · {{ $t('upstreams.responseHeader') }} {{ selected.config.timeoutDefaults.responseHeaderMs }}ms · {{ $t('upstreams.idle') }} {{ selected.config.timeoutDefaults.idleMs }}ms</td></tr>
              </tbody>
            </q-markup-table>
            </details>

            <!-- Secret replace button -->
            <div v-if="selected.config?.auth && selected.config.auth.type !== 'NONE' && 'secretRef' in selected.config.auth && selected.config.auth.secretRef" class="q-mt-xs">
              <q-btn outline color="warning" icon="key" :label="$t('upstreams.replaceSecret')" size="sm" @click="openReplaceSecret(selected.config.auth.secretRef!.secretId, selected.config.auth.secretRef!.secretVersion)" />
              <span class="text-caption text-grey-7 q-ml-xs">{{ $t('upstreams.replaceSecretHint') }}</span>
            </div>
            <div v-if="selected.config?.auth && selected.config.auth.type === 'BASIC' && 'passwordSecretRef' in selected.config.auth && selected.config.auth.passwordSecretRef" class="q-mt-xs">
              <q-btn outline color="warning" icon="key" :label="$t('upstreams.replacePasswordSecret')" size="sm" @click="openReplaceSecret(selected.config.auth.passwordSecretRef!.secretId, selected.config.auth.passwordSecretRef!.secretVersion)" />
            </div>
          </template>

          <!-- Editable form -->
          <template v-else>
            <div class="q-gutter-xs">
              <q-input v-model="editForm.name" outlined :label="$t('upstreams.name')" @update:model-value="markEditDirty" />
              <q-input v-model="editForm.baseUrl" outlined :label="$t('upstreams.baseUrl')" placeholder="https://api.example.com" @update:model-value="markEditDirty" />
              <div class="text-subtitle2">{{ $t('upstreams.auth') }}</div>
              <q-select v-model="editForm.auth.type" outlined emit-value map-options :label="$t('upstreams.authMode')" :options="authOptions" @update:model-value="markEditDirty" />
              <template v-if="editForm.auth.type !== 'NONE'">
                <PagedEntityPicker
                  :model-value="editSecretId"
                  :label="$t('upstreams.existingSecret')"
                  :empty-label="$t('upstreams.noSecretBound')"
                  :fetch-page="fetchSecretPickerPage"
                  :resolve-option="resolveSecretPickerOption"
                  data-cy="edit-secret-picker"
                  @selected="selectEditSecret"
                />
                <div v-if="'secretRef' in editForm.auth && editForm.auth.secretRef" class="row items-center q-gutter-xs">
                  <q-input :model-value="editForm.auth.secretRef?.secretId" outlined readonly :label="$t('upstreams.secretRef')" class="col" />
                  <q-input :model-value="String(editForm.auth.secretRef?.secretVersion ?? '')" outlined dense readonly :label="$t('upstreams.secretVersion')" class="col-3" />
                  <q-btn flat dense color="warning" icon="key" :label="$t('upstreams.replace')" @click="openReplaceSecret(editForm.auth.secretRef!.secretId, editForm.auth.secretRef!.secretVersion)" />
                </div>
                <q-input v-if="editForm.auth.type === 'STATIC_HEADER'" v-model="editForm.auth.headerName" outlined :label="$t('upstreams.headerName')" placeholder="X-Api-Key" @update:model-value="markEditDirty" />
                <q-input v-if="editForm.auth.type === 'BASIC'" v-model="editForm.auth.username" outlined :label="$t('upstreams.username')" @update:model-value="markEditDirty" />
              </template>
              <details data-cy="upstream-edit-advanced">
                <summary class="text-primary cursor-pointer q-mb-xs">{{ $t('upstreams.advancedSettings') }}</summary>
                <div class="q-gutter-xs">
                  <q-select v-model="editForm.transportCapabilities" outlined emit-value map-options :label="$t('upstreams.transportCapabilities')" multiple :options="transportOptions" @update:model-value="markEditDirty" />
                  <q-select v-model="editForm.correlationMode" outlined emit-value map-options :label="$t('upstreams.correlationMode')" :options="correlationOptions" @update:model-value="markEditDirty" />
                  <q-select v-model="editForm.usageCapabilityLevel" outlined emit-value map-options :label="$t('upstreams.usageCapabilityLevel')" :options="usageOptions" @update:model-value="markEditDirty" />
                  <div class="text-subtitle2">{{ $t('upstreams.timeoutDefaults') }} (ms)</div>
                  <div class="row q-gutter-xs">
                    <q-input v-model.number="editForm.timeoutDefaults.connectMs" type="number" outlined :label="$t('upstreams.connect')" class="col" @update:model-value="markEditDirty" />
                    <q-input v-model.number="editForm.timeoutDefaults.responseHeaderMs" type="number" outlined :label="$t('upstreams.responseHeader')" class="col" @update:model-value="markEditDirty" />
                    <q-input v-model.number="editForm.timeoutDefaults.idleMs" type="number" outlined :label="$t('upstreams.idle')" class="col" @update:model-value="markEditDirty" />
                  </div>
                </div>
              </details>
            </div>
          </template>

          <!-- Test result -->
          <div v-if="testResult" class="q-mt-xs">
            <div class="text-subtitle2">{{ $t('upstreams.testResult') }}</div>
            <div class="text-caption text-grey-7">{{ $t('upstreams.testScope') }}</div>
            <q-banner :class="!testResult.reachable ? 'bg-red-1' : (testResult.httpStatus ?? 0) >= 400 ? 'bg-amber-1' : 'bg-blue-1'" class="rounded-borders q-my-xs">
              <div class="row items-center justify-between">
                <span>{{ testResult.reachable ? $t('upstreams.reachable') : $t('upstreams.unreachable') }}</span>
                <span v-if="testResult.latencyMs != null" class="text-caption">{{ testResult.latencyMs }} ms</span>
              </div>
            </q-banner>
            <q-markup-table flat dense v-if="testResult.httpStatus !== undefined || testResult.warnings?.length">
              <tbody>
                <tr v-if="testResult.httpStatus !== undefined">
                  <td class="text-grey-7">{{ $t('upstreams.httpStatus') }}</td>
                  <td data-cy="upstream-test-http-status">{{ testResult.httpStatus }}</td>
                </tr>
                <tr v-if="testResult.warnings?.length">
                  <td class="text-grey-7">{{ $t('upstreams.warnings') }}</td>
                  <td>{{ testResult.warnings.join('; ') }}</td>
                </tr>
              </tbody>
            </q-markup-table>
          </div>
        </q-card-section>
        <q-card-actions align="right"><q-btn flat icon="arrow_back" :label="$t('common.close')" @click="selected = undefined; editMode = false" /></q-card-actions>
      </q-card>
      </template>
    </DetailWorkspace>

    <q-dialog v-model="applyConfirmOpen">
      <q-card class="app-dialog app-dialog--sm">
        <q-card-section class="text-h6">{{ $t(candidateVsActive?.pending ? 'upstreams.apply' : 'upstreams.reapply') }}</q-card-section>
        <q-card-section>{{ $t(candidateVsActive?.pending ? 'upstreams.applyConfirm' : 'upstreams.reapplyConfirm', { name: selected?.name }) }}</q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.cancel')" v-close-popup />
          <q-btn color="primary" :label="$t(candidateVsActive?.pending ? 'upstreams.apply' : 'upstreams.reapply')" :disable="!canMutate || applying" @click="applyUpstream" data-cy="upstream-apply-confirm" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- Secret create/replace dialog -->
    <q-dialog v-model="secretOpen">
      <q-card class="app-dialog app-dialog--sm">
        <q-card-section class="text-h6">{{ secretMode === 'create' ? $t('upstreams.createSecret') : $t('upstreams.replaceSecret') }}</q-card-section>
        <q-card-section class="q-gutter-xs">
          <p v-if="secretMode === 'create'" class="text-body2 text-grey-7 q-mt-none q-mb-xs">
            {{ $t('upstreams.secretCreateHint') }}
          </p>
          <p v-else class="text-body2 text-grey-7 q-mt-none q-mb-xs">
            {{ $t('upstreams.secretReplaceHint') }}
          </p>
          <q-input v-if="secretMode === 'create'" v-model="secretName" outlined :label="$t('upstreams.secretName')" placeholder="OpenAI key" data-cy="secret-form-name" />
          <q-input v-model="secretValue" outlined :label="$t('upstreams.secretValue')" type="password" autocomplete="new-password" :placeholder="secretMode === 'create' ? 'sk-...' : $t('upstreams.newValue')" data-cy="secret-form-value" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.cancel')" v-close-popup />
          <q-btn
            color="primary"
            :label="secretMode === 'create' ? $t('upstreams.createSecret') : $t('upstreams.replaceSecret')"
            :disable="secretMode === 'create' ? (!secretName.trim() || !secretValue) : !secretValue"
            :loading="replacingSecret"
            @click="handleSecretAction"
            data-cy="secret-form-submit"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>
