// applyAppearance projects an Appearance config onto the document root: the
// theme palette and density become inline CSS custom properties, the font
// family and size become root variables, and a custom Google Fonts family gets
// a stylesheet link. style.css carries the same defaults for the pre-JS first
// paint.
import {
  densities,
  googleFontURL,
  resolveFontFamily,
  themes,
  type Appearance,
  type ThemeColors,
} from './presets'

// CACHE_KEY is where the last applied appearance is cached so the next page
// load can paint with it before the server responds.
export const CACHE_KEY = 'platform.appearance'

const GOOGLE_FONT_LINK_ID = 'platform-google-font'

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
  accent: '--color-accent',
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

  root.style.setProperty('--font-family', resolveFontFamily(appearance))
  root.style.setProperty('--font-size-base', `${appearance.fontSize}px`)

  applyGoogleFont(appearance)
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

function applyGoogleFont(appearance: Appearance): void {
  const existing = document.getElementById(GOOGLE_FONT_LINK_ID)
  if (appearance.font !== 'custom' || appearance.customFont.trim() === '') {
    existing?.remove()
    return
  }

  const link = (existing as HTMLLinkElement | null) ?? document.createElement('link')
  link.id = GOOGLE_FONT_LINK_ID
  link.rel = 'stylesheet'
  link.href = googleFontURL(appearance.customFont)
  if (!existing) document.head.appendChild(link)
}
