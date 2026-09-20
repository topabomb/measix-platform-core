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
  BudgetTemplate,
  BudgetTemplateRule,
  PricingMeter,
  PutBudgetRequest,
  UserBudgetView,
} from '../api/usageBudget'
import { budgetRuleValueKey, capabilities } from '../api/usageBudget'
import { formatMeter, type MeterUnitLabels } from '../usageFormatting'
import { useSessionStore } from '../stores/session'
import LoadingState from './LoadingState.vue'
import ProblemBanner from './ProblemBanner.vue'
import StatusChip from './StatusChip.vue'
import BudgetRuleEditor from './BudgetRuleEditor.vue'
import PagedEntityPicker from './PagedEntityPicker.vue'
import { fetchBudgetTemplatePickerPage, resolveBudgetTemplatePickerOption } from '../api/entityPickerSources'

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
const selectedTemplateId = ref<string>()
const assignmentReason = ref('')
const assignmentConfirmOpen = ref(false)
const assignmentAction = ref<'assign' | 'unassign'>('assign')
const assignmentPreviewLoading = ref(false)
const selectedTemplatePreview = ref<BudgetTemplate>()
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
  images: $t('usage.units.images'),
}))

function capability(capabilityName: BudgetCapability): BudgetCapabilityView | undefined {
  return view.value?.items?.find(item => item.capability === capabilityName)
}

const assignmentChanges = computed(() => {
  if (!selectedTemplatePreview.value) return []
  return capabilities.flatMap(capabilityName => {
    const current = capability(capabilityName)
    if (!current || current.source === 'EXPLICIT') return []
    const after = selectedTemplatePreview.value!.rules.find(rule => rule.capability === capabilityName)
    if (after && budgetRuleValueKey(current) === budgetRuleValueKey(after)) return []
    if (!after && current.source === 'DEFAULT') return []
    return [{ capability: capabilityName, before: current, after }]
  })
})

