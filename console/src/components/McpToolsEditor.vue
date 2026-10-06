<script setup lang="ts">
import { computed, ref, watch, toRaw, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { useDraftStore } from '../stores/draft'
import { useSessionStore } from '../stores/session'
import PagedEntityPicker from './PagedEntityPicker.vue'
import ProblemBanner from './ProblemBanner.vue'
import McpToolSummary from './McpToolSummary.vue'
import McpToolDetailDialog from './McpToolDetailDialog.vue'
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
const pageSize = 50
const showCatalog = ref(false)
const catalogOpen = computed(() => props.mcp.toolAccessMode === 'ALLOWLIST' || showCatalog.value)
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
function matches(tool: Tool) { return `${tool.name} ${String(tool.definition.description ?? '')}`.toLocaleLowerCase().includes((query.value ?? '').trim().toLocaleLowerCase()) }
const searching = computed(() => Boolean(query.value?.trim()))
const visible = computed(() => rows.value.filter(({ tool }) => matches(tool)))
const matchingCandidates = computed(() => candidates.value.filter(matches))
const matchingGrants = computed(() => {
  const names = new Set(visible.value.map(row => row.tool.name))
  return grants.value.filter(tool => names.has(tool.name))
})
const pageCount = computed(() => Math.max(1, Math.ceil(visible.value.length / pageSize)))
const pageRows = computed(() => visible.value.slice((page.value - 1) * pageSize, page.value * pageSize))
function selected(tool: Tool) { return grants.value.some(grant => grant.name === tool.name) }
function changed(tool: Tool) { return grants.value.some(grant => grant.name === tool.name && grant.contractHash !== tool.contractHash) }
function select(tool: Tool, enabled: boolean) {
  if (locked.value || props.mcp.toolAccessMode !== 'ALLOWLIST') return
  props.mcp.allowedTools ??= []
  const approvalPolicy = props.mcp.allowedTools.find(grant => grant.name === tool.name)?.approvalPolicy ?? 'AUTO'
  props.mcp.allowedTools = props.mcp.allowedTools.filter(grant => grant.name !== tool.name)
  if (enabled) props.mcp.allowedTools.push({ ...structuredClone(toRaw(tool)), approvalPolicy })
  draft.markDirty()
}
function approve(tool: Tool) { select(tool, true) }
function selectAll() {
  if (locked.value || props.mcp.toolAccessMode !== 'ALLOWLIST') return
  props.mcp.allowedTools ??= []
  for (const tool of matchingCandidates.value) if (!selected(tool)) props.mcp.allowedTools.push({ ...structuredClone(toRaw(tool)), approvalPolicy: 'AUTO' })
  draft.markDirty()
}
function clear() {
  if (locked.value || props.mcp.toolAccessMode !== 'ALLOWLIST') return
  const names = new Set(matchingGrants.value.map(tool => tool.name))
  props.mcp.allowedTools = grants.value.filter(tool => !names.has(tool.name))
  draft.markDirty()
}
function mode(value: Mcp['toolAccessMode']) {
  props.mcp.toolAccessMode = value
  props.mcp.allowedTools ??= []
  if (value === 'ALL') props.mcp.allowedTools = []
  draft.markDirty()
}
function remove(name: string) { props.mcp.allowedTools = grants.value.filter(grant => grant.name !== name); draft.markDirty() }
async function discover() {
  error.value = undefined
  try { await draft.discoverMcpTools(props.mcp.mcpServerId, session.csrfToken!, workspace.value ? userId.value : undefined); showCatalog.value = true }
  catch (e) { error.value = e }
}
watch(() => props.mcp.mcpServerId, () => { query.value = ''; page.value = 1; showCatalog.value = false; error.value = undefined; detail.value = undefined; userId.value = props.mcp.toolDiscovery?.userId }, { immediate: true })
watch(() => props.mcp.toolAccessMode, () => { query.value = ''; page.value = 1; showCatalog.value = false })
watch(query, () => { page.value = 1 })
watch(() => props.mcp.toolDiscovery?.discoveredAt, () => { page.value = 1 })
watch(pageCount, count => { page.value = Math.min(page.value, count) })
watch(page, async () => { await nextTick(); catalogTop.value?.scrollIntoView?.({ block: 'start' }) })
</script>

<template>
  <q-card-section class="q-gutter-sm mcp-tools" data-cy="mcp-tools-editor" data-field="allowedTools">
    <div class="text-subtitle2">{{ t('mcpTools.title') }}</div>
    <div class="text-body2 text-grey-7">{{ t('mcpTools.hint') }}</div>
    <q-btn-toggle :model-value="mcp.toolAccessMode" no-caps unelevated spread toggle-color="primary" :options="[{ label: t('mcpTools.allTools'), value: 'ALL' }, { label: t('mcpTools.selectedTools'), value: 'ALLOWLIST' }]" :disable="locked" data-cy="mcp-tool-mode" @update:model-value="mode" />
    <div v-if="mcp.toolAccessMode === 'ALL'" class="text-body2 text-grey-7" data-cy="mcp-tools-all">{{ t('mcpTools.allSummary') }}</div>
    <q-banner v-else-if="mcp.toolAccessMode === 'ALLOWLIST' && !grants.length" dense class="bg-orange-1" data-cy="mcp-tools-empty-error">{{ t('mcpTools.serverEmptySelection') }}</q-banner>
    <PagedEntityPicker v-if="workspace" v-model="userId" :label="t('mcpTools.discoveryUser')" :empty-label="t('mcpTools.chooseUser')" :fetch-page="fetchUserPickerPage" :resolve-option="resolveUserPickerOption" :disabled="locked" />
    <div v-if="workspace" class="text-caption text-grey-7">{{ t('mcpTools.userHint') }}</div>
    <div class="row items-center q-gutter-sm">
      <q-btn outline color="primary" icon="refresh" no-caps :label="draft.dirty ? t('mcpTools.saveDiscover') : t('mcpTools.discover')" :loading="draft.discovering" :disable="locked || !mcp.enabled || (workspace && !userId)" data-cy="mcp-discover" @click="discover" />
      <q-btn v-if="mcp.toolAccessMode === 'ALL' && mcp.toolDiscovery" flat color="primary" no-caps :icon="catalogOpen ? 'expand_less' : 'expand_more'" :label="catalogOpen ? t('mcpTools.hideCatalog') : t('mcpTools.viewCatalog', { count: candidates.length })" :aria-expanded="catalogOpen" data-cy="mcp-catalog-toggle" @click="showCatalog = !showCatalog" />
    </div>
    <div v-if="draft.dirty" class="text-caption text-grey-7" data-cy="mcp-discover-save-hint">{{ t('mcpTools.saveDiscoverHint') }}</div>
    <ProblemBanner :error="error" />
    <q-banner v-if="mcp.allowedTools === undefined || !mcp.toolAccessMode" dense class="bg-orange-1">{{ t('mcpTools.unauthored') }}</q-banner>
    <div v-if="!mcp.toolDiscovery" class="text-body2 text-grey-7" data-cy="mcp-tools-no-catalog">{{ t(mcp.toolAccessMode === 'ALL' ? 'mcpTools.optionalDiscovery' : workspace ? 'mcpTools.workspaceNotDiscovered' : 'mcpTools.notDiscovered') }}</div>
    <div v-if="(mcp.toolDiscovery || grants.length) && catalogOpen" class="mcp-catalog">
      <div ref="catalogTop" class="text-caption catalog-top">{{ t(mcp.toolAccessMode === 'ALL' ? 'mcpTools.countAll' : 'mcpTools.count', { candidates: candidates.length, allowed: grants.length }) }}<template v-if="mcp.toolDiscovery"> · {{ new Date(mcp.toolDiscovery.discoveredAt).toLocaleString() }}</template></div>
      <q-input v-model="query" dense outlined clearable :label="t('common.search')" data-cy="mcp-tool-search" />
      <div v-if="mcp.toolAccessMode === 'ALLOWLIST'" class="row q-gutter-xs">
        <q-btn flat dense color="primary" no-caps :label="searching ? t('mcpTools.selectMatching', { count: matchingCandidates.length }) : t('mcpTools.selectCatalog', { count: candidates.length })" :disable="locked || !matchingCandidates.length" data-cy="mcp-tools-select-all" @click="selectAll" />
        <q-btn flat dense no-caps :label="searching ? t('mcpTools.clearMatching') : t('mcpTools.clear')" :disable="locked || !matchingGrants.length" data-cy="mcp-tools-clear" @click="clear" />
      </div>
      <div v-if="mcp.toolDiscovery && !candidates.length" class="text-body2">{{ t('mcpTools.empty') }}</div>
      <div v-if="!visible.length && rows.length" class="text-body2">{{ t('common.noData') }}</div>
      <div v-if="visible.length" class="text-caption">{{ t('mcpTools.showing', { start: (page - 1) * pageSize + 1, end: Math.min(page * pageSize, visible.length), total: visible.length }) }}</div>
      <div class="mcp-catalog-rows" tabindex="0" role="region" :aria-label="t('mcpTools.catalogLabel')" data-cy="mcp-catalog-rows">
      <div v-for="{ tool, missing, unverified } in pageRows" :key="tool.name" class="mcp-tool" :data-tool-name="tool.name">
        <div class="row no-wrap items-center q-gutter-xs">
          <q-checkbox v-if="mcp.toolAccessMode === 'ALLOWLIST' && !missing && !unverified" :model-value="selected(tool)" :aria-label="tool.name" dense :disable="locked" data-cy="mcp-tool-select" @update:model-value="value => select(tool, Boolean(value))" />
          <McpToolSummary :name="tool.name" :description="tool.definition.description" :fallback="t('mcpTools.noDescription')" />
          <q-select v-if="mcp.toolAccessMode === 'ALLOWLIST' && selected(tool)" v-model="grants.find(grant => grant.name === tool.name)!.approvalPolicy" dense outlined emit-value map-options :aria-label="`${tool.name}: ${t('mcpTools.approval')}`" :label="t('mcpTools.approval')" :options="[{ label: t('mcpTools.confirm'), value: 'REQUIRE_CONFIRMATION' }, { label: t('mcpTools.auto'), value: 'AUTO' }]" :disable="locked || changed(tool) || missing || unverified" data-cy="mcp-tool-approval" class="mcp-tool-approval" @update:model-value="draft.markDirty" />
          <q-btn flat round dense icon="info_outline" :aria-label="t('mcpTools.toolDetails', { name: tool.name })" data-cy="mcp-tool-details" @click="detail = tool" />
        </div>
          <q-banner v-if="missing" dense class="bg-red-1 text-negative" data-cy="mcp-tool-missing">{{ t('mcpTools.missing') }}<q-btn flat dense :label="t('common.remove')" :disable="locked" @click="remove(tool.name)" /></q-banner>
          <q-banner v-if="unverified" dense class="bg-orange-1" data-cy="mcp-tool-unverified">{{ t('mcpTools.unverified') }}<q-btn flat dense :label="t('common.remove')" :disable="locked" @click="remove(tool.name)" /></q-banner>
          <div v-if="changed(tool)" class="text-negative text-body2" role="alert">{{ t('mcpTools.changed') }}</div>
          <div class="row q-gutter-xs items-center q-mt-xs">
            <q-btn v-if="changed(tool)" outline dense color="primary" :label="t('mcpTools.reapprove')" :disable="locked" data-cy="mcp-tool-reapprove" @click="approve(tool)" />
          </div>
      </div>
      </div>
      <q-pagination v-if="pageCount > 1" v-model="page" :max="pageCount" :max-pages="3" :boundary-numbers="false" direction-links :disable="locked" :aria-label="t('mcpTools.catalogPages')" data-cy="mcp-tools-pages" class="q-mt-sm" />
    </div>
    <McpToolDetailDialog :tool="detail" @close="detail = undefined" />
  </q-card-section>
</template>

<style scoped>
.mcp-tools, .mcp-tool { min-width: 0; overflow-wrap: anywhere; }
.mcp-catalog { display: grid; gap: 8px; min-width: 0; }
.mcp-catalog-rows { max-height: 440px; overflow: auto; border: 1px solid #ddd; border-radius: 4px; }
.mcp-tool { padding: 8px 10px; }
.mcp-tool + .mcp-tool { border-top: 1px solid #eee; }
.mcp-tool-approval { flex: 0 0 190px; }
.catalog-top { scroll-margin-top: 60px; }
@media (max-width: 599px) {
  .mcp-tool > .row:first-child { flex-wrap: wrap; }
  .mcp-tool-approval { flex-basis: calc(100% - 36px); order: 1; margin-left: 0; }
  .mcp-catalog-rows { max-height: 380px; }
}
</style>
