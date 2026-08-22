import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type { components } from '../api/generated'

type Upstream = components['schemas']['Upstream']

export const useOperationalApplyStore = defineStore('operationalApply', () => {
  const upstream = ref<Upstream>()
  const candidateRevision = computed(() => upstream.value?.configRevision)
  const activeRevision = computed(() => upstream.value?.activeConfigRevision)
  const pending = computed(() => upstream.value?.status === 'APPLYING')
  const degraded = computed(() => upstream.value?.status === 'DEGRADED')

  function observe(value: Upstream) {
    upstream.value = value
  }

  function clear() {
    upstream.value = undefined
  }

  return { upstream, candidateRevision, activeRevision, pending, degraded, observe, clear }
})
