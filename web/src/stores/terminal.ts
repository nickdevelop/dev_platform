import { defineStore } from 'pinia'
import {
  type TerminalServer,
  type TerminalStatus,
  type CreateServerPayload,
  type UpdateServerPayload,
  type DiscoveredShell,
  type LocalShellDTO,
  type UpdateLocalShellPayload,
  getTerminalStatus,
  listTerminalServers,
  createTerminalServer,
  updateTerminalServer,
  deleteTerminalServer,
  scanLocalShells,
  getLocalShell,
  updateLocalShell,
  resetLocalShell,
  validateLocalShell,
  getTerminalConfig,
  updateTerminalConfig,
  resetTerminalConfig,
} from '../api/terminal'

export const TERMINAL_FONT_CACHE_KEY = 'platform.terminal.font_family'

function getCachedFontFamily(): string {
  try {
    return localStorage.getItem(TERMINAL_FONT_CACHE_KEY) || ''
  } catch {
    return ''
  }
}

function setCachedFontFamily(font: string): void {
  try {
    if (font) {
      localStorage.setItem(TERMINAL_FONT_CACHE_KEY, font)
    } else {
      localStorage.removeItem(TERMINAL_FONT_CACHE_KEY)
    }
  } catch {}
}

export interface TerminalTab {
  id: string
  serverId: number
  serverName: string
  title: string
  status: 'connecting' | 'connected' | 'disconnected' | 'error'
  errorMessage?: string
  cwd?: string
}

let tabCounter = 1

