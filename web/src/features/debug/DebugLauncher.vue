<script setup lang="ts">
// Floating entry button for the SSE bus debug panel. Shown only while the
// settings toggle is on; clicking toggles the debug drawer, whose open state
// lives in the debug store (capture runs exactly while the drawer is open).
import { ref } from 'vue'
import { useDebugPanelStore } from '../../stores/debugPanel'
import {
  useDraggablePosition,
  type DefaultCorner,
} from '../../shared/composables/useDraggablePosition'

const debug = useDebugPanelStore()

const buttonRef = ref<HTMLElement | null>(null)
const {
  dragging,
  buttonStyle,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onPointerCancel,
  consumeClickSuppression,
} = useDraggablePosition(buttonRef, {
  storageKey: 'dev-platform-debug-button-position',
  defaultCorner: 'bottom-right' as DefaultCorner,
})

function toggle() {
  if (consumeClickSuppression()) return
  debug.toggleDrawer()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    toggle()
  }
}
</script>

<template>
  <div
    ref="buttonRef"
    class="debug-launcher"
    :class="{ dragging, active: debug.open }"
    :style="buttonStyle"
    data-test="debug-launcher"
  >
    <button
      type="button"
      class="debug-launch-trigger"
      aria-label="SSE 总线调试面板"
      :title="'SSE 总线调试面板（点击打开/关闭）'"
      :aria-expanded="debug.open ? 'true' : 'false'"
      data-test="debug-launch-trigger"
      @click="toggle"
      @keydown="onKeydown"
    >
      <svg
        class="debug-launch-icon"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
        focusable="false"
      >
        <!-- 1.5px-feel pulse/monitor glyph: signal + data feed -->
        <rect x="2.5" y="4.5" width="19" height="15" rx="2" />
        <path d="M2.5 9.5h19" />
        <path d="M6 14.5h3M10.5 14.5h2M15 14.5h3" />
        <path d="M6 12.5h5M13 12.5h5" />
      </svg>
      <span v-if="debug.open" class="debug-launch-dot" aria-hidden="true"></span>
    </button>
  </div>
</template>

<style scoped>
.debug-launcher {
  position: fixed;
  z-index: 63;
  user-select: none;
  touch-action: none;
}

.debug-launcher.dragging {
  cursor: grabbing;
}

.debug-launch-trigger {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 1px solid var(--color-border, #222631);
  border-radius: var(--radius, 4px);
  background: var(--color-surface, #14171f);
  color: var(--color-text-muted, #8b929e);
  cursor: pointer;
  opacity: 0.55;
  transition: opacity 0.15s ease, color 0.15s ease, border-color 0.15s ease;
}

.debug-launch-trigger:hover,
.debug-launcher.active .debug-launch-trigger {
  opacity: 1;
  color: var(--color-accent, #4a8ad6);
  border-color: var(--color-accent, #4a8ad6);
}

.debug-launch-icon {
  width: 15px;
  height: 15px;
}

.debug-launch-dot {
  position: absolute;
  top: -2px;
  right: -2px;
  width: 5px;
  height: 5px;
  border-radius: 9999px;
  background: var(--color-accent, #4a8ad6);
}
</style>
