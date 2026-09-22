<script setup lang="ts">
withDefaults(defineProps<{ compact?: boolean; subtitle?: boolean }>(), { compact: false, subtitle: false })
</script>

<template>
  <div class="orchelm-brand" :class="{ 'orchelm-brand--compact': compact }">
    <svg viewBox="0 0 64 64" aria-hidden="true">
      <path d="M32 8 54 21v24L32 58 10 45V21L32 8Z" fill="none" stroke="currentColor" stroke-width="3" />
      <path d="m32 20 12 7v13l-12 7-12-7V27l12-7Z" fill="currentColor" />
      <path d="M32 8v10m22 27-9-5m-35 5 9-5" stroke="currentColor" stroke-width="4" />
      <circle cx="32" cy="8" r="4" fill="currentColor" />
      <circle cx="54" cy="45" r="4" fill="currentColor" />
      <circle cx="10" cy="45" r="4" fill="currentColor" />
    </svg>
    <div class="orchelm-brand__copy">
      <div class="orchelm-brand__name"><strong>枢策</strong><span>Orchelm</span></div>
      <div v-if="subtitle" class="orchelm-brand__subtitle">{{ $t('brand.description') }}</div>
    </div>
  </div>
</template>

<style scoped>
.orchelm-brand { display: flex; align-items: center; gap: 8px; color: var(--q-primary); min-width: 0; }
.orchelm-brand svg { width: 34px; height: 34px; flex: 0 0 34px; }
.orchelm-brand__copy { min-width: 0; line-height: 1.15; }
.orchelm-brand__name { display: flex; align-items: baseline; gap: 6px; white-space: nowrap; }
.orchelm-brand__name strong { font-size: 17px; letter-spacing: .04em; }
.orchelm-brand__name span { font-size: 13px; font-weight: 650; color: #514564; }
.orchelm-brand__subtitle { margin-top: 3px; font-size: 11px; color: #756d7e; white-space: nowrap; }
.orchelm-brand--compact svg { width: 28px; height: 28px; flex-basis: 28px; }
.orchelm-brand--compact .orchelm-brand__name strong { font-size: 15px; }
@media (max-width: 420px) { .orchelm-brand__name span { display: none; } }
</style>
