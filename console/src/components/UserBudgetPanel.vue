<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ApiProblem, apiFetch } from '../api/client'
import { cursorPath } from '../api/pagination'
import type {
  BudgetAuditItem,
  BudgetAuditPage,
  BudgetCapability,
  BudgetCapabilityView,
  BudgetLimitDefinition,
  BudgetMode,
  PricingMeter,
  PutBudgetRequest,
  UserBudgetView,
} from '../api/usageBudget'
import { budgetPeriods, capabilities, metersForCapability } from '../api/usageBudget'
import { formatMeter, type MeterUnitLabels } from '../usageFormatting'
import { useSessionStore } from '../stores/session'
import LoadingState from './LoadingState.vue'
import ProblemBanner from './ProblemBanner.vue'
import StatusChip from './StatusChip.vue'

const props = defineProps<{ userId: string }>()
const { t: $t, locale } = useI18n()
const session = useSessionStore()

const view = ref<UserBudgetView>()
const loading = ref(false)
const error = ref<unknown>()
const conflict = ref(false)
const editing = ref<BudgetCapability>()
const editMode = ref<BudgetMode>('UNLIMITED')
const editLimits = ref<BudgetLimitDefinition[]>([])
const editReason = ref('')
const saving = ref(false)
const audit = ref<Partial<Record<BudgetCapability, BudgetAuditItem[]>>>({})
const auditCursor = ref<Partial<Record<BudgetCapability, string>>>({})
const auditLoading = ref<BudgetCapability>()
const auditError = ref<unknown>()
let sequence = 0

const canMutate = computed(() => Boolean(session.csrfToken))
const unitLabels = computed<MeterUnitLabels>(() => ({
  tokens: $t('usage.units.tokens'),
  characters: $t('usage.units.characters'),
  seconds: $t('usage.units.seconds'),
  minutes: $t('usage.units.minutes'),
  requests: $t('usage.units.requests'),
}))

function capability(capabilityName: BudgetCapability): BudgetCapabilityView | undefined {
  return view.value?.items?.find(item => item.capability === capabilityName)
}

async function load() {
  const current = ++sequence
  loading.value = true
  error.value = undefined
  try {
    const result = await apiFetch<UserBudgetView>(`/api/admin/v1/users/${encodeURIComponent(props.userId)}/budgets`)
    if (current !== sequence) return
    if (!Array.isArray(result.items)) throw new Error('Invalid user budget response')
    view.value = result
  } catch (cause) {
    if (current === sequence) error.value = cause
  } finally {
    if (current === sequence) loading.value = false
  }
}

function beginEdit(item: BudgetCapabilityView) {
  conflict.value = false
  editing.value = item.capability
  editMode.value = item.mode
  editLimits.value = item.limits.map(({ period, meter, limit }) => ({ period, meter, limit }))
  editReason.value = ''
}

function addLimit(capabilityName: BudgetCapability) {
  const meters = metersForCapability(capabilityName)
  const used = new Set(editLimits.value.map(item => `${item.period}:${item.meter}`))
  for (const period of budgetPeriods) {
    const meter = meters.find(candidate => !used.has(`${period}:${candidate}`))
    if (meter) {
      editLimits.value.push({ period, meter, limit: '' })
      return
    }
  }
}

function removeLimit(index: number) {
  editLimits.value.splice(index, 1)
}

const editorValid = computed(() => {
  if (!editReason.value.trim()) return false
  if (editMode.value === 'UNLIMITED') return true
  if (!editLimits.value.length) return false
  const keys = new Set<string>()
  return editLimits.value.every(limit => {
    const key = `${limit.period}:${limit.meter}`
    if (keys.has(key) || !/^(0|[1-9]\d*)$/.test(limit.limit)) return false
    keys.add(key)
    return true
  })
})

