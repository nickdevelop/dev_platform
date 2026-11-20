<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { useProjectTerminalStore, type ProjectTerminalInstance } from '../../stores/projectTerminal'
import XtermView from './XtermView.vue'

const store = useProjectTerminalStore()

// Dragging state for individual floating windows
const draggingId = ref<string | null>(null)
let dragStartX = 0
let dragStartY = 0
let initialLeft = 0
let initialTop = 0

function onPointerDownHeader(e: PointerEvent, instance: ProjectTerminalInstance) {
  // Bring to front on any interaction
  store.bringToFront(instance.id)

  // Only start drag on primary button and ignore control buttons
  if (e.button !== 0) return
  const target = e.target as HTMLElement | null
  if (target?.closest('button')) return

  draggingId.value = instance.id
  dragStartX = e.clientX
  dragStartY = e.clientY
  initialLeft = instance.left
  initialTop = instance.top

  window.addEventListener('pointermove', onPointerMove)
  window.addEventListener('pointerup', onPointerUp)
  window.addEventListener('pointercancel', onPointerUp)
}

function onPointerMove(e: PointerEvent) {
  if (!draggingId.value) return
  const inst = store.instances.find((i) => i.id === draggingId.value)
  if (!inst) return

  const dx = e.clientX - dragStartX
  const dy = e.clientY - dragStartY

  const maxLeft = Math.max(0, window.innerWidth - 120)
  const maxTop = Math.max(0, window.innerHeight - 50)

  const newLeft = Math.min(Math.max(0, initialLeft + dx), maxLeft)
  const newTop = Math.min(Math.max(0, initialTop + dy), maxTop)

  store.updatePosition(inst.id, newLeft, newTop)
}

function onPointerUp() {
  if (!draggingId.value) return
  draggingId.value = null
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', onPointerUp)
  window.removeEventListener('pointercancel', onPointerUp)
}

onBeforeUnmount(() => {
  onPointerUp()
})

function onWindowPointerDown(instance: ProjectTerminalInstance) {
  store.bringToFront(instance.id)
}

function handleMinimize(id: string) {
  store.minimize(id)
}

function handleClose(id: string) {
  store.closeTerminal(id)
}
</script>

<template>
  <Teleport to="body">
    <!-- Render each terminal window independently -->
    <template v-for="instance in store.instances" :key="instance.id">
      <div
        v-show="!instance.isMinimized"
        class="project-terminal-window fixed flex flex-col bg-[#121316] text-platform-text rounded border border-white/15 shadow-2xl overflow-hidden select-none"
        :class="{ 'is-dragging': draggingId === instance.id }"
        :style="{
          left: `${instance.left}px`,
          top: `${instance.top}px`,
          zIndex: instance.zIndex,
          width: '800px',
          height: '500px',
          maxWidth: '95vw',
          maxHeight: '90vh',
        }"
        data-test="project-terminal-modal"
        :data-terminal-id="instance.id"
        @pointerdown="onWindowPointerDown(instance)"
      >
        <!-- Draggable title bar -->
        <div
          class="terminal-title-bar flex items-center justify-between px-3 py-2 bg-[#1a1b20] border-b border-white/10 shrink-0 cursor-grab active:cursor-grabbing"
          data-test="terminal-title-bar"
          @pointerdown="onPointerDownHeader($event, instance)"
        >
          <div class="flex items-center gap-2 min-w-0 mr-2 pointer-events-none">
            <span class="terminal-icon inline-flex items-center justify-center text-platform-primary shrink-0">
              <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M3 4.5l3.5 3.5L3 11.5M8.5 11.5h4" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
            </span>
            <span class="font-medium text-xs text-white truncate">
              终端 - {{ instance.projectName }}
            </span>
            <span
              v-if="instance.cwd"
              class="path-badge text-3xs font-mono px-1.5 py-0.5 rounded bg-white/5 text-slate-400 border border-white/10 truncate max-w-[360px]"
              :title="instance.cwd"
            >
              {{ instance.cwd }}
            </span>
          </div>

          <!-- Window action buttons: only minimize and close -->
          <div class="flex items-center gap-1 shrink-0">
            <!-- Minimize button -->
            <button
              type="button"
              class="window-btn inline-flex items-center justify-center w-6 h-6 rounded text-slate-400 hover:text-white hover:bg-white/10 transition-colors cursor-pointer"
              title="最小化到快捷栏"
              aria-label="最小化终端"
              data-test="terminal-minimize-btn"
              @click.stop="handleMinimize(instance.id)"
            >
              <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
                <line x1="3" y1="8" x2="13" y2="8" stroke-linecap="round" />
              </svg>
            </button>

            <!-- Close button -->
            <button
              type="button"
              class="window-btn close-btn inline-flex items-center justify-center w-6 h-6 rounded text-slate-400 hover:text-white hover:bg-red-500/80 transition-colors cursor-pointer"
              title="关闭并退出终端"
              aria-label="关闭终端"
              data-test="terminal-close-btn"
              @click.stop="handleClose(instance.id)"
            >
              <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="4" y1="4" x2="12" y2="12" stroke-linecap="round" />
                <line x1="12" y1="4" x2="4" y2="12" stroke-linecap="round" />
              </svg>
            </button>
          </div>
        </div>

        <!-- Terminal body -->
        <div class="terminal-window-body flex-1 min-h-0 bg-[#121316] relative overflow-hidden select-text">
          <XtermView
            :tab-id="instance.tabId"
            :server-id="0"
            :cwd="instance.cwd"
            :active="!instance.isMinimized"
            :visible="!instance.isMinimized"
            style="width: 100%; height: 100%;"
          />
        </div>
      </div>
    </template>
  </Teleport>
</template>

<style scoped>
.project-terminal-window {
  box-shadow: 0 18px 48px rgba(0, 0, 0, 0.55), 0 2px 8px rgba(0, 0, 0, 0.35);
  transition: box-shadow 0.15s ease;
}

.project-terminal-window.is-dragging {
  opacity: 0.96;
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.65);
}
</style>
