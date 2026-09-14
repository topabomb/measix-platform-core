import { computed, onUnmounted, readonly, ref, type Ref } from 'vue'
import { apiFetch } from '../api/client'
import type { components } from '../api/generated'

type SystemStatus = components['schemas']['SystemStatus']

export interface SystemHealthState {
  status: Readonly<Ref<SystemStatus | undefined>>
  loading: Readonly<Ref<boolean>>
  /** Global high-priority indicator: undefined when healthy/unknown. */
  degraded: Readonly<Ref<'degraded' | 'relay' | undefined>>
  refresh: () => Promise<void>
}

/**
 * Polls Hub runtime/degraded state for the Global Header health indicator
 * (product §4.1, implementation §3.1). A single module-level poller is shared
 * across all consumers so the header indicator and any page both react to the
 * same runtime state.
 */
const status = ref<SystemStatus>()
const loading = ref(false)
let timer: ReturnType<typeof setInterval> | undefined
let activeConsumers = 0

export function useSystemHealth(intervalMs = 15_000): SystemHealthState {
  async function refresh() {
    loading.value = true
    try {
      status.value = await apiFetch<SystemStatus>('/api/admin/v1/system/status')
    } catch {
      // Hub unavailable / session expired — keep last known status; the
      // indicator reflects it rather than surfacing raw fetch errors.
      status.value = undefined
    } finally {
      loading.value = false
    }
  }

  function start() {
    if (timer) return
    refresh()
    timer = setInterval(refresh, intervalMs)
  }
  function stop() {
    if (timer && activeConsumers <= 0) {
      clearInterval(timer)
      timer = undefined
    }
  }

  activeConsumers++
  start()
  onUnmounted(() => {
    activeConsumers--
    stop()
  })

  const degraded = computed<'degraded' | 'relay' | undefined>(() => {
    if (!status.value) return undefined
    if (status.value.runtimeStatus === 'DEGRADED') return 'degraded'
    if (status.value.relayReady === false) return 'relay'
    return undefined
  })

  return { status: readonly(status), loading: readonly(loading), degraded, refresh }
}
