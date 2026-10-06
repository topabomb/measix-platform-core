<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { useDraftStore } from '../stores/draft'
import AssistantMcpToolPicker from './AssistantMcpToolPicker.vue'
type Assistant = components['schemas']['ManagedAssistantDefinition']
type Mcp = components['schemas']['McpDefinition']
type Selection = components['schemas']['AssistantMcpBinding']['toolSelection']
const props = defineProps<{ assistant: Assistant; disabled: boolean }>()
const draft = useDraftStore()
const { t } = useI18n()
const servers = computed(() => draft.localContent?.mcp.filter(mcp => mcp.enabled) ?? [])
const bindings = computed(() => props.assistant.mcpBindings ?? [])
const missing = computed(() => bindings.value.filter(binding => !servers.value.some(server => server.mcpServerId === binding.mcpServerId)))
const pickerId = ref<string>()
const pickerServer = computed(() => servers.value.find(server => server.mcpServerId === pickerId.value && binding(server.mcpServerId)?.toolSelection === 'ALLOWLIST'))
function binding(id: string) { return bindings.value.find(value => value.mcpServerId === id) }
function useServer(id: string, use: boolean) {
  props.assistant.mcpBindings = bindings.value.filter(value => value.mcpServerId !== id)
  if (use) props.assistant.mcpBindings.push({ mcpServerId: id, toolSelection: 'ALL', toolNames: [] })
  draft.markDirty()
}
function mode(id: string, selection: Selection) {
  const value = binding(id)
  if (!value) return
  value.toolSelection = selection
  if (selection === 'ALL') value.toolNames = []
  draft.markDirty()
}
function choose(id: string, names: string[]) {
  if (!binding(id)) useServer(id, true)
  const value = binding(id)!
  value.toolSelection = 'ALLOWLIST'
  value.toolNames = names
  draft.markDirty()
}
function initialize() {
  props.assistant.mcpBindings = (props.assistant.mcpServerIds ?? []).map(id => ({ mcpServerId: id, toolSelection: 'ALL', toolNames: [] }))
  delete props.assistant.mcpServerIds
  draft.markDirty()
}
function invalid(server: Mcp) {
  if (server.toolAccessMode !== 'ALLOWLIST') return []
  return binding(server.mcpServerId)?.toolNames.filter(name => !server.allowedTools?.some(tool => tool.name === name)) ?? []
}
function unavailable(server: Mcp) {
  if (server.toolAccessMode !== 'ALL' || !server.toolDiscovery || binding(server.mcpServerId)?.toolSelection !== 'ALLOWLIST') return []
  return binding(server.mcpServerId)!.toolNames.filter(name => !server.toolDiscovery!.tools.some(tool => tool.name === name))
}
</script>

