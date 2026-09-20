<script setup lang="ts">
// A request list is the audit view of the deployment: it is read to answer "who
// sent what, from where, and did it work". Everything here serves that reading —
// each row names its user and device, the selected period is always explicit,
// and how much has been loaded is always stated, so a list that fits on one page
// does not look like a list that has no paging at all.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import type { MeterQuantity, PricingMeter, RequestUsageS02 } from '../api/usageBudget'
import { formatMeter, type MeterUnitLabels } from '../usageFormatting'
import ProblemBanner from './ProblemBanner.vue'
import DetailWorkspace from './DetailWorkspace.vue'
import CursorPager from './CursorPager.vue'
import { useCursorPager } from '../composables/useCursorPager'

type RequestUsage = components['schemas']['RequestUsageView'] & Partial<RequestUsageS02>
type RequestUsagePage = { items: RequestUsage[], nextCursor?: string }

const props = withDefaults(defineProps<{
  /** Encoded filter query string including the leading `?` when non-empty. */
  query?: string
  pageSize?: number
  maxHeight?: string
  /** Only offered where the host can act on it; elsewhere the button is hidden. */
  allowFilterResource?: boolean
  /** A host already scoped to one user hides the user name repeated on every row. */
  showUser?: boolean
}>(), {
  query: '',
  pageSize: 50,
  maxHeight: '28rem',
  allowFilterResource: false,
  showUser: true,
})

const emit = defineEmits<{ filterResource: [string] }>()

const { t: $t, locale } = useI18n()
const selected = ref<RequestUsage>()
const requestBasePath = ref(`/api/admin/v1/usage/requests?limit=${props.pageSize}${props.query}`)
const {
  items,
  nextCursor,
  pageNumber,
  loading,
  error,
  reset,
  nextPage,
  previousPage,
} = useCursorPager<RequestUsage, RequestUsagePage>(requestBasePath, path => apiFetch<RequestUsagePage>(path), () => {
  selected.value = undefined
})

watch(() => [props.query, props.pageSize], () => {
  requestBasePath.value = `/api/admin/v1/usage/requests?limit=${props.pageSize}${props.query}`
  void reset()
}, { immediate: true })

const loaded = computed(() => items.value.length)

function identity(req: RequestUsage): string {
  // userDisplayName always resolves: a usage row cannot exist without its user.
  return req.userDisplayName || req.userId
}

function openDetail(req: RequestUsage) {
  selected.value = req
}

function kindOf(resourceId: string | undefined): string | undefined {
  if (!resourceId) return undefined
  if (resourceId.startsWith('mdl_')) return 'MODEL'
  if (resourceId.startsWith('img_')) return 'IMAGE_GENERATION'
  if (resourceId.startsWith('tts_')) return 'TTS'
  if (resourceId.startsWith('asr_')) return 'ASR'
  if (resourceId.startsWith('mcp_')) return 'MCP'
  return undefined
}

function requestKind(req: RequestUsage): string | undefined {
  return req.resourceKind ?? kindOf(req.resourceId)
}

function kindColor(kind: string): string {
  switch (kind) {
    case 'MODEL': return 'primary'
    case 'IMAGE_GENERATION': return 'orange'
    case 'TTS': return 'teal'
    case 'ASR': return 'indigo'
    case 'MCP': return 'deep-purple'
    default: return 'grey'
  }
}

function kindLabel(kind: string): string {
  return $t(`usage.kind.${kind}`)
}

function errorLabel(code: string): string {
  switch (code) {
    case 'ROUTE_POLICY_DENIED': return $t('usage.errorReasons.routePolicyDenied')
    case 'INVALID_INTERACTION': return $t('usage.errorReasons.invalidInteraction')
    case 'MANAGED_SNAPSHOT_REQUIRED': return $t('usage.errorReasons.snapshotRequired')
    default: return code
  }
}

