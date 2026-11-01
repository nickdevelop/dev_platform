<script setup lang="ts">
// Segmented switch for choosing the agent backend of a new session. It is a
// provider-neutral presentational control: every option comes from the backend
// descriptor (label, enabled, order), so no backend id is special-cased here.
// Platform-disabled backends stay visible but unpressable, because hiding them
// would leave the user wondering where a familiar agent went.
import { computed, ref } from 'vue'
import type { BackendId } from '../../../api/sessions'
import type { AgentBackend } from '../../../api/agents'

const props = defineProps<{
  modelValue: BackendId | ''
  backends: AgentBackend[]
}>()

const emit = defineEmits<{
  'update:modelValue': [backend: BackendId]
}>()

const root = ref<HTMLElement | null>(null)

function isEnabled(backend: AgentBackend): boolean {
  return backend.enabled !== false
}

// The active option is the roving tab stop; when the stored value points at a
// disabled or unknown backend the first selectable option takes the tab stop.
const tabStopIndex = computed(() => {
  const selected = props.backends.findIndex((entry) => entry.backend === props.modelValue)
  if (selected >= 0 && isEnabled(props.backends[selected])) return selected
  return props.backends.findIndex((entry) => isEnabled(entry))
})

function optionButtons(): HTMLButtonElement[] {
  return Array.from(
    root.value?.querySelectorAll<HTMLButtonElement>('[role="radio"]') ?? [],
  )
}

function choose(backend: AgentBackend) {
  if (!isEnabled(backend) || backend.backend === props.modelValue) return
  emit('update:modelValue', backend.backend)
}

function focusIndex(index: number) {
  const buttons = optionButtons()
  if (buttons.length === 0) return
  const bounded = Math.max(0, Math.min(buttons.length - 1, index))
  buttons[bounded]?.focus()
}

function onKeydown(event: KeyboardEvent) {
  const keys = ['ArrowLeft', 'ArrowRight', 'Home', 'End']
  if (!keys.includes(event.key)) return
  const current = tabStopIndex.value
  if (current < 0) return
  const enabledIndexes = props.backends
    .map((entry, index) => ({ entry, index }))
    .filter(({ entry }) => isEnabled(entry))
    .map(({ index }) => index)
  if (enabledIndexes.length === 0) return

  event.preventDefault()
  const position = enabledIndexes.indexOf(current)
  let target: number
  if (event.key === 'Home') {
    target = enabledIndexes[0]
  } else if (event.key === 'End') {
    target = enabledIndexes[enabledIndexes.length - 1]
  } else {
    const delta = event.key === 'ArrowRight' ? 1 : -1
    const base = position === -1 ? 0 : position
    const next = (base + delta + enabledIndexes.length) % enabledIndexes.length
    target = enabledIndexes[next]
  }

  const backend = props.backends[target]
  if (!backend) return
  choose(backend)
  focusIndex(target)
}

function optionTitle(backend: AgentBackend): string {
  if (!isEnabled(backend)) return `${backend.label || backend.backend} 已在平台设置中停用`
  return `切换到 ${backend.label || backend.backend}`
}
</script>

<template>
  <div
    ref="root"
    class="agent-switch"
    role="radiogroup"
    aria-label="选择 Agent 后端"
    data-test="agent-switch"
    @keydown="onKeydown"
  >
    <button
      v-for="(backend, index) in backends"
      :key="backend.backend"
      type="button"
      role="radio"
      class="agent-switch-option"
      :class="{ active: backend.backend === modelValue, blocked: !isEnabled(backend) }"
      :aria-checked="backend.backend === modelValue"
      :aria-disabled="!isEnabled(backend)"
      :disabled="!isEnabled(backend)"
      :tabindex="index === tabStopIndex ? 0 : -1"
      :title="optionTitle(backend)"
      :data-test="`agent-switch-${backend.backend}`"
      :data-backend="backend.backend"
      @click="choose(backend)"
    >
      {{ backend.label || backend.backend }}
    </button>
    <span v-if="backends.length === 0" class="agent-switch-empty">加载中…</span>
  </div>
</template>

<style scoped>
.agent-switch {
  display: flex;
  align-items: stretch;
  gap: 2px;
  flex: 1;
  min-width: 0;
  padding: 2px;
  overflow-x: auto;
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border-strong);
  border-radius: 4px;
}

.agent-switch-option {
  position: relative;
  flex: 1 1 auto;
  min-width: 5.5rem;
  padding: 0.35rem 0.75rem;
  border: none;
  border-radius: 2px;
  background: transparent;
  color: var(--color-text-muted);
  font: inherit;
  font-size: 0.82rem;
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
  transition: color 0.12s ease-out, background-color 0.12s ease-out;
}

.agent-switch-option:hover:not(:disabled):not(.active) {
  color: var(--color-text);
  background: var(--color-surface);
}

.agent-switch-option.active {
  color: var(--color-text);
  background: var(--color-surface);
  font-weight: 600;
}

/* The Move: the active segment is marked by a 2px accent line on its bottom edge. */
.agent-switch-option.active::after {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  bottom: 0;
  height: 2px;
  background: var(--color-accent, #4a8ad6);
}

.agent-switch-option:focus-visible {
  outline: 1px solid var(--color-accent, #4a8ad6);
  outline-offset: -1px;
}

.agent-switch-option:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.agent-switch-empty {
  padding: 0.35rem 0.75rem;
  color: var(--color-text-muted);
  font-size: 0.8rem;
  white-space: nowrap;
}
</style>
