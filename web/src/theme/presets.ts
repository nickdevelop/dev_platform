// Appearance presets: the single source of truth for the theme palettes and
// density scales the settings UI offers. applyAppearance projects these onto the
// document's CSS custom properties, so adding a preset here and a label is all
// it takes to make it selectable. The type of face is not a preset: the UI
// bundles Google Sans Code and Noto Sans SC (src/fonts) and always uses them.

// Appearance is the shared visual configuration. It mirrors the server's
// settings.Appearance DTO; api/settings.ts re-exports it.
export interface Appearance {
  theme: string
  fontSize: number
  density: string
  plantumlMode?: 'local' | 'online'
  plantumlServerUrl?: string
}

// ThemeColors is the full palette a theme defines, not just an accent color.
export interface ThemeColors {
  bg: string
  surface: string
  surfaceMuted: string
  text: string
  textMuted: string
  border: string
  borderStrong: string
  primary: string
  primaryHover: string
  // primarySoft is the tinted fill a selected/highlighted surface sits on. It is
  // a preset color rather than a translucent overlay so a themed UI never has to
  // guess at the primary's hue with an rgba() literal.
  primarySoft: string
  onPrimary: string
  danger: string
  dangerBorder: string
  dangerSoft: string
  accent: string
  accentSoft: string
  success: string
  successSoft: string
  successBorder: string
  warning: string
  warningSoft: string
  warningBorder: string
  info: string
  infoSoft: string
  infoBorder: string
  railBg: string
  railText: string
  railActive: string
}

export interface ThemePreset {
  key: string
  label: string
  // dark marks a palette a light-on-dark drawing belongs to. It is not derived
  // from the colors: the PlantUML engine needs one flag to redraw a diagram in
  // dark, and asking it to guess per palette would drift.
  dark: boolean
  colors: ThemeColors
}

export interface DensityPreset {
  key: string
  label: string
  spaceUnit: string
  controlHeight: string
  radius: string
}

export const themes: ThemePreset[] = [
  {
    key: 'light',
    label: '浅色',
    dark: false,
    colors: {
      bg: '#f4f5f7',
      surface: '#ffffff',
      surfaceMuted: '#f8fafc',
      text: '#1f2328',
      textMuted: '#64748b',
      border: '#e3e6eb',
      borderStrong: '#cbd5e1',
      primary: '#2563eb',
      primaryHover: '#1d4ed8',
      primarySoft: '#e8effd',
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      dangerBorder: '#fca5a5',
      dangerSoft: '#fee2e2',
      accent: '#4a8ad6',
      accentSoft: '#edf3fb',
      success: '#15803d',
      successSoft: '#ecfdf5',
      successBorder: '#86efac',
      warning: '#92400e',
      warningSoft: '#fffbeb',
      warningBorder: '#fcd34d',
      info: '#0369a1',
      infoSoft: '#eff6ff',
      infoBorder: '#93c5fd',
      railBg: '#1e2330',
      railText: '#aab2c5',
      railActive: '#3b4560',
    },
  },
  {
    key: 'dark',
    label: '深色',
    dark: true,
    colors: {
      bg: '#0f1115',
      surface: '#14171f',
      surfaceMuted: '#1a1e27',
      text: '#e6e8ef',
      textMuted: '#8a8c92',
      border: '#222631',
      borderStrong: '#2e3545',
      primary: '#4a8ad6',
      primaryHover: '#3975be',
      primarySoft: '#152236',
      onPrimary: '#0b0d11',
      danger: '#e5566f',
      dangerBorder: '#6d2230',
      dangerSoft: '#2c161d',
      accent: '#4a8ad6',
      accentSoft: '#152236',
      success: '#44a282',
      successSoft: '#12241e',
      successBorder: '#225845',
      warning: '#d9a441',
      warningSoft: '#282012',
      warningBorder: '#6d521e',
      info: '#4a8ad6',
      infoSoft: '#152236',
      infoBorder: '#233e63',
      railBg: '#0a0c0f',
      railText: '#8a8c92',
      railActive: '#1c202a',
    },
  },
  {
    key: 'warm',
    label: '暖阳',
    dark: false,
    colors: {
      bg: '#f6f1e7',
      surface: '#fffaf0',
      surfaceMuted: '#f1e8d8',
      text: '#3a3226',
      textMuted: '#7a6f5c',
      border: '#e4d9c5',
      borderStrong: '#d3c4a8',
      primary: '#b45309',
      primaryHover: '#92400e',
      primarySoft: '#f9ead7',
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      dangerBorder: '#e0a3a3',
      dangerSoft: '#fbe3e3',
      accent: '#b45309',
      accentSoft: '#fdf6eb',
      success: '#15803d',
      successSoft: '#eef7ee',
      successBorder: '#a7d8a7',
      warning: '#92400e',
      warningSoft: '#fdf6e3',
      warningBorder: '#e0c98a',
      info: '#1d4ed8',
      infoSoft: '#eef2fb',
      infoBorder: '#b9c8ea',
      railBg: '#4a3f2f',
      railText: '#cbbfa8',
      railActive: '#6b5a3f',
    },
  },
  {
    key: 'cool',
    label: '冷灰',
    dark: false,
    colors: {
      bg: '#eef1f5',
      surface: '#f8fafc',
      surfaceMuted: '#e6ebf2',
      text: '#1e293b',
      textMuted: '#5b6b82',
      border: '#d5dde8',
      borderStrong: '#c2ccda',
      primary: '#0f766e',
      primaryHover: '#115e59',
      primarySoft: '#dcefec',
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      dangerBorder: '#e0a3a3',
      dangerSoft: '#fbe3e3',
      accent: '#0f766e',
      accentSoft: '#e6f4f1',
      success: '#0f766e',
      successSoft: '#e6f4f1',
      successBorder: '#8ecfc6',
      warning: '#b45309',
      warningSoft: '#fdf3e7',
      warningBorder: '#e6c79a',
      info: '#0369a1',
      infoSoft: '#e8f1f8',
      infoBorder: '#9cc3dd',
      railBg: '#1b2733',
      railText: '#9fb0c0',
      railActive: '#2f3d4d',
    },
  },
]

// isDarkTheme reports whether a palette key names a dark palette. An unknown
// key is treated as light, which is what a missing setting should draw.
export function isDarkTheme(key: string): boolean {
  return themes.find((theme) => theme.key === key)?.dark === true
}

export const densities: DensityPreset[] = [
  { key: 'compact', label: '紧凑', spaceUnit: '4px', controlHeight: '28px', radius: '3px' },
  { key: 'standard', label: '标准', spaceUnit: '6px', controlHeight: '34px', radius: '4px' },
  { key: 'relaxed', label: '宽松', spaceUnit: '8px', controlHeight: '40px', radius: '6px' },
]

export interface FontSizePreset {
  key: 'small' | 'medium' | 'large'
  label: string
  value: number
}

export const fontSizes: FontSizePreset[] = [
  { key: 'small', label: '小', value: 13 },
  { key: 'medium', label: '中', value: 14 },
  { key: 'large', label: '大', value: 16 },
]

export function resolveFontSizeKey(fontSize: number): 'small' | 'medium' | 'large' {
  if (fontSize <= 13) return 'small'
  if (fontSize >= 16) return 'large'
  return 'medium'
}
