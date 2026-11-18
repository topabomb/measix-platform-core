<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'

type TimeoutPolicy = components['schemas']['TimeoutPolicy']
const props = defineProps<{
  binding?: components['schemas']['RuntimeBindingDefinition']
  upstreamDefaults?: TimeoutPolicy
  disabled?: boolean
}>()
const emit = defineEmits<{ change: [policy: TimeoutPolicy | undefined] }>()
const { t } = useI18n()
const policy = computed(() => props.binding?.timeoutPolicy)
const fields = ['connectMs', 'responseHeaderMs', 'idleMs', 'overallMs'] as const

function toggleOverride(enabled: boolean) {
  emit('change', enabled ? { ...(props.upstreamDefaults ?? { connectMs: 5000, responseHeaderMs: 300000, idleMs: 120000 }) } : undefined)
}
function update(key: typeof fields[number], value: string | number | null) {
  if (!policy.value) return
  const next = { ...policy.value }
  if (key === 'overallMs' && (value === null || value === '')) delete next.overallMs
  else next[key] = Number(value)
  emit('change', next)
}
function valid(value: string | number | null, optional: boolean) {
  return optional && (value === null || value === '' || value === undefined)
    || Number.isInteger(Number(value)) && Number(value) > 0 || t('routeTimeouts.positiveInteger')
}
</script>

<template>
  <details v-if="binding?.upstreamId && !binding.workspaceServiceId && binding.targetKind !== 'REMOTE_WORKSPACE'" class="q-mt-sm" data-cy="route-timeout-editor">
    <summary class="text-body2 cursor-pointer">
      {{ t('routeTimeouts.title') }} · {{ t(policy ? 'routeTimeouts.overridden' : 'routeTimeouts.inherited') }}
      <span v-if="policy" class="text-caption"> · {{ policy.connectMs }} / {{ policy.responseHeaderMs }} / {{ policy.idleMs }} ms</span>
    </summary>
    <div class="q-pt-xs">
      <q-toggle :model-value="!!policy" :disable="disabled" :label="t('routeTimeouts.override')" data-cy="route-timeout-override" @update:model-value="toggleOverride" />
      <div class="text-caption text-grey-7 q-mb-xs">{{ t('routeTimeouts.hint') }}</div>
      <div v-if="policy" class="row q-col-gutter-xs">
        <q-input v-for="key in fields" :key="key" :model-value="policy[key]" :disable="disabled" type="number" min="1" step="1" dense outlined
          :label="t(`routeTimeouts.${key}`)" :hint="key === 'overallMs' ? t('routeTimeouts.overallHint') : undefined"
          :rules="[value => valid(value, key === 'overallMs')]" :clearable="key === 'overallMs'" class="col-12 col-sm-6"
          :data-cy="`route-timeout-${key}`" @update:model-value="value => update(key, value)" />
      </div>
    </div>
  </details>
</template>
