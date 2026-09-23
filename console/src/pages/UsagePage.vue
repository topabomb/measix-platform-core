<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import type { MeterQuantity, PricingMeter, UsageDistribution, UsageTrend, UserUsagePage } from '../api/usageBudget'
import { clientProtocols } from '../api/usageBudget'
import { formatMeter, type MeterUnitLabels } from '../usageFormatting'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import UsageRequestList from '../components/UsageRequestList.vue'
import PricingPanel from './PricingPanel.vue'
import UsageReconciliationPanel from '../components/UsageReconciliationPanel.vue'
import PagedEntityPicker, { type EntityPickerOption } from '../components/PagedEntityPicker.vue'
import CursorPager from '../components/CursorPager.vue'
import { fetchUpstreamPickerPage, fetchUserPickerPage, resolveUpstreamPickerOption, resolveUserPickerOption } from '../api/entityPickerSources'
import { useCursorPager } from '../composables/useCursorPager'

const { t: $t, locale } = useI18n()
const route = useRoute()
const router = useRouter()

type UsageSummary = components['schemas']['UsageSummary']

const activeTab = ref<'summary' | 'requests' | 'reconciliation' | 'pricing'>('summary')
const requestList = ref<{ refresh: () => Promise<void> }>()
const summary = ref<UsageSummary>()
const trend = ref<UsageTrend>()
const distribution = ref<UsageDistribution>()
const error = ref<unknown>()
const loading = ref(false)
const lastSuccessfulAt = ref<Date>()
let summarySequence = 0

// Usage is a time series: an unbounded default made every visit aggregate the
// whole history. The window is always explicit and starts at the last 24 hours.
const fromISO = ref<string>()
const toISO = ref<string>()
const allTime = ref(false)

function localDateTime(value: string | undefined): string {
  if (!value) return ''
  const date = new Date(value)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
}
const fromLocal = computed({
  get: () => localDateTime(fromISO.value),
  set: (value: string) => { fromISO.value = value ? new Date(value).toISOString() : undefined },
})
const toLocal = computed({
  get: () => localDateTime(toISO.value),
  set: (value: string) => { toISO.value = value ? new Date(value).toISOString() : undefined },
})

const userId = ref<string>()
const resourceId = ref<string>()
const resourceKind = ref<string>()
const upstreamId = ref<string>()
const status = ref<string>()
const completeness = ref<string>()
const clientProtocol = ref<string>()
const budgetStatus = ref<string>()
const resourceKinds = ['PROVIDER', 'MODEL', 'IMAGE_GENERATION', 'TTS', 'ASR', 'MCP']
const statuses = ['SUCCESS', 'ERROR', 'BLOCKED']
const completenesses = ['EXACT', 'PARTIAL', 'UNKNOWN']
const budgetStatuses = ['EXHAUSTED', 'NEAR_LIMIT', 'PENDING_RECONCILIATION']
const pageSizes = [25, 50, 100, 200]
const pageSize = ref(50)
const usageUsersPath = ref('/api/admin/v1/usage/users?limit=25')
const {
  items: usageUserItems,
  nextCursor: usageUsersNextCursor,
  pageNumber: usageUsersPage,
  loading: usageUsersLoading,
  error: usageUsersError,
  reset: resetUsageUsers,
  nextPage: nextUsageUsersPage,
  previousPage: previousUsageUsersPage,
} = useCursorPager<UserUsagePage['items'][number], UserUsagePage>(usageUsersPath, async path => {
  const page = await apiFetch<UserUsagePage>(path)
  return { ...page, items: (page.items ?? []).filter(item => item.budget?.items) }
})

const selectedUserOption = ref<EntityPickerOption>()
const selectedUpstreamOption = ref<EntityPickerOption>()
const advancedFiltersOpen = ref(false)
const advancedFilterCount = computed(() => [
  resourceId.value,
  resourceKind.value,
  status.value,
  completeness.value,
  clientProtocol.value,
  budgetStatus.value,
].filter(Boolean).length)

