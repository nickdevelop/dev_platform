import { create } from 'zustand'
import { describeApiError, UnauthorizedError } from '../api/client'
import {
  defaultAppearance,
  getAppearance,
  getQuickAssistantSettings,
  putAppearance,
  putQuickAssistantSettings,
  type Appearance,
  type QuickAssistantSettings,
} from '../api/settings'
import { useAppearanceStore } from './appearance'

interface SettingsState {
  appearance: Appearance
  loading: boolean
  saving: boolean
  error: string
  quickAssistant: QuickAssistantSettings
  quickAssistantLoading: boolean
  quickAssistantSaving: boolean
  quickAssistantError: string
  loadAppearance: () => Promise<void>
  saveAppearance: (patch: Partial<Appearance>) => Promise<boolean>
  loadQuickAssistant: () => Promise<void>
  saveQuickAssistantEnabled: (enabled: boolean) => Promise<boolean>
}

function currentAppearance(): Appearance {
  return useAppearanceStore.getState().appearance
}

export const useSettingsStore = create<SettingsState>((set, get) => ({
  appearance: currentAppearance(),
  loading: false,
  saving: false,
  error: '',
  quickAssistant: { enabled: false },
  quickAssistantLoading: false,
  quickAssistantSaving: false,
  quickAssistantError: '',

  loadAppearance: async () => {
    set({ loading: true })
    try {
      const appearance = await getAppearance()
      useAppearanceStore.getState().setAppearance(appearance)
      set({ appearance, error: '', loading: false })
    } catch (error) {
      if (error instanceof UnauthorizedError) {
        set({ loading: false })
        return
      }
      set({ error: describeApiError(error), loading: false })
    }
  },

  saveAppearance: async (patch) => {
    const next = { ...get().appearance, ...patch }
    set({ saving: true })
    try {
      const saved = await putAppearance(next)
      useAppearanceStore.getState().setAppearance(saved)
      set({ appearance: saved, error: '', saving: false })
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) {
        set({ saving: false })
        return false
      }
      set({ error: describeApiError(error), saving: false })
      return false
    }
  },

  loadQuickAssistant: async () => {
    set({ quickAssistantLoading: true })
    try {
      const config = await getQuickAssistantSettings()
      set({ quickAssistant: config, quickAssistantError: '', quickAssistantLoading: false })
    } catch (error) {
      if (error instanceof UnauthorizedError) {
        set({ quickAssistantLoading: false })
        return
      }
      set({ quickAssistantError: describeApiError(error), quickAssistantLoading: false })
    }
  },

  saveQuickAssistantEnabled: async (enabled) => {
    set({ quickAssistantSaving: true })
    try {
      const saved = await putQuickAssistantSettings({ enabled })
      set({ quickAssistant: saved, quickAssistantError: '', quickAssistantSaving: false })
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) {
        set({ quickAssistantSaving: false })
        return false
      }
      set({ quickAssistantError: describeApiError(error), quickAssistantSaving: false })
      return false
    }
  },
}))

// Keep the settings view aligned with the local appearance store after a theme
// picker changes it outside the server round-trip.
useAppearanceStore.subscribe((state) => {
  const current = useSettingsStore.getState().appearance
  if (state.appearance !== current) {
    useSettingsStore.setState({ appearance: state.appearance })
  }
})

export { defaultAppearance }
