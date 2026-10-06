<script setup lang="ts">
import { computed, ref, watch, toRaw, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { useDraftStore } from '../stores/draft'
import { useSessionStore } from '../stores/session'
import PagedEntityPicker from './PagedEntityPicker.vue'
import ProblemBanner from './ProblemBanner.vue'
import { fetchUserPickerPage, resolveUserPickerOption } from '../api/entityPickerSources'

type Mcp = components['schemas']['McpDefinition']
type Tool = components['schemas']['McpDiscoveredTool']
const props = defineProps<{ mcp: Mcp; disabled?: boolean }>()
const draft = useDraftStore()
const session = useSessionStore()
const { t } = useI18n()
const query = ref<string | null>('')
const page = ref(1)
const catalogTop = ref<HTMLElement>()
const pageSize = 25
const error = ref<unknown>()
const detail = ref<Tool>()
const userId = ref<string>()
const workspace = computed(() => Boolean(draft.bindingFor(props.mcp.mcpServerId)?.workspaceServiceId))
const locked = computed(() => props.disabled || draft.discovering || draft.saving)
const candidates = computed(() => props.mcp.toolDiscovery?.tools ?? [])
const grants = computed(() => props.mcp.allowedTools ?? [])
const rows = computed(() => {
  const discovered = new Set(candidates.value.map(tool => tool.name))
  return [
    ...candidates.value.map(tool => ({ tool, missing: false, unverified: false })),
    ...grants.value.filter(grant => !discovered.has(grant.name)).map(tool => ({ tool, missing: Boolean(props.mcp.toolDiscovery), unverified: !props.mcp.toolDiscovery })),
  ]
})
const visible = computed(() => rows.value.filter(({ tool }) => `${tool.name} ${String(tool.definition.description ?? '')}`.toLocaleLowerCase().includes((query.value ?? '').toLocaleLowerCase())))
const pageCount = computed(() => Math.max(1, Math.ceil(visible.value.length / pageSize)))
const pageRows = computed(() => visible.value.slice((page.value - 1) * pageSize, page.value * pageSize))
function selected(tool: Tool) { return grants.value.some(grant => grant.name === tool.name) }
function changed(tool: Tool) { return grants.value.some(grant => grant.name === tool.name && grant.contractHash !== tool.contractHash) }
function select(tool: Tool, enabled: boolean) {
  props.mcp.allowedTools ??= []
  const approvalPolicy = props.mcp.allowedTools.find(grant => grant.name === tool.name)?.approvalPolicy ?? 'AUTO'
  props.mcp.allowedTools = props.mcp.allowedTools.filter(grant => grant.name !== tool.name)
  if (enabled) props.mcp.allowedTools.push({ ...structuredClone(toRaw(tool)), approvalPolicy })
  draft.markDirty()
}
function approve(tool: Tool) { select(tool, true) }
function selectAll() {
  props.mcp.toolAccessMode = 'ALLOWLIST'
  props.mcp.allowedTools ??= []
  for (const tool of candidates.value) if (!selected(tool)) props.mcp.allowedTools.push({ ...structuredClone(toRaw(tool)), approvalPolicy: 'AUTO' })
  draft.markDirty()
}
function clear() { props.mcp.allowedTools = []; draft.markDirty() }
function mode(value: Mcp['toolAccessMode']) {
  props.mcp.toolAccessMode = value
  props.mcp.allowedTools ??= []
  if (value === 'ALL') props.mcp.allowedTools = []
  draft.markDirty()
}
function remove(name: string) { props.mcp.allowedTools = grants.value.filter(grant => grant.name !== name); draft.markDirty() }
async function discover() {
  error.value = undefined
  try { await draft.discoverMcpTools(props.mcp.mcpServerId, session.csrfToken!, workspace.value ? userId.value : undefined) }
  catch (e) { error.value = e }
}
watch(() => props.mcp.mcpServerId, () => { query.value = ''; page.value = 1; error.value = undefined; detail.value = undefined; userId.value = props.mcp.toolDiscovery?.userId }, { immediate: true })
watch(query, () => { page.value = 1 })
watch(() => props.mcp.toolDiscovery?.discoveredAt, () => { page.value = 1 })
watch(pageCount, count => { page.value = Math.min(page.value, count) })
watch(page, async () => { await nextTick(); catalogTop.value?.scrollIntoView?.({ block: 'start' }) })
</script>

<template>
  <q-card-section class="q-gutter-sm mcp-tools" data-cy="mcp-tools-editor" data-field="allowedTools">
    <div class="text-subtitle2">{{ t('mcpTools.title') }}</div>
    <div class="text-body2 text-grey-7">{{ t('mcpTools.hint') }}</div>
    <q-btn-toggle :model-value="mcp.toolAccessMode" no-caps spread :options="[{ label: t('mcpTools.allTools'), value: 'ALL' }, { label: t('mcpTools.selectedTools'), value: 'ALLOWLIST' }]" :disable="locked" data-cy="mcp-tool-mode" @update:model-value="mode" />
    <q-banner v-if="mcp.toolAccessMode === 'ALL'" dense class="bg-blue-1" data-cy="mcp-tools-all">{{ t('mcpTools.serverAll') }}</q-banner>
    <q-banner v-else-if="mcp.toolAccessMode === 'ALLOWLIST' && !grants.length" dense class="bg-orange-1" data-cy="mcp-tools-empty-error">{{ t('mcpTools.serverEmptySelection') }}</q-banner>
    <PagedEntityPicker v-if="workspace" v-model="userId" :label="t('mcpTools.discoveryUser')" :empty-label="t('mcpTools.chooseUser')" :fetch-page="fetchUserPickerPage" :resolve-option="resolveUserPickerOption" :disabled="locked" />
    <div v-if="workspace" class="text-caption text-grey-7">{{ t('mcpTools.userHint') }}</div>
    <q-btn outline color="primary" icon="refresh" :label="draft.dirty ? t('mcpTools.saveDiscover') : t('mcpTools.discover')" :loading="draft.discovering" :disable="locked || !mcp.enabled || (workspace && !userId)" data-cy="mcp-discover" @click="discover" />
    <ProblemBanner :error="error" />
    <q-banner v-if="mcp.allowedTools === undefined || !mcp.toolAccessMode" dense class="bg-orange-1">{{ t('mcpTools.unauthored') }}</q-banner>
    <div v-if="!mcp.toolDiscovery" class="text-body2 text-grey-7" data-cy="mcp-tools-no-catalog">{{ t(mcp.toolAccessMode === 'ALL' ? 'mcpTools.optionalDiscovery' : workspace ? 'mcpTools.workspaceNotDiscovered' : 'mcpTools.notDiscovered') }}</div>
    <template v-if="mcp.toolDiscovery || grants.length">
      <div ref="catalogTop" class="text-caption catalog-top">{{ t(mcp.toolAccessMode === 'ALL' ? 'mcpTools.countAll' : 'mcpTools.count', { candidates: candidates.length, allowed: grants.length }) }}<template v-if="mcp.toolDiscovery"> · {{ new Date(mcp.toolDiscovery.discoveredAt).toLocaleString() }}</template></div>
      <q-input v-model="query" dense outlined clearable :label="t('common.search')" data-cy="mcp-tool-search" />
      <div class="row q-gutter-xs">
        <q-btn flat dense :label="t('mcpTools.selectAll')" :disable="locked || !candidates.length" data-cy="mcp-tools-select-all" @click="selectAll" />
        <q-btn v-if="mcp.toolAccessMode === 'ALLOWLIST'" flat dense :label="t('mcpTools.clear')" :disable="locked" data-cy="mcp-tools-clear" @click="clear" />
      </div>
      <div v-if="mcp.toolDiscovery && !candidates.length" class="text-body2">{{ t('mcpTools.empty') }}</div>
      <div v-if="!visible.length && rows.length" class="text-body2">{{ t('common.noData') }}</div>
      <div v-if="visible.length" class="text-caption">{{ t('mcpTools.showing', { start: (page - 1) * pageSize + 1, end: Math.min(page * pageSize, visible.length), total: visible.length }) }}</div>
      <q-pagination v-if="pageCount > 1" v-model="page" :max="pageCount" :max-pages="3" :boundary-numbers="false" direction-links :disable="locked" :aria-label="t('mcpTools.catalogPages')" data-cy="mcp-tools-pages" />
      <q-card v-for="{ tool, missing, unverified } in pageRows" :key="tool.name" flat bordered class="mcp-tool" :data-tool-name="tool.name">
        <q-card-section class="q-pa-sm">
          <q-checkbox v-if="mcp.toolAccessMode === 'ALLOWLIST' && !missing && !unverified" :model-value="selected(tool)" :label="tool.name" :disable="locked" data-cy="mcp-tool-select" @update:model-value="value => select(tool, Boolean(value))" />
          <div v-else class="text-subtitle2">{{ tool.name }}</div>
          <div class="text-body2 mcp-tool-description">{{ tool.definition.description }}</div>
          <q-banner v-if="missing" dense class="bg-red-1 text-negative" data-cy="mcp-tool-missing">{{ t('mcpTools.missing') }}<q-btn flat dense :label="t('common.remove')" :disable="locked" @click="remove(tool.name)" /></q-banner>
          <q-banner v-if="unverified" dense class="bg-orange-1" data-cy="mcp-tool-unverified">{{ t('mcpTools.unverified') }}<q-btn flat dense :label="t('common.remove')" :disable="locked" @click="remove(tool.name)" /></q-banner>
          <div v-if="changed(tool)" class="text-negative text-body2" role="alert">{{ t('mcpTools.changed') }}</div>
          <div class="row q-gutter-xs items-center q-mt-xs">
            <q-btn v-if="changed(tool)" outline dense color="primary" :label="t('mcpTools.reapprove')" :disable="locked" data-cy="mcp-tool-reapprove" @click="approve(tool)" />
            <q-btn flat dense :label="t('mcpTools.details')" data-cy="mcp-tool-details" @click="detail = tool" />
          </div>
          <q-select v-if="selected(tool)" v-model="grants.find(grant => grant.name === tool.name)!.approvalPolicy" dense outlined emit-value map-options :label="t('mcpTools.approval')" :options="[{ label: t('mcpTools.confirm'), value: 'REQUIRE_CONFIRMATION' }, { label: t('mcpTools.auto'), value: 'AUTO' }]" :disable="locked || changed(tool) || missing || unverified" data-cy="mcp-tool-approval" class="q-mt-sm" @update:model-value="draft.markDirty" />
        </q-card-section>
      </q-card>
      <q-pagination v-if="pageCount > 1" v-model="page" :max="pageCount" :max-pages="3" :boundary-numbers="false" direction-links :disable="locked" :aria-label="t('mcpTools.catalogPages')" />
    </template>
    <q-dialog :model-value="Boolean(detail)" @update:model-value="open => { if (!open) detail = undefined }">
      <q-card class="app-dialog mcp-tool-dialog" data-cy="mcp-tool-dialog">
        <q-card-section class="row items-center"><div class="text-subtitle1 mcp-tool-name">{{ detail?.name }}</div><q-space /><q-btn flat round dense icon="close" :aria-label="t('common.close')" v-close-popup /></q-card-section>
        <q-separator />
        <q-card-section class="mcp-tool-contract"><div class="text-caption">{{ t('mcpTools.untrusted') }}</div><pre>{{ JSON.stringify(detail?.definition, null, 2) }}</pre><div class="text-caption">{{ detail?.contractHash }}</div></q-card-section>
      </q-card>
    </q-dialog>
  </q-card-section>
</template>

<style scoped>
.mcp-tools, .mcp-tool, .mcp-tool-name { min-width: 0; overflow-wrap: anywhere; }
.mcp-tool-description { white-space: pre-wrap; overflow-wrap: anywhere; }
.mcp-tool-dialog { width: min(760px, 94vw); max-width: 94vw; }
.mcp-tool-contract { max-height: 70vh; min-height: 0; overflow: auto; }
.catalog-top { scroll-margin-top: 60px; }
.mcp-tool-contract pre { white-space: pre-wrap; overflow-wrap: anywhere; font-size: 12px; }
</style>
