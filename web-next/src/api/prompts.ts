// API client for Prompt Management.
import { get, mutate } from './client'

export interface Prompt {
  id: number
  name: string
  description: string
  content: string
  createdAt: string
  updatedAt: string
}

export interface CreatePromptPayload {
  name: string
  description?: string
  content: string
}

export interface UpdatePromptPayload {
  name?: string
  description?: string
  content?: string
}

interface PromptListResponse {
  prompts: Prompt[]
}

export async function listPrompts(keyword?: string): Promise<Prompt[]> {
  const query = keyword ? `?q=${encodeURIComponent(keyword)}` : ''
  const res = await get<PromptListResponse>(`/prompts${query}`)
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