const activeFilters = computed(() => {
  const parts: string[] = []
  if (allTime.value) parts.push($t('usage.rangeAll'))
  else {
    if (fromISO.value) parts.push(`${$t('usage.filters.startTime')} ${new Date(fromISO.value).toLocaleString()}`)
    if (toISO.value) parts.push(`${$t('usage.filters.endTime')} ${new Date(toISO.value).toLocaleString()}`)
  }
  if (userId.value) {
    parts.push(`${$t('usage.filters.user')} ${selectedUserOption.value?.label ?? userId.value}`)
  }
  if (resourceId.value) parts.push(`${$t('usage.filters.resource')} ${resourceId.value}`)
  if (resourceKind.value) parts.push(`${$t('usage.filters.resourceKind')} ${resourceKind.value}`)
  if (upstreamId.value) parts.push(`${$t('usage.filters.upstream')} ${selectedUpstreamOption.value?.label ?? upstreamId.value}`)
  if (status.value) parts.push(`${$t('status.' + status.value)}`)
  if (completeness.value) parts.push(`${$t('usage.filters.completeness')} ${$t('status.' + completeness.value)}`)
  if (clientProtocol.value) parts.push(`${$t('usage.filters.protocol')} ${clientProtocol.value}`)
  if (budgetStatus.value) parts.push(`${$t('usage.filters.budgetStatus')} ${$t('budgets.status.' + budgetStatus.value)}`)
  return parts
})

function applyRange(days: number) {
  const now = new Date()
  allTime.value = false
  fromISO.value = new Date(now.getTime() - days * 24 * 60 * 60 * 1000).toISOString()
  toISO.value = now.toISOString()
}

function applyAllTime() {
  allTime.value = true
  fromISO.value = undefined
  toISO.value = undefined
}

function resetFilters() {
  resourceId.value = undefined
  userId.value = undefined
  resourceKind.value = undefined
  upstreamId.value = undefined
  status.value = undefined
  completeness.value = undefined
  clientProtocol.value = undefined
  budgetStatus.value = undefined
  applyRange(1)
}

/** Encoded filter set shared by the summary and the request list. */
const filterQuery = computed(() => {
  const query = new URLSearchParams()
  if (fromISO.value) query.set('from', fromISO.value)
  if (toISO.value) query.set('to', toISO.value)
  if (userId.value) query.set('userId', userId.value)
  if (resourceId.value) query.set('resourceId', resourceId.value)
  if (resourceKind.value) query.set('resourceKind', resourceKind.value)
  if (upstreamId.value) query.set('upstreamId', upstreamId.value)
  if (status.value) query.set('status', status.value)
  if (completeness.value) query.set('completeness', completeness.value)
  if (clientProtocol.value) query.set('clientProtocol', clientProtocol.value)
  const encoded = query.toString()
  return encoded ? `&${encoded}` : ''
})

async function refresh() {
  const sequence = ++summarySequence
  loading.value = true
  error.value = undefined
  try {
    const encoded = filterQuery.value.replace(/^&/, '')
    const userQuery = new URLSearchParams(encoded)
    userQuery.delete('userId')
    userQuery.set('limit', '25')
    if (budgetStatus.value) userQuery.set('budgetStatus', budgetStatus.value)
    usageUsersPath.value = `/api/admin/v1/usage/users?${userQuery.toString()}`
    const [summaryResult, trendResult, distributionResult] = await Promise.all([
      apiFetch<UsageSummary>(`/api/admin/v1/usage/summary?${encoded}`),
      apiFetch<UsageTrend>(`/api/admin/v1/usage/trend?${encoded}`),
      apiFetch<UsageDistribution>(`/api/admin/v1/usage/distribution?${encoded}`),
      resetUsageUsers(),
    ])
    if (sequence !== summarySequence) return
    summary.value = summaryResult
    trend.value = Array.isArray(trendResult.points)
      ? trendResult
      : { from: summaryResult.from, to: summaryResult.to, timezone: '', points: [] }
    distribution.value = Array.isArray(distributionResult.items) && distributionResult.items.every(item =>
      Boolean(item.resourceKind && item.clientProtocol && Array.isArray(item.semanticMeters)))
      ? distributionResult
      : { from: summaryResult.from, to: summaryResult.to, items: [] }
    lastSuccessfulAt.value = new Date()
  } catch (cause) {
    if (sequence === summarySequence) error.value = cause
  } finally {
    if (sequence === summarySequence) loading.value = false
  }
}

async function refreshCurrent() {
  if (activeTab.value === 'requests') await requestList.value?.refresh()
  else if (activeTab.value === 'summary') await refresh()
}

