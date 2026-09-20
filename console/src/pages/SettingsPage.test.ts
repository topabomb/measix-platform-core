import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import {
  Quasar, QPage, QCard, QCardSection, QCardActions, QSeparator, QInput,
  QBtn, QBanner, QList, QItem, QItemSection, QItemLabel, QIcon, QDialog,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import SettingsPage from './SettingsPage.vue'
import { useSessionStore } from '../stores/session'
import * as client from '../api/client'
import en from '../i18n/locales/en'

function mountPage() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useSessionStore(pinia).session = {
    user: { userId: 'usr_admin', displayName: 'Admin', role: 'ADMIN' },
    csrfToken: 'csrf-settings',
    expiresAt: '2026-12-31T23:59:59Z',
  }
  const wrapper = mount(SettingsPage, {
    global: {
      plugins: [[Quasar, { components: {
        QPage, QCard, QCardSection, QCardActions, QSeparator, QInput,
        QBtn, QBanner, QList, QItem, QItemSection, QItemLabel, QIcon, QDialog,
      } }], pinia],
      stubs: {
        QPage: { template: '<div><slot /></div>' },
        PageHeader: true,
        LoadingState: true,
        ProblemBanner: true,
        RouterLink: true,
      },
    },
  })
  return wrapper
}

describe('SettingsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/deployment/settings') {
        return { deploymentId: 'dep_1', name: 'MEASIX', timezone: 'Asia/Shanghai', publicOrigin: 'https://core.example', updatedAt: '2026-09-20T00:00:00Z' }
      }
      return { buildVersion: 'dev', dbHealth: 'OK', schemaIdentity: `sha256:${'0'.repeat(64)}`, runtimeStatus: 'READY', activeManagedGeneration: 1, managedStateRevision: 1, desiredControlRevision: 1, relayReady: true, portalMode: 'STANDARD', publicOrigin: 'https://core.example' }
    })
  })

  it('separates the editable enterprise identity from protected runtime settings', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.findAllComponents(QInput).map(input => input.props('modelValue'))).toEqual(['MEASIX', 'https://core.example'])
    expect(wrapper.text()).toContain('Asia/Shanghai')
    expect(wrapper.text()).toContain('https://core.example')
    expect(wrapper.text()).toContain('deployment-owned')
  })

  it('saves the trimmed name with optimistic revision and CSRF', async () => {
    const fetchSpy = vi.mocked(client.apiFetch)
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === '/api/admin/v1/deployment/settings' && init?.method === 'PUT') {
        return { deploymentId: 'dep_1', name: 'New enterprise', timezone: 'Asia/Shanghai', publicOrigin: 'https://core.example', updatedAt: '2026-09-20T01:00:00Z' }
      }
      if (path === '/api/admin/v1/deployment/settings') return { deploymentId: 'dep_1', name: 'MEASIX', timezone: 'Asia/Shanghai', publicOrigin: 'https://core.example', updatedAt: '2026-09-20T00:00:00Z' }
      return { buildVersion: 'dev', dbHealth: 'OK', schemaIdentity: `sha256:${'0'.repeat(64)}`, runtimeStatus: 'READY', activeManagedGeneration: 1, managedStateRevision: 1, desiredControlRevision: 1, relayReady: true, portalMode: 'STANDARD' }
    })
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.findAllComponents(QInput)[0]!.setValue('  New enterprise  ')
    await wrapper.get('[data-cy="settings-save"]').trigger('click')
    await flushPromises()

    const call = fetchSpy.mock.calls.find(entry => entry[0] === '/api/admin/v1/deployment/settings' && (entry[1] as RequestInit)?.method === 'PUT')
    expect(JSON.parse((call![1] as RequestInit).body as string)).toEqual({ expectedUpdatedAt: '2026-09-20T00:00:00Z', name: 'New enterprise', publicOrigin: 'https://core.example' })
    expect(call![2]).toBe('csrf-settings')
    expect(wrapper.find('[data-cy="settings-saved"]').exists()).toBe(true)
  })

  it('requires an impact confirmation before changing the public origin', async () => {
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.findAllComponents(QInput)[1]!.setValue('https://new-core.example/')
    await wrapper.get('[data-cy="settings-save"]').trigger('click')
    await flushPromises()

    expect(wrapper.findComponent(QDialog).props('modelValue')).toBe(true)
    expect(wrapper.text()).toContain('Enterprise address')
    expect(en.settings.confirmOriginClients).toContain('identity, user, device and Android session stay unchanged')
    expect(en.settings.confirmOriginClients).not.toContain('not migrated')
    expect(en.settings.confirmOriginClients).not.toContain('re-enroll')
    expect(vi.mocked(client.apiFetch).mock.calls.filter(entry => (entry[1] as RequestInit | undefined)?.method === 'PUT')).toHaveLength(0)
  })
})
