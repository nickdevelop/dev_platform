<script setup lang="ts">
// Segmented switch for choosing the agent backend of a new session. It is a
// provider-neutral presentational control: every option comes from the backend
// descriptor (label, order), so no backend id is special-cased here. The caller
// passes only the platform-enabled backends; disabled ones are not rendered.
import { computed, ref } from 'vue'
import type { BackendId } from '../../../api/sessions'
import type { AgentBackend } from '../../../api/agents'
import BackendIcon from '../../../shared/components/BackendIcon.vue'

const props = defineProps<{
  modelValue: BackendId | ''
  backends: AgentBackend[]
  loaded?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [backend: BackendId]
}>()

const root = ref<HTMLElement | null>(null)

// The active option is the roving tab stop; when the stored value is not in the
// list the first option takes the tab stop.
const tabStopIndex = computed(() => {
  const selected = props.backends.findIndex((entry) => entry.backend === props.modelValue)
  return selected >= 0 ? selected : 0
})

function optionButtons(): HTMLButtonElement[] {
  return Array.from(
    root.value?.querySelectorAll<HTMLButtonElement>('[role="radio"]') ?? [],
  )
}

function choose(backend: AgentBackend) {
  if (backend.backend === props.modelValue) return
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
  const count = props.backends.length
  if (count === 0) return

  event.preventDefault()
  let target: number
  if (event.key === 'Home') {
    target = 0
  } else if (event.key === 'End') {
    target = count - 1
  } else {
    const delta = event.key === 'ArrowRight' ? 1 : -1
    target = (tabStopIndex.value + delta + count) % count
  }

  const backend = props.backends[target]
  if (!backend) return
  choose(backend)
  focusIndex(target)
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
      :class="{ active: backend.backend === modelValue }"
      :aria-checked="backend.backend === modelValue"
      :tabindex="index === tabStopIndex ? 0 : -1"
      :title="`切换到 ${backend.label || backend.backend}`"
      :data-test="`agent-switch-${backend.backend}`"
      :data-backend="backend.backend"
      @click="choose(backend)"
    >
      <BackendIcon :backend="backend.backend" size="sm" />
      {{ backend.label || backend.backend }}
    </button>
    <span v-if="backends.length === 0" class="agent-switch-empty">{{ loaded ? '暂无可用 Agent' : '加载中…' }}</span>
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
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
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

.agent-switch-option:hover:not(.active) {
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

.agent-switch-empty {
  padding: 0.35rem 0.75rem;
  color: var(--color-text-muted);
  font-size: 0.8rem;
  white-space: nowrap;
}
</style>