function ruleValueSummary(rule: Pick<BudgetTemplateRule, 'mode' | 'limits'> | undefined): string {
  if (!rule) return $t('budgets.template.deploymentDefault')
  if (rule.mode === 'UNLIMITED') return $t('budgets.mode.UNLIMITED')
  return rule.limits.map(limit => `${$t(`budgets.period.${limit.period}`)} · ${$t(`usage.meters.${limit.meter}`)}: ${limit.limit}`).join('; ')
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
    selectedTemplateId.value = result.templateAssignment?.budgetTemplateId
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

async function assignTemplate() {
  if (!session.csrfToken || !selectedTemplateId.value || !assignmentReason.value.trim()) return
  saving.value = true
  error.value = undefined
  conflict.value = false
  try {
    view.value = await apiFetch<UserBudgetView>(
      `/api/admin/v1/users/${encodeURIComponent(props.userId)}/budget-template`,
      { method: 'PUT', body: JSON.stringify({
        budgetTemplateId: selectedTemplateId.value,
        expectedAssignmentRevision: view.value?.templateAssignment?.assignmentRevision ?? 0,
        reason: assignmentReason.value.trim(),
      }) },
      session.csrfToken,
    )
    selectedTemplateId.value = view.value.templateAssignment?.budgetTemplateId
    assignmentReason.value = ''
    assignmentConfirmOpen.value = false
  } catch (cause) {
    if (cause instanceof ApiProblem && cause.status === 409) {
      assignmentConfirmOpen.value = false
      conflict.value = true
      await load()
    } else {
      error.value = cause
    }
  } finally {
    saving.value = false
  }
}

async function unassignTemplate() {
  const assignment = view.value?.templateAssignment
  if (!session.csrfToken || !assignment || !assignmentReason.value.trim()) return
  saving.value = true
  error.value = undefined
  conflict.value = false
  try {
    const query = new URLSearchParams({ expectedAssignmentRevision: String(assignment.assignmentRevision), reason: assignmentReason.value.trim() })
    view.value = await apiFetch<UserBudgetView>(
      `/api/admin/v1/users/${encodeURIComponent(props.userId)}/budget-template?${query}`,
      { method: 'DELETE' },
      session.csrfToken,
    )
    selectedTemplateId.value = undefined
    assignmentReason.value = ''
    assignmentConfirmOpen.value = false
  } catch (cause) {
    if (cause instanceof ApiProblem && cause.status === 409) {
      assignmentConfirmOpen.value = false
      conflict.value = true
      await load()
    } else {
      error.value = cause
    }
  } finally {
    saving.value = false
  }
}

async function requestAssignment(action: 'assign' | 'unassign') {
  if (!assignmentReason.value.trim()) return
  if (action === 'assign' && !selectedTemplateId.value) return
  if (action === 'unassign' && !view.value?.templateAssignment) return
  assignmentAction.value = action
  if (action === 'assign') {
    const templateId = selectedTemplateId.value!
    assignmentPreviewLoading.value = true
    error.value = undefined
    try {
      const template = await apiFetch<BudgetTemplate>(`/api/admin/v1/budget-templates/${encodeURIComponent(templateId)}`)
      if (selectedTemplateId.value !== templateId) return
      selectedTemplatePreview.value = template
    } catch (cause) {
      error.value = cause
      return
    } finally {
      assignmentPreviewLoading.value = false
    }
  } else {
    selectedTemplatePreview.value = undefined
  }
  assignmentConfirmOpen.value = true
}

function confirmAssignment() {
  if (assignmentAction.value === 'unassign') void unassignTemplate()
  else void assignTemplate()
}

async function clearOverride(item: BudgetCapabilityView) {
  if (!session.csrfToken || item.source !== 'EXPLICIT') return
  saving.value = true
  error.value = undefined
  conflict.value = false
  try {
    const query = new URLSearchParams({ expectedRevision: String(item.revision), reason: $t('budgets.clearOverrideReason') })
    const updated = await apiFetch<BudgetCapabilityView>(
      `/api/admin/v1/users/${encodeURIComponent(props.userId)}/budgets/${item.capability}?${query}`,
      { method: 'DELETE' },
      session.csrfToken,
    )
    if (view.value) view.value.items = view.value.items.map(candidate => candidate.capability === updated.capability ? updated : candidate)
  } catch (cause) {
    if (cause instanceof ApiProblem && cause.status === 409) {
      conflict.value = true
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
  assignmentConfirmOpen.value = false
  audit.value = {}
  auditCursor.value = {}
  void load()
}, { immediate: true })
watch(selectedTemplateId, () => { selectedTemplatePreview.value = undefined })
</script>

<template>
  <section data-cy="user-budgets">
    <div class="row items-start justify-between q-mb-xs">
      <div>
        <div class="text-subtitle2">{{ $t('budgets.title') }}</div>
        <div v-if="view" class="text-caption text-grey-7">
          {{ $t('budgets.timezone', { timezone: view.timezone }) }} · {{ $t('budgets.asOf', { time: new Date(view.asOf).toLocaleString() }) }}
        </div>
        <div v-if="view" class="text-caption text-grey-7">{{ $t('budgets.sourceHint') }}</div>
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

    <q-card v-if="view" flat bordered class="q-mb-sm" data-cy="budget-template-assignment">
      <q-card-section class="q-pa-sm">
        <div class="text-subtitle2">{{ $t('budgets.template.title') }}</div>
        <div class="text-caption text-grey-7 q-mb-xs">
          {{ view.templateAssignment ? $t('budgets.template.assigned', { name: view.templateAssignment.name }) : $t('budgets.template.unassigned') }}
        </div>
        <div class="row q-col-gutter-xs items-start">
          <PagedEntityPicker v-model="selectedTemplateId" class="col-12 col-sm-4" :label="$t('budgets.template.select')" :empty-label="$t('budgets.template.unassignedShort')" :fetch-page="fetchBudgetTemplatePickerPage" :resolve-option="resolveBudgetTemplatePickerOption" />
          <q-input v-model="assignmentReason" outlined dense class="col-12 col-sm" :label="$t('budgets.reasonLabel')" />
          <div class="col-auto row q-gutter-xs">
            <q-btn color="primary" no-caps :label="$t('budgets.template.assign')" :disable="!selectedTemplateId || !assignmentReason.trim()" :loading="saving || assignmentPreviewLoading" @click="requestAssignment('assign')" />
            <q-btn v-if="view.templateAssignment" flat color="negative" no-caps :label="$t('budgets.template.unassign')" :disable="!assignmentReason.trim()" :loading="saving" @click="requestAssignment('unassign')" />
          </div>
        </div>
      </q-card-section>
    </q-card>

    <div v-if="view" class="budget-grid">
      <q-card v-for="capabilityName in capabilities" :key="capabilityName" flat bordered class="budget-card" :data-cy="`budget-${capabilityName}`">
        <template v-if="capability(capabilityName)">
          <q-card-section class="q-pa-sm">
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
              <div class="row no-wrap">
                <q-btn flat dense icon="edit" :aria-label="$t('budgets.edit')" :disable="!canMutate" @click="beginEdit(capability(capabilityName)!)" />
                <q-btn v-if="capability(capabilityName)!.source === 'EXPLICIT'" flat dense icon="undo" :aria-label="$t('budgets.clearOverride')" :data-cy="`clear-budget-override-${capabilityName}`" :disable="!canMutate" @click="clearOverride(capability(capabilityName)!)" />
              </div>
            </div>
            <div class="text-caption text-grey-7 q-mt-xs budget-meta">
              {{ $t('budgets.revision', { revision: capability(capabilityName)!.revision }) }} ·
              {{ $t('budgets.inFlight', { count: capability(capabilityName)!.inFlightRequests }) }}
            </div>
            <div v-if="capability(capabilityName)!.usageMeters.length" class="usage-history q-mt-xs" data-cy="budget-usage-history">
              <div class="text-caption text-grey-7">{{ $t('budgets.usageHistory') }} · {{ $t('budgets.usageHistoryHint') }}</div>
              <div class="row q-col-gutter-sm q-mt-xs">
                <div v-for="meter in capability(capabilityName)!.usageMeters" :key="meter.meter" class="col-auto">
                  <span class="text-weight-medium">{{ compactQuantity(meter.quantity, meter.meter) }}</span>
                  <span class="text-caption text-grey-7"> · {{ meterLabel(meter.meter) }}</span>
                </div>
              </div>
            </div>
          </q-card-section>

          <q-separator v-if="editing === capabilityName || capability(capabilityName)!.mode === 'LIMITED'" />
          <q-card-section v-if="editing === capabilityName" class="q-gutter-xs" data-cy="budget-editor">
            <BudgetRuleEditor :capability="capabilityName" :mode="editMode" :limits="editLimits" @update:mode="editMode = $event" @update:limits="editLimits = $event" />
            <q-input v-model="editReason" outlined dense maxlength="500" :label="$t('budgets.reasonLabel')" :hint="$t('budgets.reasonHint')" />
            <div class="row justify-end q-gutter-xs">
              <q-btn flat :label="$t('common.cancel')" @click="editing = undefined" />
              <q-btn color="primary" :label="$t('common.save')" :loading="saving" :disable="!editorValid" data-cy="save-budget" @click="save(capability(capabilityName)!)" />
            </div>
          </q-card-section>

          <q-card-section v-else-if="capability(capabilityName)!.mode === 'LIMITED'" class="q-pa-sm q-gutter-xs">
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

    <q-dialog v-model="assignmentConfirmOpen">
      <q-card style="min-width: min(420px, 90vw)">
        <q-card-section class="text-subtitle1">
          {{ assignmentAction === 'unassign'
            ? $t('budgets.template.confirmUnassignTitle')
            : $t(view?.templateAssignment ? 'budgets.template.confirmReplaceTitle' : 'budgets.template.confirmAssignTitle') }}
        </q-card-section>
        <q-card-section class="q-pt-none text-body2">
          <div>{{ assignmentAction === 'unassign' ? $t('budgets.template.unassignImpact') : $t('budgets.template.assignImpact') }}</div>
          <template v-if="assignmentAction === 'assign'">
            <div class="text-caption text-grey-7 q-mt-sm">{{ $t('budgets.template.effectiveChanges') }}</div>
            <div v-if="!assignmentChanges.length" class="text-caption q-mt-xs">{{ $t('budgets.template.noEffectiveChanges') }}</div>
            <div v-for="change in assignmentChanges" :key="change.capability" class="q-mt-xs" :data-cy="`budget-template-assignment-change-${change.capability}`">
              <div class="text-weight-medium">{{ $t(`usage.kind.${change.capability}`) }}</div>
              <div class="text-caption"><span class="text-grey-7">{{ $t('budgetTemplates.auditBefore') }}:</span> {{ ruleValueSummary(change.before) }}</div>
              <div class="text-caption"><span class="text-grey-7">{{ $t('budgetTemplates.auditAfter') }}:</span> {{ ruleValueSummary(change.after) }}</div>
            </div>
          </template>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps :label="$t('common.cancel')" @click="assignmentConfirmOpen = false" />
          <q-btn :color="assignmentAction === 'unassign' ? 'negative' : 'primary'" no-caps :label="assignmentAction === 'unassign' ? $t('budgets.template.unassign') : $t('budgets.template.assign')" :loading="saving" data-cy="confirm-budget-template-assignment" @click="confirmAssignment" />
        </q-card-actions>
      </q-card>
    </q-dialog>
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

.budget-meta {
  line-height: 1.25;
}


.budget-limit + .budget-limit {
  border-top: 1px solid rgba(0, 0, 0, 0.12);
  padding-top: 8px;
}

@media (max-width: 700px) {
  .budget-grid {
    grid-template-columns: minmax(0, 1fr);
  }

}
</style>
