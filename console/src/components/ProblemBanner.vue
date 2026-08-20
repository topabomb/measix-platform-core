<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ApiProblem } from '../api/client'

const { t, te } = useI18n()
const props = defineProps<{ error?: unknown }>()
const visible = computed(() => props.error !== undefined && props.error !== null)
const title = computed(() => {
  if (props.error instanceof ApiProblem) {
    const key = `problem.${props.error.code}`
    return te(key) ? t(key) : props.error.code
  }
  return t('problem.default')
})
const detail = computed(() => props.error instanceof Error ? props.error.message : String(props.error ?? ''))
</script>

<template>
  <q-banner v-if="visible" class="bg-red-1 text-negative rounded-borders" dense>
    <div class="text-weight-medium">{{ title }}</div>
    <div class="text-body2">{{ detail }}</div>
  </q-banner>
</template>
