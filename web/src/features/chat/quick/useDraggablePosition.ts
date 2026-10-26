// Draggable position for the quick-assistant launcher and floating dialog.
// The toolbar and the panel positions are completely decoupled and independently
// draggable and persistable. Panel placement stays inside the viewport.

import { computed, onBeforeUnmount, onMounted, ref, type ComputedRef, type Ref } from 'vue'

export const POSITION_STORAGE_KEY = 'dev-platform-quick-assistant-position'
export const PANEL_POSITION_STORAGE_KEY = 'dev-platform-quick-assistant-panel-position'
// The initial placement keeps the button 50px from the top and right edges.
export const BUTTON_DEFAULT_OFFSET = 50
// The drag area covers the whole viewport; this margin only prevents the
// button from being pushed completely against an edge and becoming hard to grab.
export const VIEWPORT_EDGE_MARGIN = 12
export const PANEL_GAP = 12
export const DEFAULT_PANEL_SIZE: Size = { width: 480, height: 600 }

export interface Point {
  left: number
  top: number
}

export interface Size {
  width: number
  height: number
}

export interface Viewport {
  width: number
  height: number
}

function clamp(value: number, min: number, max: number): number {
  if (max < min) return min
  return Math.min(Math.max(value, min), max)
}

export function clampButtonPosition(point: Point, button: Size, viewport: Viewport): Point {
  const maxLeft = Math.max(VIEWPORT_EDGE_MARGIN, viewport.width - VIEWPORT_EDGE_MARGIN - button.width)
  const maxTop = Math.max(VIEWPORT_EDGE_MARGIN, viewport.height - VIEWPORT_EDGE_MARGIN - button.height)
  return {
    left: clamp(point.left, VIEWPORT_EDGE_MARGIN, maxLeft),
    top: clamp(point.top, VIEWPORT_EDGE_MARGIN, maxTop),
  }
}

export function clampPanelPosition(point: Point, panel: Size, viewport: Viewport): Point {
  const maxLeft = Math.max(VIEWPORT_EDGE_MARGIN, viewport.width - VIEWPORT_EDGE_MARGIN - panel.width)
  const maxTop = Math.max(VIEWPORT_EDGE_MARGIN, viewport.height - VIEWPORT_EDGE_MARGIN - panel.height)
  return {
    left: clamp(point.left, VIEWPORT_EDGE_MARGIN, maxLeft),
    top: clamp(point.top, VIEWPORT_EDGE_MARGIN, maxTop),
  }
}

// placePanel puts a 480x600 panel beside the button: below when there is room,
// above otherwise, right-aligned to the button and clamped to the viewport.
export function placePanel(
  button: Point & Size,
  viewport: Viewport,
  panelSize: Size = DEFAULT_PANEL_SIZE,
): Point & Size {
  const width = Math.min(panelSize.width, viewport.width - VIEWPORT_EDGE_MARGIN * 2)
  const height = Math.min(panelSize.height, viewport.height - VIEWPORT_EDGE_MARGIN * 2)

  const left = clamp(
    button.left + button.width - width,
    VIEWPORT_EDGE_MARGIN,
    viewport.width - VIEWPORT_EDGE_MARGIN - width,
  )

  let top = button.top + button.height + PANEL_GAP
  if (top + height > viewport.height - VIEWPORT_EDGE_MARGIN) {
    top = button.top - PANEL_GAP - height
  }
  top = clamp(top, VIEWPORT_EDGE_MARGIN, viewport.height - VIEWPORT_EDGE_MARGIN - height)

  return { left, top, width, height }
}

