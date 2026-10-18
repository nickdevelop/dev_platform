// API client for Prompt Management.
import { apiRequest } from './client'

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
  const res = await apiRequest<PromptListResponse>('GET', `/prompts${query}`)
  return res.prompts
}

export async function getPrompt(id: number): Promise<Prompt> {
  return apiRequest<Prompt>('GET', `/prompts/${id}`)
}

export async function createPrompt(csrfToken: string, payload: CreatePromptPayload): Promise<Prompt> {
  return apiRequest<Prompt>('POST', '/prompts', { body: payload, csrfToken })
}

export async function updatePrompt(csrfToken: string, id: number, payload: UpdatePromptPayload): Promise<Prompt> {
  return apiRequest<Prompt>('PUT', `/prompts/${id}`, { body: payload, csrfToken })
}

export async function deletePrompt(csrfToken: string, id: number): Promise<void> {
  await apiRequest<{ ok: boolean }>('DELETE', `/prompts/${id}`, { csrfToken })
}
