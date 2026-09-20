import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer,
  QCard, QCardSection, QInput, QBtn, QBanner, QSelect, QList, QItem,
  QItemSection, QItemLabel, QChip, QSpinner, QIcon, QToolbarTitle,
  QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QTab, QTabs, QSeparator,
  QMenu, QDialog, QCardActions, QMarkupTable, ClosePopup,
  QField, QSpace, QBadge,
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
            QField, QSpace, QBadge,
          },
          directives: { ClosePopup },
        }], pinia, router],
      },
    },
  )
  return { wrapper }
}

async function openRequests(wrapper: ReturnType<typeof mountUsagePage>['wrapper']) {
  const tab = wrapper.findAllComponents(QTab).find(item => item.props('name') === 'requests')
  expect(tab).toBeTruthy()
  await tab!.trigger('click')
  await flushPromises()
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
          requestCompleteness: { exact: 8, partial: 1, unknown: 3 },
          forwardedRequestCount: 10,
          requestBytes: 2048,
          responseBytes: 4096,
          semanticMeters: [{ confidence: 'EXACT', meter: 'INPUT_TOKENS', quantity: '100' }],
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

  it.each([false, true])('ignores an obsolete filter response (failure=%s)', async (failOld) => {
    let resolveOld!: (value: unknown) => void
    let rejectOld!: (reason: Error) => void
    const oldResponse = new Promise((resolve, reject) => { resolveOld = resolve; rejectOld = reject })
    const currentSummary = {
      requestCount: 1, forwardedRequestCount: 1, requestBytes: 0, responseBytes: 0,
      requestCompleteness: { exact: 0, partial: 0, unknown: 1 },
      semanticMeters: [], cost: { status: 'UNKNOWN' },
    }
    vi.mocked(client.apiFetch).mockImplementation(async (path: string) => {
      if (path.includes('resourceKind=MODEL')) return oldResponse
      if (path.includes('/summary')) return currentSummary
      return { items: path.includes('resourceKind=TTS') ? [{ requestId: 'new', resourceId: 'tts_new', resourceDisplayName: 'Current speech', forwarded: true, httpStatus: 200 }] : [] }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    await openRequests(wrapper)
    const kind = wrapper.findAllComponents(QSelect).find(input => input.props('label') === 'Resource kind')!
    await kind.setValue('MODEL')
    await kind.setValue('TTS')
    await flushPromises()
    expect(wrapper.text()).toContain('Current speech')
    if (failOld) rejectOld(new Error('Obsolete query failed'))
    else resolveOld({ ...currentSummary, items: [{ requestId: 'old', resourceDisplayName: 'Obsolete model' }] })
    await flushPromises()
    expect(wrapper.text()).toContain('Current speech')
    expect(wrapper.text()).not.toContain('Obsolete model')
    expect(wrapper.text()).not.toContain('Obsolete query failed')
    wrapper.unmount()
  })

  it('discards pagination from a previous filter after a new query starts', async () => {
    let resolvePage!: (value: unknown) => void
    const pendingPage = new Promise(resolve => { resolvePage = resolve })
    const originalFetch = vi.mocked(client.apiFetch).getMockImplementation()!
    vi.mocked(client.apiFetch).mockImplementation(async (path: string) => {
      if (path.includes('/summary')) return originalFetch(path)
      if (path.includes('cursor=')) return pendingPage
      if (path.includes('resourceKind=TTS')) return { items: [{ requestId: 'current', resourceId: 'tts_current', resourceDisplayName: 'Filtered speech', forwarded: true, httpStatus: 200 }] }
      return { items: [], nextCursor: 'older-page' }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    await openRequests(wrapper)
    await wrapper.get('[aria-label="Next page"]').trigger('click')
    const kind = wrapper.findAllComponents(QSelect).find(input => input.props('label') === 'Resource kind')!
    await kind.setValue('TTS')
    await flushPromises()
    resolvePage({ items: [{ requestId: 'old', resourceId: 'mdl_old', resourceDisplayName: 'Unfiltered old model' }], nextCursor: 'still-older' })
    await flushPromises()
    expect(wrapper.text()).toContain('Filtered speech')
    expect(wrapper.text()).not.toContain('Unfiltered old model')
    expect(wrapper.get('[aria-label="Next page"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('uses whole-request completeness from the server instead of counting meter groups', async () => {
    const { wrapper } = mountUsagePage()
    await flushPromises()
    const counts = wrapper.find('[data-cy="request-completeness"]').text()
    expect(counts).toContain('8 exact')
    expect(counts).toContain('1 partial')
    expect(counts).toContain('3 unknown')
    wrapper.unmount()
  })

  it('distinguishes the start and end of the usage time filter', async () => {
    const { wrapper } = mountUsagePage()
    await flushPromises()
    await openRequests(wrapper)
    const labels = wrapper.findAllComponents(QInput).map(input => input.props('label'))
    expect(labels).toContain('Start time')
    expect(labels).toContain('End time')
    const start = wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Start time')!
    expect(start.props('type')).toBe('datetime-local')
    await start.setValue('2026-09-18T09:30')
    await flushPromises()
    const paths = vi.mocked(client.apiFetch).mock.calls.map(call => call[0]).filter(path => path.includes('/usage/requests'))
    expect(new URL(paths.at(-1)!, 'http://localhost').searchParams.get('from')).toBe(new Date('2026-09-18T09:30').toISOString())
    wrapper.unmount()
  })

  it('renders unknown cost status without amount when unknown', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z',
          to: '2026-08-20T00:00:00Z',
          requestCount: 0, requestCompleteness: { exact: 0, partial: 0, unknown: 0 },
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

  it('classifies each request row by resource kind including image generation', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 5, requestCompleteness: { exact: 0, partial: 0, unknown: 5 }, forwardedRequestCount: 4, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'KNOWN', amount: '0', currency: 'USD' },
        }
      }
      return {
        items: [
          { requestId: 'req_1', resourceId: 'mdl_aaa', resourceDisplayName: 'Enterprise model', upstreamId: 'ups_a', startedAt: '2026-08-01T00:00:00Z', forwarded: true, httpStatus: 200 },
          { requestId: 'req_img', resourceId: 'img_picture', upstreamId: 'ups_a', startedAt: '2026-08-01T00:00:00Z', forwarded: true, httpStatus: 200 },
          { requestId: 'req_2', resourceId: 'tts_bbb', upstreamId: 'ups_a', startedAt: '2026-08-01T00:00:00Z', forwarded: true, httpStatus: 200 },
          { requestId: 'req_3', resourceId: 'asr_ccc', upstreamId: 'ups_a', startedAt: '2026-08-01T00:00:00Z', forwarded: true, httpStatus: 200 },
          { requestId: 'req_4', resourceId: 'mcp_ddd', upstreamId: 'ups_a', startedAt: '2026-08-01T00:00:00Z', forwarded: false, httpStatus: 403, errorClass: 'ROUTE_POLICY_DENIED' },
        ],
        nextCursor: undefined,
      }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    await openRequests(wrapper)
    const text = wrapper.text()
    expect(text).toContain('Model')
    expect(text).toContain('Image generation')
    expect(text).toContain('TTS')
    expect(text).toContain('ASR')
    expect(text).toContain('MCP')
    expect(text).toContain('Route does not allow this request')
    const firstRow = wrapper.find('[data-cy="usage-row"]')
    expect(firstRow.text()).toContain('Enterprise model')
    expect(firstRow.text()).not.toContain('req_1')
    expect(firstRow.text()).not.toContain('ups_a')
  })

  it('shows error class and duration for a failed request', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 1, requestCompleteness: { exact: 0, partial: 0, unknown: 1 }, forwardedRequestCount: 0, requestBytes: 0, responseBytes: 0,
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
    await openRequests(wrapper)
    const text = wrapper.text()
    expect(text).toContain('UPSTREAM_TIMEOUT')
    expect(text).toContain('1234')
  })

  it('renders cost semantics status explicitly (KNOWN/PARTIAL/UNKNOWN)', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 0, requestCompleteness: { exact: 0, partial: 0, unknown: 0 }, forwardedRequestCount: 0, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'PARTIAL', amount: '0.0100', currency: 'USD' },
        }
      }
      return { items: [], nextCursor: undefined }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    expect(wrapper.text()).toContain('Partial cost')
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

  it('keeps lower-frequency filters collapsed and reports their active count', async () => {
    const { wrapper } = mountUsagePage()
    await flushPromises()
    const advanced = wrapper.get('[data-cy="usage-advanced-filters"]')
    expect((advanced.element as HTMLElement).style.display).toBe('none')
    const more = wrapper.get('[data-cy="usage-more-filters"]')
    expect(more.text()).toContain('(0)')
    await more.trigger('click')
    const kind = wrapper.findAllComponents(QSelect).find(select => String(select.props('label')).toLowerCase().includes('kind'))
    await kind!.setValue('MODEL')
    await flushPromises()
    expect(more.text()).toContain('(1)')
  })

  it('sends the completeness filter in the query string', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 0, requestCompleteness: { exact: 0, partial: 0, unknown: 0 }, forwardedRequestCount: 0, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'UNKNOWN' },
        }
      }
      return { items: [], nextCursor: undefined }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    await openRequests(wrapper)

    const completenessSelect = wrapper.findAllComponents(QSelect).find((s) => String(s.props('label')).includes('completeness'))
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
          requestCount: 0, requestCompleteness: { exact: 0, partial: 0, unknown: 0 }, forwardedRequestCount: 0, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'UNKNOWN' },
        }
      }
      return { items: [], nextCursor: undefined }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    await openRequests(wrapper)

    const kindSelect = wrapper.findAllComponents(QSelect).find((s) => String(s.props('label')).includes('kind') || String(s.props('label')).includes('Kind'))
    await kindSelect!.setValue('MODEL')
    await flushPromises()
    // The filter is proven by the query it produces, not by a chip restating it.
    const filteredCall = fetchSpy.mock.calls.find((c) => c[0].includes('/usage/requests') && c[0].includes('resourceKind=MODEL'))
    expect(filteredCall).toBeTruthy()

    const resetBtn = wrapper.findAllComponents(QBtn).find((b) => String(b.props('label') ?? '').includes('Reset'))
    expect(resetBtn).toBeTruthy()
    await resetBtn!.trigger('click')
    await flushPromises()

    const reqCall = fetchSpy.mock.calls[fetchSpy.mock.calls.length - 1]
    expect(reqCall![0].includes('resourceKind=')).toBe(false)
  })

  // ---- Task C: Request Detail (§14 Request Detail) ----

  it('opens a request detail workspace showing identity, generation, status and duration', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 1, requestCompleteness: { exact: 0, partial: 0, unknown: 1 }, forwardedRequestCount: 1, requestBytes: 0, responseBytes: 0,
          semanticMeters: [], cost: { status: 'UNKNOWN' },
        }
      }
      return {
        items: [{
          requestId: 'req_abc', interactionId: 'int_1', deploymentId: 'dep_1', userId: 'usr_x',
          deviceId: 'dev_1', resourceId: 'mdl_aaa', resourceKind: 'MODEL', clientProtocol: 'OPENAI_RESPONSES', runtimeRouteId: 'rte_1', upstreamId: 'ups_a',
          managedGeneration: 2, controlRevision: 5, startedAt: '2026-08-01T00:00:00Z',
          completedAt: '2026-08-01T00:00:01Z', forwarded: true, httpStatus: 200,
          upstreamHttpStatus: 200, requestBytes: 100, responseBytes: 200, durationMs: 45,
          requestCompleteness: 'PARTIAL', settlementState: 'RECONCILIATION_REQUIRED',
          semanticMeters: [{ meter: 'TOTAL_TOKENS', quantity: '10000', confidence: 'PARTIAL' }],
          budget: { capability: 'MODEL', mode: 'LIMITED', revision: 7, asOf: '2026-08-01T00:00:01Z', blockers: [{ period: 'DAY', meter: 'TOTAL_TOKENS', limit: '10000', used: '9000', reserved: '1000', remaining: '0', overage: '0', scopeStart: '2026-08-01T00:00:00Z', resetAt: '2026-08-02T00:00:00Z' }] },
        }],
        nextCursor: undefined,
      }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    await openRequests(wrapper)

    const firstRow = wrapper.find('[data-cy="usage-row"]')
    expect(firstRow.exists()).toBe(true)
    await firstRow.trigger('click')
    await flushPromises()

    const text = wrapper.get('[data-cy="usage-detail"]').text()
    expect(text).toContain('req_abc')
    expect(text).toContain('usr_x')
    expect(text).toContain('dev_1')
    expect(text).toContain('Generation 2')
    expect(text).toContain('45 ms')
    expect(text).toContain('Desired Revision')
    expect(text).toContain('5')
    expect(text).toContain('OPENAI_RESPONSES')
    expect(text).toContain('Reconciliation required')
    expect(text).toContain('10,000 tokens')
    expect(text).toContain('Revision 7')
    expect(wrapper.get('[data-cy="usage-detail"]').text()).not.toContain('Unknown cost')
  })

  it('request detail never shows prompt, body or secret content', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/usage/summary')) {
        return {
          from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z',
          requestCount: 1, requestCompleteness: { exact: 0, partial: 0, unknown: 1 }, forwardedRequestCount: 1, requestBytes: 0, responseBytes: 0,
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
    await openRequests(wrapper)
    await wrapper.findComponent(QItem).trigger('click')
    await flushPromises()
    const text = wrapper.get('[data-cy="usage-detail"]').text()
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
          requestCount: 10, requestCompleteness: { exact: 0, partial: 0, unknown: 10 }, forwardedRequestCount: 7, requestBytes: 0, responseBytes: 0,
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
          requestCount: 2, requestCompleteness: { exact: 0, partial: 0, unknown: 2 }, forwardedRequestCount: 2, requestBytes: 0, responseBytes: 0,
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
    expect(text).toContain('Input tokens')
    expect(text).toContain('1K tokens')
    expect(text).toContain('Output tokens')
    expect(text).toContain('500 tokens')
    expect(text).toContain('Characters')
    // Three meter groups do not turn two unknown requests into three requests.
    expect(text).toContain('0 exact')
    expect(text).toContain('0 partial')
    expect(text).toContain('2 unknown')
  })

  it('loads server-side trend, distribution and user aggregates and applies the protocol filter', async () => {
    const paths: string[] = []
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      paths.push(path)
      if (path.includes('/usage/summary')) return {
        from: '2026-09-19T00:00:00Z', to: '2026-09-20T00:00:00Z', requestCount: 2,
        forwardedRequestCount: 2, requestCompleteness: { exact: 2, partial: 0, unknown: 0 },
        requestBytes: 0, responseBytes: 0, semanticMeters: [], cost: { status: 'UNKNOWN' },
      }
      if (path.includes('/usage/trend')) return {
        from: '2026-09-19T00:00:00Z', to: '2026-09-20T00:00:00Z', timezone: 'Asia/Shanghai',
        points: [{ date: '2026-09-19', requestCount: 2, forwardedRequestCount: 2, semanticMeters: [{ meter: 'TOTAL_TOKENS', quantity: '10000', confidence: 'EXACT' }] }],
      }
      if (path.includes('/usage/distribution')) return {
        from: '2026-09-19T00:00:00Z', to: '2026-09-20T00:00:00Z',
        items: [{ resourceKind: 'ASR', clientProtocol: 'OPENAI_REALTIME_TRANSCRIPTION', requestCount: 1, semanticMeters: [{ meter: 'AUDIO_SECONDS', quantity: '120', confidence: 'EXACT' }] }],
      }
      if (path.includes('/usage/users')) return {
        items: [{ userId: 'usr_1', userDisplayName: 'Ada', requestCount: 2, semanticMeters: [], budget: { userId: 'usr_1', timezone: 'Asia/Shanghai', asOf: '2026-09-20T00:00:00Z', items: [{ capability: 'MODEL', mode: 'LIMITED', source: 'EXPLICIT', revision: 1, effectiveFrom: '2026-09-01T00:00:00Z', asOf: '2026-09-20T00:00:00Z', inFlightRequests: 0, limits: [], usageMeters: [], status: 'EXHAUSTED' }] } }],
      }
      return { items: [], nextCursor: undefined }
    })
    const { wrapper } = mountUsagePage()
    await flushPromises()
    expect(wrapper.text()).toContain('10K tokens')
    expect(wrapper.text()).toContain('2 min')
    expect(wrapper.text()).toContain('OPENAI_REALTIME_TRANSCRIPTION')
    expect(wrapper.text()).toContain('Ada')
    expect(wrapper.text()).toContain('Exhausted')

    const protocol = wrapper.findAllComponents(QSelect).find(input => input.props('label') === 'Protocol')!
    await protocol.setValue('OPENAI_RESPONSES')
    await flushPromises()
    expect(paths.some(path => path.includes('/usage/trend') && path.includes('clientProtocol=OPENAI_RESPONSES'))).toBe(true)
    expect(paths.some(path => path.includes('/usage/distribution') && path.includes('clientProtocol=OPENAI_RESPONSES'))).toBe(true)
    expect(paths.some(path => path.includes('/usage/requests'))).toBe(false)

    await openRequests(wrapper)
    expect(paths.some(path => path.includes('/usage/requests') && path.includes('clientProtocol=OPENAI_RESPONSES'))).toBe(true)

    const summaryTab = wrapper.findAllComponents(QTab).find(item => item.props('name') === 'summary')!
    await summaryTab.trigger('click')
    await flushPromises()

    const budgetHealth = wrapper.findAllComponents(QSelect).find(input => input.props('label') === 'Budget health')!
    await budgetHealth.setValue('EXHAUSTED')
    await flushPromises()
    expect(paths.some(path => path.includes('/usage/users') && path.includes('budgetStatus=EXHAUSTED'))).toBe(true)
    expect(paths.some(path => path.includes('/usage/summary') && path.includes('budgetStatus='))).toBe(false)
  })
})
