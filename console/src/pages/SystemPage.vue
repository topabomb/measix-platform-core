<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import { hasPublishedConfiguration, isManagedRuntimeConverged } from '../api/systemStatus'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import PageHeader from '../components/PageHeader.vue'

const { t: $t } = useI18n()

const activeTab = ref<'overview' | 'runtime' | 'metering' | 'events'>('overview')

type SystemStatus = components['schemas']['SystemStatus']
type SystemHealth = components['schemas']['Health']
type SystemTelemetry = components['schemas']['SystemTelemetry']
type SystemEventPage = components['schemas']['SystemEventPage']
type ProcessTelemetry = components['schemas']['ProcessTelemetry']

const status = ref<SystemStatus>()
const health = ref<SystemHealth>()
const loading = ref(false)
const error = ref<unknown>()
const healthError = ref<unknown>()
const telemetry = ref<SystemTelemetry>()
const events = ref<SystemEventPage>()
const diagnosticsError = ref<unknown>()
const telemetryWindow = ref<'15m' | '60m'>('15m')
const eventService = ref<'ALL' | 'HUB' | 'RELAY'>('ALL')
const eventLevel = ref('')
const eventPaused = ref(false)
let pollTimer: ReturnType<typeof setInterval> | undefined

const noPublishedConfiguration = computed(() => !!status.value && !hasPublishedConfiguration(status.value))
const converged = computed(() => isManagedRuntimeConverged(status.value))

async function refresh() {
  loading.value = true
  error.value = undefined
  healthError.value = undefined
  try {
    const [systemResult, healthResult] = await Promise.allSettled([
      apiFetch<SystemStatus>('/api/admin/v1/system/status'),
      apiFetch<SystemHealth>('/api/admin/v1/system/health'),
    ])
    if (systemResult.status === 'fulfilled') status.value = systemResult.value
    else {
      status.value = undefined
      error.value = systemResult.reason
    }
    if (healthResult.status === 'fulfilled') health.value = healthResult.value
    else {
      health.value = undefined
      healthError.value = healthResult.reason
    }
  } finally {
    loading.value = false
  }
}

async function refreshDiagnostics(force = false) {
  if (document.hidden) return
  diagnosticsError.value = undefined
  try {
    if (activeTab.value === 'metering') {
      telemetry.value = await apiFetch<SystemTelemetry>(`/api/admin/v1/system/telemetry?window=${telemetryWindow.value}`)
    } else if (activeTab.value === 'events' && (!eventPaused.value || force)) {
      const query = new URLSearchParams({ limit: '100' })
      if (eventService.value !== 'ALL') query.set('service', eventService.value)
      if (eventLevel.value.trim()) query.set('level', eventLevel.value.trim())
      events.value = await apiFetch<SystemEventPage>(`/api/admin/v1/system/events?${query}`)
    }
  } catch (reason) {
    diagnosticsError.value = reason
  }
}

function requestPoints(process: ProcessTelemetry | undefined): string {
  const values = process?.buckets.map(bucket => bucket.requestCount) ?? []
  if (values.length === 0) return ''
  const max = Math.max(1, ...values)
  const divisor = Math.max(1, values.length - 1)
  return values.map((value, index) => `${(index / divisor) * 100},${28 - (value / max) * 24}`).join(' ')
}

watch([activeTab, telemetryWindow, eventService, eventLevel, eventPaused], () => refreshDiagnostics())
onMounted(async () => {
  await refresh()
  await refreshDiagnostics()
  pollTimer = setInterval(refreshDiagnostics, 15_000)
})
onBeforeUnmount(() => { if (pollTimer) clearInterval(pollTimer) })
</script>

