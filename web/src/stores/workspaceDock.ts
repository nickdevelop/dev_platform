import { defineStore } from 'pinia'

export interface WorkspaceFileTab {
  key: string
  projectId: number
  worktreeId?: number
  path: string
  name: string
  mode?: 'view' | 'diff'
  commitRef?: string
  commitMessage?: string
  initialLine?: number
  isDirty?: boolean
  draftContent?: string
}

export interface WorkspaceScope {
  projectId: number
  worktreeId: number
  sessionId?: string
}

export const FILE_TREE_TAB_KEY = '__filetree__'
const ACTIVITY_STORAGE_KEY = 'dev-platform-activity-pane-v1'
const PREVIEW_WIDTH_STORAGE_KEY = 'dev-platform-preview-width-v1'
const FILE_TREE_WIDTH_STORAGE_KEY = 'dev-platform-filetree-width-v1'
const DOCK_LOCKED_KEY = 'dev-platform-workspace-dock-locked'
const DOCK_LOCKED_SCOPE_KEY = 'dev-platform-workspace-dock-locked-scope'
const DEFAULT_PREVIEW_WIDTH = 580
export const MIN_PREVIEW_WIDTH = 320
export const MIN_SPLIT_PREVIEW_WIDTH = 240
export const DEFAULT_FILE_TREE_WIDTH = 260
export const MIN_FILE_TREE_WIDTH = 200
export const MAX_FILE_TREE_WIDTH = 480

export function getFileName(filePath: string): string {
  const parts = filePath.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1] || filePath
}

export function buildFileTabKey(
  projectId: number,
  worktreeId: number | undefined,
  path: string,
  mode: 'view' | 'diff' = 'view',
): string {
  const cleanPath = path.replace(/\\/g, '/').replace(/^\/+/, '')
  const wt = worktreeId ?? 0
  return `${projectId}:${wt}:${mode}:${cleanPath}`
}

function readStorageBoolean(key: string, defaultVal: boolean): boolean {
  try {
    const item = localStorage.getItem(key)
    if (item !== null) return item === 'true' || item === 'collapsed'
  } catch {
    // Persistence is best-effort only.
  }
  return defaultVal
}

function readStorageInt(key: string, defaultVal: number, minVal: number, maxVal?: number): number {
  try {
    const item = localStorage.getItem(key)
    if (item !== null) {
      const parsed = parseInt(item, 10)
      if (!isNaN(parsed) && parsed >= minVal) {
        return maxVal !== undefined ? Math.min(maxVal, parsed) : parsed
      }
    }
  } catch {
    // Persistence is best-effort only.
  }
  return defaultVal
}

