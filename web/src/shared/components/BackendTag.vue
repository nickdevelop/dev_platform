<script setup lang="ts">
// Agent backend tag shown on sessions, the chat header and the new-session form. The
// label comes from the platform catalog; the colour follows the backend's
// declared brand colour when present, otherwise its presentation order so a new
// backend needs no UI change. A backend the list does not know shows its raw id
// in the neutral tone, never another backend's name.
import { computed } from 'vue'
import type { BackendId } from '../../api/agents'
import { useAgentsStore } from '../../stores/agents'
import { contrastTextColor } from '../../shared/color'

const props = defineProps<{ backend: BackendId }>()

const agents = useAgentsStore()

const label = computed(() => agents.backendLabel(props.backend))

const declaredColor = computed(() => agents.backendColor(props.backend))

const tone = computed(() => {
  const declaredTone = agents.backendTone(props.backend)
  if (declaredTone) return `tone-${declaredTone}`
  const index = agents.orderedBackends.findIndex((entry) => entry.backend === props.backend)
  if (index === 0) return 'tone-primary'
  if (index === 1) return 'tone-secondary'
  return 'tone-neutral'
})

// A declared brand colour is rendered as a solid fill. Light brand colours
// (emerald, orange) need dark ink while dark ones (navy) need white, so the
// foreground is computed from luminance rather than fixed per backend.
const tagStyle = computed(() =>
  declaredColor.value
    ? { backgroundColor: declaredColor.value, color: contrastTextColor(declaredColor.value) }
    : undefined,
)
</script>

<template>
  <span
    class="tag"
    :class="declaredColor ? 'has-color' : tone"
    :style="tagStyle"
    :data-backend-color="declaredColor || undefined"
  >{{ label }}</span>
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

/* The fill and ink come from the descriptor through an inline style; this rule
   only marks the variant so the tone classes are not applied. */
.tag.has-color {
  font-weight: 500;
}
</style>
