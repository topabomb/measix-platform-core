import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { Quasar, QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog, QSpinner, ClosePopup } from 'quasar'
import WorkspacePanel from './WorkspacePanel.vue'
import * as client from '../api/client'
import { useSessionStore } from '../stores/session'
import { useRemoteWorkspaceStore, type WorkspaceService } from '../stores/remoteWorkspace'
import type { components } from '../api/generated'

afterEach(() => vi.restoreAllMocks())

describe('workspace MCP and desktop credentials', () => {
  function mountPanel(service?: WorkspaceService) {
    const pinia = createPinia()
    useRemoteWorkspaceStore(pinia).current = service
    useSessionStore(pinia).session = { csrfToken: 'test-csrf' } as components['schemas']['AdminSession']
    return mount(WorkspacePanel, {
      props: { userId: 'usr_test', serviceEnabled: true },
      global: { plugins: [pinia, [Quasar, { components: { QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog, QSpinner }, directives: { ClosePopup } }]], stubs: { WorkspaceResources: true, WorkspaceFiles: true, ProblemBanner: true, QDialog: { props: ['modelValue'], template: '<div v-if="modelValue"><slot /></div>' } } },
    })
  }
  const projection = { schemaVersion: 1, state: 'CONNECTED', serviceState: 'ENABLED', bindingRevision: 1, agentSpaceId: 'spc_test', mcpAvailable: false, mcpReason: 'mcp_not_published', filesAvailable: false }
  const credentials = { mcpUrl: 'https://space.test/u/test/mcp', username: 'test', token: 'key_fixture.secret' }
  const desktopService = (origin: string): WorkspaceService => ({ workspaceServiceId: 'wss_test', type: 'AGENT_SPACE', name: 'Space', configRevision: 1, activeConfigRevision: 1, enabled: true, state: 'ACTIVE', mcpServerId: 'mcp_test', mcpPublished: false, config: { adminOrigin: origin, mcpOrigin: origin, managementSecret: { secretId: 'sec_test', secretVersion: 1 }, connectTimeoutMs: 90000, idleTimeoutMs: 120000 } })
  it.each(['https://space.example:8443', 'http://127.0.0.1:18932'])('opens the actual remote account from any panel entry and list page for %s', async origin => {
    const fetch = vi.spyOn(client, 'apiFetch').mockImplementation(async path => {
      if (path.includes('/workspaces')) return (path.includes('cursor=next')
        ? { items: [{ userId: 'usr_test', remoteUsername: 'adopted-alice', workspace: projection }] }
        : { items: [{ userId: 'usr_other', remoteUsername: 'wrong', workspace: projection }], nextCursor: 'next' }) as never
      return projection as never
    })
    const wrapper = mountPanel(desktopService(origin))
    try {
      await flushPromises()
      const button = wrapper.findAllComponents(QBtn).find(button => button.props('label') === '云桌面')
      expect(button).toBeDefined()
      const link = button!.get('a')
      const address = new URL(link.attributes('href')!)
      expect(address.hostname).toBe(origin.includes('127.0.0.1') ? window.location.hostname : 'space.example')
      expect(address.port).toBe(origin.includes('127.0.0.1') ? '18932' : '8443')
      expect(address.pathname).toBe('/u/adopted-alice/desktop/')
      expect(address.search + address.hash).toBe('')
      expect(link.attributes('target')).toBe('_blank')
      expect(link.attributes('rel')).toBe('noopener noreferrer')
      expect(fetch.mock.calls.some(([path, init]) => path.includes('connection') || init?.method === 'POST')).toBe(false)
    } finally { wrapper.unmount() }
  })
  it.each(['user', 'binding', 'config', 'unmount'])('discards a pending desktop lookup after %s changes', async change => {
    let resolve!: (value: unknown) => void
    let next = { ...projection }
    vi.spyOn(client, 'apiFetch').mockImplementation(async path => path.includes('/workspaces')
      ? new Promise(done => { resolve = done }) as never : next as never)
    const wrapper = mountPanel(desktopService('https://space.example'))
    try {
      await flushPromises()
      const stale = resolve
      if (change === 'user') await wrapper.setProps({ userId: 'usr_other' })
      if (change === 'binding') { next = { ...projection, bindingRevision: 2 }; await wrapper.get('[aria-label="刷新工作区"]').trigger('click') }
      if (change === 'config') useRemoteWorkspaceStore().current!.configRevision++
      if (change === 'unmount') wrapper.unmount()
      await flushPromises()
      stale({ items: [{ userId: 'usr_test', remoteUsername: 'stale', workspace: projection }] })
      await flushPromises()
      expect(wrapper.findAllComponents(QBtn).some(button => button.props('label') === '云桌面')).toBe(false)
    } finally { if (change !== 'unmount') wrapper.unmount() }
  })
  it('retries desktop lookup on refresh and rejects a mismatched remote binding', async () => {
    let attempts = 0
    vi.spyOn(client, 'apiFetch').mockImplementation(async path => {
      if (!path.includes('/workspaces')) return projection as never
      if (++attempts === 1) throw new Error('temporary list failure')
      return { items: [{ userId: 'usr_test', remoteUsername: 'wrong', workspace: { ...projection, agentSpaceId: 'spc_other' } }] } as never
    })
    const wrapper = mountPanel(desktopService('https://space.example'))
    try {
      await flushPromises()
      expect(wrapper.findComponent({ name: 'ProblemBanner' }).props('error')).toBeDefined()
      await wrapper.get('[aria-label="刷新工作区"]').trigger('click'); await flushPromises()
      expect(attempts).toBe(2)
      expect(wrapper.findAllComponents(QBtn).some(button => button.props('label') === '云桌面')).toBe(false)
    } finally { wrapper.unmount() }
  })
  it('keeps MCP credential repair reachable when the current credential cannot be viewed', async () => {
    const fetch = vi.spyOn(client, 'apiFetch').mockResolvedValue({ ...projection, mcpReason: 'workspace_mcp_credential_unavailable' })
    const wrapper = mountPanel()
    try {
      await flushPromises()
      expect(wrapper.findAllComponents(QBtn).some(button => button.props('label') === 'MCP / 云桌面凭据')).toBe(false)
      const reset = wrapper.findAllComponents(QBtn).find(button => button.props('label') === '重新签发 MCP 凭据')
      expect(reset).toBeDefined()
      expect(reset!.props('disable')).toBe(false)
      expect(fetch.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
    } finally { wrapper.unmount() }
  })
  it('retries a failed credential read without issuing a workspace command', async () => {
    let attempts = 0
    const fetch = vi.spyOn(client, 'apiFetch').mockImplementation(async path => {
      if (!path.includes('/mcp-connection')) return projection as never
      if (++attempts === 1) throw new Error('temporary credential read failure')
      return credentials as never
    })
    const wrapper = mountPanel()
    try {
      await flushPromises()
      await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'MCP / 云桌面凭据')!.trigger('click')
      await flushPromises()
      expect(wrapper.findComponent({ name: 'ProblemBanner' }).props('error')).toBeDefined()
      await wrapper.findAllComponents(QBtn).find(button => button.props('label') === '重新读取')!.trigger('click')
      await flushPromises()
      expect(attempts).toBe(2)
      expect(wrapper.findAllComponents(QInput).find(input => input.props('label') === 'MCP Token')?.props('type')).toBe('password')
      expect(fetch.mock.calls.filter(([, init]) => init?.method === 'POST').every(([path]) => path.includes('/mcp-connection'))).toBe(true)
    } finally { wrapper.unmount() }
  })
  it('mounts resource observations only while their section is expanded', async () => {
    vi.spyOn(client, 'apiFetch').mockResolvedValue(projection)
    const wrapper = mountPanel()
    try {
      await flushPromises()
      expect(wrapper.findComponent({ name: 'WorkspaceResources' }).exists()).toBe(false)
      const details = wrapper.get('[data-cy="workspace-resource-details"]')
      ;(details.element as HTMLDetailsElement).open = true
      await details.trigger('toggle')
      expect(wrapper.findComponent({ name: 'WorkspaceResources' }).exists()).toBe(true)
      ;(details.element as HTMLDetailsElement).open = false
      await details.trigger('toggle')
      expect(wrapper.findComponent({ name: 'WorkspaceResources' }).exists()).toBe(false)
    } finally { wrapper.unmount() }
  })
  it('reveals a masked current Token on demand without published MCP or DAV access', async () => {
    const fetch = vi.spyOn(client, 'apiFetch').mockImplementation(async path => path.includes('/mcp-connection') ? credentials as never : projection as never)
    const wrapper = mountPanel()
    try {
      await flushPromises()
      expect(fetch.mock.calls.some(([path]) => path.includes('/mcp-connection'))).toBe(false)
      const button = wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'MCP / 云桌面凭据')
      expect(button).toBeDefined()
      await button!.trigger('click')
      await flushPromises()
      expect(fetch).toHaveBeenCalledWith('/api/admin/v1/users/usr_test/workspace/mcp-connection?agentSpaceId=spc_test&expectedRevision=1', expect.objectContaining({ method: 'POST' }), 'test-csrf')
      const token = wrapper.findAllComponents(QInput).find(input => input.props('label') === 'MCP Token')!
      expect(token.props('modelValue')).toBe(credentials.token)
      expect(token.props('type')).toBe('password')
      expect(wrapper.text()).toContain('云桌面')
      await wrapper.get('[aria-label="显示 MCP Token"]').trigger('click')
      expect(token.props('type')).toBe('text')
      expect(wrapper.find('[aria-label="复制完整 MCP Token"]').exists()).toBe(true)
    } finally { wrapper.unmount() }
  })
  it.each(['close', 'hidden', 'user', 'revision', 'unavailable', 'unmount'])('discards pending credentials on %s', async change => {
    let reveal!: (value: unknown) => void
    let next = { ...projection }
    vi.spyOn(client, 'apiFetch').mockImplementation(async path => path.includes('/mcp-connection') ? new Promise(resolve => { reveal = resolve }) as never : next as never)
    const wrapper = mountPanel()
    try {
      await flushPromises()
      await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'MCP / 云桌面凭据')!.trigger('click')
      if (change === 'close') wrapper.findAllComponents(QDialog).at(-1)!.vm.$emit('update:modelValue', false)
      if (change === 'hidden') { vi.spyOn(document, 'hidden', 'get').mockReturnValue(true); document.dispatchEvent(new Event('visibilitychange')) }
      if (change === 'user') await wrapper.setProps({ userId: 'usr_other' })
      if (change === 'revision' || change === 'unavailable') {
        next = { ...projection, ...(change === 'revision' ? { bindingRevision: 2 } : { serviceState: 'DISABLED', mcpReason: 'workspace_service_unavailable' }) }
        await wrapper.get('[aria-label="刷新工作区"]').trigger('click')
      }
      if (change === 'unmount') wrapper.unmount()
      await flushPromises(); reveal(credentials); await flushPromises()
      expect(wrapper.findAllComponents(QInput).some(input => input.props('label') === 'MCP Token')).toBe(false)
    } finally { if (change !== 'unmount') wrapper.unmount() }
  })
})

