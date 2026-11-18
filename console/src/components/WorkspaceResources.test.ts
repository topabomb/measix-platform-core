import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { Quasar } from 'quasar'
import WorkspaceResources from './WorkspaceResources.vue'
import * as client from '../api/client'
import fixture from '../../../api/fixtures/workspace/resources.json'

const wrappers: ReturnType<typeof mount>[] = []
function render(){const w=mount(WorkspaceResources,{props:{baseUrl:'/api/admin/v1/users/usr_test/workspace',spaceId:fixture.agentSpaceId,revision:1},global:{plugins:[Quasar]}});wrappers.push(w);return w}
afterEach(()=>{wrappers.splice(0).forEach(w=>w.unmount());vi.restoreAllMocks();vi.useRealTimers()})
describe('workspace resource observations',()=>{
 it('shows real zero, partial absence, allocations and separate runtime',async()=>{
  vi.spyOn(client,'apiFetch').mockResolvedValue(structuredClone(fixture))
  const w=render();await flushPromises()
  expect(w.text()).toContain('运行中');expect(w.text()).toContain('0 B');expect(w.text()).toContain('暂无数据');expect(w.text()).toContain('1 核');expect(w.text()).not.toContain('CPU 使用率')
 })
 it('marks retained values historical after refresh failure and clears on identity conflict',async()=>{
  const fetch=vi.spyOn(client,'apiFetch').mockResolvedValue(structuredClone(fixture));const w=render();await flushPromises()
  fetch.mockRejectedValueOnce({status:503,code:'workspace_unavailable'})
  await w.get('[data-cy=refresh-resources]').trigger('click');await flushPromises()
  expect(w.text()).toContain('历史记录');expect(w.text()).toContain('刷新失败')
  fetch.mockRejectedValueOnce({status:409,code:'workspace_space_mismatch'})
  await w.get('[data-cy=refresh-resources]').trigger('click');await flushPromises()
  expect(w.text()).not.toContain('0 B');expect(w.text()).toContain('空间或配置已变化')
 })
 it('discards responses after switching space and aborts requests on unmount',async()=>{
  let resolve:(v:unknown)=>void=()=>{};let signal:AbortSignal|undefined
  const fetch=vi.spyOn(client,'apiFetch').mockImplementation((_p,init)=>{signal=init?.signal as AbortSignal;return new Promise(r=>{resolve=r}) as never})
  const w=render();const originalSignal=signal;const oldResolve=resolve
  fetch.mockResolvedValueOnce({...structuredClone(fixture),agentSpaceId:'spc_new'})
  await w.setProps({spaceId:'spc_new'});await flushPromises();oldResolve(fixture);await flushPromises()
  expect(originalSignal?.aborted).toBe(true);expect(w.text()).toContain('运行中')
  expect(fetch).toHaveBeenLastCalledWith(expect.stringContaining('spc_new'),expect.anything())
 })
 it('polls only while visible and cleans up timer',async()=>{
  vi.useFakeTimers();const fetch=vi.spyOn(client,'apiFetch').mockResolvedValue(structuredClone(fixture));const w=render();await flushPromises()
  await vi.advanceTimersByTimeAsync(15000);expect(fetch).toHaveBeenCalledTimes(2)
  vi.spyOn(document,'visibilityState','get').mockReturnValue('hidden');document.dispatchEvent(new Event('visibilitychange'))
  await vi.advanceTimersByTimeAsync(45000);expect(fetch).toHaveBeenCalledTimes(2)
  w.unmount();await vi.advanceTimersByTimeAsync(15000);expect(fetch).toHaveBeenCalledTimes(2)
 })
})
