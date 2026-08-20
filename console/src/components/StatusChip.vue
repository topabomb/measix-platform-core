<script setup lang="ts">
import { computed } from 'vue'

// Semantic status chip (implementation §6): maps the product status vocabulary
// to stable semantic tones — healthy / pending / degraded / failed / neutral.
// Status is always rendered with text (never color alone).

const props = defineProps<{ value: string }>()

type Tone = 'healthy' | 'pending' | 'degraded' | 'failed' | 'neutral'

const HEALTHY = ['READY', 'ACTIVE', 'COMPLETED', 'KNOWN', 'EXACT', 'SUCCESS', 'SUPERSEDED', 'LIVE', 'STAGED']
const PENDING = ['APPLYING', 'ACTIVATING', 'PENDING', 'STAGING', 'VALIDATING', 'PARTIAL', 'INACTIVE', 'ENROLLED']
const DEGRADED = ['DEGRADED', 'UNKNOWN', 'WARNING', 'ERROR']
const FAILED = ['FAILED', 'BLOCKED', 'REVOKED', 'DISABLED', 'ACTIVATION_FAILED', 'NOT_READY', 'EXPIRED']

const tone = computed<Tone>(() => {
  const value = props.value.toUpperCase()
  if (FAILED.includes(value)) return 'failed'
  if (DEGRADED.includes(value)) return 'degraded'
  if (HEALTHY.includes(value)) return 'healthy'
  if (PENDING.includes(value)) return 'pending'
  return 'neutral'
})

const classes: Record<Tone, string> = {
  healthy: 'bg-positive text-white',
  pending: 'bg-amber-6 text-white',
  degraded: 'bg-orange-7 text-white',
  failed: 'bg-negative text-white',
  neutral: 'bg-grey-7 text-white',
}
</script>

<template>
  <q-chip :class="classes[tone]" dense square>{{ value }}</q-chip>
</template>
