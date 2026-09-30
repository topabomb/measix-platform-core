<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { uid } from 'quasar'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { useDraftStore } from '../stores/draft'
import ValidationIssueItem from './ValidationIssueItem.vue'

type Starter = components['schemas']['AssistantStarterDefinition']

type ValidationIssue = components['schemas']['ValidationIssue']

const props = defineProps<{ disabled: boolean; active?: boolean }>()
const draft = useDraftStore()
const { t } = useI18n()
const selectedId = ref<string>()
const search = ref('')
const selectedSection = ref<'basic' | 'prompt' | 'memory' | 'connections' | 'starters'>('basic')
const editorRoot = ref<HTMLElement>()
const starterEditorRoot = ref<HTMLElement>()
const editingStarterId = ref<string>()
const editedStarter = computed(() => draft.localContent?.starters.find(item => item.starterId === editingStarterId.value))
const starterDialogOpen = computed({
  get: () => props.active !== false && Boolean(editedStarter.value),
  set: (open: boolean) => { if (!open) editingStarterId.value = undefined },
})
const assistants = computed(() => draft.localContent?.assistants ?? [])
const filteredAssistants = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return query ? assistants.value.filter(value => `${value.displayName} ${value.assistantDefinitionId}`.toLocaleLowerCase().includes(query)) : assistants.value
})
const selected = computed(() => assistants.value.find(a => a.assistantDefinitionId === selectedId.value))
const models = computed(() => draft.localContent?.models.filter(m => m.enabled).map(m => ({ label: m.displayName, value: m.modelId })) ?? [])
const mcps = computed(() => draft.localContent?.mcp.filter(m => m.enabled).map(m => ({ label: m.displayName, value: m.mcpServerId })) ?? [])
const starters = computed(() => (draft.localContent?.starters ?? [])
  .filter(s => s.assistantDefinitionId === selectedId.value)
  .toSorted((a, b) => a.sortOrder - b.sortOrder || a.starterId.localeCompare(b.starterId)))
const sections = computed(() => [
  { id: 'basic' as const, label: t('experience.sections.basic') },
  { id: 'prompt' as const, label: t('experience.sections.prompt') },
  { id: 'memory' as const, label: t('experience.sections.memory') },
  { id: 'connections' as const, label: t('experience.sections.connections') },
  { id: 'starters' as const, label: t('experience.sections.starters') },
])
const modelUnavailable = computed(() => Boolean(selected.value?.modelId && !models.value.some(model => model.value === selected.value?.modelId)))
const unavailableMcps = computed(() => selected.value?.mcpServerIds.filter(id => !mcps.value.some(mcp => mcp.value === id)) ?? [])
function issuesForAssistant(assistantId: string): ValidationIssue[] {
  const starterIds = new Set((draft.localContent?.starters ?? []).filter(item => item.assistantDefinitionId === assistantId).map(item => item.starterId))
  const result = draft.validationResult
  return [...(result?.errors ?? []), ...(result?.warnings ?? [])].filter(issue =>
    issue.resourceId === assistantId || Boolean(issue.resourceId && starterIds.has(issue.resourceId)),
  )
}
const selectedIssues = computed(() => selected.value ? issuesForAssistant(selected.value.assistantDefinitionId) : [])
const seedKeys = ref<Record<string, string[]>>({})
let nextSeedKey = 0

function ensureSeedKeys() {
  const assistant = selected.value
  if (!assistant) return
  const keys = seedKeys.value[assistant.assistantDefinitionId] ?? []
  while (keys.length < assistant.memorySeed.length) keys.push(`seed-${nextSeedKey++}`)
  keys.length = assistant.memorySeed.length
  seedKeys.value[assistant.assistantDefinitionId] = keys
}

watch(assistants, values => {
  if (!selectedId.value || !values.some(value => value.assistantDefinitionId === selectedId.value)) {
    selectedId.value = values[0]?.assistantDefinitionId
  }
}, { immediate: true })
watch(selected, ensureSeedKeys, { immediate: true })

