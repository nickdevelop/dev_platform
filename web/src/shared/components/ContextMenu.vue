<script setup lang="ts">
// The platform's right-click menu: one instance per host, opened at the cursor.
//
// It is teleported to `body` and positioned with `position: fixed`, because every
// host that needs one (the workspace file tree, the tab strips) lives inside a
// scrolling or clipped container, and a menu absolutely positioned in there would
// be cut off by its own scroller.
//
// Dismissal is deliberately not a full-screen backdrop: an invisible overlay would
// swallow the next right click, so moving the menu from one row to the next would
// take two clicks. A document-level `mousedown` listener closes the menu first
// (mousedown precedes contextmenu), which lets the row underneath open it afresh.
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import type { ContextMenuAnchor, ContextMenuItem } from './contextMenu'

const props = defineProps<{
  anchor: ContextMenuAnchor | null
  items: ContextMenuItem[]
}>()

const emit = defineEmits<{
  close: []
}>()

const menuEl = ref<HTMLElement | null>(null)
const placed = ref({ x: 0, y: 0 })

// Distance the menu keeps from the viewport edges when it has to be pushed back in.
const EDGE = 8

function close() {
  if (props.anchor) emit('close')
}

// The menu is measured after it is on screen, so an item list that is wider than
// the gap between the cursor and the right edge still lands fully inside the view.
async function place() {
  const anchor = props.anchor
  if (!anchor) return
  placed.value = { x: anchor.x, y: anchor.y }
  await nextTick()
  const el = menuEl.value
  if (!el || !props.anchor) return
  const box = el.getBoundingClientRect()
  placed.value = {
    x: Math.max(EDGE, Math.min(anchor.x, window.innerWidth - box.width - EDGE)),
    y: Math.max(EDGE, Math.min(anchor.y, window.innerHeight - box.height - EDGE)),
  }
}

function onPointerDown(event: MouseEvent) {
  const el = menuEl.value
  if (!el) return
  if (event.target instanceof Node && el.contains(event.target)) return
  close()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') close()
}

let bound = false

function bindDismissal() {
  if (bound) return
  bound = true
  // Capture, so the menu is gone before the click or the next contextmenu lands.
  document.addEventListener('mousedown', onPointerDown, true)
  window.addEventListener('keydown', onKeydown, true)
  window.addEventListener('resize', close)
  // A menu pinned to a coordinate must not keep pointing at a row that moved.
  window.addEventListener('scroll', close, true)
}

function unbindDismissal() {
  if (!bound) return
  bound = false
  document.removeEventListener('mousedown', onPointerDown, true)
  window.removeEventListener('keydown', onKeydown, true)
  window.removeEventListener('resize', close)
  window.removeEventListener('scroll', close, true)
}

watch(
  () => props.anchor,
  (anchor) => {
    if (anchor) {
      bindDismissal()
      void place()
    } else {
      unbindDismissal()
    }
  },
  { immediate: true },
)

onBeforeUnmount(unbindDismissal)

function onItem(item: ContextMenuItem) {
  if (item.disabled) return
  // The action runs first: some of them open a dialog or a file picker that own
  // the focus the moment this menu leaves the DOM.
  item.onSelect()
  close()
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="props.anchor"
      ref="menuEl"
      class="ctx-menu"
      role="menu"
      data-test="context-menu"
      :style="{ left: `${placed.x}px`, top: `${placed.y}px` }"
      @contextmenu.prevent
    >
      <button
        v-for="item in props.items"
        :key="item.label"
        type="button"
        role="menuitem"
        class="ctx-menu-item"
        :class="{ 'is-danger': item.danger, 'has-divider': item.divider }"
        :disabled="item.disabled"
        data-test="context-menu-item"
        @click="onItem(item)"
      >
        <span class="ctx-menu-icon" aria-hidden="true">
          <svg v-if="item.icon === 'open'" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6">
            <path d="M3 2h7l3 3v9H3V2z" />
            <polyline points="10 2 10 5 13 5" />
          </svg>
          <svg v-else-if="item.icon === 'folder'" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6">
            <path d="M2 3h4l2 2h6v8H2V3z" />
          </svg>
          <svg v-else-if="item.icon === 'upload'" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6">
            <path d="M8 11V2M4 5l4-3 4 3M2 13h12" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <svg v-else-if="item.icon === 'download'" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6">
            <path d="M8 2v9M4 8l4 3 4-3M2 13h12" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <svg v-else-if="item.icon === 'copy'" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6">
            <rect x="2.5" y="2.5" width="7" height="7" rx="1" />
            <path d="M13.5 6v7a1 1 0 0 1-1 1h-7" stroke-linecap="round" />
          </svg>
          <svg v-else-if="item.icon === 'trash'" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6">
            <path d="M3 4h10M6 4V2.5h4V4M5 4v9h6V4" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <span v-else class="ctx-menu-icon-spacer"></span>
        </span>
        <span class="ctx-menu-label">{{ item.label }}</span>
      </button>
    </div>
  </Teleport>
</template>

<style scoped>
/* Above the workbench's own overlays (the floating sidebar sits at 40), below
   Element Plus's popper layer, so a confirmation dialog always outranks the menu
   that opened it. */
.ctx-menu {
  position: fixed;
  z-index: 80;
  min-width: 10.5rem;
  display: flex;
  flex-direction: column;
  padding: 6px;
  border: 1px solid var(--color-border-strong);
  border-radius: 8px;
  background: var(--color-surface);
  box-shadow: 0 10px 25px -3px rgba(0, 0, 0, 0.15), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
}

.ctx-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  border: none;
  background: transparent;
  text-align: left;
  padding: 6px 10px;
  border-radius: 5px;
  cursor: pointer;
  font: inherit;
  font-size: var(--text-size-12);
  color: var(--color-text);
  white-space: nowrap;
  transition: background-color 0.12s ease;
}

.ctx-menu-item:hover:not(:disabled) {
  background: var(--color-surface-muted);
}

.ctx-menu-item:disabled {
  color: var(--color-text-muted);
  cursor: default;
  opacity: 0.5;
}

.ctx-menu-item.is-danger {
  color: var(--color-danger);
}

.ctx-menu-item.has-divider {
  position: relative;
  margin-top: 6px;
}

.ctx-menu-item.has-divider::before {
  content: '';
  position: absolute;
  left: -6px;
  right: -6px;
  top: -4px;
  height: 1px;
  background: var(--color-border);
}

.ctx-menu-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 12px;
  flex-shrink: 0;
  color: var(--color-text-muted);
}

.ctx-menu-item.is-danger .ctx-menu-icon {
  color: var(--color-danger);
}

.ctx-menu-icon-spacer {
  width: 12px;
  height: 12px;
}

.ctx-menu-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
