import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { Quasar, QBtn, QCard, QCardActions, QCardSection, QDialog, QInput, QSelect, QSeparator, QList, QItem, QItemSection, QItemLabel, QIcon, QBanner, QSpinner, QToggle } from 'quasar'
import BudgetTemplatesPage from './BudgetTemplatesPage.vue'
import * as client from '../api/client'
import { ApiProblem } from '../api/client'
import { useSessionStore } from '../stores/session'

function mountPage() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useSessionStore(pinia).session = { user: { userId: 'admin', displayName: 'Admin', role: 'ADMIN' }, csrfToken: 'csrf', expiresAt: '2026-12-31T00:00:00Z' }
  return mount(BudgetTemplatesPage, { global: { plugins: [[Quasar, { components: { QBtn, QCard, QCardActions, QCardSection, QDialog, QInput, QSelect, QSeparator, QList, QItem, QItemSection, QItemLabel, QIcon, QBanner, QSpinner, QToggle } }], pinia] } })
}

describe('BudgetTemplatesPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.includes('/budget-templates')) return { items: [{ budgetTemplateId: 'bgt_team', name: 'Team standard', description: 'Shared defaults', revision: 1, rules: [{ capability: 'IMAGE_GENERATION', mode: 'LIMITED', limits: [{ period: 'MONTH', meter: 'REQUESTED_IMAGES', limit: '50' }] }], assignedUserCount: 2, createdAt: '2026-09-20T00:00:00Z', updatedAt: '2026-09-20T00:00:00Z' }] }
      return {}
    })
  })

  it('uses the shared capability editor for five capability-level rules without resource selectors', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.get('[data-cy="detail-workspace"]')).toBeTruthy()
    await wrapper.get('[data-cy="budget-template-row"]').trigger('click')
    await flushPromises()
    for (const capability of ['MODEL', 'TTS', 'ASR', 'MCP', 'IMAGE_GENERATION']) {
      expect(wrapper.find(`[data-cy="template-rule-${capability}"]`).exists()).toBe(true)
    }
    expect(wrapper.text()).toContain('Requested images')
    expect(wrapper.find('[data-cy="resource-budget-selector"]').exists()).toBe(false)
  })

  it('shows collection context and lets narrow workspaces return to the list', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.get('[data-cy="budget-template-row"]').text()).toContain('Shared defaults')
    expect(wrapper.get('[data-cy="budget-template-row"]').text()).toContain('Updated')

    await wrapper.get('[data-cy="budget-template-row"]').trigger('click')
    expect(wrapper.get('[data-cy="budget-template-back"]')).toBeTruthy()
    await wrapper.get('[data-cy="budget-template-back"]').trigger('click')
    expect(wrapper.find('[data-cy="budget-template-back"]').exists()).toBe(false)

    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'Create template')!.trigger('click')
    await wrapper.get('[data-cy="budget-template-back"]').trigger('click')
    expect(wrapper.find('[data-cy="budget-template-back"]').exists()).toBe(false)
  })

  it('loads assigned users and recent audit only when requested', async () => {
    const fetch = vi.spyOn(client, 'apiFetch')
    fetch.mockImplementation(async (path: string) => {
      if (path.endsWith('/users?limit=50')) return { items: [{ userId: 'usr_1', username: 'ana.ops', displayName: 'Ana', role: 'MEMBER', status: 'ACTIVE' }] }
      if (path.endsWith('/audit?limit=20')) return { items: [{
        auditId: 1, budgetTemplateId: 'bgt_team', action: 'UPDATE', templateRevision: 2,
        assignmentRevision: 0, changedBy: 'usr_admin', reason: 'Raise image allowance',
        createdAt: '2026-09-20T00:00:00Z',
        before: { name: 'Old standard', description: '', rules: [{ capability: 'IMAGE_GENERATION', mode: 'LIMITED', limits: [{ period: 'MONTH', meter: 'REQUESTED_IMAGES', limit: '20' }] }] },
        after: { name: 'Team standard', description: '', rules: [{ capability: 'IMAGE_GENERATION', mode: 'LIMITED', limits: [{ period: 'MONTH', meter: 'REQUESTED_IMAGES', limit: '50' }] }] },
      }] }
      return { items: [{ budgetTemplateId: 'bgt_team', name: 'Team standard', description: 'Shared defaults', revision: 1, rules: [], assignedUserCount: 1, createdAt: '2026-09-20T00:00:00Z', updatedAt: '2026-09-20T00:00:00Z' }] }
    })
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-cy="budget-template-row"]').trigger('click')
    expect(fetch.mock.calls.some(([path]) => path.includes('/users?'))).toBe(false)

    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'View assigned users')!.trigger('click')
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'View audit')!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-cy="budget-template-users"]').text()).toContain('Ana')
    expect(wrapper.get('[data-cy="budget-template-audit"]').text()).toContain('Raise image allowance')
    expect(wrapper.get('[data-cy="budget-template-audit"]').text()).toContain('Old standard')
    expect(wrapper.get('[data-cy="budget-template-audit"]').text()).toContain('Team standard')
    expect(wrapper.get('[data-cy="budget-template-audit"]').text()).toContain('Image generation')
    expect(wrapper.get('[data-cy="budget-template-audit"]').text()).toContain('20')
    expect(wrapper.get('[data-cy="budget-template-audit"]').text()).toContain('50')
  })

  it('lets the Hub own the template id when creating', async () => {
    const fetch = vi.spyOn(client, 'apiFetch')
    fetch.mockImplementation(async (path: string, init?: RequestInit) => {
      if (init?.method === 'POST') return { budgetTemplateId: 'bgt_server', name: 'New standard', description: '', revision: 1, rules: [], assignedUserCount: 0, createdAt: '2026-09-20T00:00:00Z', updatedAt: '2026-09-20T00:00:00Z' }
      return { items: [] }
    })
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'Create template')!.trigger('click')
    const inputs = wrapper.findAllComponents(QInput)
    const nameInput = inputs.find(input => input.props('label') === 'Template name')!
    expect(nameInput.props('maxlength')).toBe(100)
    await nameInput.setValue('New standard')
    await inputs.find(input => input.props('label') === 'Reason for change')!.setValue('New team policy')
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'Save')!.trigger('click')
    expect(fetch.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
    expect(document.body.textContent).toContain('no users change until it is assigned')
    await (document.body.querySelector('[data-cy="confirm-budget-template-save"]') as HTMLElement).click()
    await flushPromises()
    const createCall = fetch.mock.calls.find(([, init]) => init?.method === 'POST')!
    expect(createCall[0]).toBe('/api/admin/v1/budget-templates')
    expect(JSON.parse(String(createCall[1]?.body))).toEqual({ name: 'New standard', description: '', rules: [], reason: 'New team policy' })
  })

  it('previews the actual changed capability rules and confirms live-linked application', async () => {
    const fetch = vi.spyOn(client, 'apiFetch')
    fetch.mockImplementation(async (path: string, init?: RequestInit) => {
      if (init?.method === 'PUT') return {
        budgetTemplateId: 'bgt_team', name: 'Team standard', description: 'Shared defaults', revision: 2,
        rules: [{ capability: 'IMAGE_GENERATION', mode: 'LIMITED', limits: [{ period: 'MONTH', meter: 'REQUESTED_IMAGES', limit: '80' }] }],
        assignedUserCount: 2, createdAt: '2026-09-20T00:00:00Z', updatedAt: '2026-09-20T01:00:00Z',
      }
      return { items: [{
        budgetTemplateId: 'bgt_team', name: 'Team standard', description: 'Shared defaults', revision: 1,
        rules: [{ capability: 'IMAGE_GENERATION', mode: 'LIMITED', limits: [{ period: 'MONTH', meter: 'REQUESTED_IMAGES', limit: '50' }] }],
        assignedUserCount: 2, createdAt: '2026-09-20T00:00:00Z', updatedAt: '2026-09-20T00:00:00Z',
      }] }
    })
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-cy="budget-template-row"]').trigger('click')
    const imageRule = wrapper.get('[data-cy="template-rule-IMAGE_GENERATION"]')
    await imageRule.findAllComponents(QInput).find((input: { props: (name: string) => unknown }) => input.props('label') === 'Limit')!.setValue('80')
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Reason for change')!.setValue('Raise image allowance')
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'Save')!.trigger('click')
    expect(document.body.querySelector('[data-cy="budget-template-change-IMAGE_GENERATION"]')?.textContent).toContain('50')
    expect(document.body.querySelector('[data-cy="budget-template-change-IMAGE_GENERATION"]')?.textContent).toContain('80')
    expect(document.body.querySelector('[data-cy="budget-template-change-MODEL"]')).toBeNull()
    await (document.body.querySelector('[data-cy="confirm-budget-template-save"]') as HTMLElement).click()
    await flushPromises()
    expect(wrapper.get('[data-cy="budget-template-saved"]').text()).toContain('now live for capabilities without a user override')
    expect(wrapper.get('[data-cy="budget-template-saved"]').text()).toContain('Explicit user overrides were unchanged')
  })

  it('deletes an unassigned template with revision and operator reason', async () => {
    const fetch = vi.spyOn(client, 'apiFetch')
    fetch.mockImplementation(async (path: string, init?: RequestInit) => {
      if (init?.method === 'DELETE') return undefined
      return { items: [{ budgetTemplateId: 'bgt_team', name: 'Team standard', description: '', revision: 3, rules: [], assignedUserCount: 0, createdAt: '2026-09-20T00:00:00Z', updatedAt: '2026-09-20T00:00:00Z' }] }
    })
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-cy="budget-template-row"]').trigger('click')
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Reason for change')!.setValue('Retire unused template')
    await wrapper.findAllComponents(QBtn).find(button => button.props('icon') === 'delete')!.trigger('click')
    expect(fetch.mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(false)
    await (document.body.querySelector('[data-cy="confirm-budget-template-delete"]') as HTMLElement).click()
    await flushPromises()
    const deleteCall = fetch.mock.calls.find(([, init]) => init?.method === 'DELETE')!
    expect(deleteCall[0]).toContain('/api/admin/v1/budget-templates/bgt_team?')
    expect(deleteCall[0]).toContain('expectedRevision=3')
    expect(deleteCall[0]).toContain('reason=Retire+unused+template')
  })

  it('blocks deletion while users are assigned and explains why', async () => {
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-cy="budget-template-row"]').trigger('click')
    const remove = wrapper.findAllComponents(QBtn).find(button => button.props('icon') === 'delete')!
    expect(remove.props('disable')).toBe(true)
    expect(wrapper.text()).toContain('Remove assignments before deleting')
  })

  it('reloads a template for re-edit after a conflict', async () => {
    let detailReads = 0
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string, init?: RequestInit) => {
      if (init?.method === 'PUT') throw new ApiProblem(409, 'budget_template_revision_conflict', 'stale')
      if (path === '/api/admin/v1/budget-templates/bgt_team') {
        detailReads += 1
        return { budgetTemplateId: 'bgt_team', name: 'Server standard', description: 'Current value', revision: 7, rules: [], assignedUserCount: 0, createdAt: '2026-09-20T00:00:00Z', updatedAt: '2026-09-20T02:00:00Z' }
      }
      return { items: [{ budgetTemplateId: 'bgt_team', name: 'Team standard', description: '', revision: 3, rules: [], assignedUserCount: 0, createdAt: '2026-09-20T00:00:00Z', updatedAt: '2026-09-20T00:00:00Z' }] }
    })
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-cy="budget-template-row"]').trigger('click')
    const inputs = wrapper.findAllComponents(QInput)
    await inputs.find(input => input.props('label') === 'Template name')!.setValue('Local edit')
    await inputs.find(input => input.props('label') === 'Reason for change')!.setValue('Update standard')
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'Save')!.trigger('click')
    expect(document.body.textContent).toContain('Explicit user overrides remain unchanged')
    await (document.body.querySelector('[data-cy="confirm-budget-template-save"]') as HTMLElement).click()
    await flushPromises()
    expect(detailReads).toBe(1)
    expect(wrapper.get('[data-cy="budget-template-conflict"]').text()).toContain('reloaded')
    expect(wrapper.text()).toContain('Server standard')
    expect(wrapper.text()).toContain('Revision 7')
  })
})
