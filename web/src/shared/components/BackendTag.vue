<script setup lang="ts">
// Agent backend tag shown on sessions, the chat header and the new-session form. The
// label comes from the platform catalog; the colour follows the backend's
// declared brand colour when present, otherwise its presentation order so a new
// backend needs no UI change. A backend the list does not know shows its raw id
// in the neutral tone, never another backend's name.
import { computed } from 'vue'
import type { BackendId } from '../../api/agents'
import { useAgentsStore } from '../../stores/agents'

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

// The descriptor supplies the exact brand hue. CSS mixes that hue into a soft
// surface and keeps the text in the brand colour, matching the existing tag
// style instead of turning the tag into a solid chip.
const tagStyle = computed<Record<string, string>>(() =>
  declaredColor.value ? { '--tag-base': declaredColor.value } : {},
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

/* Soft brand tint plus brand-coloured text, the same shape as the tone tags.
   On a dark palette the hue is mixed with the theme's text colour so a dark
   navy (DeepSeek) stays readable without losing its identity. */
.tag.has-color {
  --tag-base: var(--color-info);
  --tag-color: var(--tag-base);
  --tag-surface: color-mix(in srgb, var(--tag-base) 14%, var(--color-surface));
  background: var(--tag-surface);
  color: var(--tag-color);
}

:global([data-theme='dark'] .tag.has-color) {
  --tag-color: color-mix(in srgb, var(--tag-base) 65%, var(--color-text));
  --tag-surface: color-mix(in srgb, var(--tag-base) 18%, var(--color-surface));
}
</style>
