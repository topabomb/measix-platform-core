<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
defineProps<{ tool?: components['schemas']['McpDiscoveredTool'] }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
</script>

<template>
  <q-dialog :model-value="Boolean(tool)" @update:model-value="open => { if (!open) emit('close') }">
    <q-card class="app-dialog mcp-tool-dialog" data-cy="mcp-tool-dialog">
      <q-card-section class="row no-wrap items-center q-gutter-sm"><div class="text-subtitle1 mcp-tool-name">{{ tool?.name }}</div><q-space /><q-btn flat round dense icon="close" :aria-label="t('common.close')" v-close-popup /></q-card-section>
      <q-separator />
      <q-card-section class="mcp-tool-contract"><div class="text-caption">{{ t('mcpTools.untrusted') }}</div><pre>{{ JSON.stringify(tool?.definition, null, 2) }}</pre><div class="text-caption">{{ tool?.contractHash }}</div></q-card-section>
    </q-card>
  </q-dialog>
</template>

<style scoped>
.mcp-tool-dialog { width: min(760px, 94vw); max-width: 94vw; }
.mcp-tool-name { min-width: 0; overflow-wrap: anywhere; }
.mcp-tool-contract { max-height: 70vh; min-height: 0; overflow: auto; overflow-wrap: anywhere; }
.mcp-tool-contract pre { white-space: pre-wrap; overflow-wrap: anywhere; font-size: 12px; }
</style>
