<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch, createCandidateId } from '../api/client'
import { costAmounts } from '../api/cost'
import { localPricingTime, normalizePricingRule, pricingResources, pricingRuleError, pricingScope, pricingTime, resourcePricingMeters, type PricingResource, type PricingRule, type PricingScope } from '../api/pricing'
import PagedEntityPicker from '../components/PagedEntityPicker.vue'
import type { EntityPickerPage } from '../components/PagedEntityPicker.vue'
import { fetchUpstreamPickerPage, resolveUpstreamPickerOption } from '../api/entityPickerSources'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import { useSessionStore } from '../stores/session'

const { t: $t, locale } = useI18n()
type PricingSet = components['schemas']['PricingSet']
const session = useSessionStore()
const loading = ref(false)
const saving = ref(false)
const error = ref<unknown>()
const editError = ref<unknown>()
const resourceError = ref<unknown>()
const revision = ref<number>()
const rules = ref<PricingRule[]>([])
const resources = ref<PricingResource[]>([])
const upstreamNames = ref<Record<string, string>>({})
const usageCost = ref<components['schemas']['UsageSummary']['cost']>()
const editorOpen = ref(false)
const editingExisting = ref(false)
const editing = ref<PricingRule>()
const removing = ref<PricingRule>()
const scope = ref<PricingScope>('GLOBAL')
const fromLocal = ref('')
const toLocal = ref('')
const scopes = computed(() => ['GLOBAL', 'UPSTREAM', 'RESOURCE', 'RESOURCE_UPSTREAM'].map(value => ({ value, label: $t('pricing.scopes.' + value) })))
function meterLabel(meter: PricingRule['meter']) { return $t('usage.meters.' + meter) + (meter === 'AUDIO_SECONDS' ? ` (${$t('usage.units.seconds')})` : '') }
function defaultUnit(meter: PricingRule['meter']) { return meter.includes('TOKEN') ? '1000000' : meter === 'CHARACTERS' ? '1000' : '1' }
const meterOptions = computed(() => resourcePricingMeters(scope.value.includes('RESOURCE') ? editing.value?.resourceId : undefined).map(value => ({ value, label: meterLabel(value) })))
const candidate = computed(() => editing.value ? normalizePricingRule({
  ...editing.value,
  effectiveFrom: fromLocal.value === localPricingTime(editing.value.effectiveFrom) ? editing.value.effectiveFrom : pricingTime(fromLocal.value) ?? '',
  effectiveTo: toLocal.value === localPricingTime(editing.value.effectiveTo) ? editing.value.effectiveTo : toLocal.value ? pricingTime(toLocal.value) ?? 'invalid' : undefined,
}, scope.value) : undefined)
const validation = computed(() => candidate.value ? pricingRuleError(candidate.value, scope.value, rules.value) : undefined)
const changed = computed(() => candidate.value && JSON.stringify(candidate.value) !== JSON.stringify(rules.value.find(rule => rule.pricingRuleId === candidate.value?.pricingRuleId)))
const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    const set = await apiFetch<PricingSet>('/api/admin/v1/pricing')
    revision.value = set.pricingRevision
    rules.value = set.rules ?? []
    await refreshEstimate()
    const ids = [...new Set(rules.value.flatMap(rule => rule.upstreamId ? [rule.upstreamId] : []))]
    await Promise.all(ids.map(async id => {
      try { upstreamNames.value[id] = (await resolveUpstreamPickerOption(id))?.label ?? id } catch { upstreamNames.value[id] = id }
    }))
    return true
  } catch (cause) { error.value = cause; return false } finally { loading.value = false }
}

async function refreshEstimate() {
  try { usageCost.value = (await apiFetch<components['schemas']['UsageSummary']>('/api/admin/v1/usage/summary')).cost }
  catch (cause) { usageCost.value = undefined; error.value = cause }
}

async function loadResources() {
  resourceError.value = undefined
  try {
    const draft = await apiFetch<components['schemas']['Draft']>('/api/admin/v1/draft')
    resources.value = pricingResources(draft.content)
  } catch (cause) { resourceError.value = cause }
}

async function fetchResourcePage(query: string, cursor?: string): Promise<EntityPickerPage> {
  if (resourceError.value) await loadResources()
  if (resourceError.value) throw resourceError.value
  const matching = resources.value.filter(item => [item.label, item.value, $t('usage.kind.' + item.kind)].some(value => value.toLowerCase().includes(query.toLowerCase())))
  const start = cursor ? Number(cursor) : 0
  return { items: matching.slice(start, start + 50).map(item => ({ value: item.value, label: item.label, caption: $t('usage.kind.' + item.kind) })), nextCursor: matching.length > start + 50 ? String(start + 50) : undefined }
}

