<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { copyToClipboard } from 'quasar'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import { encodeEnrollmentMaterial } from '../api/enrollment'
import { cursorPath } from '../api/pagination'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import UsageRequestList from '../components/UsageRequestList.vue'
import { useActivationStore } from '../stores/activation'
import QRCode from 'qrcode'
import { useSessionStore } from '../stores/session'

const { t: $t } = useI18n()
const router = useRouter()

type User = components['schemas']['User']
type UserPage = components['schemas']['UserPage']
type Device = components['schemas']['Device']
type DevicePage = components['schemas']['DevicePage']
type Enrollment = components['schemas']['CreateEnrollmentResponse']
type Activation = components['schemas']['Activation']
type UsageSummary = components['schemas']['UsageSummary']

const session = useSessionStore()
const activation = useActivationStore()
const users = ref<User[]>([])
const nextCursor = ref<string>()
const listPath = ref('/api/admin/v1/users?limit=200')
// A paginated list without a search box is the worst of both: an operator can
// only reach an account by paging through all of them in creation order.
const search = ref('')
let listSequence = 0

async function loadMore() {
  if (!nextCursor.value || loading.value) return
  const current = listSequence
  loading.value = true
  try {
    const page = await apiFetch<UserPage>(cursorPath(listPath.value, nextCursor.value))
    if (current !== listSequence) return
    users.value.push(...page.items)
    nextCursor.value = page.nextCursor
  } catch (cause) { if (current === listSequence) error.value = cause } finally { if (current === listSequence) loading.value = false }
}

const devices = ref<Device[]>([])
const devicesTruncated = ref(false)
const loadingDevices = ref(false)
let deviceSequence = 0
const selected = ref<User>()
const loading = ref(false)
const error = ref<unknown>()
const createOpen = ref(false)
const detailOpen = ref(false)
const enrollmentOpen = ref(false)
const enrollment = ref<Enrollment>()
const enrollmentMaterial = ref('')
const enrollmentCopied = ref(false)
const createForm = ref({ username: '', displayName: '', role: 'MEMBER' as 'ADMIN' | 'MEMBER' })
const canMutate = computed(() => Boolean(session.csrfToken))

async function refresh() {
  const current = ++listSequence
  loading.value = true
  error.value = undefined
  try {
    const query = new URLSearchParams({ limit: '200' })
    if (search.value.trim()) query.set('query', search.value.trim())
    listPath.value = `/api/admin/v1/users?${query.toString()}`
    const page = await apiFetch<UserPage>(listPath.value)
    if (current !== listSequence) return
    users.value = page.items
    nextCursor.value = page.nextCursor
  } catch (cause) {
    if (current === listSequence) error.value = cause
  } finally {
    if (current === listSequence) loading.value = false
  }
}

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { void refresh() }, 300)
})

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
  applyUsagePeriod()
  await Promise.all([loadDevices(user), loadUsage()])
}

// The previous user's devices are cleared and every response is checked against
// the current selection: opening A and then B must never show A's devices under
// B's name, which is what a stale, unguarded fetch did.
async function loadDevices(user: User) {
  const current = ++deviceSequence
  loadingDevices.value = true
  devices.value = []
  devicesTruncated.value = false
  try {
    const page = await apiFetch<DevicePage>(`/api/admin/v1/users/${encodeURIComponent(user.userId)}/devices?limit=200`)
    if (current !== deviceSequence) return
    devices.value = page.items
    devicesTruncated.value = Boolean(page.nextCursor)
  } catch (cause) {
    if (current === deviceSequence) error.value = cause
  } finally {
    if (current === deviceSequence) loadingDevices.value = false
  }
}

// Usage for one user is the question this page is opened to answer, so the
// summary and the requests are read here instead of sending the operator to the
// Usage page to paste an identifier by hand.
const usagePeriods = [1, 7, 30]
const usagePeriod = ref(7)
const periodOptions = computed(() => usagePeriods.map(days => ({
  label: $t(days === 1 ? 'usage.range24h' : days === 7 ? 'usage.range7d' : 'usage.range30d'),
  value: days,
})))
const usage = ref<UsageSummary>()
const loadingUsage = ref(false)
const usageFrom = ref<string>()
const usageTo = ref<string>()
let usageSequence = 0

function applyUsagePeriod() {
  const to = new Date()
  usageTo.value = to.toISOString()
  usageFrom.value = new Date(to.getTime() - usagePeriod.value * 24 * 60 * 60 * 1000).toISOString()
}

