<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import UsageRequestList from '../components/UsageRequestList.vue'
import PricingPanel from './PricingPanel.vue'

const { t: $t } = useI18n()
const route = useRoute()
const router = useRouter()

type UsageSummary = components['schemas']['UsageSummary']
type User = components['schemas']['User']
type UserPage = components['schemas']['UserPage']

const activeTab = ref<'summary' | 'pricing'>('summary')
const summary = ref<UsageSummary>()
const error = ref<unknown>()
const loading = ref(false)
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
const resourceKinds = ['PROVIDER', 'MODEL', 'TTS', 'ASR', 'MCP']
const statuses = ['SUCCESS', 'ERROR', 'BLOCKED']
const completenesses = ['EXACT', 'PARTIAL', 'UNKNOWN']
const pageSizes = [25, 50, 100, 200]
const pageSize = ref(50)

// Users are chosen, not typed: requiring an operator to know a usr_ identifier by
// heart is what made per-user filtering unusable.
const userOptions = ref<{ label: string; value: string }[]>([])
const loadingUsers = ref(false)
let userSearchTimer: ReturnType<typeof setTimeout> | undefined

async function loadUsers(term: string, keep?: string) {
  loadingUsers.value = true
  try {
    const query = new URLSearchParams({ limit: '50' })
    if (term.trim()) query.set('query', term.trim())
    const page = await apiFetch<UserPage>(`/api/admin/v1/users?${query.toString()}`)
    const options = page.items.map((user: User) => ({
      label: user.username ? `${user.displayName} (${user.username})` : user.displayName,
      value: user.userId,
    }))
    // A selection reached through a link must stay visible even when it is not in
    // the first page of results, so fall back to showing its identifier.
    if (keep && !options.some(option => option.value === keep)) {
      options.unshift({ label: keep, value: keep })
    }
    userOptions.value = options
  } catch (cause) {
    error.value = cause
  } finally {
    loadingUsers.value = false
  }
}

function onUserFilter(term: string, update: (callback: () => void) => void) {
  if (userSearchTimer) clearTimeout(userSearchTimer)
  userSearchTimer = setTimeout(() => {
    void loadUsers(term, userId.value).then(() => update(() => {}))
  }, 250)
}

const activeFilters = computed(() => {
  const parts: string[] = []
  if (allTime.value) parts.push($t('usage.rangeAll'))
  else {
    if (fromISO.value) parts.push(`${$t('usage.filters.startTime')} ${new Date(fromISO.value).toLocaleString()}`)
    if (toISO.value) parts.push(`${$t('usage.filters.endTime')} ${new Date(toISO.value).toLocaleString()}`)
  }
  if (userId.value) {
    const option = userOptions.value.find(candidate => candidate.value === userId.value)
    parts.push(`${$t('usage.filters.user')} ${option?.label ?? userId.value}`)
  }
  if (resourceId.value) parts.push(`${$t('usage.filters.resource')} ${resourceId.value}`)
  if (resourceKind.value) parts.push(`${$t('usage.filters.resourceKind')} ${resourceKind.value}`)
  if (upstreamId.value) parts.push(`${$t('usage.filters.upstream')} ${upstreamId.value}`)
  if (status.value) parts.push(`${$t('status.' + status.value)}`)
  if (completeness.value) parts.push(`${$t('usage.filters.completeness')} ${$t('status.' + completeness.value)}`)
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
  const encoded = query.toString()
  return encoded ? `&${encoded}` : ''
})

async function refresh() {
  const sequence = ++summarySequence
  loading.value = true
  error.value = undefined
  summary.value = undefined
  try {
    const result = await apiFetch<UsageSummary>(`/api/admin/v1/usage/summary?${filterQuery.value.replace(/^&/, '')}`)
    if (sequence !== summarySequence) return
    summary.value = result
  } catch (cause) {
    if (sequence === summarySequence) error.value = cause
  } finally {
    if (sequence === summarySequence) loading.value = false
  }
}

