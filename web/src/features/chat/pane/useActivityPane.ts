// The chat pane's controls for the right activity popup/drawer: collapse
// state and persistence. Persistence is localStorage (best-effort only); the
// layout is view state, not session data.

import { onMounted, onUnmounted, ref } from 'vue'

// Whether the activity pane is collapsed. Persisted in localStorage so
// a refresh keeps the user's choice; defaults to collapsed (floating FAB).
const ACTIVITY_STORAGE_KEY = 'dev-platform-activity-pane-v1'

export function useActivityPane(onExpand: () => void) {
  const activityCollapsed = ref(true)
  try {
    const stored = localStorage.getItem(ACTIVITY_STORAGE_KEY)
    if (stored !== null) {
      activityCollapsed.value = stored === 'collapsed'
    } else {
      activityCollapsed.value = true
    }
  } catch {
    activityCollapsed.value = true
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

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !activityCollapsed.value) {
      setActivityCollapsed(true)
    }
  }

  onMounted(() => {
    window.addEventListener('keydown', onKeydown)
  })

  onUnmounted(() => {
    window.removeEventListener('keydown', onKeydown)
  })

  return {
    activityCollapsed,
    setActivityCollapsed,
  }
}
