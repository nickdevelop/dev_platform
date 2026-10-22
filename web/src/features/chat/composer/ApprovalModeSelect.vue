<script setup lang="ts">
// Approval-mode picker for the composer. It uses the same compact trigger and
// popover shell as the model picker (composer-select.css) so the two controls
// in the composer read as one component family. The selected value still lives
// in SessionChatPanel: this picker only emits update:modelValue.
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import type { ApprovalMode } from '../interactions/autoApproval'

interface ApprovalOption {
  value: ApprovalMode
  label: string
  description: string
}

const options: ApprovalOption[] = [
  { value: 'default', label: '默认', description: '工具调用前逐次确认' },
  { value: 'auto-approve', label: '自动审批', description: '自动批准工具确认' },
  { value: 'autopilot', label: '托管', description: '自动审批并按推荐答案提交表单' },
]

const props = defineProps<{ modelValue: ApprovalMode }>()

const emit = defineEmits<{
  'update:modelValue': [mode: ApprovalMode]
}>()

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)

const currentLabel = computed(
  () => options.find((option) => option.value === props.modelValue)?.label ?? '默认',
)

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
    const selected = options.findIndex((option) => option.value === props.modelValue)
    focusOption(focusLast ? options.length - 1 : Math.max(0, selected))
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

function choose(mode: ApprovalMode) {
  emit('update:modelValue', mode)
  close(true)
}

// A click anywhere outside the picker dismisses the popover, so it does not
// linger over the conversation while the user edits the message.
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

onMounted(() => document.addEventListener('mousedown', onDocumentMouseDown))
onBeforeUnmount(() => document.removeEventListener('mousedown', onDocumentMouseDown))
</script>

<template>
  <div
    ref="root"
    class="composer-select approval-mode-select"
    @keydown="onKeydown"
    @focusout="onFocusOut"
  >
    <button
      ref="trigger"
      type="button"
      class="composer-select-trigger"
      :class="{ open }"
      aria-haspopup="listbox"
      :aria-expanded="open"
      aria-label="自动化执行与审批模式"
      title="自动化执行与审批模式"
      data-test="approval-mode-select"
      @click="toggle"
    >
      <span class="approval-mode-trigger-label">{{ currentLabel }}</span>
      <span class="composer-select-caret" aria-hidden="true">▾</span>
    </button>

    <div
      v-if="open"
      class="composer-select-menu approval-mode-menu"
      role="listbox"
      aria-label="自动化执行与审批模式"
      data-test="approval-mode-menu"
    >
      <button
        v-for="option in options"
        :key="option.value"
        type="button"
        class="composer-select-option approval-mode-option"
        :class="{ selected: option.value === modelValue }"
        role="option"
        :aria-selected="option.value === modelValue"
        :data-mode="option.value"
        :data-test="`approval-mode-option-${option.value}`"
        @click="choose(option.value)"
      >
        <span class="approval-mode-option-name">{{ option.label }}</span>
        <span class="approval-mode-option-desc">{{ option.description }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped src="./composer-select.css"></style>

<style scoped>
.approval-mode-select {
  flex-shrink: 0;
}

.approval-mode-trigger-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.approval-mode-menu {
  min-width: 14rem;
  /* The picker sits near the right edge of the composer, so anchor the menu to
     its right edge and let it grow leftwards instead of off-screen. */
  right: 0;
  left: auto;
}

.approval-mode-option-name {
  font-weight: 500;
}

.approval-mode-option-desc {
  color: var(--color-text-muted);
  font-size: 0.72rem;
}
</style>
