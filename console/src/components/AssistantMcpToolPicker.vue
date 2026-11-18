<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import McpToolSummary from './McpToolSummary.vue'
import McpToolDetailDialog from './McpToolDetailDialog.vue'
type Tool = components['schemas']['McpDiscoveredTool']
const props = defineProps<{ server?: components['schemas']['McpDefinition']; names: string[]; disabled: boolean }>()
const emit = defineEmits<{ 'update:names': [names: string[]]; close: [] }>()
const { t } = useI18n()
const query = ref<string | null>('')
const selectedOnly = ref(false)
const page = ref(1)
const pageSize = 50
const rowsViewport = ref<HTMLElement>()
const detail = ref<Tool>()
const searching = computed(() => Boolean(query.value?.trim()))
const tools = computed(() => props.server?.toolAccessMode === 'ALLOWLIST' ? props.server.allowedTools ?? [] : props.server?.toolDiscovery?.tools ?? [])
const rows = computed(() => [
  ...tools.value.map(tool => ({ name: tool.name, description: tool.definition.description, tool, available: true,
    confirmation: 'approvalPolicy' in tool ? t(tool.approvalPolicy === 'AUTO' ? 'mcpTools.auto' : 'mcpTools.confirm') : undefined })),
  ...props.names.filter(name => !tools.value.some(tool => tool.name === name)).map(name => ({ name, description: undefined, tool: undefined, available: false, confirmation: undefined })),
])
function selected(name: string) { return props.names.includes(name) }
const visible = computed(() => rows.value.filter(row => (!selectedOnly.value || selected(row.name)) && `${row.name} ${String(row.description ?? '')}`.toLocaleLowerCase().includes((query.value ?? '').trim().toLocaleLowerCase())))
const selectable = computed(() => visible.value.filter(row => row.available))
const pageCount = computed(() => Math.max(1, Math.ceil(visible.value.length / pageSize)))
const pageRows = computed(() => visible.value.slice((page.value - 1) * pageSize, page.value * pageSize))
function choose(name: string, use: boolean) {
  if (props.disabled || (use && !tools.value.some(tool => tool.name === name))) return
  emit('update:names', use ? [...new Set([...props.names, name])] : props.names.filter(value => value !== name))
}
function selectMatching() {
  if (!props.disabled) emit('update:names', [...new Set([...props.names, ...selectable.value.map(row => row.name)])])
}
function clearMatching() {
  if (props.disabled) return
  const names = new Set(visible.value.map(row => row.name))
  emit('update:names', props.names.filter(name => !names.has(name)))
}
watch(() => props.server?.mcpServerId, () => { query.value = ''; page.value = 1; selectedOnly.value = false; detail.value = undefined })
watch([query, selectedOnly], () => { page.value = 1 })
watch(pageCount, count => { page.value = Math.min(page.value, count) })
watch(page, async () => { await nextTick(); if (rowsViewport.value) rowsViewport.value.scrollTop = 0 })
</script>

