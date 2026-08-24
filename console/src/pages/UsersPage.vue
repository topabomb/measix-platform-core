<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import { useActivationStore } from '../stores/activation'
import QRCode from 'qrcode'
import { useSessionStore } from '../stores/session'

const { t: $t } = useI18n()

type User = components['schemas']['User']
type UserPage = components['schemas']['UserPage']
type Device = components['schemas']['Device']
type DevicePage = components['schemas']['DevicePage']
type Enrollment = components['schemas']['CreateEnrollmentResponse']
type Activation = components['schemas']['Activation']

const session = useSessionStore()
const activation = useActivationStore()
const users = ref<User[]>([])
const devices = ref<Device[]>([])
const selected = ref<User>()
const loading = ref(false)
const error = ref<unknown>()
const createOpen = ref(false)
const detailOpen = ref(false)
const enrollmentOpen = ref(false)
const enrollment = ref<Enrollment>()
const createForm = ref({ username: '', displayName: '', role: 'MEMBER' as 'ADMIN' | 'MEMBER' })
const canMutate = computed(() => Boolean(session.csrfToken))

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    users.value = (await apiFetch<UserPage>('/api/admin/v1/users?limit=200')).items
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

async function createUser() {
  if (!session.csrfToken) return
  error.value = undefined
  try {
    await apiFetch<User>('/api/admin/v1/users', { method: 'POST', body: JSON.stringify(createForm.value) }, session.csrfToken)
    createOpen.value = false
    createForm.value = { username: '', displayName: '', role: 'MEMBER' }
    await refresh()
  } catch (cause) {
    error.value = cause
  }
}

async function openUser(user: User) {
  selected.value = user
  detailOpen.value = true
  error.value = undefined
  try {
    devices.value = (await apiFetch<DevicePage>(`/api/admin/v1/users/${encodeURIComponent(user.userId)}/devices?limit=200`)).items
  } catch (cause) {
    error.value = cause
  }
}

function clearEnrollment() {
  enrollment.value = undefined
}

const qrCanvas = ref<HTMLCanvasElement>()

async function renderQrCode() {
  if (!enrollment.value?.code || !qrCanvas.value) return
  try {
    await QRCode.toCanvas(qrCanvas.value, enrollment.value.code, { width: 200, margin: 2 })
  } catch {
    // QR rendering is best-effort — do not block the enrollment flow
  }
}

watch(() => enrollment.value, () => { nextTick(renderQrCode) }, { deep: true })

async function createEnrollment() {
  if (!selected.value || !session.csrfToken) return
  error.value = undefined
  try {
    enrollment.value = await apiFetch<Enrollment>(`/api/admin/v1/users/${encodeURIComponent(selected.value.userId)}/enrollments`, {
      method: 'POST', body: JSON.stringify({ expiresInSeconds: 600 }),
    }, session.csrfToken)
    enrollmentOpen.value = true
  } catch (cause) {
    error.value = cause
  }
}

async function runSecurity(path: string) {
  if (!session.csrfToken) return
  const key = activation.beginCommand('SECURITY_CHANGE')
  error.value = undefined
  try {
    const result = await apiFetch<Activation>(path, { method: 'POST', headers: { 'Idempotency-Key': key } }, session.csrfToken)
    activation.accept(result)
    if (result.state === 'APPLYING' || result.state === 'UNKNOWN') await activation.pollUntilSettled(result.activationId, { timeoutMs: 60_000 })
    await refresh()
    if (selected.value) {
      selected.value = users.value.find((item) => item.userId === selected.value?.userId)
      if (selected.value) await openUser(selected.value)
    }
  } catch (cause) {
    error.value = cause
  }
}

async function toggleUser() {
  if (!selected.value) return
  const action = selected.value.status === 'ACTIVE' ? $t('common.disable') : $t('common.enable')
  if (!window.confirm($t('users.disableConfirm', { action, name: selected.value.displayName }))) return
  activation.resetCommand()
  await runSecurity(`/api/admin/v1/users/${encodeURIComponent(selected.value.userId)}:${action === $t('common.disable') ? 'disable' : 'enable'}`)
}

async function revokeDevice(device: Device) {
  if (!window.confirm($t('users.revokeConfirm', { device: device.deviceId }))) return
  activation.resetCommand()
  await runSecurity(`/api/admin/v1/devices/${encodeURIComponent(device.deviceId)}:revoke`)
}

onMounted(refresh)
</script>

