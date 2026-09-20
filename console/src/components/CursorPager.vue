<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  page: number
  count: number
  hasNext: boolean
  loading?: boolean
}>()

defineEmits<{
  previous: []
  next: []
}>()

const { t: $t } = useI18n()
</script>

<template>
  <div class="cursor-pager row items-center justify-between q-px-sm q-py-xs" data-cy="cursor-pager">
    <div class="text-caption text-grey-7">
      {{ $t('common.pageStatus', { page, count }) }}
      · {{ hasNext ? $t('common.hasMore') : $t('common.lastPage') }}
    </div>
    <div class="row items-center q-gutter-xs">
      <q-btn flat round dense icon="chevron_left" :aria-label="$t('common.previousPage')" :disable="page <= 1 || loading" @click="$emit('previous')" />
      <q-btn flat round dense icon="chevron_right" :aria-label="$t('common.nextPage')" :disable="!hasNext || loading" @click="$emit('next')" />
    </div>
  </div>
</template>

<style scoped>
.cursor-pager {
  min-height: 36px;
}
</style>
