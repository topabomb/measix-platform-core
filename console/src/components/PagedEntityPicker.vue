<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

export interface EntityPickerOption {
  value: string
  label: string
  caption?: string
  status?: string
  metadata?: Record<string, string | number | boolean>
}

export interface EntityPickerPage {
  items: EntityPickerOption[]
  nextCursor?: string
}

const props = withDefaults(defineProps<{
  modelValue?: string
  label: string
  emptyLabel: string
  fetchPage: (query: string, cursor?: string) => Promise<EntityPickerPage>
  resolveOption?: (value: string) => Promise<EntityPickerOption | undefined>
  selectedOption?: EntityPickerOption
  disabled?: boolean
  clearable?: boolean
}>(), {
  modelValue: undefined,
  resolveOption: undefined,
  selectedOption: undefined,
  disabled: false,
  clearable: true,
})

const emit = defineEmits<{
  'update:modelValue': [value: string | undefined]
  selected: [option: EntityPickerOption | undefined]
}>()

const { t: $t } = useI18n()
const open = ref(false)
const query = ref('')
const options = ref<EntityPickerOption[]>([])
const nextCursor = ref<string>()
const loading = ref(false)
const error = ref<unknown>()
const selected = ref<EntityPickerOption>()
let requestSequence = 0
let searchTimer: ReturnType<typeof setTimeout> | undefined

const displayLabel = computed(() => selected.value?.label ?? props.selectedOption?.label ?? props.modelValue ?? props.emptyLabel)

async function resolveSelection(value: string | undefined) {
  if (!value) {
    selected.value = undefined
    return
  }
  const local = props.selectedOption?.value === value
    ? props.selectedOption
    : options.value.find(option => option.value === value)
  if (local) {
    selected.value = local
    return
  }
  if (!props.resolveOption) return
  const current = ++requestSequence
  try {
    const option = await props.resolveOption(value)
    if (current === requestSequence && props.modelValue === value) {
      selected.value = option
      emit('selected', option)
    }
  } catch (cause) {
    if (current === requestSequence) error.value = cause
  }
}

async function searchEntities(term = query.value) {
  const current = ++requestSequence
  query.value = term
  loading.value = true
  error.value = undefined
  try {
    const page = await props.fetchPage(term.trim(), undefined)
    if (current !== requestSequence) return
    options.value = page.items
    nextCursor.value = page.nextCursor
  } catch (cause) {
    if (current === requestSequence) error.value = cause
  } finally {
    if (current === requestSequence) loading.value = false
  }
}

async function loadMore() {
  if (!nextCursor.value || loading.value) return
  const cursor = nextCursor.value
  const current = ++requestSequence
  loading.value = true
  error.value = undefined
  try {
    const page = await props.fetchPage(query.value.trim(), cursor)
    if (current !== requestSequence) return
    const known = new Set(options.value.map(option => option.value))
    options.value.push(...page.items.filter(option => !known.has(option.value)))
    nextCursor.value = page.nextCursor
  } catch (cause) {
    if (current === requestSequence) error.value = cause
  } finally {
    if (current === requestSequence) loading.value = false
  }
}

async function openPicker() {
  if (props.disabled) return
  open.value = true
  await searchEntities(query.value)
}

function choose(option: EntityPickerOption) {
  selected.value = option
  emit('update:modelValue', option.value)
  emit('selected', option)
  open.value = false
}

function clear() {
  selected.value = undefined
  emit('update:modelValue', undefined)
  emit('selected', undefined)
}

watch(query, () => {
  if (!open.value) return
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { void searchEntities(query.value) }, 250)
})

watch(() => props.modelValue, value => { void resolveSelection(value) }, { immediate: true })
watch(() => props.selectedOption, option => {
  if (option?.value === props.modelValue) selected.value = option
}, { immediate: true })

onBeforeUnmount(() => {
  requestSequence++
  if (searchTimer) clearTimeout(searchTimer)
})

defineExpose({ openPicker, searchEntities, loadMore, choose, clear })
</script>

<template>
  <div class="entity-picker-root">
  <q-field
    outlined
    dense
    stack-label
    :label="label"
    :disable="disabled"
    class="entity-picker cursor-pointer"
  >
    <template #control>
      <div
        class="self-center full-width ellipsis"
        :class="{ 'text-grey-7': !modelValue }"
        data-cy="entity-picker-trigger"
        role="button"
        tabindex="0"
        @click="openPicker"
        @keydown.enter.prevent="openPicker"
        @keydown.space.prevent="openPicker"
      >{{ displayLabel }}</div>
    </template>
    <template #append>
      <q-btn v-if="modelValue && clearable" flat round dense size="sm" icon="close" :aria-label="$t('common.remove')" @click.stop="clear" />
      <q-icon class="cursor-pointer" name="search" size="18px" @click="openPicker" />
    </template>
  </q-field>

  <q-dialog v-model="open">
    <q-card class="app-dialog entity-picker__dialog" data-cy="entity-picker-dialog">
      <q-card-section class="row items-center q-gutter-xs q-py-xs">
        <div class="text-subtitle1 text-weight-medium">{{ label }}</div>
        <q-space />
        <q-btn flat round dense icon="close" :aria-label="$t('common.close')" v-close-popup />
      </q-card-section>
      <q-separator />
      <q-card-section class="q-pa-xs">
        <q-input v-model="query" outlined dense clearable autofocus :label="$t('common.search')" data-cy="entity-picker-search">
          <template #prepend><q-icon name="search" /></template>
        </q-input>
      </q-card-section>
      <q-separator />
      <q-card-section class="entity-picker__results q-pa-none">
        <div v-if="loading && !options.length" class="row justify-center q-pa-md"><q-spinner /></div>
        <div v-else-if="error" class="text-negative q-pa-sm" role="alert">{{ String(error) }}</div>
        <q-list v-else separator dense>
          <q-item
            v-for="option in options"
            :key="option.value"
            clickable
            :active="option.value === modelValue"
            active-class="bg-blue-1 text-primary"
            data-cy="entity-picker-option"
            @click="choose(option)"
          >
            <q-item-section>
              <q-item-label>{{ option.label }}</q-item-label>
              <q-item-label v-if="option.caption" caption>{{ option.caption }}</q-item-label>
            </q-item-section>
            <q-item-section v-if="option.status" side><q-item-label caption>{{ option.status }}</q-item-label></q-item-section>
            <q-item-section v-if="option.value === modelValue" side><q-icon name="check" color="primary" /></q-item-section>
          </q-item>
          <q-item v-if="!options.length"><q-item-section class="text-grey-7">{{ $t('common.noData') }}</q-item-section></q-item>
        </q-list>
      </q-card-section>
      <q-separator />
      <q-card-actions align="between" class="q-px-xs">
        <div class="text-caption text-grey-7">{{ $t('common.loadedCount', { count: options.length }) }}</div>
        <q-btn v-if="nextCursor" flat color="primary" :label="$t('common.loadMore')" :loading="loading" data-cy="entity-picker-load-more" @click="loadMore" />
      </q-card-actions>
    </q-card>
  </q-dialog>
  </div>
</template>

<style scoped>
.entity-picker {
  min-width: 0;
}

.entity-picker-root {
  min-width: 0;
}

.entity-picker__dialog {
  width: min(620px, 94vw);
  max-width: 94vw;
}

.entity-picker__results {
  min-height: 160px;
  max-height: min(55vh, 480px);
  overflow: auto;
}
</style>