async function save(item: BudgetCapabilityView) {
  if (!session.csrfToken || !editorValid.value) return
  saving.value = true
  error.value = undefined
  conflict.value = false
  const request: PutBudgetRequest = {
    expectedRevision: item.revision,
    mode: editMode.value,
    limits: editMode.value === 'LIMITED' ? editLimits.value : [],
    reason: editReason.value.trim(),
  }
  try {
    const updated = await apiFetch<BudgetCapabilityView>(
      `/api/admin/v1/users/${encodeURIComponent(props.userId)}/budgets/${item.capability}`,
      { method: 'PUT', body: JSON.stringify(request) },
      session.csrfToken,
    )
    if (view.value) view.value.items = view.value.items.map(candidate => candidate.capability === updated.capability ? updated : candidate)
    editing.value = undefined
    const nextAudit = { ...audit.value }
    delete nextAudit[item.capability]
    audit.value = nextAudit
  } catch (cause) {
    if (cause instanceof ApiProblem && cause.status === 409) {
      conflict.value = true
      editing.value = undefined
      await load()
    } else {
      error.value = cause
    }
  } finally {
    saving.value = false
  }
}

async function loadAudit(capabilityName: BudgetCapability, more = false) {
  if (auditLoading.value) return
  auditLoading.value = capabilityName
  auditError.value = undefined
  try {
    const base = `/api/admin/v1/users/${encodeURIComponent(props.userId)}/budgets/${capabilityName}/audit?limit=20`
    const path = more && auditCursor.value[capabilityName] ? cursorPath(base, auditCursor.value[capabilityName]) : base
    const page = await apiFetch<BudgetAuditPage>(path)
    audit.value[capabilityName] = more ? [...(audit.value[capabilityName] ?? []), ...page.items] : page.items
    auditCursor.value[capabilityName] = page.nextCursor
  } catch (cause) {
    auditError.value = cause
  } finally {
    auditLoading.value = undefined
  }
}

function ensureAudit(capabilityName: BudgetCapability) {
  if (!audit.value[capabilityName]) void loadAudit(capabilityName)
}

function meterLabel(meter: PricingMeter): string {
  return $t(`usage.meters.${meter}`)
}

function compactQuantity(quantity: string, meter: PricingMeter): string {
  return formatMeter(quantity, meter, locale.value, unitLabels.value, true)
}

function exactQuantity(quantity: string, meter: PricingMeter): string {
  return formatMeter(quantity, meter, locale.value, unitLabels.value, false)
}

watch(() => props.userId, () => {
  editing.value = undefined
  conflict.value = false
  audit.value = {}
  auditCursor.value = {}
  void load()
}, { immediate: true })
</script>