// The summary aggregates server-side, so free-text identifiers wait for typing to
// settle instead of re-aggregating on every keystroke. Selects and dates apply at
// once. Both paths end in the same refresh, which also re-reads the list because
// the list follows filterQuery.
let textTimer: ReturnType<typeof setTimeout> | undefined
const textFilters = computed(() => `${resourceId.value ?? ''}\u0000${upstreamId.value ?? ''}`)
watch(textFilters, () => {
  if (textTimer) clearTimeout(textTimer)
  textTimer = setTimeout(() => { void refresh() }, 300)
})
watch([fromISO, toISO, userId, resourceKind, status, completeness, pageSize, allTime], () => {
  void refresh()
})

// Keeping the filters in the URL makes a per-user view linkable, back-navigable
// and refresh-stable, which is what an operator sends to a colleague.
watch([filterQuery, pageSize, activeTab], () => {
  const query: Record<string, string> = {}
  new URLSearchParams(filterQuery.value.replace(/^&/, '')).forEach((value, key) => { query[key] = value })
  if (pageSize.value !== 50) query.pageSize = String(pageSize.value)
  if (activeTab.value !== 'summary') query.tab = activeTab.value
  void router.replace({ query }).catch(() => {})
})

const semanticMeters = computed(() => summary.value?.semanticMeters ?? [])

const blockedCount = computed(() => {
  if (!summary.value) return 0
  return Math.max(0, summary.value.requestCount - summary.value.forwardedRequestCount)
})

function meterColor(meter: string): string {
  if (meter.includes('TOKEN')) return 'primary'
  if (meter === 'CHARACTERS' || meter === 'AUDIO_SECONDS') return 'teal'
  return 'grey'
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
  if (typeof initial.tab === 'string' && initial.tab === 'pricing') activeTab.value = 'pricing'
  const size = Number(initial.pageSize)
  if (pageSizes.includes(size)) pageSize.value = size
  if (typeof initial.from === 'string' && typeof initial.to === 'string') {
    fromISO.value = initial.from
    toISO.value = initial.to
  } else {
    applyRange(1)
  }
  await Promise.all([refresh(), loadUsers('', userId.value)])
})

onBeforeUnmount(() => {
  summarySequence++
  if (textTimer) clearTimeout(textTimer)
  if (userSearchTimer) clearTimeout(userSearchTimer)
})
</script>

