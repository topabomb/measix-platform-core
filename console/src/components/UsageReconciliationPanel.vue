<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ApiProblem, apiFetch, commandResultUncertain } from '../api/client'
import { useCursorPager } from '../composables/useCursorPager'
import CursorPager from './CursorPager.vue'
import type { PricingMeter, ReconciliationPage, ReconciliationView, ResolveReconciliationRequest } from '../api/usageBudget'
import { formatMeter, type MeterUnitLabels } from '../usageFormatting'
import { useSessionStore } from '../stores/session'
import LoadingState from './LoadingState.vue'
import ProblemBanner from './ProblemBanner.vue'
import DetailWorkspace from './DetailWorkspace.vue'
import UsageRequestDetail from './UsageRequestDetail.vue'

const { t: $t, te: $te, locale } = useI18n()
const unitLabels = computed<MeterUnitLabels>(() => ({
  tokens: $t('usage.units.tokens'),
  characters: $t('usage.units.characters'),
  seconds: $t('usage.units.seconds'),
  minutes: $t('usage.units.minutes'),
  requests: $t('usage.units.requests'),
  images: $t('usage.units.images'),
}))
const session = useSessionStore()
const listPath = ref('/api/admin/v1/usage/reconciliations?limit=50')
const { items, nextCursor, pageNumber, loading, error, reset: load, nextPage, previousPage } = useCursorPager<ReconciliationView, ReconciliationPage>(listPath, path => apiFetch<ReconciliationPage>(path))
const resolving = ref(false)
const selectedIds = ref<string[]>([])
const targets = ref<ReconciliationView[]>([])
const bulkReview = ref(false)
const selectedPage = computed(() => items.value.filter(item => item.state === 'PENDING' && selectedIds.value.includes(item.requestId)))
const completed = ref(false)
const uncertain = ref(false)
const outcomes = ref<Array<{ item: ReconciliationView; status: 'success' | 'failed' | 'remaining'; error?: unknown }>>([])
const resultCounts = computed(() => ({ success: outcomes.value.filter(item => item.status === 'success').length, failed: outcomes.value.filter(item => item.status === 'failed').length, remaining: outcomes.value.filter(item => item.status === 'remaining').length }))
let mounted = true
const detail = ref<ReconciliationView>()
const dialogOpen = ref(false)
const reason = ref('')

const canResolve = computed(() => Boolean(session.csrfToken && targets.value.length && reason.value.trim() && !completed.value && !resolving.value))
const allSelected = computed({
  get: () => Boolean(selectedPage.value.length && selectedPage.value.length === items.value.filter(item => item.state === 'PENDING').length),
  set: (value: boolean) => { selectedIds.value = value ? items.value.filter(item => item.state === 'PENDING').map(item => item.requestId) : [] },
})
watch(items, () => { selectedIds.value = selectedIds.value.filter(id => items.value.some(item => item.requestId === id && item.state === 'PENDING')) })
onBeforeUnmount(() => { mounted = false })

function observedBusiness(item: ReconciliationView) { return item.observed.filter(meter => meter.meter !== 'REQUESTS') }
function incompleteMeters(item: ReconciliationView): string {
  const semantic = item.request?.semanticMeters ?? []
  const business = semantic.filter(meter => meter.meter !== 'REQUESTS' && (meter.meter !== 'TOTAL_TOKENS' || !semantic.some(other => other.meter === 'INPUT_TOKENS')))
  let meters: PricingMeter[] = business.filter(meter => meter.confidence !== 'EXACT').map(meter => meter.meter)
  if (!business.length && !observedBusiness(item).length && item.forwarded !== false) {
    meters = item.capability === 'MODEL' ? ['INPUT_TOKENS', 'OUTPUT_TOKENS'] : item.capability === 'ASR' ? ['AUDIO_SECONDS'] : item.capability === 'TTS' ? ['CHARACTERS'] : item.capability === 'IMAGE_GENERATION' ? ['REQUESTED_IMAGES'] : []
  }
  return meters.map(meter => $t('usage.meters.' + meter)).join(' / ')
}

function meterValue(quantity: string, meter: PricingMeter): string {
  return formatMeter(quantity, meter, locale.value, unitLabels.value)
}

