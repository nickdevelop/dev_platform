<script setup lang="ts">
// Desktop-style line-count badge: green +N for additions, red -N for
// deletions. Renders nothing when there are no counted changes, mirroring
// opencode's DiffChanges total() > 0 gate.
import { computed } from 'vue'

const props = defineProps<{ additions?: number; deletions?: number }>()

const additions = computed(() => (typeof props.additions === 'number' && props.additions > 0 ? props.additions : 0))
const deletions = computed(() => (typeof props.deletions === 'number' && props.deletions > 0 ? props.deletions : 0))
const visible = computed(() => additions.value + deletions.value > 0)
</script>

<template>
  <span v-if="visible" class="diff-badge" data-test="diff-badge">
    <span v-if="additions > 0" class="diff-additions" data-test="diff-additions">+{{ additions }}</span>
    <span v-if="deletions > 0" class="diff-deletions" data-test="diff-deletions">-{{ deletions }}</span>
  </span>
</template>

<style scoped>
.diff-badge {
  flex-shrink: 0;
  display: inline-flex;
  align-items: baseline;
  gap: 0.35rem;
  font-size: 0.78rem;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.diff-additions {
  color: var(--color-success, #16a34a);
}

.diff-deletions {
  color: var(--color-danger, #dc2626);
}
</style>
