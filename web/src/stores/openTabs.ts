// Open task tabs: the set of tasks the user has opened in this browser.
// Tabs let several tasks stay open at once; switching tabs is navigation,
// while each tab keeps its cached transcript in stores/sessions.ts and its
// live SSE subscription. Tabs persist in localStorage so a refresh restores
// the working set.
import { defineStore } from 'pinia'

export interface OpenTab {
  projectId: number
  sessionId: string
  title: string
}

const STORAGE_KEY = 'dev-platform-open-tabs'

function loadPersisted(): OpenTab[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return []
    return parsed
      .filter(
        (entry): entry is OpenTab =>
          typeof entry === 'object' &&
          entry !== null &&
          typeof (entry as OpenTab).sessionId === 'string' &&
          typeof (entry as OpenTab).projectId === 'number',
      )
      .map((entry) => ({ projectId: entry.projectId, sessionId: entry.sessionId, title: entry.title ?? '' }))
      .slice(0, 20)
  } catch {
    return []
  }
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
    openTab(projectId: number, sessionId: string, title = '') {
      const existing = this.tabs.find((tab) => tab.sessionId === sessionId)
      if (existing) {
        existing.projectId = projectId
        if (title !== '') existing.title = title
      } else {
        this.tabs.push({ projectId, sessionId, title })
        if (this.tabs.length > 20) this.tabs.shift()
      }
      persist(this.tabs)
    },

    updateTitle(sessionId: string, title: string) {
      const existing = this.tabs.find((tab) => tab.sessionId === sessionId)
      if (existing && existing.title !== title) {
        existing.title = title
        persist(this.tabs)
      }
    },

    closeTab(sessionId: string) {
      this.tabs = this.tabs.filter((tab) => tab.sessionId !== sessionId)
      persist(this.tabs)
    },

    closeOtherTabs(sessionId: string) {
      this.tabs = this.tabs.filter((tab) => tab.sessionId === sessionId)
      persist(this.tabs)
    },

    closeAllTabs() {
      this.tabs = []
      persist(this.tabs)
    },
  },
})
