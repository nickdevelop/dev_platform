// API client for Role Management.
import { get, mutate } from './client'

export interface AgentRole {
  id: number
  name: string
  description: string
  backend: string
  model: string
  promptId: number
  systemPrompt: string
  icon?: string
  color?: string
  createdAt: string
  updatedAt: string
}

export interface CreateRolePayload {
  name: string
  description?: string
  backend?: string
  model: string
  promptId?: number
  systemPrompt?: string
  icon?: string
  color?: string
}

export interface UpdateRolePayload {
  name?: string
  description?: string
  backend?: string
  model?: string
  promptId?: number
  systemPrompt?: string
  icon?: string
  color?: string
}

interface RoleListResponse {
  roles: AgentRole[]
}

export async function listRoles(keyword?: string): Promise<AgentRole[]> {
  const query = keyword ? `?q=${encodeURIComponent(keyword)}` : ''
  const res = await get<RoleListResponse>(`/roles${query}`)
  return res.roles
}

export async function getRole(id: number): Promise<AgentRole> {
  return get<AgentRole>(`/roles/${id}`)
}

export async function createRole(payload: CreateRolePayload): Promise<AgentRole> {
  return mutate<AgentRole>('POST', '/roles', payload)
}

export async function updateRole(id: number, payload: UpdateRolePayload): Promise<AgentRole> {
  return mutate<AgentRole>('PUT', `/roles/${id}`, payload)
}

export async function deleteRole(id: number): Promise<void> {
  await mutate<{ ok: boolean }>('DELETE', `/roles/${id}`)
}
