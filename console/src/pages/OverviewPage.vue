<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import PageHeader from '../components/PageHeader.vue'

const { t: $t } = useI18n()

type SystemStatus = components['schemas']['SystemStatus']
type UsageSummary = components['schemas']['UsageSummary']
type Upstream = components['schemas']['Upstream']
type UpstreamPage = components['schemas']['UpstreamPage']
type Draft = components['schemas']['Draft']

const system = ref<SystemStatus>()
const usage = ref<UsageSummary>()
const upstreams = ref<Upstream[]>([])
const draft = ref<Draft>()
const loading = ref(false)
const error = ref<unknown>()

const converged = computed(() => {
  if (!system.value) return false
  if (system.value.appliedControlRevision !== undefined && system.value.appliedControlRevision !== system.value.desiredControlRevision) return false
  if (system.value.appliedBundleHash && system.value.desiredBundleHash && system.value.appliedBundleHash !== system.value.desiredBundleHash) return false
  return true
})

const upstreamCounts = computed(() => {
  const counts: Record<string, number> = {}
  for (const u of upstreams.value) counts[u.status] = (counts[u.status] ?? 0) + 1
  return counts
})

/** Resource counts by kind from the managed draft. */
const resourceCounts = computed(() => {
  const c = draft.value?.content
  if (!c) return { models: 0, tts: 0, asr: 0, mcp: 0, providers: 0 }
  return {
    models: c.models.length,
    tts: c.tts.length,
    asr: c.asr.length,
    mcp: c.mcp.length,
    providers: c.providers.length,
  }
})

/** Recent activation failures from latest activation. */
const recentActivationFailures = computed(() => {
  if (!system.value?.latestActivation) return []
  const act = system.value.latestActivation
  if (act.state === 'FAILED') return [act]
  return []
})

