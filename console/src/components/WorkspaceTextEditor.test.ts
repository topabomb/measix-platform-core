import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { Quasar, QCard, QCardSection, QCardActions, QInput, QBtn, QBanner, QDialog, QSpinner } from 'quasar'
import WorkspaceTextEditor from './WorkspaceTextEditor.vue'
import { useSessionStore } from '../stores/session'
import * as client from '../api/client'

function setup(create=false) {
 const pinia=createPinia();setActivePinia(pinia)
 useSessionStore().session={user:{userId:'usr_test',displayName:'Admin',role:'ADMIN'},csrfToken:'csrf',expiresAt:'2099-01-01T00:00:00Z'}
 const wrapper=mount(WorkspaceTextEditor,{props:{baseUrl:'/workspace',spaceId:'spc_original',path:'a.txt',create},global:{plugins:[pinia,[Quasar,{components:{QCard,QCardSection,QCardActions,QInput,QBtn,QBanner,QDialog,QSpinner}}]]}})
 const button=(label:string)=>wrapper.findAllComponents(QBtn).find(b=>b.props('label')===label)!
 return {wrapper,button}
}
afterEach(()=>vi.restoreAllMocks())
describe('workspace text editor',()=>{
 it('uses a fresh request after access is restored without losing the draft',async()=>{
  const calls=vi.spyOn(client,'apiResponse').mockImplementation(async(_url,init)=>{
   expect(init?.signal?.aborted).toBe(false)
   return new Response(JSON.stringify({outcome:'SUCCEEDED'}))
  })
  const {wrapper,button}=setup(true);await flushPromises()
  await wrapper.findAllComponents(QInput).find(i=>i.props('label')==='文本内容')!.setValue('draft')
  await wrapper.setProps({available:false});expect(button('保存并关闭').props('disable')).toBe(true)
  await wrapper.setProps({available:true})
  await button('保存并关闭').trigger('click');await flushPromises()
  expect(calls).toHaveBeenCalledOnce();expect(wrapper.emitted('saved')).toBeTruthy()
  wrapper.unmount()
 })
 it('does not replay an unknown create until a different destination is selected',async()=>{
  vi.spyOn(client,'apiResponse').mockRejectedValue(new client.ApiProblem(503,'workspace_result_unknown','unknown'))
  const {wrapper,button}=setup(true);await flushPromises()
  await wrapper.findAllComponents(QInput).find(i=>i.props('label')==='文本内容')!.setValue('new')
  await button('保存并关闭').trigger('click');await flushPromises()
  expect(button('保存并关闭').props('disable')).toBe(true)
  await wrapper.findAllComponents(QInput).find(i=>i.props('label')==='新文件路径（从根目录起）')!.setValue('another.txt')
  expect(button('保存并关闭').props('disable')).toBe(false)
  wrapper.unmount()
 })
 it.each([409,503])('retains edits and blocks replay after a %i save failure',async status=>{
  const calls=vi.spyOn(client,'apiResponse').mockImplementation(async(_url,init)=>{
   if(init?.method==='PUT')throw new client.ApiProblem(status,status===409?'file_version_conflict':'workspace_result_unknown','failed')
   return new Response('old\r\n',{headers:{ETag:'"content-version"'}})
  })
  const {wrapper,button}=setup();await flushPromises()
  await wrapper.findAllComponents(QInput).find(i=>i.props('label')==='文本内容')!.setValue('edited\n')
  await button('保存并关闭').trigger('click');await flushPromises()
  const write=calls.mock.calls.find(([,init])=>init?.method==='PUT')!
  expect(write[0]).toContain('agentSpaceId=spc_original')
  expect(new Headers(write[1]?.headers).get('If-Match')).toBe('"content-version"')
  expect(new TextDecoder().decode(write[1]?.body as Uint8Array)).toBe('edited\r\n')
  expect(wrapper.findAllComponents(QInput).find(i=>i.props('label')==='文本内容')!.props('modelValue')).toBe('edited\n')
  expect(button('保存并关闭').props('disable')).toBe(true)
  expect(wrapper.emitted('close')).toBeUndefined()
  wrapper.unmount()
 })
 it('save as uses create-only conditions and rejects closing unsaved edits without confirmation',async()=>{
  const calls=vi.spyOn(client,'apiResponse').mockImplementation(async(_url,init)=>init?.method==='PUT'?new Response(JSON.stringify({outcome:'SUCCEEDED'})):new Response('old',{headers:{ETag:'"v1"'}}))
  const {wrapper,button}=setup();await flushPromises()
  await wrapper.findAllComponents(QInput).find(i=>i.props('label')==='文本内容')!.setValue('new')
  await button('关闭').trigger('click');await flushPromises()
  expect(wrapper.emitted('close')).toBeUndefined()
  await button('继续编辑').trigger('click');await button('另存为').trigger('click')
  await wrapper.findAllComponents(QInput).find(i=>i.props('label')==='新文件路径（从根目录起）')!.setValue('copy.txt')
  await button('保存并关闭').trigger('click');await flushPromises()
  const write=calls.mock.calls.find(([,init])=>init?.method==='PUT')!
  expect(new Headers(write[1]?.headers).get('If-None-Match')).toBe('*')
  expect(new Headers(write[1]?.headers).has('If-Match')).toBe(false)
  expect(wrapper.emitted('saved')).toBeTruthy();wrapper.unmount()
 })
})
