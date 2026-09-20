<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { BudgetCapability, BudgetLimitDefinition, BudgetMode, PricingMeter } from '../api/usageBudget'
import { budgetPeriods, metersForCapability } from '../api/usageBudget'

const props = defineProps<{
  capability: BudgetCapability
  mode: BudgetMode
  limits: BudgetLimitDefinition[]
}>()
const emit = defineEmits<{
  'update:mode': [value: BudgetMode]
  'update:limits': [value: BudgetLimitDefinition[]]
}>()
const { t: $t } = useI18n()

const valid = computed(() => {
  if (props.mode === 'UNLIMITED') return true
  if (!props.limits.length) return false
  const keys = new Set<string>()
  return props.limits.every(limit => {
    const key = `${limit.period}:${limit.meter}`
    if (keys.has(key) || !/^(0|[1-9]\d*)$/.test(limit.limit)) return false
    keys.add(key)
    return true
  })
})
defineExpose({ valid })

function setMode(mode: BudgetMode) {
  emit('update:mode', mode)
  if (mode === 'UNLIMITED') emit('update:limits', [])
  else if (!props.limits.length) addLimit()
}

function updateLimit(index: number, field: 'period' | 'meter' | 'limit', value: unknown) {
  const next = props.limits.map(item => ({ ...item }))
  next[index] = { ...next[index]!, [field]: value }
  emit('update:limits', next)
}

function addLimit() {
  const used = new Set(props.limits.map(item => `${item.period}:${item.meter}`))
  for (const period of budgetPeriods) {
    const meter = metersForCapability(props.capability).find(candidate => !used.has(`${period}:${candidate}`))
    if (meter) {
      emit('update:limits', [...props.limits, { period, meter, limit: '' }])
      return
    }
  }
}

function removeLimit(index: number) {
  emit('update:limits', props.limits.filter((_, candidate) => candidate !== index))
}
</script>

<template>
  <div class="q-gutter-xs" data-cy="budget-rule-editor">
    <div class="row q-gutter-xs">
      <q-btn :outline="mode !== 'UNLIMITED'" :color="mode === 'UNLIMITED' ? 'primary' : undefined" no-caps :label="$t('budgets.mode.UNLIMITED')" @click="setMode('UNLIMITED')" />
      <q-btn :outline="mode !== 'LIMITED'" :color="mode === 'LIMITED' ? 'primary' : undefined" no-caps :label="$t('budgets.mode.LIMITED')" @click="setMode('LIMITED')" />
    </div>
    <template v-if="mode === 'LIMITED'">
      <div v-for="(limit, index) in limits" :key="index" class="budget-rule">
        <q-select :model-value="limit.period" outlined dense emit-value map-options :label="$t('budgets.periodLabel')" :options="budgetPeriods.map(value => ({ value, label: $t(`budgets.period.${value}`) }))" @update:model-value="value => updateLimit(index, 'period', value)" />
        <q-select :model-value="limit.meter" outlined dense emit-value map-options :label="$t('budgets.meterLabel')" :options="metersForCapability(capability).map(value => ({ value, label: $t(`usage.meters.${value}`) }))" @update:model-value="value => updateLimit(index, 'meter', value as PricingMeter)" />
        <q-input :model-value="limit.limit" outlined dense inputmode="numeric" :label="$t('budgets.limitLabel')" :error="Boolean(limit.limit) && !/^(0|[1-9]\d*)$/.test(limit.limit)" @update:model-value="value => updateLimit(index, 'limit', String(value ?? ''))" />
        <q-btn flat dense round icon="delete" color="negative" :aria-label="$t('common.remove')" @click="removeLimit(index)" />
      </div>
      <q-btn flat dense no-caps icon="add" :label="$t('budgets.addLimit')" @click="addLimit" />
    </template>
  </div>
</template>

<style scoped>
.budget-rule {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) minmax(0, 1fr) auto;
  gap: 4px;
  align-items: start;
}
@media (max-width: 700px) {
  .budget-rule { grid-template-columns: minmax(0, 1fr); }
}
</style>
