<script setup lang="ts">
defineProps<{
  detailOpen: boolean
  /** Keeps a dense but usable collection rail on wide screens. */
  listWidth?: string
}>()
</script>

<template>
  <div
    class="detail-workspace"
    :class="{ 'detail-workspace--open': detailOpen }"
    :style="{ '--detail-list-width': listWidth ?? 'minmax(280px, 360px)' }"
    data-cy="detail-workspace"
  >
    <div class="detail-workspace__list" data-cy="detail-workspace-list">
      <slot name="list" />
    </div>
    <div v-if="detailOpen" class="detail-workspace__detail" data-cy="detail-workspace-detail">
      <slot name="detail" />
    </div>
  </div>
</template>

<style scoped>
.detail-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 4px;
  align-items: start;
  min-width: 0;
}

.detail-workspace--open {
  grid-template-columns: var(--detail-list-width) minmax(0, 1fr);
}

.detail-workspace__list,
.detail-workspace__detail {
  min-width: 0;
}

@media (max-width: 1023px) {
  .detail-workspace--open {
    grid-template-columns: minmax(0, 1fr);
  }

  .detail-workspace--open .detail-workspace__list {
    display: none;
  }
}
</style>
