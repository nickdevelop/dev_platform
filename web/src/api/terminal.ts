// API client for Terminal and Server Management.
import { apiRequest } from './client'

export interface TerminalStatus {
  supported: boolean
  os: string
  message?: string
}

export interface TerminalServer {
  id: number
  name: string
  host: string
  port: number
  username: string
  auth_type: 'local' | 'password' | 'key'
  is_local: boolean
  has_secret: boolean
  created_at?: string
  updated_at?: string
}

export interface CreateServerPayload {
  name: string
  host: string
  port: number
  username: string
  auth_type: 'password' | 'key'
  secret: string
}

export interface UpdateServerPayload {
  name: string
  host: string
  port: number
  username: string
  auth_type: 'password' | 'key'
  secret?: string
}

export interface TestConnectionPayload {
  host: string
  port: number
  username: string
  auth_type: 'password' | 'key'
  secret?: string
  server_id?: number
}

export async function getTerminalStatus(): Promise<TerminalStatus> {
  return apiRequest<TerminalStatus>('GET', '/terminal/status')
}

export async function listTerminalServers(): Promise<TerminalServer[]> {
  return apiRequest<TerminalServer[]>('GET', '/terminal/servers')
}

export async function getTerminalServer(id: number): Promise<TerminalServer> {
  return apiRequest<TerminalServer>('GET', `/terminal/servers/${id}`)
}

export async function createTerminalServer(
  csrfToken: string,
  payload: CreateServerPayload,
): Promise<TerminalServer> {
  return apiRequest<TerminalServer>('POST', '/terminal/servers', {
    body: payload,
    csrfToken,
  })
}

export async function updateTerminalServer(
  csrfToken: string,
  id: number,
  payload: UpdateServerPayload,
): Promise<TerminalServer> {
  return apiRequest<TerminalServer>('PUT', `/terminal/servers/${id}`, {
    body: payload,
    csrfToken,
  })
}

export async function deleteTerminalServer(csrfToken: string, id: number): Promise<void> {
  await apiRequest<{ ok: boolean }>('DELETE', `/terminal/servers/${id}`, { csrfToken })
}

export async function testServerConnection(
  csrfToken: string,
  payload: TestConnectionPayload,
): Promise<{ ok: boolean; message?: string }> {
  return apiRequest<{ ok: boolean; message?: string }>('POST', '/terminal/servers/test', {
    body: payload,
    csrfToken,
  })
}

export function buildTerminalWSUrl(serverId: number, cols = 80, rows = 24): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  return `${protocol}//${host}/api/terminal/ws/${serverId}?cols=${cols}&rows=${rows}`
}
