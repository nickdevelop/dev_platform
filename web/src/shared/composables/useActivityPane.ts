// The chat pane's controls for the right workspace pane (file preview tabs & file-tree drawer).
// Persistence is localStorage (best-effort only); the layout is view state, not session data.

import { computed, onMounted, onUnmounted, ref } from 'vue'

const ACTIVITY_STORAGE_KEY = 'dev-platform-activity-pane-v1'
const PREVIEW_WIDTH_STORAGE_KEY = 'dev-platform-preview-width-v1'
const FILETREE_PINNED_STORAGE_KEY = 'dev-platform-chat-filetree-pinned-v1'
const FILETREE_COLLAPSED_STORAGE_KEY = 'dev-platform-chat-filetree-collapsed-v1'
const DEFAULT_PREVIEW_WIDTH = 580
const MIN_PREVIEW_WIDTH = 320

// Module-level singleton refs so SessionTabs in the header and SessionChatPanel in the body
// share the exact same reactive state without prop drilling.
const previewCollapsed = ref(true)
const previewWidth = ref(DEFAULT_PREVIEW_WIDTH)
const fileTreePinned = ref(true)
const fileTreeCollapsed = ref(false)
const fileTreeFloatingOpen = ref(false)

function readStorage() {
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

  try {
    const storedWidth = localStorage.getItem(PREVIEW_WIDTH_STORAGE_KEY)
    if (storedWidth !== null) {
      const parsed = parseInt(storedWidth, 10)
      if (!isNaN(parsed) && parsed >= MIN_PREVIEW_WIDTH) {
        previewWidth.value = parsed
      } else {
        previewWidth.value = DEFAULT_PREVIEW_WIDTH
      }
    } else {
      previewWidth.value = DEFAULT_PREVIEW_WIDTH
    }
  } catch {
    previewWidth.value = DEFAULT_PREVIEW_WIDTH
  }

  try {
    const storedPinned = localStorage.getItem(FILETREE_PINNED_STORAGE_KEY)
    if (storedPinned !== null) {
      fileTreePinned.value = storedPinned === 'pinned'
    } else {
      fileTreePinned.value = true
    }
  } catch {
    fileTreePinned.value = true
  }

  try {
    const storedCollapsed = localStorage.getItem(FILETREE_COLLAPSED_STORAGE_KEY)
    if (storedCollapsed !== null) {
      fileTreeCollapsed.value = storedCollapsed === 'collapsed'
    } else {
      fileTreeCollapsed.value = false
    }
  } catch {
    fileTreeCollapsed.value = false
  }
}

// Initial read
readStorage()

export function useActivityPane(onExpand?: () => void) {
  // Re-read storage on hook call so tests that set localStorage before mount will be picked up
  readStorage()

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

  function setPreviewWidth(width: number) {
    const clamped = Math.max(MIN_PREVIEW_WIDTH, Math.round(width))
    previewWidth.value = clamped
    try {
      localStorage.setItem(PREVIEW_WIDTH_STORAGE_KEY, String(clamped))
    } catch {
      // Persistence is best-effort only.
    }
  }

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
    previewWidth,
    setPreviewWidth,
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