<template>
  <q-page class="admin-page" data-cy="system-page">
    <PageHeader :title="$t('system.title')" :subtitle="$t('system.subtitle')">
      <template #actions>
        <q-btn flat dense icon="refresh" :aria-label="$t('common.refresh')" :loading="loading" @click="refresh" />
      </template>
    </PageHeader>
    <q-tabs v-model="activeTab" dense align="left" class="q-mb-xs">
      <q-tab name="overview" icon="dashboard" :label="$t('system.tabs.overview')" />
      <q-tab name="runtime" icon="sync_alt" :label="$t('system.tabs.runtime')" />
      <q-tab name="metering" icon="monitoring" :label="$t('system.tabs.metering')" />
      <q-tab name="events" icon="receipt_long" :label="$t('system.tabs.events')" />
    </q-tabs>
    <ProblemBanner :error="error" class="q-mb-xs" />
    <ProblemBanner :error="diagnosticsError" class="q-mb-xs" />
    <q-banner v-if="noPublishedConfiguration" data-cy="system-setup-state" class="bg-amber-1 text-warning q-mb-xs rounded-borders">
      {{ $t('system.noPublishedConfiguration') }} {{ $t('system.setupGuidance') }}
      <div class="row q-gutter-xs q-mt-xs">
        <q-btn flat dense :to="{ name: 'Upstreams' }" :label="$t('nav.upstreams')" />
        <q-btn flat dense :to="{ name: 'Resources' }" :label="$t('nav.resources')" />
      </div>
    </q-banner>
    <LoadingState v-if="loading && !status" />
    <template v-else-if="status">
      <div v-show="activeTab === 'overview'" class="system-overview-grid q-mb-xs">
      <q-card flat bordered data-cy="platform-public-origin">
        <q-card-section>
          <div class="text-subtitle1">{{ $t('system.publicOrigin') }}</div>
          <div v-if="status.publicOrigin" class="text-body1 text-break q-mt-xs">{{ status.publicOrigin }}</div>
          <div v-else class="text-negative q-mt-xs">{{ $t('system.publicOriginMissing') }}</div>
          <div class="text-caption text-grey-7 q-mt-xs">{{ $t('system.publicOriginHint') }}</div>
        </q-card-section>
      </q-card>
      <q-card flat bordered data-cy="portal-status">
        <q-card-section>
          <div class="row items-center q-gutter-sm">
            <div class="text-subtitle1">{{ $t('system.portal') }}</div>
            <q-badge :color="status.portalMode === 'UNAVAILABLE' ? 'red' : status.portalMode === 'CUSTOM' ? 'blue' : 'green'" :label="$t(`system.portalModes.${status.portalMode}`)" />
          </div>
          <a v-if="status.portalUrl" :href="status.portalUrl" target="_blank" rel="noopener noreferrer" class="text-body1 text-break q-mt-xs block">{{ status.portalUrl }}</a>
          <div v-if="status.portalUpstreamUrl" class="text-caption text-grey-7 text-break q-mt-xs">{{ $t('system.portalUpstream') }}: {{ status.portalUpstreamUrl }}</div>
          <div class="text-caption text-grey-7 q-mt-xs">{{ $t('system.portalHint') }}</div>
        </q-card-section>
      </q-card>
      </div>
      <div class="row q-col-gutter-xs q-mb-xs">
        <div v-show="activeTab === 'overview'" class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('system.hubVersion') }}</div>
              <div class="text-h6">{{ status.buildVersion }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div v-show="activeTab === 'overview'" class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('system.dbHealth') }}</div>
              <div class="text-h6">{{ status.dbHealth }}</div>
              <div class="text-caption">{{ $t('system.schemaIdentity') }} <code class="schema-identity">{{ status.schemaIdentity }}</code></div>
            </q-card-section>
          </q-card>
        </div>
        <div v-show="activeTab === 'overview'" class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('overview.managedRuntime') }}</div>
              <div data-cy="system-runtime-status"><StatusChip :value="status.runtimeStatus" /></div>
              <div class="text-caption q-mt-xs">{{ $t('overview.activeGeneration') }} {{ status.activeManagedGeneration }} · {{ $t('system.managedStateRevision') }} {{ status.managedStateRevision }}</div>
            </q-card-section>
          </q-card>
        </div>
        <div v-show="activeTab === 'overview'" class="col-xs-12 col-sm-6 col-md-3">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-caption text-grey-7">{{ $t('system.relayReady') }}</div>
              <div class="text-caption">{{ $t('system.relayVersion') }}: <span data-cy="relay-build-version">{{ status.relayBuildVersion ?? '—' }}</span></div>
              <q-badge data-cy="system-relay-status" :color="status.relayReady ? 'green' : 'red'" :label="status.relayReady ? $t('status.READY') : $t('status.NOT_READY')" />
              <div class="text-caption q-mt-xs">{{ $t('overview.desiredRevision') }} {{ status.desiredControlRevision }} · {{ $t('overview.appliedRevision') }} {{ status.appliedControlRevision ?? '—' }}</div>
              <div class="text-caption">{{ $t('system.bundle').toLowerCase() }} {{ status.appliedBundleHash ? status.appliedBundleHash.slice(7, 19) : '—' }}</div>
              <div v-if="!converged" class="text-caption text-warning q-mt-xs">{{ $t('status.NOT_CONVERGED') }}</div>
              <div class="text-caption text-grey-7 q-mt-xs">{{ $t('system.lastRelaySeen') }}: {{ status.lastRelaySeenAt ?? '—' }}</div>
            </q-card-section>
          </q-card>
        </div>
      <!-- Metering & spool state -->
        <div v-show="activeTab === 'metering'" class="col-12">
          <div class="row items-center q-gutter-xs q-mb-xs">
            <q-btn-toggle v-model="telemetryWindow" dense no-caps toggle-color="primary" :options="[{ label: '15m', value: '15m' }, { label: '60m', value: '60m' }]" />
            <q-btn flat dense icon="refresh" :aria-label="$t('common.refresh')" @click="refreshDiagnostics()" />
            <span v-if="telemetry" class="text-caption text-grey-7">{{ $t('system.telemetrySince') }} {{ new Date(telemetry.hub.startedAt).toLocaleString() }}</span>
          </div>
          <div v-if="telemetry" class="telemetry-grid q-mb-xs" data-cy="system-telemetry">
            <q-card v-for="process in [{ name: 'Hub', value: telemetry.hub }, { name: 'Relay', value: telemetry.relay }]" :key="process.name" flat bordered>
              <q-card-section>
                <div class="row items-center justify-between"><div class="text-subtitle2">{{ process.name }}</div><div class="text-caption text-grey-7">P95 {{ process.value?.summary.durationP95Ms ?? '—' }} ms</div></div>
                <template v-if="process.value">
                  <div class="row q-col-gutter-sm q-mt-xs text-caption">
                    <div class="col">{{ $t('system.requests') }} <strong>{{ process.value.summary.requestCount }}</strong></div>
                    <div class="col">{{ $t('system.errors') }} <strong>{{ process.value.summary.clientErrorCount + process.value.summary.serverErrorCount }}</strong></div>
                    <div class="col">{{ $t('system.inFlight') }} <strong>{{ process.value.summary.inFlight }}</strong></div>
                  </div>
                  <svg class="telemetry-chart" viewBox="0 0 100 32" preserveAspectRatio="none" role="img" :aria-label="$t('system.requestTrend')">
                    <polyline :points="requestPoints(process.value)" fill="none" stroke="currentColor" stroke-width="1.5" vector-effect="non-scaling-stroke" />
                  </svg>
                </template>
                <div v-else class="text-grey-7">{{ $t('system.telemetryUnavailable') }}</div>
              </q-card-section>
            </q-card>
          </div>
          <q-card flat bordered>
            <q-card-section class="text-subtitle2">{{ $t('system.meteringSpool') }}</q-card-section>
            <q-list separator class="system-metering-list">
              <q-item>
                <q-item-section>{{ $t('system.semanticUnknown') }}<q-item-label caption>{{ $t('system.semanticUnknownHint') }}</q-item-label></q-item-section>
                <q-item-section side data-cy="semantic-unknown-count">{{ status.semanticUnknownRequestCount ?? '—' }}</q-item-section>
              </q-item>
              <q-item><q-item-section>{{ $t('overview.ingestLag') }}</q-item-section><q-item-section side>{{ status.requestUsageIngestLagSeconds === undefined ? '—' : `${status.requestUsageIngestLagSeconds}s` }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.spoolState') }}</q-item-section><q-item-section side>{{ status.spoolState ?? '—' }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.spoolPending') }}</q-item-section><q-item-section side>{{ status.spoolPendingCount ?? '—' }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.spoolOldest') }}</q-item-section><q-item-section side>{{ status.oldestPendingAgeSeconds ?? '—' }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.semanticOrphan') }}</q-item-section><q-item-section side>{{ status.semanticOrphanCount ?? '—' }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('overview.activeGeneration') }}</q-item-section><q-item-section side>{{ status.activeManagedGeneration }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.managedStateRevision') }}</q-item-section><q-item-section side>{{ status.managedStateRevision }}</q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
        <div v-show="activeTab === 'runtime'" class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle2">{{ $t('system.runtimeStatus') }}</q-card-section>
            <q-list separator>
              <q-item><q-item-section>{{ $t('overview.desiredRevision') }}</q-item-section><q-item-section side>{{ status.desiredControlRevision }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('overview.appliedRevision') }}</q-item-section><q-item-section side>{{ status.appliedControlRevision ?? '—' }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.bundleHash') }} ({{ $t('overview.desiredRevision').toLowerCase() }})</q-item-section><q-item-section side><code>{{ status.desiredBundleHash ? status.desiredBundleHash.slice(7, 19) : '—' }}</code></q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.bundleHash') }} ({{ $t('overview.appliedRevision').toLowerCase() }})</q-item-section><q-item-section side><code>{{ status.appliedBundleHash ? status.appliedBundleHash.slice(7, 19) : '—' }}</code></q-item-section></q-item>
              <q-item v-if="!converged">
                <q-item-section><q-item-label class="text-warning">{{ $t('status.NOT_CONVERGED') }}</q-item-label></q-item-section>
                <q-item-section side><q-badge data-cy="system-convergence-status" color="orange" :label="noPublishedConfiguration ? $t('system.notConfigured') : $t('status.NOT_CONVERGED')" /></q-item-section>
              </q-item>
              <q-item v-else>
                <q-item-section><q-item-label class="text-positive">{{ $t('status.CONVERGED') }}</q-item-label></q-item-section>
                <q-item-section side><q-badge data-cy="system-convergence-status" color="green" :label="$t('status.CONVERGED')" /></q-item-section>
              </q-item>
            </q-list>
          </q-card>
        </div>
      <!-- In-flight and completed operations are independent observations. -->
        <div v-for="operation in [{ key: 'currentActivation', value: status.currentActivation }, { key: 'lastActivation', value: status.lastActivation }]" v-show="activeTab === 'runtime'" :key="operation.key" class="col-12 col-md-6" :data-cy="operation.key">
          <q-card flat bordered>
            <q-card-section class="text-subtitle2">{{ $t(`system.${operation.key}`) }}</q-card-section>
            <q-list separator>
              <template v-if="operation.value">
                <q-item>
                  <q-item-section>
                    <q-item-label>{{ $t(`system.operationKinds.${operation.value.kind}`) }}</q-item-label>
                    <q-item-label caption>{{ new Date(operation.value.updatedAt).toLocaleString() }}</q-item-label>
                    <details class="text-caption q-mt-xs">
                      <summary>{{ $t('resources.review.technicalDetails') }}</summary>
                      <div class="text-break">{{ operation.value.activationId }}</div>
                      <div>{{ $t('overview.desiredRevision') }} {{ operation.value.desiredControlRevision }}</div>
                      <div v-if="operation.value.releaseId" class="text-break">{{ operation.value.releaseId }}</div>
                    </details>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-xs">
                      <StatusChip :value="operation.value.state" />
                      <q-badge v-if="operation.value.errorCode" color="negative" :label="operation.value.errorCode" />
                    </div>
                  </q-item-section>
                </q-item>
              </template>
              <q-item v-else><q-item-section class="text-grey-7">{{ $t(operation.key === 'currentActivation' ? 'system.noCurrentOperation' : 'system.noCompletedOperation') }}</q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
      </div>
    </template>

    <div v-show="activeTab === 'events'" class="q-mt-xs" data-cy="system-events">
      <div class="row items-center q-gutter-xs q-mb-xs">
        <q-btn-toggle v-model="eventService" dense no-caps toggle-color="primary" :options="[{ label: $t('common.all'), value: 'ALL' }, { label: 'Hub', value: 'HUB' }, { label: 'Relay', value: 'RELAY' }]" />
        <q-select v-model="eventLevel" dense outlined clearable emit-value map-options :label="$t('system.level')" :options="['INFO', 'WARN', 'ERROR']" style="min-width: 120px" />
        <q-toggle v-model="eventPaused" :label="$t('system.pauseRefresh')" />
        <q-btn flat dense icon="refresh" :aria-label="$t('common.refresh')" @click="refreshDiagnostics(true)" />
      </div>
      <q-card flat bordered>
        <q-virtual-scroll v-if="events?.items.length" :items="events.items" virtual-scroll-item-size="56" style="max-height: 520px">
          <template #default="{ item }">
            <q-item :key="`${item.time}-${item.service}-${item.event}`">
              <q-item-section>
                <q-item-label><q-badge outline :color="item.level === 'ERROR' ? 'negative' : item.level === 'WARN' ? 'warning' : 'primary'" :label="item.level" /> {{ item.event }}</q-item-label>
                <q-item-label caption>{{ new Date(item.time).toLocaleString() }} · {{ item.service }} · {{ item.message }}</q-item-label>
                <q-item-label v-if="item.requestId || item.activationId || item.resourceId" caption class="text-mono">{{ item.requestId || item.activationId || item.resourceId }}</q-item-label>
              </q-item-section>
            </q-item>
          </template>
        </q-virtual-scroll>
        <q-card-section v-else class="text-grey-7">{{ $t('system.noEvents') }}</q-card-section>
      </q-card>
    </div>

    <ProblemBanner :error="healthError" class="q-mb-xs" />
    <q-card v-if="health" v-show="activeTab === 'overview'" flat bordered>
      <q-card-section><div class="text-subtitle2">{{ $t('system.hubHealth') }}</div></q-card-section>
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

<style scoped>
.system-overview-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px;
}

.system-metering-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.system-metering-list :deep(.q-item) {
  border-bottom: 1px solid rgba(0, 0, 0, .08);
}

.schema-identity {
  overflow-wrap: anywhere;
}

.telemetry-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 4px; }
.telemetry-chart { width: 100%; height: 72px; color: var(--q-primary); margin-top: 8px; }
.text-mono { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; overflow-wrap: anywhere; }

@media (max-width: 700px) {
  .system-overview-grid,
  .system-metering-list,
  .telemetry-grid { grid-template-columns: minmax(0, 1fr); }
}
</style>