<template>
  <div class="q-gutter-sm assistant-mcp-tools" data-cy="assistant-mcp-tools" data-field="mcpBindings">
    <div class="text-body2 text-grey-7">{{ t('mcpTools.orchestration') }}</div>
    <q-banner v-if="assistant.mcpBindings === undefined || assistant.mcpServerIds?.length" class="bg-orange-1">
      {{ t('mcpTools.legacy') }}
      <div v-if="assistant.mcpServerIds?.length" class="text-caption">{{ assistant.mcpServerIds.map(id => draft.localContent?.mcp.find(mcp => mcp.mcpServerId === id)?.displayName ?? id).join(', ') }}</div>
      <q-btn flat :label="t('mcpTools.initialize')" :disable="disabled" data-cy="assistant-mcp-initialize" @click="initialize" />
    </q-banner>
    <template v-else>
      <q-card v-for="server in servers" :key="server.mcpServerId" flat bordered :data-mcp-id="server.mcpServerId">
        <q-card-section class="q-gutter-sm">
          <div class="row items-center justify-between"><div class="text-subtitle2">{{ server.displayName }}</div><q-checkbox :model-value="Boolean(binding(server.mcpServerId))" :label="t('mcpTools.useServer')" :disable="disabled" data-cy="assistant-mcp-use" @update:model-value="value => useServer(server.mcpServerId, Boolean(value))" /></div>
          <template v-if="binding(server.mcpServerId)">
            <q-btn-toggle :model-value="binding(server.mcpServerId)!.toolSelection" no-caps unelevated spread toggle-color="primary" :options="[{ label: t('mcpTools.allTools'), value: 'ALL' }, { label: t('mcpTools.selectedTools'), value: 'ALLOWLIST' }]" :disable="disabled" data-cy="assistant-tool-mode" @update:model-value="value => mode(server.mcpServerId, value)" />
            <div v-if="binding(server.mcpServerId)!.toolSelection === 'ALL'" class="text-body2 text-grey-7" data-cy="assistant-tools-all">{{ t('mcpTools.assistantAll') }}</div>
            <template v-else>
              <div class="text-caption">{{ t('mcpTools.selectedCount', { count: binding(server.mcpServerId)!.toolNames.length }) }}</div>
              <div class="row items-center q-gutter-xs" data-cy="assistant-tool-summary">
                  <q-chip v-for="name in binding(server.mcpServerId)!.toolNames.slice(0, 2)" :key="name" dense :removable="!disabled" :title="name" class="selected-tool-chip" @remove="choose(server.mcpServerId, binding(server.mcpServerId)!.toolNames.filter(value => value !== name))"><span class="ellipsis">{{ name }}</span></q-chip>
                  <span v-if="binding(server.mcpServerId)!.toolNames.length > 2" class="text-caption">+{{ binding(server.mcpServerId)!.toolNames.length - 2 }}</span>
              </div>
              <div class="row q-gutter-xs"><q-btn outline dense no-caps color="primary" icon="checklist" :label="t('mcpTools.chooseTools')" :disable="disabled" data-cy="assistant-tools-choose" @click="pickerId = server.mcpServerId" /><q-btn flat dense no-caps :label="t('mcpTools.clear')" :disable="disabled || !binding(server.mcpServerId)!.toolNames.length" data-cy="assistant-tools-clear" @click="choose(server.mcpServerId, [])" /></div>
              <q-banner v-if="!binding(server.mcpServerId)!.toolNames.length" dense class="bg-orange-1" data-cy="assistant-tools-empty-error">{{ t('mcpTools.emptySelection') }}</q-banner>
            </template>
            <div v-if="server.toolAccessMode === 'ALLOWLIST'" class="text-caption text-grey-7">{{ t('mcpTools.serverRestricted') }}</div>
            <q-banner v-if="unavailable(server).length" dense class="bg-orange-1" data-cy="assistant-tools-unavailable">{{ t('mcpTools.unavailableRefs') }}: {{ unavailable(server).join(', ') }}</q-banner>
            <q-banner v-if="invalid(server).length" dense class="bg-red-1 text-negative" data-cy="assistant-tools-invalid">{{ t('mcpTools.invalidRefs') }}: {{ invalid(server).join(', ') }}</q-banner>
          </template>
        </q-card-section>
      </q-card>
      <q-banner v-for="value in missing" :key="value.mcpServerId" dense class="bg-red-1 text-negative" data-cy="assistant-mcp-missing">{{ t('mcpTools.invalidServer') }}: {{ draft.localContent?.mcp.find(server => server.mcpServerId === value.mcpServerId)?.displayName ?? value.mcpServerId }}<q-btn flat :label="t('common.remove')" :disable="disabled" @click="useServer(value.mcpServerId, false)" /></q-banner>
      <div v-if="!servers.length" class="text-grey-7">{{ t('mcpTools.noServers') }}</div>
    </template>
  </div>
  <AssistantMcpToolPicker :server="pickerServer" :names="binding(pickerId ?? '')?.toolNames ?? []" :disabled="disabled" @update:names="names => { if (pickerServer) choose(pickerServer.mcpServerId, names) }" @close="pickerId = undefined" />
</template>

<style scoped>
.assistant-mcp-tools { min-width: 0; overflow-wrap: anywhere; }
.selected-tool-chip { max-width: min(190px, 65vw); }
.selected-tool-chip :deep(.q-chip__content) { min-width: 0; }
</style>
