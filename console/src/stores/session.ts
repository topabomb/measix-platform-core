import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'

type AdminSession = components['schemas']['AdminSession']

export const useSessionStore = defineStore('session', () => {
  const session = ref<AdminSession>()
  const loading = ref(false)
  const authenticated = computed(() => session.value !== undefined)
  const csrfToken = computed(() => session.value?.csrfToken)
  const user = computed(() => session.value?.user)

  function clear() {
    session.value = undefined
  }

  async function restore() {
    loading.value = true
    try {
      session.value = await apiFetch<AdminSession>('/api/admin/v1/session')
      return session.value
    } finally {
      loading.value = false
    }
  }

  async function login(username: string, password: string) {
    loading.value = true
    try {
      session.value = await apiFetch<AdminSession>('/api/admin/v1/session/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      })
      return session.value
    } finally {
      loading.value = false
    }
  }

  async function logout() {
    if (!session.value) return
    try {
      await apiFetch<void>('/api/admin/v1/session', { method: 'DELETE' }, session.value.csrfToken)
    } finally {
      clear()
    }
  }

  return { session, loading, authenticated, csrfToken, user, clear, restore, login, logout }
})
