import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { h } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { Quasar, QLayout, QPageContainer, QPage, QCard, QCardSection, QCardActions, QToggle, QInput, QCheckbox, QBtn, QBanner, QSeparator, QBadge, QList, QItem, QItemSection, QItemLabel, QIcon, QDialog, ClosePopup } from 'quasar'
import RemoteWorkspacesPage from './RemoteWorkspacesPage.vue'
import { useSessionStore } from '../stores/session'
import { useRemoteWorkspaceStore } from '../stores/remoteWorkspace'
import * as client from '../api/client'
import type { components } from '../api/generated'

type Service = components['schemas']['WorkspaceService']
let service: Service | undefined
let operationAction = ''
let checkResult: components['schemas']['WorkspaceServiceCheck']
const configured = (): Service => ({ workspaceServiceId: 'wss_test', type: 'AGENT_SPACE', name: 'Remote workspace', configRevision: 1, activeConfigRevision: 1, enabled: true, state: 'ACTIVE', mcpServerId: 'mcp_test', mcpPublished: false, config: { adminOrigin: 'http://space.test', mcpOrigin: 'http://space.test', managementSecret: { secretId: 'sec_test', secretVersion: 1 }, connectTimeoutMs: 90000, idleTimeoutMs: 120000 } })
function mountPage() {
  const pinia = createPinia(); setActivePinia(pinia)
  useSessionStore(pinia).session = { user: { userId: 'usr_admin', displayName: 'Admin', role: 'ADMIN' }, csrfToken: 'csrf', expiresAt: '2026-12-31T00:00:00Z' }
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: RemoteWorkspacesPage }] })
  return mount({ render: () => h(QLayout, {}, () => h(QPageContainer, {}, () => h(RemoteWorkspacesPage))) }, {
    global: { plugins: [pinia, router, [Quasar, { components: { QLayout, QPageContainer, QPage, QCard, QCardSection, QCardActions, QToggle, QInput, QCheckbox, QBtn, QBanner, QSeparator, QBadge, QList, QItem, QItemSection, QItemLabel, QIcon, QDialog }, directives: { ClosePopup } }]], stubs: { PageHeader: true, WorkspacePanel: true, PagedEntityPicker: true } },
  })
}
function button(wrapper: ReturnType<typeof mountPage>, label: string) { return wrapper.findAllComponents(QBtn).find(item => item.props('label') === label)! }

