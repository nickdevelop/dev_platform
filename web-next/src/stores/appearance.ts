import { create } from 'zustand'
import { CACHE_KEY } from '../theme/apply'
import { densities, themes, type Appearance } from '../theme/presets'

export const defaultAppearance: Appearance = {
  theme: 'light',
  fontSize: 14,
  density: 'standard',
  plantumlMode: 'local',
}

function readCachedAppearance(): Appearance {
  try {
    const raw = localStorage.getItem(CACHE_KEY)
    if (!raw) return defaultAppearance
    const parsed = JSON.parse(raw) as Partial<Appearance>
    const theme = themes.some((entry) => entry.key === parsed.theme) ? parsed.theme! : 'light'
    const density = densities.some((entry) => entry.key === parsed.density)
      ? parsed.density!
      : 'standard'
    const fontSize =
      typeof parsed.fontSize === 'number' && parsed.fontSize >= 12 && parsed.fontSize <= 20
        ? parsed.fontSize
        : 14
    return { ...defaultAppearance, ...parsed, theme, density, fontSize }
  } catch {
    return defaultAppearance
  }
}

interface AppearanceState {
  appearance: Appearance
  setAppearance: (patch: Partial<Appearance>) => void
  reset: () => void
}

export const useAppearanceStore = create<AppearanceState>((set) => ({
  appearance: readCachedAppearance(),
  setAppearance: (patch) => {
    set((state) => {
      const next = { ...state.appearance, ...patch }
      try {
        localStorage.setItem(CACHE_KEY, JSON.stringify(next))
      } catch {
        // Best-effort only, same contract as the Vue application.
      }
      return { appearance: next }
    })
  },
  reset: () => {
    try {
      localStorage.removeItem(CACHE_KEY)
    } catch {
      // Best-effort only.
    }
    set({ appearance: defaultAppearance })
  },
}))
