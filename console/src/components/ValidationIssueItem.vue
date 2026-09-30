<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'

type ValidationIssue = components['schemas']['ValidationIssue']

const props = defineProps<{
  issue: ValidationIssue
  target?: string
  clickable?: boolean
}>()
const emit = defineEmits<{ activate: [] }>()
const { t, te } = useI18n()

const message = computed(() => {
  const fieldKey = props.issue.field ? `resources.validation.issues.${props.issue.code}_${props.issue.field}` : ''
  if (fieldKey && te(fieldKey)) return t(fieldKey)
  const key = `resources.validation.issues.${props.issue.code}`
  return te(key) ? t(key) : t('resources.validation.unknownIssue')
})
const warning = computed(() => props.issue.severity === 'WARNING')
</script>

<template>
  <q-item :clickable="clickable" :data-validation-code="issue.code" @click="clickable && emit('activate')">
    <q-item-section avatar>
      <q-icon :name="warning ? 'warning' : 'error'" :color="warning ? 'amber-8' : 'negative'" />
    </q-item-section>
    <q-item-section>
      <q-item-label v-if="target" class="text-weight-medium">{{ target }}</q-item-label>
      <q-item-label :class="warning ? 'text-amber-9' : 'text-negative'">{{ message }}</q-item-label>
      <q-item-label caption>{{ issue.code }} · {{ issue.path }}</q-item-label>
    </q-item-section>
  </q-item>
</template>
