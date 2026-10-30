export type ConnectionType = 'local' | 'remote'

export interface ServerConnection {
  id: string
  name: string
  type: ConnectionType
  url?: string // Required when type === 'remote', e.g. http://192.168.1.100:8080
  token?: string // Optional auth password/token
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