function reasonText(value: string): string {
  return /^settlement revision \d+ is incomplete$/.test(value)
    ? $t('usage.reconciliation.reasons.incompleteSettlement')
    : value
}

function errorClassText(value: string): string {
  const key = `usage.reconciliation.errors.${value}`
  return $te(key) ? `${$t(key)} (${value})` : value
}

async function refresh() {
  selectedIds.value = []
  await load()
  if (!error.value && detail.value && !items.value.some(item => item.requestId === detail.value?.requestId)) detail.value = undefined
}

function openResolve(item: ReconciliationView) {
  openReviews([item])
}

function openReviews(rows: ReconciliationView[], bulk = false) {
  bulkReview.value = bulk
  targets.value = [...rows]
  reason.value = ''
  outcomes.value = []
  completed.value = false
  uncertain.value = false
  dialogOpen.value = true
}

async function turnPage(next: boolean) {
  selectedIds.value = []
  detail.value = undefined
  await (next ? nextPage() : previousPage())
}

function openDetail(item: ReconciliationView) {
  if (item.request) detail.value = item
}

function identity(item: ReconciliationView): string {
  return item.request?.userDisplayName || item.userId
}

function resourceName(item: ReconciliationView): string {
  return item.request?.resourceDisplayName || item.resourceId
}

