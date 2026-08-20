import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer, QCard, QCardSection, QBtn, QList,
  QItem, QItemSection, QItemLabel, QChip, QSpinner, QIcon, QToolbarTitle,
  QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBadge, QMarkupTable,
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
            QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBadge, QMarkupTable,
            PageHeader, StatusChip,
          },
        }], pinia, router],
      },
    },
  )
  return { wrapper }
}

const BASE = {
  buildVersion: 'v0.1.0',
  dbHealth: 'HEALTHY',
  migrationRevision: '1',
  runtimeStatus: 'READY',
  activeManagedGeneration: 2,
  managedStateRevision: 2,
  desiredControlRevision: 5,
  relayReady: true,
}

describe('SystemPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, appliedControlRevision: 5, appliedBundleHash: 'sha256:abc123' }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      return {}
    })
  })

  it('shows Relay applied control revision and bundle hash', async () => {
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(wrapper.text()).toContain('applied 5')
    expect(wrapper.text()).toContain('abc123')
  })

  it('flags control-not-converged when applied revision differs from desired', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/system/status') return { ...BASE, appliedControlRevision: 4, appliedBundleHash: 'sha256:stale' }
      if (path === '/api/admin/v1/system/health') return { live: true, ready: true }
      return {}
    })
    const { wrapper } = mountSystem()
    await flushPromises()
    expect(/control not converged/i.test(wrapper.text())).toBe(true)
  })
})
