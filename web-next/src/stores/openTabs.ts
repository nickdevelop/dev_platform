import { create } from 'zustand'

export type OpenTab =
  | { kind: 'session'; projectId: number; sessionId: string; title: string; pinned?: boolean }
  | { kind: 'project-settings'; projectId: number; pinned?: boolean }
  | { kind: 'new-session'; pinned?: boolean }
  | { kind: 'task'; taskId: number; title: string; pinned?: boolean }
  | { kind: 'platform-settings'; pinned?: boolean }

export function tabKey(tab: OpenTab): string {
  switch (tab.kind) {
    case 'session':
      return tab.sessionId
    case 'project-settings':
      return `settings:${tab.projectId}`
    case 'new-session':
      return 'new-session'
    case 'task':
      return `task:${tab.taskId}`
    case 'platform-settings':
      return 'platform-settings'
  }
}

const STORAGE_KEY = 'dev-platform-open-tabs-v2'
const ACTIVE_TAB_KEY = 'dev-platform-active-tab-key'
const LEGACY_STORAGE_KEY = 'dev-platform-open-tabs'
const MAX_TABS = 20
const PINNED = { pinned: true } as const

function loadPersistedActiveKey(): string | null {
  try {
    return localStorage.getItem(ACTIVE_TAB_KEY)
  } catch {
    return null
  }
}

function persistActiveKey(key: string | null) {
  try {
    if (key) localStorage.setItem(ACTIVE_TAB_KEY, key)
    else localStorage.removeItem(ACTIVE_TAB_KEY)
  } catch {
    // Best-effort preference only.
  }
}

function isSession(entry: unknown): entry is Extract<OpenTab, { kind: 'session' }> {
  const tab = entry as Record<string, unknown>
  return (
    typeof tab === 'object' &&
    tab !== null &&
    typeof tab.sessionId === 'string' &&
    typeof tab.projectId === 'number'
  )
}

function normalize(entry: unknown): OpenTab | null {
  const tab = entry as Record<string, unknown>
  if (typeof tab !== 'object' || tab === null) return null
  const pinned = tab.pinned === true ? PINNED : {}
  if (tab.kind === 'new-session') return { kind: 'new-session', ...pinned }
  if (tab.kind === 'task') {
    if (typeof tab.taskId !== 'number') return null
    return {
      kind: 'task',
      taskId: tab.taskId,
      title: typeof tab.title === 'string' ? tab.title : '',
      ...pinned,
    }
  }
  if (tab.kind === 'platform-settings') return { kind: 'platform-settings', ...pinned }
  if (typeof tab.projectId !== 'number') return null
  if (tab.kind === 'project-settings') {
    return { kind: 'project-settings', projectId: tab.projectId, ...pinned }
  }
  if (tab.kind === 'session' || tab.kind === undefined) {
    if (!isSession(entry)) return null
    return {
      kind: 'session',
      projectId: entry.projectId,
      sessionId: entry.sessionId,
      title: typeof tab.title === 'string' ? tab.title : '',
      ...pinned,
    }
  }
  return null
}

function read(key: string): unknown[] {
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return []
    const parsed = JSON.parse(raw) as unknown
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

function hasStored(key: string): boolean {
  try {
    return localStorage.getItem(key) !== null
  } catch {
    return false
  }
}

function forget(key: string) {
  try {
    localStorage.removeItem(key)
  } catch {
    // Best-effort only.
  }
}

function persist(tabs: OpenTab[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(tabs))
  } catch {
    // Best-effort only.
  }
}

function loadPersisted(): OpenTab[] {
  if (hasStored(STORAGE_KEY)) {
    forget(LEGACY_STORAGE_KEY)
    return read(STORAGE_KEY)
      .map(normalize)
      .filter((tab): tab is OpenTab => tab !== null)
      .slice(0, MAX_TABS)
  }
  const migrated = read(LEGACY_STORAGE_KEY)
    .map(normalize)
    .filter((tab): tab is OpenTab => tab !== null)
    .slice(0, MAX_TABS)
  if (migrated.length > 0) persist(migrated)
  forget(LEGACY_STORAGE_KEY)
  return migrated
}

function pinnedFirst(tabs: OpenTab[]): OpenTab[] {
  return [...tabs.filter((tab) => tab.pinned === true), ...tabs.filter((tab) => tab.pinned !== true)]
}

function trim(tabs: OpenTab[]): OpenTab[] {
  const next = [...tabs]
  while (next.length > MAX_TABS) {
    const index = next.findIndex((tab) => tab.pinned !== true)
    if (index === -1) break
    next.splice(index, 1)
  }
  return next
}

interface OpenTabsState {
  tabs: OpenTab[]
  lastActiveKey: string | null
  setActiveKey: (key: string | null) => void
  openTab: (projectId: number, sessionId: string, title?: string) => void
  openProjectSettings: (projectId: number) => void
  openNewSession: () => void
  openTask: (taskId: number, title?: string) => void
  openPlatformSettings: () => void
  promoteNewSession: (projectId: number, sessionId: string) => void
  updateTaskTitle: (taskId: number, title: string) => void
  updateTitle: (sessionId: string, title: string) => void
  closeTab: (sessionId: string) => void
  closeTabByKey: (key: string) => void
  pinTab: (key: string) => void
  unpinTab: (key: string) => void
  togglePin: (key: string) => void
  closeOtherTabsByKey: (key: string) => OpenTab[]
  closeAllTabs: () => OpenTab[]
}

