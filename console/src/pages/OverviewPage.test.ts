import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer, QCard, QCardSection, QBtn, QList,
  QItem, QItemSection, QItemLabel, QChip, QSpinner, QIcon, QToolbarTitle,
  QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBanner, QBadge,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { h } from 'vue'
import OverviewPage from './OverviewPage.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusChip from '../components/StatusChip.vue'
import * as client from '../api/client'

function mountOverview() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: ['/', '/users', '/upstreams', '/resources'].map(path => ({ path, component: { template: '<div/>' } })),
  })
  const wrapper = mount(
    {
      components: { OverviewPage },
      render() {
        return h(QLayout, {}, () => [h(QPageContainer, {}, () => [h(OverviewPage)])])
      },
    },
    {
      global: {
        plugins: [[Quasar, {
          components: {
            QLayout, QPage, QPageContainer, QCard, QCardSection, QBtn, QList,
            QItem, QItemSection, QItemLabel, QChip, QSpinner, QIcon, QToolbarTitle,
            QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBanner, QBadge, PageHeader, StatusChip,
          },
        }], pinia, router],
      },
    },
  )
  return { wrapper }
}

// Contract-valid values: dbHealth is OK/DEGRADED only and bundle hashes are
// sha256 with 64 hex characters. The baseline is a converged runtime, so a test
// that injects divergence actually proves something.
const BUNDLE_HASH = `sha256:${'c'.repeat(64)}`
const STALE_HASH = `sha256:${'d'.repeat(64)}`

const STATUS = {
  buildVersion: 'v0.1.0',
  dbHealth: 'OK',
  schemaIdentity: 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
  runtimeStatus: 'READY',
  portalMode: 'STANDARD' as const,
  activeManagedGeneration: 3,
  managedStateRevision: 2,
  desiredControlRevision: 7,
  desiredBundleHash: BUNDLE_HASH,
  relayReady: true,
  appliedControlRevision: 7,
  appliedBundleHash: BUNDLE_HASH,
  lastRelaySeenAt: '2026-08-20T00:00:00Z',
  lastActivation: { activationId: 'act_11111111-1111-4111-8111-111111111111', kind: 'PUBLISH', state: 'COMPLETED', desiredControlRevision: 7, createdAt: '2026-08-20T00:00:00Z' },
  requestUsageIngestLagSeconds: 0,
  semanticOrphanCount: 0,
}

describe('OverviewPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return structuredClone(STATUS)
      if (path.startsWith('/api/admin/v1/upstreams')) {
        return {
          items: [
            { upstreamId: 'ups_a', name: 'OpenAI', status: 'ACTIVE' },
            { upstreamId: 'ups_b', name: 'Degraded provider', status: 'DEGRADED' },
            { upstreamId: 'ups_c', name: 'Disabled', status: 'DISABLED' },
          ],
          nextCursor: undefined,
        }
      }
      return { from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z', requestCount: 5, requestCompleteness: { exact: 0, partial: 0, unknown: 5 }, forwardedRequestCount: 5, requestBytes: 0, responseBytes: 0, semanticMeters: [], cost: { status: 'UNKNOWN' } }
    })
  })

  it('shows the last Activation state on the Overview', async () => {
    const { wrapper } = mountOverview()
    await flushPromises()
    expect(wrapper.text()).toContain('act_11111111-1111-4111-8111-111111111111')
    expect(wrapper.text()).toContain('Completed')
  })

  it('does not warn while the published release is applied', async () => {
    const { wrapper } = mountOverview()
    await flushPromises()
    expect(wrapper.text()).not.toContain('waiting for the runtime')
  })

  it('shows a degraded warning when bundle hashes or revisions do not converge', async () => {
    const diverged = structuredClone(STATUS) as typeof STATUS
    diverged.appliedControlRevision = 6
    diverged.appliedBundleHash = STALE_HASH
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return diverged
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      return { from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z', requestCount: 0, requestCompleteness: { exact: 0, partial: 0, unknown: 0 }, forwardedRequestCount: 0, requestBytes: 0, responseBytes: 0, semanticMeters: [], cost: { status: 'UNKNOWN' } }
    })
    const { wrapper } = mountOverview()
    await flushPromises()
    expect(wrapper.text()).toContain('waiting for the runtime')
  })

  it('leads with the Android delivery task and keeps database identity in diagnostics', async () => {
    const { wrapper } = mountOverview()
    await flushPromises()
    const guide = wrapper.get('[data-cy="overview-delivery-guide"]')
    expect(guide.text()).toContain('Configuration published')
    expect(guide.text()).toContain('Android')
    expect(guide.findAllComponents(QBtn).map((button: { props: (name: string) => unknown }) => button.props('to'))).toContain('/resources')
    const diagnostics = wrapper.get('[data-cy="overview-diagnostics"]')
    expect((diagnostics.element as HTMLDetailsElement).open).toBe(false)
    expect(diagnostics.text()).toContain('Current schema identity')
  })

  it('does not show an unpublished runtime as converged or unknown lag as zero', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return {
        ...STATUS, runtimeStatus: 'DEGRADED', activeManagedGeneration: 0,
        desiredControlRevision: 0, relayReady: false, appliedControlRevision: 0,
        desiredBundleHash: undefined, appliedBundleHash: undefined,
        requestUsageIngestLagSeconds: undefined,
      }
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      if (path === '/api/admin/v1/draft') return { draftRevision: 1, content: { providers: [], models: [], tts: [], asr: [], mcp: [], bindings: [], assistants: [], starters: [] } }
      return { from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z', requestCount: 0, requestCompleteness: { exact: 0, partial: 0, unknown: 0 }, forwardedRequestCount: 0, requestBytes: 0, responseBytes: 0, semanticMeters: [], cost: { status: 'UNKNOWN' } }
    })
    const { wrapper } = mountOverview()
    await flushPromises()
    expect(wrapper.find('[data-cy="overview-setup-state"]').exists()).toBe(true)
    expect(wrapper.find('[data-cy="overview-ingest-lag"]').text()).not.toContain('0s')
  })

  it('summarises upstream active / degraded / disabled counts', async () => {
    const { wrapper } = mountOverview()
    await flushPromises()
    expect(wrapper.text()).toContain('Upstreams')
    expect(wrapper.text()).toContain('1 active')
    expect(wrapper.text()).toContain('1 degraded')
    expect(wrapper.text()).toContain('1 disabled / inactive')
  })
})