<template>
  <q-dialog :model-value="Boolean(server)" :aria-label="t('mcpTools.chooseForServer', { name: server?.displayName })" @update:model-value="open => { if (!open) emit('close') }">
    <q-card class="assistant-tool-picker" data-cy="assistant-tool-picker">
      <q-card-section class="row no-wrap items-center q-gutter-sm picker-heading">
        <div class="col"><div class="text-subtitle1">{{ t('mcpTools.chooseTools') }}</div><div class="text-caption text-grey-7 ellipsis" :title="server?.displayName">{{ server?.displayName }}</div></div>
        <q-btn flat round dense icon="close" :aria-label="t('common.close')" @click="emit('close')" />
      </q-card-section>
      <q-separator />
      <q-card-section class="q-py-sm picker-controls">
        <q-input v-model="query" outlined dense clearable :label="t('mcpTools.searchTools')" data-cy="assistant-tool-search" />
        <div class="row items-center justify-between"><div class="text-caption">{{ t('mcpTools.selectionStatus', { selected: names.length, available: tools.length }) }}</div><q-toggle v-model="selectedOnly" dense :label="t('mcpTools.selectedOnly')" data-cy="assistant-tools-selected-only" /></div>
        <div class="row q-gutter-xs">
          <q-btn flat dense no-caps color="primary" :label="searching || selectedOnly ? t('mcpTools.selectMatching', { count: selectable.length }) : t('mcpTools.selectCatalog', { count: tools.length })" :disable="disabled || !selectable.some(row => !selected(row.name))" data-cy="assistant-tools-select-all" @click="selectMatching" />
          <q-btn flat dense no-caps :label="searching ? t('mcpTools.clearMatching') : t('mcpTools.clear')" :disable="disabled || !visible.some(row => selected(row.name))" data-cy="assistant-picker-clear" @click="clearMatching" />
        </div>
      </q-card-section>
      <div ref="rowsViewport" class="picker-rows" tabindex="0" role="region" :aria-label="t('mcpTools.catalogLabel')">
        <div v-for="row in pageRows" :key="row.name" class="picker-row" :class="{ 'picker-row-selected': selected(row.name) }" :data-tool-option="row.name">
          <q-checkbox :model-value="selected(row.name)" :aria-label="row.name" dense :disable="disabled || (!row.available && !selected(row.name))" data-cy="assistant-tool-checkbox" @update:model-value="value => choose(row.name, Boolean(value))" />
          <div class="picker-summary"><McpToolSummary :name="row.name" :description="row.description" :fallback="t(row.available ? 'mcpTools.noDescription' : 'mcpTools.toolUnavailable')" /><div v-if="row.confirmation" class="text-caption text-grey-7">{{ row.confirmation }}</div></div>
          <q-badge :color="selected(row.name) ? 'primary' : 'grey-3'" :text-color="selected(row.name) ? 'white' : 'grey-8'" data-cy="assistant-tool-state">{{ t(selected(row.name) ? 'mcpTools.selectedState' : 'mcpTools.unselectedState') }}</q-badge>
          <q-btn v-if="row.tool" flat round dense icon="info_outline" :aria-label="t('mcpTools.toolDetails', { name: row.name })" data-cy="assistant-tool-details" @click="detail = row.tool" />
        </div>
        <div v-if="!visible.length" class="q-pa-md text-grey-7">{{ t(tools.length || names.length ? 'common.noData' : 'mcpTools.discoverForSelection') }}</div>
      </div>
      <q-separator />
      <q-card-section class="picker-footer">
        <q-pagination v-if="pageCount > 1" v-model="page" :max="pageCount" :max-pages="3" :boundary-numbers="false" direction-links :aria-label="t('mcpTools.catalogPages')" data-cy="assistant-picker-pages" />
        <div class="row items-center justify-between q-gutter-sm"><div class="text-caption text-grey-7 col">{{ t('mcpTools.selectionDraftHint') }}</div><q-btn color="primary" unelevated no-caps :label="t('mcpTools.done')" data-cy="assistant-tool-picker-done" @click="emit('close')" /></div>
      </q-card-section>
    </q-card>
  </q-dialog>
  <McpToolDetailDialog :tool="detail" @close="detail = undefined" />
</template>

<style scoped>
.assistant-tool-picker { width: min(800px, calc(100vw - 32px)); max-width: calc(100vw - 32px); max-height: calc(100dvh - 32px); display: flex; flex-direction: column; }
.picker-heading, .picker-controls, .picker-footer { flex: 0 0 auto; }
.picker-controls { display: grid; gap: 8px; }
.picker-rows { min-height: 0; max-height: 440px; overflow: auto; }
.picker-row { display: flex; align-items: center; gap: 8px; padding: 10px 16px; min-height: 64px; }
.picker-row + .picker-row { border-top: 1px solid #eee; }
.picker-row-selected { background: #f0ebfa; }
.picker-summary { min-width: 0; flex: 1; }
.picker-row > .q-badge { flex-shrink: 0; }
.picker-footer { display: grid; gap: 8px; }
.picker-footer .col { min-width: 0; }
@media (max-width: 599px) {
  .picker-row { padding: 8px; gap: 4px; }
  .picker-row > .q-badge { font-size: 10px; padding: 3px; }
}
</style>
