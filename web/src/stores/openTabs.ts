// Open workbench tabs: what this browser has open right now. Two kinds share
// the strip — a session (a session with its cached transcript and live SSE
// subscription) and a project's settings page. Switching a tab is navigation;
// the tab strip renders both kinds from one `tabKey`, so nothing downstream has
// to know which is which. Tabs persist in localStorage so a refresh restores the
// working set.
import { defineStore } from 'pinia'

export type OpenTab =
  | { kind: 'session'; projectId: number; sessionId: string; title: string }
  | { kind: 'project-settings'; projectId: number }

// tabKey identifies a tab. A session is identified by its session (machine-unique),
// a settings page by its project; the prefixes keep the two namespaces apart.
export function tabKey(tab: OpenTab): string {
  return tab.kind === 'session' ? tab.sessionId : `settings:${tab.projectId}`
}

const STORAGE_KEY = 'dev-platform-open-tabs-v2'
// The pre-tab-model key: it stored session tabs without a kind. Read once, then
// never written again, so a user who installed this version keeps their tabs and
// never sees the old shape again.
const LEGACY_STORAGE_KEY = 'dev-platform-open-tabs'

const MAX_TABS = 20

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
  if (typeof tab !== 'object' || tab === null || typeof tab.projectId !== 'number') return null
  if (tab.kind === 'project-settings') return { kind: 'project-settings', projectId: tab.projectId }
  if (tab.kind === 'session' || tab.kind === 'task' || tab.kind === undefined) {
    if (!isSession(entry)) return null
    // A stored session tab may predate any title being set; only a string title is
    // carried over, so a hand-edited store cannot inject one.
    const title = typeof tab.title === 'string' ? tab.title : ''
    return { kind: 'session', projectId: entry.projectId, sessionId: entry.sessionId, title }
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
  const current = read(STORAGE_KEY)
    .map(normalize)
    .filter((tab): tab is OpenTab => tab !== null)
  if (current.length > 0) return current.slice(0, MAX_TABS)

  const migrated = read(LEGACY_STORAGE_KEY)
    .map(normalize)
    .filter((tab): tab is OpenTab => tab !== null)
    .slice(0, MAX_TABS)
  // Writing the migrated set down at once means the old shape is read exactly
  // once, and a later visit never has to know it existed.
  if (migrated.length > 0) persist(migrated)
  return migrated
}

function persist(tabs: OpenTab[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(tabs))
  } catch {
    // Persistence is best-effort only.
  }
}

export const useOpenTabsStore = defineStore('openTabs', {
  state: () => ({
    tabs: loadPersisted() as OpenTab[],
  }),

  actions: {
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
        if (this.tabs.length > MAX_TABS) this.tabs.shift()
      }
      persist(this.tabs)
    },

    // Opens (or brings to front) a project's settings tab — one per project.
    openProjectSettings(projectId: number) {
      const existing = this.tabs.find(
        (tab) => tab.kind === 'project-settings' && tab.projectId === projectId,
      )
      if (existing) {
        this.tabs = [...this.tabs.filter((tab) => tab !== existing), existing]
      } else {
        this.tabs.push({ kind: 'project-settings', projectId })
        if (this.tabs.length > MAX_TABS) this.tabs.shift()
      }
      persist(this.tabs)
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
      this.tabs = this.tabs.filter((tab) => !(tab.kind === 'session' && tab.sessionId === sessionId))
      persist(this.tabs)
    },

    // closeTabByKey closes either kind of tab, which is what a tab strip needs.
    closeTabByKey(key: string) {
      this.tabs = this.tabs.filter((tab) => tabKey(tab) !== key)
      persist(this.tabs)
    },

    closeOtherTabs(sessionId: string) {
      this.tabs = this.tabs.filter(
        (tab) => tab.kind === 'project-settings' || tab.sessionId === sessionId,
      )
      persist(this.tabs)
    },

    closeAllTabs() {
      this.tabs = []
      persist(this.tabs)
    },
  },
})
