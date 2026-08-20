<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch, ApiProblem } from '../api/client'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
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
const upstreams = ref<Upstream[]>([])
const selected = ref<Upstream>()
const loading = ref(false)
const error = ref<unknown>()
const createOpen = ref(false)
const detailOpen = ref(false)
const testing = ref(false)
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

const AUTH_TYPES = ['NONE', 'BEARER', 'STATIC_HEADER', 'BASIC'] as const
const USAGE_LEVELS = ['LEVEL_0', 'LEVEL_1', 'LEVEL_2'] as const
const CORRELATION_MODES = ['HEADER_ECHO', 'VIRTUAL_KEY', 'REQUEST_LOG_ID', 'USAGE_API', 'WEBHOOK', 'NONE'] as const
const TRANSPORT_CAPS = ['HTTP_REQUEST_RESPONSE', 'HTTP_STREAMING_SSE', 'HTTP_BINARY_STREAM', 'HTTP_MULTIPART'] as const

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
const secretId = ref('')
const secretVersion = ref<number | undefined>(undefined)
const headerName = ref('')
const username = ref('')
const authSecret = ref('')

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    const page = await apiFetch<UpstreamPage>('/api/admin/v1/upstreams?limit=200')
    upstreams.value = page.items
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

function secretRefFor(value: string): { secretId: string; secretVersion: number } | undefined {
  const v = Number(value)
  if (!secretId.value || !Number.isInteger(v) || v < 1) return undefined
  return { secretId: secretId.value, secretVersion: v }
}

