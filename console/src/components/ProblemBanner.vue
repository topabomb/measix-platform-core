<script setup lang="ts">
import { computed } from 'vue'
import { ApiProblem } from '../api/client'

const props = defineProps<{ error?: unknown }>()
const visible = computed(() => props.error !== undefined && props.error !== null)
const title = computed(() => props.error instanceof ApiProblem ? props.error.code : 'request_failed')
const detail = computed(() => props.error instanceof Error ? props.error.message : String(props.error ?? ''))
</script>

<template>
  <q-banner v-if="visible" class="bg-red-1 text-negative rounded-borders" dense>
    <div class="text-weight-medium">{{ title }}</div>
    <div class="text-body2">{{ detail }}</div>
  </q-banner>
</template>