/** Cost completeness status. */
const costCompleteness = computed(() => usage.value?.cost.status ?? 'UNKNOWN')

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    const [systemStatus, usageSummary, upstreamPage, draftData] = await Promise.all([
      apiFetch<SystemStatus>('/api/admin/v1/system/status'),
      apiFetch<UsageSummary>('/api/admin/v1/usage/summary'),
      apiFetch<UpstreamPage>('/api/admin/v1/upstreams?limit=200'),
      apiFetch<Draft>('/api/admin/v1/draft').catch(() => undefined as Draft | undefined),
    ])
    system.value = systemStatus
    usage.value = usageSummary
    upstreams.value = upstreamPage.items
    draft.value = draftData
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <q-page padding data-cy="overview-page">
    <PageHeader :title="$t('overview.title')" :subtitle="$t('overview.subtitle')">
      <template #primary><q-btn flat icon="refresh" :label="$t('common.refresh')" :loading="loading" @click="refresh" /></template>
    </PageHeader>
    <ProblemBanner :error="error" class="q-mb-md" />
    <LoadingState v-if="loading && !system" />
    <template v-else-if="system">
      <q-banner v-if="system.runtimeStatus !== 'READY' || !system.relayReady || !converged" class="bg-orange-1 text-warning q-mb-md rounded-borders">
        {{ $t('overview.subtitle') }}
        <template v-if="!converged"> <span class="text-weight-medium">{{ $t('status.NOT_CONVERGED') }}</span> ({{ $t('overview.desiredRevision') }} {{ system.desiredControlRevision }} / {{ $t('overview.appliedRevision') }} {{ system.appliedControlRevision ?? '—' }}).</template>
      </q-banner>
      <div class="row q-col-gutter-md">
        <div class="col-12 col-sm-6 col-lg-3">
          <q-card flat bordered><q-card-section><div class="text-caption text-grey-7">{{ $t('overview.managedRuntime') }}</div><StatusChip :value="system.runtimeStatus" /><div class="text-caption q-mt-sm">{{ $t('system.relay') }} {{ system.relayReady ? $t('status.READY').toLowerCase() : $t('status.NOT_CONVERGED').toLowerCase() }}</div></q-card-section></q-card>
        </div>
        <div class="col-12 col-sm-6 col-lg-3">
          <q-card flat bordered><q-card-section><div class="text-caption text-grey-7">{{ $t('overview.activeGeneration') }}</div><div class="text-h4">{{ system.activeManagedGeneration }}</div><div class="text-caption">{{ $t('system.managedStateRevision') }} {{ system.managedStateRevision }}</div></q-card-section></q-card>
        </div>
        <div class="col-12 col-sm-6 col-lg-3">
          <q-card flat bordered><q-card-section><div class="text-caption text-grey-7">{{ $t('overview.desiredRevision') }}</div><div class="text-h4">{{ system.desiredControlRevision }}</div><div class="text-caption">{{ $t('system.relay') }} {{ system.appliedControlRevision ?? '—' }} · <span class="text-caption">{{ $t('system.bundle').toLowerCase() }} {{ system.desiredBundleHash ? system.desiredBundleHash.slice(7, 19) : '—' }}</span></div></q-card-section></q-card>
        </div>
        <div class="col-12 col-sm-6 col-lg-3">
          <q-card flat bordered><q-card-section><div class="text-caption text-grey-7">{{ $t('overview.requests') }}</div><div class="text-h4">{{ usage?.requestCount ?? 0 }}</div><div class="text-caption">{{ usage?.forwardedRequestCount ?? 0 }} {{ $t('usage.detail.forwarded').toLowerCase() }}</div></q-card-section></q-card>
        </div>
      </div>
      <div class="row q-col-gutter-md q-mt-xs">
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle1 text-weight-medium">{{ $t('system.currentActivation') }}</q-card-section>
            <q-list separator>
              <template v-if="system.latestActivation">
                <q-item><q-item-section><q-item-label>{{ system.latestActivation.activationId }}</q-item-label><q-item-label caption>{{ system.latestActivation.kind }} · rev {{ system.latestActivation.desiredControlRevision }}</q-item-label></q-item-section><q-item-section side><StatusChip :value="system.latestActivation.state" /></q-item-section></q-item>
              </template>
              <q-item v-else><q-item-section class="text-grey-7">{{ $t('common.noData') }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.lastReconciliation') }}</q-item-section><q-item-section side>{{ system.lastRelaySeenAt ?? '—' }}</q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle1 text-weight-medium">{{ $t('nav.upstreams') }}</q-card-section>
            <q-list separator>
              <q-item><q-item-section>{{ $t('status.ACTIVE') }}</q-item-section><q-item-section side>{{ upstreamCounts.ACTIVE ?? 0 }} {{ $t('status.ACTIVE').toLowerCase() }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('status.DEGRADED') }}</q-item-section><q-item-section side>{{ upstreamCounts.DEGRADED ?? 0 }} {{ $t('status.DEGRADED').toLowerCase() }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('status.DISABLED') }} / {{ $t('status.INACTIVE').toLowerCase() }}</q-item-section><q-item-section side>{{ (upstreamCounts.DISABLED ?? 0) + (upstreamCounts.INACTIVE ?? 0) }} {{ $t('status.DISABLED').toLowerCase() }}</q-item-section></q-item>
              <q-item v-if="!upstreams.length"><q-item-section class="text-grey-7">{{ $t('upstreams.noUpstreams') }}</q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
      </div>
      <!-- Resource counts by kind -->
      <div class="row q-col-gutter-md q-mt-xs">
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle1 text-weight-medium">{{ $t('overview.resourceCounts') }}</q-card-section>
            <q-list separator>
              <q-item><q-item-section><q-item-label>{{ $t('resources.tabs.models') }}</q-item-label></q-item-section><q-item-section side><q-badge color="primary" :label="String(resourceCounts.models)" /></q-item-section></q-item>
              <q-item><q-item-section><q-item-label>{{ $t('resources.tabs.tts') }}</q-item-label></q-item-section><q-item-section side><q-badge color="teal" :label="String(resourceCounts.tts)" /></q-item-section></q-item>
              <q-item><q-item-section><q-item-label>{{ $t('resources.tabs.asr') }}</q-item-label></q-item-section><q-item-section side><q-badge color="indigo" :label="String(resourceCounts.asr)" /></q-item-section></q-item>
              <q-item><q-item-section><q-item-label>{{ $t('resources.tabs.mcp') }}</q-item-label></q-item-section><q-item-section side><q-badge color="deep-purple" :label="String(resourceCounts.mcp)" /></q-item-section></q-item>
              <q-item><q-item-section><q-item-label>{{ $t('resources.preview.providers') }}</q-item-label></q-item-section><q-item-section side><q-badge color="grey" :label="String(resourceCounts.providers)" /></q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle1 text-weight-medium">{{ $t('overview.recentFailures') }}</q-card-section>
            <q-list separator>
              <q-item v-for="fail in recentActivationFailures" :key="fail.activationId">
                <q-item-section>
                  <q-item-label>{{ fail.activationId }}</q-item-label>
                  <q-item-label caption>{{ fail.kind }} · {{ fail.errorCode ?? $t('common.unknown').toLowerCase() }}</q-item-label>
                </q-item-section>
                <q-item-section side><StatusChip :value="fail.state" /></q-item-section>
              </q-item>
              <q-item v-if="!recentActivationFailures.length"><q-item-section class="text-positive">{{ $t('overview.noFailures') }}</q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
      </div>
      <div class="row q-col-gutter-md q-mt-xs">
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle1 text-weight-medium">{{ $t('system.title') }}</q-card-section>
            <q-list separator>
              <q-item><q-item-section>{{ $t('system.dbHealth') }}</q-item-section><q-item-section side>{{ system.dbHealth }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.managedStateRevision') }}</q-item-section><q-item-section side>{{ system.migrationRevision }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('overview.ingestLag') }}</q-item-section><q-item-section side>{{ system.requestUsageIngestLagSeconds ?? 0 }}s</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('system.semanticOrphan') }}</q-item-section><q-item-section side>{{ system.semanticOrphanCount ?? 0 }}</q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section class="text-subtitle1 text-weight-medium">{{ $t('overview.usageCompleteness') }}</q-card-section>
            <q-list separator>
              <q-item v-for="meter in usage?.semanticMeters ?? []" :key="meter.meter">
                <q-item-section><q-item-label>{{ meter.meter }}</q-item-label><q-item-label caption>{{ meter.quantity }}</q-item-label></q-item-section>
                <q-item-section side><StatusChip :value="meter.confidence" /></q-item-section>
              </q-item>
              <q-item v-if="!usage?.semanticMeters.length"><q-item-section class="text-grey-7">{{ $t('common.noData') }}</q-item-section></q-item>
              <q-item><q-item-section>{{ $t('overview.costStatus') }}</q-item-section><q-item-section side><StatusChip :value="usage?.cost.status ?? 'UNKNOWN'" /></q-item-section></q-item>
            </q-list>
          </q-card>
        </div>
      </div>
    </template>
  </q-page>
</template>
