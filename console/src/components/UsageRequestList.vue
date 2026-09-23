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
import { costAmounts } from '../api/cost'
import type { RequestUsageS02 } from '../api/usageBudget'
import ProblemBanner from './ProblemBanner.vue'
import DetailWorkspace from './DetailWorkspace.vue'
import CursorPager from './CursorPager.vue'
import UsageRequestDetail from './UsageRequestDetail.vue'
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

const { t: $t, te: $te } = useI18n()
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
  if (resourceId.startsWith('prv_')) return 'PROVIDER'
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
  const key = `usage.reconciliation.errors.${code}`
  if ($te(key)) return `${$t(key)} (${code})`
  switch (code) {
    case 'ROUTE_POLICY_DENIED': return `${$t('usage.errorReasons.routePolicyDenied')} (${code})`
    case 'INVALID_INTERACTION': return `${$t('usage.errorReasons.invalidInteraction')} (${code})`
    case 'MANAGED_SNAPSHOT_REQUIRED': return `${$t('usage.errorReasons.snapshotRequired')} (${code})`
    default: return code
  }
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
        <q-item v-for="req in items" :key="req.requestId" clickable :active="selected?.requestId === req.requestId" active-class="bg-purple-1" data-cy="usage-row" @click="openDetail(req)">
          <q-item-section>
            <q-item-label>
              {{ req.resourceDisplayName || req.resourceId || $t('usage.unnamedResource') }}
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
              <span v-if="req.cost" class="text-caption text-grey-7">{{ costAmounts(req.cost) }} · {{ $t(`usage.cost${req.cost.status === 'KNOWN' ? 'Known' : req.cost.status === 'PARTIAL' ? 'Partial' : 'Unknown'}`) }}</span>
              <q-chip dense :color="req.forwarded ? 'green-2' : 'orange-2'">{{ req.forwarded ? $t('usage.detail.forwarded').toLowerCase() : $t('usage.blocked').toLowerCase() }}</q-chip>
              <q-chip v-if="req.settlementState && req.settlementState !== 'NOT_REQUIRED'" dense :color="req.settlementState === 'SETTLED' ? 'green-2' : req.settlementState === 'RECONCILIATION_REQUIRED' ? 'red-2' : 'orange-2'">{{ $t(`usage.settlement.${req.settlementState}`) }}</q-chip>
              <q-chip v-if="req.requestCompleteness && req.requestCompleteness !== 'EXACT' && req.settlementState !== 'RECONCILIATION_REQUIRED'" dense :color="req.requestCompleteness === 'PARTIAL' ? 'orange-2' : 'grey-3'">{{ $t(`status.${req.requestCompleteness}`) }}</q-chip>
              <q-chip dense :class="req.httpStatus >= 400 ? 'text-negative' : 'text-grey-8'">HTTP {{ req.httpStatus }}</q-chip>
              <q-chip v-if="req.upstreamHttpStatus && req.upstreamHttpStatus !== req.httpStatus" dense class="text-grey-8">{{ $t('usage.reconciliation.upstream') }} {{ req.upstreamHttpStatus }}</q-chip>
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
      <UsageRequestDetail
        v-if="selected"
        :request="selected"
        :allow-filter-resource="allowFilterResource"
        @filter-resource="emit('filterResource', $event); selected = undefined"
        @close="selected = undefined"
      />
    </template>
  </DetailWorkspace>
</template>
