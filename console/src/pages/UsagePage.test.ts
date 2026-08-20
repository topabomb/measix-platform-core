import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer,
  QCard, QCardSection, QInput, QBtn, QBanner, QSelect, QList, QItem,
  QItemSection, QItemLabel, QChip, QSpinner, QIcon, QToolbarTitle,
  QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QTab, QTabs, QSeparator,
  QMenu, QDialog, QCardActions, QMarkupTable, ClosePopup,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { h } from 'vue'
import UsagePage from './UsagePage.vue'
import PageHeader from '../components/PageHeader.vue'
import PricingPanel from './PricingPanel.vue'
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
            QChip, QSpinner, QIcon, QToolbarTitle, QBreadcrumbs, QBreadcrumbsEl,
            QBtnDropdown, QTab, QTabs, QSeparator, QMenu, QDialog, QCardActions,
            QMarkupTable, PageHeader, PricingPanel,
          },
          directives: { ClosePopup },
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

  it('classifies each request row by resource kind (MODEL/TTS/ASR/MCP)', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 4, forwardedRequestCount: 4, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'KNOWN', amount: '0', currency: 'USD' },
        }
      }
      return {
        items: [
          { requestId: 'req_1', resourceId: 'mdl_aaa', upstreamId: 'ups_a', startedAt: '2026-08-01T00:00:00Z', forwarded: true, httpStatus: 200 },
          { requestId: 'req_2', resourceId: 'tts_bbb', upstreamId: 'ups_a', startedAt: '2026-08-01T00:00:00Z', forwarded: true, httpStatus: 200 },
          { requestId: 'req_3', resourceId: 'asr_ccc', upstreamId: 'ups_a', startedAt: '2026-08-01T00:00:00Z', forwarded: true, httpStatus: 200 },
          { requestId: 'req_4', resourceId: 'mcp_ddd', upstreamId: 'ups_a', startedAt: '2026-08-01T00:00:00Z', forwarded: true, httpStatus: 200 },
        ],
        nextCursor: undefined,
      }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('MODEL')
    expect(text).toContain('TTS')
    expect(text).toContain('ASR')
    expect(text).toContain('MCP')
  })

  it('shows error class and duration for a failed request', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 1, forwardedRequestCount: 0, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'UNKNOWN' },
        }
      }
      return {
        items: [
          { requestId: 'req_err', resourceId: 'mdl_aaa', upstreamId: 'ups_a', startedAt: '2026-08-01T00:00:00Z', forwarded: false, httpStatus: 504, upstreamHttpStatus: 504, durationMs: 1234, errorClass: 'UPSTREAM_TIMEOUT' },
        ],
        nextCursor: undefined,
      }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('UPSTREAM_TIMEOUT')
    expect(text).toContain('1234')
  })

  it('renders cost semantics status explicitly (KNOWN/PARTIAL/UNKNOWN)', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 0, forwardedRequestCount: 0, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'PARTIAL', amount: '0.0100', currency: 'USD' },
        }
      }
      return { items: [], nextCursor: undefined }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    expect(wrapper.text()).toContain('PARTIAL')
    expect(wrapper.text()).toContain('0.0100 USD')
  })

  it('switches to the Pricing tab and renders the pricing panel', async () => {
    const { wrapper } = mountUsagePage()
    await flushPromises()
    const tabs = wrapper.findAllComponents(QTab).map((t) => String(t.props('label')))
    expect(tabs).toContain('Summary')
    expect(tabs).toContain('Pricing')

    const pricingTab = wrapper.findAllComponents(QTab).find((t) => String(t.props('label')) === 'Pricing')
    await pricingTab!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Pricing')
    expect(wrapper.findComponent(PricingPanel).exists()).toBe(true)
  })

  // ---- Task B: Filters (§14 Filter) ----

  it('sends the completeness filter in the query string', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 0, forwardedRequestCount: 0, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'UNKNOWN' },
        }
      }
      return { items: [], nextCursor: undefined }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()

    const completenessSelect = wrapper.findAllComponents(QSelect).find((s) => String(s.props('label')) === 'Completeness')
    expect(completenessSelect).toBeTruthy()
    await completenessSelect!.setValue('UNKNOWN')
    await flushPromises()

    const reqCalls = fetchSpy.mock.calls.filter((c) => c[0].includes('/api/admin/v1/usage/requests'))
    expect(reqCalls.length).toBeGreaterThanOrEqual(2)
    expect(reqCalls[reqCalls.length - 1]![0]).toContain('completeness=UNKNOWN')
  })

  it('reset clears all filters and returns to the unfiltered query', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 0, forwardedRequestCount: 0, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'UNKNOWN' },
        }
      }
      return { items: [], nextCursor: undefined }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()

    const kindSelect = wrapper.findAllComponents(QSelect).find((s) => String(s.props('label')) === 'Kind')
    await kindSelect!.setValue('MODEL')
    await flushPromises()
    expect(wrapper.findAllComponents(QChip).some((c) => c.text().includes('MODEL'))).toBe(true)

    const resetBtn = wrapper.findAllComponents(QBtn).find((b) => String(b.props('label') ?? '') === 'Reset')
    expect(resetBtn).toBeTruthy()
    await resetBtn!.trigger('click')
    await flushPromises()

    const reqCall = fetchSpy.mock.calls[fetchSpy.mock.calls.length - 1]
    expect(reqCall![0].includes('resourceKind=')).toBe(false)
  })

  // ---- Task C: Request Detail (§14 Request Detail) ----

  it('opens a request detail dialog showing identity, generation, status and duration', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 1, forwardedRequestCount: 1, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'UNKNOWN' },
        }
      }
      return {
        items: [{
          requestId: 'req_abc', interactionId: 'int_1', deploymentId: 'dep_1', userId: 'usr_x',
          deviceId: 'dev_1', resourceId: 'mdl_aaa', runtimeRouteId: 'rte_1', upstreamId: 'ups_a',
          managedGeneration: 2, controlRevision: 5, startedAt: '2026-08-01T00:00:00Z',
          completedAt: '2026-08-01T00:00:01Z', forwarded: true, httpStatus: 200,
          upstreamHttpStatus: 200, requestBytes: 100, responseBytes: 200, durationMs: 45,
        }],
        nextCursor: undefined,
      }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()

    const firstRow = wrapper.findComponent(QItem)
    expect(firstRow.exists()).toBe(true)
    await firstRow.trigger('click')
    await flushPromises()

    const text = document.body.innerHTML
    expect(text).toContain('req_abc')
    expect(text).toContain('usr_x')
    expect(text).toContain('dev_1')
    expect(text).toContain('generation 2')
    expect(text).toContain('45 ms')
    expect(text).toContain('Control revision')
    expect(text).toContain('>5<')
  })

  it('request detail never shows prompt, body or secret content', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 1, forwardedRequestCount: 1, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'UNKNOWN' },
        }
      }
      return {
        items: [{ requestId: 'req_abc', resourceId: 'mdl_aaa', upstreamId: 'ups_a', managedGeneration: 1, controlRevision: 1, startedAt: '2026-08-01T00:00:00Z', forwarded: true, httpStatus: 200, requestBytes: 100, responseBytes: 200, durationMs: 45 }],
        nextCursor: undefined,
      }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    await wrapper.findComponent(QItem).trigger('click')
    await flushPromises()
    const text = document.body.innerHTML
    expect(text).not.toContain('prompt')
    expect(text).not.toContain('secret')
    expect(text).not.toContain('Authorization')
  })

  // ---- Task A: Summary metrics (§14 Summary) ----

  it('renders blocked count as total minus forwarded requests', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 10, forwardedRequestCount: 7, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'UNKNOWN' },
        }
      }
      return { items: [], nextCursor: undefined }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    expect(wrapper.text()).toContain('Requests')
    expect(wrapper.text()).toContain('10')
    expect(wrapper.text()).toContain('Blocked')
    expect(wrapper.text()).toContain('3')
  })

  it('renders semantic meters with confidence and a completeness breakdown', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 2, forwardedRequestCount: 2, requestBytes: 0, responseBytes: 0,
          semanticMeters: [
            { meter: 'INPUT_TOKENS', quantity: '1000', confidence: 'EXACT' },
            { meter: 'OUTPUT_TOKENS', quantity: '500', confidence: 'PARTIAL' },
            { meter: 'CHARACTERS', quantity: '300', confidence: 'UNKNOWN' },
          ],
          cost: { status: 'PARTIAL', amount: '0.0050', currency: 'USD' },
        }
      }
      return { items: [], nextCursor: undefined }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('INPUT_TOKENS')
    expect(text).toContain('1000')
    expect(text).toContain('OUTPUT_TOKENS')
    expect(text).toContain('CHARACTERS')
    // Completeness summary counts by confidence.
    expect(text).toContain('1 exact')
    expect(text).toContain('1 partial')
    expect(text).toContain('1 unknown')
  })
})
