// applyAppearance projects an Appearance config onto the document root: the
// theme palette and density become inline CSS custom properties and the font
// size becomes the root variable. style.css carries the same defaults for the
// pre-JS first paint, and owns the font family — the faces are built into the
// build (src/fonts), so applying an appearance never reaches for a stylesheet.
import { densities, themes, type Appearance, type ThemeColors } from './presets'

// CACHE_KEY is where the last applied appearance is cached so the next page
// load can paint with it before the server responds.
export const CACHE_KEY = 'platform.appearance'

// colorVariable maps a ThemeColors field to the CSS variable that consumes it.
const colorVariables: Record<keyof ThemeColors, string> = {
  bg: '--color-bg',
  surface: '--color-surface',
  surfaceMuted: '--color-surface-muted',
  text: '--color-text',
  textMuted: '--color-text-muted',
  border: '--color-border',
  borderStrong: '--color-border-strong',
  primary: '--color-primary',
  primaryHover: '--color-primary-hover',
  onPrimary: '--color-on-primary',
  danger: '--color-danger',
  dangerBorder: '--color-danger-border',
  accent: '--color-accent',
  success: '--color-success',
  successSoft: '--color-success-soft',
  successBorder: '--color-success-border',
  warning: '--color-warning',
  warningSoft: '--color-warning-soft',
  warningBorder: '--color-warning-border',
  info: '--color-info',
  infoSoft: '--color-info-soft',
  infoBorder: '--color-info-border',
  railBg: '--rail-bg',
  railText: '--rail-text',
  railActive: '--rail-active',
}

export function applyAppearance(appearance: Appearance): void {
  const root = document.documentElement
  root.dataset.theme = appearance.theme
  root.dataset.density = appearance.density

  const theme = themes.find((entry) => entry.key === appearance.theme) ?? themes[0]
  for (const [field, variable] of Object.entries(colorVariables) as [keyof ThemeColors, string][]) {
    root.style.setProperty(variable, theme.colors[field])
  }

  const density = densities.find((entry) => entry.key === appearance.density) ?? densities[1]
  root.style.setProperty('--space-unit', density.spaceUnit)
  root.style.setProperty('--control-height', density.controlHeight)
  root.style.setProperty('--radius', density.radius)

  root.style.setProperty('--font-size-base', `${appearance.fontSize}px`)
}

// applyCachedAppearance paints the last cached appearance before the server
// config is available, so the page does not flash the default theme. It is a
// no-op when nothing valid is cached.
export function applyCachedAppearance(): void {
  try {
    const cached = localStorage.getItem(CACHE_KEY)
    if (cached) applyAppearance(JSON.parse(cached) as Appearance)
  } catch {
    // A corrupt cache is ignored; the server config will replace it.
  }
}
