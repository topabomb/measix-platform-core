import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { h } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { Quasar, QLayout, QPageContainer, QPage, QCard, QCardSection, QCardActions, QToggle, QInput, QCheckbox, QBtn, QBanner, QSeparator, QBadge, QList, QItem, QItemSection, QItemLabel, QIcon, QDialog, ClosePopup } from 'quasar'
import RemoteWorkspacesPage from './RemoteWorkspacesPage.vue'
import { useSessionStore } from '../stores/session'
import * as client from '../api/client'
import type { components } from '../api/generated'

type Service = components['schemas']['WorkspaceService']
let service: Service | undefined
let operationAction = ''
const configured = (): Service => ({ workspaceServiceId: 'wss_test', type: 'AGENT_SPACE', name: 'Remote workspace', configRevision: 1, activeConfigRevision: 1, enabled: true, state: 'ACTIVE', mcpServerId: 'mcp_test', mcpPublished: false, config: { adminOrigin: 'http://space.test', mcpOrigin: 'http://space.test', managementSecret: { secretId: 'sec_test', secretVersion: 1 }, releaseIdentity: '3ea01c167fb263f8ef2467b5fe3103353f9a5ddc', connectTimeoutMs: 90000, idleTimeoutMs: 120000 } })
function mountPage() {
  const pinia = createPinia(); setActivePinia(pinia)
  useSessionStore(pinia).session = { user: { userId: 'usr_admin', displayName: 'Admin', role: 'ADMIN' }, csrfToken: 'csrf', expiresAt: '2026-12-31T00:00:00Z' }
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: RemoteWorkspacesPage }] })
  return mount({ render: () => h(QLayout, {}, () => h(QPageContainer, {}, () => h(RemoteWorkspacesPage))) }, {
    global: { plugins: [pinia, router, [Quasar, { components: { QLayout, QPageContainer, QPage, QCard, QCardSection, QCardActions, QToggle, QInput, QCheckbox, QBtn, QBanner, QSeparator, QBadge, QList, QItem, QItemSection, QItemLabel, QIcon, QDialog }, directives: { ClosePopup } }]], stubs: { WorkspacePanel: true, PagedEntityPicker: true } },
  })
}
function button(wrapper: ReturnType<typeof mountPage>, label: string) { return wrapper.findAllComponents(QBtn).find(item => item.props('label') === label)! }

beforeEach(() => {
  vi.useFakeTimers(); service = undefined; operationAction = ''; vi.restoreAllMocks()
  vi.spyOn(client, 'apiFetch').mockImplementation(async (path, init) => {
    if (path.endsWith('/workspace-operations/wop_test')) {
      service = { ...service!, state: operationAction === 'apply' ? 'ACTIVE' : 'DISABLED', enabled: operationAction === 'apply', activeConfigRevision: service!.configRevision }
      return { operationId: 'wop_test', state: 'COMPLETED', step: 'DONE' } as never
    }
    if (path.endsWith('/apply') || path.endsWith('/disable')) {
      operationAction = path.endsWith('/apply') ? 'apply' : 'disable'
      service = { ...service!, enabled: operationAction === 'apply', state: operationAction === 'apply' ? 'APPLYING' : 'DISABLING' }
      return { operationId: 'wop_test', state: 'PENDING', step: 'START' } as never
    }
    if (path === '/api/admin/v1/secrets') return { secretId: 'sec_test', secretVersion: 1 } as never
    if (path.endsWith('/workspaces')) return { items: [] } as never
    if (path === '/api/admin/v1/remote-workspace/services' && !init?.method) return { items: service ? [service] : [] } as never
    if (init?.method === 'POST' || init?.method === 'PUT') { const body = JSON.parse(init.body as string); service = { ...configured(), ...body, enabled: false, state: 'SAVED', activeConfigRevision: undefined }; return service as never }
    throw new Error('Unexpected API ' + path)
  })
})
afterEach(() => vi.useRealTimers())

describe('remote workspace administrator flow', () => {
  it('requires confirmation before saving a changed service origin', async () => {
    service = configured(); const wrapper = mountPage(); await flushPromises()
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === '文件服务地址（可选）')!.setValue('http://dav.test')
    expect(button(wrapper, '保存配置').props('disable')).toBe(true)
    await wrapper.findAllComponents(QCheckbox).find(input => input.props('label') === '新地址仍指向原服务和原有空间')!.setValue(true)
    expect(button(wrapper, '保存配置').props('disable')).toBe(false)
    wrapper.unmount()
  })
  it('the switch only expresses intent; saving enables service without publishing MCP', async () => {
    const wrapper = mountPage(); await flushPromises()
    expect(button(wrapper, '保存配置').props('disable')).toBe(true)
    await wrapper.findComponent(QToggle).setValue(true)
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === '管理服务地址')!.setValue('http://space.test')
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === '管理凭据')!.setValue('management')
    await wrapper.findComponent(QCheckbox).setValue(true)
    expect(vi.mocked(client.apiFetch).mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
    await button(wrapper, '保存配置').trigger('click'); await flushPromises()
    expect(operationAction).toBe('apply')
    expect(wrapper.text()).toContain('正在启用')
    await vi.advanceTimersByTimeAsync(1000); await flushPromises()
    expect(wrapper.text()).toContain('现在可以开通用户工作区')
    expect(vi.mocked(client.apiFetch).mock.calls.some(([path]) => path.includes('draft') || path.includes('publish'))).toBe(false)
    wrapper.unmount()
  })
  it('closing requires saving and confirming, preserving the stored connection', async () => {
    service = configured(); const wrapper = mountPage(); await flushPromises()
    await wrapper.findComponent(QToggle).setValue(false)
    expect(operationAction).toBe('')
    await button(wrapper, '保存配置').trigger('click'); await flushPromises()
    expect(operationAction).toBe('')
    await button(wrapper, '保存并关闭').trigger('click'); await flushPromises()
    expect(operationAction).toBe('disable')
    expect(vi.mocked(client.apiFetch).mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
    await vi.advanceTimersByTimeAsync(1000); await flushPromises()
    expect(wrapper.text()).toContain('已有空间和文件保留')
    wrapper.unmount()
  })
})
