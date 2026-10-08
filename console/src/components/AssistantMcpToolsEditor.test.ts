import { expect, it } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { reactive } from 'vue'
import { Quasar, QCard, QCardSection, QCheckbox, QBtnToggle, QBtn, QInput, QBanner, QDialog, QToggle, QChip, QBadge, QSpace, QSeparator, QPagination } from 'quasar'
import type { components } from '../api/generated'
import { useDraftStore } from '../stores/draft'
import AssistantMcpToolsEditor from './AssistantMcpToolsEditor.vue'
import McpToolDetailDialog from './McpToolDetailDialog.vue'
import AssistantMcpToolPicker from './AssistantMcpToolPicker.vue'

type Mcp = components['schemas']['McpDefinition']
const tool = (name: string, description = 'Read documents') => ({ name, contractHash: `sha256:${'1'.repeat(64)}`, definition: { name, description, inputSchema: { type: 'object' } } })
function setup(serverOverrides: Partial<Mcp> = {}, bound = true, names: string[] = []) {
  setActivePinia(createPinia())
  const assistant = reactive<components['schemas']['ManagedAssistantDefinition']>({ assistantDefinitionId: 'asst_test', displayName: 'Test', systemPrompt: 'Help', modelId: 'mdl_test', memorySeed: [], enabled: true, mcpBindings: bound ? [{ mcpServerId: 'mcp_test', toolSelection: 'ALLOWLIST', toolNames: names }] : [] })
  const server: Mcp = { mcpServerId: 'mcp_test', displayName: 'Enterprise Tools', clientProtocol: 'MCP_STREAMABLE_HTTP', authOwnership: 'NONE', enabled: true, runtimePath: '/mcp', toolAccessMode: 'ALL', allowedTools: [], toolDiscovery: { sourceHash: `sha256:${'2'.repeat(64)}`, discoveredAt: '2026-10-06T00:00:00Z', tools: [tool('read')] }, ...serverOverrides }
  const draft = useDraftStore()
  draft.localContent = { providers: [], models: [], imageGenerators: [], tts: [], asr: [], mcp: [server], assistants: [assistant], starters: [], bindings: [], policy: { policyId: 'pol_test', allowLocalProviders: true, allowLocalMcp: true, allowLocalTts: true, allowLocalAsr: true, allowLocalAssistants: true } }
  const wrapper = mount(AssistantMcpToolsEditor, { props: { assistant, disabled: false }, global: { plugins: [[Quasar, { components: { QCard, QCardSection, QCheckbox, QBtnToggle, QBtn, QInput, QBanner, QDialog, QToggle, QChip, QBadge, QSpace, QSeparator, QPagination } }]], stubs: { QDialog: { props: ['modelValue'], template: '<div v-if="modelValue"><slot /></div>' } } } })
  return { assistant, draft, wrapper }
}

it('enables ALL by default and retains an emptied allowlist until an explicit mode change', async () => {
  const { assistant, wrapper } = setup({}, false)
  wrapper.getComponent(QCheckbox).vm.$emit('update:modelValue', true)
  await flushPromises()
  expect(assistant.mcpBindings).toEqual([{ mcpServerId: 'mcp_test', toolSelection: 'ALL', toolNames: [] }])
  wrapper.getComponent(QBtnToggle).vm.$emit('update:modelValue', 'ALLOWLIST')
  await flushPromises()
  await wrapper.get('[data-cy="assistant-tools-choose"]').trigger('click')
  const checkbox = wrapper.get('[data-tool-option="read"] [data-cy="assistant-tool-checkbox"]').getComponent(QCheckbox)
  checkbox.vm.$emit('update:modelValue', true)
  await flushPromises()
  expect(assistant.mcpBindings![0]!.toolNames).toEqual(['read'])
  checkbox.vm.$emit('update:modelValue', false)
  await wrapper.get('[data-cy="assistant-tool-picker-done"]').trigger('click')
  expect(assistant.mcpBindings![0]!.toolSelection).toBe('ALLOWLIST')
  expect(wrapper.find('[data-cy="assistant-tools-empty-error"]').exists()).toBe(true)
  wrapper.getComponent(QBtnToggle).vm.$emit('update:modelValue', 'ALL')
  await flushPromises()
  wrapper.getComponent(QCheckbox).vm.$emit('update:modelValue', false)
  expect(assistant.mcpBindings).toEqual([])
})

