// Open workbench tabs: what this browser has open right now. Five kinds share
// the strip — a session (a session with its cached transcript and live SSE
// subscription), a project's settings page, the new-session form (labelled
// "new" until the session is created), a task's detail page, and the platform
// settings center. Switching a tab is navigation; the tab strip renders every
// kind from one `tabKey`, so nothing downstream has to know which is which.
// Tabs persist in localStorage so a refresh restores the working set. A tab can
// be pinned: pinned tabs sort to the front, carry no close control, and survive
// "close others"/"close all", so the pages a user anchored never disappear by
// accident.
import { defineStore } from 'pinia'

export type OpenTab =
  | { kind: 'session'; projectId: number; sessionId: string; title: string; pinned?: boolean }
  | { kind: 'project-settings'; projectId: number; pinned?: boolean }
  // The new-session form is a single strip-wide tab: opening it from any project
  // focuses the same tab, and the form's project picker decides the target.
  | { kind: 'new-session'; pinned?: boolean }
  | { kind: 'task'; taskId: number; title: string; pinned?: boolean }
  | { kind: 'platform-settings'; pinned?: boolean }

// tabKey identifies a tab. A session is identified by its session (machine-unique),
// a project's settings page by its project, a task by its task id, and the two
// platform-wide pages have fixed singleton keys; the prefixes keep the kinds
// apart.
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
// The pre-tab-model key: it stored session tabs without a kind. Read once, then
// never written again, so a user who installed this version keeps their tabs and
// never sees the old shape again.
const LEGACY_STORAGE_KEY = 'dev-platform-open-tabs'

const MAX_TABS = 20

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
    // Persistence is best-effort only.
  }
}

// Only set the flag when it is true, so an unpinned tab carries no `pinned`
// field and a round-tripped tab compares equal to one that was just opened.
const PINNED = { pinned: true } as const

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
    const title = typeof tab.title === 'string' ? tab.title : ''
    return { kind: 'task', taskId: tab.taskId, title, ...pinned }
  }
  if (tab.kind === 'platform-settings') return { kind: 'platform-settings', ...pinned }
  if (typeof tab.projectId !== 'number') return null
  if (tab.kind === 'project-settings') return { kind: 'project-settings', projectId: tab.projectId, ...pinned }
  if (tab.kind === 'session' || tab.kind === undefined) {
    if (!isSession(entry)) return null
    // A stored session tab may predate any title being set; only a string title is
    // carried over, so a hand-edited store cannot inject one.
    const title = typeof tab.title === 'string' ? tab.title : ''
    return { kind: 'session', projectId: entry.projectId, sessionId: entry.sessionId, title, ...pinned }
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

function loadPersisted(): OpenTab[] {
  // An explicitly emptied strip persists as `[]`: the key existing means the
  // user has a (possibly empty) working set, so the legacy key must not be
  // consulted again — otherwise closing every tab resurrects old tabs on the
  // next refresh. Only a missing key falls back to the one-time migration.
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
  // Writing the migrated set down at once means the old shape is read exactly
  // once, and a later visit never has to know it existed.
  if (migrated.length > 0) persist(migrated)
  forget(LEGACY_STORAGE_KEY)
  return migrated
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
    // Persistence is best-effort only.
  }
}

function persist(tabs: OpenTab[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(tabs))
  } catch {
    // Persistence is best-effort only.
  }
}

// Pinned tabs sit at the front of the strip, in the order they were pinned;
// unpinned tabs follow in opening order. A stable partition keeps both groups'
// relative order intact, which is what re-opening a tab relies on.
function pinnedFirst(tabs: OpenTab[]): OpenTab[] {
  return [...tabs.filter((tab) => tab.pinned === true), ...tabs.filter((tab) => tab.pinned !== true)]
}

// trim drops the oldest unpinned tabs until the set fits MAX_TABS. Pinned tabs
// are the user's anchors, so they are never the one dropped.
function trim(tabs: OpenTab[]) {
  while (tabs.length > MAX_TABS) {
    const index = tabs.findIndex((tab) => tab.pinned !== true)
    if (index === -1) break
    tabs.splice(index, 1)
  }
}

