import { defineStore } from 'pinia'

export interface TerminalTab {
  id: string
  serverId: number
  serverName: string
  title: string
  cwd?: string
  status: 'connecting' | 'connected' | 'disconnected' | 'error'
  errorMessage?: string
  windowId: string // 'main' or detached window id (e.g. 'win-xxx')
}

export interface ProjectTerminalWindow {
  id: string // 'main' for default tabbed window, or 'win-...' for detached window
  isMain: boolean
  projectName: string
  cwd: string
  tabId: string // active tab id for backward compatibility
  activeTabId: string
  isMinimized: boolean
  left: number
  top: number
  zIndex: number
  width?: number
  height?: number
}

// Type alias for backward compatibility
export type ProjectTerminalInstance = ProjectTerminalWindow

let counter = 1
const BASE_Z_INDEX = 60

export const useProjectTerminalStore = defineStore('projectTerminal', {
  state: () => ({
    windows: [] as ProjectTerminalWindow[],
    tabs: [] as TerminalTab[],
    maxZIndex: BASE_Z_INDEX,
  }),

  getters: {
    // Backward compatibility: alias instances to windows
    instances: (state): ProjectTerminalWindow[] => state.windows,
    hasActiveTerminal: (state): boolean => state.tabs.length > 0,
    isOpen: (state): boolean => state.windows.length > 0 && state.windows.some((w) => !w.isMinimized),
    isMinimized: (state): boolean => state.windows.length > 0 && state.windows.every((w) => w.isMinimized),
    minimizedInstances: (state): ProjectTerminalWindow[] => state.windows.filter((w) => w.isMinimized),
    mainWindow: (state): ProjectTerminalWindow | undefined => state.windows.find((w) => w.isMain || w.id === 'main'),
    mainTabs: (state): TerminalTab[] => state.tabs.filter((t) => t.windowId === 'main'),
    // Backward compatibility for single-instance tests
    cwd: (state): string => state.tabs[0]?.cwd ?? '',
    projectName: (state): string => state.windows[0]?.projectName ?? '',
    tabId: (state): string => state.windows[0]?.activeTabId ?? '',
  },

  actions: {
    ensureMainWindow(projectName = '项目', cwd = ''): ProjectTerminalWindow {
      let mainWin = this.windows.find((w) => w.isMain || w.id === 'main')
      if (mainWin) {
        if (projectName && projectName !== '项目') mainWin.projectName = projectName
        if (cwd) mainWin.cwd = cwd
        return mainWin
      }

      let baseLeft = 60
      let baseTop = 60
      if (typeof window !== 'undefined') {
        baseLeft = Math.max(16, Math.round((window.innerWidth - 800) / 2))
        baseTop = Math.max(16, Math.round((window.innerHeight - 500) / 2))
      }

      this.maxZIndex++
      mainWin = {
        id: 'main',
        isMain: true,
        projectName: projectName || '终端',
        cwd: cwd || '',
        tabId: '',
        activeTabId: '',
        isMinimized: false,
        left: baseLeft,
        top: baseTop,
        zIndex: this.maxZIndex,
      }
      this.windows.unshift(mainWin)
      return mainWin
    },

    openLocalTab(cwd?: string, projectName?: string, targetWindowId = 'main'): string {
      const name = projectName || '项目'
      const win = targetWindowId === 'main'
        ? this.ensureMainWindow(name, cwd || '')
        : this.windows.find((w) => w.id === targetWindowId) || this.ensureMainWindow(name, cwd || '')

      win.isMinimized = false
      win.projectName = name
      if (cwd) win.cwd = cwd

      // Count existing local tabs to format friendly title
      const existingCount = this.tabs.filter((t) => t.serverId === 0 && t.windowId === win.id).length
      const tabTitle = existingCount > 0 ? `${name} #${existingCount + 1}` : name

      const tabId = `term-${Date.now()}-${counter++}`
      const newTab: TerminalTab = {
        id: tabId,
        serverId: 0,
        serverName: '本机 (Localhost)',
        title: tabTitle,
        cwd: cwd || '',
        status: 'connecting',
        windowId: win.id,
      }

      this.tabs.push(newTab)
      win.activeTabId = tabId
      win.tabId = tabId
      this.bringToFront(win.id)
      return tabId
    },

    openServerTab(serverId: number, serverName?: string, targetWindowId = 'main'): string {
      const name = serverName || (serverId === 0 ? '本机 (Localhost)' : `服务器 ${serverId}`)
      const win = targetWindowId === 'main'
        ? this.ensureMainWindow(name)
        : this.windows.find((w) => w.id === targetWindowId) || this.ensureMainWindow(name)

      win.isMinimized = false

      const existingCount = this.tabs.filter((t) => t.serverId === serverId && t.windowId === win.id).length
      const tabTitle = existingCount > 0 ? `${name} #${existingCount + 1}` : name

      const tabId = `term-${Date.now()}-${counter++}`
      const newTab: TerminalTab = {
        id: tabId,
        serverId,
        serverName: name,
        title: tabTitle,
        status: 'connecting',
        windowId: win.id,
      }

      this.tabs.push(newTab)
      win.activeTabId = tabId
      win.tabId = tabId
      this.bringToFront(win.id)
      return tabId
    },

    // Backward compatibility for existing callers
    openTerminal(cwd: string, projectName?: string): string {
      return this.openLocalTab(cwd, projectName, 'main')
    },

    detachTab(tabId: string): string | null {
      const tab = this.tabs.find((t) => t.id === tabId)
      if (!tab) return null

      // If already in a detached window alone, return its window
      const currentWin = this.windows.find((w) => w.id === tab.windowId)
      if (currentWin && !currentWin.isMain) {
        const tabsInCurrent = this.tabs.filter((t) => t.windowId === currentWin.id)
        if (tabsInCurrent.length === 1) return currentWin.id
      }

      const winId = `win-${Date.now()}-${counter++}`
      const cascadeOffset = (this.windows.length % 6) * 24

      let baseLeft = 100
      let baseTop = 100
      if (currentWin) {
        baseLeft = Math.min(window?.innerWidth ? window.innerWidth - 820 : 600, currentWin.left + 32)
        baseTop = Math.min(window?.innerHeight ? window.innerHeight - 520 : 400, currentWin.top + 32)
      }

      this.maxZIndex++
      const detachedWindow: ProjectTerminalWindow = {
        id: winId,
        isMain: false,
        projectName: tab.title || tab.serverName,
        cwd: tab.cwd || '',
        tabId: tab.id,
        activeTabId: tab.id,
        isMinimized: false,
        left: Math.max(16, baseLeft),
        top: Math.max(16, baseTop),
        zIndex: this.maxZIndex,
      }

      tab.windowId = winId
      this.windows.push(detachedWindow)

      // If previous window has remaining tabs, switch activeTabId
      if (currentWin) {
        const remaining = this.tabs.filter((t) => t.windowId === currentWin.id)
        if (remaining.length > 0) {
          if (currentWin.activeTabId === tabId) {
            currentWin.activeTabId = remaining[remaining.length - 1].id
            currentWin.tabId = currentWin.activeTabId
          }
        } else {
          // No remaining tabs in previous window -> close it
          this.windows = this.windows.filter((w) => w.id !== currentWin.id)
        }
      }

      this.bringToFront(winId)
      return winId
    },

    attachTab(tabId: string): void {
      const tab = this.tabs.find((t) => t.id === tabId)
      if (!tab) return

      const oldWinId = tab.windowId
      if (oldWinId === 'main') return

      const mainWin = this.ensureMainWindow()
      tab.windowId = 'main'
      mainWin.activeTabId = tab.id
      mainWin.tabId = tab.id
      mainWin.isMinimized = false

      // Check if old window has remaining tabs
      const remainingOld = this.tabs.filter((t) => t.windowId === oldWinId)
      if (remainingOld.length === 0) {
        this.windows = this.windows.filter((w) => w.id !== oldWinId)
      } else {
        const oldWin = this.windows.find((w) => w.id === oldWinId)
        if (oldWin && oldWin.activeTabId === tabId) {
          oldWin.activeTabId = remainingOld[remainingOld.length - 1].id
          oldWin.tabId = oldWin.activeTabId
        }
      }

      this.bringToFront('main')
    },

    closeTab(tabId: string): void {
      const index = this.tabs.findIndex((t) => t.id === tabId)
      if (index === -1) return

      const tab = this.tabs[index]
      const winId = tab.windowId
      this.tabs.splice(index, 1)

      const win = this.windows.find((w) => w.id === winId)
      if (!win) return

      const remaining = this.tabs.filter((t) => t.windowId === winId)
      if (remaining.length === 0) {
        // No more tabs -> remove window
        this.windows = this.windows.filter((w) => w.id !== winId)
      } else if (win.activeTabId === tabId) {
        // Switch to adjacent tab
        const nextActive = remaining[Math.min(index, remaining.length - 1)]
        win.activeTabId = nextActive.id
        win.tabId = nextActive.id
      }
    },

    setActiveTab(windowId: string, tabId: string): void {
      const win = this.windows.find((w) => w.id === windowId)
      if (win && this.tabs.some((t) => t.id === tabId && t.windowId === windowId)) {
        win.activeTabId = tabId
        win.tabId = tabId
      }
    },

    toggleShortcut(): void {
      this.toggleAltT()
    },

    toggleAltT(): void {
      if (this.windows.length === 0 || this.tabs.length === 0) {
        this.openLocalTab()
        return
      }

      const visible = this.windows.filter((w) => !w.isMinimized)
      if (visible.length > 0) {
        // If any window is visible, minimize all visible windows
        visible.forEach((w) => {
          w.isMinimized = true
        })
      } else {
        // If all are minimized, restore all windows (or main window)
        this.windows.forEach((w) => {
          w.isMinimized = false
        })
        const main = this.mainWindow || this.windows[0]
        if (main) this.bringToFront(main.id)
      }
    },

    bringToFront(id: string) {
      const win = this.windows.find((w) => w.id === id)
      if (win) {
        this.maxZIndex++
        win.zIndex = this.maxZIndex
      }
    },

    minimize(id?: string) {
      if (id) {
        const win = this.windows.find((w) => w.id === id)
        if (win) win.isMinimized = true
      } else if (this.windows.length > 0) {
        this.windows[this.windows.length - 1].isMinimized = true
      }
    },

    restore(id?: string) {
      if (id) {
        const win = this.windows.find((w) => w.id === id)
        if (win) {
          win.isMinimized = false
          this.bringToFront(id)
        }
      } else {
        const minimized = this.windows.find((w) => w.isMinimized)
        if (minimized) {
          minimized.isMinimized = false
          this.bringToFront(minimized.id)
        }
      }
    },

    updatePosition(id: string, left: number, top: number) {
      const win = this.windows.find((w) => w.id === id)
      if (win) {
        win.left = left
        win.top = top
      }
    },

    closeTerminal(id?: string) {
      if (id) {
        this.tabs = this.tabs.filter((t) => t.windowId !== id)
        this.windows = this.windows.filter((w) => w.id !== id)
      } else if (this.windows.length > 0) {
        const last = this.windows[this.windows.length - 1]
        this.tabs = this.tabs.filter((t) => t.windowId !== last.id)
        this.windows.pop()
      }
    },

    updateTabStatus(tabId: string, status: TerminalTab['status'], errorMessage?: string) {
      const tab = this.tabs.find((t) => t.id === tabId)
      if (tab) {
        tab.status = status
        tab.errorMessage = errorMessage
      }
    },

    updateTabTitle(tabId: string, title: string) {
      const tab = this.tabs.find((t) => t.id === tabId)
      if (tab) {
        tab.title = title
      }
    },
  },
})
