<script setup lang="ts">
defineProps<{ name: string; description: unknown; fallback: string }>()
</script>

<template>
  <div class="mcp-tool-summary">
    <div class="mcp-tool-name text-weight-medium" :title="name">{{ name }}</div>
    <div class="mcp-tool-description text-caption text-grey-7">{{ typeof description === 'string' && description.trim() ? description : fallback }}</div>
  </div>
</template>

<style scoped>
.mcp-tool-summary { min-width: 0; max-width: 100%; width: 100%; flex: 1; overflow: hidden; }
.mcp-tool-name, .mcp-tool-description { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
</style>
