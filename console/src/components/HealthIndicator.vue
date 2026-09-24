<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useSystemHealth } from '../composables/useSystemHealth'
import StatusChip from './StatusChip.vue'

// Global Header health indicator (product §4.1): surfaces a high-priority
// runtime degraded / Relay-down / unpublished state persistently, with text
// (never color alone). A healthy runtime shows a plain READY chip.
const { t: $t } = useI18n()
const { degraded, status } = useSystemHealth()

const relayRevision = () => (status.value?.desiredControlRevision != null ? ` · r${status.value.desiredControlRevision}` : '')
</script>

<template>
  <StatusChip v-if="degraded === 'degraded'" value="DEGRADED" :label="$t('health.degraded')" />
  <StatusChip v-else-if="degraded === 'unconfigured'" value="NOT_CONFIGURED" :label="$t('health.notConfigured')" />
  <StatusChip v-else-if="degraded === 'relay'" value="NOT_READY" :label="`${$t('health.relayDown')}${relayRevision()}`" />
  <StatusChip v-else-if="!status" value="UNKNOWN" />
  <StatusChip v-else value="READY" />
</template>