function addAssistant() {
  selectedId.value = draft.addAssistant(t('experience.newAssistant'))
  selectedSection.value = 'basic'
}
function addStarter() {
  if (props.disabled || !selected.value) return
  editingStarterId.value = draft.addStarter(selected.value.assistantDefinitionId, t('experience.newStarter'))
}
function chooseAssistant(id: string) {
  selectedId.value = id
  selectedSection.value = 'basic'
}
function moveSeed(index: number, offset: number) {
  const seeds = selected.value?.memorySeed
  if (!seeds || index + offset < 0 || index + offset >= seeds.length) return
  const [seed] = seeds.splice(index, 1)
  if (seed !== undefined) seeds.splice(index + offset, 0, seed)
  const keys = seedKeys.value[selected.value!.assistantDefinitionId]!
  const [key] = keys.splice(index, 1)
  if (key !== undefined) keys.splice(index + offset, 0, key)
  draft.markDirty()
}
function addSeed() {
  if (!selected.value) return
  selected.value.memorySeed.push('')
  ensureSeedKeys()
  draft.markDirty()
}
function removeSeed(index: number) {
  if (!selected.value) return
  selected.value.memorySeed.splice(index, 1)
  seedKeys.value[selected.value.assistantDefinitionId]?.splice(index, 1)
  draft.markDirty()
}
function seedKeyAt(index: number): string {
  if (!selected.value) return String(index)
  return seedKeys.value[selected.value.assistantDefinitionId]?.[index] ?? `${selected.value.assistantDefinitionId}-${index}`
}
function removeAssistant() {
  if (!selected.value || !window.confirm(t('experience.removeConfirm', { name: selected.value.displayName, count: starters.value.length }))) return
  draft.removeAssistant(selected.value.assistantDefinitionId)
  selectedId.value = undefined
  selectedSection.value = 'basic'
}

function useAssistantSystem(starter: Starter) {
  if (props.disabled || !selected.value) return
  if (!starter.openingSnapshot) {
    starter.openingSnapshot = { format: 1, systemPrompt: '', initialContexts: [] }
  } else {
    if (!starter.openingSnapshot.systemPrompt.trim()) return
    if (!window.confirm(t('experience.openingResetConfirm'))) return
    starter.openingSnapshot.systemPrompt = ''
  }
  draft.markDirty()
}
function addContext(starter: Starter) {
  if (props.disabled || !starter.openingSnapshot) return
  const contexts = starter.openingSnapshot.initialContexts
  contexts.push({ id: uid(), title: t('experience.contextNew', { index: contexts.length + 1 }), content: '' })
  draft.markDirty()
}
function moveContext(starter: Starter, index: number, offset: number) {
  if (props.disabled) return
  const contexts = starter.openingSnapshot?.initialContexts
  if (!contexts || index + offset < 0 || index + offset >= contexts.length) return
  const [item] = contexts.splice(index, 1)
  if (item) contexts.splice(index + offset, 0, item)
  draft.markDirty()
}
function removeContext(starter: Starter, index: number) {
  if (props.disabled) return
  starter.openingSnapshot?.initialContexts.splice(index, 1)
  draft.markDirty()
}

async function focusIssue(kind: 'ASSISTANT' | 'STARTER', resourceId?: string, field?: string, path = '') {
  if (!resourceId) return
  if (kind === 'STARTER') {
    const starter = draft.localContent?.starters.find(item => item.starterId === resourceId)
    if (!starter) return
    selectedId.value = starter.assistantDefinitionId
    selectedSection.value = 'starters'
    editingStarterId.value = starter.starterId
  } else {
    selectedId.value = resourceId
    selectedSection.value = field === 'systemPrompt' ? 'prompt'
      : field === 'memorySeed' ? 'memory'
        : field === 'modelId' || field === 'mcpServerIds' ? 'connections'
          : 'basic'
  }
  await nextTick()
  let element: HTMLElement | null = null
  if (kind === 'STARTER') {
    const root = starterEditorRoot.value
    if (path.includes('openingSnapshot') || field === 'openingSnapshot') {
      const details = root?.querySelector<HTMLDetailsElement>('[data-cy="starter-opening"]')
      if (details) details.open = true
      const contextIndex = /initialContexts\[(\d+)\]/.exec(path)?.[1]
      const contextField = /\.(id|title|content)$/.exec(path)?.[1]
      element = contextIndex === undefined
        ? root?.querySelector<HTMLElement>('[data-cy="starter-opening-system"], [data-cy="starter-opening-create"]') ?? null
        : root?.querySelector<HTMLElement>(`[data-context-index="${contextIndex}"] [data-field="${contextField === 'content' ? 'content' : 'title'}"]`) ?? null
    } else {
      element = root?.querySelector<HTMLElement>(`[data-field="${field ?? ''}"]`) ?? null
    }
  } else if (field === 'memorySeed') {
    const index = /memorySeed\[(\d+)\]/.exec(path)?.[1]
    element = index === undefined ? null : editorRoot.value?.querySelector<HTMLElement>(`[data-seed-index="${index}"]`) ?? null
  } else if (field) {
    element = editorRoot.value?.querySelector<HTMLElement>(`[data-field="${field}"]`) ?? null
  }
  element?.scrollIntoView?.({ block: 'center' })
  const focusElement = element?.matches('button, input, textarea, [tabindex]') ? element : element?.querySelector<HTMLElement>('input, textarea, button, [tabindex]')
  focusElement?.focus()
}

