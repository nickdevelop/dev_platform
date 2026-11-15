<script setup lang="ts">
// The enlarged view of one PlantUML drawing: the SVG on a full-viewport canvas
// that can be panned and zoomed. The chat diagram block and the file preview
// share it, so both zoom the same way. It owns its transform and resets it on
// every open, so one diagram's zoom never leaks into the next. The parent owns
// whether it is open and reacts to `close`.
import { computed, nextTick, onBeforeUnmount, ref, useTemplateRef, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    svg: string
    dark: boolean
    open: boolean
    title?: string
  }>(),
  { title: '图表 · plantuml' },
)

const emit = defineEmits<{
  close: []
}>()

const MIN_ZOOM = 0.2
const MAX_ZOOM = 4
const zoom = ref(1)
const panX = ref(0)
const panY = ref(0)
const dragging = ref(false)
const closeButton = useTemplateRef<HTMLElement>('viewer-close')
let bodyOverflow = ''
let dragStartX = 0
let dragStartY = 0
let dragOriginX = 0
let dragOriginY = 0

const zoomPercent = computed(() => `${Math.round(zoom.value * 100)}%`)
const contentStyle = computed(() => ({
  transform: `translate(${panX.value}px, ${panY.value}px) scale(${zoom.value})`,
}))

function clampZoom(value: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Math.round(value * 100) / 100))
}

function resetView(): void {
  zoom.value = 1
  panX.value = 0
  panY.value = 0
}

function lockBody(): void {
  bodyOverflow = document.body.style.overflow
  document.body.style.overflow = 'hidden'
}

function unlockBody(): void {
  document.body.style.overflow = bodyOverflow
  bodyOverflow = ''
}

function zoomIn(): void {
  zoom.value = clampZoom(zoom.value * 1.25)
}

function zoomOut(): void {
  zoom.value = clampZoom(zoom.value / 1.25)
}

function zoomAround(clientX: number, clientY: number, rect: DOMRect, factor: number): void {
  const next = clampZoom(zoom.value * factor)
  if (next === zoom.value) return
  const ratio = next / zoom.value
  const cursorX = clientX - rect.left
  const cursorY = clientY - rect.top
  panX.value = cursorX - (cursorX - panX.value) * ratio
  panY.value = cursorY - (cursorY - panY.value) * ratio
  zoom.value = next
}

function onWheel(event: WheelEvent): void {
  // The viewer owns the wheel: scrolling here zooms instead of moving the page
  // behind the backdrop, which is already locked.
  event.preventDefault()
  const viewport = event.currentTarget as HTMLElement
  zoomAround(event.clientX, event.clientY, viewport.getBoundingClientRect(), event.deltaY < 0 ? 1.15 : 1 / 1.15)
}

function startDrag(event: MouseEvent): void {
  if (event.button !== 0) return
  dragging.value = true
  dragStartX = event.clientX
  dragStartY = event.clientY
  dragOriginX = panX.value
  dragOriginY = panY.value
}

function onDrag(event: MouseEvent): void {
  if (!dragging.value) return
  panX.value = dragOriginX + (event.clientX - dragStartX)
  panY.value = dragOriginY + (event.clientY - dragStartY)
}

function endDrag(): void {
  dragging.value = false
}

function startTouchDrag(event: TouchEvent): void {
  if (event.touches.length !== 1) return
  const touch = event.touches[0]
  dragging.value = true
  dragStartX = touch.clientX
  dragStartY = touch.clientY
  dragOriginX = panX.value
  dragOriginY = panY.value
}

function onTouchDrag(event: TouchEvent): void {
  if (!dragging.value || event.touches.length !== 1) return
  const touch = event.touches[0]
  panX.value = dragOriginX + (touch.clientX - dragStartX)
  panY.value = dragOriginY + (touch.clientY - dragStartY)
}

function onViewerKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    event.stopPropagation()
    closeViewer()
  }
}


function closeViewer(): void {
  emit('close')
}

let windowKey: ((event: KeyboardEvent) => void) | null = null

watch(
  () => props.open,
  async (open) => {
    if (open) {
      resetView()
      lockBody()
      windowKey = (event: KeyboardEvent) => {
        if (event.key === 'Escape') closeViewer()
      }
      window.addEventListener('keydown', windowKey)
      await nextTick()
      closeButton.value?.focus()
    } else {
      dragging.value = false
      unlockBody()
      if (windowKey) window.removeEventListener('keydown', windowKey)
      windowKey = null
    }
  },
)

