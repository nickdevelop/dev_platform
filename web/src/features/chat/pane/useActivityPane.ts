// The chat pane's controls for the middle preview pane (thinking/tools & file tabs)
// and right file-tree drawer (docked vs floating, collapse state and persistence).
// Persistence is localStorage (best-effort only); the layout is view state, not session data.

import { computed, onMounted, onUnmounted, ref } from 'vue'

const ACTIVITY_STORAGE_KEY = 'dev-platform-activity-pane-v1'
const FILETREE_PINNED_STORAGE_KEY = 'dev-platform-chat-filetree-pinned-v1'
const FILETREE_COLLAPSED_STORAGE_KEY = 'dev-platform-chat-filetree-collapsed-v1'

export function useActivityPane(onExpand?: () => void) {
  // Preview pane collapse state (persisted as activity-pane for compatibility)
  const previewCollapsed = ref(true)
  try {
    const stored = localStorage.getItem(ACTIVITY_STORAGE_KEY)
    if (stored !== null) {
      previewCollapsed.value = stored === 'collapsed'
    } else {
      previewCollapsed.value = true
    }
  } catch {
    previewCollapsed.value = true
  }

  function setPreviewCollapsed(collapsed: boolean) {
    previewCollapsed.value = collapsed
    try {
      localStorage.setItem(ACTIVITY_STORAGE_KEY, collapsed ? 'collapsed' : 'expanded')
    } catch {
      // Persistence is best-effort only.
    }
    if (!collapsed && onExpand) {
      onExpand()
    }
  }

  // File tree drawer state: pinned vs floating, collapsed vs expanded
  const fileTreePinned = ref(true)
  try {
    const stored = localStorage.getItem(FILETREE_PINNED_STORAGE_KEY)
    if (stored !== null) {
      fileTreePinned.value = stored === 'pinned'
    } else {
      fileTreePinned.value = true
    }
  } catch {
    fileTreePinned.value = true
  }

  const fileTreeCollapsed = ref(false)
  try {
    const stored = localStorage.getItem(FILETREE_COLLAPSED_STORAGE_KEY)
    if (stored !== null) {
      fileTreeCollapsed.value = stored === 'collapsed'
    } else {
      fileTreeCollapsed.value = false
    }
  } catch {
    fileTreeCollapsed.value = false
  }

  const fileTreeFloatingOpen = ref(false)

  function setFileTreePinned(pinned: boolean) {
    fileTreePinned.value = pinned
    try {
      localStorage.setItem(FILETREE_PINNED_STORAGE_KEY, pinned ? 'pinned' : 'unpinned')
    } catch {
      // Persistence is best-effort only.
    }
    if (pinned) {
      fileTreeCollapsed.value = false
      fileTreeFloatingOpen.value = false
      try {
        localStorage.setItem(FILETREE_COLLAPSED_STORAGE_KEY, 'expanded')
      } catch {
        // Persistence is best-effort only.
      }
    } else {
      fileTreeFloatingOpen.value = true
    }
  }

  function toggleFileTreePinned() {
    setFileTreePinned(!fileTreePinned.value)
  }

  function setFileTreeCollapsed(collapsed: boolean) {
    fileTreeCollapsed.value = collapsed
    try {
      localStorage.setItem(FILETREE_COLLAPSED_STORAGE_KEY, collapsed ? 'collapsed' : 'expanded')
    } catch {
      // Persistence is best-effort only.
    }
    if (collapsed) {
      fileTreeFloatingOpen.value = false
    }
  }

  function toggleFileTree() {
    if (isFileTreeVisible.value) {
      setFileTreeCollapsed(true)
      fileTreeFloatingOpen.value = false
    } else {
      if (fileTreePinned.value) {
        setFileTreeCollapsed(false)
      } else {
        fileTreeFloatingOpen.value = true
      }
    }
  }

  function openFloatingFileTree() {
    fileTreeFloatingOpen.value = true
  }

  function closeFloatingFileTree() {
    fileTreeFloatingOpen.value = false
  }

  const isFileTreeDocked = computed(() => fileTreePinned.value && !fileTreeCollapsed.value)
  const isFileTreeFloating = computed(() => {
    if (isFileTreeDocked.value) return false
    return fileTreeFloatingOpen.value
  })
  const isFileTreeVisible = computed(() => isFileTreeDocked.value || isFileTreeFloating.value)

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      if (isFileTreeFloating.value) {
        closeFloatingFileTree()
        return
      }
      if (!previewCollapsed.value) {
        setPreviewCollapsed(true)
      }
    }
  }

  onMounted(() => {
    window.addEventListener('keydown', onKeydown)
  })

  onUnmounted(() => {
    window.removeEventListener('keydown', onKeydown)
  })

  return {
    // Preview pane (with activityCollapsed alias for test compatibility)
    previewCollapsed,
    setPreviewCollapsed,
    activityCollapsed: previewCollapsed,
    setActivityCollapsed: setPreviewCollapsed,

    // File tree drawer
    fileTreePinned,
    fileTreeCollapsed,
    fileTreeFloatingOpen,
    isFileTreeDocked,
    isFileTreeFloating,
    isFileTreeVisible,
    setFileTreePinned,
    toggleFileTreePinned,
    setFileTreeCollapsed,
    toggleFileTree,
    openFloatingFileTree,
    closeFloatingFileTree,
  }
}
