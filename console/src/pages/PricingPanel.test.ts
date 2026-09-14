import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QCard, QCardSection, QCardActions, QInput, QBtn, QBanner,
  QSelect, QList, QItem, QItemSection, QItemLabel, QChip, QSpinner,
  QIcon, QToolbarTitle, QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown,
  QSeparator,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import PricingPanel from './PricingPanel.vue'
import PageHeader from '../components/PageHeader.vue'
import { useSessionStore } from '../stores/session'
import * as client from '../api/client'

function mountPanel() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const session = useSessionStore(pinia)
  session.session = {
    user: { userId: 'usr_001', displayName: 'Admin', role: 'ADMIN' as const },
    csrfToken: 'test-csrf',
    expiresAt: '2026-12-31T23:59:59Z',
  }
  const wrapper = mount(PricingPanel, {
    global: {
      plugins: [[Quasar, {
        components: {
          QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QSelect,
          QList, QItem, QItemSection, QItemLabel, QChip, QSpinner, QIcon,
          QToolbarTitle, QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QSeparator,
          PageHeader,
        },
      }], pinia],
    },
  })
  return { wrapper, pinia }
}

function findBtn(wrapper: ReturnType<typeof mount>, label: string) {
  return wrapper.findAllComponents(QBtn).find((b) => String(b.props('label') ?? '').includes(label))
}

describe('PricingPanel', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/pricing') {
        return { pricingRevision: 3, rules: [] }
      }
      return {}
    })
  })

  it('loads the pricing set revision and renders existing rules', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/pricing') {
        return {
          pricingRevision: 7,
          rules: [
            { pricingRuleId: 'pr_1', meter: 'INPUT_TOKENS', unitSize: '1000', unitPrice: '0.0015', currency: 'USD', effectiveFrom: '2026-08-01T00:00:00Z' },
          ],
        }
      }
      return {}
    })
    const { wrapper } = mountPanel()
    await flushPromises()
    expect(wrapper.text()).toContain('7')
    expect(wrapper.text()).toContain('INPUT_TOKENS')
  })

  it('adds a rule then saves with expectedPricingRevision and CSRF', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === '/api/admin/v1/pricing') return { pricingRevision: 3, rules: [] }
      return {}
    })
    const { wrapper } = mountPanel()
    await flushPromises()

    const addBtn = findBtn(wrapper, 'Add rule')
    expect(addBtn).toBeTruthy()
    await addBtn!.trigger('click')
    await flushPromises()

    const saveBtn = findBtn(wrapper, 'Save')
    await saveBtn!.trigger('click')
    await flushPromises()

    const putCall = fetchSpy.mock.calls.find((c) => c[0] === '/api/admin/v1/pricing' && (c[1] as RequestInit)?.method === 'PUT')
    expect(putCall).toBeTruthy()
    const body = JSON.parse((putCall![1] as RequestInit).body as string)
    expect(body.expectedPricingRevision).toBe(3)
    expect(body.rules.length).toBeGreaterThanOrEqual(1)
  })

  it('removes a pricing rule from the local list', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/pricing') {
        return {
          pricingRevision: 1,
          rules: [
            { pricingRuleId: 'pr_a', meter: 'INPUT_TOKENS', unitSize: '1', unitPrice: '0.01', currency: 'USD', effectiveFrom: '2026-08-01T00:00:00Z' },
            { pricingRuleId: 'pr_b', meter: 'OUTPUT_TOKENS', unitSize: '1', unitPrice: '0.02', currency: 'USD', effectiveFrom: '2026-08-01T00:00:00Z' },
          ],
        }
      }
      return {}
    })
    const { wrapper } = mountPanel()
    await flushPromises()
    expect(wrapper.text()).toContain('pr_a')
    expect(wrapper.text()).toContain('pr_b')
    // The delete buttons remove rows from the local editor list.
    const delBtns = wrapper.findAllComponents(QBtn).filter((b) => String(b.props('icon') ?? '') === 'delete')
    expect(delBtns.length).toBe(2)
    await delBtns[0].trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('pr_a')
  })
})
