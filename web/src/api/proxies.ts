// API client for ContextPath Service Proxy management.
import { get, mutate } from './client'

export type ProxyType = 'directory' | 'port'

export interface ServiceProxy {
  id: number
  name: string
  alias: string
  type: ProxyType
  target: string
  enabled: boolean
  online?: boolean
  createdAt: string
  updatedAt: string
}

export interface CreateProxyPayload {
  name: string
  alias: string
  type: ProxyType
  target: string
  enabled?: boolean
}

export interface UpdateProxyPayload {
  name?: string
  alias?: string
  type?: ProxyType
  target?: string
  enabled?: boolean
}

export interface DirectoryEntry {
  name: string
  path: string
}

export interface BrowseDirectoriesResponse {
  current: string
  parent: string
  entries: DirectoryEntry[]
}

export async function listProxies(): Promise<ServiceProxy[]> {
  const res = await get<{ proxies: ServiceProxy[] }>('/proxies')
  return res.proxies
}

export async function getProxy(id: number): Promise<ServiceProxy> {
  return get<ServiceProxy>(`/proxies/${id}`)
}

export async function createProxy(payload: CreateProxyPayload): Promise<ServiceProxy> {
  return mutate<ServiceProxy>('POST', '/proxies', payload)
}

export async function updateProxy(id: number, payload: UpdateProxyPayload): Promise<ServiceProxy> {
  return mutate<ServiceProxy>('PUT', `/proxies/${id}`, payload)
}

export async function deleteProxy(id: number): Promise<void> {
  return mutate<void>('DELETE', `/proxies/${id}`)
}

export async function toggleProxy(id: number): Promise<ServiceProxy> {
  return mutate<ServiceProxy>('POST', `/proxies/${id}/toggle`)
}

export async function browseDirectories(path?: string): Promise<BrowseDirectoriesResponse> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  return get<BrowseDirectoriesResponse>(`/proxies/browse-directories${query}`)
}

