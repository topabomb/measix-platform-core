import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { apiFetch } from '../api/client'
import type { components } from '../api/generated'

export type WorkspaceService = components['schemas']['WorkspaceService']

// A view of the server-owned service configuration, shared by its three Admin
// entry points. This store never creates a workspace or publishes capabilities.
export const useRemoteWorkspaceStore = defineStore('remoteWorkspace', () => {
  const current = ref<WorkspaceService>()
  const loading = ref(false)
  const error = ref<unknown>()
  const enabled = computed(() => current.value?.enabled === true && current.value.state === 'ACTIVE')
  let sequence = 0

  async function load() {
    const request = ++sequence
    loading.value = true
    error.value = undefined
    try {
      const result = await apiFetch<components['schemas']['WorkspaceServiceList']>('/api/admin/v1/remote-workspace/services')
      if (request === sequence) current.value = result.items[0]
    } catch (cause) {
      if (request === sequence) {
        current.value = undefined
        error.value = cause
      }
    } finally {
      if (request === sequence) loading.value = false
    }
  }
  return { current, enabled, loading, error, load }
})