// The summary aggregates server-side, so free-text identifiers wait for typing to
// settle instead of re-aggregating on every keystroke. Selects and dates apply at
// once. Both paths end in the same refresh, which also re-reads the list because
// the list follows filterQuery.
let textTimer: ReturnType<typeof setTimeout> | undefined
const textFilters = computed(() => `${resourceId.value ?? ''}\u0000${upstreamId.value ?? ''}`)
watch(textFilters, () => {
  if (textTimer) clearTimeout(textTimer)
  textTimer = setTimeout(() => { if (activeTab.value === 'summary') void refresh() }, 300)
})
watch([fromISO, toISO, userId, resourceKind, status, completeness, clientProtocol, budgetStatus, pageSize, allTime], () => {
  if (activeTab.value === 'summary') void refresh()
})
watch(activeTab, tab => { if (tab === 'summary') void refresh() })

// Keeping the filters in the URL makes a per-user view linkable, back-navigable
// and refresh-stable, which is what an operator sends to a colleague.
watch([filterQuery, budgetStatus, pageSize, activeTab], () => {
  const query: Record<string, string> = {}
  new URLSearchParams(filterQuery.value.replace(/^&/, '')).forEach((value, key) => { query[key] = value })
  if (pageSize.value !== 50) query.pageSize = String(pageSize.value)
  if (budgetStatus.value) query.budgetStatus = budgetStatus.value
  if (activeTab.value !== 'summary') query.tab = activeTab.value
  void router.replace({ query }).catch(() => {})
})

const semanticMeters = computed(() => summary.value?.semanticMeters ?? [])
const hasStaleData = computed(() => Boolean(error.value && summary.value))

const blockedCount = computed(() => {
  if (!summary.value) return 0
  return Math.max(0, summary.value.requestCount - summary.value.forwardedRequestCount)
})

function meterColor(meter: string): string {
  if (meter.includes('TOKEN')) return 'primary'
  if (meter === 'CHARACTERS' || meter === 'AUDIO_SECONDS') return 'teal'
  return 'grey'
}

const unitLabels = computed<MeterUnitLabels>(() => ({
  tokens: $t('usage.units.tokens'),
  characters: $t('usage.units.characters'),
  seconds: $t('usage.units.seconds'),
  minutes: $t('usage.units.minutes'),
  requests: $t('usage.units.requests'),
  images: $t('usage.units.images'),
}))

function meterLabel(meter: PricingMeter): string {
  return $t(`usage.meters.${meter}`)
}

function meterValue(item: MeterQuantity, compact = true): string {
  return formatMeter(item.quantity, item.meter, locale.value, unitLabels.value, compact)
}

