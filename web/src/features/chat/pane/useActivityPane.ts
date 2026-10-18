// The chat pane's split-view controls for the right activity pane: collapse
// state, adjustable width and the draggable divider. Persistence is
// localStorage (best-effort only); the layout is view state, not session data.

import { computed, ref } from 'vue'

// Whether the right activity pane is collapsed. Persisted in localStorage so
// a refresh keeps the user's choice; defaults to expanded.
const ACTIVITY_STORAGE_KEY = 'dev-platform-activity-pane-v1'
// The activity pane width, adjustable by dragging the middle divider.
// Persisted in localStorage alongside the collapsed state.
const ACTIVITY_WIDTH_KEY = 'dev-platform-activity-width-v1'
const DEFAULT_ACTIVITY_WIDTH = 340
const MIN_ACTIVITY_WIDTH = 240
const MAX_ACTIVITY_WIDTH = 640

export function useActivityPane(onExpand: () => void) {
  const activityCollapsed = ref(false)
  try {
    activityCollapsed.value = localStorage.getItem(ACTIVITY_STORAGE_KEY) === 'collapsed'
  } catch {
    activityCollapsed.value = false
  }
  function setActivityCollapsed(collapsed: boolean) {
    activityCollapsed.value = collapsed
    try {
      localStorage.setItem(ACTIVITY_STORAGE_KEY, collapsed ? 'collapsed' : 'expanded')
    } catch {
      // Persistence is best-effort only.
    }
    // Expanding returns to the newest activity when the pane was following it.
    if (!collapsed) {
      onExpand()
    }
  }

  function clampActivityWidth(px: number): number {
    if (!Number.isFinite(px)) return DEFAULT_ACTIVITY_WIDTH
    return Math.min(MAX_ACTIVITY_WIDTH, Math.max(MIN_ACTIVITY_WIDTH, Math.round(px)))
  }
  const activityWidth = ref(DEFAULT_ACTIVITY_WIDTH)
  try {
    const stored = localStorage.getItem(ACTIVITY_WIDTH_KEY)
    if (stored !== null) activityWidth.value = clampActivityWidth(Number(stored))
  } catch {
    activityWidth.value = DEFAULT_ACTIVITY_WIDTH
  }
  function persistActivityWidth() {
    try {
      localStorage.setItem(ACTIVITY_WIDTH_KEY, String(activityWidth.value))
    } catch {
      // Persistence is best-effort only.
    }
  }
  function resetActivityWidth() {
    activityWidth.value = DEFAULT_ACTIVITY_WIDTH
    persistActivityWidth()
  }

  // Dragging the divider resizes the activity pane. The drag starts only from
  // the divider surface itself (never from its toggle button), tracks the
  // pointer on the window so fast moves don't lose it, and clamps to leave at
  // least a usable message column.
  const splitRef = ref<HTMLElement | null>(null)
  const draggingDivider = ref(false)
  // flex: 0 1 auto with min-width 0 lets the fixed width yield when the
  // viewport narrows, so the activity pane can never push the message column
  // (and with it the whole shell) into a horizontal scrollbar.
  const activityPaneStyle = computed(() =>
    activityCollapsed.value ? {} : { width: `${activityWidth.value}px`, flex: '0 1 auto', minWidth: '0' },
  )
  function onDividerMouseDown(event: MouseEvent) {
    if (event.button !== 0) return
    if ((event.target as HTMLElement).closest('button')) return
    const container = splitRef.value
    if (!container) return
    event.preventDefault()
    draggingDivider.value = true
    const startX = event.clientX
    const startWidth = activityWidth.value
    const containerWidth = container.clientWidth
    const maxWidth = Math.max(
      MIN_ACTIVITY_WIDTH,
      Math.min(MAX_ACTIVITY_WIDTH, containerWidth > 0 ? containerWidth - 320 : MAX_ACTIVITY_WIDTH),
    )
    const onMove = (move: MouseEvent) => {
      const delta = startX - move.clientX
      activityWidth.value = Math.min(maxWidth, Math.max(MIN_ACTIVITY_WIDTH, startWidth + delta))
    }
    const onUp = () => {
      draggingDivider.value = false
      persistActivityWidth()
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }
  function onDividerKeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowLeft') {
      event.preventDefault()
      activityWidth.value = clampActivityWidth(activityWidth.value + 16)
      persistActivityWidth()
    } else if (event.key === 'ArrowRight') {
      event.preventDefault()
      activityWidth.value = clampActivityWidth(activityWidth.value - 16)
      persistActivityWidth()
    } else if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      setActivityCollapsed(!activityCollapsed.value)
    }
  }

  return {
    activityCollapsed,
    setActivityCollapsed,
    activityWidth,
    splitRef,
    draggingDivider,
    activityPaneStyle,
    onDividerMouseDown,
    onDividerKeydown,
    resetActivityWidth,
  }
}
