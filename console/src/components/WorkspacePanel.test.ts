import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { Quasar, QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog, ClosePopup } from 'quasar'
import WorkspacePanel from './WorkspacePanel.vue'
import * as client from '../api/client'

afterEach(() => vi.restoreAllMocks())

describe('workspace file prerequisites', () => {
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