function resourceName(id: string) { return resources.value.find(item => item.value === id)?.label ?? id }
function scopeName(rule: PricingRule) {
  if (rule.resourceId) return resourceName(rule.resourceId) + (rule.upstreamId ? ' · ' + (upstreamNames.value[rule.upstreamId] ?? rule.upstreamId) : '')
  return rule.upstreamId ? upstreamNames.value[rule.upstreamId] ?? rule.upstreamId : $t('pricing.scopes.GLOBAL')
}
function date(value: string) { return new Date(value).toLocaleString(locale.value) }
function priceLabel(rule: PricingRule) {
  const unit = /^\d+$/.test(rule.unitSize) ? new Intl.NumberFormat(locale.value).format(BigInt(rule.unitSize)) : rule.unitSize
  return $t('pricing.rate', { price: rule.unitPrice, currency: rule.currency, unit, meter: meterLabel(rule.meter) })
}
function ruleStatus(rule: PricingRule) {
  return Date.parse(rule.effectiveFrom) > Date.now() ? 'scheduled' : rule.effectiveTo && Date.parse(rule.effectiveTo) <= Date.now() ? 'expired' : 'active'
}
function openEditor(rule?: PricingRule) {
  editingExisting.value = Boolean(rule)
  editing.value = rule ? { ...rule } : { pricingRuleId: createCandidateId('prc'), meter: 'INPUT_TOKENS', unitSize: '1000000', unitPrice: '', currency: 'CNY', effectiveFrom: new Date().toISOString() }
  scope.value = pricingScope(editing.value)
  fromLocal.value = localPricingTime(editing.value.effectiveFrom)
  toLocal.value = localPricingTime(editing.value.effectiveTo)
  editError.value = undefined
  editorOpen.value = true
}

watch(() => [scope.value, editing.value?.resourceId], () => {
  if (editing.value && !meterOptions.value.some(option => option.value === editing.value?.meter)) editing.value.meter = meterOptions.value[0]!.value
})
watch(() => editing.value?.meter, (meter, previous) => {
  if (editing.value && meter && previous && !rules.value.some(rule => rule.pricingRuleId === editing.value?.pricingRuleId) && editing.value.unitSize === defaultUnit(previous)) editing.value.unitSize = defaultUnit(meter)
})

async function writeSet(nextRules: PricingRule[]): Promise<boolean> {
  if (revision.value === undefined || !session.csrfToken) return false
  saving.value = true
  editError.value = undefined
  try {
    const set = await apiFetch<PricingSet>('/api/admin/v1/pricing', { method: 'PUT', body: JSON.stringify({ expectedPricingRevision: revision.value, rules: nextRules }) }, session.csrfToken)
    revision.value = set.pricingRevision
    rules.value = set.rules ?? []
    await refreshEstimate()
    return true
  } catch (cause) { editError.value = cause; return false } finally { saving.value = false }
}
async function save() {
  if (!candidate.value || validation.value || !changed.value || saving.value || editError.value) return
  const next = rules.value.filter(rule => rule.pricingRuleId !== candidate.value?.pricingRuleId)
  next.push(candidate.value)
  if (await writeSet(next)) editorOpen.value = false
}
async function remove() {
  if (!removing.value || saving.value || editError.value) return
  if (await writeSet(rules.value.filter(rule => rule.pricingRuleId !== removing.value?.pricingRuleId))) removing.value = undefined
}
async function reloadAfterConflict() {
  if (!await refresh()) return
  editError.value = undefined
  // A refreshed revision must not silently authorize overwriting a stale form.
  if (editorOpen.value && editingExisting.value) {
    const current = rules.value.find(rule => rule.pricingRuleId === editing.value?.pricingRuleId)
    if (current) openEditor(current)
    else editorOpen.value = false
  }
  if (removing.value) removing.value = rules.value.find(rule => rule.pricingRuleId === removing.value?.pricingRuleId)
}

onMounted(async () => { await Promise.all([refresh(), loadResources()]) })
</script>

