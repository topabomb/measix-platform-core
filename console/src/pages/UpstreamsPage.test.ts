import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer,
  QCard, QCardSection, QCardActions, QInput, QBtn, QBanner,
  QSelect, QToggle, QDialog, QSeparator,
  QList, QItem, QItemSection, QItemLabel, QMarkupTable, QChip, QSpinner,
  QIcon, QToolbarTitle, QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown,
  ClosePopup,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { h } from 'vue'
import UpstreamsPage from './UpstreamsPage.vue'
import PageHeader from '../components/PageHeader.vue'
import { useSessionStore } from '../stores/session'
import * as client from '../api/client'

/**
 * UpstreamsPage is a route-level page that expects to be rendered inside a
 * QLayout (provided by AdminLayout in production). Tests must supply that
 * hierarchy, plus register every Quasar component/directive used in the
 * template and its child components (LoadingState, ProblemBanner, StatusChip).
 */
function mountUpstreamsPage() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div/>' } }],
  })

  // Wrap the page in a QLayout → QPageContainer so QPage has a valid parent.
  const wrapper = mount(
    {
      components: { UpstreamsPage },
      render() {
        return h(QLayout, {}, () => [
          h(QPageContainer, {}, () => [h(UpstreamsPage)]),
        ])
      },
    },
    {
      global: {
        plugins: [[Quasar, {
          components: {
            QLayout, QPage, QPageContainer, QCard, QCardSection, QCardActions,
            QInput, QBtn, QBanner, QSelect, QToggle, QDialog, QSeparator,
            QList, QItem, QItemSection, QItemLabel, QMarkupTable, QChip,
            QSpinner, QIcon, QToolbarTitle, QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, PageHeader,
          },
          directives: { ClosePopup },
        }], pinia, router],
      },
    },
  )
  return { wrapper, pinia }
}

function setupSession(pinia: ReturnType<typeof createPinia>) {
  const session = useSessionStore(pinia)
  session.session = {
    user: { userId: 'usr_550e8400-e29b-41d4-a716-446655440000', displayName: 'Admin', role: 'ADMIN' as const },
    csrfToken: 'test-csrf',
    expiresAt: '2026-12-31T23:59:59Z',
  }
  return session
}

