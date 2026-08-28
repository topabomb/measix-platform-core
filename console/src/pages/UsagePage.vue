<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import PricingPanel from './PricingPanel.vue'

const { t: $t } = useI18n()

type UsageSummary = components['schemas']['UsageSummary']
type RequestUsagePage = components['schemas']['RequestUsagePage']
type RequestUsage = components['schemas']['RequestUsage']

const activeTab = ref<'summary' | 'pricing'>('summary')
const summary = ref<UsageSummary>()
const requests = ref<RequestUsage[]>([])
const loading = ref(false)
const error = ref<unknown>()
const selectedRequest = ref<RequestUsage>()
const detailOpen = ref(false)
const fromISO = ref<string>()
const toISO = ref<string>()
const userId = ref<string>()
const resourceId = ref<string>()
const resourceKind = ref<string>()
const upstreamId = ref<string>()
const status = ref<string>()
const completeness = ref<string>()
const resourceKinds = ['PROVIDER', 'MODEL', 'TTS', 'ASR', 'MCP']
const statuses = ['SUCCESS', 'ERROR', 'BLOCKED']
const completenesses = ['EXACT', 'PARTIAL', 'UNKNOWN']
const activeFilters = computed(() => {
  const parts: string[] = []
  if (fromISO.value) parts.push(`${$t('usage.filters.time')} ${fromISO.value}`)
  if (toISO.value) parts.push(`${$t('usage.filters.time')} ${toISO.value}`)
  if (userId.value) parts.push(`${$t('usage.filters.user')} ${userId.value}`)
  if (resourceId.value) parts.push(`${$t('usage.filters.resource')} ${resourceId.value}`)
  if (resourceKind.value) parts.push(`${$t('usage.filters.resourceKind')} ${resourceKind.value}`)
  if (upstreamId.value) parts.push(`${$t('usage.filters.upstream')} ${upstreamId.value}`)
  if (status.value) parts.push(status.value)
  if (completeness.value) parts.push(`${$t('usage.filters.completeness')} ${completeness.value}`)
  return parts
})

function applyRange(days: number) {
  const now = new Date()
  const from = new Date(now.getTime() - days * 24 * 60 * 60 * 1000)
  fromISO.value = from.toISOString()
  toISO.value = now.toISOString()
  refresh()
}

function resetFilters() {
  fromISO.value = undefined
  toISO.value = undefined
  userId.value = undefined
  resourceId.value = undefined
  resourceKind.value = undefined
  upstreamId.value = undefined
  status.value = undefined
  completeness.value = undefined
  refresh()
}
const semanticMeterKeys = computed(() => {
  if (!summary.value) return []
  return Object.keys(summary.value.semanticMeters ?? {})
})

const semanticMeters = computed(() => summary.value?.semanticMeters ?? [])

/** Display semantic meters as a flat list — do NOT guess resource kind from meter strings.
 *  Users can filter by resourceKind using the API filter which uses real resource attribution. */
const semanticMetersDisplay = computed(() => {
  return semanticMeters.value
})

const blockedCount = computed(() => {
  if (!summary.value) return 0
  return Math.max(0, summary.value.requestCount - summary.value.forwardedRequestCount)
})

const completenessCounts = computed(() => {
  const counts = { EXACT: 0, PARTIAL: 0, UNKNOWN: 0 }
  for (const m of semanticMeters.value) {
    if (m.confidence in counts) counts[m.confidence as keyof typeof counts] += 1
  }
  return counts
})

function meterColor(meter: string): string {
  if (meter.includes('TOKEN')) return 'primary'
  if (meter === 'CHARACTERS' || meter === 'AUDIO_SECONDS') return 'teal'
  return 'grey'
}

