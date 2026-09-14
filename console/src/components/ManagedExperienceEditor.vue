<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDraftStore } from '../stores/draft'

defineProps<{ disabled: boolean }>()
const draft = useDraftStore()
const { t } = useI18n()
const selectedId = ref<string>()
const search = ref('')
const selectedSection = ref<'basic' | 'prompt' | 'memory' | 'connections' | 'starters'>('basic')
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
  { id: 'basic' as const, label: t('experience.sections.basic'), hint: t('experience.sections.basicHint'), icon: 'tune' },
  { id: 'prompt' as const, label: t('experience.sections.prompt'), hint: t('experience.sections.promptHint'), icon: 'chat' },
  { id: 'memory' as const, label: t('experience.sections.memory'), hint: t('experience.sections.memoryHint'), icon: 'psychology', badge: selected.value?.memorySeed.length ?? 0 },
  { id: 'connections' as const, label: t('experience.sections.connections'), hint: t('experience.sections.connectionsHint'), icon: 'hub', badge: selected.value?.mcpServerIds.length ?? 0 },
  { id: 'starters' as const, label: t('experience.sections.starters'), hint: t('experience.sections.startersHint'), icon: 'forum', badge: starters.value.length },
])
const modelUnavailable = computed(() => Boolean(selected.value?.modelId && !models.value.some(model => model.value === selected.value?.modelId)))
const unavailableMcps = computed(() => selected.value?.mcpServerIds.filter(id => !mcps.value.some(mcp => mcp.value === id)) ?? [])
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
</script>