<template>
  <q-card flat bordered>
    <q-card-section class="row items-center justify-between q-gutter-sm">
      <div>
        <div class="text-subtitle2">{{ $t('pricing.title') }}</div>
        <div class="text-caption text-grey-7">{{ $t('pricing.subtitle') }} · {{ $t('pricing.revision') }} {{ revision ?? '—' }}</div>
      </div>
      <div class="row items-center q-gutter-xs">
        <q-btn flat dense icon="refresh" :aria-label="$t('common.refresh')" :loading="loading" :disable="saving || editorOpen || Boolean(removing)" @click="refresh" />
        <q-btn outline dense icon="add" :label="$t('pricing.addRule')" :disable="loading || revision === undefined" @click="openEditor()" data-cy="pricing-add-rule-btn" />
      </div>
    </q-card-section>
    <q-banner v-if="usageCost" class="bg-grey-2 card-inset rounded-borders">
      <div class="text-caption text-grey-7">{{ $t('pricing.currentCostStatus') }}</div>
      <div class="row items-center q-gutter-sm">
        <q-chip dense :color="usageCost.status === 'KNOWN' ? 'green' : usageCost.status === 'PARTIAL' ? 'amber' : 'grey'" text-color="white">{{ $t('usage.cost' + (usageCost.status === 'KNOWN' ? 'Known' : usageCost.status === 'PARTIAL' ? 'Partial' : 'Unknown')) }}</q-chip>
        <span class="text-h6" data-cy="pricing-cost-amount">{{ costAmounts(usageCost) }}</span>
      </div>
      <div class="text-caption text-grey-7">{{ $t('pricing.costScope') }}</div>
      <div v-if="usageCost.missingPricingRequests" class="text-caption">{{ $t('pricing.missingPricing', { count: usageCost.missingPricingRequests }) }}</div>
      <div v-if="usageCost.unknownMeterRequests" class="text-caption">{{ $t('pricing.unknownMeters', { count: usageCost.unknownMeterRequests }) }}</div>
      <div v-if="usageCost.status !== 'KNOWN'" class="text-caption">{{ $t('pricing.costUnknownHint') }}</div>
    </q-banner>
    <div class="card-inset text-caption text-grey-7 q-py-sm">{{ $t('pricing.matchingHint') }}</div>
    <ProblemBanner :error="error" class="card-inset" />
    <LoadingState v-if="loading" />
    <q-list v-else separator>
      <q-item v-for="rule in rules" :key="rule.pricingRuleId" data-cy="pricing-rule-row" class="pricing-row">
        <q-item-section>
          <q-item-label class="text-weight-medium text-break">{{ scopeName(rule) }}</q-item-label>
          <q-item-label caption>{{ $t('pricing.scopes.' + pricingScope(rule)) }} · {{ $t('pricing.states.' + ruleStatus(rule)) }}</q-item-label>
          <q-item-label class="q-mt-sm">{{ priceLabel(rule) }}</q-item-label>
          <q-item-label caption class="text-break">{{ date(rule.effectiveFrom) }} → {{ rule.effectiveTo ? date(rule.effectiveTo) : $t('pricing.noEnd') }}</q-item-label>
        </q-item-section>
        <q-item-section side class="pricing-actions">
          <q-btn flat dense icon="edit" :aria-label="$t('common.edit')" @click="openEditor(rule)" data-cy="pricing-edit-rule" />
          <q-btn flat dense color="negative" icon="delete" :aria-label="$t('common.delete')" @click="removing = rule; editError = undefined" data-cy="pricing-delete-rule" />
        </q-item-section>
      </q-item>
      <q-item v-if="!rules.length"><q-item-section class="text-grey-7">{{ $t('pricing.noRules') }}</q-item-section></q-item>
    </q-list>
  </q-card>

  <q-dialog v-model="editorOpen" persistent>
    <q-card v-if="editing" class="pricing-dialog" data-cy="pricing-editor">
      <q-card-section>
        <div class="text-h6">{{ rules.some(rule => rule.pricingRuleId === editing?.pricingRuleId) ? $t('pricing.editRule') : $t('pricing.addRule') }}</div>
        <div class="text-caption text-grey-7 q-mt-xs">{{ $t('pricing.historyWarning') }}</div>
      </q-card-section>
      <q-card-section class="pricing-form q-pt-none">
        <q-select v-model="scope" outlined :options="scopes" emit-value map-options :label="$t('pricing.scope')" :disable="saving" class="full-row" />
        <template v-if="scope.includes('RESOURCE')">
          <PagedEntityPicker v-model="editing.resourceId" :label="$t('pricing.resource')" :empty-label="$t('pricing.chooseResource')" :fetch-page="fetchResourcePage" :selected-option="resources.find(item => item.value === editing?.resourceId)" :disabled="saving" />
          <div class="text-caption text-grey-7">{{ $t('pricing.resourceSourceHint') }}<div v-if="editing.resourceId">{{ editing.resourceId }}</div></div>
          <ProblemBanner :error="resourceError" class="full-row" />
        </template>
        <PagedEntityPicker v-if="scope.includes('UPSTREAM')" v-model="editing.upstreamId" :label="$t('usage.filters.upstream')" :empty-label="$t('pricing.chooseUpstream')" :fetch-page="fetchUpstreamPickerPage" :resolve-option="resolveUpstreamPickerOption" :disabled="saving" @selected="option => { if (option) upstreamNames[option.value] = option.label }" />
        <q-select v-model="editing.meter" outlined :options="meterOptions" emit-value map-options :label="$t('pricing.meter')" :disable="saving" />
        <q-input v-model="editing.unitSize" outlined inputmode="decimal" :label="$t('pricing.unitSize')" :hint="$t('pricing.unitHint')" :disable="saving" />
        <q-input v-model="editing.unitPrice" outlined inputmode="decimal" :label="$t('pricing.unitPrice')" :disable="saving" data-cy="pricing-unit-price" />
        <q-select v-model="editing.currency" outlined :options="['CNY', 'USD', 'EUR', 'GBP', 'JPY', 'HKD']" use-input new-value-mode="add-unique" :label="$t('pricing.currency')" :disable="saving" />
        <div class="full-row text-weight-medium">{{ candidate ? priceLabel(candidate) : '' }}</div>
        <q-input v-model="fromLocal" outlined type="datetime-local" step="0.001" :label="$t('pricing.effectiveFrom')" :disable="saving" />
        <q-input v-model="toLocal" outlined type="datetime-local" step="0.001" :label="$t('pricing.effectiveTo')" :disable="saving" />
        <div class="full-row text-caption text-grey-7">{{ $t('pricing.localTimeHint', { timezone }) }}</div>
        <div v-if="editing.meter.includes('TOKEN')" class="full-row text-caption text-grey-7">{{ $t('pricing.tokenHint') }}</div>
        <div v-if="validation" class="full-row text-negative" role="alert">{{ $t('pricing.validation.' + validation) }}</div>
        <ProblemBanner :error="editError" class="full-row" />
        <q-btn v-if="editError" flat :label="$t('pricing.reloadRules')" :disable="saving || loading" @click="reloadAfterConflict" class="full-row" />
        <details class="full-row text-caption text-grey-7"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ editing.pricingRuleId }}</details>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat :label="$t('common.cancel')" :disable="saving" @click="editorOpen = false" />
        <q-btn color="primary" :label="$t('common.save')" :disable="!changed || Boolean(validation) || Boolean(editError) || revision === undefined || loading" :loading="saving" @click="save" data-cy="pricing-save-btn" />
      </q-card-actions>
    </q-card>
  </q-dialog>

  <q-dialog :model-value="Boolean(removing)" persistent>
    <q-card class="pricing-dialog">
      <q-card-section><div class="text-h6">{{ $t('pricing.removeRule') }}</div><div class="q-mt-sm">{{ removing ? scopeName(removing) + ' · ' + priceLabel(removing) : '' }}</div><div class="text-body2 q-mt-sm">{{ $t('pricing.historyWarning') }}</div></q-card-section>
      <ProblemBanner :error="editError" class="card-inset" />
      <q-btn v-if="editError" flat :label="$t('pricing.reloadRules')" :disable="saving || loading" @click="reloadAfterConflict" class="card-inset" />
      <q-card-actions align="right"><q-btn flat :label="$t('common.cancel')" :disable="saving" @click="removing = undefined" /><q-btn color="negative" :label="$t('common.delete')" :disable="Boolean(editError) || loading" :loading="saving" @click="remove" /></q-card-actions>
    </q-card>
  </q-dialog>
</template>

<style scoped>
.pricing-dialog { width:min(720px,calc(100vw - 24px)); max-width:720px; }
.pricing-form { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:16px; }
.full-row { grid-column:1 / -1; }
.pricing-actions { flex-direction:row; align-self:flex-start; padding-left:8px; }
.pricing-row { padding:16px; }
@media(max-width:599px) { .pricing-form { grid-template-columns:minmax(0,1fr); } .pricing-row { padding:12px; } }
</style>
