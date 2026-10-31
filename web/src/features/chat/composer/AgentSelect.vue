<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import type { BackendId } from '../../../api/sessions'
import type { AgentBackendConfig } from '../../../stores/agents'

const props = defineProps<{
  modelValue: BackendId | ''
  backends: AgentBackendConfig[]
}>()

const emit = defineEmits<{
  'update:modelValue': [backend: BackendId]
}>()

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)

const activeBackend = computed(() => {
  return props.backends.find((b) => b.backend === props.modelValue) ?? null
})

const currentLabel = computed(() => {
  return activeBackend.value?.label || activeBackend.value?.backend || '选择 Agent'
})

function optionButtons(): HTMLButtonElement[] {
  return Array.from(root.value?.querySelectorAll<HTMLButtonElement>('.composer-select-option') ?? [])
}

function focusOption(index: number) {
  const buttons = optionButtons()
  if (buttons.length === 0) return
  const bounded = Math.max(0, Math.min(buttons.length - 1, index))
  buttons[bounded]?.focus()
}

function openMenu(focusLast = false) {
  open.value = true
  void nextTick(() => {
    const selected = props.backends.findIndex((b) => b.backend === props.modelValue)
    focusOption(focusLast ? props.backends.length - 1 : Math.max(0, selected))
  })
}

function close(refocus = false) {
  open.value = false
  if (refocus) void nextTick(() => trigger.value?.focus())
}

function toggle() {
  if (open.value) close()
  else openMenu()
}

function choose(backend: BackendId) {
  emit('update:modelValue', backend)
  close(true)
}

function onDocumentMouseDown(event: MouseEvent) {
  if (!open.value) return
  const target = event.target as Node | null
  if (root.value && target && !root.value.contains(target)) close()
}

function onFocusOut(event: FocusEvent) {
  if (!open.value) return
  const next = event.relatedTarget as Node | null
  if (next && root.value?.contains(next)) return
  close()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && open.value) {
    event.stopPropagation()
    close(true)
    return
  }
  if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
  event.preventDefault()
  if (!open.value) {
    openMenu(event.key === 'ArrowUp')
    return
  }
  const buttons = optionButtons()
  const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
  const delta = event.key === 'ArrowDown' ? 1 : -1
  const next = index === -1 ? (delta > 0 ? 0 : buttons.length - 1) : index + delta
  focusOption(next)
}

onMounted(() => {
  document.addEventListener('mousedown', onDocumentMouseDown)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocumentMouseDown)
})
</script>

<template>
  <div
    ref="root"
    class="composer-select agent-select"
    data-test="agent-select"
    @focusout="onFocusOut"
    @keydown="onKeydown"
  >
    <button
      ref="trigger"
      type="button"
      class="composer-select-trigger"
      :class="{ open }"
      :aria-expanded="open"
      aria-haspopup="listbox"
      title="选择 Agent 后端"
      data-test="agent-select-trigger"
      @click="toggle"
    >
      <svg class="agent-icon shrink-0" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
        <rect x="2.5" y="3.5" width="11" height="9" rx="1.5" />
        <line x1="8" y1="1.5" x2="8" y2="3.5" />
        <circle cx="6" cy="7.5" r="0.75" fill="currentColor" />
        <circle cx="10" cy="7.5" r="0.75" fill="currentColor" />
        <line x1="6" y1="9.5" x2="10" y2="9.5" />
      </svg>
      <span class="composer-select-label truncate">{{ currentLabel }}</span>
      <span class="composer-select-caret" aria-hidden="true">▾</span>
    </button>

    <div
      v-if="open"
      class="composer-select-menu"
      role="listbox"
      aria-label="Agent 后端列表"
      data-test="agent-select-menu"
    >
      <button
        v-for="b in backends"
        :key="b.backend"
        type="button"
        role="option"
        class="composer-select-option"
        :class="{ active: b.backend === modelValue }"
        :aria-selected="b.backend === modelValue"
        :data-backend="b.backend"
        @click="choose(b.backend)"
      >
        <span class="option-header flex items-center justify-between w-full">
          <span class="composer-select-option-label font-medium">{{ b.label || b.backend }}</span>
          <span v-if="b.backend === modelValue" class="text-blue-500 text-xs">✓</span>
        </span>
        <span class="composer-select-option-desc text-[11px] text-slate-400">
          {{ b.backend === 'opencode' ? '全功能 AI coding agent，支持多工具及模型' : b.backend === 'pi' ? '极简终端 harness' : '已配置的 Agent 后端' }}
        </span>
      </button>
    </div>
  </div>
</template>

<style src="./composer-select.css" scoped></style>
<style scoped>
.agent-icon {
  color: var(--color-text-muted);
}
.agent-select .composer-select-trigger {
  min-width: 6.5rem;
}
</style>