export const useWorkspaceDockStore = defineStore('workspaceDock', {
  state: () => {
    let initialLocked = false
    let initialLockedScope: WorkspaceScope | null = null
    try {
      initialLocked = localStorage.getItem(DOCK_LOCKED_KEY) === 'true'
      const scopeRaw = localStorage.getItem(DOCK_LOCKED_SCOPE_KEY)
      if (scopeRaw) {
        initialLockedScope = JSON.parse(scopeRaw) as WorkspaceScope
      }
    } catch {
      initialLocked = false
      initialLockedScope = null
    }

    return {
      activeScope: null as WorkspaceScope | null,
      isLocked: initialLocked,
      lockedScope: initialLockedScope,
      openTabs: [] as WorkspaceFileTab[],
      activeTabKey: FILE_TREE_TAB_KEY as string,
      previewCollapsed: readStorageBoolean(ACTIVITY_STORAGE_KEY, false),
      previewWidth: readStorageInt(PREVIEW_WIDTH_STORAGE_KEY, DEFAULT_PREVIEW_WIDTH, MIN_PREVIEW_WIDTH),
      fileTreeVisible: false,
      fileTreeWidth: readStorageInt(
        FILE_TREE_WIDTH_STORAGE_KEY,
        DEFAULT_FILE_TREE_WIDTH,
        MIN_FILE_TREE_WIDTH,
        MAX_FILE_TREE_WIDTH,
      ),
      quickSearchOpen: false,
    }
  },

  getters: {
    effectiveScope(state): WorkspaceScope | null {
      if (state.isLocked && state.lockedScope) {
        return state.lockedScope
      }
      return state.activeScope
    },

    currentTab(state): WorkspaceFileTab | undefined {
      if (state.activeTabKey === FILE_TREE_TAB_KEY) return undefined
      return state.openTabs.find((t) => t.key === state.activeTabKey)
    },

    isTabForeign(state) {
      return (tab: WorkspaceFileTab): boolean => {
        const scope = state.isLocked && state.lockedScope ? state.lockedScope : state.activeScope
        if (!scope) return false
        return tab.projectId !== scope.projectId || (tab.worktreeId ?? 0) !== scope.worktreeId
      }
    },

    dirtyTabs(state): WorkspaceFileTab[] {
      return state.openTabs.filter((t) => t.isDirty)
    },

    hasDirtyTabs(state): boolean {
      return state.openTabs.some((t) => t.isDirty)
    },
  },

  actions: {
    setSessionScope(projectId: number, worktreeId: number = 0, sessionId?: string) {
      this.activeScope = { projectId, worktreeId, sessionId }
      if (!this.isLocked) {
        this.lockedScope = null
      }
    },

    toggleLock(explicitScope?: WorkspaceScope) {
      this.isLocked = !this.isLocked
      if (this.isLocked) {
        const scopeToLock = explicitScope ?? this.effectiveScope ?? this.activeScope
        this.lockedScope = scopeToLock ? { ...scopeToLock } : null
        try {
          localStorage.setItem(DOCK_LOCKED_KEY, 'true')
          if (this.lockedScope) {
            localStorage.setItem(DOCK_LOCKED_SCOPE_KEY, JSON.stringify(this.lockedScope))
          }
        } catch {
          // ignore
        }
      } else {
        this.lockedScope = null
        try {
          localStorage.setItem(DOCK_LOCKED_KEY, 'false')
          localStorage.removeItem(DOCK_LOCKED_SCOPE_KEY)
        } catch {
          // ignore
        }
      }
    },

    openFile(options: {
      projectId: number
      worktreeId?: number
      path: string
      name?: string
      mode?: 'view' | 'diff'
      commitRef?: string
      commitMessage?: string
      initialLine?: number
    }): WorkspaceFileTab {
      let cleanPath = options.path.replace(/\\/g, '/')
      const isAbs = cleanPath.startsWith('/')
      if (!isAbs) {
        cleanPath = cleanPath.replace(/^\/+/, '')
      }
      const mode = options.mode || 'view'
      const key = buildFileTabKey(options.projectId, options.worktreeId, cleanPath, mode)
      const existing = this.openTabs.find((t) => t.key === key)

      if (existing) {
        existing.commitRef = options.commitRef
        existing.commitMessage = options.commitMessage
        existing.initialLine = options.initialLine
        this.activeTabKey = key
        if (this.previewCollapsed) {
          this.setPreviewCollapsed(false)
        }
        return existing
      }

      const newTab: WorkspaceFileTab = {
        key,
        projectId: options.projectId,
        worktreeId: options.worktreeId ?? 0,
        path: cleanPath,
        name: options.name || getFileName(cleanPath),
        mode,
        commitRef: options.commitRef,
        commitMessage: options.commitMessage,
        initialLine: options.initialLine,
        isDirty: false,
        draftContent: undefined,
      }

      this.openTabs.push(newTab)
      this.activeTabKey = key
      if (this.previewCollapsed) {
        this.setPreviewCollapsed(false)
      }
      return newTab
    },

    selectTab(tabKey: string) {
      this.activeTabKey = tabKey
    },

    closeTab(tabKey: string) {
      const idx = this.openTabs.findIndex((t) => t.key === tabKey)
      if (idx === -1) return

      this.openTabs.splice(idx, 1)

      if (this.activeTabKey === tabKey) {
        if (this.openTabs.length > 0) {
          const nextIdx = Math.min(idx, this.openTabs.length - 1)
          this.activeTabKey = this.openTabs[nextIdx].key
        } else {
          this.activeTabKey = FILE_TREE_TAB_KEY
        }
      }
    },

    setDraft(tabKey: string, content: string, isDirty: boolean) {
      const tab = this.openTabs.find((t) => t.key === tabKey)
      if (tab) {
        tab.draftContent = content
        tab.isDirty = isDirty
      }
    },

    markSaved(tabKey: string) {
      const tab = this.openTabs.find((t) => t.key === tabKey)
      if (tab) {
        tab.isDirty = false
      }
    },

    setPreviewCollapsed(collapsed: boolean) {
      this.previewCollapsed = collapsed
      try {
        localStorage.setItem(ACTIVITY_STORAGE_KEY, collapsed ? 'collapsed' : 'expanded')
      } catch {
        // ignore
      }
    },

    setPreviewWidth(width: number) {
      const clamped = Math.max(MIN_PREVIEW_WIDTH, Math.round(width))
      this.previewWidth = clamped
      try {
        localStorage.setItem(PREVIEW_WIDTH_STORAGE_KEY, String(clamped))
      } catch {
        // ignore
      }
    },

    setFileTreeVisible(visible: boolean) {
      this.fileTreeVisible = visible
      if (
        visible &&
        !this.previewCollapsed &&
        this.previewWidth < MIN_SPLIT_PREVIEW_WIDTH + this.fileTreeWidth
      ) {
        this.setPreviewWidth(MIN_SPLIT_PREVIEW_WIDTH + this.fileTreeWidth)
      }
    },

    toggleFileTree() {
      this.setFileTreeVisible(!this.fileTreeVisible)
    },

    setFileTreeWidth(width: number) {
      const clamped = Math.min(MAX_FILE_TREE_WIDTH, Math.max(MIN_FILE_TREE_WIDTH, Math.round(width)))
      this.fileTreeWidth = clamped
      try {
        localStorage.setItem(FILE_TREE_WIDTH_STORAGE_KEY, String(clamped))
      } catch {
        // ignore
      }
    },

    openQuickSearch() {
      this.quickSearchOpen = true
    },

    closeQuickSearch() {
      this.quickSearchOpen = false
    },
  },
})