onBeforeUnmount(() => {
  if (props.open) unlockBody()
  if (windowKey) window.removeEventListener('keydown', windowKey)
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="props.open"
      class="plantuml-viewer-backdrop"
      role="dialog"
      aria-modal="true"
      aria-label="图表放大查看"
      data-test="plantuml-viewer"
      @click.self="closeViewer"
      @keydown="onViewerKeydown"
    >
      <div class="plantuml-viewer-panel" :class="{ 'is-dark': dark }">
        <div class="plantuml-viewer-toolbar">
          <span class="plantuml-viewer-title">{{ props.title }}</span>
          <div class="plantuml-viewer-zoom" role="group" aria-label="缩放">
            <button
              type="button"
              class="plantuml-viewer-btn"
              data-test="plantuml-zoom-out"
              aria-label="缩小"
              :disabled="zoom <= MIN_ZOOM"
              @click="zoomOut"
            >
              −
            </button>
            <button
              type="button"
              class="plantuml-viewer-btn plantuml-viewer-percent"
              data-test="plantuml-zoom-reset"
              title="重置为 100%"
              @click="resetView"
            >
              {{ zoomPercent }}
            </button>
            <button
              type="button"
              class="plantuml-viewer-btn"
              data-test="plantuml-zoom-in"
              aria-label="放大"
              :disabled="zoom >= MAX_ZOOM"
              @click="zoomIn"
            >
              +
            </button>
          </div>
          <button
            ref="viewer-close"
            type="button"
            class="plantuml-viewer-btn plantuml-viewer-close"
            data-test="plantuml-viewer-close"
            @click="closeViewer"
          >
            关闭
          </button>
        </div>
        <div
          class="plantuml-viewer-canvas"
          :class="{ 'is-dark': dark, 'is-dragging': dragging }"
          data-test="plantuml-viewer-canvas"
          @mousedown="startDrag"
          @mousemove="onDrag"
          @mouseup="endDrag"
          @mouseleave="endDrag"
          @wheel.prevent="onWheel"
          @touchstart="startTouchDrag"
          @touchmove.prevent="onTouchDrag"
          @touchend="endDrag"
        >
          <div
            class="plantuml-viewer-content"
            data-test="plantuml-viewer-content"
            :style="contentStyle"
            v-html="svg"
          ></div>
        </div>
        <p class="plantuml-viewer-hint">拖拽平移 · 滚轮缩放 · Esc 关闭</p>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/* Enlarged viewer: the backdrop locks the page (body overflow is hidden while
   open) and the panel owns nearly the whole viewport, so a diagram shrunk in
   the message log gets room to breathe. The canvas clips and pans instead of
   scrolling the page; the content keeps its intrinsic size and is moved with
   translate + scale from the top-left, which is what makes drag deltas and
   wheel-around-cursor math line up. */
.plantuml-viewer-backdrop {
  position: fixed;
  inset: 0;
  z-index: 40;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  background: rgba(15, 23, 42, 0.7);
}

.plantuml-viewer-panel {
  display: flex;
  flex-direction: column;
  width: 100vw;
  height: 100vh;
  max-height: 100vh;
  border-radius: 0;
  background: var(--color-surface);
  box-shadow: none;
  overflow: hidden;
}

.plantuml-viewer-toolbar {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.6rem 0.9rem;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface-muted);
}

.plantuml-viewer-title {
  font-weight: 700;
}

.plantuml-viewer-zoom {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  margin-left: auto;
}

.plantuml-viewer-btn {
  padding: 0.25rem 0.6rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.85rem;
  cursor: pointer;
}

.plantuml-viewer-btn:disabled {
  opacity: 0.4;
  cursor: default;
}

.plantuml-viewer-percent {
  min-width: 3.5rem;
}

.plantuml-viewer-canvas {
  flex: 1;
  min-height: 0;
  max-height: none;
  overflow: hidden;
  padding: 1rem;
  background: #ffffff;
  cursor: grab;
  touch-action: none;
  user-select: none;
}

.plantuml-viewer-canvas.is-dark {
  background: #1e1e2e;
}

.plantuml-viewer-canvas.is-dragging {
  cursor: grabbing;
}

.plantuml-viewer-content {
  transform-origin: 0 0;
  display: inline-block;
}

.plantuml-viewer-content :deep(svg) {
  display: block;
  max-width: none;
  height: auto;
}

.plantuml-viewer-hint {
  margin: 0;
  padding: 0.5rem 0.9rem;
  border-top: 1px solid var(--color-border);
  color: var(--color-text-muted);
  font-size: 0.78rem;
  background: var(--color-surface-muted);
}

</style>