function userBudgetStatus(page: UserUsagePage['items'][number]): string {
  const pending = page.budget.items.find(item => item.status === 'PENDING_RECONCILIATION')
  if (pending) return $t('budgets.status.PENDING_RECONCILIATION')
  const exhausted = page.budget.items.find(item => item.status === 'EXHAUSTED')
  if (exhausted) return $t('budgets.status.EXHAUSTED')
  const near = page.budget.items.some(item => item.mode === 'LIMITED' && item.limits.some(limit => {
    const cap = BigInt(limit.limit)
    const consumed = BigInt(limit.used) + BigInt(limit.reserved)
    return cap > 0n && consumed < cap && consumed * 5n >= cap * 4n
  }))
  if (near) return $t('budgets.status.NEAR_LIMIT')
  return $t('budgets.status.AVAILABLE')
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

onMounted(async () => {
  const initial = route.query
  if (typeof initial.userId === 'string' && initial.userId) userId.value = initial.userId
  if (typeof initial.resourceId === 'string' && initial.resourceId) resourceId.value = initial.resourceId
  if (typeof initial.resourceKind === 'string' && initial.resourceKind) resourceKind.value = initial.resourceKind
  if (typeof initial.upstreamId === 'string' && initial.upstreamId) upstreamId.value = initial.upstreamId
  if (typeof initial.status === 'string' && initial.status) status.value = initial.status
  if (typeof initial.completeness === 'string' && initial.completeness) completeness.value = initial.completeness
  if (typeof initial.clientProtocol === 'string' && initial.clientProtocol) clientProtocol.value = initial.clientProtocol
  if (typeof initial.budgetStatus === 'string' && budgetStatuses.includes(initial.budgetStatus)) budgetStatus.value = initial.budgetStatus
  if (typeof initial.tab === 'string' && initial.tab === 'pricing') activeTab.value = 'pricing'
  if (typeof initial.tab === 'string' && initial.tab === 'reconciliation') activeTab.value = 'reconciliation'
  if (typeof initial.tab === 'string' && initial.tab === 'requests') activeTab.value = 'requests'
  const size = Number(initial.pageSize)
  if (pageSizes.includes(size)) pageSize.value = size
  if (typeof initial.from === 'string' && typeof initial.to === 'string') {
    fromISO.value = initial.from
    toISO.value = initial.to
  } else {
    applyRange(1)
  }
  if (activeTab.value === 'summary') await refresh()
})

onBeforeUnmount(() => {
  summarySequence++
  if (textTimer) clearTimeout(textTimer)
})
</script>

<template>
  <q-page class="admin-page" data-cy="usage-page">
    <PageHeader :title="$t('usage.title')" :subtitle="$t('usage.subtitle')">
      <template #actions>
        <q-btn flat dense icon="refresh" :aria-label="$t('common.refresh')" :loading="loading" @click="refreshCurrent" />
      </template>
    </PageHeader>
    <q-tabs v-model="activeTab" class="q-mb-xs" dense align="left">
      <q-tab name="summary" :label="$t('usage.summary')" icon="insights" />
      <q-tab name="requests" :label="$t('usage.requests')" icon="receipt_long" />
      <q-tab name="reconciliation" :label="$t('usage.reconciliation.tab')" icon="rule" />
      <q-tab name="pricing" :label="$t('pricing.title')" icon="sell" />
    </q-tabs>

    <PricingPanel v-if="activeTab === 'pricing'" />
    <UsageReconciliationPanel v-else-if="activeTab === 'reconciliation'" />

    <template v-else>
      <div class="usage-filters usage-filters--primary q-mb-xs">
        <q-input v-model="fromLocal" type="datetime-local" outlined dense stack-label :label="$t('usage.filters.startTime')" :disable="allTime" />
        <q-input v-model="toLocal" type="datetime-local" outlined dense stack-label :label="$t('usage.filters.endTime')" :disable="allTime" />
        <q-btn-dropdown dense outline no-caps class="usage-filters__cell-btn" :label="$t('usage.filters.quickRange')" :no-icon-animation="true">
          <q-list>
            <q-item clickable v-close-popup @click="applyRange(1)"><q-item-section>{{ $t('usage.range24h') }}</q-item-section></q-item>
            <q-item clickable v-close-popup @click="applyRange(7)"><q-item-section>{{ $t('usage.range7d') }}</q-item-section></q-item>
            <q-item clickable v-close-popup @click="applyRange(30)"><q-item-section>{{ $t('usage.range30d') }}</q-item-section></q-item>
            <q-item clickable v-close-popup @click="applyAllTime"><q-item-section>{{ $t('usage.rangeAll') }}</q-item-section></q-item>
          </q-list>
        </q-btn-dropdown>
        <!-- The user picker filters like the others; it is not a companion of
             the date range. No hint: it made this one control stand taller
             than the whole row. -->
        <PagedEntityPicker
          v-model="userId"
          :label="$t('usage.filters.user')"
          :empty-label="$t('usage.filters.anyUser')"
          :fetch-page="fetchUserPickerPage"
          :resolve-option="resolveUserPickerOption"
          data-cy="usage-user-filter"
          @selected="selectedUserOption = $event"
        />
        <PagedEntityPicker
          v-model="upstreamId"
          :label="$t('usage.filters.upstream')"
          :empty-label="$t('common.all')"
          :fetch-page="fetchUpstreamPickerPage"
          :resolve-option="resolveUpstreamPickerOption"
          data-cy="usage-upstream-filter"
          @selected="selectedUpstreamOption = $event"
        />
        <q-btn outline dense no-caps align="left" class="usage-filters__cell-btn" icon="tune" :label="$t('usage.filters.more', { count: advancedFilterCount })" :aria-expanded="advancedFiltersOpen" data-cy="usage-more-filters" @click="advancedFiltersOpen = !advancedFiltersOpen" />
        <q-btn flat dense align="left" class="usage-filters__cell-btn" icon="filter_alt_off" :label="$t('usage.filters.reset')" :disable="!activeFilters.length" @click="resetFilters" />
      </div>
      <div v-show="advancedFiltersOpen" class="usage-filters usage-filters--advanced q-mb-xs" data-cy="usage-advanced-filters">
        <q-select v-model="resourceKind" outlined dense :label="$t('usage.filters.resourceKind')" :options="resourceKinds" clearable />
        <q-select v-model="status" outlined dense :label="$t('usage.filters.status')" :options="statuses" clearable />
        <q-select v-model="completeness" outlined dense :label="$t('usage.filters.completeness')" :options="completenesses" clearable />
        <q-select v-model="clientProtocol" outlined dense :label="$t('usage.filters.protocol')" :options="clientProtocols" clearable data-cy="usage-protocol-filter" />
        <q-select v-model="budgetStatus" outlined dense :label="$t('usage.filters.budgetStatus')" :options="budgetStatuses.map(value => ({ label: $t('budgets.status.' + value), value }))" emit-value map-options clearable data-cy="usage-budget-status-filter" />
        <q-input v-model="resourceId" outlined dense :label="$t('usage.filters.resource')" placeholder="mdl_..." />
      </div>
      <ProblemBanner :error="error" class="q-mb-xs" />
      <q-banner v-if="loading && summary" class="bg-blue-1 q-mb-xs" data-cy="usage-refreshing">
        {{ $t('usage.refreshing', { time: lastSuccessfulAt?.toLocaleString() ?? '—' }) }}
      </q-banner>
      <q-banner v-if="hasStaleData" class="bg-orange-1 q-mb-xs" data-cy="usage-stale">
        {{ $t('usage.stale', { time: lastSuccessfulAt?.toLocaleString() ?? '—' }) }}
      </q-banner>

      <template v-if="activeTab === 'summary'">
      <LoadingState v-if="loading && !summary" />
      <div v-else-if="summary" class="row q-col-gutter-xs q-mb-xs">
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('usage.requests') }}</div>
              <div class="text-h6" data-cy="usage-request-count">{{ summary.requestCount }}</div>
              <div class="text-caption">{{ $t('usage.detail.forwarded') }} {{ summary.forwardedRequestCount }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('usage.bytes') }}</div>
              <div class="text-h6">{{ fmtBytes(summary.requestBytes) }}</div>
              <div class="text-caption">{{ $t('usage.responseBytes') }} {{ fmtBytes(summary.responseBytes) }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('overview.costStatus') }}</div>
              <div class="text-h6">{{ costLabel }}</div>
              <q-chip dense :color="costStatusColor(costStatus)" :label="costStatusLabel(costStatus)" text-color="white" />
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('usage.blocked') }}</div>
              <div class="text-h6">{{ blockedCount }}</div>
              <div class="text-caption">{{ $t('usage.blockedHint') }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('usage.semanticMeters') }}</div>
              <div class="q-mt-xs">
                <q-chip v-for="m in semanticMeters" :key="m.meter" dense :color="meterColor(m.meter)" text-color="white" size="sm">
                  {{ meterLabel(m.meter) }}: {{ meterValue(m) }}
                  <q-badge v-if="m.confidence === 'UNKNOWN'" color="grey" label="?" class="q-ml-xs" />
                  <q-badge v-else-if="m.confidence === 'PARTIAL'" color="amber" label="~" class="q-ml-xs" />
                </q-chip>
              </div>
              <div v-if="!semanticMeters.length" class="text-body2 text-grey-7">{{ $t('usage.noSemanticMeters') }}</div>
              <div class="text-caption text-grey-7 q-mt-xs">{{ $t('usage.unknownMetersHint') }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('overview.usageCompleteness') }}</div>
              <div class="row q-gutter-xs items-center" data-cy="request-completeness">
                <q-chip dense color="green" text-color="white">{{ summary.requestCompleteness.exact }} {{ $t('status.EXACT').toLowerCase() }}</q-chip>
                <q-chip dense color="amber" text-color="white">{{ summary.requestCompleteness.partial }} {{ $t('status.PARTIAL').toLowerCase() }}</q-chip>
                <q-chip dense color="grey" text-color="white">{{ summary.requestCompleteness.unknown }} {{ $t('status.UNKNOWN').toLowerCase() }}</q-chip>
              </div>
            </q-card-section>
          </q-card>
        </div>
      </div>

      <div v-if="summary" class="usage-analysis q-mb-xs" data-cy="usage-analysis">
        <q-card flat bordered class="usage-analysis__card">
          <q-card-section class="q-pb-xs">
            <div class="text-subtitle2">{{ $t('usage.trend.title') }}</div>
            <div class="text-caption text-grey-7">{{ trend ? $t('usage.trend.timezone', { timezone: trend.timezone }) : $t('common.loading') }}</div>
          </q-card-section>
          <q-list v-if="trend?.points.length" dense separator class="usage-analysis__list">
            <q-item v-for="point in trend.points" :key="point.date">
              <q-item-section>
                <q-item-label>{{ new Date(`${point.date}T00:00:00`).toLocaleDateString() }}</q-item-label>
                <q-item-label caption>{{ $t('usage.trend.requestsLine', { requests: point.requestCount, forwarded: point.forwardedRequestCount }) }}</q-item-label>
                <q-item-label v-if="point.semanticMeters.length" caption class="text-break">
                  {{ point.semanticMeters.map(item => `${meterLabel(item.meter)} ${meterValue(item)}`).join(' · ') }}
                </q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
          <q-card-section v-else class="text-grey-7">{{ $t('usage.trend.empty') }}</q-card-section>
        </q-card>

        <q-card flat bordered class="usage-analysis__card">
          <q-card-section class="q-pb-xs">
            <div class="text-subtitle2">{{ $t('usage.distribution.title') }}</div>
            <div class="text-caption text-grey-7">{{ $t('usage.distribution.hint') }}</div>
          </q-card-section>
          <q-list v-if="distribution?.items.length" dense separator class="usage-analysis__list">
            <q-item v-for="item in distribution.items" :key="`${item.resourceKind}:${item.clientProtocol}:${item.resourceId ?? ''}`">
              <q-item-section>
                <q-item-label>{{ item.resourceDisplayName || item.resourceId || $t(`usage.kind.${item.resourceKind}`) }}</q-item-label>
                <q-item-label caption class="text-break">{{ item.clientProtocol }} · {{ $t('usage.distribution.requests', { count: item.requestCount }) }}</q-item-label>
                <q-item-label v-if="item.semanticMeters.length" caption class="text-break">
                  {{ item.semanticMeters.map(meter => `${meterLabel(meter.meter)} ${meterValue(meter)}`).join(' · ') }}
                </q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
          <q-card-section v-else class="text-grey-7">{{ $t('usage.distribution.empty') }}</q-card-section>
        </q-card>

        <q-card flat bordered class="usage-analysis__card">
          <q-card-section class="q-pb-xs">
            <div class="text-subtitle2">{{ $t('usage.users.title') }}</div>
            <div class="text-caption text-grey-7">{{ $t('usage.users.hint') }}</div>
          </q-card-section>
          <ProblemBanner :error="usageUsersError" class="card-inset q-mb-xs" />
          <q-list v-if="usageUserItems.length" dense separator class="usage-analysis__list">
            <q-item v-for="item in usageUserItems" :key="item.userId">
              <q-item-section>
                <q-item-label>{{ item.userDisplayName }}</q-item-label>
                <q-item-label caption>{{ $t('usage.users.requests', { count: item.requestCount }) }} · {{ userBudgetStatus(item) }}</q-item-label>
                <q-item-label v-if="item.semanticMeters.length" caption class="text-break">
                  {{ item.semanticMeters.map(meter => `${meterLabel(meter.meter)} ${meterValue(meter)}`).join(' · ') }}
                </q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
          <q-card-section v-else-if="!usageUsersLoading" class="text-grey-7">{{ $t('usage.users.empty') }}</q-card-section>
          <CursorPager
            :page="usageUsersPage"
            :count="usageUserItems.length"
            :has-next="Boolean(usageUsersNextCursor)"
            :loading="usageUsersLoading"
            @previous="previousUsageUsersPage"
            @next="nextUsageUsersPage"
          />
        </q-card>
      </div>

      </template>

      <UsageRequestList v-else ref="requestList" :query="filterQuery" :page-size="pageSize" allow-filter-resource @filter-resource="value => { resourceId = value }">
        <template #toolbar>
          <q-select v-model="pageSize" :options="pageSizes" :label="$t('usage.pageSize')" outlined dense emit-value map-options style="width: 150px" data-cy="usage-page-size" />
        </template>
      </UsageRequestList>
    </template>
  </q-page>
</template>

<style scoped>
/* Equal filter cells: controls stretch to the cell, the grid wraps in whole
   columns, and buttons match the 40px dense field height so the row reads as
   one line of same-shaped controls. */
.usage-filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 4px;
  align-items: end;
}

.usage-filters__cell-btn {
  width: 100%;
  height: 40px;
}

.usage-analysis {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 4px;
}

.usage-analysis__card {
  min-width: 0;
}

.usage-analysis__list {
  max-height: 22rem;
  overflow-y: auto;
}

@media (max-width: 900px) {
  .usage-analysis {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 420px) {
  .usage-filters {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
