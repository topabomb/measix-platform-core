import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type { components } from '../api/generated'
import { apiFetch, createIdempotencyKey } from '../api/client'

type Activation = components['schemas']['Activation']
type ActivationKind = Activation['kind']

export const useActivationStore = defineStore('activation', () => {
  const activation = ref<Activation>()
  const retryKey = ref<string>()
  const commandKind = ref<ActivationKind>()
  const polling = ref(false)
  const lastPollAt = ref<string>()

  const pending = computed(() => activation.value?.state === 'APPLYING' || activation.value?.state === 'UNKNOWN')
  const succeeded = computed(() => activation.value?.state === 'COMPLETED')
  const failed = computed(() => activation.value?.state === 'FAILED')

  function beginCommand(kind: ActivationKind): string {
    if (commandKind.value !== kind || !retryKey.value) {
      commandKind.value = kind
      retryKey.value = createIdempotencyKey()
      activation.value = undefined
    }
    return retryKey.value
  }

  function accept(value: Activation) {
    activation.value = value
    commandKind.value = value.kind
  }

  async function poll(activationId = activation.value?.activationId) {
    if (!activationId) throw new Error('activation id is required')
    polling.value = true
    try {
      const value = await apiFetch<Activation>(`/api/admin/v1/activations/${encodeURIComponent(activationId)}`)
      accept(value)
      lastPollAt.value = new Date().toISOString()
      return value
    } finally {
      polling.value = false
    }
  }

  function resetCommand() {
    activation.value = undefined
    retryKey.value = undefined
    commandKind.value = undefined
    lastPollAt.value = undefined
  }

  return { activation, retryKey, commandKind, polling, lastPollAt, pending, succeeded, failed, beginCommand, accept, poll, resetCommand }
})
