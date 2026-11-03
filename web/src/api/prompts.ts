// API client for Prompt Management.
import { get, mutate } from './client'

export type PromptType = 'role' | 'flow'

export interface Prompt {
  id: number
  name: string
  type: PromptType
  description: string
  content: string
  createdAt: string
  updatedAt: string
}

export interface CreatePromptPayload {
  name: string
  type?: PromptType
  description?: string
  content: string
}

export interface UpdatePromptPayload {
  name?: string
  type?: PromptType
  description?: string
  content?: string
}

interface PromptListResponse {
  prompts: Prompt[]
}

export async function listPrompts(keyword?: string, type?: PromptType): Promise<Prompt[]> {
  const params = new URLSearchParams()
  if (keyword) params.append('q', keyword)
  if (type) params.append('type', type)
  const qs = params.toString() ? `?${params.toString()}` : ''
  const res = await get<PromptListResponse>(`/prompts${qs}`)
  return res.prompts
}

export async function getPrompt(id: number): Promise<Prompt> {
  return get<Prompt>(`/prompts/${id}`)
}

export async function createPrompt(payload: CreatePromptPayload): Promise<Prompt> {
  return mutate<Prompt>('POST', '/prompts', payload)
}

export async function updatePrompt(id: number, payload: UpdatePromptPayload): Promise<Prompt> {
  return mutate<Prompt>('PUT', `/prompts/${id}`, payload)
}

export async function deletePrompt(id: number): Promise<void> {
  await mutate<{ ok: boolean }>('DELETE', `/prompts/${id}`)
}
