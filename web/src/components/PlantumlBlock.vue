<script setup lang="ts">
// One PlantUML diagram inside an agent reply. The block is shaped like the
// Thinking and tool blocks next to it: a title row that says what is in here,
// and the drawing underneath. The drawing is the default — a diagram is the
// reason the agent drew it — and the source is one click away for the person
// who wants to copy or correct it.
//
// Two rules the rest of the file serves: a reply that is still arriving holds a
// half-written fence, and asking the engine for that every few tokens is both
// pointless (it errors on the missing @enduml) and noisy, so drawing waits for
// `ready`. And the toggle belongs to this block alone — a message with three
// diagrams toggles them one at a time.
//
// The head row is a switch, not a disclosure: it never folds the block away, it
// exchanges the drawing for the source it came from, so it says so in words
// (查看源码 / 返回预览) and carries no fold marker.
//
// A reply that finished without closing the fence (stopped, truncated, or the
// model simply forgot) is a third case: there is nothing to draw, but the text
// is final and someone needs to read it, so the source is shown as they would
// have seen it today.
import { computed, nextTick, onBeforeUnmount, ref, useTemplateRef, watch } from 'vue'
import { renderDiagram } from '../plantuml/renderer'
import { isDarkTheme } from '../theme/presets'
import { useSettingsStore } from '../stores/settings'

const props = defineProps<{ source: string; ready: boolean; finished: boolean }>()

const settings = useSettingsStore()
const dark = computed(() => isDarkTheme(settings.appearance.theme))

const mode = ref<'preview' | 'source'>('preview')
const state = ref<'waiting' | 'rendering' | 'ready' | 'failed' | 'incomplete'>('waiting')
const svg = ref('')
const reason = ref('')

// Which (source, palette) pair is currently on screen, so a re-render does not
// redraw the diagram that is already there.
let drawn = ''
let request = 0
// Once someone opens the source of a drawing themselves, a redraw for a new
// palette must not shove them back to the picture.
const openedSource = ref(false)

const headLabel = computed(() => {
  if (state.value === 'waiting') return '等待完成…'
  if (state.value === 'rendering') return '正在生成图…'
  if (state.value === 'incomplete') return '图没画完'
  if (state.value === 'failed') return ''
  return mode.value === 'preview' ? '查看源码' : '返回预览'
})

const note = computed(() => {
  if (state.value === 'waiting') return '图还没写完,等这条回复结束再画。'
  if (state.value === 'rendering') return '正在生成图…'
  return '图没画完(回复在围栏结束前停止了),以下是源码。'
})

async function draw(): Promise<void> {
  const token = ++request
  if (!props.ready) {
    // Settled with an unclosed fence: the drawing can never arrive, so show what
    // is there instead of an empty promise forever.
    state.value = props.finished ? 'incomplete' : 'waiting'
    return
  }

  const key = `${dark.value ? 'd' : 'l'}\u0000${props.source}`
  if (drawn === key && state.value === 'ready') return

  state.value = 'rendering'
  const result = await renderDiagram({ source: props.source, dark: dark.value })
  if (token !== request) return // a newer reply or palette change superseded this one

  if (result.ok) {
    svg.value = result.svg
    drawn = key
    reason.value = ''
    state.value = 'ready'
    if (!openedSource.value) mode.value = 'preview'
  } else {
    reason.value = result.reason
    state.value = 'failed'
    mode.value = 'source'
  }
}

function toggle(): void {
  // Only a drawing has two faces: a waiting, failed or unfinished block shows
  // the one thing it has.
  if (state.value !== 'ready') return
  openedSource.value = mode.value === 'preview'
  mode.value = mode.value === 'preview' ? 'source' : 'preview'
}

// --- Enlarged viewer: a large diagram is shrunk to the message width, so the
// head row offers a popup that shows the same SVG on a viewport the person can
// zoom and pan. The viewer reuses `svg` (no second render) and resets its
// transform on every open, so one diagram's zoom never leaks into the next.
const MIN_ZOOM = 0.2
const MAX_ZOOM = 4
const viewerOpen = ref(false)
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

const canExpand = computed(() => state.value === 'ready' && svg.value !== '')
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

async function openViewer(): Promise<void> {
  if (!canExpand.value || viewerOpen.value) return
  resetView()
  viewerOpen.value = true
  lockBody()
  await nextTick()
  closeButton.value?.focus()
}