async function resolve() {
  if (!session.csrfToken || !canResolve.value) return
  resolving.value = true
  const csrf = session.csrfToken
  const request: ResolveReconciliationRequest = { expectedState: 'PENDING', action: 'RELEASE_UNCERTAIN', reason: reason.value.trim() }
  outcomes.value = targets.value.map(item => ({ item, status: 'remaining' }))
  try {
    for (const outcome of outcomes.value) {
      if (!mounted) break
      try {
        await apiFetch<ReconciliationView>(`/api/admin/v1/usage/reconciliations/${outcome.item.requestId}:resolve`, { method: 'POST', body: JSON.stringify(request) }, csrf)
        outcome.status = 'success'
        items.value = items.value.filter(item => item.requestId !== outcome.item.requestId)
        if (detail.value?.requestId === outcome.item.requestId) detail.value = undefined
      } catch (cause) {
        outcome.status = 'failed'
        outcome.error = cause
        if (commandResultUncertain(cause) || cause instanceof ApiProblem && (cause.status === 401 || cause.status === 403)) {
          uncertain.value = commandResultUncertain(cause)
          break
        }
      }
    }
    completed.value = true
    if (!bulkReview.value && targets.value.length === 1 && resultCounts.value.success === 1) dialogOpen.value = false
    if (mounted) await refresh()
  } finally {
    resolving.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <DetailWorkspace :detail-open="Boolean(detail)" list-width="minmax(440px, 58%)">
    <template #list>
      <q-card flat bordered data-cy="usage-reconciliation-panel">
        <q-card-section class="row items-center q-pb-xs">
          <div class="col">
            <div class="text-subtitle1">{{ $t('usage.reconciliation.title') }}</div>
            <div class="text-caption text-grey-7">{{ $t('usage.reconciliation.subtitle') }}</div>
          </div>
          <q-btn flat dense icon="refresh" :aria-label="$t('common.refresh')" :loading="loading" :disable="resolving" @click="refresh" />
        </q-card-section>
        <q-card-section v-if="items.length" class="row items-center q-gutter-sm q-py-xs">
          <q-checkbox v-model="allSelected" :label="$t('usage.reconciliation.selectPage')" :disable="loading || resolving" data-cy="reconciliation-select-page" />
          <span class="text-caption">{{ $t('usage.reconciliation.selection', { count: selectedPage.length }) }}</span>
          <q-btn outline color="primary" :label="$t('usage.reconciliation.batchResolve')" :disable="!selectedPage.length || loading || resolving" @click="openReviews(selectedPage, true)" data-cy="reconciliation-batch-btn" />
        </q-card-section>
        <ProblemBanner :error="error" class="q-mx-md q-mb-sm" />
        <LoadingState v-if="loading && !items.length" />
        <q-list v-else-if="items.length" separator>
          <q-item
            v-for="item in items"
            :key="item.requestId"
            :clickable="Boolean(item.request)"
            :active="detail?.requestId === item.requestId"
            active-class="bg-purple-1"
            data-cy="reconciliation-row"
            @click="openDetail(item)"
          >
            <q-item-section side>
              <q-checkbox v-model="selectedIds" :val="item.requestId" :aria-label="$t('usage.reconciliation.selectRequest', { resource: resourceName(item) })" :disable="loading || resolving || item.state !== 'PENDING'" data-cy="reconciliation-select" @click.stop />
            </q-item-section>
            <q-item-section>
              <q-item-label class="row items-center q-gutter-xs">
                <span class="text-weight-medium">{{ resourceName(item) }}</span>
                <q-chip dense color="primary" text-color="white" size="sm">{{ $t(`usage.kind.${item.capability}`) }}</q-chip>
                <q-chip v-if="item.errorClass" dense color="negative" text-color="white" size="sm">{{ errorClassText(item.errorClass) }}</q-chip>
              </q-item-label>
              <q-item-label caption>
                {{ $t('usage.filters.user') }}: {{ identity(item) }}
                <template v-if="item.request?.deviceName"> · {{ $t('usage.detail.device') }}: {{ item.request.deviceName }}</template>
              </q-item-label>
              <q-item-label caption>
                {{ new Date(item.request?.startedAt || item.startedAt || item.admittedAt).toLocaleString() }}
                <template v-if="item.request?.durationMs !== undefined"> · {{ item.request.durationMs }} ms</template>
                <template v-if="!item.request"> · {{ item.clientProtocol }}</template>
              </q-item-label>
              <q-item-label caption class="text-break q-mt-xs">
                {{ $t('usage.reconciliation.reasonLabel') }}: {{ reasonText(item.reconciliationReason) }}
              </q-item-label>
              <div class="row items-center q-gutter-xs q-mt-xs">
                <q-chip dense :color="item.forwarded === true ? 'green-2' : item.forwarded === false ? 'orange-2' : 'grey-3'">
                  {{ item.forwarded === true ? $t('usage.reconciliation.forwarded') : item.forwarded === false ? $t('usage.reconciliation.notForwarded') : $t('common.unknown') }}
                </q-chip>
                <q-chip dense color="red-2">{{ $t('usage.settlement.RECONCILIATION_REQUIRED') }}</q-chip>
                <q-chip v-if="item.httpStatus" dense :class="item.httpStatus >= 400 ? 'text-negative' : 'text-grey-8'">HTTP {{ item.httpStatus }}</q-chip>
                <q-chip v-if="item.upstreamHttpStatus && item.upstreamHttpStatus !== item.httpStatus" dense class="text-grey-8">{{ $t('usage.reconciliation.upstream') }} {{ item.upstreamHttpStatus }}</q-chip>
              </div>
              <div class="row q-gutter-xs q-mt-xs">
                <q-chip v-for="meter in observedBusiness(item)" :key="`observed:${meter.meter}`" dense outline color="primary">
                  {{ $t('usage.reconciliation.observed') }} {{ $t(`usage.meters.${meter.meter}`) }}: {{ meterValue(meter.quantity, meter.meter) }}
                </q-chip>
                <q-chip v-for="meter in item.reservation" :key="`reserved:${meter.meter}`" dense outline color="grey-7">
                  {{ $t('usage.reconciliation.reserved') }} {{ $t(`usage.meters.${meter.meter}`) }}: {{ meterValue(meter.quantity, meter.meter) }}
                </q-chip>
              </div>
              <q-item-label v-if="incompleteMeters(item)" caption class="text-negative q-mt-xs">{{ $t('usage.reconciliation.missingMeters', { meters: incompleteMeters(item) }) }}</q-item-label>
              <q-item-label v-if="!observedBusiness(item).length" caption class="q-mt-xs">{{ $t('usage.reconciliation.noReliableUsage') }}</q-item-label>
            </q-item-section>
            <q-item-section side>
              <q-icon v-if="item.request" name="chevron_right" color="grey-6" />
              <q-btn v-else outline color="primary" :label="$t('usage.reconciliation.resolve')" @click.stop="openResolve(item)" />
            </q-item-section>
          </q-item>
        </q-list>
        <q-card-section v-else class="text-grey-7">{{ $t('usage.reconciliation.empty') }}</q-card-section>
        <CursorPager :page="pageNumber" :count="items.length" :has-next="Boolean(nextCursor)" :loading="loading || resolving" @previous="turnPage(false)" @next="turnPage(true)" />
      </q-card>
    </template>

    <template #detail>
      <UsageRequestDetail v-if="detail?.request" :request="detail.request" @close="detail = undefined">
        <div class="reconciliation-context q-mt-md">
          <div class="text-subtitle2">{{ $t('usage.reconciliation.contextTitle') }}</div>
          <div class="text-body2 q-mt-xs">{{ reasonText(detail.reconciliationReason) }}</div>
          <div class="text-caption text-grey-7 q-mt-xs text-break">{{ detail.requestId }}</div>
          <div class="row q-gutter-xs q-mt-sm">
            <q-chip v-for="meter in observedBusiness(detail)" :key="`detail-observed:${meter.meter}`" dense outline color="primary">
              {{ $t('usage.reconciliation.observed') }} {{ $t(`usage.meters.${meter.meter}`) }}: {{ meterValue(meter.quantity, meter.meter) }}
            </q-chip>
            <q-chip v-for="meter in detail.reservation" :key="`detail-reserved:${meter.meter}`" dense outline color="grey-7">
              {{ $t('usage.reconciliation.reserved') }} {{ $t(`usage.meters.${meter.meter}`) }}: {{ meterValue(meter.quantity, meter.meter) }}
            </q-chip>
          </div>
        </div>
        <template #actions>
          <q-btn outline color="primary" :label="$t('usage.reconciliation.resolve')" @click="openResolve(detail)" />
        </template>
      </UsageRequestDetail>
    </template>
  </DetailWorkspace>

  <q-dialog v-model="dialogOpen" persistent>
    <q-card class="reconciliation-dialog" data-cy="reconciliation-dialog">
      <q-card-section>
        <div class="text-h6">{{ targets.length > 1 ? $t('usage.reconciliation.batchTitle', { count: targets.length }) : $t('usage.reconciliation.resolveTitle') }}</div>
        <div class="text-body2 text-grey-7 q-mt-xs">{{ $t('usage.reconciliation.resolveWarning') }}</div>
      </q-card-section>
      <q-card-section v-if="targets.length" class="q-pt-none">
        <div v-if="resolving">{{ $t('usage.reconciliation.batchProgress', { done: resultCounts.success + resultCounts.failed, total: targets.length }) }}</div>
        <div v-if="completed" role="status">{{ $t('usage.reconciliation.batchResult', resultCounts) }}</div>
        <div v-if="uncertain" class="text-negative q-mt-sm">{{ $t('usage.reconciliation.batchUncertain') }}</div>
        <q-list class="reconciliation-preview" separator>
          <q-item v-for="item in targets" :key="item.requestId">
            <q-item-section><q-item-label>{{ resourceName(item) }} · {{ identity(item) }}</q-item-label><q-item-label caption>{{ new Date(item.request?.startedAt || item.startedAt || item.admittedAt).toLocaleString() }}</q-item-label>
              <ProblemBanner v-if="completed" :error="outcomes.find(outcome => outcome.item.requestId === item.requestId)?.error" />
            </q-item-section>
            <q-item-section v-if="completed" side>{{ $t('usage.reconciliation.' + (outcomes.find(outcome => outcome.item.requestId === item.requestId)?.status === 'success' ? 'batchSuccess' : outcomes.find(outcome => outcome.item.requestId === item.requestId)?.status === 'failed' ? 'batchFailed' : 'batchRemaining')) }}</q-item-section>
          </q-item>
        </q-list>
      </q-card-section>
      <q-card-section class="q-pt-none q-gutter-sm">
        <q-input v-model="reason" outlined autogrow :label="$t('usage.reconciliation.reason')" maxlength="500" counter :disable="resolving || completed" data-cy="reconciliation-reason" />
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat :label="completed ? $t('common.close') : $t('common.cancel')" v-close-popup :disable="resolving" />
        <q-btn v-if="!completed" color="primary" :label="$t('usage.reconciliation.confirm')" :disable="!canResolve" :loading="resolving" data-cy="confirm-reconciliation" @click="resolve" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<style scoped>
.reconciliation-dialog {
  width: min(560px, calc(100vw - 32px));
}
.reconciliation-preview { max-height: 240px; overflow: auto; }

.reconciliation-context {
  border-top: 1px solid rgba(0, 0, 0, 0.12);
  padding-top: 12px;
}
</style>
