<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../api/client'
import { cursorPath } from '../api/pagination'
import type { ReconciliationPage, ReconciliationView, ResolveReconciliationRequest } from '../api/usageBudget'
import { useSessionStore } from '../stores/session'
import LoadingState from './LoadingState.vue'
import ProblemBanner from './ProblemBanner.vue'

const { t: $t } = useI18n()
const session = useSessionStore()
const page = ref<ReconciliationPage>({ items: [] })
const loading = ref(false)
const loadingMore = ref(false)
const resolving = ref(false)
const error = ref<unknown>()
const selected = ref<ReconciliationView>()
const dialogOpen = ref(false)
const action = ref<ResolveReconciliationRequest['action']>('ACCEPT_OBSERVED')
const reason = ref('')

const canResolve = computed(() => Boolean(session.csrfToken && selected.value && reason.value.trim()))

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    page.value = await apiFetch<ReconciliationPage>('/api/admin/v1/usage/reconciliations?limit=50')
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
  action.value = 'ACCEPT_OBSERVED'
  reason.value = ''
  dialogOpen.value = true
}

async function resolve() {
  if (!selected.value || !session.csrfToken || !canResolve.value) return
  resolving.value = true
  error.value = undefined
  try {
    await apiFetch<ReconciliationView>(`/api/admin/v1/usage/reconciliations/${selected.value.requestId}:resolve`, {
      method: 'POST',
      body: JSON.stringify({ expectedState: 'PENDING', action: action.value, reason: reason.value.trim() }),
    }, session.csrfToken)
    dialogOpen.value = false
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
      <q-item v-for="item in page.items" :key="item.requestId" data-cy="reconciliation-row">
        <q-item-section>
          <q-item-label class="text-weight-medium">{{ item.userId }} · {{ $t(`usage.kind.${item.capability}`) }}</q-item-label>
          <q-item-label caption class="text-break">{{ item.requestId }} · {{ new Date(item.createdAt).toLocaleString() }}</q-item-label>
          <div class="row q-gutter-xs q-mt-xs">
            <q-chip v-for="meter in item.observed" :key="`observed:${meter.meter}`" dense outline color="primary">
              {{ $t('usage.reconciliation.observed') }} {{ meter.meter }}: {{ meter.quantity }}
            </q-chip>
            <q-chip v-for="meter in item.reservation" :key="`reserved:${meter.meter}`" dense outline color="grey-7">
              {{ $t('usage.reconciliation.reserved') }} {{ meter.meter }}: {{ meter.quantity }}
            </q-chip>
          </div>
        </q-item-section>
        <q-item-section side>
          <q-btn outline color="primary" :label="$t('usage.reconciliation.resolve')" @click="openResolve(item)" />
        </q-item-section>
      </q-item>
    </q-list>
    <q-card-section v-else class="text-grey-7">{{ $t('usage.reconciliation.empty') }}</q-card-section>
    <q-card-actions v-if="page.nextCursor" align="center">
      <q-btn flat :label="$t('common.loadMore')" :loading="loadingMore" @click="loadMore" />
    </q-card-actions>
  </q-card>

  <q-dialog v-model="dialogOpen" persistent>
    <q-card class="reconciliation-dialog" data-cy="reconciliation-dialog">
      <q-card-section>
        <div class="text-h6">{{ $t('usage.reconciliation.resolveTitle') }}</div>
        <div class="text-body2 text-grey-7 q-mt-xs">{{ $t('usage.reconciliation.resolveWarning') }}</div>
      </q-card-section>
      <q-card-section class="q-pt-none q-gutter-sm">
        <q-option-group v-model="action" :options="[
          { label: $t('usage.reconciliation.acceptObserved'), value: 'ACCEPT_OBSERVED' },
          { label: $t('usage.reconciliation.releaseUncertain'), value: 'RELEASE_UNCERTAIN' },
        ]" type="radio" color="primary" />
        <q-banner v-if="action === 'RELEASE_UNCERTAIN'" class="bg-orange-1">
          {{ $t('usage.reconciliation.releaseWarning') }}
        </q-banner>
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
</style>