export const useTerminalStore = defineStore('terminal', {
  state: () => ({
    status: null as TerminalStatus | null,
    servers: [] as TerminalServer[],
    activeServerId: null as number | null,
    tabs: [] as TerminalTab[],
    activeTabId: null as string | null,
    localShell: null as LocalShellDTO | null,
    discoveredShells: [] as DiscoveredShell[],
    scanningShells: false,
    fontFamily: getCachedFontFamily(),
    fontLoading: false,
    fontSaving: false,
    fontError: null as string | null,
    loading: false,
    error: null as string | null,
  }),

  getters: {
    isSupported: (state) => state.status?.supported ?? true,
    activeTab: (state): TerminalTab | null => {
      if (!state.activeTabId) return null
      return state.tabs.find((t) => t.id === state.activeTabId) ?? null
    },
    activeServer: (state): TerminalServer | null => {
      if (state.activeServerId === null) return null
      return state.servers.find((s) => s.id === state.activeServerId) ?? null
    },
    effectiveFontFamily: (state): string => {
      const custom = state.fontFamily?.trim()
      if (custom) {
        return `"${custom.replace(/"/g, '\\"')}", var(--font-mono)`
      }
      return 'var(--font-mono)'
    },
  },

  actions: {
    async loadStatus() {
      try {
        this.status = await getTerminalStatus()
      } catch (err: unknown) {
        this.status = {
          supported: false,
          os: 'unknown',
          message: err instanceof Error ? err.message : '无法获取终端支持状态',
        }
      }
    },

    async loadServers() {
      this.loading = true
      this.error = null
      try {
        const servers = await listTerminalServers()
        this.servers = servers
        // If activeServerId is not set, select the first server (usually localhost)
        if (this.activeServerId === null && servers.length > 0) {
          this.activeServerId = servers[0].id
        }
      } catch (err: unknown) {
        this.error = err instanceof Error ? err.message : '获取服务器列表失败'
      } finally {
        this.loading = false
      }
    },

    selectServer(id: number | null) {
      this.activeServerId = id
    },

    openTerminal(serverId: number, options?: { cwd?: string; title?: string }): string {
      const server = this.servers.find((s) => s.id === serverId)
      const serverName = server?.name ?? (serverId === 0 ? '本机 (Localhost)' : `服务器 ${serverId}`)

      // Count existing tabs for this server to produce a friendly suffix
      const existingCount = this.tabs.filter((t) => t.serverId === serverId).length
      const tabTitle = options?.title ?? `${serverName} #${existingCount + 1}`

      const tabId = `term-${Date.now()}-${tabCounter++}`
      const newTab: TerminalTab = {
        id: tabId,
        serverId,
        serverName,
        title: tabTitle,
        status: 'connecting',
        cwd: options?.cwd,
      }

      this.tabs.push(newTab)
      this.activeTabId = tabId
      return tabId
    },

    closeTab(tabId: string) {
      const index = this.tabs.findIndex((t) => t.id === tabId)
      if (index === -1) return

      const wasActive = this.activeTabId === tabId
      this.tabs.splice(index, 1)

      if (wasActive) {
        if (this.tabs.length > 0) {
          // Switch to neighbor tab
          const nextIndex = Math.min(index, this.tabs.length - 1)
          this.activeTabId = this.tabs[nextIndex].id
        } else {
          this.activeTabId = null
        }
      }
    },

    setActiveTab(tabId: string) {
      if (this.tabs.some((t) => t.id === tabId)) {
        this.activeTabId = tabId
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

    async createServer(payload: CreateServerPayload): Promise<TerminalServer> {
      const created = await createTerminalServer(payload)
      this.servers.push(created)
      return created
    },

    async updateServer(id: number, payload: UpdateServerPayload): Promise<TerminalServer> {
      const updated = await updateTerminalServer(id, payload)
      const idx = this.servers.findIndex((s) => s.id === id)
      if (idx !== -1) {
        this.servers[idx] = updated
      }
      return updated
    },

    async deleteServer(id: number): Promise<void> {
      await deleteTerminalServer(id)
      this.servers = this.servers.filter((s) => s.id !== id)
      if (this.activeServerId === id) {
        this.activeServerId = this.servers.length > 0 ? this.servers[0].id : null
      }
    },

    async loadLocalShell() {
      try {
        this.localShell = await getLocalShell()
      } catch (err: unknown) {
        // Non-fatal if local shell fails to load
        console.warn('Failed to load local shell:', err)
      }
    },

    async scanShells(): Promise<DiscoveredShell[]> {
      this.scanningShells = true
      try {
        const shells = await scanLocalShells()
        this.discoveredShells = shells
        return shells
      } catch (err: unknown) {
        this.error = err instanceof Error ? err.message : '扫描本地 Shell 失败'
        return []
      } finally {
        this.scanningShells = false
      }
    },

    async saveLocalShell(payload: UpdateLocalShellPayload): Promise<LocalShellDTO> {
      const updated = await updateLocalShell(payload)
      this.localShell = updated
      return updated
    },

    async resetLocalShell(): Promise<LocalShellDTO> {
      const reset = await resetLocalShell()
      this.localShell = reset
      return reset
    },

    async validateLocalShell(path: string): Promise<{ ok: boolean; message?: string }> {
      return validateLocalShell(path)
    },

    async loadTerminalConfig() {
      this.fontLoading = true
      this.fontError = null
      try {
        const cfg = await getTerminalConfig()
        this.fontFamily = cfg.font_family || ''
        setCachedFontFamily(this.fontFamily)
      } catch (err: unknown) {
        // Non-fatal, fallback to cached / default
        console.warn('Failed to load terminal config:', err)
      } finally {
        this.fontLoading = false
      }
    },

    async saveTerminalFontFamily(font: string): Promise<string> {
      this.fontSaving = true
      this.fontError = null
      try {
        const clean = font.trim()
        const cfg = await updateTerminalConfig({ font_family: clean })
        this.fontFamily = cfg.font_family || ''
        setCachedFontFamily(this.fontFamily)
        return this.fontFamily
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : '保存终端字体设置失败'
        this.fontError = msg
        throw err
      } finally {
        this.fontSaving = false
      }
    },

    async resetTerminalFontFamily(): Promise<void> {
      this.fontSaving = true
      this.fontError = null
      try {
        await resetTerminalConfig()
        this.fontFamily = ''
        setCachedFontFamily('')
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : '重置终端字体设置失败'
        this.fontError = msg
        throw err
      } finally {
        this.fontSaving = false
      }
    },
  },
})