function buildAuth(): UpstreamConfig['auth'] {
  const authType = createForm.value.auth.type
  if (authType === 'NONE') return { type: 'NONE' }
  if (authType === 'BEARER') {
    const ref = secretRefFor(authSecret.value)
    return ref ? { type: 'BEARER', secretRef: ref } : { type: 'BEARER' }
  }
  if (authType === 'STATIC_HEADER') {
    const ref = secretRefFor(authSecret.value)
    return headerName.value.trim()
      ? { type: 'STATIC_HEADER', headerName: headerName.value.trim(), ...(ref ? { secretRef: ref } : {}) }
      : { type: 'STATIC_HEADER', ...(ref ? { secretRef: ref } : {}) }
  }
  if (authType === 'BASIC') {
    const ref = secretRefFor(authSecret.value)
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
    secretId.value = ''
    secretVersion.value = undefined
    authSecret.value = ''
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
    secretId.value = created.secretId
    secretVersion.value = created.secretVersion
    authSecret.value = String(created.secretVersion)
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
  detailOpen.value = true
  editMode.value = false
  conflictRevision.value = undefined
}

function startEdit() {
  if (!selected.value?.config) return
  editUpstreamId.value = selected.value.upstreamId
  editExpectedRevision.value = selected.value.configRevision
  editForm.value = structuredClone(selected.value.config)
  editDirty.value = false
  editMode.value = true
  conflictRevision.value = undefined
}

function markEditDirty() {
  editDirty.value = true
  conflictRevision.value = undefined
}

function discardEdit() {
  if (!selected.value?.config) return
  editForm.value = structuredClone(selected.value.config)
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
  if (!selected.value || !session.csrfToken) return
  if (!window.confirm($t('upstreams.applyConfirm', { name: selected.value.name, rev: selected.value.configRevision }))) return
  activation.resetCommand()
  const key = activation.beginCommand('RUNTIME_CONFIG')
  error.value = undefined
  try {
    const result = await apiFetch<Activation>(
      `/api/admin/v1/upstreams/${encodeURIComponent(selected.value.upstreamId)}:apply`,
      { method: 'POST', headers: { 'Idempotency-Key': key } },
      session.csrfToken,
    )
    activation.accept(result)
    if (result.state === 'APPLYING' || result.state === 'UNKNOWN') {
      await activation.poll(result.activationId)
    }
    await refresh()
    if (selected.value) {
      const updated = upstreams.value.find((u) => u.upstreamId === selected.value?.upstreamId)
      if (updated) selected.value = updated
    }
  } catch (cause) {
    error.value = cause
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
</script>

<template>
  <q-page padding>
    <PageHeader :title="$t('upstreams.title')" :subtitle="$t('upstreams.subtitle')">
      <template #actions>
        <q-btn flat icon="refresh" :loading="loading" @click="refresh" />
        <q-btn outline color="secondary" icon="key" :label="$t('upstreams.createSecret')" :disable="!canMutate" @click="secretMode = 'create'; secretName = ''; secretValue = ''; secretOpen = true" />
        <q-btn color="primary" icon="cloud_queue" :label="$t('upstreams.createUpstream')" :disable="!canMutate" @click="createOpen = true" />
      </template>
    </PageHeader>
    <ProblemBanner :error="error" class="q-mb-md" />
    <q-banner v-if="activation.activation" :class="activation.succeeded ? 'bg-green-1' : 'bg-orange-1'" class="q-mb-md rounded-borders">
      <div class="row items-center justify-between">
        <span>{{ $t('system.currentActivation') }} {{ activation.activation.activationId }} ({{ activation.activation.kind }})</span>
        <StatusChip :value="activation.activation.state" />
      </div>
      <div v-if="activation.activation.errorCode" class="text-caption text-negative">{{ activation.activation.errorCode }}</div>
      <div class="text-caption text-grey-7">{{ $t('upstreams.activationRecoveryHint') }}</div>
    </q-banner>
    <LoadingState v-if="loading && !upstreams.length" />
    <q-card v-else flat bordered>
      <q-list separator>
        <q-item v-for="upstream in upstreams" :key="upstream.upstreamId" clickable @click="openUpstream(upstream)">
          <q-item-section>
            <q-item-label>{{ upstream.name }}</q-item-label>
            <q-item-label caption>
              {{ upstream.upstreamId }}
              · {{ $t('upstreams.candidateRevision') }} {{ upstream.configRevision }}
              <span v-if="upstream.activeConfigRevision"> · {{ $t('upstreams.activeRevision') }} {{ upstream.activeConfigRevision }}</span>
              <span v-else> · {{ $t('upstreams.noActiveRevision') }}</span>
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
    </q-card>

    <!-- Create upstream dialog -->
    <q-dialog v-model="createOpen">
      <q-card style="min-width: 600px; max-width: 95vw">
        <q-card-section class="text-h6">{{ $t('upstreams.createUpstream') }}</q-card-section>
        <q-card-section class="q-gutter-md">
          <q-input v-model="createForm.name" outlined :label="$t('upstreams.name')" />
          <q-input v-model="createForm.baseUrl" outlined :label="$t('upstreams.baseUrl')" placeholder="https://api.example.com" />

          <q-select v-model="createForm.transportCapabilities" outlined :label="$t('upstreams.transportCapabilities')" multiple :options="[...TRANSPORT_CAPS]" />

          <div class="text-subtitle2">{{ $t('upstreams.auth') }}</div>
          <q-select v-model="createForm.auth.type" outlined :label="$t('upstreams.authMode')" :options="[...AUTH_TYPES]" />
          <template v-if="createForm.auth.type !== 'NONE'">
            <q-input v-model="secretId" outlined :label="$t('upstreams.secretRef')" placeholder="sec_..." />
            <q-input v-model="authSecret" outlined :label="$t('upstreams.secretVersion')" placeholder="1" />
            <q-input v-if="createForm.auth.type === 'STATIC_HEADER'" v-model="headerName" outlined :label="$t('upstreams.headerName')" placeholder="X-Api-Key" />
            <q-input v-if="createForm.auth.type === 'BASIC'" v-model="username" outlined :label="$t('upstreams.username')" />
          </template>

          <q-select v-model="createForm.correlationMode" outlined :label="$t('upstreams.correlationMode')" :options="[...CORRELATION_MODES]" />
          <q-select v-model="createForm.usageCapabilityLevel" outlined :label="$t('upstreams.usageCapabilityLevel')" :options="[...USAGE_LEVELS]" />

          <div class="text-subtitle2">{{ $t('upstreams.timeoutDefaults') }} (ms)</div>
          <div class="row q-gutter-sm">
            <q-input v-model.number="createForm.timeoutDefaults.connectMs" type="number" outlined :label="$t('upstreams.connect')" class="col" />
            <q-input v-model.number="createForm.timeoutDefaults.responseHeaderMs" type="number" outlined :label="$t('upstreams.responseHeader')" class="col" />
            <q-input v-model.number="createForm.timeoutDefaults.idleMs" type="number" outlined :label="$t('upstreams.idle')" class="col" />
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.cancel')" v-close-popup />
          <q-btn color="primary" :label="$t('common.create')" :disable="!createForm.name.trim() || !createForm.baseUrl.trim()" @click="createUpstream" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- Upstream detail + editor dialog -->
    <q-dialog v-model="detailOpen">
      <q-card v-if="selected" style="width: 760px; max-width: 95vw">
        <q-card-section class="row items-start justify-between">
          <div>
            <div class="text-h6">{{ selected.name }}</div>
            <div class="text-caption text-grey-7">{{ selected.upstreamId }}</div>
          </div>
          <div class="text-right">
            <StatusChip :value="selected.status" />
            <div v-if="candidateVsActive" class="text-caption q-mt-xs">
              <span :class="candidateVsActive.pending ? 'text-orange' : 'text-grey-7'">
                {{ $t('upstreams.candidateRevision') }} {{ candidateVsActive.candidate }}
                <template v-if="candidateVsActive.active !== null"> · {{ $t('upstreams.activeRevision') }} {{ candidateVsActive.active }}</template>
                <template v-else> · {{ $t('upstreams.noActiveRevision') }}</template>
              </span>
            </div>
          </div>
        </q-card-section>
        <q-separator />

        <q-card-section>
          <!-- Candidate vs Active banner -->
          <q-banner v-if="candidateVsActive?.pending" class="bg-orange-1 q-mb-md rounded-borders">
            <div class="text-body2">{{ $t('upstreams.candidateDiffers', { candidate: candidateVsActive.candidate, active: candidateVsActive.active ?? '—' }) }}</div>
            <div class="text-caption text-grey-7">{{ $t('upstreams.applyHint') }}</div>
          </q-banner>

          <!-- Stale revision banner -->
          <q-banner v-if="conflictRevision !== undefined" class="bg-orange-1 q-mb-md rounded-borders">
            <div class="text-weight-medium">{{ $t('upstreams.staleCandidate', { rev: editExpectedRevision }) }}</div>
            <div class="text-body2">{{ $t('upstreams.staleHint', { rev: conflictRevision }) }}</div>
            <template #action><q-btn flat :label="$t('upstreams.discardLocal')" @click="discardEdit" /></template>
          </q-banner>

          <!-- Action buttons -->
          <div class="row q-gutter-sm q-mb-md">
            <q-btn v-if="!editMode" outline color="primary" :label="$t('upstreams.editCandidate')" @click="startEdit" />
            <template v-else>
              <q-btn outline color="primary" :label="$t('upstreams.saveCandidate')" :loading="saving" :disable="!editDirty" @click="saveEdit" />
              <q-btn flat :label="$t('common.discard')" :disable="!editDirty" @click="discardEdit" />
            </template>
            <q-btn outline color="secondary" :label="$t('upstreams.test')" :loading="testing" @click="testUpstream" />
            <q-btn outline color="positive" :label="$t('upstreams.apply')" @click="applyUpstream" />
          </div>

          <!-- Read-only or editable config -->
          <template v-if="!editMode">
            <q-markup-table flat dense>
              <tbody>
                <tr><td class="text-grey-7">{{ $t('upstreams.candidateRevision') }}</td><td>{{ selected.configRevision }}</td></tr>
                <tr><td class="text-grey-7">{{ $t('upstreams.activeRevision') }}</td><td>{{ selected.activeConfigRevision ?? '—' }}</td></tr>
                <tr v-if="selected.config"><td class="text-grey-7">{{ $t('upstreams.baseUrl') }}</td><td>{{ selected.config.baseUrl ?? '—' }}</td></tr>
                <tr v-if="selected.config"><td class="text-grey-7">{{ $t('upstreams.authMode') }}</td><td>{{ selected.config.auth?.type ?? '—' }}</td></tr>
                <tr v-if="selected.config?.auth?.type === 'STATIC_HEADER'"><td class="text-grey-7">{{ $t('upstreams.headerName') }}</td><td>{{ selected.config.auth.headerName ?? '—' }}</td></tr>
                <tr v-if="selected.config?.auth?.type === 'BASIC'"><td class="text-grey-7">{{ $t('upstreams.username') }}</td><td>{{ selected.config.auth.username ?? '—' }}</td></tr>
                <tr v-if="selected.config?.auth && 'secretRef' in selected.config.auth"><td class="text-grey-7">{{ $t('upstreams.secretRef') }}</td><td>{{ selected.config.auth.secretRef?.secretId ?? '—' }} v{{ selected.config.auth.secretRef?.secretVersion ?? '—' }}</td></tr>
                <tr v-if="selected.config?.auth && 'passwordSecretRef' in selected.config.auth"><td class="text-grey-7">{{ $t('upstreams.passwordSecret') }}</td><td>{{ selected.config.auth.passwordSecretRef?.secretId ?? '—' }} v{{ selected.config.auth.passwordSecretRef?.secretVersion ?? '—' }}</td></tr>
                <tr v-if="selected.config"><td class="text-grey-7">{{ $t('upstreams.correlationMode') }}</td><td>{{ selected.config.correlationMode ?? '—' }}</td></tr>
                <tr v-if="selected.config"><td class="text-grey-7">{{ $t('upstreams.usageCapabilityLevel') }}</td><td>{{ selected.config.usageCapabilityLevel ?? '—' }}</td></tr>
                <tr v-if="selected.config"><td class="text-grey-7">{{ $t('upstreams.transport') }}</td><td>{{ selected.config.transportCapabilities?.join(', ') ?? '—' }}</td></tr>
                <tr v-if="selected.config?.timeoutDefaults"><td class="text-grey-7">{{ $t('upstreams.timeoutDefaults') }}</td><td>{{ $t('upstreams.connect') }} {{ selected.config.timeoutDefaults.connectMs }}ms · {{ $t('upstreams.responseHeader') }} {{ selected.config.timeoutDefaults.responseHeaderMs }}ms · {{ $t('upstreams.idle') }} {{ selected.config.timeoutDefaults.idleMs }}ms</td></tr>
              </tbody>
            </q-markup-table>

            <!-- Secret replace button -->
            <div v-if="selected.config?.auth && selected.config.auth.type !== 'NONE' && 'secretRef' in selected.config.auth && selected.config.auth.secretRef" class="q-mt-md">
              <q-btn outline color="warning" icon="key" :label="$t('upstreams.replaceSecret')" size="sm" @click="openReplaceSecret(selected.config.auth.secretRef!.secretId, selected.config.auth.secretRef!.secretVersion)" />
              <span class="text-caption text-grey-7 q-ml-sm">{{ $t('upstreams.replaceSecretHint') }}</span>
            </div>
            <div v-if="selected.config?.auth && selected.config.auth.type === 'BASIC' && 'passwordSecretRef' in selected.config.auth && selected.config.auth.passwordSecretRef" class="q-mt-md">
              <q-btn outline color="warning" icon="key" :label="$t('upstreams.replacePasswordSecret')" size="sm" @click="openReplaceSecret(selected.config.auth.passwordSecretRef!.secretId, selected.config.auth.passwordSecretRef!.secretVersion)" />
            </div>
          </template>

          <!-- Editable form -->
          <template v-else>
            <div class="q-gutter-md">
              <q-input v-model="editForm.name" outlined :label="$t('upstreams.name')" @update:model-value="markEditDirty" />
              <q-input v-model="editForm.baseUrl" outlined :label="$t('upstreams.baseUrl')" placeholder="https://api.example.com" @update:model-value="markEditDirty" />
              <q-select v-model="editForm.transportCapabilities" outlined :label="$t('upstreams.transportCapabilities')" multiple :options="[...TRANSPORT_CAPS]" @update:model-value="markEditDirty" />
              <div class="text-subtitle2">{{ $t('upstreams.auth') }}</div>
              <q-select v-model="editForm.auth.type" outlined :label="$t('upstreams.authMode')" :options="[...AUTH_TYPES]" @update:model-value="markEditDirty" />
              <template v-if="editForm.auth.type !== 'NONE'">
                <div v-if="'secretRef' in editForm.auth && editForm.auth.secretRef" class="row items-center q-gutter-sm">
                  <q-input :model-value="editForm.auth.secretRef?.secretId" outlined readonly :label="$t('upstreams.secretRef')" class="col" />
                  <q-input :model-value="String(editForm.auth.secretRef?.secretVersion ?? '')" outlined readonly :label="$t('upstreams.secretVersion')" style="width: 100px" />
                  <q-btn flat dense color="warning" icon="key" :label="$t('upstreams.replace')" @click="openReplaceSecret(editForm.auth.secretRef!.secretId, editForm.auth.secretRef!.secretVersion)" />
                </div>
                <q-input v-if="editForm.auth.type === 'STATIC_HEADER'" v-model="editForm.auth.headerName" outlined :label="$t('upstreams.headerName')" placeholder="X-Api-Key" @update:model-value="markEditDirty" />
                <q-input v-if="editForm.auth.type === 'BASIC'" v-model="editForm.auth.username" outlined :label="$t('upstreams.username')" @update:model-value="markEditDirty" />
              </template>
              <q-select v-model="editForm.correlationMode" outlined :label="$t('upstreams.correlationMode')" :options="[...CORRELATION_MODES]" @update:model-value="markEditDirty" />
              <q-select v-model="editForm.usageCapabilityLevel" outlined :label="$t('upstreams.usageCapabilityLevel')" :options="[...USAGE_LEVELS]" @update:model-value="markEditDirty" />
              <div class="text-subtitle2">{{ $t('upstreams.timeoutDefaults') }} (ms)</div>
              <div class="row q-gutter-sm">
                <q-input v-model.number="editForm.timeoutDefaults.connectMs" type="number" outlined :label="$t('upstreams.connect')" class="col" @update:model-value="markEditDirty" />
                <q-input v-model.number="editForm.timeoutDefaults.responseHeaderMs" type="number" outlined :label="$t('upstreams.responseHeader')" class="col" @update:model-value="markEditDirty" />
                <q-input v-model.number="editForm.timeoutDefaults.idleMs" type="number" outlined :label="$t('upstreams.idle')" class="col" @update:model-value="markEditDirty" />
              </div>
            </div>
          </template>

          <!-- Test result -->
          <div v-if="testResult" class="q-mt-md">
            <div class="text-subtitle2">{{ $t('upstreams.testResult') }}</div>
            <q-banner :class="testResult.reachable ? 'bg-green-1' : 'bg-red-1'" class="rounded-borders q-my-sm">
              <div class="row items-center justify-between">
                <span>{{ testResult.reachable ? $t('upstreams.reachable') : $t('upstreams.unreachable') }}</span>
                <span v-if="testResult.latencyMs != null" class="text-caption">{{ testResult.latencyMs }} ms</span>
              </div>
            </q-banner>
            <q-markup-table flat dense v-if="testResult.verifiedCapabilities?.length || testResult.warnings?.length">
              <tbody>
                <tr v-if="testResult.verifiedCapabilities?.length">
                  <td class="text-grey-7">{{ $t('upstreams.verifiedCapabilities') }}</td>
                  <td>{{ testResult.verifiedCapabilities.join(', ') }}</td>
                </tr>
                <tr v-if="testResult.warnings?.length">
                  <td class="text-grey-7">{{ $t('upstreams.warnings') }}</td>
                  <td>{{ testResult.warnings.join('; ') }}</td>
                </tr>
              </tbody>
            </q-markup-table>
          </div>
        </q-card-section>
        <q-card-actions align="right"><q-btn flat :label="$t('common.close')" v-close-popup /></q-card-actions>
      </q-card>
    </q-dialog>

    <!-- Secret create/replace dialog -->
    <q-dialog v-model="secretOpen">
      <q-card style="min-width: 480px; max-width: 95vw">
        <q-card-section class="text-h6">{{ secretMode === 'create' ? $t('upstreams.createSecret') : $t('upstreams.replaceSecret') }}</q-card-section>
        <q-card-section class="q-gutter-md">
          <p v-if="secretMode === 'create'" class="text-body2 text-grey-7 q-mt-none q-mb-sm">
            {{ $t('upstreams.secretCreateHint') }}
          </p>
          <p v-else class="text-body2 text-grey-7 q-mt-none q-mb-sm">
            {{ $t('upstreams.secretReplaceHint') }}
          </p>
          <q-input v-if="secretMode === 'create'" v-model="secretName" outlined :label="$t('upstreams.secretName')" placeholder="OpenAI key" />
          <q-input v-model="secretValue" outlined :label="$t('upstreams.secretValue')" type="password" autocomplete="new-password" :placeholder="secretMode === 'create' ? 'sk-...' : $t('upstreams.newValue')" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.cancel')" v-close-popup />
          <q-btn
            color="primary"
            :label="secretMode === 'create' ? $t('upstreams.createSecret') : $t('upstreams.replaceSecret')"
            :disable="secretMode === 'create' ? (!secretName.trim() || !secretValue) : !secretValue"
            :loading="replacingSecret"
            @click="handleSecretAction"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>
