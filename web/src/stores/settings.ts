// Pinia store for the platform's shared appearance settings. Loading applies
// the server config to the document and caches it for the next first paint;
// saving applies the stored result and leaves the previous value untouched when
// the server rejects the change.
import { defineStore } from 'pinia'
import {
  defaultAppearance,
  getAppearance,
  putAppearance,
  type Appearance,
} from '../api/settings'
import { describeApiError, UnauthorizedError } from '../api/client'
import { applyAppearance, CACHE_KEY } from '../theme/apply'
import { useAuthStore } from './auth'

function cacheAppearance(appearance: Appearance): void {
  try {
    localStorage.setItem(CACHE_KEY, JSON.stringify(appearance))
  } catch {
    // A full or unavailable localStorage is not fatal; the server remains the
    // source of truth.
  }
}

export const useSettingsStore = defineStore('settings', {
  state: () => ({
    appearance: { ...defaultAppearance } as Appearance,
    loading: false,
    saving: false,
    error: '',
  }),

  actions: {
    async loadAppearance() {
      this.loading = true
      try {
        const appearance = await getAppearance()
        this.appearance = appearance
        applyAppearance(appearance)
        cacheAppearance(appearance)
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return // global handler went to login
        this.error = describeApiError(error)
      } finally {
        this.loading = false
      }
    },

    // saveAppearance persists the appearance merged with the patch. On failure
    // the previous appearance stays applied and in state.
    async saveAppearance(patch: Partial<Appearance>): Promise<boolean> {
      const next = { ...this.appearance, ...patch }
      this.saving = true
      try {
        const saved = await putAppearance(next, useAuthStore().csrfToken)
        this.appearance = saved
        applyAppearance(saved)
        cacheAppearance(saved)
        this.error = ''
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      } finally {
        this.saving = false
      }
    },
  },
})
