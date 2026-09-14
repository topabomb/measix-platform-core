import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer, QCard, QCardSection, QCardActions,
  QBtn, QBanner, QList, QItem, QItemSection, QItemLabel, QChip, QSpinner,
  QIcon, QToolbarTitle, QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QDialog,
  QSeparator, QMarkupTable, QTimeline, QTimelineEntry, ClosePopup,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { h } from 'vue'
import ReleasesPage from './ReleasesPage.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusChip from '../components/StatusChip.vue'
import { useSessionStore } from '../stores/session'
import * as client from '../api/client'

function mountReleases() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const session = useSessionStore(pinia)
  session.session = {
    user: { userId: 'usr_001', displayName: 'Admin', role: 'ADMIN' as const },
    csrfToken: 'test-csrf',
    expiresAt: '2026-12-31T23:59:59Z',
  }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div/>' } }],
  })
  const wrapper = mount(
    {
      components: { ReleasesPage },
      render() {
        return h(QLayout, {}, () => [h(QPageContainer, {}, () => [h(ReleasesPage)])])
      },
    },
    {
      global: {
        plugins: [[Quasar, {
          components: {
            QLayout, QPage, QPageContainer, QCard, QCardSection, QCardActions,
            QBtn, QBanner, QList, QItem, QItemSection, QItemLabel, QChip, QSpinner,
            QIcon, QToolbarTitle, QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QDialog,
            QSeparator, QMarkupTable, QTimeline, QTimelineEntry, PageHeader, StatusChip,
          },
          directives: { ClosePopup },
        }], pinia, router],
      },
    },
  )
  return { wrapper, pinia }
}

const RELEASE = {
  releaseId: 'rls_001',
  managedGeneration: 3,
  sourceDraftRevision: 5,
  status: 'ACTIVE' as const,
  snapshotHash: 'sha256:abc',
  publishedAt: '2026-08-01T00:00:00Z',
  publishedBy: 'admin',
  diffSummary: { added: 2, changed: 1, removed: 0, details: [{ kind: 'Model', added: 2, changed: 1, removed: 0 }] },
  activationHistory: [{ activationId: 'act_1', kind: 'PUBLISH', state: 'COMPLETED', desiredControlRevision: 5, createdAt: '2026-08-01T00:00:00Z' }],
}

describe('ReleasesPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith('/api/admin/v1/releases')) return { items: [structuredClone(RELEASE)], nextCursor: undefined }
      return {}
    })
  })

  it('lists immutable staged releases with generation and diff summary', async () => {
    const { wrapper } = mountReleases()
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('Generation 3')
    expect(text).toContain('+2')
    expect(text).toContain('2026-08-01T00:00:00Z')
    expect(text).toContain('admin')
  })

  it('opens a release detail dialog with snapshot hash and activation history', async () => {
    const { wrapper } = mountReleases()
    await flushPromises()
    await wrapper.findComponent(QItem).trigger('click')
    await flushPromises()
    const body = document.body.innerHTML
    expect(body).toContain('sha256:abc')
    expect(body).toContain('act_1')
    expect(body.toLowerCase()).toContain('completed')
  })

  it('republishes with an Idempotency-Key and surfaces the activation', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith('/api/admin/v1/releases') && path.includes(':republish')) {
        return { activationId: 'act_rp', kind: 'PUBLISH', state: 'COMPLETED', desiredControlRevision: 6 }
      }
      if (path.startsWith('/api/admin/v1/releases')) return { items: [structuredClone(RELEASE)], nextCursor: undefined }
      return {}
    })
    window.confirm = vi.fn(() => true)
    const { wrapper } = mountReleases()
    await flushPromises()

    const republishBtn = wrapper.findAllComponents(QBtn).find((b) => String(b.props('label') ?? '') === 'Republish')
    expect(republishBtn).toBeTruthy()
    await republishBtn!.trigger('click')
    await flushPromises()

    const rpCall = fetchSpy.mock.calls.find((c) => c[0].includes(':republish'))
    expect(rpCall).toBeTruthy()
    expect((rpCall![1] as RequestInit).headers).toHaveProperty('Idempotency-Key')
    expect(wrapper.text()).toContain('act_rp')
  })
})