beforeEach(() => {
  vi.useFakeTimers(); service = undefined; operationAction = ''; vi.restoreAllMocks()
  checkResult = { managementReady: true, filesStatus: 'NOT_CONFIGURED', configRevision: 1, replacementAvailable: false }
  vi.spyOn(client, 'apiFetch').mockImplementation(async (path, init) => {
    if (path.endsWith('/workspace-operations/wop_test')) {
      service = { ...service!, state: operationAction === 'apply' ? 'ACTIVE' : 'DISABLED', enabled: operationAction === 'apply', activeConfigRevision: service!.configRevision }
      return { operationId: 'wop_test', state: 'COMPLETED', step: 'DONE' } as never
    }
    if (path.endsWith('/check')) return checkResult as never
    if (path.endsWith('/apply') || path.endsWith('/disable') || path.endsWith('/replace')) {
      operationAction = path.endsWith('/disable') ? 'disable' : 'apply'
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
  it('checks saved origins and follows the management address before applying', async () => {
    service = configured(); const wrapper = mountPage(); await flushPromises()
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === '管理服务地址')!.setValue('http://new.test')
    expect(button(wrapper, '保存并连接').props('disable')).toBe(false)
    await button(wrapper, '保存并连接').trigger('click'); await flushPromises()
    const write=vi.mocked(client.apiFetch).mock.calls.find(([path,init])=>path.endsWith('/wss_test')&&init?.method==='PUT')!
    expect(JSON.parse(write[1]!.body as string).config.mcpOrigin).toBe('http://new.test')
    const paths=vi.mocked(client.apiFetch).mock.calls.map(([path])=>path)
    expect(paths.findIndex(path=>path.endsWith('/check'))).toBeLessThan(paths.findIndex(path=>path.endsWith('/apply')))
    wrapper.unmount()
  })
  it('preserves published resource information when a save only returns connection fields',async()=>{
    service={...configured(),davResourceId:'dav_test',davDraftEnabled:true,davPublishedEnabled:true,mcpPublished:true}
    checkResult={managementReady:false,filesStatus:'UNVERIFIED',configRevision:2,replacementAvailable:false,diagnosticCode:'management_credential_unavailable'}
    const fetch=vi.mocked(client.apiFetch),original=fetch.getMockImplementation()!
    fetch.mockImplementation(async(path,init,csrf)=>{
      const result=await original(path,init,csrf)
      if(init?.method==='PUT'){
        const value=result as Partial<Service>
        delete value.davResourceId;delete value.davDraftEnabled;delete value.davPublishedEnabled;delete value.mcpPublished
      }
      return result
    })
    const wrapper=mountPage();await flushPromises()
    await wrapper.findAllComponents(QInput).find(input=>input.props('label')==='管理服务地址')!.setValue('http://new.test')
    await button(wrapper,'保存并连接').trigger('click');await flushPromises()
    expect(wrapper.find('[data-cy="workspace-dav-resource"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('MCP 定义已发布')
    wrapper.unmount()
  })
  it('the switch only expresses intent; saving enables service without publishing MCP', async () => {
    const wrapper = mountPage(); await flushPromises()
    expect(button(wrapper, '保存并连接').props('disable')).toBe(true)
    await wrapper.findComponent(QToggle).setValue(true)
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === '管理服务地址')!.setValue('http://space.test')
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === '管理凭据')!.setValue('management')
    expect(wrapper.text()).not.toContain('已确认工作区服务版本')
    expect(button(wrapper, '保存并连接').props('disable')).toBe(false)
    expect(vi.mocked(client.apiFetch).mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
    await button(wrapper, '保存并连接').trigger('click'); await flushPromises()
    expect(operationAction).toBe('apply')
    const write = vi.mocked(client.apiFetch).mock.calls.find(([path, init]) => path === '/api/admin/v1/remote-workspace/services' && init?.method === 'POST')
    expect(JSON.parse(write![1]!.body as string).config).not.toHaveProperty('releaseIdentity')
    expect(wrapper.text()).toContain('正在启用')
    await vi.advanceTimersByTimeAsync(1000); await flushPromises()
    expect(wrapper.text()).toContain('连接已生效')
    expect(wrapper.get('.workspace-service').attributes('open')).toBeUndefined()
    expect(vi.mocked(client.apiFetch).mock.calls.some(([path]) => path.includes('draft') || path.includes('publish'))).toBe(false)
    wrapper.unmount()
  })
  it('closing requires saving and confirming, preserving the stored connection', async () => {
    service = configured(); const wrapper = mountPage(); await flushPromises()
    await wrapper.findComponent(QToggle).setValue(false)
    expect(operationAction).toBe('')
    await button(wrapper, '保存并关闭').trigger('click'); await flushPromises()
    expect(operationAction).toBe('')
    await button(wrapper, '确认关闭').trigger('click'); await flushPromises()
    expect(operationAction).toBe('disable')
    expect(vi.mocked(client.apiFetch).mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
    await vi.advanceTimersByTimeAsync(1000); await flushPromises()
    expect(wrapper.text()).toContain('已有空间和文件保留')
    wrapper.unmount()
  })
  it('allows checking a saved configuration that failed to become active', async () => {
    service={...configured(),configRevision:2}; const wrapper=mountPage(); await flushPromises()
    expect(wrapper.text()).toContain('新配置未生效')
    expect(button(wrapper,'检查连接').props('disable')).toBe(false)
    await button(wrapper,'检查连接').trigger('click');await flushPromises()
    expect(vi.mocked(client.apiFetch).mock.calls.some(([path])=>path.endsWith('/check'))).toBe(true)
    wrapper.unmount()
  })
  it('offers explicit replacement only for a verified missing deployment', async () => {
    service=configured();checkResult={managementReady:false,filesStatus:'UNVERIFIED',configRevision:1,replacementAvailable:true,diagnosticCode:'workspace_targets_missing',missingWorkspaces:[{userId:'usr_test',displayName:'Employee',agentSpaceId:'spc_old',bindingRevision:3,davBindingRevision:4}]}
    const wrapper=mountPage();await flushPromises();await button(wrapper,'检查连接').trigger('click');await flushPromises()
    await button(wrapper,'修复连接').trigger('click');await flushPromises()
    expect(button(wrapper,'确认重新开通').props('disable')).toBe(true)
    expect(vi.mocked(client.apiFetch).mock.calls.some(([path])=>path.endsWith('/replace'))).toBe(false)
    expect(document.body.textContent).toContain('新空间为空')
    await wrapper.findAllComponents(QCheckbox).find(v=>v.props('label')==='旧部署和旧数据已清理，原管理请求已结束')!.setValue(true)
    await button(wrapper,'确认重新开通').trigger('click');await flushPromises()
    const write=vi.mocked(client.apiFetch).mock.calls.find(([path])=>path.endsWith('/replace'))!
    expect(JSON.parse(write[1]!.body as string)).toEqual({expectedConfigRevision:1,confirmedDestroyedDeployment:true,bindings:checkResult.missingWorkspaces})
    wrapper.unmount()
  })
  it('preserves a separately configured MCP address when management moves', async () => {
    service=configured(); service.config.mcpOrigin='http://tools.test'
    const wrapper=mountPage(); await flushPromises()
    await wrapper.findAllComponents(QInput).find(input=>input.props('label')==='管理服务地址')!.setValue('http://new.test')
    await button(wrapper,'保存并连接').trigger('click'); await flushPromises()
    const write=vi.mocked(client.apiFetch).mock.calls.find(([path,init])=>path.endsWith('/wss_test')&&init?.method==='PUT')!
    expect(JSON.parse(write[1]!.body as string).config.mcpOrigin).toBe('http://tools.test')
    wrapper.unmount()
  })
  it.each(['workspace_targets_missing','management_credential_unavailable','workspace_target_mismatch'])('does not offer replacement for an unconfirmed check: %s',async diagnosticCode=>{
    service=configured();checkResult={managementReady:false,filesStatus:'UNVERIFIED',configRevision:1,replacementAvailable:false,diagnosticCode}
    const wrapper=mountPage();await flushPromises();await button(wrapper,'检查连接').trigger('click');await flushPromises()
    expect(button(wrapper,'修复连接')).toBeUndefined()
    expect(vi.mocked(client.apiFetch).mock.calls.some(([path])=>path.endsWith('/replace'))).toBe(false)
    wrapper.unmount()
  })
  it('reconciles an unknown replacement with its frozen request and idempotency key',async()=>{
    service=configured();checkResult={managementReady:false,filesStatus:'UNVERIFIED',configRevision:1,replacementAvailable:true,missingWorkspaces:[{userId:'usr_test',agentSpaceId:'spc_old',bindingRevision:3,davBindingRevision:4}]}
    const fetch=vi.mocked(client.apiFetch),original=fetch.getMockImplementation()!
    let lost=true
    fetch.mockImplementation(async(path,init,csrf)=>{
      if(path.endsWith('/replace')&&lost){lost=false;throw new TypeError('response lost')}
      return original(path,init,csrf)
    })
    const wrapper=mountPage();await flushPromises();await button(wrapper,'检查连接').trigger('click');await flushPromises()
    await button(wrapper,'修复连接').trigger('click');await flushPromises()
    await wrapper.findAllComponents(QCheckbox).find(v=>v.props('label')==='旧部署和旧数据已清理，原管理请求已结束')!.setValue(true)
    await button(wrapper,'确认重新开通').trigger('click');await flushPromises()
    const first=fetch.mock.calls.find(([path])=>path.endsWith('/replace'))!
    service={...service!,configRevision:2};checkResult={...checkResult,configRevision:2,missingWorkspaces:[]}
    useRemoteWorkspaceStore().current=undefined
    await wrapper.findAllComponents(QBtn).filter(v=>v.props('label')==='核实原操作').at(-1)!.trigger('click');await flushPromises()
    const calls=fetch.mock.calls.filter(([path])=>path.endsWith('/replace'))
    expect(calls).toHaveLength(2)
    expect(calls[1]![1]?.body).toBe(first[1]?.body)
    expect(calls[1]![1]?.headers).toEqual(first[1]?.headers)
    expect(fetch.mock.calls.filter(([path])=>path.endsWith('/apply'))).toHaveLength(0)
    wrapper.unmount()
  })
  it('clears the progress notice and requires a new check after a rejected replacement',async()=>{
    service=configured();checkResult={managementReady:false,filesStatus:'UNVERIFIED',configRevision:1,replacementAvailable:true,missingWorkspaces:[{userId:'usr_test',agentSpaceId:'spc_old',bindingRevision:3,davBindingRevision:4}]}
    const fetch=vi.mocked(client.apiFetch),original=fetch.getMockImplementation()!
    fetch.mockImplementation(async(path,init,csrf)=>{
      if(path.endsWith('/replace'))throw new client.ApiProblem(409,'workspace_revision_conflict','changed')
      return original(path,init,csrf)
    })
    const wrapper=mountPage();await flushPromises();await button(wrapper,'检查连接').trigger('click');await flushPromises()
    await button(wrapper,'修复连接').trigger('click');await flushPromises()
    await wrapper.findAllComponents(QCheckbox).find(v=>v.props('label')==='旧部署和旧数据已清理，原管理请求已结束')!.setValue(true)
    await button(wrapper,'确认重新开通').trigger('click');await flushPromises()
    expect(wrapper.text()).not.toContain('正在重新开通用户空间')
    expect(button(wrapper,'修复连接')).toBeUndefined()
    expect(button(wrapper,'检查连接').props('disable')).toBe(false)
    expect(fetch.mock.calls.filter(([path])=>path.endsWith('/replace'))).toHaveLength(1)
    wrapper.unmount()
  })
  it('keeps per-user recovery reachable while a replacement batch needs verification',async()=>{
    service=configured();checkResult={managementReady:false,filesStatus:'UNVERIFIED',configRevision:1,replacementAvailable:true,missingWorkspaces:[{userId:'usr_test',agentSpaceId:'spc_old',bindingRevision:3,davBindingRevision:4}]}
    const fetch=vi.mocked(client.apiFetch),original=fetch.getMockImplementation()!
    fetch.mockImplementation(async(path,init,csrf)=>{
      if(path.endsWith('/replace')){
        service={...service!,enabled:true,state:'APPLYING',operationId:'wop_test'}
        return {operationId:'wop_test',action:'REPLACE',state:'RUNNING',step:'CONTROL'} as never
      }
      if(path.endsWith('/workspace-operations/wop_test')){
        service={...service!,state:'ACTIVE'}
        return {operationId:'wop_test',action:'REPLACE',state:'RUNNING',step:'RECREATING',replacementProgress:{total:1,completed:0,needsAttention:1}} as never
      }
      return original(path,init,csrf)
    })
    const wrapper=mountPage();await flushPromises();await button(wrapper,'检查连接').trigger('click');await flushPromises()
    await button(wrapper,'修复连接').trigger('click');await flushPromises()
    await wrapper.findAllComponents(QCheckbox).find(v=>v.props('label')==='旧部署和旧数据已清理，原管理请求已结束')!.setValue(true)
    await button(wrapper,'确认重新开通').trigger('click');await flushPromises()
    expect(useRemoteWorkspaceStore().enabled).toBe(false)
    await vi.advanceTimersByTimeAsync(1000);await flushPromises()
    expect(useRemoteWorkspaceStore().enabled).toBe(true)
    expect(wrapper.text()).toContain('请在用户工作区核实')
    expect(wrapper.findComponent({name:'PagedEntityPicker'}).exists()).toBe(true)
    wrapper.unmount()
  })
})