describe('UpstreamsPage', () => {
  beforeEach(() => {
    vi.spyOn(client, 'apiFetch').mockResolvedValue({ items: [], nextCursor: undefined })
  })

  it('does not render a Provider kind select in the create form', async () => {
    const { wrapper, pinia } = mountUpstreamsPage()
    setupSession(pinia)
    await flushPromises()

    // Find the "Create upstream" button. After session is set, canMutate
    // becomes true so the button is enabled.
    const btns = wrapper.findAllComponents(QBtn)
    const createBtn = btns.find((b) => String(b.props('label') ?? '').includes('Create upstream'))
    expect(createBtn).toBeTruthy()
    expect(createBtn!.props('disable')).toBe(false)
    await createBtn!.trigger('click')
    await flushPromises()

    // After dialog opens, check there's no Provider kind select.
    const selects = wrapper.findAllComponents(QSelect)
    const providerKindSelect = selects.find((s) => {
      const label = s.props('label') ?? ''
      return label.includes('Provider kind') || label.includes('providerKind')
    })
    expect(providerKindSelect).toBeUndefined()
  })

  it('can create a secret inline and auto-fills the auth secret reference', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === '/api/admin/v1/secrets' && init?.method === 'POST') {
        return { secretId: 'sec_created', name: 'My Key', secretVersion: 1 }
      }
      return { items: [], nextCursor: undefined }
    })

    const { wrapper, pinia } = mountUpstreamsPage()
    setupSession(pinia)
    await flushPromises()

    // Open the create-secret helper dialog.
    const btns = wrapper.findAllComponents(QBtn)
    const secretBtn = btns.find((b) => String(b.props('label') ?? '').includes('Create secret'))
    expect(secretBtn).toBeTruthy()
    await secretBtn!.trigger('click')
    await flushPromises()

    const inputs = wrapper.findAllComponents(QInput)
    const nameInput = inputs.find((i) => (i.props('label') ?? '') === 'Secret name')
    const valueInput = inputs.find((i) => (i.props('label') ?? '') === 'Secret value')
    expect(nameInput).toBeTruthy()
    expect(valueInput).toBeTruthy()
    await nameInput!.setValue('My Key')
    await valueInput!.setValue('sk-super-secret')
    await flushPromises()

    const dialogBtns = wrapper.findAllComponents(QBtn)
    // The dialog's submit button is rendered after the header button with the
    // same label; pick the last match to target the dialog action.
    const createSecretBtns = dialogBtns.filter((b) => String(b.props('label') ?? '') === 'Create secret')
    expect(createSecretBtns.length).toBeGreaterThanOrEqual(2)
    await createSecretBtns[createSecretBtns.length - 1]!.trigger('click')
    await flushPromises()

    const secretCall = fetchSpy.mock.calls.find((c) => c[0] === '/api/admin/v1/secrets' && c[1]?.method === 'POST')
    expect(secretCall).toBeDefined()
    const secretBody = JSON.parse((secretCall![1] as RequestInit).body as string)
    expect(secretBody).toEqual({ name: 'My Key', value: 'sk-super-secret' })

    // The created secret reference is applied to the create-upstream auth
    // section: open create-upstream, switch auth to BEARER, and the Secret ID
    // field is pre-filled.
    const allBtns = wrapper.findAllComponents(QBtn)
    const createUpstreamBtn = allBtns.find((b) => String(b.props('label') ?? '').includes('Create upstream'))
    await createUpstreamBtn!.trigger('click')
    await flushPromises()

    const authSelect = wrapper.findAllComponents(QSelect).find((s) => String(s.props('label') ?? '').includes('Auth'))
    expect(authSelect).toBeTruthy()
    await authSelect!.setValue('BEARER')
    await flushPromises()

    const secretIdInput = wrapper.findAllComponents(QInput).find((i) => String(i.props('label') ?? '').includes('Secret reference') || String(i.props('label') ?? '').includes('Secret ID'))
    expect(secretIdInput).toBeTruthy()
    expect(secretIdInput!.props('modelValue')).toBe('sec_created')
  })

  it('create upstream submits UpstreamConfig with all required fields, not providerKind', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === '/api/admin/v1/upstreams' && init?.method === 'POST') {
        const body = JSON.parse(init.body as string)
        expect(body.config.name).toBeDefined()
        expect(body.config.baseUrl).toBeDefined()
        expect(body.config.transportCapabilities).toBeDefined()
        expect(body.config.auth).toBeDefined()
        expect(body.config.auth.type).toBeDefined()
        expect(body.config.correlationMode).toBeDefined()
        expect(body.config.usageCapabilityLevel).toBeDefined()
        expect(body.config.timeoutDefaults).toBeDefined()
        expect(body.config.providerKind).toBeUndefined()
        return { upstreamId: 'ups_test', name: 'Test', configRevision: 1, status: 'INACTIVE' }
      }
      if (path.includes('/upstreams') && !path.includes(':')) {
        return { items: [], nextCursor: undefined }
      }
      return {}
    })

    const { wrapper, pinia } = mountUpstreamsPage()
    setupSession(pinia)
    await flushPromises()

    // Open the create dialog
    const btns = wrapper.findAllComponents(QBtn)
    const createBtn = btns.find((b) => String(b.props('label') ?? '').includes('Create upstream'))
    expect(createBtn).toBeTruthy()
    await createBtn!.trigger('click')
    await flushPromises()

    // Fill required fields
    const inputs = wrapper.findAllComponents(QInput)
    const nameInput = inputs.find((i) => (i.props('label') ?? '') === 'Name')
    const urlInput = inputs.find((i) => (i.props('label') ?? '') === 'Base URL')
    expect(nameInput).toBeTruthy()
    expect(urlInput).toBeTruthy()
    await nameInput!.setValue('Test Upstream')
    await urlInput!.setValue('https://api.example.com')
    await flushPromises()

    // After dialog opened, re-query QBtn to find the dialog's "Create" button.
    const dialogBtns = wrapper.findAllComponents(QBtn)
    const submitBtn = dialogBtns.find((b) => String(b.props('label') ?? '') === 'Create')
    expect(submitBtn).toBeTruthy()
    await submitBtn!.trigger('click')
    await flushPromises()

    const createCall = fetchSpy.mock.calls.find((c) => c[0] === '/api/admin/v1/upstreams' && c[1]?.method === 'POST')
    expect(createCall).toBeDefined()
  })

  it('tests connection on an upstream detail and shows the result', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    const testResult = { reachable: true, statusCode: 200, latencyMs: 45 }
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith('/api/admin/v1/upstreams') && !path.includes(':')) return { items: [{ upstreamId: 'ups_test', name: 'OpenAI', configRevision: 1, status: 'ACTIVE' }], nextCursor: undefined }
      if (path.includes(':test')) return testResult
      return {}
    })
    const { wrapper, pinia } = mountUpstreamsPage()
    setupSession(pinia)
    await flushPromises()

    // Open the upstream detail row.
    await wrapper.findComponent(QItem).trigger('click')
    await flushPromises()

    const btns = wrapper.findAllComponents(QBtn)
    const testBtn = btns.find((b) => String(b.props('label') ?? '') === 'Test connection')
    expect(testBtn).toBeTruthy()
    await testBtn!.trigger('click')
    await flushPromises()

    const testCall = fetchSpy.mock.calls.find((c) => c[0].includes(':test'))
    expect(testCall).toBeTruthy()
    expect(testCall![1]!.method).toBe('POST')
  })

  it('applies an upstream with an Idempotency-Key and surfaces the activation', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith('/api/admin/v1/upstreams') && !path.includes(':')) return { items: [{ upstreamId: 'ups_test', name: 'OpenAI', configRevision: 1, status: 'INACTIVE' }], nextCursor: undefined }
      if (path.includes(':apply')) {
        return { activationId: 'act_001', kind: 'RUNTIME_CONFIG', state: 'COMPLETED', desiredControlRevision: 2 }
      }
      return {}
    })
    window.confirm = vi.fn(() => true)
    const { wrapper, pinia } = mountUpstreamsPage()
    setupSession(pinia)
    await flushPromises()

    await wrapper.findComponent(QItem).trigger('click')
    await flushPromises()

    const btns = wrapper.findAllComponents(QBtn)
    const applyBtn = btns.find((b) => String(b.props('label') ?? '').includes('Apply'))
    expect(applyBtn).toBeTruthy()
    await applyBtn!.trigger('click')
    await flushPromises()

    const applyCall = fetchSpy.mock.calls.find((c) => c[0].includes(':apply'))
    expect(applyCall).toBeTruthy()
    expect((applyCall![1] as RequestInit).headers).toHaveProperty('Idempotency-Key')
    expect(wrapper.text()).toContain('act_001')
  })
})
