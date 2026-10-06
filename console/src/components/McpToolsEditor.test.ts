import { beforeEach, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { reactive, toRaw } from 'vue'
import { Quasar, QCardSection, QCard, QCheckbox, QBtn, QBtnToggle, QInput, QSelect, QBanner, QDialog, QSpace, QSeparator, QPagination, ClosePopup } from 'quasar'
import type { components } from '../api/generated'
import McpToolsEditor from './McpToolsEditor.vue'
import { useDraftStore } from '../stores/draft'
import * as client from '../api/client'

type Mcp = components['schemas']['McpDefinition']
beforeEach(() => setActivePinia(createPinia()))
function setup() {
  useDraftStore().localContent = { providers: [], models: [], imageGenerators: [], tts: [], asr: [], mcp: [], assistants: [], starters: [], bindings: [], policy: { policyId: 'pol_1', allowLocalProviders: true, allowLocalMcp: true, allowLocalTts: true, allowLocalAsr: true, allowLocalAssistants: true } }
  const tool = { name: 'read', contractHash: `sha256:${'1'.repeat(64)}`, definition: { name: 'read', inputSchema: { type: 'object' }, description: 'Read records' } }
  const mcp = reactive<Mcp>({ mcpServerId: 'mcp_1', displayName: 'Test', clientProtocol: 'MCP_STREAMABLE_HTTP', authOwnership: 'NONE', runtimePath: '/mcp', enabled: true, toolAccessMode: 'ALLOWLIST', allowedTools: [], toolDiscovery: { sourceHash: `sha256:${'2'.repeat(64)}`, discoveredAt: '2026-10-06T00:00:00Z', tools: [tool] } })
  const wrapper = mount(McpToolsEditor, { props: { mcp }, global: { plugins: [[Quasar, { components: { QCardSection, QCard, QCheckbox, QBtn, QBtnToggle, QInput, QSelect, QBanner, QDialog, QSpace, QSeparator, QPagination }, directives: { ClosePopup } }]], stubs: { QDialog: true } } })
  return { wrapper, mcp }
}
it('selects a current definition without extra confirmation and preserves explicit confirmation after drift review', async () => {
  const { wrapper, mcp } = setup()
  wrapper.getComponent(QCheckbox).vm.$emit('update:modelValue', true)
  await flushPromises()
  expect(mcp.allowedTools).toHaveLength(1)
  expect(mcp.allowedTools![0]!.approvalPolicy).toBe('AUTO')
  wrapper.getComponent(QSelect).vm.$emit('update:modelValue', 'REQUIRE_CONFIRMATION')
  await flushPromises()
  mcp.toolDiscovery!.tools[0]!.contractHash = `sha256:${'3'.repeat(64)}`
  await flushPromises()
  expect(wrapper.find('[data-cy="mcp-tool-reapprove"]').exists()).toBe(true)
  expect(mcp.allowedTools![0]!.contractHash).toBe(`sha256:${'1'.repeat(64)}`)
  await wrapper.get('[data-cy="mcp-tool-reapprove"]').trigger('click')
  expect(mcp.allowedTools![0]!.contractHash).toBe(`sha256:${'3'.repeat(64)}`)
  expect(mcp.allowedTools![0]!.approvalPolicy).toBe('REQUIRE_CONFIRMATION')
  expect(wrapper.find('[data-cy="mcp-tool-reapprove"]').exists()).toBe(false)
})
it('does not use select-all to silently reapprove changed tools', async () => {
  const { wrapper, mcp } = setup()
  await wrapper.get('[data-cy="mcp-tools-select-all"]').trigger('click')
  expect(mcp.allowedTools![0]!.approvalPolicy).toBe('AUTO')
  mcp.allowedTools![0]!.approvalPolicy = 'REQUIRE_CONFIRMATION'
  const source = structuredClone(toRaw(mcp.toolDiscovery!.tools[0]!))
  mcp.toolDiscovery!.tools.push({ ...source, name: 'second', definition: { ...source.definition, name: 'second' } })
  mcp.toolDiscovery!.tools[0]!.contractHash = `sha256:${'3'.repeat(64)}`
  await wrapper.get('[data-cy="mcp-tools-select-all"]').trigger('click')
  expect(mcp.allowedTools![0]!.contractHash).toBe(`sha256:${'1'.repeat(64)}`)
  expect(mcp.allowedTools!.map(tool => tool.approvalPolicy)).toEqual(['REQUIRE_CONFIRMATION', 'AUTO'])
})
it('preserves an explicit AUTO policy when reviewing a changed contract', async () => {
  const { wrapper, mcp } = setup()
  const tool = structuredClone(toRaw(mcp.toolDiscovery!.tools[0]!))
  mcp.allowedTools = [{ ...tool, approvalPolicy: 'AUTO' }]
  mcp.toolDiscovery!.tools[0]!.contractHash = `sha256:${'3'.repeat(64)}`
  await flushPromises()
  await wrapper.get('[data-cy="mcp-tool-reapprove"]').trigger('click')
  expect(mcp.allowedTools![0]!.approvalPolicy).toBe('AUTO')
  expect(mcp.allowedTools![0]!.contractHash).toBe(`sha256:${'3'.repeat(64)}`)
})
it('clearing the search restores the catalog', async () => {
  const { wrapper } = setup()
  const search = wrapper.getComponent(QInput)
  search.vm.$emit('update:modelValue', 'missing')
  await flushPromises()
  expect(wrapper.find('[data-tool-name="read"]').exists()).toBe(false)
  search.vm.$emit('update:modelValue', null)
  await flushPromises()
  expect(wrapper.find('[data-tool-name="read"]').exists()).toBe(true)
})
it('saves before discovery and preserves authored data on a failed save', async () => {
  const draft = useDraftStore()
  draft.baselineRevision = 5
  draft.localContent = { providers: [], models: [], imageGenerators: [], tts: [], asr: [], mcp: [], assistants: [], starters: [], bindings: [], policy: { policyId: 'pol_1', allowLocalProviders: true, allowLocalMcp: true, allowLocalTts: true, allowLocalAsr: true, allowLocalAssistants: true } }
  const id = draft.addMcp()
  const mock = vi.spyOn(client, 'apiFetch').mockRejectedValueOnce(new client.ApiProblem(409, 'stale_draft_revision', 'Changed'))
  await expect(draft.discoverMcpTools(id, 'csrf')).rejects.toMatchObject({ code: 'stale_draft_revision' })
  expect(mock).toHaveBeenCalledTimes(1)
  expect(draft.localContent.mcp[0]!.mcpServerId).toBe(id)
  expect(draft.dirty).toBe(true)
  expect(draft.discovering).toBe(false)
})

it('bounds catalog rendering and preserves selections across pages and search', async () => {
  const { wrapper, mcp } = setup()
  const source = structuredClone(toRaw(mcp.toolDiscovery!.tools[0]!))
  mcp.toolDiscovery!.tools = Array.from({ length: 61 }, (_, i) => ({ ...source, name: `read_${i}`, definition: { ...source.definition, name: `read_${i}` } }))
  await flushPromises()
  expect(wrapper.findAll('[data-tool-name]')).toHaveLength(50)
  wrapper.findAllComponents(QCheckbox)[0]!.vm.$emit('update:modelValue', true)
  wrapper.getComponent(QPagination).vm.$emit('update:modelValue', 2)
  await flushPromises()
  expect(wrapper.findAll('[data-tool-name]')).toHaveLength(11)
  expect(wrapper.find('[data-tool-name="read_50"]').exists()).toBe(true)
  expect(mcp.allowedTools![0]!.name).toBe('read_0')
  wrapper.getComponent(QInput).vm.$emit('update:modelValue', 'read_60')
  await flushPromises()
  expect(wrapper.findAll('[data-tool-name]')).toHaveLength(1)
  expect(wrapper.find('[data-tool-name="read_60"]').exists()).toBe(true)
  wrapper.getComponent(QInput).vm.$emit('update:modelValue', null)
  await flushPromises()
  expect(wrapper.find('[data-tool-name="read_0"] [aria-checked="true"]').exists()).toBe(true)
})

it('explains server restrictions without referring to the assistant binding switch', async () => {
  const { wrapper, mcp } = setup()
  expect(wrapper.get('[data-cy="mcp-tools-empty-error"]').text()).toContain('disable the server')
  expect(wrapper.get('[data-cy="mcp-tools-empty-error"]').text()).not.toContain('Use this MCP')
  mcp.toolAccessMode = 'ALL'
  delete mcp.toolDiscovery
  await flushPromises()
  expect(wrapper.get('[data-cy="mcp-tools-no-catalog"]').text()).toContain('optional')
  expect(wrapper.get('[data-cy="mcp-tools-no-catalog"]').text()).not.toContain('apply its upstream')
  expect(wrapper.get('[data-cy="mcp-tools-all"]').text()).toContain('New tools')
  expect(wrapper.findAllComponents(QSelect)).toHaveLength(0)
})

it('keeps ALL catalog viewing read-only without selection actions or dirtying the draft', async () => {
  const { wrapper, mcp } = setup()
  const draft = useDraftStore()
  mcp.toolAccessMode = 'ALL'
  await flushPromises()
  expect(wrapper.find('[data-tool-name="read"]').exists()).toBe(false)
  await wrapper.get('[data-cy="mcp-catalog-toggle"]').trigger('click')
  expect(wrapper.find('[data-tool-name="read"]').exists()).toBe(true)
  expect(wrapper.find('[data-cy="mcp-tools-select-all"]').exists()).toBe(false)
  expect(wrapper.find('[data-cy="mcp-tools-clear"]').exists()).toBe(false)
  expect(wrapper.findAllComponents(QCheckbox)).toHaveLength(0)
  expect(wrapper.findAllComponents(QSelect)).toHaveLength(0)
  expect(mcp.toolAccessMode).toBe('ALL')
  expect(mcp.allowedTools).toEqual([])
  expect(draft.dirty).toBe(false)
})

it('selects only matching catalog results while preserving outside selections and explicit policies', async () => {
  const { wrapper, mcp } = setup()
  const source = structuredClone(toRaw(mcp.toolDiscovery!.tools[0]!))
  mcp.toolDiscovery!.tools.push({ ...source, name: 'write', definition: { ...source.definition, name: 'write', description: 'Write records' } })
  mcp.allowedTools = [{ ...source, approvalPolicy: 'REQUIRE_CONFIRMATION' }]
  wrapper.getComponent(QInput).vm.$emit('update:modelValue', 'write')
  await flushPromises()
  expect(wrapper.get('[data-cy="mcp-tools-select-all"]').text()).toContain('matching')
  await wrapper.get('[data-cy="mcp-tools-select-all"]').trigger('click')
  expect(mcp.allowedTools!.map(tool => [tool.name, tool.approvalPolicy])).toEqual([['read', 'REQUIRE_CONFIRMATION'], ['write', 'AUTO']])
  await wrapper.get('[data-cy="mcp-tools-clear"]').trigger('click')
  expect(mcp.allowedTools!.map(tool => tool.name)).toEqual(['read'])
})

it('keeps missing approved tools searchable and bounded instead of mounting every stale row', async () => {
  const { wrapper, mcp } = setup()
  const source = structuredClone(toRaw(mcp.toolDiscovery!.tools[0]!))
  mcp.allowedTools = Array.from({ length: 61 }, (_, i) => ({ ...source, name: `missing_${i}`, definition: { ...source.definition, name: `missing_${i}` }, approvalPolicy: 'REQUIRE_CONFIRMATION' as const }))
  mcp.toolDiscovery!.tools = []
  await flushPromises()
  expect(wrapper.findAll('[data-cy="mcp-tool-missing"]')).toHaveLength(50)
  wrapper.getComponent(QInput).vm.$emit('update:modelValue', 'missing_60')
  await flushPromises()
  expect(wrapper.findAll('[data-cy="mcp-tool-missing"]')).toHaveLength(1)
  await wrapper.get('[data-cy="mcp-tool-missing"] button').trigger('click')
  expect(mcp.allowedTools).toHaveLength(60)
})

it('clears the visible changed tool by its current description while retaining unrelated approvals', async () => {
  const { wrapper, mcp } = setup()
  const source = structuredClone(toRaw(mcp.toolDiscovery!.tools[0]!))
  mcp.allowedTools = [{ ...source, approvalPolicy: 'REQUIRE_CONFIRMATION' }]
  mcp.toolDiscovery!.tools[0]!.definition.description = 'Updated search purpose'
  wrapper.getComponent(QInput).vm.$emit('update:modelValue', 'updated search')
  await flushPromises()
  expect(wrapper.find('[data-tool-name="read"]').exists()).toBe(true)
  expect(wrapper.get('[data-cy="mcp-tools-clear"]').attributes('disabled')).toBeUndefined()
  await wrapper.get('[data-cy="mcp-tools-clear"]').trigger('click')
  expect(mcp.allowedTools).toEqual([])
})

it('retains approved definitions without labeling an undiscovered catalog as a confirmed deletion', async () => {
  const { wrapper, mcp } = setup()
  await wrapper.get('[data-cy="mcp-tools-select-all"]').trigger('click')
  mcp.enabled = false
  delete mcp.toolDiscovery
  await flushPromises()
  expect(wrapper.get('[data-tool-name="read"]').text()).toContain('Read records')
  expect(wrapper.get('[data-cy="mcp-tool-unverified"]').text()).toContain('not been discovered')
  expect(wrapper.find('[data-cy="mcp-tool-missing"]').exists()).toBe(false)
  expect(mcp.allowedTools).toHaveLength(1)
})