export function readStoredPosition(): Point | null {
  try {
    const raw = localStorage.getItem(POSITION_STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<Point>
    if (typeof parsed.left !== 'number' || typeof parsed.top !== 'number') return null
    if (!Number.isFinite(parsed.left) || !Number.isFinite(parsed.top)) return null
    return { left: parsed.left, top: parsed.top }
  } catch {
    return null
  }
}

function persistPosition(point: Point) {
  try {
    localStorage.setItem(POSITION_STORAGE_KEY, JSON.stringify(point))
  } catch {
    // Best-effort preference only.
  }
}

export function readStoredPanelPosition(): Point | null {
  try {
    const raw = localStorage.getItem(PANEL_POSITION_STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<Point>
    if (typeof parsed.left !== 'number' || typeof parsed.top !== 'number') return null
    if (!Number.isFinite(parsed.left) || !Number.isFinite(parsed.top)) return null
    return { left: parsed.left, top: parsed.top }
  } catch {
    return null
  }
}

export function persistPanelPosition(point: Point) {
  try {
    localStorage.setItem(PANEL_POSITION_STORAGE_KEY, JSON.stringify(point))
  } catch {
    // Best-effort preference only.
  }
}

export function initialPanelPosition(viewport: Viewport, panelSize: Size = DEFAULT_PANEL_SIZE): Point {
  const width = Math.min(panelSize.width, viewport.width - VIEWPORT_EDGE_MARGIN * 2)
  const height = Math.min(panelSize.height, viewport.height - VIEWPORT_EDGE_MARGIN * 2)
  const defaultLeft = Math.max(VIEWPORT_EDGE_MARGIN, viewport.width - width - 60)
  const defaultTop = Math.max(VIEWPORT_EDGE_MARGIN, 60)
  return clampPanelPosition({ left: defaultLeft, top: defaultTop }, { width, height }, viewport)
}

interface DragState {
  pointerId: number
  startX: number
  startY: number
  origin: Point
  moved: boolean
}

export function useDraggablePosition(buttonRef: Ref<HTMLElement | null>) {
  const position = ref<Point | null>(null)
  const buttonSize = ref<Size>({ width: 0, height: 0 })
  const viewport = ref<Viewport>({
    width: typeof window === 'undefined' ? 1024 : window.innerWidth,
    height: typeof window === 'undefined' ? 768 : window.innerHeight,
  })
  const dragging = ref(false)

  let drag: DragState | null = null
  let suppressClick = false

  function measureButton(): Size {
    const element = buttonRef.value
    if (!element) return buttonSize.value
    const rect = element.getBoundingClientRect()
    buttonSize.value = { width: rect.width, height: rect.height }
    return buttonSize.value
  }

  function setPosition(next: Point) {
    position.value = clampButtonPosition(next, buttonSize.value, viewport.value)
  }

  function refreshOnResize() {
    viewport.value = {
      width: window.innerWidth,
      height: window.innerHeight,
    }
    if (position.value) setPosition(position.value)
  }

  onMounted(() => {
    const size = measureButton()
    const stored = readStoredPosition()
    position.value = clampButtonPosition(
      stored ?? {
        left: viewport.value.width - BUTTON_DEFAULT_OFFSET - size.width,
        top: BUTTON_DEFAULT_OFFSET,
      },
      size,
      viewport.value,
    )
    window.addEventListener('resize', refreshOnResize)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('resize', refreshOnResize)
  })

  function onPointerDown(event: PointerEvent) {
    if (event.pointerType === 'mouse' && event.button !== 0) return
    const element = buttonRef.value
    if (!element) return
    const size = measureButton()
    if (!position.value) {
      const rect = element.getBoundingClientRect()
      position.value = clampButtonPosition({ left: rect.left, top: rect.top }, size, viewport.value)
    }
    drag = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      origin: { ...position.value },
      moved: false,
    }
  }

  function onPointerMove(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.pointerId) return
    const dx = event.clientX - drag.startX
    const dy = event.clientY - drag.startY
    if (!drag.moved) {
      if (Math.hypot(dx, dy) < 4) return
      drag.moved = true
      dragging.value = true
      try {
        buttonRef.value?.setPointerCapture?.(event.pointerId)
      } catch {
        // Pointer capture is an enhancement; dragging still works without it.
      }
    }
    setPosition({ left: drag.origin.left + dx, top: drag.origin.top + dy })
  }

  function finishDrag(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.pointerId) return
    const moved = drag.moved
    const pointerId = drag.pointerId
    drag = null
    dragging.value = false
    if (moved) {
      try {
        buttonRef.value?.releasePointerCapture?.(pointerId)
      } catch {
        // Already released.
      }
      if (position.value) persistPosition(position.value)
      // The click event follows pointerup; swallow exactly that one so dragging
      // never toggles the popover.
      suppressClick = true
      window.setTimeout(() => {
        suppressClick = false
      }, 0)
    }
  }

  function onPointerUp(event: PointerEvent) {
    finishDrag(event)
  }

  function onPointerCancel(event: PointerEvent) {
    finishDrag(event)
  }

  // consumeClickSuppression reports, once, whether the click was the tail of a
  // drag and should be ignored.
  function consumeClickSuppression(): boolean {
    if (!suppressClick) return false
    suppressClick = false
    return true
  }

  const buttonStyle: ComputedRef<Record<string, string> | undefined> = computed(() => {
    if (!position.value) return undefined
    return {
      left: `${position.value.left}px`,
      top: `${position.value.top}px`,
      right: 'auto',
    }
  })

  return {
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
  }
}

