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
import { cursorPath } from '../api/pagination'
import ProblemBanner from './ProblemBanner.vue'

type RequestUsage = components['schemas']['RequestUsageView']
type RequestUsagePage = components['schemas']['RequestUsagePage']

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

const { t: $t } = useI18n()
const items = ref<RequestUsage[]>([])
const nextCursor = ref<string>()
const loading = ref(false)
const error = ref<unknown>()
const selected = ref<RequestUsage>()
const detailOpen = ref(false)

// A response that arrives after the filters changed describes a list the operator
// is no longer looking at, so it is discarded rather than merged.
let sequence = 0

function requestPath(cursor?: string): string {
  const base = `/api/admin/v1/usage/requests?limit=${props.pageSize}${props.query}`
  return cursor ? cursorPath(base, cursor) : base
}

async function load(more: boolean) {
  if (more && (!nextCursor.value || loading.value)) return
  const current = ++sequence
  loading.value = true
  error.value = undefined
  if (!more) {
    items.value = []
    nextCursor.value = undefined
  }
  try {
    const page = await apiFetch<RequestUsagePage>(requestPath(more ? nextCursor.value : undefined))
    if (current !== sequence) return
    items.value = more ? [...items.value, ...page.items] : page.items
    nextCursor.value = page.nextCursor
  } catch (cause) {
    if (current === sequence) error.value = cause
  } finally {
    if (current === sequence) loading.value = false
  }
}

watch(() => [props.query, props.pageSize], () => { void load(false) }, { immediate: true })

const loaded = computed(() => items.value.length)

function identity(req: RequestUsage): string {
  // userDisplayName always resolves: a usage row cannot exist without its user.
  return req.userDisplayName || req.userId
}

function openDetail(req: RequestUsage) {
  selected.value = req
  detailOpen.value = true
}

function kindOf(resourceId: string | undefined): string | undefined {
  if (!resourceId) return undefined
  if (resourceId.startsWith('mdl_')) return 'MODEL'
  if (resourceId.startsWith('tts_')) return 'TTS'
  if (resourceId.startsWith('asr_')) return 'ASR'
  if (resourceId.startsWith('mcp_')) return 'MCP'
  return undefined
}

function kindColor(kind: string): string {
  switch (kind) {
    case 'MODEL': return 'primary'
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

defineExpose({ refresh: () => load(false) })
</script>

<template>
  <q-card flat bordered>
    <q-card-section class="row items-center justify-between q-py-xs">
      <div class="text-subtitle2">{{ $t('usage.requests') }}</div>
      <div class="row items-center q-gutter-xs">
        <!-- View controls for this list (page size and the like) belong next to
             the count they affect, not among the query filters above. -->
        <slot name="toolbar" />
        <div class="text-caption text-grey-7">
          {{ $t('common.loadedCount', { count: loaded }) }}
          <template v-if="nextCursor"> · {{ $t('common.hasMore') }}</template>
          <template v-else-if="loaded"> · {{ $t('common.allLoaded') }}</template>
        </div>
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
              <q-chip v-if="kindOf(req.resourceId)" dense :color="kindColor(kindOf(req.resourceId)!)" text-color="white" size="sm">{{ kindLabel(kindOf(req.resourceId)!) }}</q-chip>
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
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-xs">
              <q-chip dense :color="req.forwarded ? 'green-2' : 'orange-2'">{{ req.forwarded ? $t('usage.detail.forwarded').toLowerCase() : $t('usage.blocked').toLowerCase() }}</q-chip>
              <q-chip dense :class="req.httpStatus >= 400 ? 'text-negative' : 'text-grey-8'">{{ req.httpStatus }}</q-chip>
            </div>
          </q-item-section>
        </q-item>
        <q-item v-if="!items.length && !loading"><q-item-section class="text-grey-7">{{ $t('usage.noRequests') }}</q-item-section></q-item>
      </q-list>
    </div>
    <q-card-actions class="justify-center">
      <q-btn v-if="nextCursor" outline :label="$t('common.loadMore')" :loading="loading" data-cy="load-more" @click="load(true)" />
      <span v-else-if="loading" class="text-caption text-grey-7">{{ $t('common.loading') }}</span>
    </q-card-actions>

    <q-dialog v-model="detailOpen" data-cy="usage-detail">
      <q-card class="app-dialog">
        <q-card-section>
          <div class="text-h6">{{ $t('usage.detail.title') }}</div>
          <div class="text-caption text-grey-7">{{ selected?.requestId }}</div>
        </q-card-section>
        <q-card-section v-if="selected" class="app-dialog__body">
          <q-markup-table flat dense>
            <tbody>
              <tr><td class="text-grey-7">{{ $t('usage.detail.requestId') }}</td><td>{{ selected.requestId }}</td></tr>
              <tr v-if="selected.interactionId"><td class="text-grey-7">{{ $t('usage.detail.interactionId') }}</td><td>{{ selected.interactionId }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.user') }}</td><td>{{ identity(selected) }} <span class="text-caption text-grey-7">({{ selected.userId }})</span></td></tr>
              <tr v-if="selected.deviceId"><td class="text-grey-7">{{ $t('usage.detail.device') }}</td><td>{{ selected.deviceName || '—' }} <span class="text-caption text-grey-7">({{ selected.deviceId }})</span></td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.resource') }}</td><td>{{ selected.resourceDisplayName }}<div class="text-caption">{{ selected.resourceId }}</div></td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.upstream') }}</td><td>{{ selected.upstreamId }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.runtimeRoute') }}</td><td>{{ selected.runtimeRouteId }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.generation') }}</td><td>{{ $t('releases.generation') }} {{ selected.managedGeneration }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('overview.desiredRevision') }}</td><td>{{ selected.controlRevision }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('common.status') }}</td><td>{{ selected.forwarded ? $t('usage.detail.forwarded').toLowerCase() : $t('usage.blocked').toLowerCase() }} · {{ selected.httpStatus }}<template v-if="selected.upstreamHttpStatus"> · {{ $t('usage.detail.upstream') }} {{ selected.upstreamHttpStatus }}</template></td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.detail.duration') }}</td><td>{{ selected.durationMs }} ms</td></tr>
              <tr><td class="text-grey-7">{{ $t('usage.bytes') }}</td><td>{{ fmtBytes(selected.requestBytes) }} in · {{ fmtBytes(selected.responseBytes) }} out</td></tr>
              <tr v-if="selected.errorClass"><td class="text-grey-7">{{ $t('usage.errorClass') }}</td><td>{{ errorLabel(selected.errorClass) }} <span class="text-caption text-grey-7">({{ selected.errorClass }})</span></td></tr>
            </tbody>
          </q-markup-table>
          <div class="text-caption text-grey-7 q-mt-xs">{{ $t('usage.detail.secretHint') }}</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-if="allowFilterResource && selected?.resourceId" flat :label="$t('usage.filterThisResource')" data-cy="filter-this-resource" @click="emit('filterResource', selected!.resourceId!); detailOpen = false" />
          <q-btn flat :label="$t('common.close')" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-card>
</template>