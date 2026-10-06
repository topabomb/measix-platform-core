import { expect, it } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { reactive } from 'vue'
import { Quasar, QCard, QCardSection, QCheckbox, QBtnToggle, QBtn, QSelect, QBanner } from 'quasar'
import type { components } from '../api/generated'
import { useDraftStore } from '../stores/draft'
import AssistantMcpToolsEditor from './AssistantMcpToolsEditor.vue'

it('enables a server with all tools and keeps an emptied allowlist restricted until an explicit mode change', async () => {
  setActivePinia(createPinia())
  const assistant = reactive<components['schemas']['ManagedAssistantDefinition']>({ assistantDefinitionId: 'asst_test', displayName: 'Test', systemPrompt: '', modelId: 'mdl_test', memorySeed: [], enabled: true, mcpBindings: [] })
  useDraftStore().localContent = { providers: [], models: [], imageGenerators: [], tts: [], asr: [], mcp: [{ mcpServerId: 'mcp_test', displayName: 'Tools', clientProtocol: 'MCP_STREAMABLE_HTTP', authOwnership: 'NONE', enabled: true, runtimePath: '/mcp', toolAccessMode: 'ALL', allowedTools: [] }], assistants: [assistant], starters: [], bindings: [], policy: { policyId: 'pol_test', allowLocalProviders: true, allowLocalMcp: true, allowLocalTts: true, allowLocalAsr: true, allowLocalAssistants: true } }
  const wrapper = mount(AssistantMcpToolsEditor, { props: { assistant, disabled: false }, global: { plugins: [[Quasar, { components: { QCard, QCardSection, QCheckbox, QBtnToggle, QBtn, QSelect, QBanner } }]] } })
  expect(wrapper.find('[data-cy="assistant-mcp-use"]').exists()).toBe(true)
  wrapper.getComponent(QCheckbox).vm.$emit('update:modelValue', true)
  await flushPromises()
  expect(assistant.mcpBindings).toEqual([{ mcpServerId: 'mcp_test', toolSelection: 'ALL', toolNames: [] }])
  wrapper.getComponent(QBtnToggle).vm.$emit('update:modelValue', 'ALLOWLIST')
  await flushPromises()
  wrapper.getComponent(QSelect).vm.$emit('update:modelValue', ['read'])
  await flushPromises()
  wrapper.getComponent(QSelect).vm.$emit('update:modelValue', [])
  await flushPromises()
  expect(assistant.mcpBindings![0]!.toolSelection).toBe('ALLOWLIST')
  expect(wrapper.find('[data-cy="assistant-tools-empty-error"]').exists()).toBe(true)
  wrapper.getComponent(QBtnToggle).vm.$emit('update:modelValue', 'ALL')
  await flushPromises()
  expect(assistant.mcpBindings![0]!.toolSelection).toBe('ALL')
  wrapper.getComponent(QCheckbox).vm.$emit('update:modelValue', false)
  await flushPromises()
  expect(assistant.mcpBindings).toEqual([])
})