export const useOpenTabsStore = defineStore('openTabs', {
  state: () => ({
    tabs: loadPersisted() as OpenTab[],
    lastActiveKey: loadPersistedActiveKey(),
  }),

  getters: {
    // preferredTab returns the last active tab if still open, otherwise the first tab, or null if no tabs.
    preferredTab: (state): OpenTab | null => {
      if (state.tabs.length === 0) return null
      if (state.lastActiveKey) {
        const found = state.tabs.find((tab) => tabKey(tab) === state.lastActiveKey)
        if (found) return found
      }
      return state.tabs[0]
    },
  },

  actions: {
    setActiveKey(key: string | null) {
      this.lastActiveKey = key
      persistActiveKey(key)
    },

    // Opens a session tab, or updates the existing one in place: re-opening the
    // session the user is already looking at must not reshuffle the strip.
    openTab(projectId: number, sessionId: string, title = '') {
      const existing = this.tabs.find(
        (tab): tab is Extract<OpenTab, { kind: 'session' }> =>
          tab.kind === 'session' && tab.sessionId === sessionId,
      )
      if (existing) {
        existing.projectId = projectId
        if (title !== '') existing.title = title
      } else {
        this.tabs.push({ kind: 'session', projectId, sessionId, title })
        trim(this.tabs)
      }
      this.setActiveKey(sessionId)
      persist(this.tabs)
    },

    // Opens (or brings to front) a project's settings tab — one per project.
    openProjectSettings(projectId: number) {
      const existing = this.tabs.find(
        (tab) => tab.kind === 'project-settings' && tab.projectId === projectId,
      )
      if (existing) {
        this.tabs = pinnedFirst([...this.tabs.filter((tab) => tab !== existing), existing])
      } else {
        this.tabs.push({ kind: 'project-settings', projectId })
        trim(this.tabs)
        this.tabs = pinnedFirst(this.tabs)
      }
      this.setActiveKey(`settings:${projectId}`)
      persist(this.tabs)
    },

    // Opens (or focuses) the one new-session tab. It labels itself "new" until
    // the session is created, and whichever project the form targets, the same
    // tab is reused.
    openNewSession() {
      const existing = this.tabs.find((tab) => tab.kind === 'new-session')
      if (existing) {
        this.tabs = pinnedFirst([...this.tabs.filter((tab) => tab !== existing), existing])
      }
      if (!existing) {
        this.tabs.push({ kind: 'new-session' })
        trim(this.tabs)
      }
      this.setActiveKey('new-session')
      persist(this.tabs)
    },

    // Opens (or focuses) a task's detail tab. The title follows the task list
    // once it loads; a stored tab keeps the last-known title until then.
    openTask(taskId: number, title = '') {
      const existing = this.tabs.find(
        (tab): tab is Extract<OpenTab, { kind: 'task' }> => tab.kind === 'task' && tab.taskId === taskId,
      )
      if (existing) {
        if (title !== '') existing.title = title
      } else {
        this.tabs.push({ kind: 'task', taskId, title })
        trim(this.tabs)
      }
      this.setActiveKey(`task:${taskId}`)
      persist(this.tabs)
    },

    // Opens (or focuses) the platform settings tab — one per strip.
    openPlatformSettings() {
      const existing = this.tabs.find((tab) => tab.kind === 'platform-settings')
      if (existing) {
        this.tabs = pinnedFirst([...this.tabs.filter((tab) => tab !== existing), existing])
      } else {
        this.tabs.push({ kind: 'platform-settings' })
        trim(this.tabs)
      }
      this.setActiveKey('platform-settings')
      persist(this.tabs)
    },

    // promoteNewSession turns the new-session tab into a session tab in place:
    // the position in the strip is kept, and the strip's focus follows the
    // created session. A stored title arrives later, once the session list
    // reports the server-derived one. Without a new-session tab the session
    // opens as an ordinary tab, so the action stays safe from any call site.
    promoteNewSession(projectId: number, sessionId: string) {
      const index = this.tabs.findIndex((tab) => tab.kind === 'new-session')
      if (index === -1) {
        this.tabs.push({ kind: 'session', projectId, sessionId, title: '' })
      } else {
        const { pinned } = this.tabs[index]
        const carried = pinned === true ? PINNED : {}
        this.tabs.splice(index, 1, { kind: 'session', projectId, sessionId, title: '', ...carried })
      }
      this.setActiveKey(sessionId)
      persist(this.tabs)
    },

    // updateTaskTitle renames a task tab without touching the other tabs.
    updateTaskTitle(taskId: number, title: string) {
      const existing = this.tabs.find(
        (tab): tab is Extract<OpenTab, { kind: 'task' }> => tab.kind === 'task' && tab.taskId === taskId,
      )
      if (existing && existing.title !== title) {
        existing.title = title
        persist(this.tabs)
      }
    },

    updateTitle(sessionId: string, title: string) {
      const existing = this.tabs.find(
        (tab): tab is Extract<OpenTab, { kind: 'session' }> =>
          tab.kind === 'session' && tab.sessionId === sessionId,
      )
      if (existing && existing.title !== title) {
        existing.title = title
        persist(this.tabs)
      }
    },

    // closeTab closes a session tab by session id — the shape callers that know
    // they are finished with a session already use.
    closeTab(sessionId: string) {
      this.closeTabByKey(sessionId)
    },

    // closeTabByKey closes either kind of tab. It forces the close: cleanup
    // paths (a deleted project, a deleted session) must be able to drop a pinned
    // tab, so the pinned guard lives in the strip, not here.
    closeTabByKey(key: string) {
      this.tabs = this.tabs.filter((tab) => tabKey(tab) !== key)
      if (this.lastActiveKey === key) {
        this.setActiveKey(this.tabs.length > 0 ? tabKey(this.tabs[this.tabs.length - 1]) : null)
      }
      persist(this.tabs)
    },

    // pinTab moves the tab to the front of the strip and marks it pinned.
    pinTab(key: string) {
      const tab = this.tabs.find((entry) => tabKey(entry) === key)
      if (!tab || tab.pinned === true) return
      tab.pinned = true
      this.tabs = pinnedFirst(this.tabs)
      persist(this.tabs)
    },

    // unpinTab returns the tab to the front of the unpinned group; the property
    // is removed so an unpinned tab carries no leftover flag.
    unpinTab(key: string) {
      const tab = this.tabs.find((entry) => tabKey(entry) === key)
      if (!tab || tab.pinned !== true) return
      delete tab.pinned
      this.tabs = pinnedFirst(this.tabs)
      persist(this.tabs)
    },

    togglePin(key: string) {
      const tab = this.tabs.find((entry) => tabKey(entry) === key)
      if (!tab) return
      if (tab.pinned === true) this.unpinTab(key)
      else this.pinTab(key)
    },

    // closeOtherTabsByKey closes every unpinned tab except the one named; pinned
    // tabs are kept. Returns the tabs it closed, so callers can release their
    // streams.
    closeOtherTabsByKey(key: string): OpenTab[] {
      const closing = this.tabs.filter((tab) => tabKey(tab) !== key && tab.pinned !== true)
      this.tabs = this.tabs.filter((tab) => tabKey(tab) === key || tab.pinned === true)
      if (this.lastActiveKey && !this.tabs.some((t) => tabKey(t) === this.lastActiveKey)) {
        this.setActiveKey(this.tabs.length > 0 ? tabKey(this.tabs[0]) : null)
      }
      persist(this.tabs)
      return closing
    },

    // closeAllTabs closes every unpinned tab; pinned tabs stay. Returns what it
    // closed so the caller can release their streams.
    closeAllTabs(): OpenTab[] {
      const closing = this.tabs.filter((tab) => tab.pinned !== true)
      this.tabs = this.tabs.filter((tab) => tab.pinned === true)
      if (this.lastActiveKey && !this.tabs.some((t) => tabKey(t) === this.lastActiveKey)) {
        this.setActiveKey(this.tabs.length > 0 ? tabKey(this.tabs[0]) : null)
      }
      persist(this.tabs)
      return closing
    },
  },
})
