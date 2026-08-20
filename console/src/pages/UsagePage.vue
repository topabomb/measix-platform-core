<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'

type UsageSummary = components['schemas']['UsageSummary']
type RequestUsagePage = components['schemas']['RequestUsagePage']
type RequestUsage = components['schemas']['RequestUsage']

const summary = ref<UsageSummary>()
const requests = ref<RequestUsage[]>([])
const loading = ref(false)
const error = ref<unknown>()
const fromISO = ref<string>()
const toISO = ref<string>()
const userId = ref<string>()
const resourceId = ref<string>()
const resourceKind = ref<string>()
const upstreamId = ref<string>()
const status = ref<string>()
const resourceKinds = ['PROVIDER', 'MODEL', 'TTS', 'ASR', 'MCP']
const statuses = ['SUCCESS', 'ERROR', 'BLOCKED']
const activeFilters = computed(() => {
  const parts: string[] = []
  if (fromISO.value) parts.push(`from ${fromISO.value}`)
  if (toISO.value) parts.push(`to ${toISO.value}`)
  if (userId.value) parts.push(`user ${userId.value}`)
  if (resourceId.value) parts.push(`resource ${resourceId.value}`)
  if (resourceKind.value) parts.push(`kind ${resourceKind.value}`)
  if (upstreamId.value) parts.push(`upstream ${upstreamId.value}`)
  if (status.value) parts.push(status.value)
  return parts
})
const semanticMeterKeys = computed(() => {
  if (!summary.value) return []
  return Object.keys(summary.value.semanticMeters ?? {})
})

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
  return 'unknown'
})

onMounted(refresh)
</script>

<template>
  <q-page padding>
    <div class="row items-center justify-between q-mb-lg">
      <div>
        <div class="text-h5 text-weight-bold">Usage</div>
        <div class="text-body2 text-grey-7">Aggregate usage summary and per-request ledger.</div>
      </div>
      <q-btn flat icon="refresh" @click="refresh" />
    </div>
    <div class="row items-center q-gutter-sm q-mb-md flex-wrap">
      <q-input v-model="fromISO" outlined dense label="From" placeholder="2026-08-01T00:00:00Z" style="width: 190px" />
      <q-input v-model="toISO" outlined dense label="To" placeholder="2026-08-31T23:59:59Z" style="width: 190px" />
      <q-input v-model="userId" outlined dense label="User ID" placeholder="usr_..." style="width: 180px" />
      <q-input v-model="resourceId" outlined dense label="Resource ID" placeholder="mdl_..." style="width: 180px" />
      <q-select v-model="resourceKind" outlined dense label="Kind" :options="resourceKinds" clearable style="width: 150px" />
      <q-input v-model="upstreamId" outlined dense label="Upstream ID" placeholder="ups_..." style="width: 180px" />
      <q-select v-model="status" outlined dense label="Status" :options="statuses" clearable style="width: 140px" />
    </div>
    <q-banner v-if="activeFilters.length" class="q-mb-md bg-grey-2 rounded-borders">
      <div class="row items-center q-gutter-sm">
        <span class="text-caption text-grey-7">Active filters:</span>
        <q-chip v-for="f in activeFilters" :key="f" dense>{{ f }}</q-chip>
      </div>
    </q-banner>
    <ProblemBanner :error="error" class="q-mb-md" />

    <LoadingState v-if="loading && !summary" />
    <template v-else-if="summary">
      <div class="row q-gutter-md q-mb-md">
        <q-card flat bordered class="col-3">
          <q-card-section>
            <div class="text-caption text-grey-7">Requests</div>
            <div class="text-h5">{{ summary.requestCount }}</div>
            <div class="text-caption">forwarded {{ summary.forwardedRequestCount }}</div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="col-3">
          <q-card-section>
            <div class="text-caption text-grey-7">Bytes</div>
            <div class="text-h5">{{ fmtBytes(summary.requestBytes) }}</div>
            <div class="text-caption">resp {{ fmtBytes(summary.responseBytes) }}</div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="col-3">
          <q-card-section>
            <div class="text-caption text-grey-7">Cost</div>
            <div class="text-h5">{{ costLabel }}</div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="col">
          <q-card-section>
            <div class="text-caption text-grey-7">Semantic meters</div>
            <q-chip v-for="key in semanticMeterKeys" :key="key" dense>{{ key }}</q-chip>
            <div v-if="!semanticMeterKeys.length" class="text-body2 text-grey-7">none</div>
          </q-card-section>
        </q-card>
      </div>
    </template>

    <q-card flat bordered>
      <q-card-section><div class="text-subtitle2">Requests</div></q-card-section>
      <q-list separator>
        <q-item v-for="req in requests" :key="req.requestId">
          <q-item-section>
            <q-item-label>{{ req.requestId }}</q-item-label>
            <q-item-label caption>{{ req.startedAt }} · {{ req.resourceId }} · {{ req.upstreamId }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-sm">
              <q-chip dense :color="req.forwarded ? 'green-2' : 'orange-2'">{{ req.forwarded ? 'forwarded' : 'blocked' }}</q-chip>
              <q-chip dense :class="req.httpStatus >= 400 ? 'text-negative' : 'text-grey-8'">{{ req.httpStatus }}</q-chip>
            </div>
          </q-item-section>
        </q-item>
        <q-item v-if="!requests.length"><q-item-section class="text-grey-7">No usage requests recorded.</q-item-section></q-item>
      </q-list>
    </q-card>
  </q-page>
</template>
