<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import PageHeader from '../components/PageHeader.vue'

const { t: $t } = useI18n()

type SystemStatus = components['schemas']['SystemStatus']
type SystemHealth = components['schemas']['Health']

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
    <PageHeader :title="$t('system.title')" :subtitle="$t('system.subtitle')">
      <template #actions>
        <q-btn flat icon="refresh" @click="refresh" />
      </template>
    </PageHeader>
    <ProblemBanner :error="error" class="q-mb-md" />
    <LoadingState v-if="loading && !status" />
    <template v-else-if="status">
      <div class="row q-col-gutter-md q-mb-md">
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('system.hubVersion') }}</div>
              <div class="text-h6">{{ status.buildVersion }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('system.dbHealth') }}</div>
              <div class="text-h6">{{ status.dbHealth }}</div>
              <div class="text-caption">{{ $t('system.managedStateRevision') }} {{ status.migrationRevision }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('overview.managedRuntime') }}</div>
              <div data-cy="system-runtime-status"><StatusChip :value="status.runtimeStatus" /></div>
              <div class="text-caption q-mt-sm">{{ $t('overview.activeGeneration') }} {{ status.activeManagedGeneration }} · {{ $t('system.managedStateRevision') }} {{ status.managedStateRevision }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('system.relayReady') }}</div>
              <q-badge data-cy="system-relay-status" :color="status.relayReady ? 'green' : 'red'" :label="status.relayReady ? $t('status.READY') : $t('status.NOT_CONVERGED')" />
              <div class="text-caption q-mt-sm">{{ $t('overview.desiredRevision') }} {{ status.desiredControlRevision }} · {{ $t('overview.appliedRevision') }} {{ status.appliedControlRevision ?? '—' }}</div>
              <div class="text-caption">{{ $t('system.bundle').toLowerCase() }} {{ status.appliedBundleHash ? status.appliedBundleHash.slice(7, 19) : '—' }}</div>
              <div v-if="status.appliedControlRevision !== undefined && status.appliedControlRevision !== status.desiredControlRevision" class="text-caption text-warning q-mt-xs">{{ $t('status.NOT_CONVERGED') }}</div>
              <div class="text-caption text-grey-7 q-mt-xs">{{ $t('system.lastReconciliation') }}: {{ status.lastRelaySeenAt ?? '—' }}</div>
            </q-card-section>
          </q-card>
        </div>
      </div>

      <!-- Metering & spool state -->
      <div class="row q-col-gutter-md q-mb-md">
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle2">{{ $t('system.meteringSpool') }}</q-card-section>
            <q-list separator>
              <q-item><q-item-section>{{ $t('overview.ingestLag') }}</q-item-section><q-item-section side>{{ status.requestUsageIngestLagSeconds ?? '—' }}s</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.spoolState') }}</q-item-section><q-item-section side>{{ status.spoolState ?? '—' }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.spoolPending') }}</q-item-section><q-item-section side>{{ status.spoolPendingCount ?? '—' }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.spoolOldest') }}</q-item-section><q-item-section side>{{ status.oldestPendingAgeSeconds ?? '—' }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.semanticOrphan') }}</q-item-section><q-item-section side>{{ status.semanticOrphanCount ?? '—' }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('overview.activeGeneration') }}</q-item-section><q-item-section side>{{ status.activeManagedGeneration }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.managedStateRevision') }}</q-item-section><q-item-section side>{{ status.managedStateRevision }}</q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle2">{{ $t('system.runtimeStatus') }}</q-card-section>
            <q-list separator>
              <q-item><q-item-section>{{ $t('overview.desiredRevision') }}</q-item-section><q-item-section side>{{ status.desiredControlRevision }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('overview.appliedRevision') }}</q-item-section><q-item-section side>{{ status.appliedControlRevision ?? '—' }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.bundleHash') }} ({{ $t('overview.desiredRevision').toLowerCase() }})</q-item-section><q-item-section side><code>{{ status.desiredBundleHash ? status.desiredBundleHash.slice(7, 19) : '—' }}</code></q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.bundleHash') }} ({{ $t('overview.appliedRevision').toLowerCase() }})</q-item-section><q-item-section side><code>{{ status.appliedBundleHash ? status.appliedBundleHash.slice(7, 19) : '—' }}</code></q-item-section></q-item>
              <q-item v-if="status.appliedControlRevision !== undefined && status.appliedControlRevision !== status.desiredControlRevision">
                <q-item-section><q-item-label class="text-warning">{{ $t('status.NOT_CONVERGED') }}</q-item-label><q-item-label caption>{{ $t('system.lastReconciliation') }}</q-item-label></q-item-section>
                <q-item-section side><q-badge color="orange" :label="$t('common.pending')" /></q-item-section>
              </q-item>
              <q-item v-else>
                <q-item-section><q-item-label class="text-positive">{{ $t('status.CONVERGED') }}</q-item-label></q-item-section>
                <q-item-section side><q-badge color="green" :label="$t('status.CONVERGED')" /></q-item-section>
              </q-item>
            </q-list>
          </q-card>
        </div>
      </div>

      <!-- Latest activation -->
      <div class="row q-col-gutter-md q-mb-md">
        <div class="col-12">
          <q-card flat bordered>
            <q-card-section class="text-subtitle2">{{ $t('system.currentActivation') }}</q-card-section>
            <q-list separator>
              <template v-if="status.latestActivation">
                <q-item>
                  <q-item-section>
                    <q-item-label>{{ status.latestActivation.activationId }}</q-item-label>
                    <q-item-label caption>{{ status.latestActivation.kind }} · {{ $t('overview.desiredRevision') }} {{ status.latestActivation.desiredControlRevision }}</q-item-label>
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
              <q-item v-else><q-item-section class="text-grey-7">{{ $t('common.noData') }}</q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
      </div>
    </template>

    <q-card v-if="health" flat bordered>
      <q-card-section><div class="text-subtitle2">{{ $t('system.upstreamHealth') }}</div></q-card-section>
      <q-markup-table flat dense>
        <tbody>
          <tr v-for="(value, key) in health" :key="String(key)">
            <td class="text-grey-7">{{ key }}</td>
            <td>{{ typeof value === 'boolean' ? (value ? $t('common.success') : $t('common.error')) : value }}</td>
          </tr>
        </tbody>
      </q-markup-table>
    </q-card>
  </q-page>
</template>
