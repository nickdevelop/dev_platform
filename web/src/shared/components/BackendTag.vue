<script setup lang="ts">
// Agent backend tag shown on sessions, the chat header and the new-session form. The
// label comes from the platform catalog; the colour follows the backend's
// presentation order so a new backend needs no UI change. A backend the list
// does not know shows its raw id in the neutral tone, never another backend's
// name.
import { computed } from 'vue'
import type { BackendId } from '../../api/agents'
import { useAgentsStore } from '../../stores/agents'

const props = defineProps<{ backend: BackendId }>()

const agents = useAgentsStore()

const label = computed(() => agents.backendLabel(props.backend))

const tone = computed(() => {
  const declaredTone = agents.backendTone(props.backend)
  if (declaredTone) return `tone-${declaredTone}`
  const index = agents.orderedBackends.findIndex((entry) => entry.backend === props.backend)
  if (index === 0) return 'tone-primary'
  if (index === 1) return 'tone-secondary'
  return 'tone-neutral'
})
</script>

<template>
  <span class="tag" :class="tone">{{ label }}</span>
</template>

<style scoped>
.tag {
  font-size: 0.786rem;
  padding: calc(var(--space-unit) * 0.167) calc(var(--space-unit) * 1.000);
  border-radius: 4px;
  white-space: nowrap;
}

.tag.tone-primary,
.tag.tone-info {
  background: var(--color-info-soft);
  color: var(--color-info);
}

.tag.tone-secondary {
  background: var(--color-warning-soft);
  color: var(--color-warning);
}

.tag.tone-neutral {
  background: var(--color-surface-muted, transparent);
  color: var(--color-text-muted, inherit);
}
</style>