export function useDraggablePanel() {
  const panelPosition = ref<Point | null>(null)
  const panelSize = ref<Size>(DEFAULT_PANEL_SIZE)
  const viewport = ref<Viewport>({
    width: typeof window === 'undefined' ? 1024 : window.innerWidth,
    height: typeof window === 'undefined' ? 768 : window.innerHeight,
  })
  const draggingPanel = ref(false)
  let drag: DragState | null = null

  function actualPanelSize(): Size {
    return {
      width: Math.min(panelSize.value.width, viewport.value.width - VIEWPORT_EDGE_MARGIN * 2),
      height: Math.min(panelSize.value.height, viewport.value.height - VIEWPORT_EDGE_MARGIN * 2),
    }
  }

  function setPanelPos(next: Point) {
    const size = actualPanelSize()
    panelPosition.value = clampPanelPosition(next, size, viewport.value)
  }

  function refreshOnResize() {
    viewport.value = {
      width: window.innerWidth,
      height: window.innerHeight,
    }
    if (panelPosition.value) setPanelPos(panelPosition.value)
  }

  onMounted(() => {
    const stored = readStoredPanelPosition()
    const size = actualPanelSize()
    panelPosition.value = clampPanelPosition(
      stored ?? initialPanelPosition(viewport.value, size),
      size,
      viewport.value,
    )
    window.addEventListener('resize', refreshOnResize)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('resize', refreshOnResize)
  })

  function onPanelPointerDown(event: PointerEvent) {
    if (event.pointerType === 'mouse' && event.button !== 0) return
    const target = event.target as HTMLElement | null
    if (target?.closest('button, input, select, textarea, a')) return

    const size = actualPanelSize()
    if (!panelPosition.value) {
      panelPosition.value = initialPanelPosition(viewport.value, size)
    }

    drag = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      origin: { ...panelPosition.value },
      moved: false,
    }
    draggingPanel.value = true
    try {
      ;(event.currentTarget as HTMLElement)?.setPointerCapture?.(event.pointerId)
    } catch {
      // Pointer capture is an enhancement
    }
  }

  function onPanelPointerMove(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.pointerId) return
    const dx = event.clientX - drag.startX
    const dy = event.clientY - drag.startY
    if (!drag.moved && Math.hypot(dx, dy) < 4) return
    drag.moved = true
    setPanelPos({ left: drag.origin.left + dx, top: drag.origin.top + dy })
  }

  function finishPanelDrag(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.pointerId) return
    const moved = drag.moved
    drag = null
    draggingPanel.value = false
    try {
      ;(event.currentTarget as HTMLElement)?.releasePointerCapture?.(event.pointerId)
    } catch {
      // Already released
    }
    if (moved && panelPosition.value) {
      persistPanelPosition(panelPosition.value)
    }
  }

  function onPanelPointerUp(event: PointerEvent) {
    finishPanelDrag(event)
  }

  function onPanelPointerCancel(event: PointerEvent) {
    finishPanelDrag(event)
  }

  const panelStyle = computed(() => {
    const size = actualPanelSize()
    const pos = panelPosition.value ?? initialPanelPosition(viewport.value, size)
    return {
      left: `${pos.left}px`,
      top: `${pos.top}px`,
      width: `${size.width}px`,
      height: `${size.height}px`,
    }
  })

  return {
    panelPosition,
    panelStyle,
    draggingPanel,
    onPanelPointerDown,
    onPanelPointerMove,
    onPanelPointerUp,
    onPanelPointerCancel,
  }
}
