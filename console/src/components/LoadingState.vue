<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t: $t } = useI18n()
defineProps<{ label?: string }>()
</script>

<template>
  <div class="row items-center q-gutter-xs q-pa-xs text-grey-7">
    <q-spinner size="24px" />
    <span>{{ label ?? $t('common.loading') }}</span>
  </div>
</template>
