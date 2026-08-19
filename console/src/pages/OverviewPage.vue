<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'

type SystemStatus = components['schemas']['SystemStatus']
type UsageSummary = components['schemas']['UsageSummary']

const system = ref<SystemStatus>()
const usage = ref<UsageSummary>()
const loading = ref(false)
const error = ref<unknown>()

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    const [systemStatus, usageSummary] = await Promise.all([
      apiFetch<SystemStatus>('/api/admin/v1/system/status'),
      apiFetch<UsageSummary>('/api/admin/v1/usage/summary'),
    ])
    system.value = systemStatus
    usage.value = usageSummary
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <q-page padding>
    <div class="row items-center justify-between q-mb-lg">
      <div>
        <div class="text-h5 text-weight-bold">Overview</div>
        <div class="text-body2 text-grey-7">Current Control Hub, Relay and usage state.</div>
      </div>
      <q-btn flat icon="refresh" label="Refresh" :loading="loading" @click="refresh" />
    </div>
    <ProblemBanner :error="error" class="q-mb-md" />
    <LoadingState v-if="loading && !system" />
    <template v-else-if="system">
      <q-banner v-if="system.runtimeStatus !== 'READY' || !system.relayReady" class="bg-orange-1 text-warning q-mb-md rounded-borders">
        Managed runtime is not fully ready. New managed interactions may be blocked until Hub and Relay converge.
      </q-banner>
      <div class="row q-col-gutter-md">
        <div class="col-12 col-sm-6 col-lg-3">
          <q-card flat bordered><q-card-section><div class="text-caption text-grey-7">Runtime</div><StatusChip :value="system.runtimeStatus" /><div class="text-caption q-mt-sm">Relay {{ system.relayReady ? 'ready' : 'not ready' }}</div></q-card-section></q-card>
        </div>
        <div class="col-12 col-sm-6 col-lg-3">
          <q-card flat bordered><q-card-section><div class="text-caption text-grey-7">Managed generation</div><div class="text-h4">{{ system.activeManagedGeneration }}</div><div class="text-caption">state rev {{ system.managedStateRevision }}</div></q-card-section></q-card>
        </div>
        <div class="col-12 col-sm-6 col-lg-3">
          <q-card flat bordered><q-card-section><div class="text-caption text-grey-7">Control revision</div><div class="text-h4">{{ system.desiredControlRevision }}</div><div class="text-caption">Relay {{ system.appliedControlRevision ?? '—' }}</div></q-card-section></q-card>
        </div>
        <div class="col-12 col-sm-6 col-lg-3">
          <q-card flat bordered><q-card-section><div class="text-caption text-grey-7">Requests</div><div class="text-h4">{{ usage?.requestCount ?? 0 }}</div><div class="text-caption">{{ usage?.forwardedRequestCount ?? 0 }} forwarded</div></q-card-section></q-card>
        </div>
      </div>
      <div class="row q-col-gutter-md q-mt-xs">
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle1 text-weight-medium">Diagnostics</q-card-section>
            <q-list separator>
              <q-item><q-item-section>Database</q-item-section><q-item-section side>{{ system.dbHealth }}</q-item-section></q-item>
              <q-item><q-item-section>Migration</q-item-section><q-item-section side>{{ system.migrationRevision }}</q-item-section></q-item>
              <q-item><q-item-section>Usage ingest lag</q-item-section><q-item-section side>{{ system.requestUsageIngestLagSeconds ?? 0 }}s</q-item-section></q-item>
              <q-item><q-item-section>Semantic orphans</q-item-section><q-item-section side>{{ system.semanticOrphanCount ?? 0 }}</q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle1 text-weight-medium">Usage completeness</q-card-section>
            <q-list separator>
              <q-item v-for="meter in usage?.semanticMeters ?? []" :key="meter.meter">
                <q-item-section><q-item-label>{{ meter.meter }}</q-item-label><q-item-label caption>{{ meter.quantity }}</q-item-label></q-item-section>
                <q-item-section side><StatusChip :value="meter.confidence" /></q-item-section>
              </q-item>
              <q-item v-if="!usage?.semanticMeters.length"><q-item-section class="text-grey-7">No semantic usage reported.</q-item-section></q-item>
              <q-item><q-item-section>Cost</q-item-section><q-item-section side><StatusChip :value="usage?.cost.status ?? 'UNKNOWN'" /></q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
      </div>
    </template>
  </q-page>
</template>
