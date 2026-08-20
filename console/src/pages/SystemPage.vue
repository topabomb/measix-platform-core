<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import PageHeader from '../components/PageHeader.vue'

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
  <q-page padding data-cy="system-page">
    <PageHeader title="System" subtitle="Build, runtime, database and Relay health.">
      <template #primary><q-btn flat icon="refresh" @click="refresh" /></template>
    </PageHeader>
    <ProblemBanner :error="error" class="q-mb-md" />
    <LoadingState v-if="loading && !status" />
    <template v-else-if="status">
      <div class="row q-col-gutter-md q-mb-md">
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">Build</div>
              <div class="text-h6">{{ status.buildVersion }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">Database</div>
              <div class="text-h6">{{ status.dbHealth }}</div>
              <div class="text-caption">migration {{ status.migrationRevision }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">Runtime</div>
              <div data-cy="system-runtime-status"><StatusChip :value="status.runtimeStatus" /></div>
              <div class="text-caption q-mt-sm">generation {{ status.activeManagedGeneration }} · revision {{ status.managedStateRevision }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">Relay</div>
              <q-badge :color="status.relayReady ? 'green' : 'red'" :label="status.relayReady ? 'Ready' : 'Not ready'" />
              <div class="text-caption q-mt-sm">desired {{ status.desiredControlRevision }} · applied {{ status.appliedControlRevision ?? '—' }}</div>
              <div class="text-caption">bundle {{ status.appliedBundleHash ? status.appliedBundleHash.slice(7, 19) : '—' }}</div>
              <div v-if="status.appliedControlRevision !== undefined && status.appliedControlRevision !== status.desiredControlRevision" class="text-caption text-warning q-mt-xs">control not converged</div>
              <div class="text-caption text-grey-7 q-mt-xs">last seen: {{ status.lastRelaySeenAt ?? '—' }}</div>
            </q-card-section>
          </q-card>
        </div>
      </div>

      <!-- Metering & spool state -->
      <div class="row q-col-gutter-md q-mb-md">
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle2">Metering & Spool</q-card-section>
            <q-list separator>
              <q-item><q-item-section>Usage ingest lag</q-item-section><q-item-section side>{{ status.requestUsageIngestLagSeconds ?? 0 }}s</q-item-section></q-item>
              <q-item><q-item-section>Semantic orphans</q-item-section><q-item-section side>{{ status.semanticOrphanCount ?? 0 }}</q-item-section></q-item>
              <q-item><q-item-section>Active generation</q-item-section><q-item-section side>{{ status.activeManagedGeneration }}</q-item-section></q-item>
              <q-item><q-item-section>Managed state revision</q-item-section><q-item-section side>{{ status.managedStateRevision }}</q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle2">Control & Reconciliation</q-card-section>
            <q-list separator>
              <q-item><q-item-section>Desired control revision</q-item-section><q-item-section side>{{ status.desiredControlRevision }}</q-item-section></q-item>
              <q-item><q-item-section>Applied control revision</q-item-section><q-item-section side>{{ status.appliedControlRevision ?? '—' }}</q-item-section></q-item>
              <q-item><q-item-section>Desired bundle hash</q-item-section><q-item-section side><code>{{ status.desiredBundleHash ? status.desiredBundleHash.slice(7, 19) : '—' }}</code></q-item-section></q-item>
              <q-item><q-item-section>Applied bundle hash</q-item-section><q-item-section side><code>{{ status.appliedBundleHash ? status.appliedBundleHash.slice(7, 19) : '—' }}</code></q-item-section></q-item>
              <q-item v-if="status.appliedControlRevision !== undefined && status.appliedControlRevision !== status.desiredControlRevision">
                <q-item-section><q-item-label class="text-warning">Control not converged</q-item-label><q-item-label caption>Last reconciliation pending</q-item-label></q-item-section>
                <q-item-section side><q-badge color="orange" label="pending" /></q-item-section>
              </q-item>
              <q-item v-else>
                <q-item-section><q-item-label class="text-positive">Control converged</q-item-label></q-item-section>
                <q-item-section side><q-badge color="green" label="converged" /></q-item-section>
              </q-item>
            </q-list>
          </q-card>
        </div>
      </div>

      <!-- Latest activation -->
      <div class="row q-col-gutter-md q-mb-md">
        <div class="col-12">
          <q-card flat bordered>
            <q-card-section class="text-subtitle2">Latest activation</q-card-section>
            <q-list separator>
              <template v-if="status.latestActivation">
                <q-item>
                  <q-item-section>
                    <q-item-label>{{ status.latestActivation.activationId }}</q-item-label>
                    <q-item-label caption>{{ status.latestActivation.kind }} · desired rev {{ status.latestActivation.desiredControlRevision }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-sm">
                      <StatusChip :value="status.latestActivation.state" />
                      <q-badge v-if="status.latestActivation.errorCode" color="negative" :label="status.latestActivation.errorCode" />
                      <q-badge v-if="status.latestActivation.releaseId" color="blue" :label="status.latestActivation.releaseId.slice(0, 12) + '...'" />
                    </div>
                  </q-item-section>
                </q-item>
              </template>
              <q-item v-else><q-item-section class="text-grey-7">No activation recorded.</q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
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