function closeViewer(): void {
  if (!viewerOpen.value) return
  viewerOpen.value = false
  dragging.value = false
  unlockBody()
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

watch(viewerOpen, (open) => {
  if (!open) return
  const onWindowKey = (event: KeyboardEvent) => {
    if (event.key === 'Escape') closeViewer()
  }
  window.addEventListener('keydown', onWindowKey, { once: true })
})

onBeforeUnmount(() => {
  if (viewerOpen.value) unlockBody()
})

watch(() => [props.ready, props.source, dark.value, props.finished], draw, { immediate: true })
</script>

<template>
  <div class="plantuml-block" data-test="plantuml-block">
    <div class="plantuml-head-row">
      <button type="button" class="plantuml-head" :disabled="state !== 'ready'" @click="toggle">
        <span class="plantuml-title">图表 · plantuml</span>
        <span v-if="headLabel" class="plantuml-mode">{{ headLabel }}</span>
      </button>
      <button
        v-if="canExpand"
        type="button"
        class="plantuml-expand"
        data-test="plantuml-expand"
        title="放大查看"
        aria-label="放大查看图表"
        @click="openViewer"
      >
        ⤢
      </button>
    </div>

    <div
      v-if="state === 'waiting' || state === 'rendering' || state === 'incomplete'"
      class="plantuml-note"
      role="status"
      data-test="plantuml-waiting"
    >
      {{ note }}
    </div>

    <p v-if="state === 'failed'" class="plantuml-error" role="alert" data-test="plantuml-error">
      无法渲染:{{ reason }}
    </p>

    <div
      v-if="state === 'ready' && mode === 'preview'"
      class="plantuml-canvas"
      :class="{ 'is-dark': dark }"
      data-test="plantuml-canvas"
      v-html="svg"
    ></div>

    <pre
      v-if="state === 'failed' || state === 'incomplete' || mode === 'source'"
      class="plantuml-src"
      data-test="plantuml-source"
    >{{ source }}</pre>

    <Teleport to="body">
      <div
        v-if="viewerOpen"
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
            <span class="plantuml-viewer-title">图表 · plantuml</span>
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
  </div>
</template>

<style scoped>
.plantuml-block {
  margin: calc(var(--space-unit) * 2) 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  overflow: hidden;
}

.plantuml-head-row {
  display: flex;
  align-items: stretch;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface-muted);
}

.plantuml-head {
  display: flex;
  align-items: center;
  gap: var(--space-unit);
  flex: 1;
  min-width: 0;
  padding: var(--space-unit) calc(var(--space-unit) * 1.5);
  border: 0;
  background: transparent;
  color: var(--color-text);
  font: inherit;
  font-size: calc(var(--font-size-base) - 1px);
  text-align: left;
  cursor: pointer;
}

.plantuml-head:disabled {
  cursor: default;
}

.plantuml-expand {
  flex-shrink: 0;
  padding: 0 calc(var(--space-unit) * 1.5);
  border: 0;
  border-left: 1px solid var(--color-border);
  background: transparent;
  color: var(--color-primary);
  font: inherit;
  font-size: 1rem;
  line-height: 1;
  cursor: pointer;
}

.plantuml-expand:hover {
  background: var(--color-border);
}

.plantuml-title {
  color: var(--color-text);
}

.plantuml-mode {
  margin-left: auto;
  color: var(--color-primary);
}

.plantuml-note {
  padding: calc(var(--space-unit) * 2);
  color: var(--color-text-muted);
}

.plantuml-error {
  margin: 0;
  padding: var(--space-unit) calc(var(--space-unit) * 1.5);
  border-bottom: 1px solid var(--color-danger-border);
  color: var(--color-danger);
  background: var(--color-surface-muted);
}

/* The drawing is shown whole. A diagram you have to scroll inside to read is
   not a diagram, and the message log is already the scroller here, so this
   canvas grows the message instead of clipping. A picture wider than the pane
   is scaled down as a picture (the engine's viewBox carries the ratio), never
   cropped. The canvas background follows the flag the drawing was made with: a
   dark drawing carries light ink, and neither should sit on the opposite
   background. */
.plantuml-canvas {
  padding: calc(var(--space-unit) * 2);
  background: #ffffff;
}

.plantuml-canvas.is-dark {
  background: #1e1e2e;
}

.plantuml-canvas :deep(svg) {
  display: block;
  max-width: 100%;
  height: auto;
}

.plantuml-src {
  margin: 0;
  padding: calc(var(--space-unit) * 2);
  background: var(--color-surface-muted);
  color: var(--color-text);
  font-family: var(--font-family);
  font-size: calc(var(--font-size-base) - 1px);
  line-height: 1.55;
  overflow: auto;
  white-space: pre;
}

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
  padding: 1rem;
  background: rgba(15, 23, 42, 0.55);
}

.plantuml-viewer-panel {
  display: flex;
  flex-direction: column;
  width: min(72rem, 94vw);
  max-height: 90vh;
  border-radius: 12px;
  background: var(--color-surface);
  box-shadow: 0 10px 40px rgba(15, 23, 42, 0.4);
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
  min-height: 50vh;
  max-height: calc(90vh - 7rem);
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