<template>
  <section data-cy="experience-editor">
    <q-banner class="bg-blue-1 q-mb-md rounded-borders">
      <div class="text-weight-medium">{{ t('experience.managedSource') }}</div>
      <div class="text-body2">{{ t('experience.hint') }}</div>
    </q-banner>
    <div class="row q-col-gutter-md">
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
              <q-item-section side><q-icon name="chevron_right" /></q-item-section>
            </q-item>
            <q-item v-if="!assistants.length"><q-item-section class="text-grey-7">{{ t('experience.noAssistants') }}</q-item-section></q-item>
          </q-list>
        </q-card>
      </div>

      <div v-if="selected" class="col-12 col-md-8">
        <q-card flat bordered>
          <q-card-section class="row items-start justify-between q-gutter-sm">
            <div>
              <div class="text-h6">{{ selected.displayName }}</div>
              <div class="text-caption text-grey-7">{{ t('experience.managedIdentity') }} · {{ selected.assistantDefinitionId }}</div>
            </div>
            <q-toggle v-model="selected.enabled" :label="t('experience.enabled')" :disable="disabled" @update:model-value="draft.markDirty" />
          </q-card-section>
          <q-separator />

          <div class="assistant-settings-workbench">
            <q-list bordered separator class="assistant-settings-sections">
              <q-item v-for="section in sections" :key="section.id" clickable :active="selectedSection === section.id" active-class="bg-blue-1 text-primary" :data-cy="`assistant-section-${section.id}`" @click="selectedSection = section.id">
                <q-item-section avatar><q-icon :name="section.icon" /></q-item-section>
                <q-item-section>
                  <q-item-label>{{ section.label }}</q-item-label>
                  <q-item-label caption>{{ section.hint }}</q-item-label>
                </q-item-section>
                <q-item-section side><q-badge v-if="section.badge !== undefined" color="grey-3" text-color="grey-9" :label="section.badge" /></q-item-section>
              </q-item>
            </q-list>

            <q-card-section class="assistant-settings-detail q-gutter-md">
              <template v-if="selectedSection === 'basic'">
                <div class="text-subtitle1">{{ t('experience.sections.basic') }}</div>
                <q-input v-model="selected.displayName" outlined :label="t('experience.name')" :disable="disabled" data-cy="assistant-name" @update:model-value="draft.markDirty" />
                <q-input v-model="selected.description" outlined :label="t('experience.description')" :disable="disabled" @update:model-value="draft.markDirty" />
                <q-btn flat color="negative" icon="delete" :label="t('experience.removeAssistant')" :disable="disabled" @click="removeAssistant" />
              </template>

              <template v-else-if="selectedSection === 'prompt'">
                <div class="text-subtitle1">{{ t('experience.sections.prompt') }}</div>
                <div class="text-body2 text-grey-7">{{ t('experience.promptHint') }}</div>
                <q-input v-model="selected.systemPrompt" outlined autogrow type="textarea" :label="t('experience.systemPrompt')" :disable="disabled" data-cy="assistant-prompt" @update:model-value="draft.markDirty" />
              </template>

              <template v-else-if="selectedSection === 'memory'">
                <div class="text-subtitle1">{{ t('experience.memorySeed') }}</div>
                <div class="text-body2 text-grey-7">{{ t('experience.memoryHint') }}</div>
                <div v-for="(_, i) in selected.memorySeed" :key="seedKeyAt(i)" class="seed-row">
                  <q-input v-model="selected.memorySeed[i]" outlined autogrow type="textarea" :label="t('experience.seedEntry', { index: i + 1 })" :disable="disabled" :data-cy="'seed-input-' + i" @update:model-value="draft.markDirty" />
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
                <q-select data-cy="assistant-model" v-model="selected.modelId" outlined :options="models" emit-value map-options :label="t('experience.model')" :disable="disabled" @update:model-value="draft.markDirty" />
                <q-banner v-if="modelUnavailable" class="bg-red-1 rounded-borders text-negative">{{ t('experience.unavailableModel', { id: selected.modelId }) }}</q-banner>
                <q-select v-model="selected.mcpServerIds" outlined :options="mcps" multiple use-chips emit-value map-options :label="t('experience.mcps')" :disable="disabled" @update:model-value="draft.markDirty" />
                <q-banner v-if="unavailableMcps.length" class="bg-red-1 rounded-borders text-negative">{{ t('experience.unavailableMcps', { ids: unavailableMcps.join(', ') }) }}</q-banner>
              </template>

              <template v-else>
                <div class="row items-center justify-between q-gutter-sm">
                  <div>
                    <div class="text-subtitle1">{{ t('experience.starters') }}</div>
                    <div class="text-body2 text-grey-7">{{ t('experience.startersHint') }}</div>
                  </div>
                  <q-btn outline icon="add" :label="t('experience.addStarter')" :disable="disabled" data-cy="starter-add" @click="draft.addStarter(selected.assistantDefinitionId, t('experience.newStarter'))" />
                </div>
                <q-card v-for="s in starters" :key="s.starterId" flat bordered>
                  <q-card-section class="q-gutter-sm">
                    <div class="text-caption text-grey-7">{{ s.starterId }}</div>
                    <q-input data-cy="starter-title" v-model="s.title" outlined :label="t('experience.title')" :disable="disabled" @update:model-value="draft.markDirty" />
                    <q-input v-model="s.description" outlined :label="t('experience.description')" :disable="disabled" @update:model-value="draft.markDirty" />
                    <q-input data-cy="starter-prompt" v-model="s.prompt" outlined autogrow type="textarea" :label="t('experience.starterPrompt')" :disable="disabled" @update:model-value="draft.markDirty" />
                    <div class="row items-center q-col-gutter-md">
                      <q-input v-model.number="s.sortOrder" outlined dense type="number" step="1" :label="t('experience.sortOrder')" :disable="disabled" class="col-12 col-sm-6" @update:model-value="draft.markDirty" />
                      <q-toggle v-model="s.enabled" :label="t('experience.enabled')" :disable="disabled" class="col-12 col-sm-6" @update:model-value="draft.markDirty" />
                    </div>
                    <q-btn flat color="negative" icon="delete" :label="t('common.remove')" :disable="disabled" @click="draft.removeStarter(s.starterId)" />
                  </q-card-section>
                </q-card>
              </template>
            </q-card-section>
          </div>
        </q-card>
      </div>
    </div>
  </section>
</template>

<style scoped>
.assistant-settings-workbench {
  display: grid;
  grid-template-columns: minmax(180px, 240px) minmax(0, 1fr);
  gap: 16px;
  padding: 16px;
}

.assistant-settings-sections {
  align-self: start;
  border-radius: 10px;
}

.assistant-settings-detail {
  min-width: 0;
  padding: 0;
}

.seed-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  align-items: start;
}

@media (max-width: 1399px) {
  .assistant-settings-workbench {
    grid-template-columns: minmax(0, 1fr);
  }

  .assistant-settings-sections {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 699px) {
  .assistant-settings-sections,
  .seed-row {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
