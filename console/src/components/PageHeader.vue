<script setup lang="ts">
import { useRouter } from 'vue-router'
import StatusChip from './StatusChip.vue'

// PageHeader — consistent page header for every business page.
//
// Design:
//   - Left: title + optional subtitle, breadcrumbs, status chip.
//   - Right: #actions slot — all page-level action buttons go here.
//   - Wide screens (sm+): actions rendered inline.
//   - Narrow screens (xs): actions collapsed into an overflow dropdown.
//
// Usage:
//   <PageHeader title="..." :subtitle="...">
//     <template #actions>
//       <q-btn ... />
//       <q-btn ... />
//     </template>
//   </PageHeader>

defineProps<{
  title: string
  subtitle?: string
  status?: string
  breadcrumbs?: { label: string; to?: string }[]
}>()

const router = useRouter()
</script>

<template>
  <div class="page-header row items-center justify-between q-mb-lg q-gutter-sm">
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
      <div class="row items-center q-gutter-sm">
        <div class="text-h5 text-weight-bold text-no-wrap" style="min-width: 0">{{ title }}</div>
        <StatusChip v-if="status" :value="status" />
      </div>
      <div v-if="subtitle" class="text-body2 text-grey-7">{{ subtitle }}</div>
    </div>
    <!-- Actions: visible inline on sm+, collapsed into dropdown on xs -->
    <div v-if="$slots.actions" class="row items-center q-gutter-sm gt-xs">
      <slot name="actions" />
    </div>
    <q-btn-dropdown v-if="$slots.actions" flat round dense icon="more_vert" class="xs">
      <q-list>
        <slot name="actions" />
      </q-list>
    </q-btn-dropdown>
  </div>
</template>
