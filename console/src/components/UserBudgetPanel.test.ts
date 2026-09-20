import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import {
  QBanner, QBtn, QCard, QCardActions, QCardSection, QChip, QDialog, QIcon, QInput, QItem,
  QItemLabel, QItemSection, QList, QSelect, QSeparator, QSpinner, Quasar,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { ApiProblem } from '../api/client'
import * as client from '../api/client'
import { useSessionStore } from '../stores/session'
import UserBudgetPanel from './UserBudgetPanel.vue'
import PagedEntityPicker from './PagedEntityPicker.vue'

const limited = {
  capability: 'MODEL', mode: 'LIMITED', source: 'EXPLICIT', revision: 4,
  effectiveFrom: '2026-09-20T00:00:00Z', asOf: '2026-09-20T01:00:00Z',
  inFlightRequests: 2, status: 'AVAILABLE',
  usageMeters: [{ meter: 'TOTAL_TOKENS', quantity: '12500', confidence: 'EXACT' }],
  limits: [
    { period: 'DAY', meter: 'TOTAL_TOKENS', limit: '10000', used: '4000', reserved: '1000', remaining: '5000', overage: '0', scopeStart: '2026-09-20T00:00:00Z', resetAt: '2026-09-21T00:00:00Z' },
    { period: 'MONTH', meter: 'REQUESTS', limit: '1000', used: '100', reserved: '5', remaining: '895', overage: '0', scopeStart: '2026-09-01T00:00:00Z', resetAt: '2026-10-01T00:00:00Z' },
  ],
} as const

function budgetView(revision = 4) {
  return {
    userId: 'usr_1', timezone: 'Asia/Shanghai', asOf: '2026-09-20T01:00:00Z',
    items: [
      { ...limited, revision },
      { capability: 'TTS', mode: 'UNLIMITED', source: 'DEFAULT', revision: 0, effectiveFrom: '2026-09-01T00:00:00Z', asOf: '2026-09-20T01:00:00Z', inFlightRequests: 0, limits: [], usageMeters: [{ meter: 'CHARACTERS', quantity: '1200', confidence: 'EXACT' }], status: 'AVAILABLE' },
      { capability: 'ASR', mode: 'UNLIMITED', source: 'EXPLICIT', revision: 2, effectiveFrom: '2026-09-01T00:00:00Z', asOf: '2026-09-20T01:00:00Z', inFlightRequests: 0, limits: [], usageMeters: [], status: 'AVAILABLE' },
      { capability: 'MCP', mode: 'LIMITED', source: 'EXPLICIT', revision: 1, effectiveFrom: '2026-09-01T00:00:00Z', asOf: '2026-09-20T01:00:00Z', inFlightRequests: 1, limits: [{ period: 'LIFETIME', meter: 'REQUESTS', limit: '100', used: '100', reserved: '0', remaining: '0', overage: '0', scopeStart: '2026-09-01T00:00:00Z' }], usageMeters: [{ meter: 'REQUESTS', quantity: '100', confidence: 'EXACT' }], status: 'EXHAUSTED' },
      { capability: 'IMAGE_GENERATION', mode: 'LIMITED', source: 'TEMPLATE', revision: 3, effectiveFrom: '2026-09-01T00:00:00Z', asOf: '2026-09-20T01:00:00Z', inFlightRequests: 0, limits: [{ period: 'MONTH', meter: 'REQUESTED_IMAGES', limit: '20', used: '4', reserved: '0', remaining: '16', overage: '0', scopeStart: '2026-09-01T00:00:00Z', resetAt: '2026-10-01T00:00:00Z' }], usageMeters: [], status: 'AVAILABLE' },
    ],
    templateAssignment: { budgetTemplateId: 'bgt_1', name: 'Standard team', templateRevision: 2, assignmentRevision: 1, assignedAt: '2026-09-20T00:00:00Z' },
  }
}

function mountPanel() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const session = useSessionStore(pinia)
  session.session = {
    user: { userId: 'admin', displayName: 'Admin', role: 'ADMIN' },
    csrfToken: 'csrf', expiresAt: '2026-12-31T00:00:00Z',
  }
  const wrapper = mount(UserBudgetPanel, {
    props: { userId: 'usr_1' },
    global: {
      plugins: [[Quasar, { components: {
        QBanner, QBtn, QCard, QCardActions, QCardSection, QChip, QDialog, QIcon, QInput, QItem,
        QItemLabel, QItemSection, QList, QSelect, QSeparator, QSpinner,
      } }], pinia],
    },
  })
  return wrapper
}