<template>
  <q-page padding data-cy="users-page">
    <PageHeader :title="$t('users.title')" :subtitle="$t('users.subtitle')">
      <template #actions>
        <q-btn flat icon="refresh" :loading="loading" @click="refresh" />
        <q-btn color="primary" icon="person_add" :label="$t('users.createUser')" data-cy="create-user-btn" :disable="!canMutate" @click="createOpen = true" />
      </template>
    </PageHeader>
    <ProblemBanner :error="error" class="q-mb-md" />
    <q-banner v-if="activation.activation && !activation.succeeded" class="bg-orange-1 q-mb-md rounded-borders">
      <div class="row items-center justify-between"><span>{{ $t('users.securityActivation', { id: activation.activation.activationId }) }}</span><StatusChip :value="activation.activation.state" /></div>
      <div v-if="activation.activation.errorCode" class="text-caption">{{ activation.activation.errorCode }}</div>
    </q-banner>
    <LoadingState v-if="loading && !users.length" />
    <q-card v-else flat bordered>
      <q-list separator>
        <q-item v-for="user in users" :key="user.userId" clickable data-cy="user-row" @click="openUser(user)">
          <q-item-section><q-item-label>{{ user.displayName }}</q-item-label><q-item-label caption>{{ user.username }} · {{ user.userId }}</q-item-label></q-item-section>
          <q-item-section side><div class="row items-center q-gutter-xs"><q-chip dense>{{ user.role }}</q-chip><StatusChip :value="user.status" /></div></q-item-section>
        </q-item>
        <q-item v-if="!users.length"><q-item-section class="text-grey-7">{{ $t('users.noUsers') }}</q-item-section></q-item>
      </q-list>
    </q-card>

    <q-dialog v-model="createOpen">
      <q-card class="responsive-modal" style="max-width: 95vw">
        <q-card-section class="text-h6">{{ $t('users.createUser') }}</q-card-section>
        <q-card-section class="q-gutter-md">
          <q-input v-model="createForm.username" outlined :label="$t('users.username')" data-cy="user-form-username" />
          <q-input v-model="createForm.displayName" outlined :label="$t('users.displayName')" data-cy="user-form-display-name" />
          <q-select v-model="createForm.role" outlined :label="$t('users.role')" :options="['MEMBER','ADMIN']" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.cancel')" v-close-popup />
          <q-btn color="primary" :label="$t('common.create')" data-cy="user-form-submit" :disable="!createForm.username.trim() || !createForm.displayName.trim()" @click="createUser" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="detailOpen">
      <q-card v-if="selected" style="width: 760px; max-width: 95vw">
        <q-card-section class="row items-start justify-between"><div><div class="text-h6">{{ selected.displayName }}</div><div class="text-caption">{{ selected.userId }}</div></div><StatusChip :value="selected.status" /></q-card-section>
        <q-separator />
        <q-card-section><div class="row q-gutter-sm"><q-btn outline color="primary" :label="$t('users.generateEnrollment')" @click="createEnrollment" data-cy="generate-enrollment-btn" /><q-btn outline :color="selected.status === 'ACTIVE' ? 'negative' : 'positive'" :label="selected.status === 'ACTIVE' ? $t('common.disable') : $t('common.enable')" @click="toggleUser" /></div></q-card-section>
        <q-card-section><div class="text-subtitle2 q-mb-sm">{{ $t('users.devices') }}</div><q-list bordered separator>
          <q-item v-for="device in devices" :key="device.deviceId"><q-item-section><q-item-label>{{ device.deviceId }}</q-item-label><q-item-label caption>{{ device.appVersion ?? $t('common.unknown') }} · {{ $t('users.lastSeen') }} {{ device.lastSeenAt ?? '—' }}</q-item-label></q-item-section><q-item-section side><div class="row items-center q-gutter-sm"><StatusChip :value="device.status" /><q-btn v-if="device.status !== 'REVOKED'" flat dense color="negative" :label="$t('users.revoke')" @click="revokeDevice(device)" /></div></q-item-section></q-item>
          <q-item v-if="!devices.length"><q-item-section class="text-grey-7">{{ $t('users.noDevices') }}</q-item-section></q-item>
        </q-list></q-card-section>
        <q-card-actions align="right"><q-btn flat :label="$t('common.close')" v-close-popup /></q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="enrollmentOpen" @hide="clearEnrollment">
      <q-card v-if="enrollment" class="responsive-modal" style="max-width: 95vw"><q-card-section class="text-h6">{{ $t('users.enrollmentCode') }}</q-card-section><q-card-section>
        <q-banner class="bg-amber-1 q-mb-md rounded-borders">{{ $t('users.enrollmentCodeHint') }}</q-banner>
        <q-input :model-value="enrollment.code" readonly outlined :label="$t('users.enrollmentCode')" data-cy="enrollment-code-field"><template #append><q-btn flat dense icon="content_copy" @click="navigator.clipboard.writeText(enrollment!.code)" /></template></q-input>
        <div class="row justify-center q-mt-md"><div class="text-center"><div class="text-caption q-mb-xs">{{ $t('users.enrollmentQr') }}</div><canvas ref="qrCanvas" data-cy="enrollment-qr" /></div></div>
        <div class="text-caption q-mt-sm">{{ $t('users.expiresAt') }} {{ enrollment.expiresAt }}</div>
      </q-card-section><q-card-actions align="right"><q-btn color="primary" :label="$t('common.done')" v-close-popup /></q-card-actions></q-card>
    </q-dialog>
  </q-page>
</template>