defineExpose({ focusIssue })
</script>

<template>
  <section ref="editorRoot" class="assistant-editor" data-cy="experience-editor">
    <q-banner class="bg-blue-1 q-mb-xs rounded-borders">
      <div class="text-weight-medium">{{ t('experience.managedSource') }}</div>
      <div class="text-body2">{{ t('experience.hint') }}</div>
    </q-banner>
    <div class="row q-col-gutter-xs">
      <div class="col-12 col-md-4">
        <q-card flat bordered>
          <q-card-section class="row items-center justify-between">
            <div>
              <div class="text-subtitle2">{{ t('experience.tab') }}</div>
              <div class="text-caption text-grey-7">{{ t('experience.assistantCount', { count: assistants.length }) }}</div>
            </div>
            <q-btn round flat color="primary" icon="add" :aria-label="t('experience.addAssistant')" :disable="disabled" data-cy="assistant-add" @click="addAssistant" />
          </q-card-section>
          <q-separator />
          <q-card-section>
            <q-input v-model="search" dense outlined clearable :label="t('common.search')" data-cy="assistant-search" />
          </q-card-section>
          <q-separator />
          <q-list separator>
            <q-item v-for="a in filteredAssistants" :key="a.assistantDefinitionId" clickable :active="selectedId === a.assistantDefinitionId" active-class="bg-blue-1 text-primary" @click="chooseAssistant(a.assistantDefinitionId)">
              <q-item-section avatar><q-icon name="smart_toy" /></q-item-section>
              <q-item-section>
                <q-item-label>{{ a.displayName }}</q-item-label>
                <q-item-label caption>{{ a.enabled ? t('common.enabled') : t('common.disabled') }} · {{ a.memorySeed.length }} {{ t('experience.seedCount') }}</q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-badge v-if="issuesForAssistant(a.assistantDefinitionId).length" color="negative" :label="issuesForAssistant(a.assistantDefinitionId).length" />
                <q-icon v-else name="chevron_right" />
              </q-item-section>
            </q-item>
            <q-item v-if="!assistants.length"><q-item-section class="text-grey-7">{{ t('experience.noAssistants') }}</q-item-section></q-item>
          </q-list>
        </q-card>
      </div>

      <div v-if="selected" class="col-12 col-md-8">
        <q-card flat bordered>
          <q-card-section class="row items-start justify-between no-wrap q-gutter-xs">
            <div class="assistant-heading">
              <div class="text-h6">{{ selected.displayName }}</div>
              <details class="text-caption text-grey-7" data-cy="assistant-identity"><summary>{{ t('resources.review.technicalDetails') }}</summary>{{ selected.assistantDefinitionId }}</details>
            </div>
            <q-toggle v-model="selected.enabled" :label="t('experience.enabled')" :disable="disabled" @update:model-value="draft.markDirty" />
          </q-card-section>
          <q-separator />

          <div class="assistant-settings-workbench">
            <q-tabs v-model="selectedSection" dense no-caps mobile-arrows outside-arrows align="left" active-color="primary" indicator-color="primary" class="assistant-settings-sections">
              <q-tab v-for="section in sections" :key="section.id" :name="section.id" :label="section.label" :data-cy="`assistant-section-${section.id}`" />
            </q-tabs>

            <q-card-section class="assistant-settings-detail q-gutter-xs">
              <template v-if="selectedSection === 'basic'">
                <div class="text-subtitle1">{{ t('experience.sections.basic') }}</div>
                <q-input v-model="selected.displayName" outlined :label="t('experience.name')" :disable="disabled" data-cy="assistant-name" data-field="displayName" @update:model-value="draft.markDirty" />
                <q-input v-model="selected.description" outlined :label="t('experience.description')" :disable="disabled" @update:model-value="draft.markDirty" />
                <q-btn flat color="negative" icon="delete" :label="t('experience.removeAssistant')" :disable="disabled" @click="removeAssistant" />
              </template>

              <template v-else-if="selectedSection === 'prompt'">
                <div class="text-subtitle1">{{ t('experience.sections.prompt') }}</div>
                <div class="text-body2 text-grey-7">{{ t('experience.promptHint') }}</div>
                <q-input v-model="selected.systemPrompt" outlined autogrow type="textarea" :label="t('experience.systemPrompt')" :disable="disabled" data-cy="assistant-prompt" data-field="systemPrompt" @update:model-value="draft.markDirty" />
              </template>

              <template v-else-if="selectedSection === 'memory'">
                <div class="text-subtitle1">{{ t('experience.memorySeed') }}</div>
                <div class="text-body2 text-grey-7">{{ t('experience.memoryHint') }}</div>
                <div v-for="(_, i) in selected.memorySeed" :key="seedKeyAt(i)" class="seed-row">
                  <q-input v-model="selected.memorySeed[i]" outlined autogrow type="textarea" :label="t('experience.seedEntry', { index: i + 1 })" :disable="disabled" :data-cy="'seed-input-' + i" data-field="memorySeed" :data-seed-index="i" @update:model-value="draft.markDirty" />
                  <div class="row no-wrap">
                    <q-btn flat icon="arrow_upward" :aria-label="t('experience.moveUp')" :disable="disabled || i === 0" :data-cy="'seed-up-' + i" @click="moveSeed(i, -1)" />
                    <q-btn flat icon="arrow_downward" :aria-label="t('experience.moveDown')" :disable="disabled || i === selected.memorySeed.length - 1" @click="moveSeed(i, 1)" />
                    <q-btn flat color="negative" icon="delete" :aria-label="t('common.remove')" :disable="disabled" @click="removeSeed(i)" />
                  </div>
                </div>
                <q-btn outline icon="add" :label="t('experience.addSeed')" :disable="disabled" data-cy="seed-add" @click="addSeed" />
              </template>

              <template v-else-if="selectedSection === 'connections'">
                <div class="text-subtitle1">{{ t('experience.sections.connections') }}</div>
                <div class="text-body2 text-grey-7">{{ t('experience.connectionsHint') }}</div>
                <q-select data-cy="assistant-model" v-model="selected.modelId" outlined :options="models" emit-value map-options :label="t('experience.model')" :disable="disabled" data-field="modelId" @update:model-value="draft.markDirty" />
                <q-banner v-if="modelUnavailable" class="bg-red-1 rounded-borders text-negative">{{ t('experience.unavailableModel', { id: selected.modelId }) }}</q-banner>
                <q-select v-model="selected.mcpServerIds" outlined :options="mcps" multiple use-chips emit-value map-options :label="t('experience.mcps')" :disable="disabled" data-field="mcpServerIds" @update:model-value="draft.markDirty" />
                <q-banner v-if="unavailableMcps.length" class="bg-red-1 rounded-borders text-negative">{{ t('experience.unavailableMcps', { ids: unavailableMcps.join(', ') }) }}</q-banner>
              </template>

              <template v-else>
                <div class="row items-center justify-between q-gutter-xs">
                  <div>
                    <div class="text-subtitle1">{{ t('experience.starters') }}</div>
                    <div class="text-body2 text-grey-7">{{ t('experience.startersHint') }}</div>
                  </div>
                  <q-btn outline icon="add" :label="t('experience.addStarter')" :disable="disabled" data-cy="starter-add" @click="addStarter" />
                </div>
                <div class="text-caption text-grey-7">{{ t('experience.starterOrderHint') }}</div>
                <q-card v-for="(s, index) in starters" :key="s.starterId" flat bordered :data-starter-id="s.starterId" data-cy="starter-summary">
                  <q-card-section class="q-gutter-xs">
                    <div class="row items-start justify-between no-wrap q-gutter-xs">
                      <div class="col starter-summary-text">
                        <div class="text-subtitle2">{{ s.title }}</div>
                        <div class="text-body2 ellipsis-2-lines">{{ s.prompt || t('experience.starterPromptEmpty') }}</div>
                      </div>
                      <q-btn flat dense no-caps class="starter-edit" color="primary" icon="edit" :label="t('common.edit')" :disable="disabled" data-cy="starter-edit" @click="editingStarterId = s.starterId" />
                    </div>
                    <div class="text-caption text-grey-7">{{ s.openingSnapshot ? t('experience.opening', { count: s.openingSnapshot.initialContexts.length }) : t('experience.openingMissing') }}</div>
                    <div class="row items-center justify-between q-gutter-xs">
                      <q-toggle v-model="s.enabled" dense :label="t('experience.enabled')" :disable="disabled" @update:model-value="draft.markDirty" />
                      <div class="row no-wrap">
                        <q-btn flat round dense icon="arrow_upward" :aria-label="`${t('experience.moveUp')} ${s.title}`" :disable="disabled || index === 0" data-cy="starter-up" @click="draft.moveStarter(s.starterId, -1)" />
                        <q-btn flat round dense icon="arrow_downward" :aria-label="`${t('experience.moveDown')} ${s.title}`" :disable="disabled || index === starters.length - 1" data-cy="starter-down" @click="draft.moveStarter(s.starterId, 1)" />
                        <q-btn flat round dense color="negative" icon="delete" :aria-label="`${t('common.remove')} ${s.title}`" :disable="disabled" data-cy="starter-remove" @click="draft.removeStarter(s.starterId)" />
                      </div>
                    </div>
                  </q-card-section>
                </q-card>
              </template>
            </q-card-section>
          </div>
          <q-separator v-if="draft.validationResult" />
          <q-card-section v-if="draft.validationResult">
            <div class="text-subtitle2">{{ t('resources.model.validation') }}</div>
            <q-list v-if="selectedIssues.length" dense>
              <ValidationIssueItem v-for="issue in selectedIssues" :key="issue.path + issue.code" :issue="issue" />
            </q-list>
            <div v-else class="text-positive q-mt-xs">{{ t('resources.model.noIssues') }}</div>
          </q-card-section>
        </q-card>
      </div>
    </div>
    <q-dialog v-model="starterDialogOpen" :persistent="disabled">
      <q-card v-if="editedStarter && selected" class="app-dialog app-dialog--lg starter-editor" data-cy="starter-editor-dialog">
        <q-card-section class="row items-center justify-between no-wrap">
          <div class="text-h6">{{ t('experience.editStarter') }}</div>
          <q-btn flat round dense icon="close" :aria-label="t('common.close')" :disable="disabled" data-cy="starter-editor-close" @click="starterDialogOpen = false" />
        </q-card-section>
        <q-separator />
        <q-card-section class="app-dialog__body">
          <div ref="starterEditorRoot" class="q-gutter-xs">
                    <details class="text-caption text-grey-7"><summary>{{ t('resources.review.technicalDetails') }}</summary>{{ editedStarter.starterId }}</details>
                    <q-input data-cy="starter-title" v-model="editedStarter.title" outlined :label="t('experience.title')" :disable="disabled" data-field="title" @update:model-value="draft.markDirty" />
                    <q-input v-model="editedStarter.description" outlined :label="t('experience.description')" :disable="disabled" @update:model-value="draft.markDirty" />
                    <q-input data-cy="starter-prompt" v-model="editedStarter.prompt" outlined autogrow type="textarea" :label="t('experience.starterPrompt')" :disable="disabled" data-field="prompt" @update:model-value="draft.markDirty" />
                    <details data-cy="starter-opening" class="starter-opening">
                      <summary class="cursor-pointer text-body2">{{ editedStarter.openingSnapshot ? t('experience.opening', { count: editedStarter.openingSnapshot.initialContexts.length }) : t('experience.openingMissing') }}</summary>
                      <div class="q-gutter-xs q-mt-xs">
                        <q-btn v-if="!editedStarter.openingSnapshot" flat no-caps color="primary" :label="t('experience.openingCreate')" :disable="disabled" data-cy="starter-opening-create" @click="useAssistantSystem(editedStarter)" />
                        <template v-else>
                          <div class="row items-center justify-between q-gutter-xs">
                            <span class="text-subtitle2">{{ editedStarter.openingSnapshot.systemPrompt.trim() ? t('experience.openingCustom') : t('experience.openingInherited') }}</span>
                            <q-btn v-if="editedStarter.openingSnapshot.systemPrompt.trim()" flat dense no-caps color="primary" :label="t('experience.openingReset')" :disable="disabled" data-cy="starter-opening-reset" @click="useAssistantSystem(editedStarter)" />
                          </div>
                          <q-input v-model="editedStarter.openingSnapshot.systemPrompt" outlined type="textarea" :rows="3" :label="t('experience.openingOverride')" :placeholder="t('experience.openingPlaceholder')" :disable="disabled" data-cy="starter-opening-system" data-field="systemPrompt" @update:model-value="draft.markDirty" />
                          <div class="text-caption text-grey-7" data-cy="starter-opening-hint">{{ t('experience.openingHint') }}</div>
                          <details class="text-caption text-grey-7" data-cy="starter-assistant-system">
                            <summary class="cursor-pointer">{{ t('experience.openingAssistantPreview') }}</summary>
                            <div class="starter-literal q-mt-xs">{{ selected.systemPrompt || t('experience.emptyAssistantSystem') }}</div>
                          </details>
                          <q-separator class="q-my-xs" />
                          <div class="text-subtitle2">{{ t('experience.contexts') }}</div>
                          <div class="text-caption text-grey-7">{{ t('experience.contextHint') }}</div>
                          <div v-for="(context, index) in editedStarter.openingSnapshot.initialContexts" :key="context.id" class="starter-context q-gutter-xs" :data-context-index="index">
                            <div class="row items-center justify-between no-wrap">
                              <span class="text-caption text-grey-7">{{ t('experience.contextNew', { index: index + 1 }) }}</span>
                              <div class="row no-wrap">
                                <q-btn flat round dense icon="arrow_upward" :aria-label="`${t('experience.moveUp')} ${context.title}`" :disable="disabled || index === 0" data-cy="starter-context-up" @click="moveContext(editedStarter, index, -1)" />
                                <q-btn flat round dense icon="arrow_downward" :aria-label="`${t('experience.moveDown')} ${context.title}`" :disable="disabled || index === editedStarter.openingSnapshot.initialContexts.length - 1" data-cy="starter-context-down" @click="moveContext(editedStarter, index, 1)" />
                                <q-btn flat round dense color="negative" icon="delete" :aria-label="`${t('common.remove')} ${context.title}`" :disable="disabled" data-cy="starter-context-remove" @click="removeContext(editedStarter, index)" />
                              </div>
                            </div>
                            <q-input v-model="context.title" dense outlined :label="t('experience.contextTitle')" :disable="disabled" data-cy="starter-context-title" data-field="title" @update:model-value="draft.markDirty" />
                            <q-input v-model="context.content" outlined type="textarea" :rows="3" :label="t('experience.contextContent')" :disable="disabled" data-cy="starter-context-content" data-field="content" @update:model-value="draft.markDirty" />
                            <details class="text-caption text-grey-7"><summary>{{ t('resources.review.technicalDetails') }}</summary>{{ context.id }}</details>
                          </div>
                          <q-btn outline icon="add" :label="t('experience.contextAdd')" :disable="disabled" data-cy="starter-context-add" @click="addContext(editedStarter)" />
                        </template>
                      </div>
                    </details>
          </div>
        </q-card-section>
        <q-separator />
        <q-card-actions class="starter-editor-footer">
          <span class="text-caption text-grey-7">{{ t('experience.starterDraftHint') }}</span>
          <q-btn color="primary" no-caps :label="t('common.done')" :disable="disabled" data-cy="starter-editor-done" @click="starterDialogOpen = false" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </section>
</template>

<style scoped>
.assistant-editor, .starter-editor { min-width: 0; overflow-wrap: anywhere; }
.assistant-heading { flex: 1; min-width: 0; }
.assistant-heading + .q-toggle { flex-shrink: 0; }
.starter-opening { min-width: 0; overflow-wrap: anywhere; }
.starter-literal { white-space: pre-wrap; max-height: 240px; overflow: auto; }
.starter-context { border: 1px solid var(--q-separator-color, #ddd); border-radius: 8px; padding: 4px; min-width: 0; }
.starter-summary-text { min-width: 0; }
.starter-edit { flex-shrink: 0; }
.starter-editor-footer { display: flex; align-items: center; justify-content: space-between; gap: 4px; flex-wrap: wrap; }
.assistant-settings-workbench { display: flex; flex-direction: column; min-width: 0; gap: 4px; padding: 4px; }
.assistant-settings-sections { min-width: 0; max-width: 100%; border-bottom: 1px solid var(--q-separator-color, #ddd); }
.assistant-settings-detail { min-width: 0; padding: 0; }

.seed-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 4px;
  align-items: start;
}

@media (max-width: 699px) {
  .assistant-settings-sections :deep(.q-tab) { padding: 0 8px; }
  .seed-row {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
