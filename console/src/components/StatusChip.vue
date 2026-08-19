<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ value: string }>()
const tone = computed(() => {
  const value = props.value.toUpperCase()
  if (['READY', 'ACTIVE', 'COMPLETED', 'KNOWN', 'EXACT'].includes(value)) return 'positive'
  if (['APPLYING', 'ACTIVATING', 'STAGED', 'PARTIAL', 'UNKNOWN', 'INACTIVE'].includes(value)) return 'warning'
  if (['FAILED', 'DEGRADED', 'DISABLED', 'REVOKED', 'ACTIVATION_FAILED'].includes(value)) return 'negative'
  return 'grey-7'
})
</script>

<template>
  <q-chip :color="tone" text-color="white" dense square>{{ value }}</q-chip>
</template>
