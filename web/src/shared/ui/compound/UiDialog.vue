<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    title?: string
    widthClass?: string
    showClose?: boolean
  }>(),
  {
    title: '',
    widthClass: 'w-full max-w-lg',
    showClose: true,
  },
)

const emit = defineEmits<{
  close: []
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
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/75"
    @keydown="onKeydown"
  >
    <div
      ref="panel"
      class="relative bg-platform-surface text-platform-text rounded-[var(--radius,4px)] border border-platform-border-strong max-h-[calc(100vh-3rem)] flex flex-col outline-none overflow-hidden"
      :class="widthClass"
      role="dialog"
      aria-modal="true"
      :aria-label="title"
      tabindex="-1"
    >
      <div
        v-if="title || showClose"
        class="flex items-center justify-between px-5 py-3.5 border-b border-platform-border shrink-0"
      >
        <h3 class="font-medium text-base text-platform-text truncate">
          <slot name="title">{{ title }}</slot>
        </h3>
        <button
          v-if="showClose"
          type="button"
          class="text-platform-text-muted hover:text-platform-text p-1 rounded transition-colors inline-flex items-center justify-center"
          title="关闭"
          aria-label="关闭对话框"
          @click="emit('close')"
        >
          <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
            <line x1="3" y1="3" x2="13" y2="13" />
            <line x1="13" y1="3" x2="3" y2="13" />
          </svg>
        </button>
      </div>

      <div class="p-5 overflow-y-auto flex-1 leading-relaxed">
        <slot />
      </div>

      <div
        v-if="$slots.footer"
        class="flex items-center justify-end gap-2.5 px-5 py-3 bg-platform-surface-muted border-t border-platform-border shrink-0"
      >
        <slot name="footer" />
      </div>
    </div>
  </div>
</template>