export const useOpenTabsStore = create<OpenTabsState>((set, get) => {
  const commit = (tabs: OpenTab[], activeKey = get().lastActiveKey) => {
    persist(tabs)
    set({ tabs, lastActiveKey: activeKey })
    persistActiveKey(activeKey)
  }

  return {
    tabs: loadPersisted(),
    lastActiveKey: loadPersistedActiveKey(),

    setActiveKey: (key) => {
      set({ lastActiveKey: key })
      persistActiveKey(key)
    },

    openTab: (projectId, sessionId, title = '') => {
      const tabs = [...get().tabs]
      const index = tabs.findIndex(
        (tab) => tab.kind === 'session' && tab.sessionId === sessionId,
      )
      if (index >= 0) {
        const existing = tabs[index] as Extract<OpenTab, { kind: 'session' }>
        tabs[index] = {
          ...existing,
          projectId,
          title: title !== '' ? title : existing.title,
        }
      } else {
        tabs.push({ kind: 'session', projectId, sessionId, title })
      }
      commit(trim(tabs), sessionId)
    },

    openProjectSettings: (projectId) => {
      const tabs = [...get().tabs]
      const index = tabs.findIndex(
        (tab) => tab.kind === 'project-settings' && tab.projectId === projectId,
      )
      if (index >= 0) {
        const [existing] = tabs.splice(index, 1)
        tabs.push(existing)
      } else {
        tabs.push({ kind: 'project-settings', projectId })
      }
      commit(trim(pinnedFirst(tabs)), `settings:${projectId}`)
    },

    openNewSession: () => {
      const tabs = [...get().tabs]
      const index = tabs.findIndex((tab) => tab.kind === 'new-session')
      if (index < 0) tabs.push({ kind: 'new-session' })
      commit(trim(tabs), 'new-session')
    },

    openTask: (taskId, title = '') => {
      const tabs = [...get().tabs]
      const index = tabs.findIndex((tab) => tab.kind === 'task' && tab.taskId === taskId)
      if (index >= 0) {
        const existing = tabs[index] as Extract<OpenTab, { kind: 'task' }>
        tabs[index] = { ...existing, title: title !== '' ? title : existing.title }
      } else {
        tabs.push({ kind: 'task', taskId, title })
      }
      commit(trim(tabs), `task:${taskId}`)
    },

    openPlatformSettings: () => {
      const tabs = [...get().tabs]
      if (!tabs.some((tab) => tab.kind === 'platform-settings')) {
        tabs.push({ kind: 'platform-settings' })
      }
      commit(trim(tabs), 'platform-settings')
    },

    promoteNewSession: (projectId, sessionId) => {
      const tabs = [...get().tabs]
      const index = tabs.findIndex((tab) => tab.kind === 'new-session')
      if (index < 0) {
        tabs.push({ kind: 'session', projectId, sessionId, title: '' })
      } else {
        const pinned = tabs[index].pinned === true ? PINNED : {}
        tabs.splice(index, 1, { kind: 'session', projectId, sessionId, title: '', ...pinned })
      }
      commit(trim(tabs), sessionId)
    },

    updateTaskTitle: (taskId, title) => {
      const tabs = get().tabs.map((tab) =>
        tab.kind === 'task' && tab.taskId === taskId ? { ...tab, title } : tab,
      )
      commit(tabs)
    },

    updateTitle: (sessionId, title) => {
      const tabs = get().tabs.map((tab) =>
        tab.kind === 'session' && tab.sessionId === sessionId ? { ...tab, title } : tab,
      )
      commit(tabs)
    },

    closeTab: (sessionId) => get().closeTabByKey(sessionId),

    closeTabByKey: (key) => {
      const tabs = get().tabs.filter((tab) => tabKey(tab) !== key)
      const active =
        get().lastActiveKey === key
          ? tabs.length > 0
            ? tabKey(tabs[tabs.length - 1])
            : null
          : get().lastActiveKey
      commit(tabs, active)
    },

    pinTab: (key) => {
      const tabs = get().tabs.map((tab) =>
        tabKey(tab) === key ? { ...tab, pinned: true } : tab,
      )
      commit(pinnedFirst(tabs))
    },

    unpinTab: (key) => {
      const tabs = get().tabs.map((tab) => {
        if (tabKey(tab) !== key) return tab
        const next = { ...tab }
        delete next.pinned
        return next
      })
      commit(pinnedFirst(tabs))
    },

    togglePin: (key) => {
      const tab = get().tabs.find((entry) => tabKey(entry) === key)
      if (!tab) return
      if (tab.pinned === true) get().unpinTab(key)
      else get().pinTab(key)
    },

    closeOtherTabsByKey: (key) => {
      const closing = get().tabs.filter(
        (tab) => tabKey(tab) !== key && tab.pinned !== true,
      )
      const tabs = get().tabs.filter(
        (tab) => tabKey(tab) === key || tab.pinned === true,
      )
      const active = tabs.some((tab) => tabKey(tab) === get().lastActiveKey)
        ? get().lastActiveKey
        : tabs.length > 0
          ? tabKey(tabs[0])
          : null
      commit(tabs, active)
      return closing
    },

    closeAllTabs: () => {
      const closing = get().tabs.filter((tab) => tab.pinned !== true)
      const tabs = get().tabs.filter((tab) => tab.pinned === true)
      const active = tabs.some((tab) => tabKey(tab) === get().lastActiveKey)
        ? get().lastActiveKey
        : tabs.length > 0
          ? tabKey(tabs[0])
          : null
      commit(tabs, active)
      return closing
    },
  }
})