it('distinguishes temporarily missing dynamic tools from server permission violations', async () => {
  setActivePinia(createPinia())
  const assistant = reactive<components['schemas']['ManagedAssistantDefinition']>({ assistantDefinitionId: 'asst_test', displayName: 'Test', systemPrompt: 'Help', modelId: 'mdl_test', memorySeed: [], enabled: true, mcpBindings: [{ mcpServerId: 'mcp_test', toolSelection: 'ALLOWLIST', toolNames: ['future_tool'] }] })
  const server = { mcpServerId: 'mcp_test', displayName: 'Enterprise Tools', clientProtocol: 'MCP_STREAMABLE_HTTP' as const, authOwnership: 'NONE' as const, enabled: true, runtimePath: '/mcp', toolAccessMode: 'ALL' as const, allowedTools: [], toolDiscovery: { sourceHash: `sha256:${'2'.repeat(64)}`, discoveredAt: '2026-10-06T00:00:00Z', tools: [] } }
  const draft = useDraftStore()
  draft.localContent = { providers: [], models: [], imageGenerators: [], tts: [], asr: [], mcp: [server], assistants: [assistant], starters: [], bindings: [], policy: { policyId: 'pol_test', allowLocalProviders: true, allowLocalMcp: true, allowLocalTts: true, allowLocalAsr: true, allowLocalAssistants: true } }
  const wrapper = mount(AssistantMcpToolsEditor, { props: { assistant, disabled: false }, global: { plugins: [[Quasar, { components: { QCard, QCardSection, QCheckbox, QBtnToggle, QBtn, QSelect, QBanner } }]] } })
  expect(wrapper.get('[data-cy="assistant-tools-unavailable"]').text()).toContain('future_tool')
  expect(wrapper.get('[data-cy="assistant-tools-unavailable"]').text()).toContain('keep the selection')
  expect(assistant.mcpBindings![0]!.toolNames).toEqual(['future_tool'])
  draft.localContent.mcp[0]!.toolAccessMode = 'ALLOWLIST'
  await flushPromises()
  expect(wrapper.find('[data-cy="assistant-tools-unavailable"]').exists()).toBe(false)
  expect(wrapper.get('[data-cy="assistant-tools-invalid"]').text()).toContain('outside the server allowlist')
  draft.localContent.mcp[0]!.enabled = false
  await flushPromises()
  expect(wrapper.get('[data-cy="assistant-mcp-missing"]').text()).toContain('Enterprise Tools')
})

it('searches a large assistant picker without discarding chosen tools or narrowing select-current-tools', async () => {
  setActivePinia(createPinia())
  const assistant = reactive<components['schemas']['ManagedAssistantDefinition']>({ assistantDefinitionId: 'asst_test', displayName: 'Test', systemPrompt: 'Help', modelId: 'mdl_test', memorySeed: [], enabled: true, mcpBindings: [{ mcpServerId: 'mcp_test', toolSelection: 'ALLOWLIST', toolNames: ['tool-000'] }] })
  const tools = Array.from({ length: 64 }, (_, index) => ({ name: `tool-${String(index).padStart(3, '0')}`, contractHash: `sha256:${'1'.repeat(64)}`, definition: { name: `tool-${index}`, inputSchema: { type: 'object' } } }))
  useDraftStore().localContent = { providers: [], models: [], imageGenerators: [], tts: [], asr: [], mcp: [{ mcpServerId: 'mcp_test', displayName: 'Tools', clientProtocol: 'MCP_STREAMABLE_HTTP', authOwnership: 'NONE', enabled: true, runtimePath: '/mcp', toolAccessMode: 'ALL', allowedTools: [], toolDiscovery: { sourceHash: `sha256:${'2'.repeat(64)}`, discoveredAt: '2026-10-06T00:00:00Z', tools } }], assistants: [assistant], starters: [], bindings: [], policy: { policyId: 'pol_test', allowLocalProviders: true, allowLocalMcp: true, allowLocalTts: true, allowLocalAsr: true, allowLocalAssistants: true } }
  const wrapper = mount(AssistantMcpToolsEditor, { props: { assistant, disabled: false }, global: { plugins: [[Quasar, { components: { QCard, QCardSection, QCheckbox, QBtnToggle, QBtn, QSelect, QBanner } }]] } })
  const picker = wrapper.getComponent(QSelect)
  expect(picker.props('useInput')).toBe(true)
  picker.vm.$emit('filter', 'TOOL-063', (update: () => void) => update())
  await flushPromises()
  expect(picker.props('options')).toEqual([{ value: 'tool-063', label: 'tool-063' }])
  expect(assistant.mcpBindings![0]!.toolNames).toEqual(['tool-000'])
  await wrapper.get('[data-cy="assistant-tools-select-all"]').trigger('click')
  expect(assistant.mcpBindings![0]!.toolNames).toHaveLength(64)
})