<template>
  <q-page padding data-cy="usage-page">
    <PageHeader :title="$t('usage.title')" :subtitle="$t('usage.subtitle')">
      <template #actions>
        <q-btn flat icon="refresh" :aria-label="$t('common.refresh')" :loading="loading" @click="refresh" />
      </template>
    </PageHeader>
    <q-tabs v-model="activeTab" class="q-mb-md" dense align="left">
      <q-tab name="summary" :label="$t('usage.summary')" icon="insights" />
      <q-tab name="pricing" :label="$t('pricing.title')" icon="sell" />
    </q-tabs>

    <PricingPanel v-if="activeTab === 'pricing'" />

    <template v-else>
      <div class="row items-end q-col-gutter-sm q-mb-md">
        <div class="col-auto"><q-input v-model="fromLocal" type="datetime-local" outlined dense stack-label :label="$t('usage.filters.startTime')" :disable="allTime" style="width: 230px" /></div>
        <div class="col-auto"><q-input v-model="toLocal" type="datetime-local" outlined dense stack-label :label="$t('usage.filters.endTime')" :disable="allTime" style="width: 230px" /></div>
        <div class="col-auto"><q-btn-dropdown dense flat :label="$t('usage.filters.quickRange')" :no-icon-animation="true" class="q-px-xs">
          <q-list>
            <q-item clickable v-close-popup @click="applyRange(1)"><q-item-section>{{ $t('usage.range24h') }}</q-item-section></q-item>
            <q-item clickable v-close-popup @click="applyRange(7)"><q-item-section>{{ $t('usage.range7d') }}</q-item-section></q-item>
            <q-item clickable v-close-popup @click="applyRange(30)"><q-item-section>{{ $t('usage.range30d') }}</q-item-section></q-item>
            <q-item clickable v-close-popup @click="applyAllTime"><q-item-section>{{ $t('usage.rangeAll') }}</q-item-section></q-item>
          </q-list>
        </q-btn-dropdown></div>
        </div>
      <!-- One row of query filters. The user picker filters like the others; it is
           not a companion of the date range. -->
      <div class="row items-end q-col-gutter-sm q-mb-sm">
        <div class="col-auto"><q-select v-model="userId" :options="userOptions" :label="$t('usage.filters.user')" :hint="$t('usage.filters.userHint')" :placeholder="$t('usage.filters.anyUser')" :loading="loadingUsers" outlined dense clearable use-input emit-value map-options @filter="onUserFilter" style="width: 280px" data-cy="usage-user-filter" /></div>
        <div class="col-auto"><q-select v-model="resourceKind" outlined dense :label="$t('usage.filters.resourceKind')" :options="resourceKinds" clearable style="width: 150px" /></div>
        <div class="col-auto"><q-select v-model="status" outlined dense :label="$t('usage.filters.status')" :options="statuses" clearable style="width: 130px" /></div>
        <div class="col-auto"><q-select v-model="completeness" outlined dense :label="$t('usage.filters.completeness')" :options="completenesses" clearable style="width: 150px" /></div>
        <div class="col-auto"><q-btn flat dense icon="filter_alt_off" :label="$t('usage.filters.reset')" :disable="!activeFilters.length" @click="resetFilters" /></div>
      </div>
      <!-- Its own collapsed disclosure: an optional raw-identifier path, not a
           stray line trailing the filter row. -->
      <details class="q-mb-md" data-cy="usage-identity-filters">
        <summary class="text-caption text-grey-7 cursor-pointer">{{ $t('usage.filters.byIdentity') }}</summary>
        <div class="row q-col-gutter-sm q-mt-xs">
          <div class="col-12 col-sm-6"><q-input v-model="resourceId" outlined dense :label="$t('usage.filters.resource')" placeholder="mdl_..." /></div>
          <div class="col-12 col-sm-6"><q-input v-model="upstreamId" outlined dense :label="$t('usage.filters.upstream')" placeholder="ups_..." /></div>
        </div>
      </details>
      <q-banner v-if="activeFilters.length" class="q-mb-md bg-grey-2 rounded-borders">
        <div class="row items-center q-gutter-sm">
          <span class="text-caption text-grey-7">{{ $t('usage.filters.active') }}:</span>
          <q-chip v-for="f in activeFilters" :key="f" dense>{{ f }}</q-chip>
        </div>
      </q-banner>
      <ProblemBanner :error="error" class="q-mb-md" />

      <LoadingState v-if="loading && !summary" />
      <div v-else-if="summary" class="row q-col-gutter-md q-mb-md">
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
              <q-chip dense :color="costStatusColor(costStatus)" :label="costStatusLabel(costStatus)" text-color="white" />
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
                <q-chip v-for="m in semanticMeters" :key="m.meter" dense :color="meterColor(m.meter)" text-color="white" size="sm">
                  {{ m.meter }}: {{ m.quantity }}
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

      <UsageRequestList :query="filterQuery" :page-size="pageSize" allow-filter-resource @filter-resource="value => { resourceId = value }">
        <template #toolbar>
          <q-select v-model="pageSize" :options="pageSizes" :label="$t('usage.pageSize')" outlined dense emit-value map-options style="width: 150px" data-cy="usage-page-size" />
        </template>
      </UsageRequestList>
    </template>
  </q-page>
</template>