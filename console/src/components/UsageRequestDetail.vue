<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import type { MeterQuantity, PricingMeter, RequestUsageS02 } from '../api/usageBudget'
import { formatMeter, type MeterUnitLabels } from '../usageFormatting'

type RequestUsage = components['schemas']['RequestUsageView'] & Partial<RequestUsageS02>

const props = withDefaults(defineProps<{
  request: RequestUsage
  allowFilterResource?: boolean
}>(), { allowFilterResource: false })

const emit = defineEmits<{
  close: []
  filterResource: [string]
}>()

const { t: $t, te: $te, locale } = useI18n()
const unitLabels = computed<MeterUnitLabels>(() => ({
  tokens: $t('usage.units.tokens'),
  characters: $t('usage.units.characters'),
  seconds: $t('usage.units.seconds'),
  minutes: $t('usage.units.minutes'),
  requests: $t('usage.units.requests'),
  images: $t('usage.units.images'),
}))

function identity(): string {
  return props.request.userDisplayName || props.request.userId
}

function kindLabel(kind: string): string {
  return $t(`usage.kind.${kind}`)
}

function errorLabel(code: string): string {
  const key = `usage.reconciliation.errors.${code}`
  return $te(key) ? $t(key) : code
}

function fmtBytes(n: number | undefined): string {
  if (n === undefined) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

function meterLabel(meter: PricingMeter): string {
  return $t(`usage.meters.${meter}`)
}

function meterValue(item: MeterQuantity): string {
  return formatMeter(item.quantity, item.meter, locale.value, unitLabels.value, false)
}
</script>

<template>
  <q-card flat bordered data-cy="usage-detail">
    <q-card-section>
      <div class="text-h6">{{ $t('usage.detail.title') }}</div>
      <div class="text-caption text-grey-7 text-break">{{ request.requestId }}</div>
    </q-card-section>
    <q-card-section>
      <q-markup-table flat dense>
        <tbody>
          <tr><td class="text-grey-7">{{ $t('usage.detail.requestId') }}</td><td class="text-break">{{ request.requestId }}</td></tr>
          <tr v-if="request.interactionId"><td class="text-grey-7">{{ $t('usage.detail.interactionId') }}</td><td class="text-break">{{ request.interactionId }}</td></tr>
          <tr><td class="text-grey-7">{{ $t('usage.detail.user') }}</td><td>{{ identity() }} <span class="text-caption text-grey-7">({{ request.userId }})</span></td></tr>
          <tr v-if="request.deviceId"><td class="text-grey-7">{{ $t('usage.detail.device') }}</td><td>{{ request.deviceName || '—' }} <span class="text-caption text-grey-7">({{ request.deviceId }})</span></td></tr>
          <tr><td class="text-grey-7">{{ $t('usage.detail.resource') }}</td><td>{{ request.resourceDisplayName || request.resourceId || $t('usage.unnamedResource') }}<div v-if="request.resourceDisplayName && request.resourceId" class="text-caption text-break">{{ request.resourceId }}</div></td></tr>
          <tr v-if="request.resourceKind"><td class="text-grey-7">{{ $t('usage.filters.resourceKind') }}</td><td>{{ kindLabel(request.resourceKind) }}</td></tr>
          <tr v-if="request.clientProtocol"><td class="text-grey-7">{{ $t('usage.filters.protocol') }}</td><td class="text-break">{{ request.clientProtocol }}</td></tr>
          <tr><td class="text-grey-7">{{ $t('usage.detail.upstream') }}</td><td class="text-break">{{ request.upstreamId }}</td></tr>
          <tr><td class="text-grey-7">{{ $t('usage.detail.runtimeRoute') }}</td><td class="text-break">{{ request.runtimeRouteId }}</td></tr>
          <tr><td class="text-grey-7">{{ $t('usage.detail.generation') }}</td><td>{{ $t('releases.generation') }} {{ request.managedGeneration }}</td></tr>
          <tr><td class="text-grey-7">{{ $t('overview.desiredRevision') }}</td><td>{{ request.controlRevision }}</td></tr>
          <tr><td class="text-grey-7">{{ $t('common.status') }}</td><td>{{ request.forwarded ? $t('usage.detail.forwarded').toLowerCase() : $t('usage.blocked').toLowerCase() }} · HTTP {{ request.httpStatus }}<template v-if="request.upstreamHttpStatus"> · {{ $t('usage.detail.upstream') }} HTTP {{ request.upstreamHttpStatus }}</template></td></tr>
          <tr><td class="text-grey-7">{{ $t('usage.detail.duration') }}</td><td>{{ request.durationMs }} ms</td></tr>
          <tr><td class="text-grey-7">{{ $t('usage.bytes') }}</td><td>{{ fmtBytes(request.requestBytes) }} in · {{ fmtBytes(request.responseBytes) }} out</td></tr>
          <tr v-if="request.requestCompleteness"><td class="text-grey-7">{{ $t('usage.detail.usageCompleteness') }}</td><td>{{ $t(`status.${request.requestCompleteness}`) }}</td></tr>
          <tr v-if="request.settlementState"><td class="text-grey-7">{{ $t('usage.detail.settlement') }}</td><td>{{ $t(`usage.settlement.${request.settlementState}`) }}</td></tr>
          <tr v-if="request.errorClass"><td class="text-grey-7">{{ $t('usage.errorClass') }}</td><td>{{ errorLabel(request.errorClass) }} <span class="text-caption text-grey-7">({{ request.errorClass }})</span></td></tr>
        </tbody>
      </q-markup-table>
      <div class="text-subtitle2 q-mt-sm">{{ $t('usage.detail.semanticMeters') }}</div>
      <div v-if="request.semanticMeters?.length" class="usage-detail-meters q-mt-xs">
        <div v-for="meter in request.semanticMeters" :key="meter.meter" class="usage-detail-meter">
          <div>{{ meterLabel(meter.meter) }}</div>
          <div class="text-weight-medium">{{ meterValue(meter) }}</div>
          <div class="text-caption text-grey-7">{{ $t(`status.${meter.confidence}`) }}</div>
        </div>
      </div>
      <div v-else class="text-body2 text-grey-7 q-mt-xs">{{ $t('usage.noSemanticMeters') }}</div>
      <template v-if="request.budget">
        <div class="text-subtitle2 q-mt-sm">{{ $t('usage.detail.budget') }}</div>
        <div class="text-body2">
          {{ $t(`usage.kind.${request.budget.capability}`) }} · {{ $t(`budgets.mode.${request.budget.mode}`) }} · {{ $t('budgets.revision', { revision: request.budget.revision }) }}
        </div>
        <div v-if="request.budget.blockers.length" class="usage-detail-meters q-mt-xs">
          <div v-for="blocker in request.budget.blockers" :key="`${blocker.period}:${blocker.meter}`" class="usage-detail-meter">
            <div>{{ meterLabel(blocker.meter) }} · {{ $t(`budgets.period.${blocker.period}`) }}</div>
            <div class="text-caption">{{ $t('budgets.remaining') }}: {{ formatMeter(blocker.remaining, blocker.meter, locale, unitLabels, false) }}</div>
          </div>
        </div>
      </template>
      <div class="text-caption text-grey-7 q-mt-xs">{{ $t('usage.detail.secretHint') }}</div>
      <slot />
    </q-card-section>
    <q-card-actions align="right">
      <slot name="actions" />
      <q-btn v-if="allowFilterResource && request.resourceId" flat :label="$t('usage.filterThisResource')" data-cy="filter-this-resource" @click="emit('filterResource', request.resourceId)" />
      <q-btn flat icon="arrow_back" :label="$t('common.close')" @click="emit('close')" />
    </q-card-actions>
  </q-card>
</template>

<style scoped>
.usage-detail-meters {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
}

.usage-detail-meter {
  min-width: 0;
  border: 1px solid rgba(0, 0, 0, 0.12);
  border-radius: 4px;
  padding: 6px;
  overflow-wrap: anywhere;
}

@media (max-width: 420px) {
  .usage-detail-meters { grid-template-columns: minmax(0, 1fr); }
}
</style>
