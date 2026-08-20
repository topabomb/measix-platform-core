import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer,
  QCard, QCardSection, QInput, QBtn, QBanner, QSelect, QList, QItem,
  QItemSection, QItemLabel, QChip, QSpinner,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { h } from 'vue'
import UsagePage from './UsagePage.vue'
import * as client from '../api/client'

function mountUsagePage() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div/>' } }],
  })
  const wrapper = mount(
    {
      components: { UsagePage },
      render() {
        return h(QLayout, {}, () => [
          h(QPageContainer, {}, () => [h(UsagePage)]),
        ])
      },
    },
    {
      global: {
        plugins: [[Quasar, {
          components: {
            QLayout, QPage, QPageContainer, QCard, QCardSection, QInput,
            QBtn, QBanner, QSelect, QList, QItem, QItemSection, QItemLabel,
            QChip, QSpinner,
          },
        }], pinia, router],
      },
    },
  )
  return { wrapper }
}

describe('UsagePage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z',
          to: '2026-08-20T00:00:00Z',
          requestCount: 12,
          forwardedRequestCount: 10,
          requestBytes: 2048,
          responseBytes: 4096,
          semanticMeters: [{ confidence: 'EXACT', meter: 'prompt_tokens', quantity: '100' }],
          cost: { status: 'KNOWN', amount: '0.0420', currency: 'USD' },
        }
      }
      return { items: [], nextCursor: undefined }
    })
  })

  it('renders cost amount and currency, not the raw object', async () => {
    const { wrapper } = mountUsagePage()
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('0.0420 USD')
    expect(text).not.toContain('[object Object]')
  })

  it('renders unknown cost status without amount when unknown', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z',
          to: '2026-08-20T00:00:00Z',
          requestCount: 0,
          forwardedRequestCount: 0,
          requestBytes: 0,
          responseBytes: 0,
          semanticMeters: [],
          cost: { status: 'UNKNOWN' },
        }
      }
      return { items: [] }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    expect(wrapper.text()).toContain('unknown')
  })
})
