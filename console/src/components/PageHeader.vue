<script setup lang="ts">
import { useRouter } from 'vue-router'
import StatusChip from './StatusChip.vue'

// PageHeader (implementation §3.3): consistent page header for every business
// page — breadcrumbs on deep routes, title + concise context, authoritative
// status, primary + secondary actions. Narrow screens keep only the primary
// action and push the rest into an overflow menu.

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
    <div class="row items-center q-gutter-sm">
      <!-- Primary action: exactly one visual primary button per page. -->
      <slot name="primary" />
      <!-- Secondary actions on wide screens. -->
      <slot name="actions" />
      <!-- On narrow screens the overflow menu carries secondary actions. -->
      <q-btn-dropdown v-if="$slots.actions" flat round icon="more_vert" class="lt-md">
        <q-list>
          <slot name="actions" />
        </q-list>
      </q-btn-dropdown>
    </div>
  </div>
</template>