describe('UserBudgetPanel', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.endsWith('/budgets')) return budgetView()
      if (path.startsWith('/api/admin/v1/budget-templates')) return { items: [{ budgetTemplateId: 'bgt_1', name: 'Standard team', description: '', revision: 2, rules: [], assignedUserCount: 1, createdAt: '2026-09-20T00:00:00Z', updatedAt: '2026-09-20T00:00:00Z' }] }
      if (path.includes('/audit')) return { items: [], nextCursor: undefined }
      return limited
    })
  })

  it('distinguishes inherited and explicit unlimited budgets and shows current state', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('Deployment default')
    expect(text).toContain('User override')
    expect(text).toContain('Budget template')
    expect(text).toContain('Standard team')
    expect(wrapper.find('[data-cy="budget-IMAGE_GENERATION"]').exists()).toBe(true)
    expect(text).toContain('inherits the global setting')
    expect(text).toContain('5K tokens')
    expect(text).toContain('12.5K tokens')
    expect(text).toContain('1.2K chars')
    expect(text).toContain('Retained cumulative history')
    expect(text).toContain('2 in flight')
    expect(text).toContain('Exhausted')
  })

  it('clears a user override back to the assigned template', async () => {
    const fetch = vi.spyOn(client, 'apiFetch')
    fetch.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.endsWith('/budgets')) return budgetView()
      if (init?.method === 'DELETE') return { ...limited, source: 'TEMPLATE', revision: 5 }
      return { items: [] }
    })
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[data-cy="clear-budget-override-MODEL"]').trigger('click')
    await flushPromises()
    const call = fetch.mock.calls.find(([, init]) => init?.method === 'DELETE')!
    expect(call[0]).toContain('/budgets/MODEL?')
    expect(call[0]).toContain('expectedRevision=4')
  })

  it('previews the preserved overrides before replacing or removing a template assignment', async () => {
    const fetch = vi.spyOn(client, 'apiFetch')
    fetch.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.endsWith('/budgets')) return budgetView()
      if (path === '/api/admin/v1/budget-templates/bgt_2') return {
        budgetTemplateId: 'bgt_2', name: 'Focused team', description: '', revision: 1,
        rules: [
          { capability: 'MODEL', mode: 'LIMITED', limits: [{ period: 'MONTH', meter: 'REQUESTS', limit: '250' }] },
          { capability: 'TTS', mode: 'LIMITED', limits: [{ period: 'MONTH', meter: 'CHARACTERS', limit: '500' }] },
          { capability: 'IMAGE_GENERATION', mode: 'LIMITED', limits: [{ period: 'MONTH', meter: 'REQUESTED_IMAGES', limit: '20' }] },
        ],
        assignedUserCount: 0, createdAt: '2026-09-20T00:00:00Z', updatedAt: '2026-09-20T00:00:00Z',
      }
      if (init?.method === 'PUT' || init?.method === 'DELETE') return budgetView()
      return { items: [] }
    })
    const wrapper = mountPanel()
    await flushPromises()
    wrapper.findComponent(PagedEntityPicker).vm.$emit('update:modelValue', 'bgt_2')
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Reason for change')!.setValue('Move teams')
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'Assign / replace')!.trigger('click')
    await flushPromises()
    expect(fetch.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
    expect(document.body.textContent).toContain('only capabilities without a user override')
    expect(document.body.textContent).toContain('Explicit user overrides remain unchanged')
    expect(document.body.querySelector('[data-cy="budget-template-assignment-change-TTS"]')?.textContent).toContain('500')
    expect(document.body.querySelector('[data-cy="budget-template-assignment-change-MODEL"]')).toBeNull()
    expect(document.body.querySelector('[data-cy="budget-template-assignment-change-IMAGE_GENERATION"]')).toBeNull()
    await (document.body.querySelector('[data-cy="confirm-budget-template-assignment"]') as HTMLElement).click()
    await flushPromises()
    expect(fetch.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(true)

    await wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Reason for change')!.setValue('Return to defaults')
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'Unassign')!.trigger('click')
    expect(fetch.mock.calls.filter(([, init]) => init?.method === 'DELETE')).toHaveLength(0)
    await (document.body.querySelector('[data-cy="confirm-budget-template-assignment"]') as HTMLElement).click()
    await flushPromises()
    expect(fetch.mock.calls.filter(([, init]) => init?.method === 'DELETE')).toHaveLength(1)
  })

  it('reloads the current assignment after an assignment conflict', async () => {
    let reads = 0
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.endsWith('/budgets')) return budgetView(++reads === 1 ? 4 : 9)
      if (path === '/api/admin/v1/budget-templates/bgt_1') return { budgetTemplateId: 'bgt_1', name: 'Standard team', description: '', revision: 2, rules: [], assignedUserCount: 1, createdAt: '2026-09-20T00:00:00Z', updatedAt: '2026-09-20T00:00:00Z' }
      if (init?.method === 'PUT') throw new ApiProblem(409, 'budget_template_assignment_revision_conflict', 'stale')
      return { items: [] }
    })
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Reason for change')!.setValue('Move teams')
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'Assign / replace')!.trigger('click')
    await flushPromises()
    await (document.body.querySelector('[data-cy="confirm-budget-template-assignment"]') as HTMLElement).click()
    await flushPromises()
    expect(reads).toBe(2)
    expect(wrapper.get('[data-cy="budget-conflict"]').text()).toContain('reloaded')
  })

  it('reloads current state when clearing an override conflicts', async () => {
    let reads = 0
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.endsWith('/budgets')) return budgetView(++reads === 1 ? 4 : 9)
      if (init?.method === 'DELETE') throw new ApiProblem(409, 'budget_revision_conflict', 'stale')
      return { items: [] }
    })
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[data-cy="clear-budget-override-MODEL"]').trigger('click')
    await flushPromises()
    expect(reads).toBe(2)
    expect(wrapper.get('[data-cy="budget-conflict"]').text()).toContain('reloaded')
    expect(wrapper.text()).toContain('Revision 9')
  })

  it('saves all limited rules with the loaded revision', async () => {
    const fetch = vi.spyOn(client, 'apiFetch')
    fetch.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.endsWith('/budgets')) return budgetView()
      if (init?.method === 'PUT') return { ...limited, revision: 5 }
      return { items: [] }
    })
    const wrapper = mountPanel()
    await flushPromises()
    const model = wrapper.get('[data-cy="budget-MODEL"]')
    await model.findAllComponents(QBtn).find((button: { props: (name: string) => unknown }) => button.props('icon') === 'edit')!.trigger('click')
    await flushPromises()
    await model.findAllComponents(QInput).at(-1)!.setValue('Quarterly allocation')
    await model.get('[data-cy="save-budget"]').trigger('click')
    await flushPromises()
    const call = fetch.mock.calls.find(([, init]) => init?.method === 'PUT')!
    expect(call[0]).toContain('/budgets/MODEL')
    expect(JSON.parse(String(call[1]?.body))).toEqual({
      expectedRevision: 4,
      mode: 'LIMITED',
      reason: 'Quarterly allocation',
      limits: [
        { period: 'DAY', meter: 'TOTAL_TOKENS', limit: '10000' },
        { period: 'MONTH', meter: 'REQUESTS', limit: '1000' },
      ],
    })
  })

  it('reloads the current server state after a CAS conflict', async () => {
    let reads = 0
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.endsWith('/budgets')) return budgetView(++reads === 1 ? 4 : 9)
      if (init?.method === 'PUT') throw new ApiProblem(409, 'budget_revision_conflict', 'stale')
      return { items: [] }
    })
    const wrapper = mountPanel()
    await flushPromises()
    const model = wrapper.get('[data-cy="budget-MODEL"]')
    await model.findAllComponents(QBtn).find((button: { props: (name: string) => unknown }) => button.props('icon') === 'edit')!.trigger('click')
    await model.findAllComponents(QInput).at(-1)!.setValue('Quarterly allocation')
    await model.get('[data-cy="save-budget"]').trigger('click')
    await flushPromises()
    expect(reads).toBe(2)
    expect(wrapper.get('[data-cy="budget-conflict"]').text()).toContain('reloaded')
    expect(wrapper.text()).toContain('Revision 9')
  })

  it('loads immutable audit history only when requested', async () => {
    const fetch = vi.spyOn(client, 'apiFetch')
    fetch.mockImplementation(async (path: string) => {
      if (path.endsWith('/budgets')) return budgetView()
      if (path.includes('/MODEL/audit')) return {
        items: [{
          auditId: 'aud_1', userId: 'usr_1', capability: 'MODEL', previousRevision: 3,
          revision: 4, mode: 'LIMITED', limits: [], changedBy: 'admin',
          reason: 'Quarterly allocation', createdAt: '2026-09-20T00:00:00Z',
        }],
      }
      return { items: [] }
    })
    const wrapper = mountPanel()
    await flushPromises()
    expect(fetch.mock.calls.some(([path]) => path.includes('/audit'))).toBe(false)
    await wrapper.get('[data-cy="budget-audit-MODEL"]').trigger('click')
    await flushPromises()
    expect(fetch.mock.calls.some(([path]) => path.includes('/MODEL/audit?limit=20'))).toBe(true)
    expect(wrapper.text()).toContain('Quarterly allocation')
    expect(wrapper.text()).toContain('admin')
  })
})
