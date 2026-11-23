<script setup lang="ts">
// An agent backend's logo: the official SVG the descriptor declares, shown in
// place of the backend's short name. A backend without a logo, or a logo that
// fails to load, keeps the short name on a chip coloured from the catalog, so
// the identity never disappears. Nothing here branches on a backend id.
import { computed, ref, watch } from 'vue'
import type { BackendId } from '../../api/agents'
import { useAgentsStore } from '../../stores/agents'

const props = withDefaults(
  defineProps<{
    backend: BackendId
    size?: 'sm' | 'md'
  }>(),
  { size: 'md' },
)

const agents = useAgentsStore()
const failed = ref(false)

watch(
  () => props.backend,
  () => {
    failed.value = false
  },
)

const iconUrl = computed(() => (failed.value ? '' : agents.backendIconUrl(props.backend)))
const shortName = computed(() => agents.backendShortName(props.backend))
const declaredColor = computed(() => agents.backendColor(props.backend))

const chipTone = computed(() => {
  const declaredTone = agents.backendTone(props.backend)
  return declaredTone ? `tone-${declaredTone}` : 'tone-neutral'
})

const chipStyle = computed<Record<string, string>>((): Record<string, string> =>
  declaredColor.value ? { '--logo-base': declaredColor.value } : {},
)
</script>

<template>
  <span
    class="agent-logo"
    :class="[
      `size-${props.size}`,
      iconUrl ? 'has-icon' : declaredColor ? 'has-color' : chipTone,
    ]"
    :style="iconUrl ? undefined : chipStyle"
    :data-test="`agent-logo-${props.backend}`"
  >
    <img
      v-if="iconUrl"
      class="agent-logo-img"
      :src="iconUrl"
      alt=""
      draggable="false"
      @error="failed = true"
    />
    <svg
      v-else-if="props.backend === 'group-flow'"
      viewBox="0 0 16 16"
      width="13"
      height="13"
      fill="none"
      stroke="currentColor"
      stroke-width="1.8"
      class="text-blue-500 shrink-0"
      aria-label="团队协同"
    >
      <circle cx="8" cy="8" r="2.8" />
      <path d="M8 1v2.5M8 12.5V15M1 8h2.5M12.5 8H15M3 3l2 2M11 11l2 2M3 13l2-2M11 5l2-2" />
    </svg>
    <template v-else>{{ shortName }}</template>
  </span>
</template>

<style scoped>
.agent-logo {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  user-select: none;
}

.agent-logo.has-icon {
  line-height: 0;
}

.agent-logo-img {
  display: block;
  width: 14px;
  height: 14px;
  object-fit: contain;
}

.agent-logo.size-sm .agent-logo-img {
  width: 12px;
  height: 12px;
}

.agent-logo:not(.has-icon) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: var(--text-size-10);
  font-weight: 700;
  line-height: 1;
  padding: 1.5px 3.5px;
  border-radius: 3px;
  letter-spacing: 0.5px;
}

.agent-logo.has-color:not(.has-icon) {
  --logo-base: var(--color-info);
  --logo-color: var(--logo-base);
  --logo-surface: color-mix(in srgb, var(--logo-base) 14%, var(--color-surface, #fff));
  background: var(--logo-surface);
  color: var(--logo-color);
  border: 1px solid color-mix(in srgb, var(--logo-base) 25%, transparent);
}

:global([data-theme='dark'] .agent-logo.has-color:not(.has-icon)) {
  --logo-color: color-mix(in srgb, var(--logo-base) 75%, var(--color-text, #fff));
  --logo-surface: color-mix(in srgb, var(--logo-base) 22%, var(--color-surface, #1e293b));
  border-color: color-mix(in srgb, var(--logo-base) 35%, transparent);
}

.agent-logo.tone-primary:not(.has-icon),
.agent-logo.tone-info:not(.has-icon) {
  background: var(--color-info-soft, rgba(59, 130, 246, 0.12));
  color: var(--color-info, #2563eb);
}

.agent-logo.tone-secondary:not(.has-icon) {
  background: var(--color-warning-soft, rgba(245, 158, 11, 0.12));
  color: var(--color-warning, #d97706);
}

.agent-logo.tone-neutral:not(.has-icon) {
  background: var(--color-surface-muted, #f1f5f9);
  color: var(--color-text-muted, #64748b);
}
</style>