describe('workspace file prerequisites', () => {
  it.each(['already displayed', 'still pending'])('discards %s credentials when the independent DAV revision changes', async timing => {
    const pinia = createPinia()
    useSessionStore(pinia).session = { csrfToken: 'test-csrf' } as components['schemas']['AdminSession']
    let revision = 2
    let reveal!: (value: unknown) => void
    const pendingReveal = new Promise(resolve => { reveal = resolve })
    const credentials = { davUrl: 'http://dav.test/u/test/dav/', username: 'test', token: 'old-token' }
    vi.spyOn(client, 'apiFetch').mockImplementation(async path => {
      if (path.endsWith('/dav-connection')) return pendingReveal as never
      return { schemaVersion: 1, state: 'CONNECTED', bindingRevision: 1, agentSpaceId: 'spc_test', mcpAvailable: true, filesAvailable: true,
        webdav: { resourceId: 'dav_test', available: true, reason: 'available', accessMode: 'FULL', desiredAccessMode: 'FULL', bindingRevision: revision, desiredBindingRevision: revision } } as never
    })
    const wrapper = mount(WorkspacePanel, {
      props: { userId: 'usr_test', serviceEnabled: true },
      global: { plugins: [pinia, [Quasar, { components: { QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog }, directives: { ClosePopup } }]], stubs: { WorkspaceResources: true, WorkspaceFiles: true, ProblemBanner: true, QDialog: { props: ['modelValue'], template: '<div v-if="modelValue"><slot /></div>' } } },
    })
    try {
      await flushPromises()
      await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'WebDAV 连接信息')!.trigger('click')
      if (timing === 'already displayed') {
        reveal(credentials); await flushPromises()
        expect(wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Token')?.props('modelValue')).toBe('old-token')
      }
      revision++
      await wrapper.get('[aria-label="刷新工作区"]').trigger('click'); await flushPromises()
      if (timing === 'still pending') { reveal(credentials); await flushPromises() }
      expect(wrapper.findAllComponents(QInput).some(input => input.props('label') === 'Token')).toBe(false)
      expect(wrapper.findAllComponents(QInput).some(input => input.props('label') === 'Token')).toBe(false)
    } finally { wrapper.unmount() }
  })
  it('shows effective DAV permission separately from Files and the default exchange designation', async () => {
    vi.spyOn(client, 'apiFetch').mockResolvedValue({
      schemaVersion: 1, state: 'CONNECTED', bindingRevision: 1, agentSpaceId: 'spc_test',
      mcpAvailable: true, filesAvailable: true, fileExchangeWebdavResourceId: 'dav_test',
      webdav: { resourceId: 'dav_test', available: true, reason: 'available', accessMode: 'READ_ONLY', desiredAccessMode: 'READ_ONLY', bindingRevision: 2, desiredBindingRevision: 2 },
    })
    const wrapper = mount(WorkspacePanel, {
      props: { userId: 'usr_test', serviceEnabled: true },
      global: { plugins: [createPinia(), [Quasar, { components: { QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog }, directives: { ClosePopup } }]], stubs: { WorkspaceResources: true, WorkspaceFiles: true, ProblemBanner: true } },
    })
    try {
      await flushPromises()
      expect(wrapper.text()).toContain('默认文件交换空间')
      expect(wrapper.text()).toContain('已开通')
      expect(wrapper.text()).not.toContain('MCP 可用')
      expect(wrapper.text()).not.toContain('文件 可用')
      expect(vi.mocked(client.apiFetch).mock.calls.some(([path]) => path.includes('/files'))).toBe(false)
      expect(wrapper.text()).toContain('当前生效：只读')
      expect(wrapper.text()).toContain('不改变 Console 文件管理权限')
    } finally { wrapper.unmount() }
  })
  it('clears a stale query error after a successful refresh', async () => {
    const failure = new Error('temporary query failure')
    vi.spyOn(client, 'apiFetch').mockRejectedValueOnce(failure).mockResolvedValue({
      schemaVersion: 1, state: 'UNPROVISIONED', bindingRevision: 1,
      mcpAvailable: false, filesAvailable: false,
    })
    const wrapper = mount(WorkspacePanel, {
      props: { userId: 'usr_test', serviceEnabled: true },
      global: { plugins: [createPinia(), [Quasar, { components: { QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog }, directives: { ClosePopup } }]], stubs: { WorkspaceResources: true, WorkspaceFiles: true, ProblemBanner: true } },
    })
    try {
      await flushPromises()
      expect(wrapper.findComponent({ name: 'ProblemBanner' }).props('error')).toBe(failure)
      await wrapper.get('[aria-label="刷新工作区"]').trigger('click')
      await flushPromises()
      expect(wrapper.findComponent({ name: 'ProblemBanner' }).props('error')).toBeUndefined()
    } finally { wrapper.unmount() }
  })
  it('allows verified disconnect from an unknown DAV write and requires evidence', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async path => (path.includes('workspace-operations') ? {
      operationId: 'wop_test', action: 'CREATE', state: 'UNKNOWN', step: 'DAV_SENT',
    } : {
      schemaVersion: 1, state: 'CONNECTED', bindingRevision: 1, operationId: 'wop_test',
      agentSpaceId: 'spc_test', mcpAvailable: true, filesAvailable: false,
    }) as never)
    const wrapper = mount(WorkspacePanel, {
      props: { userId: 'usr_test', serviceEnabled: true },
      global: { plugins: [createPinia(), [Quasar, { components: { QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog }, directives: { ClosePopup } }]], stubs: { WorkspaceResources: true, WorkspaceFiles: true, ProblemBanner: true } },
    })
    try {
      await flushPromises()
      const disconnect = wrapper.findAllComponents(QBtn).find(button => button.props('label') === '断开')!
      expect(disconnect.props('disable')).toBe(false)
      expect(wrapper.text()).toContain('文件凭据签发结果待核实')
      expect(wrapper.text()).toContain('MCP 可继续使用')
      expect(wrapper.findAllComponents(QBtn).some(button => button.props('label') === 'WebDAV 连接信息')).toBe(false)
      await disconnect.trigger('click'); await flushPromises()
      expect(wrapper.findAllComponents(QInput).some(input => input.props('label') === '核实依据（必填）')).toBe(true)
      expect(wrapper.findAllComponents(QBtn).find(button => button.props('label') === '确认')!.props('disable')).toBe(true)
    } finally { wrapper.unmount() }
  })
  it('keeps verified cleanup reachable after a lost create and user revocation', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async path => (path.includes('workspace-operations') ? {
      operationId: 'wop_test', action: 'CREATE', state: 'UNKNOWN', step: 'CREATE_SENT',
    } : {
      schemaVersion: 1, state: 'NEEDS_ATTENTION', bindingRevision: 1, operationId: 'wop_test',
      mcpAvailable: false, mcpReason: 'user_unavailable', filesAvailable: false, filesReason: 'user_unavailable',
    }) as never)
    const wrapper = mount(WorkspacePanel, {
      props: { userId: 'usr_test', serviceEnabled: false },
      global: { plugins: [createPinia(), [Quasar, { components: { QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog }, directives: { ClosePopup } }]], stubs: { WorkspaceResources: true, WorkspaceFiles: true, ProblemBanner: true } },
    })
    try {
      await flushPromises()
      expect(wrapper.findAllComponents(QBtn).some(button => button.props('label') === '接管已有空间')).toBe(false)
      const resume = wrapper.findAllComponents(QBtn).find(button => button.props('label') === '核实后继续')!
      expect(resume).toBeDefined()
      await resume.trigger('click'); await flushPromises()
      expect(wrapper.findAllComponents(QInput).map(input => input.props('label'))).toEqual(expect.arrayContaining(['现有远端账号', '原 agentSpaceId', '核实依据（必填）', '新的管理凭据（仅原凭据失效时填写）']))
    } finally { wrapper.unmount() }
  })
  it.each(['dav_not_configured', 'dav_credential_unavailable'])('shows the actionable next step for %s', async reason => {
    vi.spyOn(client, 'apiFetch').mockResolvedValue({
      schemaVersion: 1, state: 'CONNECTED', bindingRevision: 1, agentSpaceId: 'spc_test',
      mcpAvailable: false, mcpReason: 'mcp_not_published', filesAvailable: false, filesReason: reason,
    })
    const wrapper = mount(WorkspacePanel, {
      props: { userId: 'usr_test', serviceEnabled: true },
      global: { plugins: [createPinia(), [Quasar, { components: { QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog }, directives: { ClosePopup } }]], stubs: { WorkspaceResources: true, WorkspaceFiles: true, ProblemBanner: true } },
    })
    try {
      await flushPromises()
      const connectionButton = wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'WebDAV 连接信息')
      expect(!!connectionButton).toBe(false)
      expect(wrapper.findAllComponents(QBtn).some(button => button.props('label') === '签发文件凭据')).toBe(reason === 'dav_credential_unavailable')
      if (reason === 'dav_not_configured') expect(wrapper.text()).toContain('请先在远程工作区服务配置中填写文件服务地址')
    } finally { wrapper.unmount() }
  })
})
