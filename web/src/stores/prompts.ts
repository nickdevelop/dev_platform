// Store for Prompt Management.
import { defineStore } from 'pinia'
import { describeApiError, UnauthorizedError } from '../api/client'
import {
  listPrompts,
  getPrompt,
  createPrompt,
  updatePrompt,
  deletePrompt,
  type Prompt,
  type CreatePromptPayload,
  type UpdatePromptPayload,
} from '../api/prompts'
import { useAuthStore } from './auth'

export const usePromptsStore = defineStore('prompts', {
  state: () => ({
    prompts: [] as Prompt[],
    currentPrompt: null as Prompt | null,
    loading: false,
    error: '',
  }),

  getters: {
    byId: (state) => (id: number): Prompt | undefined =>
      state.prompts.find((p) => p.id === id),
  },

  actions: {
    async loadPrompts(keyword?: string) {
      this.loading = true
      try {
        this.prompts = await listPrompts(keyword)
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      } finally {
        this.loading = false
      }
    },

    async loadPrompt(id: number) {
      this.loading = true
      try {
        const prompt = await getPrompt(id)
        this.currentPrompt = prompt
        const idx = this.prompts.findIndex((p) => p.id === id)
        if (idx !== -1) {
          this.prompts[idx] = prompt
        } else {
          this.prompts.unshift(prompt)
        }
        this.error = ''
        return prompt
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async createPrompt(payload: CreatePromptPayload) {
      this.loading = true
      try {
        const auth = useAuthStore()
        const prompt = await createPrompt(payload)
        this.prompts.unshift(prompt)
        this.error = ''
        return prompt
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async updatePrompt(id: number, payload: UpdatePromptPayload) {
      this.loading = true
      try {
        const auth = useAuthStore()
        const updated = await updatePrompt(id, payload)
        const idx = this.prompts.findIndex((p) => p.id === id)
        if (idx !== -1) {
          this.prompts[idx] = updated
        }
        if (this.currentPrompt?.id === id) {
          this.currentPrompt = updated
        }
        this.error = ''
        return updated
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async deletePrompt(id: number) {
      this.loading = true
      try {
        const auth = useAuthStore()
        await deletePrompt(id)
        this.prompts = this.prompts.filter((p) => p.id !== id)
        if (this.currentPrompt?.id === id) {
          this.currentPrompt = null
        }
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },
  },
})