async function refresh() {
  loading.value = true
  error.value = undefined
  const query = new URLSearchParams()
  if (fromISO.value) query.set('from', fromISO.value)
  if (toISO.value) query.set('to', toISO.value)
  if (userId.value) query.set('userId', userId.value)
  if (resourceId.value) query.set('resourceId', resourceId.value)
  if (resourceKind.value) query.set('resourceKind', resourceKind.value)
  if (upstreamId.value) query.set('upstreamId', upstreamId.value)
  if (status.value) query.set('status', status.value)
  if (completeness.value) query.set('completeness', completeness.value)
  const qs = query.toString()
  try {
    const [s, r] = await Promise.all([
      apiFetch<UsageSummary>(`/api/admin/v1/usage/summary${qs ? `?${qs}` : ''}`),
      apiFetch<RequestUsagePage>(`/api/admin/v1/usage/requests?limit=200${qs ? `&${qs}` : ''}`),
    ])
    summary.value = s
    requests.value = r.items
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

function fmtBytes(n: number | undefined): string {
  if (n === undefined) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

const costLabel = computed(() => {
  if (!summary.value) return '—'
  const cost = summary.value.cost
  if (cost.status === 'KNOWN' || cost.status === 'PARTIAL') {
    return `${cost.amount ?? '0'} ${cost.currency ?? ''}`.trim()
  }
  return $t('common.unknown').toLowerCase()
})

const costStatus = computed(() => summary.value?.cost.status ?? 'UNKNOWN')

/** Classify a resource id into its capability kind from the stable id prefix. */
function kindOf(resourceId: string): string | undefined {
  if (resourceId.startsWith('mdl_')) return 'MODEL'
  if (resourceId.startsWith('tts_')) return 'TTS'
  if (resourceId.startsWith('asr_')) return 'ASR'
  if (resourceId.startsWith('mcp_')) return 'MCP'
  return undefined
}

function kindColor(kind: string): string {
  switch (kind) {
    case 'MODEL': return 'primary'
    case 'TTS': return 'teal'
    case 'ASR': return 'indigo'
    case 'MCP': return 'deep-purple'
    default: return 'grey'
  }
}

function costStatusColor(status: string): string {
  switch (status) {
    case 'KNOWN': return 'green'
    case 'PARTIAL': return 'amber'
    default: return 'grey'
  }
}

function costStatusLabel(status: string): string {
  switch (status) {
    case 'KNOWN': return $t('usage.costKnown')
    case 'PARTIAL': return $t('usage.costPartial')
    default: return $t('usage.costUnknown')
  }
}

function openDetail(req: RequestUsage) {
  selectedRequest.value = req
  detailOpen.value = true
}

// Auto-refresh when filters change (Usage Admin UX §14 Filter applies live).
watch(
  [fromISO, toISO, userId, resourceId, resourceKind, upstreamId, status, completeness],
  refresh,
)

onMounted(refresh)
</script>

<template>
  <q-page padding data-cy="usage-page">
    <PageHeader :title="$t('usage.title')" :subtitle="$t('usage.subtitle')">
      <template #actions>
        <q-btn flat icon="refresh" :loading="loading" @click="refresh" />
      </template>
    </PageHeader>
    <q-tabs v-model="activeTab" class="q-mb-md" dense align="left">
      <q-tab name="summary" :label="$t('usage.summary')" icon="insights" />
      <q-tab name="pricing" :label="$t('pricing.title')" icon="sell" />
    </q-tabs>

    <template v-if="activeTab === 'pricing'">
      <PricingPanel />
    </template>

    <div v-else class="row items-end q-col-gutter-sm q-mb-md">
      <div class="col-auto"><q-input v-model="fromISO" outlined dense :label="$t('usage.filters.time')" placeholder="2026-08-01T00:00:00Z" style="width: 190px" /></div>
      <div class="col-auto"><q-input v-model="toISO" outlined dense :label="$t('usage.filters.time')" placeholder="2026-08-31T23:59:59Z" style="width: 190px" /></div>
      <div class="col-auto"><q-btn-dropdown dense flat :label="$t('usage.filters.time')" :no-icon-animation="true" class="q-px-xs">
        <q-list>
          <q-item clickable v-close-popup @click="applyRange(1)"><q-item-section>{{ $t('usage.range24h') }}</q-item-section></q-item>
          <q-item clickable v-close-popup @click="applyRange(7)"><q-item-section>{{ $t('usage.range7d') }}</q-item-section></q-item>
          <q-item clickable v-close-popup @click="applyRange(30)"><q-item-section>{{ $t('usage.range30d') }}</q-item-section></q-item>
        </q-list>
      </q-btn-dropdown></div>
      <div class="col-auto"><q-input v-model="userId" outlined dense :label="$t('usage.filters.user')" placeholder="usr_..." style="width: 160px" /></div>
      <div class="col-auto"><q-input v-model="resourceId" outlined dense :label="$t('usage.filters.resource')" placeholder="mdl_..." style="width: 170px" /></div>
      <div class="col-auto"><q-select v-model="resourceKind" outlined dense :label="$t('usage.filters.resourceKind')" :options="resourceKinds" clearable style="width: 150px" /></div>
      <div class="col-auto"><q-input v-model="upstreamId" outlined dense :label="$t('usage.filters.upstream')" placeholder="ups_..." style="width: 170px" /></div>
      <div class="col-auto"><q-select v-model="status" outlined dense :label="$t('usage.filters.status')" :options="statuses" clearable style="width: 130px" /></div>
      <div class="col-auto"><q-select v-model="completeness" outlined dense :label="$t('usage.filters.completeness')" :options="completenesses" clearable style="width: 150px" /></div>
      <div class="col-auto"><q-btn flat dense icon="filter_alt_off" :label="$t('usage.filters.reset')" :disable="!activeFilters.length" @click="resetFilters" /></div>
    </div>
    <q-banner v-if="activeFilters.length" class="q-mb-md bg-grey-2 rounded-borders">
      <div class="row items-center q-gutter-sm">
        <span class="text-caption text-grey-7">{{ $t('usage.filters.active') }}:</span>
        <q-chip v-for="f in activeFilters" :key="f" dense>{{ f }}</q-chip>
      </div>
    </q-banner>
    <ProblemBanner :error="error" class="q-mb-md" />

    <LoadingState v-if="loading && !summary" />
    <template v-else-if="summary">
      <div class="row q-col-gutter-md q-mb-md">
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('usage.requests') }}</div>
              <div class="text-h5">{{ summary.requestCount }}</div>
              <div class="text-caption">{{ $t('usage.detail.forwarded') }} {{ summary.forwardedRequestCount }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('usage.bytes') }}</div>
              <div class="text-h5">{{ fmtBytes(summary.requestBytes) }}</div>
              <div class="text-caption">{{ $t('usage.responseBytes') }} {{ fmtBytes(summary.responseBytes) }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('overview.costStatus') }}</div>
              <div class="text-h5">{{ costLabel }}</div>
              <q-chip dense :color="costStatusColor(costStatus)" :label="`${$t('overview.costStatus').toLowerCase()} ${costStatus}`" text-color="white" />
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('usage.blocked') }}</div>
              <div class="text-h5">{{ blockedCount }}</div>
              <div class="text-caption">{{ $t('usage.blockedHint') }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('usage.semanticMeters') }}</div>
              <div class="q-mt-sm">
                <q-chip v-for="m in semanticMetersDisplay" :key="m.meter" dense :color="meterColor(m.meter)" text-color="white" size="sm">
                  {{ m.meter }}: {{ m.quantity }}
                  <q-badge v-if="m.confidence === 'UNKNOWN'" color="grey" label="?" class="q-ml-xs" />
                  <q-badge v-else-if="m.confidence === 'PARTIAL'" color="amber" label="~" class="q-ml-xs" />
                </q-chip>
              </div>
              <div v-if="!semanticMeters.length" class="text-body2 text-grey-7">{{ $t('common.none') }}</div>
              <div class="text-caption text-grey-7 q-mt-xs">{{ $t('usage.unknownMetersHint') }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('overview.usageCompleteness') }}</div>
              <div class="row q-gutter-xs items-center">
                <q-chip dense color="green" text-color="white">{{ completenessCounts.EXACT }} {{ $t('status.EXACT').toLowerCase() }}</q-chip>
                <q-chip dense color="amber" text-color="white">{{ completenessCounts.PARTIAL }} {{ $t('status.PARTIAL').toLowerCase() }}</q-chip>
                <q-chip dense color="grey" text-color="white">{{ completenessCounts.UNKNOWN }} {{ $t('status.UNKNOWN').toLowerCase() }}</q-chip>
              </div>
            </q-card-section>
          </q-card>
        </div>
      </div>
    </template>

    <q-card flat bordered>
      <q-card-section><div class="text-subtitle2">{{ $t('usage.requests') }}</div></q-card-section>
      <q-list separator>
        <q-item v-for="req in requests" :key="req.requestId" clickable data-cy="usage-row" @click="openDetail(req)">
          <q-item-section>
            <q-item-label>
              {{ req.requestId }}
              <q-chip v-if="kindOf(req.resourceId)" dense :color="kindColor(kindOf(req.resourceId)!)" text-color="white" size="sm">{{ kindOf(req.resourceId) }}</q-chip>
              <q-chip v-if="req.errorClass" dense color="negative" text-color="white" size="sm">{{ req.errorClass }}</q-chip>
            </q-item-label>
            <q-item-label caption>
              {{ req.startedAt }} · {{ req.resourceId }} · {{ req.upstreamId }}
              <template v-if="req.durationMs !== undefined"> · {{ req.durationMs }} ms</template>
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-sm">
              <q-chip dense :color="req.forwarded ? 'green-2' : 'orange-2'">{{ req.forwarded ? $t('usage.detail.forwarded').toLowerCase() : $t('usage.blocked').toLowerCase() }}</q-chip>
              <q-chip dense :class="req.httpStatus >= 400 ? 'text-negative' : 'text-grey-8'">{{ req.httpStatus }}</q-chip>
              <q-chip v-if="req.upstreamHttpStatus" dense :class="req.upstreamHttpStatus >= 400 ? 'text-negative' : 'text-grey-8'">{{ $t('usage.detail.upstream') }} {{ req.upstreamHttpStatus }}</q-chip>
            </div>
          </q-item-section>
        </q-item>
        <q-item v-if="!requests.length"><q-item-section class="text-grey-7">{{ $t('usage.noRequests') }}</q-item-section></q-item>
      </q-list>
    </q-card>

    <!-- Request Detail (Usage Admin UX §14): never shows prompt/body/Secret. -->
    <q-dialog v-model="detailOpen">
      <q-card class="responsive-modal" style="max-width: 95vw">
        <q-card-section>
          <div class="text-h6">{{ $t('usage.detail.title') }}</div>
          <div class="text-caption text-grey-7">{{ selectedRequest?.requestId }}</div>
        </q-card-section>
        <q-card-section v-if="selectedRequest">
          <q-markup-table flat dense>
            <tbody>
              <tr><td class="text-grey-7">{{ $t('usage.detail.requestId') }}</td><td>{{ selectedRequest.requestId }}</td></tr>
              <tr v-if="selectedRequest.interactionId"><td class="text-grey-7">{{ $t('usage.detail.interactionId') }}</td><td>{{ selectedRequest.interactionId }}</td></tr>
              <tr v-if="selectedRequest.userId"><td class="text-grey-7">{{ $t('usage.detail.user') }}</td><td>{{ selectedRequest.userId }}</td></tr>
              <tr v-if="selectedRequest.deviceId"><td class="text-grey-7">{{ $t('usage.detail.device') }}</td><td>{{ selectedRequest.deviceId }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.resource') }}</td><td>{{ selectedRequest.resourceId }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.upstream') }}</td><td>{{ selectedRequest.upstreamId }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.runtimeRoute') }}</td><td>{{ selectedRequest.runtimeRouteId }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.generation') }}</td><td>{{ $t('releases.generation') }} {{ selectedRequest.managedGeneration }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('overview.desiredRevision') }}</td><td>{{ selectedRequest.controlRevision }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('common.status') }}</td><td>{{ selectedRequest.forwarded ? $t('usage.detail.forwarded').toLowerCase() : $t('usage.blocked').toLowerCase() }} · {{ selectedRequest.httpStatus }} <template v-if="selectedRequest.upstreamHttpStatus">· {{ $t('usage.detail.upstream') }} {{ selectedRequest.upstreamHttpStatus }}</template></td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.duration') }}</td><td>{{ selectedRequest.durationMs }} ms</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.bytes') }}</td><td>{{ fmtBytes(selectedRequest.requestBytes) }} in · {{ fmtBytes(selectedRequest.responseBytes) }} out</td></tr>
              <tr v-if="selectedRequest.errorClass"><td class="text-grey-7">{{ $t('usage.errorClass') }}</td><td><q-chip dense color="negative" text-color="white">{{ selectedRequest.errorClass }}</q-chip></td></tr>
            </tbody>
          </q-markup-table>
          <div class="text-caption text-grey-7 q-mt-sm">{{ $t('usage.detail.secretHint') }}</div>

          <!-- Cost completeness for this request's kind -->
          <div class="text-subtitle2 q-mt-md">{{ $t('overview.costStatus') }} & {{ $t('overview.usageCompleteness') }}</div>
          <div class="row items-center q-gutter-sm q-mt-xs">
            <q-chip dense :color="costStatusColor(costStatus)" text-color="white" :label="costStatusLabel(costStatus)" />
            <q-chip dense :color="costStatusColor(selectedRequest.forwarded ? 'KNOWN' : 'UNKNOWN')" text-color="white" :label="selectedRequest.forwarded ? $t('usage.detail.forwarded').toLowerCase() : $t('usage.blocked').toLowerCase()" />
            <span class="text-caption text-grey-7">{{ $t('usage.costUnknownHint') }}</span>
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.close')" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>
