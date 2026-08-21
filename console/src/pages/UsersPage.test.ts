import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer,
  QCard, QCardSection, QCardActions, QInput, QBtn, QBanner,
  QSelect, QDialog, QSeparator, QList, QItem, QItemSection, QItemLabel,
  QChip, QSpinner, QIcon, QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown,
  ClosePopup,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { h } from 'vue'
import UsersPage from './UsersPage.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusChip from '../components/StatusChip.vue'
import { useSessionStore } from '../stores/session'
import * as client from '../api/client'

// Mock qrcode — jsdom does not implement canvas getContext('2d')
vi.mock('qrcode', () => ({
  default: {
    toCanvas: vi.fn().mockResolvedValue(undefined),
  },
}))

function mountUsersPage() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div/>' } }],
  })
  const wrapper = mount(
    {
      components: { UsersPage },
      render() {
        return h(QLayout, {}, () => [
          h(QPageContainer, {}, () => [h(UsersPage)]),
        ])
      },
    },
    {
      global: {
        plugins: [[Quasar, {
          components: {
            QLayout, QPage, QPageContainer, QCard, QCardSection, QCardActions,
            QInput, QBtn, QBanner, QSelect, QDialog, QSeparator, QList, QItem,
            QItemSection, QItemLabel, QChip, QSpinner, QIcon, PageHeader, StatusChip,
            QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown,
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
    user: { userId: 'usr_001', displayName: 'Admin', role: 'ADMIN' as const },
    csrfToken: 'test-csrf',
    expiresAt: '2026-12-31T23:59:59Z',
  }
  return session
}

describe('UsersPage', () => {
  beforeEach(() => {
    vi.spyOn(client, 'apiFetch').mockResolvedValue({ items: [], nextCursor: undefined })
  })

  it('renders a user list with display name, role and status', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/users')) {
        return {
          items: [
            { userId: 'usr_001', username: 'admin', displayName: 'Admin User', role: 'ADMIN', status: 'ACTIVE' },
            { userId: 'usr_002', username: 'member', displayName: 'Member User', role: 'MEMBER', status: 'DISABLED' },
          ],
          nextCursor: undefined,
        }
      }
      return { items: [], nextCursor: undefined }
    })

    const { wrapper, pinia } = mountUsersPage()
    setupSession(pinia)
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('Admin User')
    expect(text).toContain('Member User')
    expect(text).toContain('ADMIN')
    expect(text).toContain('MEMBER')
  })

  it('opens create user dialog with username, display name and role fields', async () => {
    const { wrapper, pinia } = mountUsersPage()
    setupSession(pinia)
    await flushPromises()

    const createBtn = wrapper.findAllComponents(QBtn).find((b) => String(b.props('label') ?? '').includes('Create user'))
    expect(createBtn).toBeTruthy()
    await createBtn!.trigger('click')
    await flushPromises()

    const body = document.body.innerHTML
    expect(body).toContain('Username')
    expect(body).toContain('Display name')
  })

  it('creates a user via POST with username, displayName and role', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === '/api/admin/v1/users' && init?.method === 'POST') {
        return { userId: 'usr_new', username: 'newuser', displayName: 'New User', role: 'MEMBER', status: 'ACTIVE' }
      }
      if (path.startsWith('/api/admin/v1/users')) return { items: [], nextCursor: undefined }
      return { items: [], nextCursor: undefined }
    })

    const { wrapper, pinia } = mountUsersPage()
    setupSession(pinia)
    await flushPromises()

    const createBtn = wrapper.findAllComponents(QBtn).find((b) => String(b.props('label') ?? '').includes('Create user'))
    await createBtn!.trigger('click')
    await flushPromises()

    const inputs = wrapper.findAllComponents(QInput)
    const usernameInput = inputs.find((i) => (i.props('label') ?? '') === 'Username')
    const displayNameInput = inputs.find((i) => (i.props('label') ?? '') === 'Display name')
    expect(usernameInput).toBeTruthy()
    expect(displayNameInput).toBeTruthy()
    await usernameInput!.setValue('newuser')
    await displayNameInput!.setValue('New User')
    await flushPromises()

    const submitBtn = wrapper.findAllComponents(QBtn).find((b) => String(b.props('label') ?? '') === 'Create')
    expect(submitBtn).toBeTruthy()
    await submitBtn!.trigger('click')
    await flushPromises()

    const createCall = fetchSpy.mock.calls.find((c) => c[0] === '/api/admin/v1/users' && c[1]?.method === 'POST')
    expect(createCall).toBeDefined()
    const body = JSON.parse((createCall![1] as RequestInit).body as string)
    expect(body.username).toBe('newuser')
    expect(body.displayName).toBe('New User')
    expect(body.role).toBeDefined()
  })

  it('generates an enrollment code and shows it in a dialog with copy button', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.startsWith('/api/admin/v1/users') && path.includes('/enrollments') && init?.method === 'POST') {
        return { code: 'ENROLL-CODE-12345', expiresAt: '2026-12-31T23:59:59Z' }
      }
      if (path.startsWith('/api/admin/v1/users')) {
        return {
          items: [{ userId: 'usr_001', username: 'admin', displayName: 'Admin User', role: 'ADMIN', status: 'ACTIVE' }],
          nextCursor: undefined,
        }
      }
      if (path.includes('/devices')) return { items: [], nextCursor: undefined }
      return { items: [], nextCursor: undefined }
    })

    const { wrapper, pinia } = mountUsersPage()
    setupSession(pinia)
    await flushPromises()

    // Open user detail — the dialog is teleported to document.body
    const userRow = wrapper.findComponent(QItem)
    expect(userRow.exists()).toBe(true)
    await userRow.trigger('click')
    await flushPromises()

    // Click generate enrollment — the button is inside the teleported dialog
    const enrollBtn = [...document.querySelectorAll('button')].find((b) =>
      (b.textContent ?? '').includes('enrollment') || (b.textContent ?? '').includes('Enrollment'),
    )
    expect(enrollBtn).toBeTruthy()
    enrollBtn!.click()
    await flushPromises()

    const enrollCall = fetchSpy.mock.calls.find((c) => c[0].includes('/enrollments') && c[1]?.method === 'POST')
    expect(enrollCall).toBeDefined()

    const body = document.body.innerHTML
    expect(body).toContain('ENROLL-CODE-12345')
    // Copy button should be present
    expect(body).toContain('content_copy')
  })

  it('shows enrollment code expiry time', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.includes('/enrollments') && init?.method === 'POST') {
        return { code: 'ENROLL-EXPIRY', expiresAt: '2026-12-31T23:59:59Z' }
      }
      if (path.startsWith('/api/admin/v1/users') && !path.includes('/devices')) {
        return {
          items: [{ userId: 'usr_001', username: 'admin', displayName: 'Admin', role: 'ADMIN', status: 'ACTIVE' }],
          nextCursor: undefined,
        }
      }
      return { items: [], nextCursor: undefined }
    })

    const { wrapper, pinia } = mountUsersPage()
    setupSession(pinia)
    await flushPromises()

    await wrapper.findComponent(QItem).trigger('click')
    await flushPromises()

    // The enrollment button is inside the teleported detail dialog
    const enrollBtn = [...document.querySelectorAll('button')].find((b) =>
      (b.textContent ?? '').includes('enrollment') || (b.textContent ?? '').includes('Enrollment'),
    )
    expect(enrollBtn).toBeTruthy()
    enrollBtn!.click()
    await flushPromises()

    const body = document.body.innerHTML
    expect(body).toContain('2026-12-31T23:59:59Z')
  })

  it('renders a QR code canvas in the enrollment dialog', async () => {
    const QRCode = (await import('qrcode')).default
    const toCanvasSpy = vi.spyOn(QRCode, 'toCanvas')

    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.includes('/enrollments') && init?.method === 'POST') {
        return { code: 'ENROLL-QR-CODE-67890', expiresAt: '2026-12-31T23:59:59Z' }
      }
      if (path.startsWith('/api/admin/v1/users') && !path.includes('/devices')) {
        return {
          items: [{ userId: 'usr_001', username: 'admin', displayName: 'Admin User', role: 'ADMIN', status: 'ACTIVE' }],
          nextCursor: undefined,
        }
      }
      return { items: [], nextCursor: undefined }
    })

    const { wrapper, pinia } = mountUsersPage()
    setupSession(pinia)
    await flushPromises()

    await wrapper.findComponent(QItem).trigger('click')
    await flushPromises()

    const enrollBtn = [...document.querySelectorAll('button')].find((b) =>
      (b.textContent ?? '').includes('enrollment') || (b.textContent ?? '').includes('Enrollment'),
    )
    expect(enrollBtn).toBeTruthy()
    enrollBtn!.click()
    await flushPromises()

    // QRCode.toCanvas should have been called with the enrollment code
    expect(toCanvasSpy).toHaveBeenCalled()
    const callArgs = toCanvasSpy.mock.calls.at(-1)
    expect(callArgs?.[1]).toBe('ENROLL-QR-CODE-67890')

    // The canvas element with data-cy should be present in the dialog
    const qrCanvas = document.querySelector('[data-cy="enrollment-qr"]')
    expect(qrCanvas).toBeTruthy()
  })

  it('shows enrollment code with one-time warning hint and copy button', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path.includes('/enrollments') && init?.method === 'POST') {
        return { code: 'ENROLL-ONE-TIME-XYZ', expiresAt: '2026-12-31T23:59:59Z' }
      }
      if (path.startsWith('/api/admin/v1/users') && !path.includes('/devices')) {
        return {
          items: [{ userId: 'usr_001', username: 'admin', displayName: 'Admin', role: 'ADMIN', status: 'ACTIVE' }],
          nextCursor: undefined,
        }
      }
      return { items: [], nextCursor: undefined }
    })

    const { wrapper, pinia } = mountUsersPage()
    setupSession(pinia)
    await flushPromises()

    await wrapper.findComponent(QItem).trigger('click')
    await flushPromises()

    // The enrollment button is inside the teleported detail dialog
    const enrollBtn = [...document.querySelectorAll('button')].find((b) =>
      (b.textContent ?? '').includes('enrollment') || (b.textContent ?? '').includes('Enrollment'),
    )
    expect(enrollBtn).toBeTruthy()
    enrollBtn!.click()
    await flushPromises()

    const body = document.body.innerHTML
    // The one-time code is shown
    expect(body).toContain('ENROLL-ONE-TIME-XYZ')
    // The one-time warning hint should be present
    expect(body.toLowerCase()).toContain('shown once')
    // Copy button with icon is present
    expect(body).toContain('content_copy')
    // Expiry time is shown
    expect(body).toContain('2026-12-31T23:59:59Z')
  })

  it('lists devices for a selected user with status and revoke button', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string) => {
      if (path.startsWith('/api/admin/v1/users') && !path.includes('/devices') && !path.includes('/enrollments')) {
        return {
          items: [{ userId: 'usr_001', username: 'admin', displayName: 'Admin', role: 'ADMIN', status: 'ACTIVE' }],
          nextCursor: undefined,
        }
      }
      if (path.includes('/devices')) {
        return {
          items: [
            { deviceId: 'dev_001', status: 'ACTIVE', appVersion: '1.0.0', lastSeenAt: '2026-08-01T00:00:00Z' },
            { deviceId: 'dev_002', status: 'REVOKED', appVersion: '0.9.0', lastSeenAt: '2026-07-01T00:00:00Z' },
          ],
          nextCursor: undefined,
        }
      }
      return { items: [], nextCursor: undefined }
    })

    const { wrapper, pinia } = mountUsersPage()
    setupSession(pinia)
    await flushPromises()

    await wrapper.findComponent(QItem).trigger('click')
    await flushPromises()

    const body = document.body.innerHTML
    expect(body).toContain('dev_001')
    expect(body).toContain('dev_002')
    // StatusChip renders status via i18n ("Active" / "Revoked")
    expect(body.toLowerCase()).toContain('active')
    expect(body.toLowerCase()).toContain('revoked')
    // Revoke button should be present for non-revoked devices
    expect(body).toContain('Revoke')
  })

  it('shows empty state when no users exist', async () => {
    vi.spyOn(client, 'apiFetch').mockResolvedValue({ items: [], nextCursor: undefined })
    const { wrapper, pinia } = mountUsersPage()
    setupSession(pinia)
    await flushPromises()

    expect(wrapper.text()).toContain('No enterprise users')
  })

  it('disables create user button when not authenticated', async () => {
    const { wrapper } = mountUsersPage()
    // No session setup - canMutate should be false
    await flushPromises()

    const createBtn = wrapper.findAllComponents(QBtn).find((b) => String(b.props('label') ?? '').includes('Create user'))
    expect(createBtn).toBeTruthy()
    expect(createBtn!.props('disable')).toBe(true)
  })
})
