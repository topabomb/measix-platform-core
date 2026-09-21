import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer, QCard, QCardSection, QBtn, QList,
  QItem, QItemSection, QItemLabel, QChip, QSpinner, QIcon, QToolbarTitle,
  QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBadge, QMarkupTable, QBanner,
  QTabs, QTab, QBtnToggle, QSelect, QToggle, QVirtualScroll,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { h } from 'vue'
import SystemPage from './SystemPage.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusChip from '../components/StatusChip.vue'
import * as client from '../api/client'

function mountSystem() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div/>' } }],
  })
  const wrapper = mount(
    {
      components: { SystemPage },
      render() {
        return h(QLayout, {}, () => [h(QPageContainer, {}, () => [h(SystemPage)])])
      },
    },
    {
      global: {
        plugins: [[Quasar, {
          components: {
            QLayout, QPage, QPageContainer, QCard, QCardSection, QBtn, QList,
            QItem, QItemSection, QItemLabel, QChip, QSpinner, QIcon, QToolbarTitle,
            QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBadge, QMarkupTable, QBanner,
            QTabs, QTab, QBtnToggle, QSelect, QToggle, QVirtualScroll,
            PageHeader, StatusChip,
          },
        }], pinia, router],
      },
    },
  )
  return { wrapper }
}

// Contract-valid values: dbHealth is OK/DEGRADED only, and bundle hashes are
// sha256 with 64 hex characters. Fake hashes get mistaken for live captures.
const BUNDLE_HASH = `sha256:${'a'.repeat(64)}`
const STALE_HASH = `sha256:${'b'.repeat(64)}`

const BASE = {
  buildVersion: 'v0.1.0',
  dbHealth: 'OK',
  schemaIdentity: 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
  runtimeStatus: 'READY',
  activeManagedGeneration: 2,
  managedStateRevision: 2,
  desiredControlRevision: 5,
  desiredBundleHash: BUNDLE_HASH,
  appliedBundleHash: BUNDLE_HASH,
  appliedControlRevision: 5,
  relayReady: true,
  portalMode: 'STANDARD' as const,
  portalUrl: 'http://192.168.1.20:9000/portal/',
}

