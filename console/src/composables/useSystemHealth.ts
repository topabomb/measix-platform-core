import { computed, getCurrentInstance, onUnmounted, readonly, ref, type Ref } from 'vue'
import { apiFetch } from '../api/client'
import { hasPublishedConfiguration, isManagedRuntimeConverged } from '../api/systemStatus'
import type { components } from '../api/generated'

type SystemStatus = components['schemas']['SystemStatus']

export interface SystemHealthState {
  status: Readonly<Ref<SystemStatus | undefined>>
  loading: Readonly<Ref<boolean>>
  /**
   * Global high-priority indicator: undefined when healthy/unknown.
   * `unconfigured` means no release has been published yet, which is not a
   * Relay fault and must not be presented as one.
   */
  degraded: Readonly<Ref<'degraded' | 'relay' | 'unconfigured' | undefined>>
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
let inFlight: Promise<void> | undefined
let requestSeq = 0

export function useSystemHealth(intervalMs = 15_000): SystemHealthState {
  async function refresh() {
    // One request at a time: a slow poll must not stack or let an older
    // response overwrite a newer reading.
    if (inFlight) return inFlight
    const seq = ++requestSeq
    loading.value = true
    inFlight = (async () => {
      try {
        const next = await apiFetch<SystemStatus>('/api/admin/v1/system/status')
        if (seq === requestSeq) status.value = next
      } catch {
        // Hub unavailable / session expired. Drop the reading instead of
        // presenting a stale one as current; the indicator renders UNKNOWN.
        if (seq === requestSeq) status.value = undefined
      } finally {
        if (seq === requestSeq) loading.value = false
        inFlight = undefined
      }
    })()
    return inFlight
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

  // Only track consumers that can be unmounted; otherwise the count would grow
  // without bound and the shared poller could never be released.
  if (getCurrentInstance()) {
    activeConsumers++
    onUnmounted(() => {
      activeConsumers--
      stop()
    })
  }
  start()

  const degraded = computed<'degraded' | 'relay' | 'unconfigured' | undefined>(() => {
    const value = status.value
    if (!value) return undefined
    if (value.dbHealth !== 'OK' || value.runtimeStatus !== 'READY') return 'degraded'
    // Nothing published yet is a configuration state, not a Relay failure.
    if (!hasPublishedConfiguration(value)) return 'unconfigured'
    if (!isManagedRuntimeConverged(value)) return 'relay'
    return undefined
  })

  return { status: readonly(status), loading: readonly(loading), degraded, refresh }
}