it('makes bindings mandatory choices and leaves an unbound enabled server available on mobile', async () => {
  const { assistant, draft, wrapper } = setup({}, false)
  expect(wrapper.getComponent(QCheckbox).props('label')).toBe('Require this MCP')
  expect(wrapper.get('[data-cy="assistant-mcp-optional"]').text()).toContain('Users can select')
  expect(draft.localContent!.mcp[0]!.enabled).toBe(true)
  expect(assistant.mcpBindings).toEqual([])
  wrapper.getComponent(QCheckbox).vm.$emit('update:modelValue', true)
  await flushPromises()
  expect(assistant.mcpBindings).toEqual([{ mcpServerId: 'mcp_test', toolSelection: 'ALL', toolNames: [] }])
  expect(wrapper.find('[data-cy="assistant-mcp-optional"]').exists()).toBe(false)
  expect(wrapper.get('[data-cy="assistant-mcp-required"]').text()).toContain('cannot turn it off')
  wrapper.getComponent(QCheckbox).vm.$emit('update:modelValue', false)
  await flushPromises()
  expect(assistant.mcpBindings).toEqual([])
  expect(draft.localContent!.mcp[0]!.enabled).toBe(true)
  expect(wrapper.get('[data-cy="assistant-mcp-optional"]').text()).toContain('assistant tool restriction')
})

it('distinguishes missing dynamic tools from permission violations and disabled servers', async () => {
  const { assistant, draft, wrapper } = setup({ toolDiscovery: { sourceHash: `sha256:${'2'.repeat(64)}`, discoveredAt: '2026-10-06T00:00:00Z', tools: [] } }, true, ['future_tool'])
  expect(wrapper.get('[data-cy="assistant-tools-unavailable"]').text()).toContain('keep the selection')
  expect(assistant.mcpBindings![0]!.toolNames).toEqual(['future_tool'])
  draft.localContent!.mcp[0]!.toolAccessMode = 'ALLOWLIST'
  await flushPromises()
  expect(wrapper.find('[data-cy="assistant-tools-unavailable"]').exists()).toBe(false)
  expect(wrapper.get('[data-cy="assistant-tools-invalid"]').text()).toContain('outside the server allowlist')
  draft.localContent!.mcp[0]!.enabled = false
  await flushPromises()
  expect(wrapper.get('[data-cy="assistant-mcp-missing"]').text()).toContain('Enterprise Tools')
})

