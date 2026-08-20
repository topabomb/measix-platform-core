import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer, QCard, QCardSection, QBtn, QList,
  QItem, QItemSection, QItemLabel, QChip, QSpinner, QIcon, QToolbarTitle,
  QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBanner,
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
    routes: [{ path: '/', component: { template: '<div/>' } }],
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
            QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBanner, PageHeader, StatusChip,
          },
        }], pinia, router],
      },
    },
  )
  return { wrapper }
}

const STATUS = {
  buildVersion: 'v0.1.0',
  dbHealth: 'HEALTHY',
  migrationRevision: '1',
  runtimeStatus: 'READY',
  activeManagedGeneration: 3,
  managedStateRevision: 2,
  desiredControlRevision: 7,
  desiredBundleHash: 'sha256:desired',
  relayReady: true,
  appliedControlRevision: 7,
  appliedBundleHash: 'sha256:applied',
  lastRelaySeenAt: '2026-08-20T00:00:00Z',
  latestActivation: { activationId: 'act_11111111-1111-4111-8111-111111111111', kind: 'PUBLISH', state: 'SUCCEEDED', desiredControlRevision: 7, createdAt: '2026-08-20T00:00:00Z' },
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
      return { from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z', requestCount: 5, forwardedRequestCount: 5, requestBytes: 0, responseBytes: 0, semanticMeters: [], cost: { status: 'UNKNOWN' } }
    })
  })

  it('shows the last Activation state on the Overview', async () => {
    const { wrapper } = mountOverview()
    await flushPromises()
    expect(wrapper.text()).toContain('act_11111111-1111-4111-8111-111111111111')
    expect(wrapper.text()).toContain('SUCCEEDED')
  })

  it('shows a degraded warning when bundle hashes or revisions do not converge', async () => {
    const diverged = structuredClone(STATUS) as typeof STATUS
    diverged.appliedControlRevision = 6
    diverged.appliedBundleHash = 'sha256:stale'
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return diverged
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      return { from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z', requestCount: 0, forwardedRequestCount: 0, requestBytes: 0, responseBytes: 0, semanticMeters: [], cost: { status: 'UNKNOWN' } }
    })
    const { wrapper } = mountOverview()
    await flushPromises()
    expect(/control not converged/i.test(wrapper.text())).toBe(true)
  })

  it('summarises upstream active / degraded / disabled counts', async () => {
    const { wrapper } = mountOverview()
    await flushPromises()
    expect(wrapper.text()).toContain('Upstreams')
    expect(wrapper.text()).toContain('1 active')
    expect(wrapper.text()).toContain('1 degraded')
    expect(wrapper.text()).toContain('1 disabled')
  })
})
