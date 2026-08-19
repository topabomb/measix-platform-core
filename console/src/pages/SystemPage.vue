<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'

type SystemStatus = components['schemas']['SystemStatus']
type SystemHealth = components['schemas']['SystemHealth']

const status = ref<SystemStatus>()
const health = ref<SystemHealth>()
const loading = ref(false)
const error = ref<unknown>()

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    const [s, h] = await Promise.all([
      apiFetch<SystemStatus>('/api/admin/v1/system/status'),
      apiFetch<SystemHealth>('/api/admin/v1/system/health'),
    ])
    status.value = s
    health.value = h
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
        <div class="text-h5 text-weight-bold">System</div>
        <div class="text-body2 text-grey-7">Build, runtime, database and Relay health.</div>
      </div>
      <q-btn flat icon="refresh" @click="refresh" />
    </div>
    <ProblemBanner :error="error" class="q-mb-md" />
    <LoadingState v-if="loading && !status" />
    <template v-else-if="status">
      <div class="row q-gutter-md q-mb-md">
        <q-card flat bordered class="col-3">
          <q-card-section>
            <div class="text-caption text-grey-7">Build</div>
            <div class="text-h6">{{ status.buildVersion }}</div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="col-3">
          <q-card-section>
            <div class="text-caption text-grey-7">Database</div>
            <div class="text-h6">{{ status.dbHealth }}</div>
            <div class="text-caption">migration {{ status.migrationRevision }}</div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="col-3">
          <q-card-section>
            <div class="text-caption text-grey-7">Runtime</div>
            <StatusChip :value="status.runtimeStatus" />
            <div class="text-caption q-mt-sm">generation {{ status.activeManagedGeneration }} · revision {{ status.managedStateRevision }}</div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="col-3">
          <q-card-section>
            <div class="text-caption text-grey-7">Relay</div>
            <q-badge :color="status.relayReady ? 'green' : 'red'" :label="status.relayReady ? 'Ready' : 'Not ready'" />
            <div class="text-caption q-mt-sm">desired revision {{ status.desiredControlRevision }}</div>
          </q-card-section>
        </q-card>
      </div>
    </template>

    <q-card v-if="health" flat bordered>
      <q-card-section><div class="text-subtitle2">Health probes</div></q-card-section>
      <q-markup-table flat dense>
        <tbody>
          <tr v-for="(value, key) in health" :key="String(key)">
            <td class="text-grey-7">{{ key }}</td>
            <td>{{ typeof value === 'boolean' ? (value ? 'ok' : 'fail') : value }}</td>
          </tr>
        </tbody>
      </q-markup-table>
    </q-card>
  </q-page>
</template>
