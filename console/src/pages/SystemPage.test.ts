import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer, QCard, QCardSection, QBtn, QList,
  QItem, QItemSection, QItemLabel, QChip, QSpinner, QIcon, QToolbarTitle,
  QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBadge, QMarkupTable, QBanner,
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

  it('summarizes upstream configuration state without presenting it as a live probe', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, appliedControlRevision: 5, appliedBundleHash: BUNDLE_HASH }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [
        { upstreamId: 'ups_a', name: 'Configured', status: 'ACTIVE' },
        { upstreamId: 'ups_b', name: 'Failed apply', status: 'DEGRADED' },
      ], nextCursor: undefined }
      return {}
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    const summary = wrapper.find('[data-cy="system-upstream-status"]')
    expect(summary.exists()).toBe(true)
    expect(summary.text()).toMatch(/active/i)
    expect(summary.text()).toMatch(/degraded/i)
    expect(summary.text()).toMatch(/not a live/i)
  })
})
