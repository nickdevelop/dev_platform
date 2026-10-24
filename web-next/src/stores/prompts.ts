import { create } from 'zustand'
import {
  createPrompt,
  deletePrompt,
  listPrompts,
  updatePrompt,
  type CreatePromptPayload,
  type Prompt,
  type UpdatePromptPayload,
} from '../api/prompts'
import { describeApiError, UnauthorizedError } from '../api/client'

interface PromptsState {
  prompts: Prompt[]
  loading: boolean
  error: string
  loadPrompts: (keyword?: string) => Promise<void>
  create: (payload: CreatePromptPayload) => Promise<Prompt | null>
  update: (id: number, payload: UpdatePromptPayload) => Promise<Prompt | null>
  remove: (id: number) => Promise<boolean>
  byId: (id: number) => Prompt | undefined
}

export const usePromptsStore = create<PromptsState>((set, getState) => ({
  prompts: [],
  loading: false,
  error: '',

  loadPrompts: async (keyword) => {
    set({ loading: true })
    try {
      const prompts = await listPrompts(keyword)
      set({ prompts, loading: false, error: '' })
    } catch (error) {
      if (error instanceof UnauthorizedError) {
        set({ loading: false })
        return
      }
      set({ loading: false, error: describeApiError(error) })
    }
  },

  create: async (payload) => {
    try {
      const created = await createPrompt(payload)
      set((state) => ({ prompts: [created, ...state.prompts], error: '' }))
      return created
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ error: describeApiError(error) })
      return null
    }
  },

  update: async (id, payload) => {
    try {
      const updated = await updatePrompt(id, payload)
      set((state) => ({
        prompts: state.prompts.map((prompt) => (prompt.id === id ? updated : prompt)),
        error: '',
      }))
      return updated
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ error: describeApiError(error) })
      return null
    }
  },

  remove: async (id) => {
    try {
      await deletePrompt(id)
      set((state) => ({ prompts: state.prompts.filter((prompt) => prompt.id !== id), error: '' }))
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      set({ error: describeApiError(error) })
      return false
    }
  },

  byId: (id) => getState().prompts.find((prompt) => prompt.id === id),
}))
