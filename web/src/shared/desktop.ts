import { ref, onMounted, onUnmounted, computed } from 'vue'

export type ConnectionType = 'local' | 'remote'

export interface ServerConnection {
  id: string
  name: string
  type: ConnectionType
  url?: string
  token?: string
  createdAt: number
  lastConnectedAt?: number
}

export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'disconnected' | 'error'

export interface LocalServiceStatus {
  running: boolean
  pid?: number
  port?: number
  error?: string
}

export interface DesktopStatus {
  activeConnectionId: string
  connectionState: ConnectionState
  statusMessage?: string
  localService: LocalServiceStatus
  proxyPort: number
}

export interface TestConnectionResult {
  ok: boolean
  status?: number
  message?: string
  latencyMs?: number
  version?: string
}

export interface DesktopIpcApi {
  getConnections: () => Promise<ServerConnection[]>
  saveConnection: (connection: Omit<ServerConnection, 'createdAt' | 'id'> & { id?: string }) => Promise<ServerConnection>
  removeConnection: (id: string) => Promise<boolean>
  switchConnection: (id: string) => Promise<DesktopStatus>
  getStatus: () => Promise<DesktopStatus>
  testConnection: (url: string, token?: string) => Promise<TestConnectionResult>
  openDirectoryDialog: (defaultPath?: string) => Promise<string | null>
  onStatusChanged: (callback: (status: DesktopStatus) => void) => () => void
}

declare global {
  interface Window {
    desktopAPI?: DesktopIpcApi
  }
}

export const isDesktopApp = typeof window !== 'undefined' && Boolean(window.desktopAPI)

const desktopStatus = ref<DesktopStatus | null>(null)
const desktopConnections = ref<ServerConnection[]>([])
const isInitialized = ref(false)
let removeListener: (() => void) | null = null

export function useDesktopConnection() {
  const isDesktop = computed(() => typeof window !== 'undefined' && Boolean(window.desktopAPI))

  const activeConnection = computed(() => {
    if (!desktopStatus.value) return null
    return desktopConnections.value.find((c) => c.id === desktopStatus.value?.activeConnectionId) || null
  })

  const isLocal = computed(() => activeConnection.value?.type === 'local')

  async function refresh() {
    if (!window.desktopAPI) return
    try {
      const [status, conns] = await Promise.all([
        window.desktopAPI.getStatus(),
        window.desktopAPI.getConnections(),
      ])
      desktopStatus.value = status
      desktopConnections.value = conns
    } catch (e) {
      console.error('[useDesktopConnection] refresh failed:', e)
    }
  }

  async function switchTarget(id: string) {
    if (!window.desktopAPI) return
    const newStatus = await window.desktopAPI.switchConnection(id)
    desktopStatus.value = newStatus
    // Refresh connections list to update lastConnectedAt
    const conns = await window.desktopAPI.getConnections()
    desktopConnections.value = conns
  }

  async function save(conn: Omit<ServerConnection, 'createdAt' | 'id'> & { id?: string }) {
    if (!window.desktopAPI) throw new Error('Not running in desktop app')
    const saved = await window.desktopAPI.saveConnection(conn)
    await refresh()
    return saved
  }

  async function remove(id: string) {
    if (!window.desktopAPI) return false
    const ok = await window.desktopAPI.removeConnection(id)
    await refresh()
    return ok
  }

  async function test(url: string, token?: string) {
    if (!window.desktopAPI) return { ok: false, message: 'Not desktop environment' }
    return window.desktopAPI.testConnection(url, token)
  }

  async function chooseDirectory(defaultPath?: string) {
    if (!window.desktopAPI) return null
    return window.desktopAPI.openDirectoryDialog(defaultPath)
  }

  if (typeof window !== 'undefined' && window.desktopAPI && !isInitialized.value) {
    isInitialized.value = true
    refresh()
    removeListener = window.desktopAPI.onStatusChanged((status) => {
      desktopStatus.value = status
    })
  }

  return {
    isDesktop,
    status: desktopStatus,
    connections: desktopConnections,
    activeConnection,
    isLocal,
    refresh,
    switchTarget,
    save,
    remove,
    test,
    chooseDirectory,
  }
}

export function resetDesktopStateForTesting() {
  desktopStatus.value = null
  desktopConnections.value = []
  isInitialized.value = false
  if (removeListener) {
    removeListener()
    removeListener = null
  }
}
