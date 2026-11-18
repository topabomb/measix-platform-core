<script setup lang="ts">
withDefaults(defineProps<{ compact?: boolean; subtitle?: boolean }>(), { compact: false, subtitle: false })
</script>

<template>
  <div class="orchelm-brand" :class="{ 'orchelm-brand--compact': compact }">
    <svg viewBox="0 0 64 64" fill="currentColor" aria-hidden="true">
      <path d="M35.118 9.800L49.667 18.200Q52.785 20.000 52.785 23.600L52.785 40.400Q52.785 44.000 49.667 45.800L35.118 54.200Q32.000 56.000 28.882 54.200L14.333 45.800Q11.215 44.000 11.215 40.400L11.215 23.600Q11.215 20.000 14.333 18.200L28.882 9.800Q32.000 8.000 35.118 9.800Z" fill="none" stroke="currentColor" stroke-width="3.8" />
      <g transform="rotate(0 32 32)"><path d="M32 9.3L35.8 22.7" fill="none" stroke="currentColor" stroke-width="3.8" stroke-linecap="round" /></g>
      <g transform="rotate(120 32 32)"><path d="M32 9.3L35.8 22.7" fill="none" stroke="currentColor" stroke-width="3.8" stroke-linecap="round" /></g>
      <g transform="rotate(240 32 32)"><path d="M32 9.3L35.8 22.7" fill="none" stroke="currentColor" stroke-width="3.8" stroke-linecap="round" /></g>
      <path d="M33.732 21.500L40.227 25.250Q41.959 26.250 41.959 28.250L41.959 35.750Q41.959 37.750 40.227 38.750L33.732 42.500Q32.000 43.500 30.268 42.500L23.773 38.750Q22.041 37.750 22.041 35.750L22.041 28.250Q22.041 26.250 23.773 25.250L30.268 21.500Q32.000 20.500 33.732 21.500Z" transform="rotate(18 32 32)" />
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
