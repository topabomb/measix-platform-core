import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { Quasar, QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog, ClosePopup } from 'quasar'
import WorkspacePanel from './WorkspacePanel.vue'
import * as client from '../api/client'

afterEach(() => vi.restoreAllMocks())

describe('workspace file prerequisites', () => {
  it('keeps verified cleanup reachable after a lost create and user revocation', async () => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async path => (path.includes('workspace-operations') ? {
      operationId: 'wop_test', action: 'CREATE', state: 'UNKNOWN', step: 'CREATE_SENT',
    } : {
      schemaVersion: 1, state: 'NEEDS_ATTENTION', bindingRevision: 1, operationId: 'wop_test',
      mcpAvailable: false, mcpReason: 'user_unavailable', filesAvailable: false, filesReason: 'user_unavailable',
    }) as never)
    const wrapper = mount(WorkspacePanel, {
      props: { userId: 'usr_test', serviceEnabled: false },
      global: { plugins: [createPinia(), [Quasar, { components: { QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog }, directives: { ClosePopup } }]], stubs: { WorkspaceFiles: true, ProblemBanner: true } },
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
      global: { plugins: [createPinia(), [Quasar, { components: { QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QChip, QDialog }, directives: { ClosePopup } }]], stubs: { WorkspaceFiles: true, ProblemBanner: true } },
    })
    try {
      await flushPromises()
      const connectionButton = wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'WebDAV 连接信息')
      expect(!!connectionButton).toBe(reason !== 'dav_not_configured')
      if (reason === 'dav_not_configured') expect(wrapper.text()).toContain('请先在远程工作区服务配置中填写文件服务地址')
    } finally { wrapper.unmount() }
  })
})
