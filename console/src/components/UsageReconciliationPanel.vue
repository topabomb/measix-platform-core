<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../api/client'
import { cursorPath } from '../api/pagination'
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
const page = ref<ReconciliationPage>({ items: [] })
const loading = ref(false)
const loadingMore = ref(false)
const resolving = ref(false)
const error = ref<unknown>()
const selected = ref<ReconciliationView>()
const detail = ref<ReconciliationView>()
const dialogOpen = ref(false)
const reason = ref('')

const canResolve = computed(() => Boolean(session.csrfToken && selected.value && reason.value.trim()))

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
  loading.value = true
  error.value = undefined
  try {
    const result = await apiFetch<ReconciliationPage>('/api/admin/v1/usage/reconciliations?limit=50')
    page.value = result
    if (detail.value && !result.items.some(item => item.requestId === detail.value?.requestId)) detail.value = undefined
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (!page.value.nextCursor || loadingMore.value) return
  loadingMore.value = true
  try {
    const next = await apiFetch<ReconciliationPage>(cursorPath('/api/admin/v1/usage/reconciliations?limit=50', page.value.nextCursor))
    page.value = { items: [...page.value.items, ...next.items], nextCursor: next.nextCursor }
  } catch (cause) {
    error.value = cause
  } finally {
    loadingMore.value = false
  }
}

function openResolve(item: ReconciliationView) {
  selected.value = item
  reason.value = ''
  dialogOpen.value = true
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
  if (!selected.value || !session.csrfToken || !canResolve.value) return
  resolving.value = true
  error.value = undefined
  try {
    const request: ResolveReconciliationRequest = {
      expectedState: 'PENDING',
      action: 'RELEASE_UNCERTAIN',
      reason: reason.value.trim(),
    }
    await apiFetch<ReconciliationView>(`/api/admin/v1/usage/reconciliations/${selected.value.requestId}:resolve`, {
      method: 'POST',
      body: JSON.stringify(request),
    }, session.csrfToken)
    dialogOpen.value = false
    if (detail.value?.requestId === selected.value.requestId) detail.value = undefined
    await refresh()
  } catch (cause) {
    error.value = cause
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
          <q-btn flat dense icon="refresh" :aria-label="$t('common.refresh')" :loading="loading" @click="refresh" />
        </q-card-section>
        <ProblemBanner :error="error" class="q-mx-md q-mb-sm" />
        <LoadingState v-if="loading && !page.items.length" />
        <q-list v-else-if="page.items.length" separator>
          <q-item
            v-for="item in page.items"
            :key="item.requestId"
            :clickable="Boolean(item.request)"
            :active="detail?.requestId === item.requestId"
            active-class="bg-purple-1"
            data-cy="reconciliation-row"
            @click="openDetail(item)"
          >
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
                · {{ item.clientProtocol }}
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
                <q-chip v-for="meter in item.observed" :key="`observed:${meter.meter}`" dense outline color="primary">
                  {{ $t('usage.reconciliation.observed') }} {{ $t(`usage.meters.${meter.meter}`) }}: {{ meterValue(meter.quantity, meter.meter) }}
                </q-chip>
                <q-chip v-for="meter in item.reservation" :key="`reserved:${meter.meter}`" dense outline color="grey-7">
                  {{ $t('usage.reconciliation.reserved') }} {{ $t(`usage.meters.${meter.meter}`) }}: {{ meterValue(meter.quantity, meter.meter) }}
                </q-chip>
              </div>
            </q-item-section>
            <q-item-section side>
              <q-icon v-if="item.request" name="chevron_right" color="grey-6" />
              <q-btn v-else outline color="primary" :label="$t('usage.reconciliation.resolve')" @click.stop="openResolve(item)" />
            </q-item-section>
          </q-item>
        </q-list>
        <q-card-section v-else class="text-grey-7">{{ $t('usage.reconciliation.empty') }}</q-card-section>
        <q-card-actions v-if="page.nextCursor" align="center">
          <q-btn flat :label="$t('common.loadMore')" :loading="loadingMore" @click="loadMore" />
        </q-card-actions>
      </q-card>
    </template>

    <template #detail>
      <UsageRequestDetail v-if="detail?.request" :request="detail.request" @close="detail = undefined">
        <div class="reconciliation-context q-mt-md">
          <div class="text-subtitle2">{{ $t('usage.reconciliation.contextTitle') }}</div>
          <div class="text-body2 q-mt-xs">{{ reasonText(detail.reconciliationReason) }}</div>
          <div class="text-caption text-grey-7 q-mt-xs text-break">{{ detail.requestId }}</div>
          <div class="row q-gutter-xs q-mt-sm">
            <q-chip v-for="meter in detail.observed" :key="`detail-observed:${meter.meter}`" dense outline color="primary">
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
        <div class="text-h6">{{ $t('usage.reconciliation.resolveTitle') }}</div>
        <div class="text-body2 text-grey-7 q-mt-xs">{{ $t('usage.reconciliation.resolveWarning') }}</div>
      </q-card-section>
      <q-card-section class="q-pt-none q-gutter-sm">
        <q-input v-model="reason" outlined autogrow :label="$t('usage.reconciliation.reason')" maxlength="500" counter data-cy="reconciliation-reason" />
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat :label="$t('common.cancel')" v-close-popup :disable="resolving" />
        <q-btn color="primary" :label="$t('usage.reconciliation.confirm')" :disable="!canResolve" :loading="resolving" data-cy="confirm-reconciliation" @click="resolve" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<style scoped>
.reconciliation-dialog {
  width: min(560px, calc(100vw - 32px));
}

.reconciliation-context {
  border-top: 1px solid rgba(0, 0, 0, 0.12);
  padding-top: 12px;
}
</style>