<template>
  <section data-cy="user-budgets">
    <div class="row items-start justify-between q-mb-xs">
      <div>
        <div class="text-subtitle2">{{ $t('budgets.title') }}</div>
        <div v-if="view" class="text-caption text-grey-7">
          {{ $t('budgets.timezone', { timezone: view.timezone }) }} · {{ $t('budgets.asOf', { time: new Date(view.asOf).toLocaleString() }) }}
        </div>
      </div>
      <q-btn flat dense icon="refresh" :aria-label="$t('common.refresh')" :loading="loading" @click="load" />
    </div>

    <q-banner v-if="conflict" class="bg-orange-1 q-mb-xs" data-cy="budget-conflict">
      {{ $t('budgets.conflictReloaded') }}
    </q-banner>
    <q-banner v-if="error && view" class="bg-orange-1 q-mb-xs">
      {{ $t('budgets.stale') }}
    </q-banner>
    <ProblemBanner :error="error" class="q-mb-xs" />
    <LoadingState v-if="loading && !view" />

    <div v-else-if="view" class="budget-grid">
      <q-card v-for="capabilityName in capabilities" :key="capabilityName" flat bordered class="budget-card" :data-cy="`budget-${capabilityName}`">
        <template v-if="capability(capabilityName)">
          <q-card-section class="q-pb-xs">
            <div class="row items-start justify-between no-wrap">
              <div>
                <div class="text-subtitle2">{{ $t(`usage.kind.${capabilityName}`) }}</div>
                <div class="row q-gutter-xs q-mt-xs">
                  <q-chip dense :color="capability(capabilityName)!.source === 'DEFAULT' ? 'blue-grey-2' : 'primary'" :text-color="capability(capabilityName)!.source === 'DEFAULT' ? 'blue-grey-10' : 'white'">
                    {{ $t(`budgets.source.${capability(capabilityName)!.source}`) }}
                  </q-chip>
                  <q-chip dense>{{ $t(`budgets.mode.${capability(capabilityName)!.mode}`) }}</q-chip>
                  <StatusChip :value="capability(capabilityName)!.status" />
                </div>
              </div>
              <q-btn flat dense icon="edit" :aria-label="$t('budgets.edit')" :disable="!canMutate" @click="beginEdit(capability(capabilityName)!)" />
            </div>
            <div v-if="capability(capabilityName)!.source === 'DEFAULT'" class="text-caption text-grey-7 q-mt-xs">
              {{ $t('budgets.defaultHint') }}
            </div>
            <div v-else-if="capability(capabilityName)!.mode === 'UNLIMITED'" class="text-caption text-grey-7 q-mt-xs">
              {{ $t('budgets.explicitUnlimitedHint') }}
            </div>
            <div class="text-caption text-grey-7 q-mt-xs">
              {{ $t('budgets.revision', { revision: capability(capabilityName)!.revision }) }} ·
              {{ $t('budgets.inFlight', { count: capability(capabilityName)!.inFlightRequests }) }}
            </div>
            <div class="usage-history q-mt-sm" data-cy="budget-usage-history">
              <div class="text-caption text-weight-medium">{{ $t('budgets.usageHistory') }}</div>
              <div class="text-caption text-grey-7">{{ $t('budgets.usageHistoryHint') }}</div>
              <div v-if="capability(capabilityName)!.usageMeters.length" class="row q-col-gutter-sm q-mt-xs">
                <div v-for="meter in capability(capabilityName)!.usageMeters" :key="meter.meter" class="col-auto">
                  <span class="text-weight-medium">{{ compactQuantity(meter.quantity, meter.meter) }}</span>
                  <span class="text-caption text-grey-7"> · {{ meterLabel(meter.meter) }}</span>
                </div>
              </div>
              <div v-else class="text-caption text-grey-7 q-mt-xs">{{ $t('budgets.noRecordedUsage') }}</div>
            </div>
          </q-card-section>

          <q-separator />
          <q-card-section v-if="editing === capabilityName" class="q-gutter-xs" data-cy="budget-editor">
            <div class="budget-mode-actions">
              <q-btn :outline="editMode !== 'UNLIMITED'" :color="editMode === 'UNLIMITED' ? 'primary' : undefined" no-caps :label="$t('budgets.mode.UNLIMITED')" @click="editMode = 'UNLIMITED'; editLimits = []" />
              <q-btn :outline="editMode !== 'LIMITED'" :color="editMode === 'LIMITED' ? 'primary' : undefined" no-caps :label="$t('budgets.mode.LIMITED')" @click="editMode = 'LIMITED'; if (!editLimits.length) addLimit(capabilityName)" />
            </div>
            <template v-if="editMode === 'LIMITED'">
              <div v-for="(limit, index) in editLimits" :key="index" class="budget-rule">
                <q-select v-model="limit.period" outlined dense emit-value map-options :label="$t('budgets.periodLabel')" :options="budgetPeriods.map(value => ({ value, label: $t(`budgets.period.${value}`) }))" />
                <q-select v-model="limit.meter" outlined dense emit-value map-options :label="$t('budgets.meterLabel')" :options="metersForCapability(capabilityName).map(value => ({ value, label: meterLabel(value) }))" />
                <q-input v-model="limit.limit" outlined dense inputmode="numeric" :label="$t('budgets.limitLabel')" :error="Boolean(limit.limit) && !/^(0|[1-9]\d*)$/.test(limit.limit)" />
                <q-btn flat dense round icon="delete" color="negative" :aria-label="$t('common.remove')" @click="removeLimit(index)" />
              </div>
              <q-btn flat dense no-caps icon="add" :label="$t('budgets.addLimit')" @click="addLimit(capabilityName)" />
            </template>
            <q-input v-model="editReason" outlined dense maxlength="500" :label="$t('budgets.reasonLabel')" :hint="$t('budgets.reasonHint')" />
            <div class="row justify-end q-gutter-xs">
              <q-btn flat :label="$t('common.cancel')" @click="editing = undefined" />
              <q-btn color="primary" :label="$t('common.save')" :loading="saving" :disable="!editorValid" data-cy="save-budget" @click="save(capability(capabilityName)!)" />
            </div>
          </q-card-section>

          <q-card-section v-else-if="capability(capabilityName)!.mode === 'UNLIMITED'" class="text-grey-7">
            {{ $t('budgets.noLimit') }}
          </q-card-section>
          <q-card-section v-else class="q-gutter-xs">
            <div v-for="limit in capability(capabilityName)!.limits" :key="`${limit.period}:${limit.meter}`" class="budget-limit" data-cy="budget-limit">
              <div class="row items-center justify-between no-wrap">
                <div class="text-weight-medium">{{ meterLabel(limit.meter) }} · {{ $t(`budgets.period.${limit.period}`) }}</div>
                <div class="text-caption">{{ compactQuantity(limit.remaining, limit.meter) }} {{ $t('budgets.remaining') }}</div>
              </div>
              <div class="text-caption text-grey-7">
                {{ $t('budgets.usedReserved', { used: compactQuantity(limit.used, limit.meter), reserved: compactQuantity(limit.reserved, limit.meter), limit: compactQuantity(limit.limit, limit.meter) }) }}
              </div>
              <div class="text-caption text-grey-7">
                <template v-if="limit.resetAt">{{ $t('budgets.resets', { time: new Date(limit.resetAt).toLocaleString() }) }}</template>
                <template v-else>{{ $t('budgets.neverResets') }}</template>
                <template v-if="limit.overage !== '0'"> · {{ $t('budgets.overage', { value: exactQuantity(limit.overage, limit.meter) }) }}</template>
              </div>
              <details class="text-caption q-mt-xs">
                <summary>{{ $t('budgets.preciseValues') }}</summary>
                {{ $t('budgets.preciseLine', {
                  used: exactQuantity(limit.used, limit.meter),
                  reserved: exactQuantity(limit.reserved, limit.meter),
                  remaining: exactQuantity(limit.remaining, limit.meter),
                  limit: exactQuantity(limit.limit, limit.meter),
                }) }}
              </details>
            </div>
          </q-card-section>

          <q-separator />
          <q-card-section class="q-py-xs">
            <details>
              <summary class="cursor-pointer text-caption text-primary" :data-cy="`budget-audit-${capabilityName}`" @click="ensureAudit(capabilityName)">{{ $t('budgets.audit') }}</summary>
              <div v-if="auditLoading === capabilityName" class="text-caption text-grey-7 q-mt-xs">{{ $t('common.loading') }}</div>
              <ProblemBanner v-if="auditError" :error="auditError" class="q-mt-xs" />
              <q-list v-if="audit[capabilityName]?.length" dense separator class="q-mt-xs">
                <q-item v-for="item in audit[capabilityName]" :key="item.auditId">
                  <q-item-section>
                    <q-item-label>{{ $t(`budgets.mode.${item.mode}`) }} · {{ $t('budgets.revision', { revision: item.revision }) }}</q-item-label>
                    <q-item-label caption>{{ new Date(item.createdAt).toLocaleString() }} · {{ item.changedBy }}</q-item-label>
                    <q-item-label caption>{{ item.reason }}</q-item-label>
                  </q-item-section>
                </q-item>
              </q-list>
              <div v-else-if="audit[capabilityName] && auditLoading !== capabilityName" class="text-caption text-grey-7 q-mt-xs">{{ $t('budgets.noAudit') }}</div>
              <q-btn v-if="auditCursor[capabilityName]" flat dense no-caps :label="$t('common.loadMore')" @click.prevent="loadAudit(capabilityName, true)" />
            </details>
          </q-card-section>
        </template>
      </q-card>
    </div>
  </section>
</template>

<style scoped>
.budget-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.budget-card {
  min-width: 0;
}

.budget-mode-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.budget-rule {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) minmax(0, 1fr) auto;
  gap: 4px;
  align-items: start;
}

.budget-limit + .budget-limit {
  border-top: 1px solid rgba(0, 0, 0, 0.12);
  padding-top: 8px;
}

@media (max-width: 700px) {
  .budget-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .budget-rule {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