it('opens a server-labelled picker with explicit states and scopes bulk actions to search results', async () => {
  const tools = Array.from({ length: 64 }, (_, index) => tool(`tool-${String(index).padStart(3, '0')}`, index === 63 ? 'Search company records' : 'Read documents'))
  const { assistant, wrapper } = setup({ toolDiscovery: { sourceHash: `sha256:${'2'.repeat(64)}`, discoveredAt: '2026-10-06T00:00:00Z', tools } }, true, ['tool-000'])
  await wrapper.get('[data-cy="assistant-tools-choose"]').trigger('click')
  expect(wrapper.get('[data-cy="assistant-tool-picker"]').text()).toContain('Enterprise Tools')
  expect(wrapper.findAll('[data-tool-option]')).toHaveLength(50)
  expect(wrapper.get('[data-tool-option="tool-000"] [data-cy="assistant-tool-state"]').text()).toBe('Selected')
  expect(wrapper.get('[data-tool-option="tool-001"] [data-cy="assistant-tool-state"]').text()).toBe('Not selected')
  wrapper.getComponent(QToggle).vm.$emit('update:modelValue', true)
  await flushPromises()
  expect(wrapper.get('[data-cy="assistant-picker-clear"]').text()).toBe('Clear allowlist')
  wrapper.getComponent(QToggle).vm.$emit('update:modelValue', false)
  await flushPromises()
  wrapper.getComponent(QPagination).vm.$emit('update:modelValue', 2)
  await flushPromises()
  expect(wrapper.findAll('[data-tool-option]')).toHaveLength(14)
  expect(wrapper.get('[data-tool-option="tool-063"] [data-cy="assistant-tool-state"]').text()).toBe('Not selected')
  const search = wrapper.getComponent(QInput)
  search.vm.$emit('update:modelValue', 'COMPANY RECORDS')
  await flushPromises()
  expect(wrapper.findAll('[data-tool-option]')).toHaveLength(1)
  expect(wrapper.get('[data-tool-option="tool-063"]').text()).toContain('Search company records')
  await wrapper.get('[data-cy="assistant-tools-select-all"]').trigger('click')
  expect(assistant.mcpBindings![0]!.toolNames).toEqual(['tool-000', 'tool-063'])
  await wrapper.get('[data-cy="assistant-picker-clear"]').trigger('click')
  expect(assistant.mcpBindings![0]!.toolNames).toEqual(['tool-000'])
  search.vm.$emit('update:modelValue', '   ')
  await flushPromises()
  expect(wrapper.findAll('[data-tool-option]')).toHaveLength(50)
  await wrapper.get('[data-cy="assistant-tools-select-all"]').trigger('click')
  expect(assistant.mcpBindings![0]!.toolNames).toHaveLength(64)
  wrapper.getComponent(AssistantMcpToolPicker).getComponent(QDialog).vm.$emit('update:modelValue', false)
  await flushPromises()
  expect(wrapper.find('[data-cy="assistant-tool-picker"]').exists()).toBe(false)
  expect(assistant.mcpBindings![0]!.toolNames).toHaveLength(64)
  await wrapper.get('[data-cy="assistant-tools-choose"]').trigger('click')
  await wrapper.get('[data-cy="assistant-tool-picker-done"]').trigger('click')
  expect(wrapper.findAll('.selected-tool-chip')).toHaveLength(2)
  expect(wrapper.get('[data-cy="assistant-tool-summary"]').text()).toContain('+62')
})

it('uses approved descriptions and confirmation policies and views details without selecting', async () => {
  const approved = { ...tool('read', 'Approved read-only records search'), approvalPolicy: 'REQUIRE_CONFIRMATION' as const }
  const { assistant, wrapper } = setup({ toolAccessMode: 'ALLOWLIST', allowedTools: [approved], toolDiscovery: { sourceHash: `sha256:${'2'.repeat(64)}`, discoveredAt: '2026-10-06T00:00:00Z', tools: [tool('read', 'Changed upstream description')] } })
  await wrapper.get('[data-cy="assistant-tools-choose"]').trigger('click')
  expect(wrapper.get('[data-tool-option="read"]').text()).toContain(approved.definition.description)
  expect(wrapper.get('[data-tool-option="read"]').text()).toContain('Confirm each invocation')
  expect(wrapper.get('[data-tool-option="read"]').text()).not.toContain('Changed upstream')
  await wrapper.get('[data-cy="assistant-tool-details"]').trigger('click')
  expect(wrapper.getComponent(McpToolDetailDialog).props('tool')).toEqual(approved)
  expect(assistant.mcpBindings![0]!.toolNames).toEqual([])
})

it('keeps unavailable selected names removable and does not offer them as new choices', async () => {
  const { assistant, wrapper } = setup({}, true, ['future_tool'])
  await wrapper.get('[data-cy="assistant-tools-choose"]').trigger('click')
  expect(wrapper.get('[data-tool-option="future_tool"]').text()).toContain('Unavailable')
  wrapper.getComponent(QToggle).vm.$emit('update:modelValue', true)
  await flushPromises()
  expect(wrapper.findAll('[data-tool-option]')).toHaveLength(1)
  wrapper.get('[data-tool-option="future_tool"] [data-cy="assistant-tool-checkbox"]').getComponent(QCheckbox).vm.$emit('update:modelValue', false)
  await flushPromises()
  expect(assistant.mcpBindings![0]!.toolNames).toEqual([])
  expect(wrapper.find('[data-tool-option="future_tool"]').exists()).toBe(false)
})
