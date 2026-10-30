import * as fs from 'node:fs'
import * as path from 'node:path'
import { app, safeStorage } from 'electron'
import { ServerConnection } from '../shared/types'

interface StoreData {
  activeId: string
  connections: ServerConnection[]
}

const DEFAULT_LOCAL_CONNECTION: ServerConnection = {
  id: 'local',
  name: '本地内置服务',
  type: 'local',
  createdAt: Date.now(),
}

export class ConnectionStore {
  private filePath: string
  private data: StoreData

  constructor(customUserDataPath?: string) {
    const userData = customUserDataPath || (app?.getPath ? app.getPath('userData') : path.join(process.cwd(), '.desktop-data'))
    this.filePath = path.join(userData, 'connections.json')
    this.data = this.load()
  }

  private load(): StoreData {
    try {
      if (fs.existsSync(this.filePath)) {
        const raw = fs.readFileSync(this.filePath, 'utf-8')
        const parsed = JSON.parse(raw) as Partial<StoreData>
        let connections = parsed.connections || []
        // Ensure default local connection exists
        if (!connections.some((c) => c.id === 'local')) {
          connections = [DEFAULT_LOCAL_CONNECTION, ...connections]
        }
        const activeId = parsed.activeId && connections.some((c) => c.id === parsed.activeId)
          ? parsed.activeId
          : 'local'
        return { activeId, connections }
      }
    } catch (e) {
      console.error('[ConnectionStore] Failed to load connections, using defaults:', e)
    }

    return {
      activeId: 'local',
      connections: [DEFAULT_LOCAL_CONNECTION],
    }
  }

  private save(): void {
    try {
      const dir = path.dirname(this.filePath)
      if (!fs.existsSync(dir)) {
        fs.mkdirSync(dir, { recursive: true })
      }
      fs.writeFileSync(this.filePath, JSON.stringify(this.data, null, 2), 'utf-8')
    } catch (e) {
      console.error('[ConnectionStore] Failed to save connections:', e)
    }
  }

  private encryptToken(token: string): string {
    if (!token) return ''
    try {
      if (safeStorage && safeStorage.isEncryptionAvailable()) {
        return 'enc:' + safeStorage.encryptString(token).toString('base64')
      }
    } catch (e) {
      console.warn('[ConnectionStore] safeStorage failed, fallback to base64:', e)
    }
    return 'b64:' + Buffer.from(token).toString('base64')
  }

  private decryptToken(encrypted: string): string {
    if (!encrypted) return ''
    try {
      if (encrypted.startsWith('enc:') && safeStorage && safeStorage.isEncryptionAvailable()) {
        const buf = Buffer.from(encrypted.slice(4), 'base64')
        return safeStorage.decryptString(buf)
      }
      if (encrypted.startsWith('b64:')) {
        return Buffer.from(encrypted.slice(4), 'base64').toString('utf-8')
      }
    } catch (e) {
      console.warn('[ConnectionStore] Token decrypt failed:', e)
    }
    return encrypted
  }

  public getConnections(): ServerConnection[] {
    // Return sanitized connections without exposing actual password
    return this.data.connections.map((c) => ({
      ...c,
      token: c.token ? '***' : undefined,
    }))
  }

  public getRawConnection(id: string): (ServerConnection & { resolvedToken?: string }) | null {
    const conn = this.data.connections.find((c) => c.id === id)
    if (!conn) return null
    return {
      ...conn,
      resolvedToken: conn.token ? this.decryptToken(conn.token) : undefined,
    }
  }

  public getActiveConnection(): ServerConnection & { resolvedToken?: string } {
    const raw = this.getRawConnection(this.data.activeId)
    if (raw) return raw
    return {
      ...DEFAULT_LOCAL_CONNECTION,
      resolvedToken: undefined,
    }
  }

  public setActiveId(id: string): boolean {
    if (this.data.connections.some((c) => c.id === id)) {
      this.data.activeId = id
      const conn = this.data.connections.find((c) => c.id === id)
      if (conn) {
        conn.lastConnectedAt = Date.now()
      }
      this.save()
      return true
    }
    return false
  }

  public saveConnection(input: Omit<ServerConnection, 'createdAt' | 'id'> & { id?: string }): ServerConnection {
    const isNew = !input.id || input.id.trim() === ''
    const id = isNew ? 'conn_' + Math.random().toString(36).substring(2, 10) : input.id!

    const existingIndex = this.data.connections.findIndex((c) => c.id === id)
    let tokenToStore = input.token

    if (existingIndex >= 0) {
      // If updating and token is '***' or unchanged, keep existing token
      if (input.token === '***' || input.token === undefined) {
        tokenToStore = this.data.connections[existingIndex].token
      } else if (input.token) {
        tokenToStore = this.encryptToken(input.token)
      }
    } else if (input.token) {
      tokenToStore = this.encryptToken(input.token)
    }

    const newConn: ServerConnection = {
      id,
      name: input.name.trim() || (input.type === 'local' ? '本地内置服务' : '远程服务器'),
      type: input.type,
      url: input.type === 'remote' ? (input.url?.trim() || '') : undefined,
      token: tokenToStore,
      createdAt: existingIndex >= 0 ? this.data.connections[existingIndex].createdAt : Date.now(),
      lastConnectedAt: existingIndex >= 0 ? this.data.connections[existingIndex].lastConnectedAt : undefined,
    }

    if (existingIndex >= 0) {
      this.data.connections[existingIndex] = newConn
    } else {
      this.data.connections.push(newConn)
    }

    this.save()
    return {
      ...newConn,
      token: newConn.token ? '***' : undefined,
    }
  }

  public removeConnection(id: string): boolean {
    if (id === 'local') return false // Local cannot be removed
    const lenBefore = this.data.connections.length
    this.data.connections = this.data.connections.filter((c) => c.id !== id)
    if (this.data.connections.length < lenBefore) {
      if (this.data.activeId === id) {
        this.data.activeId = 'local'
      }
      this.save()
      return true
    }
    return false
  }
}
