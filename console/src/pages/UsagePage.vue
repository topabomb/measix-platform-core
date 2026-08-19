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

onMounted(refresh)
</script>

<template>
  <q-page padding>
    <div class="row items-center justify-between q-mb-lg">
      <div>
        <div class="text-h5 text-weight-bold">Usage</div>
        <div class="text-body2 text-grey-7">Aggregate usage summary and per-request ledger.</div>
      </div>
      <div class="row items-center q-gutter-sm">
        <q-input v-model="fromISO" outlined dense label="From (ISO)" style="width: 200px" />
        <q-input v-model="toISO" outlined dense label="To (ISO)" style="width: 200px" />
        <q-btn flat icon="refresh" @click="refresh" />
      </div>
    </div>
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
            <div class="text-h5">{{ summary.cost }}</div>
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
            <q-item-label caption>{{ req.startedAt }} · {{ req.modelId ?? 'unknown model' }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-chip dense :color="req.completeness === 'COMPLETE' ? 'green-2' : 'orange-2'">{{ req.completeness }}</q-chip>
          </q-item-section>
        </q-item>
        <q-item v-if="!requests.length"><q-item-section class="text-grey-7">No usage requests recorded.</q-item-section></q-item>
      </q-list>
    </q-card>
  </q-page>
</template>
