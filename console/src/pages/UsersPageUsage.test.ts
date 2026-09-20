import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer, QCard, QCardSection, QCardActions,
  QInput, QBtn, QBanner, QSelect, QDialog, QSeparator, QList, QItem,
  QItemSection, QItemLabel, QChip, QSpinner, QIcon, QMarkupTable, ClosePopup,
  QBtnDropdown, QTab, QTabs, QBreadcrumbs, QBreadcrumbsEl,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { h } from 'vue'
import UsersPage from './UsersPage.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusChip from '../components/StatusChip.vue'
import * as client from '../api/client'

vi.mock('qrcode', () => ({ default: { toCanvas: vi.fn().mockResolvedValue(undefined) } }))

const userA = { userId: 'usr_A', username: 'ana.ops', displayName: 'Ana Ruiz', role: 'MEMBER', status: 'ACTIVE' }
const userB = { userId: 'usr_B', username: 'ben.ops', displayName: 'Ben Okafor', role: 'MEMBER', status: 'ACTIVE' }

function device(id: string, name: string) {
  return { deviceId: id, deviceName: name, userId: userA.userId, status: 'ACTIVE', applicationState: 'UNKNOWN', targetManagedGeneration: 1 }
}

function mountUsersPage() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div/>' } }] })
  const wrapper = mount(
    {
      components: { UsersPage },
      render() { return h(QLayout, {}, () => [h(QPageContainer, {}, () => [h(UsersPage)])]) },
    },
    {
      global: {
        plugins: [[Quasar, {
          components: {
            QLayout, QPage, QPageContainer, QCard, QCardSection, QCardActions, QInput, QBtn,
            QBanner, QSelect, QDialog, QSeparator, QList, QItem, QItemSection, QItemLabel,
            QChip, QSpinner, QIcon, QMarkupTable, PageHeader, StatusChip,
            QBtnDropdown, QTab, QTabs, QBreadcrumbs, QBreadcrumbsEl,
          },
          directives: { ClosePopup },
        }], pinia, router],
      },
    },
  )
  return { wrapper }
}

const summary = {
  requestCount: 7, forwardedRequestCount: 5, requestBytes: 1024, responseBytes: 2048,
  requestCompleteness: { exact: 7, partial: 0, unknown: 0 }, semanticMeters: [],
  cost: { status: 'UNKNOWN' },
}

describe('UsersPage per-user usage and list safety', () => {
  beforeEach(() => { vi.restoreAllMocks() })

  it('loads the usage of the opened user, filtered by that user and an explicit period', async () => {
    const paths: string[] = []
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      paths.push(path)
      if (path.startsWith('/api/admin/v1/users?')) return { items: [userA], nextCursor: undefined }
      if (path.includes('/devices')) return { items: [device('dev_A', 'Ana phone')], nextCursor: undefined }
      if (path.includes('/usage/summary')) return summary
      if (path.includes('/usage/requests')) return { items: [], nextCursor: undefined }
      return {}
    })
    const { wrapper } = mountUsersPage()
    await flushPromises()
    await wrapper.get('[data-cy="user-row"]').trigger('click')
    await flushPromises()
    await wrapper.findAllComponents(QTab).find(tab => tab.props('name') === 'usage')!.trigger('click')
    await flushPromises()

    const summaryPath = paths.find(path => path.includes('/usage/summary'))
    expect(summaryPath, 'the usage section must ask for this user\'s usage').toBeTruthy()
    expect(summaryPath).toContain('userId=usr_A')
    // An explicit window keeps the aggregate bounded; unbounded it would read the
    // whole history on every open.
    expect(summaryPath).toContain('from=')
    expect(summaryPath).toContain('to=')

    const requestsPath = paths.find(path => path.includes('/usage/requests'))
    expect(requestsPath).toContain('userId=usr_A')
    expect(wrapper.text()).toContain('Ana Ruiz')
  })

  it('never shows one user\'s devices under another user\'s name', async () => {
    let releaseA!: (value: unknown) => void
    let releaseB!: (value: unknown) => void
    const devicesA = new Promise(resolve => { releaseA = resolve })
    const devicesB = new Promise(resolve => { releaseB = resolve })
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/users?')) return { items: [userA, userB], nextCursor: undefined }
      if (path.includes('/devices')) return path.includes('usr_A') ? devicesA : devicesB
      if (path.includes('/usage/summary')) return summary
      if (path.includes('/usage/requests')) return { items: [], nextCursor: undefined }
      return {}
    })
    const { wrapper } = mountUsersPage()
    await flushPromises()

    // Open A (its device response stays in flight), then open B.
    await wrapper.findAll('[data-cy="user-row"]')[0].trigger('click')
    await flushPromises()
    await wrapper.findAll('[data-cy="user-row"]')[1].trigger('click')
    await flushPromises()

    releaseB({ items: [device('dev_B', 'Ben phone')], nextCursor: undefined })
    await flushPromises()
    expect(wrapper.get('[data-cy="user-devices"]').text()).toContain('Ben phone')

    // A's late response resolves last and must still be discarded.
    releaseA({ items: [device('dev_A', 'Ana phone')], nextCursor: undefined })
    await flushPromises()
    const shown = wrapper.get('[data-cy="user-devices"]').text()
    expect(shown).toContain('Ben phone')
    expect(shown).not.toContain('Ana phone')
  })

  it('sends the search term instead of making the operator page through every user', async () => {
    const paths: string[] = []
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      paths.push(path)
      if (path.startsWith('/api/admin/v1/users?')) return { items: [userA, userB], nextCursor: undefined }
      return { items: [], nextCursor: undefined }
    })
    const { wrapper } = mountUsersPage()
    await flushPromises()
    const field = wrapper.get('[data-cy="user-search"]')
    const input = field.element.tagName === 'INPUT' ? field : field.get('input')
    await input.setValue('ana')
    // The field is debounced so a request is not issued per keystroke.
    expect(paths.some(path => path.includes('query=ana'))).toBe(false)
    await new Promise(resolve => setTimeout(resolve, 350))
    await flushPromises()
    expect(paths.some(path => path.includes('query=ana'))).toBe(true)
  })
})
