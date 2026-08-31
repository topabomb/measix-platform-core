<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDraftStore } from '../stores/draft'

defineProps<{ disabled: boolean }>()
const draft = useDraftStore()
const { t } = useI18n()
const selectedId = ref<string>()
const assistants = computed(() => draft.localContent?.assistants ?? [])
const selected = computed(() => assistants.value.find(a => a.assistantDefinitionId === selectedId.value))
const models = computed(() => draft.localContent?.models.filter(m => m.enabled).map(m => ({ label: m.displayName, value: m.modelId })) ?? [])
const mcps = computed(() => draft.localContent?.mcp.filter(m => m.enabled).map(m => ({ label: m.displayName, value: m.mcpServerId })) ?? [])
const starters = computed(() => (draft.localContent?.starters ?? [])
  .filter(s => s.assistantDefinitionId === selectedId.value)
  .toSorted((a, b) => a.sortOrder - b.sortOrder || a.starterId.localeCompare(b.starterId)))

function addAssistant() { selectedId.value = draft.addAssistant(t('experience.newAssistant')) }
function moveSeed(index: number, offset: number) {
  const seeds = selected.value?.memorySeed
  if (!seeds || index + offset < 0 || index + offset >= seeds.length) return
  const [seed] = seeds.splice(index, 1)
  if (seed !== undefined) seeds.splice(index + offset, 0, seed)
  draft.markDirty()
}
function removeAssistant() {
  if (!selected.value) return
  draft.removeAssistant(selected.value.assistantDefinitionId)
  selectedId.value = undefined
}
</script>

<template>
  <section data-cy="experience-editor">
    <q-banner class="bg-blue-1 q-mb-md">{{ t('experience.hint') }}</q-banner>
    <div class="row q-col-gutter-md">
      <div class="col-12 col-md-4">
        <q-btn color="primary" icon="add" :label="t('experience.addAssistant')" :disable="disabled" data-cy="assistant-add" @click="addAssistant" />
        <q-list bordered separator class="q-mt-md">
          <q-item v-for="a in assistants" :key="a.assistantDefinitionId" clickable :active="selectedId === a.assistantDefinitionId" @click="selectedId = a.assistantDefinitionId">
            <q-item-section><q-item-label>{{ a.displayName }}</q-item-label><q-item-label caption>{{ a.assistantDefinitionId }}</q-item-label></q-item-section>
          </q-item>
        </q-list>
      </div>
      <div v-if="selected" class="col-12 col-md-8">
        <q-card flat bordered>
          <q-card-section class="q-gutter-md">
            <q-input v-model="selected.displayName" :label="t('experience.name')" :disable="disabled" data-cy="assistant-name" @update:model-value="draft.markDirty" />
            <q-input v-model="selected.description" :label="t('experience.description')" :disable="disabled" @update:model-value="draft.markDirty" />
            <q-select data-cy="assistant-model" v-model="selected.modelId" :options="models" emit-value map-options :label="t('experience.model')" :disable="disabled" @update:model-value="draft.markDirty" />
            <q-select v-model="selected.mcpServerIds" :options="mcps" multiple emit-value map-options :label="t('experience.mcps')" :disable="disabled" @update:model-value="draft.markDirty" />
            <q-input v-model="selected.systemPrompt" type="textarea" :label="t('experience.systemPrompt')" :disable="disabled" data-cy="assistant-prompt" @update:model-value="draft.markDirty" />
            <q-toggle v-model="selected.enabled" :label="t('experience.enabled')" :disable="disabled" @update:model-value="draft.markDirty" />
            <div class="text-subtitle2">{{ t('experience.memorySeed') }}</div>
            <div v-for="(_, i) in selected.memorySeed" :key="i" class="row items-start q-gutter-sm">
              <q-input v-model="selected.memorySeed[i]" type="textarea" class="col" :label="t('experience.seedEntry', { index: i + 1 })" :disable="disabled" :data-cy="'seed-input-' + i" @update:model-value="draft.markDirty" />
              <q-btn flat icon="arrow_upward" :aria-label="t('experience.moveUp')" :disable="disabled || i === 0" :data-cy="'seed-up-' + i" @click="moveSeed(i, -1)" />
              <q-btn flat icon="arrow_downward" :aria-label="t('experience.moveDown')" :disable="disabled || i === selected.memorySeed.length - 1" @click="moveSeed(i, 1)" />
              <q-btn flat icon="delete" :aria-label="t('common.remove')" :disable="disabled" @click="selected.memorySeed.splice(i, 1); draft.markDirty()" />
            </div>
            <q-btn outline :label="t('experience.addSeed')" :disable="disabled" data-cy="seed-add" @click="selected.memorySeed.push(''); draft.markDirty()" />
          </q-card-section>
          <q-separator />
          <q-card-section>
            <div class="row items-center justify-between q-mb-md">
              <span class="text-subtitle2">{{ t('experience.starters') }}</span>
              <q-btn outline :label="t('experience.addStarter')" :disable="disabled" data-cy="starter-add" @click="draft.addStarter(selected.assistantDefinitionId, t('experience.newStarter'))" />
            </div>
            <q-card v-for="s in starters" :key="s.starterId" flat bordered class="q-mb-md">
              <q-card-section class="q-gutter-sm">
                <div class="text-caption">{{ s.starterId }}</div>
                <q-input data-cy="starter-title" v-model="s.title" :label="t('experience.title')" :disable="disabled" @update:model-value="draft.markDirty" />
                <q-input v-model="s.description" :label="t('experience.description')" :disable="disabled" @update:model-value="draft.markDirty" />
                <q-input data-cy="starter-prompt" v-model="s.prompt" type="textarea" :label="t('experience.starterPrompt')" :disable="disabled" @update:model-value="draft.markDirty" />
                <q-input v-model.number="s.sortOrder" type="number" step="1" :label="t('experience.sortOrder')" :disable="disabled" @update:model-value="draft.markDirty" />
                <q-toggle v-model="s.enabled" :label="t('experience.enabled')" :disable="disabled" @update:model-value="draft.markDirty" />
                <q-btn flat color="negative" :label="t('common.remove')" :disable="disabled" @click="draft.removeStarter(s.starterId)" />
              </q-card-section>
            </q-card>
            <q-btn flat color="negative" :label="t('experience.removeAssistant')" :disable="disabled" @click="removeAssistant" />
          </q-card-section>
        </q-card>
      </div>
    </div>
  </section>
</template>
