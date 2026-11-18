<script setup lang="ts">
export type ConfigurationSection = {
  id: string
  label: string
  description: string
  icon: string
  badge?: string | number
}

const props = defineProps<{
  modelValue: string
  title: string
  subtitle: string
  items: ConfigurationSection[]
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

function select(value: string) {
  emit('update:modelValue', value)
}
</script>

<template>
  <nav class="configuration-section-nav" data-cy="configuration-section-nav" :aria-label="title">
    <q-card flat bordered class="configuration-section-nav__desktop">
      <q-card-section class="q-pa-sm">
        <div class="text-subtitle2">{{ title }}</div>
        <div class="text-caption text-grey-7">{{ subtitle }}</div>
      </q-card-section>
      <q-separator />
      <q-list dense>
        <q-item
          v-for="item in items"
          :key="item.id"
          clickable
          :active="modelValue === item.id"
          active-class="bg-blue-1 text-primary"
          :data-cy="`config-section-${item.id}`"
          @click="select(item.id)"
        >
          <q-item-section avatar><q-icon :name="item.icon" /></q-item-section>
          <q-item-section>
            <q-item-label>{{ item.label }}</q-item-label>
            <q-item-label caption lines="2">{{ item.description }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-badge v-if="item.badge !== undefined" color="grey-3" text-color="grey-9" :label="item.badge" />
            <q-icon name="chevron_right" size="18px" />
          </q-item-section>
        </q-item>
      </q-list>
    </q-card>

    <div class="configuration-section-nav__mobile">
      <div class="text-caption text-grey-7 q-mb-xs">{{ title }}</div>
    <q-select
      outlined
      dense
      emit-value
      map-options
      :aria-label="title"
      :model-value="props.modelValue"
      :options="items.map(item => ({ label: item.label, value: item.id, description: item.description, icon: item.icon }))"
      @update:model-value="select"
    >
      <template #selected-item="scope">
        <q-item dense class="q-pa-none">
          <q-item-section avatar><q-icon :name="scope.opt.icon" /></q-item-section>
          <q-item-section>
            <q-item-label>{{ scope.opt.label }}</q-item-label>
            <q-item-label caption>{{ scope.opt.description }}</q-item-label>
          </q-item-section>
        </q-item>
      </template>
    </q-select>
    </div>
  </nav>
</template>

<style scoped>
.configuration-section-nav {
  min-width: 0;
}

.configuration-section-nav__desktop {
  position: sticky;
  /* Header (50px) + the single page margin (4px). */
  top: 54px;
}

.configuration-section-nav__desktop .q-item {
  min-height: 44px;
  padding: 3px 8px;
  border-radius: 4px;
}

.configuration-section-nav__desktop :deep(.q-item__section--avatar) {
  min-width: 30px;
}

.configuration-section-nav__mobile {
  display: none;
}

@media (max-width: 899px) {
  .configuration-section-nav__desktop {
    display: none;
  }

  .configuration-section-nav__mobile {
    display: block;
  }
}
</style>
