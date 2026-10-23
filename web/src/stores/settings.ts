// Pinia store for the platform's shared appearance settings. Loading applies
// the server config to the document and caches it for the next first paint;
// saving applies the stored result and leaves the previous value untouched when
// the server rejects the change.
import { defineStore } from 'pinia'
import {
  defaultAppearance,
  detectAgent,
  getAppearance,
  getQuickAssistantSettings,
  listAgents,
  putAppearance,
  putQuickAssistantSettings,
  setAgentEnabled,
  setAgentMode,
  setAgentPath,
  type AgentDetection,
  type AgentDto,
  type Appearance,
  type QuickAssistantSettings,
} from '../api/settings'
import { describeApiError, UnauthorizedError } from '../api/client'
import { applyAppearance, CACHE_KEY } from '../theme/apply'

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
    quickAssistant: { enabled: false } as QuickAssistantSettings,
    quickAssistantLoading: false,
    quickAssistantSaving: false,
    quickAssistantError: '',
    agents: [] as AgentDto[],
    agentsLoading: false,
    agentsError: '',
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
        const saved = await putAppearance(next)
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

    async loadQuickAssistant() {
      this.quickAssistantLoading = true
      try {
        const config = await getQuickAssistantSettings()
        this.quickAssistant = config
        this.quickAssistantError = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.quickAssistantError = describeApiError(error)
      } finally {
        this.quickAssistantLoading = false
      }
    },

    // saveQuickAssistantEnabled persists the toggle. On failure the previous
    // value stays in state so the floating button's visibility cannot drift
    // from the server's stored setting.
    async saveQuickAssistantEnabled(enabled: boolean): Promise<boolean> {
      this.quickAssistantSaving = true
      try {
        const saved = await putQuickAssistantSettings({ enabled })
        this.quickAssistant = saved
        this.quickAssistantError = ''
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.quickAssistantError = describeApiError(error)
        return false
      } finally {
        this.quickAssistantSaving = false
      }
    },

    async loadAgents() {
      this.agentsLoading = true
      try {
        const { agents } = await listAgents()
        this.agents = agents
        this.agentsError = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.agentsError = describeApiError(error)
      } finally {
        this.agentsLoading = false
      }
    },

    async setAgentEnabled(backend: string, enabled: boolean): Promise<void> {
      try {
        await setAgentEnabled(backend, enabled)
        await this.loadAgents()
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.agentsError = describeApiError(error)
      }
    },

    async setAgentPath(backend: string, path: string): Promise<boolean> {
      try {
        await setAgentPath(backend, path)
        await this.loadAgents()
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.agentsError = describeApiError(error)
        return false
      }
    },

    async setAgentMode(backend: string, mode: string): Promise<boolean> {
      try {
        await setAgentMode(backend, mode)
        await this.loadAgents()
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.agentsError = describeApiError(error)
        return false
      }
    },

    async detectAgent(backend: string): Promise<AgentDetection | null> {
      try {
        const detection = await detectAgent(backend)
        await this.loadAgents()
        return detection
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.agentsError = describeApiError(error)
        return null
      }
    },
  },
})
