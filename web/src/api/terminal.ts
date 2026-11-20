// API client for Terminal and Server Management.
import { get, mutate } from './client'

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

export interface DiscoveredShell {
  id: string
  name: string
  path: string
  args: string[]
  isRecommended: boolean
}

export interface LocalShellDTO {
  path: string
  args: string[]
  name: string
  is_custom: boolean
  is_recommended: boolean
}

export interface UpdateLocalShellPayload {
  path: string
  args?: string[]
}

export interface TerminalConfig {
  font_family: string
}

export interface UpdateTerminalConfigPayload {
  font_family: string
}

export async function getTerminalStatus(): Promise<TerminalStatus> {
  return get<TerminalStatus>('/terminal/status')
}

export async function listTerminalServers(): Promise<TerminalServer[]> {
  return get<TerminalServer[]>('/terminal/servers')
}

export async function getTerminalServer(id: number): Promise<TerminalServer> {
  return get<TerminalServer>(`/terminal/servers/${id}`)
}

export async function createTerminalServer(
  payload: CreateServerPayload,
): Promise<TerminalServer> {
  return mutate<TerminalServer>('POST', '/terminal/servers', payload)
}

export async function updateTerminalServer(
  id: number,
  payload: UpdateServerPayload,
): Promise<TerminalServer> {
  return mutate<TerminalServer>('PUT', `/terminal/servers/${id}`, payload)
}

export async function deleteTerminalServer(id: number): Promise<void> {
  await mutate<{ ok: boolean }>('DELETE', `/terminal/servers/${id}`)
}

export async function testServerConnection(
  payload: TestConnectionPayload,
): Promise<{ ok: boolean; message?: string }> {
  return mutate<{ ok: boolean; message?: string }>('POST', '/terminal/servers/test', payload)
}

export async function scanLocalShells(): Promise<DiscoveredShell[]> {
  return get<DiscoveredShell[]>('/terminal/local/shells/scan')
}

export async function getLocalShell(): Promise<LocalShellDTO> {
  return get<LocalShellDTO>('/terminal/local/shell')
}

export async function updateLocalShell(
  payload: UpdateLocalShellPayload,
): Promise<LocalShellDTO> {
  return mutate<LocalShellDTO>('PUT', '/terminal/local/shell', payload)
}

export async function resetLocalShell(): Promise<LocalShellDTO> {
  return mutate<LocalShellDTO>('POST', '/terminal/local/shell/reset')
}

export async function validateLocalShell(
  path: string,
): Promise<{ ok: boolean; message?: string }> {
  return mutate<{ ok: boolean; message?: string }>('POST', '/terminal/local/shell/validate', { path })
}

export async function getTerminalConfig(): Promise<TerminalConfig> {
  return get<TerminalConfig>('/terminal/config')
}

export async function updateTerminalConfig(
  payload: UpdateTerminalConfigPayload,
): Promise<TerminalConfig> {
  return mutate<TerminalConfig>('PUT', '/terminal/config', payload)
}

export async function resetTerminalConfig(): Promise<TerminalConfig> {
  return mutate<TerminalConfig>('POST', '/terminal/config/reset')
}

export function buildTerminalWSUrl(serverId: number, cols = 80, rows = 24, cwd?: string): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  let url = `${protocol}//${host}/api/terminal/ws/${serverId}?cols=${cols}&rows=${rows}`
  if (cwd && cwd.trim()) {
    url += `&cwd=${encodeURIComponent(cwd.trim())}`
  }
  return url
}
