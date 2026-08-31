<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useSystemHealth } from '../composables/useSystemHealth'
import StatusChip from './StatusChip.vue'

// Global Header health indicator (product §4.1): surfaces a high-priority
// runtime degraded / Relay-down state persistently, with text (never color
// alone). Renders nothing while healthy so the header stays calm.
const { t: $t } = useI18n()
const { degraded, status } = useSystemHealth()

const relayRevision = () => (status.value?.desiredControlRevision != null ? ` · r${status.value.desiredControlRevision}` : '')
</script>

<template>
  <StatusChip v-if="degraded === 'degraded'" :value="$t('health.degraded')" />
  <StatusChip v-else-if="degraded === 'relay'" :value="`${$t('health.relayDown')}${relayRevision()}`" />
  <StatusChip v-else :value="$t('status.READY')" />
</template>
