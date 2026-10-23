<script setup lang="ts">
// Floating entry point for the quick assistant. The button is draggable and its
// position is remembered; the popover opens over the current page without a
// route change and sizes itself to 480x600 whenever the viewport allows.
import { computed, nextTick, ref, watch } from 'vue'
import { placePanel, useDraggablePosition } from './useDraggablePosition'
import QuickAssistantPanel from './QuickAssistantPanel.vue'

const open = ref(false)
const buttonRef = ref<HTMLButtonElement | null>(null)
const {
  position,
  buttonSize,
  viewport,
  dragging,
  buttonStyle,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onPointerCancel,
  consumeClickSuppression,
} = useDraggablePosition(buttonRef)

const panelStyle = computed(() => {
  if (!position.value) return undefined
  const placed = placePanel(
    {
      left: position.value.left,
      top: position.value.top,
      width: buttonSize.value.width,
      height: buttonSize.value.height,
    },
    viewport.value,
  )
  return {
    left: `${placed.left}px`,
    top: `${placed.top}px`,
    width: `${placed.width}px`,
    height: `${placed.height}px`,
  }
})

function toggle() {
  if (consumeClickSuppression()) return
  open.value = !open.value
}

function close() {
  open.value = false
}

watch(open, (value, previous) => {
  if (value || !previous) return
  void nextTick(() => buttonRef.value?.focus())
})
</script>

<template>
  <div class="quick-assistant-launcher">
    <button
      ref="buttonRef"
      type="button"
      class="quick-assistant-button"
      :class="{ dragging }"
      :style="buttonStyle"
      aria-label="快捷助手"
      title="快捷助手"
      aria-controls="quick-assistant-popover"
      :aria-expanded="open.toString()"
      data-test="quick-assistant-button"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerCancel"
      @click="toggle"
    >
      <svg class="quick-assistant-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <path
          d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
      <span>快捷助手</span>
    </button>

    <div
      v-if="open && panelStyle"
      id="quick-assistant-popover"
      class="quick-assistant-popover"
      :style="panelStyle"
      data-test="quick-assistant-popover"
    >
      <QuickAssistantPanel @close="close" />
    </div>
  </div>
</template>

<style scoped>
.quick-assistant-button {
  position: fixed;
  top: 50px;
  right: 50px;
  z-index: 61;
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.5rem 0.8rem;
  border: 1px solid var(--color-primary);
  border-radius: 999px;
  background: var(--color-primary);
  color: #fff;
  font: inherit;
  font-size: 0.85rem;
  font-weight: 500;
  cursor: grab;
  user-select: none;
  touch-action: none;
  box-shadow: 0 6px 20px rgba(15, 23, 42, 0.22);
  transition: background-color 0.15s ease, box-shadow 0.15s ease;
}

.quick-assistant-button:hover {
  background: var(--color-primary-hover, var(--color-primary));
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.28);
}

.quick-assistant-button.dragging {
  cursor: grabbing;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.32);
}

.quick-assistant-icon {
  width: 16px;
  height: 16px;
}

.quick-assistant-popover {
  position: fixed;
  z-index: 60;
  box-sizing: border-box;
  border: 1px solid var(--color-border-strong);
  border-radius: 12px;
  background: var(--color-surface);
  box-shadow: 0 18px 48px rgba(15, 23, 42, 0.3);
  overflow: hidden;
  animation: quick-assistant-pop 0.15s ease-out;
}

@keyframes quick-assistant-pop {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (max-width: 560px) {
  .quick-assistant-button {
    right: 16px;
  }
}
</style>
