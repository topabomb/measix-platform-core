<script setup lang="ts">
import { useRouter } from 'vue-router'
import StatusChip from './StatusChip.vue'

defineProps<{
  title: string
  subtitle?: string
  status?: string
  breadcrumbs?: { label: string; to?: string }[]
}>()

const router = useRouter()
</script>

<template>
  <div class="page-header q-mb-xs">
    <div class="col-grow" style="min-width: 0">
      <q-breadcrumbs v-if="breadcrumbs?.length" class="q-mb-xs text-grey-6">
        <q-breadcrumbs-el
          v-for="crumb in breadcrumbs"
          :key="crumb.label"
          :label="crumb.label"
          clickable
          @click="crumb.to && router.push(crumb.to)"
        />
      </q-breadcrumbs>
      <div class="row items-center q-gutter-xs">
        <div class="page-header__title text-h5 text-weight-bold" style="min-width: 0">{{ title }}</div>
        <StatusChip v-if="status" :value="status" />
      </div>
      <div v-if="subtitle" class="page-header__subtitle text-body2 text-grey-7">{{ subtitle }}</div>
    </div>
    <!-- Actions: visible inline on sm+, collapsed into dropdown on xs -->
    <div v-if="$slots.actions" class="row items-center q-gutter-xs gt-xs">
      <slot name="actions" />
    </div>
    <!-- Narrow screens: the same actions, one per row. The column wrapper is
         what gives a slotted button its own row and a full-width hit area
         instead of letting buttons flow inline inside the list. -->
    <q-btn-dropdown v-if="$slots.actions" flat dense no-caps auto-close :aria-label="$t('common.actions')" icon="more_vert" dropdown-icon="none" class="xs">
      <q-list>
        <div class="column q-gutter-xs q-pa-xs">
          <slot name="actions" />
        </div>
      </q-list>
    </q-btn-dropdown>
  </div>
</template>
<style scoped>
.page-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; margin-bottom: 12px; }
.page-header > :first-child { flex: 1; min-width: 0; }
.page-header > :last-child:not(:first-child) { flex-shrink: 0; }
.page-header__title { white-space: normal; overflow-wrap: anywhere; }
.page-header__subtitle { margin-top: 2px; }
@media (max-width: 599px) {
 .page-header__title { font-size: 1.3rem; line-height: 1.5; }
 .page-header__subtitle { font-size: .8rem; line-height: 1.5; }
}
</style>