describe('SystemPage', () => {
  it('shows the configured HTTP client address, independent of the browser origin', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, publicOrigin: 'http://192.168.1.20:9000' }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      return { items: [] }
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(wrapper.find('[data-cy="platform-public-origin"]').text()).toContain('http://192.168.1.20:9000')
    wrapper.unmount()
  })
  it('shows the effective standard or custom Portal selected by Core deployment', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return {
        ...BASE,
        portalMode: 'CUSTOM',
        portalUpstreamUrl: 'https://portal.enterprise.example/workbench/',
      }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      return { items: [] }
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    const portal = wrapper.get('[data-cy="portal-status"]')
    expect(portal.text()).toContain('Enterprise workbench')
    expect(portal.text()).toContain('https://portal.enterprise.example/workbench/')
    expect(portal.get('a').attributes('href')).toBe(BASE.portalUrl)
    wrapper.unmount()
  })
  it('keeps the last failure visible while a different operation awaits confirmation', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return {
        ...BASE,
        currentActivation: { activationId: 'act_pending', kind: 'PUBLISH', state: 'UNKNOWN', desiredControlRevision: 6, updatedAt: '2026-09-18T12:00:00Z' },
        lastActivation: { activationId: 'act_failed', kind: 'RUNTIME_CONFIG', state: 'FAILED', desiredControlRevision: 5, updatedAt: '2026-09-18T11:00:00Z' },
      }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      return { items: [] }
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(wrapper.get('[data-cy="currentActivation"]').text()).toContain('Publish enterprise configuration')
    expect(wrapper.get('[data-cy="lastActivation"]').text()).toContain('Apply upstream configuration')
    expect(wrapper.get('[data-cy="lastActivation"]').text()).toContain('Failed')
    expect(wrapper.get('[data-cy="currentActivation"] details').attributes('open')).toBeUndefined()
    wrapper.unmount()
  })
  it('shows the Relay build separately from the Hub build and leaves unavailable data unknown', async () => {
    const fetch = vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, relayBuildVersion: 'relay-build-123' }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      return { items: [] }
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(wrapper.find('[data-cy="relay-build-version"]').text()).toBe('relay-build-123')
    wrapper.unmount()
    fetch.mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return BASE
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      return { items: [] }
    })
    const unavailable = mountSystem().wrapper
    await flushPromises()
    expect(unavailable.find('[data-cy="relay-build-version"]').text()).toBe('—')
    unavailable.unmount()
  })
  it('distinguishes unavailable unknown-metering count from an observed zero', async () => {
    const fetch = vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, semanticUnknownRequestCount: 0 }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      return { items: [] }
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(wrapper.find('[data-cy="semantic-unknown-count"]').text()).toBe('0')
    wrapper.unmount()
    fetch.mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return BASE
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      return { items: [] }
    })
    const unavailable = mountSystem().wrapper
    await flushPromises()
    expect(unavailable.find('[data-cy="semantic-unknown-count"]').text()).toBe('—')
    unavailable.unmount()
  })
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, appliedControlRevision: 5, appliedBundleHash: BUNDLE_HASH }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      return {}
    })
  })

  it('shows Relay applied control revision and bundle hash', async () => {
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(wrapper.text()).toContain('5')
    expect(wrapper.text()).toContain(BUNDLE_HASH)
  })

  it('reports convergence when the applied revision and bundle hash match the published release', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, appliedControlRevision: 5, appliedBundleHash: BUNDLE_HASH }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      return {}
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(wrapper.find('[data-cy="system-convergence-status"]').text()).toMatch(/^(converged|已收敛)$/i)
  })

  it('localizes the Relay not-ready badge instead of rendering a raw i18n key', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, relayReady: false }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      return {}
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(wrapper.get('[data-cy="system-relay-status"]').text()).toBe('Not ready')
    expect(wrapper.text()).not.toContain('status.NOT_READY')
  })

  it('flags control-not-converged when applied revision differs from desired', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, appliedControlRevision: 4, appliedBundleHash: STALE_HASH }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      return {}
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(/not converged/i.test(wrapper.text())).toBe(true)
  })

  it('does not call an unpublished and unready runtime converged', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return {
        ...BASE, runtimeStatus: 'DEGRADED', activeManagedGeneration: 0,
        desiredControlRevision: 0, relayReady: false, appliedControlRevision: 0,
      }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      return {}
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(wrapper.find('[data-cy="system-setup-state"]').exists()).toBe(true)
    expect(wrapper.find('[data-cy="system-convergence-status"]').text()).not.toMatch(/^(converged|已收敛)$/i)
  })

  it('keeps the authenticated status visible when the separate health probe fails', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, appliedControlRevision: 5, appliedBundleHash: BUNDLE_HASH }
      if (path === '/api/admin/v1/system/health') throw new Error('health probe unavailable')
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      return {}
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(wrapper.find('[data-cy="system-runtime-status"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('health probe unavailable')
  })

  it('does not enumerate the upstream dataset from the system overview', async () => {
    const fetch = vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, appliedControlRevision: 5, appliedBundleHash: BUNDLE_HASH }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      return {}
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(wrapper.find('[data-cy="system-upstream-status"]').exists()).toBe(false)
    expect(fetch.mock.calls.some(([path]) => String(path).startsWith('/api/admin/v1/upstreams'))).toBe(false)
  })

  it('loads bounded telemetry and recent events only for their visible tabs', async () => {
    const fetch = vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return BASE
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      if (path === '/api/admin/v1/system/telemetry?window=15m') return {
        windowMinutes: 15, collectedAt: '2026-09-21T10:00:00Z',
        hub: { startedAt: '2026-09-21T09:45:00Z', summary: { requestCount: 4, successCount: 3, clientErrorCount: 1, serverErrorCount: 0, rejectedCount: 0, timeoutCount: 0, cancelledCount: 0, durationP95Ms: 25, inFlight: 0 }, buckets: [] },
      }
      if (path.startsWith('/api/admin/v1/system/events?')) return { items: [{ time: '2026-09-21T10:00:00Z', service: 'HUB', level: 'WARN', event: 'sample.failed', message: 'safe' }], truncated: false }
      return {}
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(fetch.mock.calls.some(([path]) => String(path).includes('/system/telemetry'))).toBe(false)
    const tabs = wrapper.findAllComponents(QTab)
    await tabs[2]!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-cy="system-telemetry"]').text()).toContain('4')
    await tabs[3]!.trigger('click')
    await flushPromises()
    expect(fetch.mock.calls.some(([path]) => String(path).startsWith('/api/admin/v1/system/events?'))).toBe(true)
    expect(wrapper.findComponent(QVirtualScroll).exists()).toBe(true)
    wrapper.unmount()
  })

  it('refreshes basic status while visible instead of freezing the first observation', async () => {
    vi.useFakeTimers()
    try {
      let observations = 0
      const fetch = vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
        if (path === '/api/admin/v1/system/status') return { ...BASE, spoolPendingCount: ++observations }
        if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
        return {}
      })
      const { wrapper } = mountSystem()
      await flushPromises()
      const initial = fetch.mock.calls.filter(([path]) => path === '/api/admin/v1/system/status').length
      await vi.advanceTimersByTimeAsync(15_000)
      await flushPromises()
      expect(fetch.mock.calls.filter(([path]) => path === '/api/admin/v1/system/status').length).toBe(initial + 1)
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not overlap slow background polls', async () => {
    vi.useFakeTimers()
    try {
      let statusCalls = 0
      let releaseSlowPoll: (() => void) | undefined
      const slowPoll = new Promise<void>(resolve => { releaseSlowPoll = resolve })
      const fetch = vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
        if (path === '/api/admin/v1/system/status') {
          statusCalls += 1
          if (statusCalls === 2) await slowPoll
          return BASE
        }
        if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
        return {}
      })
      const { wrapper } = mountSystem()
      await flushPromises()
      await vi.advanceTimersByTimeAsync(15_000)
      expect(statusCalls).toBe(2)
      await vi.advanceTimersByTimeAsync(15_000)
      expect(statusCalls).toBe(2)
      releaseSlowPoll?.()
      await flushPromises()
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })
})
