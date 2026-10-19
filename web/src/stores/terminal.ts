import { defineStore } from 'pinia'
import {
  type TerminalServer,
  type TerminalStatus,
  type CreateServerPayload,
  type UpdateServerPayload,
  getTerminalStatus,
  listTerminalServers,
  createTerminalServer,
  updateTerminalServer,
  deleteTerminalServer,
} from '../api/terminal'

export interface TerminalTab {
  id: string
  serverId: number
  serverName: string
  title: string
  status: 'connecting' | 'connected' | 'disconnected' | 'error'
  errorMessage?: string
}

let tabCounter = 1

export const useTerminalStore = defineStore('terminal', {
  state: () => ({
    status: null as TerminalStatus | null,
    servers: [] as TerminalServer[],
    activeServerId: null as number | null,
    tabs: [] as TerminalTab[],
    activeTabId: null as string | null,
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

    openTerminal(serverId: number): string {
      const server = this.servers.find((s) => s.id === serverId)
      const serverName = server?.name ?? (serverId === 0 ? '本机 (Localhost)' : `服务器 ${serverId}`)

      // Count existing tabs for this server to produce a friendly suffix
      const existingCount = this.tabs.filter((t) => t.serverId === serverId).length
      const tabTitle = `${serverName} #${existingCount + 1}`

      const tabId = `term-${Date.now()}-${tabCounter++}`
      const newTab: TerminalTab = {
        id: tabId,
        serverId,
        serverName,
        title: tabTitle,
        status: 'connecting',
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

    async createServer(csrfToken: string, payload: CreateServerPayload): Promise<TerminalServer> {
      const created = await createTerminalServer(csrfToken, payload)
      this.servers.push(created)
      return created
    },

    async updateServer(
      csrfToken: string,
      id: number,
      payload: UpdateServerPayload,
    ): Promise<TerminalServer> {
      const updated = await updateTerminalServer(csrfToken, id, payload)
      const idx = this.servers.findIndex((s) => s.id === id)
      if (idx !== -1) {
        this.servers[idx] = updated
      }
      return updated
    },

    async deleteServer(csrfToken: string, id: number): Promise<void> {
      await deleteTerminalServer(csrfToken, id)
      this.servers = this.servers.filter((s) => s.id !== id)
      if (this.activeServerId === id) {
        this.activeServerId = this.servers.length > 0 ? this.servers[0].id : null
      }
    },
  },
})