function fmtBytes(n: number | undefined): string {
  if (n === undefined) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

const unitLabels = computed<MeterUnitLabels>(() => ({
  tokens: $t('usage.units.tokens'),
  characters: $t('usage.units.characters'),
  seconds: $t('usage.units.seconds'),
  minutes: $t('usage.units.minutes'),
  requests: $t('usage.units.requests'),
  images: $t('usage.units.images'),
}))

function meterLabel(meter: PricingMeter): string {
  return $t(`usage.meters.${meter}`)
}

function meterValue(item: MeterQuantity, compact: boolean): string {
  return formatMeter(item.quantity, item.meter, locale.value, unitLabels.value, compact)
}

defineExpose({ refresh: reset })
</script>

<template>
  <DetailWorkspace :detail-open="Boolean(selected)" list-width="minmax(420px, 58%)">
    <template #list>
  <q-card flat bordered>
    <q-card-section class="row items-center justify-between q-py-xs">
      <div class="text-subtitle2">{{ $t('usage.requests') }}</div>
      <div class="row items-center q-gutter-xs">
        <!-- View controls for this list (page size and the like) belong next to
             the count they affect, not among the query filters above. -->
        <slot name="toolbar" />
        <div class="text-caption text-grey-7">{{ $t('common.loadedCount', { count: loaded }) }}</div>
      </div>
    </q-card-section>
    <div class="text-caption text-grey-7 card-inset">{{ $t('usage.windowHint') }}</div>
    <ProblemBanner :error="error" class="card-inset q-mt-xs" />
    <!-- Bounded height: a long page scrolls inside the card instead of pushing the
         filter controls and the rest of the page out of reach. -->
    <div :style="{ maxHeight, overflowY: 'auto' }" class="q-mt-xs">
      <q-list separator>
        <q-item v-for="req in items" :key="req.requestId" clickable data-cy="usage-row" @click="openDetail(req)">
          <q-item-section>
            <q-item-label>
              {{ req.resourceDisplayName || $t('usage.unnamedResource') }}
              <q-chip v-if="requestKind(req)" dense :color="kindColor(requestKind(req)!)" text-color="white" size="sm">{{ kindLabel(requestKind(req)!) }}</q-chip>
              <q-chip v-if="req.errorClass" dense color="negative" text-color="white" size="sm">{{ errorLabel(req.errorClass) }}</q-chip>
            </q-item-label>
            <q-item-label v-if="showUser || req.deviceName" caption data-cy="usage-row-identity">
              <template v-if="showUser">{{ $t('usage.filters.user') }}: {{ identity(req) }}</template>
              <template v-if="showUser && req.deviceName"> · </template>
              <template v-if="req.deviceName">{{ $t('usage.detail.device') }}: {{ req.deviceName }}</template>
            </q-item-label>
            <q-item-label caption>
              {{ new Date(req.startedAt).toLocaleString() }}
              <template v-if="req.durationMs !== undefined"> · {{ req.durationMs }} ms</template>
              <template v-if="req.clientProtocol"> · {{ req.clientProtocol }}</template>
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-xs">
              <q-chip dense :color="req.forwarded ? 'green-2' : 'orange-2'">{{ req.forwarded ? $t('usage.detail.forwarded').toLowerCase() : $t('usage.blocked').toLowerCase() }}</q-chip>
              <q-chip v-if="req.settlementState && req.settlementState !== 'NOT_REQUIRED'" dense :color="req.settlementState === 'SETTLED' ? 'green-2' : req.settlementState === 'RECONCILIATION_REQUIRED' ? 'red-2' : 'orange-2'">{{ $t(`usage.settlement.${req.settlementState}`) }}</q-chip>
              <q-chip dense :class="req.httpStatus >= 400 ? 'text-negative' : 'text-grey-8'">{{ req.httpStatus }}</q-chip>
            </div>
          </q-item-section>
        </q-item>
        <q-item v-if="!items.length && !loading"><q-item-section class="text-grey-7">{{ $t('usage.noRequests') }}</q-item-section></q-item>
      </q-list>
    </div>
    <CursorPager
      :page="pageNumber"
      :count="items.length"
      :has-next="Boolean(nextCursor)"
      :loading="loading"
      @previous="previousPage"
      @next="nextPage"
    />

  </q-card>
    </template>
    <template #detail>
      <q-card v-if="selected" flat bordered data-cy="usage-detail">
        <q-card-section>
          <div class="text-h6">{{ $t('usage.detail.title') }}</div>
          <div class="text-caption text-grey-7">{{ selected?.requestId }}</div>
        </q-card-section>
        <q-card-section>
          <q-markup-table flat dense>
            <tbody>
              <tr><td class="text-grey-7">{{ $t('usage.detail.requestId') }}</td><td>{{ selected.requestId }}</td></tr>
              <tr v-if="selected.interactionId"><td class="text-grey-7">{{ $t('usage.detail.interactionId') }}</td><td>{{ selected.interactionId }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.user') }}</td><td>{{ identity(selected) }} <span class="text-caption text-grey-7">({{ selected.userId }})</span></td></tr>
              <tr v-if="selected.deviceId"><td class="text-grey-7">{{ $t('usage.detail.device') }}</td><td>{{ selected.deviceName || '—' }} <span class="text-caption text-grey-7">({{ selected.deviceId }})</span></td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.resource') }}</td><td>{{ selected.resourceDisplayName }}<div class="text-caption">{{ selected.resourceId }}</div></td></tr>
              <tr v-if="selected.resourceKind"><td class="text-grey-7">{{ $t('usage.filters.resourceKind') }}</td><td>{{ kindLabel(selected.resourceKind) }}</td></tr>
              <tr v-if="selected.clientProtocol"><td class="text-grey-7">{{ $t('usage.filters.protocol') }}</td><td class="text-break">{{ selected.clientProtocol }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.upstream') }}</td><td>{{ selected.upstreamId }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.runtimeRoute') }}</td><td>{{ selected.runtimeRouteId }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.generation') }}</td><td>{{ $t('releases.generation') }} {{ selected.managedGeneration }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('overview.desiredRevision') }}</td><td>{{ selected.controlRevision }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('common.status') }}</td><td>{{ selected.forwarded ? $t('usage.detail.forwarded').toLowerCase() : $t('usage.blocked').toLowerCase() }} · {{ selected.httpStatus }}<template v-if="selected.upstreamHttpStatus"> · {{ $t('usage.detail.upstream') }} {{ selected.upstreamHttpStatus }}</template></td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.duration') }}</td><td>{{ selected.durationMs }} ms</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.bytes') }}</td><td>{{ fmtBytes(selected.requestBytes) }} in · {{ fmtBytes(selected.responseBytes) }} out</td></tr>
              <tr v-if="selected.requestCompleteness"><td class="text-grey-7">{{ $t('usage.detail.usageCompleteness') }}</td><td>{{ $t(`status.${selected.requestCompleteness}`) }}</td></tr>
              <tr v-if="selected.settlementState"><td class="text-grey-7">{{ $t('usage.detail.settlement') }}</td><td>{{ $t(`usage.settlement.${selected.settlementState}`) }}</td></tr>
              <tr v-if="selected.errorClass"><td class="text-grey-7">{{ $t('usage.errorClass') }}</td><td>{{ errorLabel(selected.errorClass) }} <span class="text-caption text-grey-7">({{ selected.errorClass }})</span></td></tr>
            </tbody>
          </q-markup-table>
          <div class="text-subtitle2 q-mt-sm">{{ $t('usage.detail.semanticMeters') }}</div>
          <div v-if="selected.semanticMeters?.length" class="usage-detail-meters q-mt-xs">
            <div v-for="meter in selected.semanticMeters" :key="meter.meter" class="usage-detail-meter">
              <div>{{ meterLabel(meter.meter) }}</div>
              <div class="text-weight-medium">{{ meterValue(meter, false) }}</div>
              <div class="text-caption text-grey-7">{{ $t(`status.${meter.confidence}`) }}</div>
            </div>
          </div>
          <div v-else class="text-body2 text-grey-7 q-mt-xs">{{ $t('usage.noSemanticMeters') }}</div>
          <template v-if="selected.budget">
            <div class="text-subtitle2 q-mt-sm">{{ $t('usage.detail.budget') }}</div>
            <div class="text-body2">
              {{ $t(`usage.kind.${selected.budget.capability}`) }} · {{ $t(`budgets.mode.${selected.budget.mode}`) }} · {{ $t('budgets.revision', { revision: selected.budget.revision }) }}
            </div>
            <div v-if="selected.budget.blockers.length" class="usage-detail-meters q-mt-xs">
              <div v-for="blocker in selected.budget.blockers" :key="`${blocker.period}:${blocker.meter}`" class="usage-detail-meter">
                <div>{{ meterLabel(blocker.meter) }} · {{ $t(`budgets.period.${blocker.period}`) }}</div>
                <div class="text-caption">{{ $t('budgets.remaining') }}: {{ formatMeter(blocker.remaining, blocker.meter, locale, unitLabels, false) }}</div>
              </div>
            </div>
          </template>
          <div class="text-caption text-grey-7 q-mt-xs">{{ $t('usage.detail.secretHint') }}</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-if="allowFilterResource && selected?.resourceId" flat :label="$t('usage.filterThisResource')" data-cy="filter-this-resource" @click="emit('filterResource', selected!.resourceId!); selected = undefined" />
          <q-btn flat icon="arrow_back" :label="$t('common.close')" @click="selected = undefined" />
        </q-card-actions>
      </q-card>
    </template>
  </DetailWorkspace>
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
  .usage-detail-meters {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