function userUsageQuery(userId: string): string {
  const query = new URLSearchParams({ from: usageFrom.value ?? '', to: usageTo.value ?? '', userId })
  return `&${query.toString()}`
}

const usageQuery = computed(() => (selected.value && usageFrom.value ? userUsageQuery(selected.value.userId) : ''))

async function loadUsage() {
  if (!selected.value) return
  const current = ++usageSequence
  loadingUsage.value = true
  usage.value = undefined
  try {
    const query = userUsageQuery(selected.value.userId).replace(/^&/, '')
    const result = await apiFetch<UsageSummary>(`/api/admin/v1/usage/summary?${query}`)
    if (current !== usageSequence) return
    usage.value = result
  } catch (cause) {
    if (current === usageSequence) error.value = cause
  } finally {
    if (current === usageSequence) loadingUsage.value = false
  }
}

watch(usagePeriod, () => {
  applyUsagePeriod()
  void loadUsage()
})

function viewAllUsage() {
  if (!selected.value) return
  void router.push({
    name: 'Usage',
    query: { userId: selected.value.userId, from: usageFrom.value, to: usageTo.value },
  }).catch(() => {})
}

function fmtBytes(n: number | undefined): string {
  if (n === undefined) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

const usageCost = computed(() => {
  const cost = usage.value?.cost
  if (cost && (cost.status === 'KNOWN' || cost.status === 'PARTIAL')) {
    return `${cost.amount ?? '0'} ${cost.currency ?? ''}`.trim()
  }
  return $t('usage.costUnknown')
})

function clearEnrollment() {
  enrollmentCopied.value = false
  enrollment.value = undefined
  enrollmentMaterial.value = ''
}

const qrCanvas = ref<HTMLCanvasElement>()

async function renderQrCode() {
  if (!enrollmentMaterial.value || !qrCanvas.value) return
  try {
    await QRCode.toCanvas(qrCanvas.value, enrollmentMaterial.value, { width: 280, margin: 4 })
  } catch {
    // QR rendering is best-effort — do not block the enrollment flow
  }
}

watch(() => enrollment.value, () => { nextTick(renderQrCode) }, { deep: true })

async function createEnrollment() {
  if (!selected.value || !session.csrfToken) return
  error.value = undefined
  try {
    const grant = await apiFetch<Enrollment>(`/api/admin/v1/users/${encodeURIComponent(selected.value.userId)}/enrollments`, {
      method: 'POST', body: JSON.stringify({ expiresInSeconds: 600 }),
    }, session.csrfToken)
    // Loopback HTTP is supported for isolated local deployments; other origins require HTTPS.
    enrollmentMaterial.value = encodeEnrollmentMaterial(grant.platformUrl, grant)
    enrollment.value = grant
    enrollmentOpen.value = true
  } catch (cause) {
    error.value = cause
  }
}

async function runSecurity(path: string) {
  if (!session.csrfToken) return
  const key = activation.beginCommand('SECURITY_CHANGE', path)
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
  await runSecurity(`/api/admin/v1/users/${encodeURIComponent(selected.value.userId)}:${action === $t('common.disable') ? 'disable' : 'enable'}`)
}

async function revokeDevice(device: Device) {
  if (!window.confirm($t('users.revokeConfirm', { device: device.deviceId }))) return
  await runSecurity(`/api/admin/v1/devices/${encodeURIComponent(device.deviceId)}:revoke`)
}

async function copyEnrollment() {
  if (!enrollmentMaterial.value) return
  const material = enrollmentMaterial.value
  enrollmentCopied.value = false
  try {
    await copyToClipboard(material)
    if (enrollmentMaterial.value === material) enrollmentCopied.value = true
  } catch (cause) { error.value = cause }
}

onMounted(refresh)
onBeforeUnmount(() => {
  listSequence++
  deviceSequence++
  usageSequence++
  if (searchTimer) clearTimeout(searchTimer)
})
</script>

<template>
  <q-page padding data-cy="users-page">
    <PageHeader :title="$t('users.title')" :subtitle="$t('users.subtitle')">
      <template #actions>
        <q-btn flat icon="refresh" :aria-label="$t('common.refresh')" :loading="loading" @click="refresh" />
        <q-btn color="primary" icon="person_add" :label="$t('users.createUser')" data-cy="create-user-btn" :disable="!canMutate" @click="createOpen = true" />
      </template>
    </PageHeader>
    <ProblemBanner :error="error" class="q-mb-md" />
    <q-banner v-if="activation.activation && !activation.succeeded" class="bg-orange-1 q-mb-md rounded-borders">
      <div class="row items-center justify-between"><span>{{ $t('users.securityActivation', { id: activation.activation.activationId }) }}</span><StatusChip :value="activation.activation.state" /></div>
      <div v-if="activation.activation.errorCode" class="text-caption">{{ activation.activation.errorCode }}</div>
    </q-banner>
    <div class="row q-mb-md">
      <q-input v-model="search" outlined dense clearable debounce="0" :label="$t('users.search')" :hint="$t('usage.filters.userHint')" data-cy="user-search" style="width: 340px">
        <template #prepend><q-icon name="search" /></template>
      </q-input>
    </div>
    <LoadingState v-if="loading && !users.length" />
    <q-card v-else flat bordered>
      <q-list separator>
        <q-item v-for="user in users" :key="user.userId" clickable data-cy="user-row" @click="openUser(user)">
          <q-item-section><q-item-label>{{ user.displayName }}</q-item-label><q-item-label caption>{{ user.username }}</q-item-label></q-item-section>
          <q-item-section side><div class="row items-center q-gutter-xs"><q-chip dense>{{ $t(`roles.${user.role}`) }}</q-chip><StatusChip :value="user.status" /></div></q-item-section>
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
          <q-select v-model="createForm.role" outlined :label="$t('users.role')" :options="['MEMBER','ADMIN'].map(value => ({label: $t(`roles.${value}`), value}))" emit-value map-options />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.cancel')" v-close-popup />
          <q-btn color="primary" :label="$t('common.create')" data-cy="user-form-submit" :disable="!createForm.username.trim() || !createForm.displayName.trim()" @click="createUser" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="detailOpen">
      <!-- Bounded and internally scrollable: this dialog carries the devices and
           the usage for one account, and it must not push its own actions off
           screen when either grows. -->
      <q-card v-if="selected" style="width: 820px; max-width: 95vw; max-height: 90vh; display: flex; flex-direction: column">
        <q-card-section class="row items-start justify-between"><div><div class="text-h6">{{ selected.displayName }}</div><div class="text-caption">{{ selected.username }} · {{ $t(`roles.${selected.role}`) }}</div><details class="text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ selected.userId }}</details></div><StatusChip :value="selected.status" /></q-card-section>
        <q-separator />
        <!-- min-height:0 lets this flex child shrink below its content height so
             the body scrolls instead of stretching the card. -->
        <div style="flex: 1 1 auto; min-height: 0; overflow-y: auto">
        <q-card-section><div class="row q-gutter-sm"><q-btn outline no-caps color="primary" :label="$t('users.generateEnrollment')" @click="createEnrollment" data-cy="generate-enrollment-btn" /><q-btn outline :color="selected.status === 'ACTIVE' ? 'negative' : 'positive'" :label="selected.status === 'ACTIVE' ? $t('common.disable') : $t('common.enable')" @click="toggleUser" /></div></q-card-section>
        <q-card-section><div class="text-subtitle2 q-mb-sm">{{ $t('users.devices') }}</div>
          <div v-if="loadingDevices" class="text-caption text-grey-7 q-mb-sm" data-cy="devices-loading">{{ $t('common.loading') }}</div>
          <q-list bordered separator data-cy="user-devices">
          <q-item v-for="device in devices" :key="device.deviceId">
            <q-item-section>
              <q-item-label>{{ device.deviceName }}</q-item-label>
              <q-item-label caption>{{ $t('users.appVersion') }} {{ device.appVersion ?? $t('common.unknown') }} · {{ $t('users.lastSeen') }} {{ device.lastSeenAt ? new Date(device.lastSeenAt).toLocaleString() : '—' }}</q-item-label>
              <q-item-label data-cy="device-application-state">{{ $t(`users.application.${device.applicationState}`) }}</q-item-label>
              <q-item-label v-if="device.appliedReportedAt" caption>{{ $t('users.appliedReport', { applied: device.appliedManagedGeneration, target: device.targetManagedGeneration, time: new Date(device.appliedReportedAt).toLocaleString() }) }}</q-item-label>
              <details data-cy="device-identity" class="text-caption q-mt-xs">
                <summary>{{ $t('resources.review.technicalDetails') }}</summary>
                <div class="text-break">{{ device.deviceId }}</div>
              </details>
            </q-item-section>
            <q-item-section side>
              <div class="row items-center q-gutter-sm">
                <StatusChip :value="device.status" />
                <q-btn v-if="device.status !== 'REVOKED'" flat dense color="negative" :label="$t('users.revoke')" @click="revokeDevice(device)" />
              </div>
            </q-item-section>
          </q-item>
          <q-item v-if="!devices.length && !loadingDevices"><q-item-section class="text-grey-7">{{ $t('users.noDevices') }}</q-item-section></q-item>
        </q-list>
        <div v-if="devicesTruncated" class="text-caption text-grey-7 q-mt-xs">{{ $t('users.devicesTruncated', { count: devices.length }) }}</div>
        </q-card-section>
        <q-card-section data-cy="user-usage">
          <div class="row items-center justify-between q-mb-sm">
            <div class="text-subtitle2">{{ $t('users.usage') }}</div>
            <q-select v-model="usagePeriod" :options="periodOptions" :label="$t('users.usagePeriod')" outlined dense emit-value map-options data-cy="user-usage-period" style="width: 180px" />
          </div>
          <div class="row q-col-gutter-lg" data-cy="user-usage-summary">
            <div><div class="text-caption text-grey-7">{{ $t('usage.requests') }}</div><div class="text-h6">{{ usage?.requestCount ?? '—' }}</div></div>
            <div><div class="text-caption text-grey-7">{{ $t('usage.detail.forwarded') }}</div><div class="text-h6">{{ usage?.forwardedRequestCount ?? '—' }}</div></div>
            <div><div class="text-caption text-grey-7">{{ $t('usage.bytes') }}</div><div class="text-h6">{{ usage ? fmtBytes(usage.requestBytes) : '—' }}</div></div>
            <div><div class="text-caption text-grey-7">{{ $t('overview.costStatus') }}</div><div class="text-h6">{{ usageCost }}</div></div>
          </div>
          <div class="text-caption text-grey-7 q-mt-sm">{{ $t('users.usageHint') }}</div>
          <div v-if="loadingUsage" class="text-caption text-grey-7 q-mt-sm">{{ $t('common.loading') }}</div>
          <div class="text-subtitle2 q-mt-md q-mb-sm">{{ $t('users.recentRequests') }}</div>
          <UsageRequestList :query="usageQuery" :page-size="25" max-height="none" :show-user="false" />
          <div class="q-mt-sm"><q-btn flat color="primary" no-caps :label="$t('users.viewAllUsage')" data-cy="view-all-usage" @click="viewAllUsage" /></div>
        </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right"><q-btn flat :label="$t('common.close')" v-close-popup /></q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="enrollmentOpen" @hide="clearEnrollment">
      <q-card v-if="enrollment" class="responsive-modal" style="max-width: 95vw"><q-card-section class="text-h6">{{ $t('users.enrollmentTitle') }}</q-card-section><q-card-section>
        <q-banner class="bg-amber-1 q-mb-md rounded-borders">{{ $t('users.enrollmentCodeHint') }}</q-banner>
        <div class="row justify-center"><div class="text-center"><div class="text-body2 q-mb-xs">{{ $t('users.enrollmentQr') }}</div><canvas ref="qrCanvas" data-cy="enrollment-qr" /></div></div>
        <q-input :model-value="enrollmentMaterial" readonly outlined autogrow :label="$t('users.enrollmentMaterial')" data-cy="enrollment-material-field" class="q-mt-md"><template #append><q-btn flat dense icon="content_copy" :aria-label="$t('users.enrollmentMaterial')" data-cy="copy-enrollment-material" @click="copyEnrollment()" /></template></q-input>
        <div v-if="enrollmentCopied" role="status" class="text-positive q-mt-sm" data-cy="enrollment-copy-result">{{ $t('common.copied') }}</div>
        <div class="text-caption q-mt-sm">{{ $t('users.expiresAt') }} {{ new Date(enrollment.expiresAt).toLocaleString() }}</div>
        <details data-cy="enrollment-code-details" class="q-mt-md"><summary class="text-primary cursor-pointer">{{ $t('users.showCodeForTroubleshooting') }}</summary>
          <q-input :model-value="enrollment.code" readonly outlined :label="$t('users.enrollmentCode')" data-cy="enrollment-code-field" class="q-mt-sm" />
        </details>
      </q-card-section><q-card-actions align="right"><q-btn color="primary" :label="$t('common.done')" v-close-popup /></q-card-actions></q-card>
    </q-dialog>
    <q-btn v-if="nextCursor" outline :label="$t('common.loadMore')" :loading="loading" @click="loadMore" data-cy="load-more" class="q-mt-md" />
  </q-page>
</template>
