import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { h } from 'vue'
import { Quasar, QLayout, QPageContainer, QPage, QCard, QCardSection, QCardActions,
  QInput, QSelect, QBtn, QBanner, QDialog, QSeparator, QList, QItem, QItemSection,
  QItemLabel, QBadge, QChip, QSpinner, QIcon, QToolbarTitle, QBreadcrumbs,
  QBreadcrumbsEl, QBtnDropdown, ClosePopup } from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import EnterpriseUpdatesPage from './EnterpriseUpdatesPage.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusChip from '../components/StatusChip.vue'
import { useSessionStore } from '../stores/session'
import * as client from '../api/client'

function mountUpdates() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div/>' } }] })
  const wrapper = mount({
    render: () => h(QLayout, {}, () => [h(QPageContainer, {}, () => [h(EnterpriseUpdatesPage)])]),
  }, {
    global: { plugins: [[Quasar, { components: {
      QLayout, QPageContainer, QPage, QCard, QCardSection, QCardActions, QInput,
      QSelect, QBtn, QBanner, QDialog, QSeparator, QList, QItem, QItemSection,
      QItemLabel, QBadge, QChip, QSpinner, QIcon, QToolbarTitle, QBreadcrumbs,
      QBreadcrumbsEl, QBtnDropdown, PageHeader, StatusChip,
    }, directives: { ClosePopup } }], pinia, router] },
  })
  useSessionStore(pinia).session = {
    user: { userId: 'usr_001', displayName: 'Admin', role: 'ADMIN' },
    csrfToken: 'test-csrf', expiresAt: '2026-12-31T23:59:59Z',
  }
  return wrapper
}

describe('EnterpriseUpdatesPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(client, 'apiFetch').mockResolvedValue({ items: [], feedRevision: 0 })
  })

  it('uses understandable option labels while retaining protocol values', async () => {
    const fetchSpy = vi.mocked(client.apiFetch)
    const wrapper = mountUpdates()
    await flushPromises()
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'Create')!.trigger('click')
    await flushPromises()
    const selects = wrapper.findAllComponents(QSelect)
    const format = selects.find(select => select.props('label') === 'Format')!
    const category = selects.find(select => select.props('label') === 'Category')!
    const severity = selects.find(select => select.props('label') === 'Severity')!
    expect(format.props('options')).toContainEqual({ label: 'Plain text', value: 'PLAIN' })
    expect(category.props('options')).toContainEqual({ label: 'Notice', value: 'NOTICE' })
    expect(severity.props('options')).toContainEqual({ label: 'Information', value: 'INFO' })
    await format.setValue('MARKDOWN')
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Title')!.setValue('Example')
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Content')!.setValue('Hello')
    await wrapper.findAllComponents(QBtn).filter(button => button.props('label') === 'Create').at(-1)!.trigger('click')
    await flushPromises()
    const call = fetchSpy.mock.calls.find(([path, init]) => path === '/api/admin/v1/enterprise-updates' && init?.method === 'POST')
    expect(JSON.parse(call![1]!.body as string).contentFormat).toBe('MARKDOWN')
    wrapper.unmount()
  })

  it('renders announcement summaries as safe formatted content and a localized status', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue({ items: [{
      enterpriseUpdateId: 'eup_1', title: 'Safety update', content: '**Important** update',
      contentFormat: 'MARKDOWN', category: 'NOTICE', severity: 'INFO', status: 'PUBLISHED',
      createdAt: '2026-09-18T00:00:00Z',
    }], feedRevision: 1 })
    const wrapper = mountUpdates()
    await flushPromises()
    expect(wrapper.get('[data-cy="update-list-preview"]').find('strong').text()).toBe('Important')
    expect(wrapper.get('details').element).not.toHaveProperty('open', true)
    expect(wrapper.text()).not.toContain('eup_1')
    await wrapper.findComponent(QItem).trigger('click')
    await flushPromises()
    expect(document.querySelector('.q-dialog')?.textContent).toContain('Published')
    wrapper.unmount()
  })
})
