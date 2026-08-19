<script setup lang="ts">
defineProps<{ label?: string }>()
</script>

<template>
  <div class="row items-center q-gutter-sm q-pa-md text-grey-7">
    <q-spinner size="24px" />
    <span>{{ label ?? 'Loading…' }}</span>
  </div>
</template>
