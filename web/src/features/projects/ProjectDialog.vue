<script setup lang="ts">
// Accessible modal dialog: role="dialog" with an accessible name, focus
// moved into the dialog on open and restored to the trigger on close,
// Escape to cancel, and a Tab trap that keeps keyboard focus inside.
import { onBeforeUnmount, onMounted, ref } from 'vue'

const emit = defineEmits<{ close: [] }>()

defineProps<{
  title: string
}>()

const panel = ref<HTMLElement | null>(null)
let previouslyFocused: HTMLElement | null = null

function focusables(): HTMLElement[] {
  const selector =
    'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
  return Array.from(panel.value?.querySelectorAll<HTMLElement>(selector) ?? [])
}

onMounted(() => {
  previouslyFocused = document.activeElement as HTMLElement | null
  const first = focusables()[0]
  ;(first ?? panel.value)?.focus()
})

onBeforeUnmount(() => {
  previouslyFocused?.focus()
  previouslyFocused = null
})

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.stopPropagation()
    emit('close')
    return
  }
  if (event.key !== 'Tab') return

  const items = focusables()
  if (items.length === 0) return
  const first = items[0]
  const last = items[items.length - 1]
  const active = document.activeElement
  if (event.shiftKey && (active === first || active === panel.value)) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && active === last) {
    event.preventDefault()
    first.focus()
  }
}
</script>

<template>
  <div class="dialog-backdrop" @keydown="onKeydown">
    <div
      ref="panel"
      class="dialog"
      role="dialog"
      aria-modal="true"
      :aria-label="title"
      tabindex="-1"
    >
      <h2 class="dialog-title">{{ title }}</h2>
      <slot />
    </div>
  </div>
</template>

<style scoped>
.dialog-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 10;
}

.dialog {
  background: var(--color-surface);
  color: var(--color-text);
  border-radius: 8px;
  padding: 1.25rem 1.5rem;
  width: min(30rem, calc(100vw - 2rem));
  max-height: calc(100vh - 4rem);
  overflow: auto;
  box-shadow: 0 10px 40px rgba(15, 23, 42, 0.35);
}

.dialog-title {
  margin: 0 0 0.75rem;
  font-size: 1.1rem;
}
</style>
