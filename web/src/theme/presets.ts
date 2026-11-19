// Appearance presets: the single source of truth for the theme palettes the
// settings UI offers. applyAppearance projects these onto the document's CSS
// custom properties, so adding a preset here and a label is all it takes to
// make it selectable. The type of face is not a preset: the UI bundles Google
// Sans Code and Noto Sans SC (src/fonts) and always uses them.

// Appearance is the shared visual configuration. It mirrors the server's
// settings.Appearance DTO; api/settings.ts re-exports it.
export interface Appearance {
  theme: string
  fontSize: number
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
  // previewGradient optionally renders a gradient bar in the settings card preview.
  previewGradient?: string
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
  {
    key: 'ocean',
    label: '蔚蓝',
    dark: false,
    colors: {
      bg: '#f0f4f9',
      surface: '#ffffff',
      surfaceMuted: '#e8eef6',
      text: '#1e293b',
      textMuted: '#64748b',
      border: '#dbe4ef',
      borderStrong: '#cbd5e1',
      primary: '#0284c7',
      primaryHover: '#0369a1',
      primarySoft: '#e0f2fe',
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      dangerBorder: '#fca5a5',
      dangerSoft: '#fee2e2',
      accent: '#0284c7',
      accentSoft: '#e0f2fe',
      success: '#15803d',
      successSoft: '#ecfdf5',
      successBorder: '#86efac',
      warning: '#b45309',
      warningSoft: '#fef3c7',
      warningBorder: '#fcd34d',
      info: '#0284c7',
      infoSoft: '#e0f2fe',
      infoBorder: '#7dd3fc',
      railBg: '#0f172a',
      railText: '#94a3b8',
      railActive: '#1e293b',
    },
  },
  {
    key: 'midnight',
    label: '极光',
    dark: true,
    colors: {
      bg: '#0b1120',
      surface: '#111827',
      surfaceMuted: '#1e293b',
      text: '#f1f5f9',
      textMuted: '#94a3b8',
      border: '#1f293d',
      borderStrong: '#334155',
      primary: '#38bdf8',
      primaryHover: '#0ea5e9',
      primarySoft: '#082f49',
      onPrimary: '#0f172a',
      danger: '#ef4444',
      dangerBorder: '#7f1d1d',
      dangerSoft: '#450a0a',
      accent: '#38bdf8',
      accentSoft: '#0c4a6e',
      success: '#22c55e',
      successSoft: '#052e16',
      successBorder: '#14532d',
      warning: '#f59e0b',
      warningSoft: '#451a03',
      warningBorder: '#78350f',
      info: '#38bdf8',
      infoSoft: '#082f49',
      infoBorder: '#0284c7',
      railBg: '#060a12',
      railText: '#64748b',
      railActive: '#0f172a',
    },
  },
  {
    key: 'emerald',
    label: '翡翠',
    dark: false,
    colors: {
      bg: '#f2f7f4',
      surface: '#ffffff',
      surfaceMuted: '#e8f2ec',
      text: '#132a1e',
      textMuted: '#587163',
      border: '#d1e3d7',
      borderStrong: '#b8d2c0',
      primary: '#059669',
      primaryHover: '#047857',
      primarySoft: '#d1fae5',
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      dangerBorder: '#fca5a5',
      dangerSoft: '#fee2e2',
      accent: '#059669',
      accentSoft: '#d1fae5',
      success: '#059669',
      successSoft: '#d1fae5',
      successBorder: '#6ee7b7',
      warning: '#b45309',
      warningSoft: '#fef3c7',
      warningBorder: '#fcd34d',
      info: '#0284c7',
      infoSoft: '#e0f2fe',
      infoBorder: '#7dd3fc',
      railBg: '#12241b',
      railText: '#8aa695',
      railActive: '#1b3528',
    },
  },
  {
    key: 'sky',
    label: '晴空',
    dark: false,
    previewGradient: 'linear-gradient(to right, #4168d4, #748ed9)',
    colors: {
      bg: '#f1f4fb',
      surface: '#ffffff',
      surfaceMuted: '#e9eef8',
      text: '#1a233a',
      textMuted: '#63718e',
      border: '#dbe2f1',
      borderStrong: '#c3d0e8',
      primary: '#4168d4',
      primaryHover: '#3354b6',
      primarySoft: '#ebf0fb',
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      dangerBorder: '#fca5a5',
      dangerSoft: '#fee2e2',
      accent: '#5a7bd6',
      accentSoft: '#eef3fc',
      success: '#15803d',
      successSoft: '#ecfdf5',
      successBorder: '#86efac',
      warning: '#b45309',
      warningSoft: '#fffbeb',
      warningBorder: '#fcd34d',
      info: '#3354b6',
      infoSoft: '#eff4fc',
      infoBorder: '#93b5fd',
      railBg: '#1c2438',
      railText: '#a3b2d1',
      railActive: '#2e3b5a',
    },
  },
  {
    key: 'bamboo',
    label: '青竹',
    dark: false,
    previewGradient: 'linear-gradient(to right, #70855e, #81966e)',
    colors: {
      bg: '#f3f6f1',
      surface: '#ffffff',
      surfaceMuted: '#eaf0e6',
      text: '#222d1d',
      textMuted: '#63745c',
      border: '#dbe4d5',
      borderStrong: '#c2d1b9',
      primary: '#70855e',
      primaryHover: '#5a6e49',
      primarySoft: '#f0f5ec',
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      dangerBorder: '#fca5a5',
      dangerSoft: '#fee2e2',
      accent: '#788d66',
      accentSoft: '#f0f5ec',
      success: '#5a6e49',
      successSoft: '#edf4e8',
      successBorder: '#b3cca3',
      warning: '#b45309',
      warningSoft: '#fffbeb',
      warningBorder: '#fcd34d',
      info: '#0284c7',
      infoSoft: '#eff6ff',
      infoBorder: '#93c5fd',
      railBg: '#21291d',
      railText: '#a4b59e',
      railActive: '#35432f',
    },
  },
  {
    key: 'turquoise',
    label: '松石',
    dark: false,
    previewGradient: 'linear-gradient(to right, #40bdb5, #40bf92)',
    colors: {
      bg: '#f0f9f7',
      surface: '#ffffff',
      surfaceMuted: '#e4f4f0',
      text: '#132e29',
      textMuted: '#50756e',
      border: '#d0ebe4',
      borderStrong: '#b2ddd3',
      primary: '#2ea898',
      primaryHover: '#228a7c',
      primarySoft: '#e6f7f3',
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      dangerBorder: '#fca5a5',
      dangerSoft: '#fee2e2',
      accent: '#40bea3',
      accentSoft: '#eaf8f4',
      success: '#15803d',
      successSoft: '#ecfdf5',
      successBorder: '#86efac',
      warning: '#b45309',
      warningSoft: '#fffbeb',
      warningBorder: '#fcd34d',
      info: '#0284c7',
      infoSoft: '#eff6ff',
      infoBorder: '#93c5fd',
      railBg: '#132824',
      railText: '#90b7af',
      railActive: '#22433d',
    },
  },
  {
    key: 'firework',
    label: '花火',
    dark: false,
    previewGradient: 'linear-gradient(to right, #eb8594, #eb8e84)',
    colors: {
      bg: '#fdf4f4',
      surface: '#ffffff',
      surfaceMuted: '#faeaeb',
      text: '#341f22',
      textMuted: '#7f595e',
      border: '#f5d5d8',
      borderStrong: '#ebbdc2',
      primary: '#db6779',
      primaryHover: '#c14e60',
      primarySoft: '#fdeff0',
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      dangerBorder: '#fca5a5',
      dangerSoft: '#fee2e2',
      accent: '#eb8789',
      accentSoft: '#fdf1f1',
      success: '#15803d',
      successSoft: '#ecfdf5',
      successBorder: '#86efac',
      warning: '#b45309',
      warningSoft: '#fffbeb',
      warningBorder: '#fcd34d',
      info: '#0284c7',
      infoSoft: '#eff6ff',
      infoBorder: '#93c5fd',
      railBg: '#301c20',
      railText: '#be9da3',
      railActive: '#4e2f35',
    },
  },
  {
    key: 'ink',
    label: '沉墨',
    dark: true,
    previewGradient: 'linear-gradient(to right, #464e66, #6a748f)',
    colors: {
      bg: '#161922',
      surface: '#1e222d',
      surfaceMuted: '#272c3a',
      text: '#e7eaf2',
      textMuted: '#9aa3b8',
      border: '#2e3445',
      borderStrong: '#444d65',
      primary: '#6d7b9d',
      primaryHover: '#8795b8',
      primarySoft: '#282e40',
      onPrimary: '#ffffff',
      danger: '#ef4444',
      dangerBorder: '#7f1d1d',
      dangerSoft: '#450a0a',
      accent: '#7f8ea8',
      accentSoft: '#2e3549',
      success: '#22c55e',
      successSoft: '#052e16',
      successBorder: '#14532d',
      warning: '#f59e0b',
      warningSoft: '#451a03',
      warningBorder: '#78350f',
      info: '#60a5fa',
      infoSoft: '#1e293b',
      infoBorder: '#3b82f6',
      railBg: '#0f1118',
      railText: '#7e879c',
      railActive: '#1d212c',
    },
  },
  {
    key: 'iris',
    label: '菖蒲',
    dark: true,
    previewGradient: 'linear-gradient(to right, #63495d, #856b7d)',
    colors: {
      bg: '#1d171d',
      surface: '#261f26',
      surfaceMuted: '#322932',
      text: '#f2eaf1',
      textMuted: '#ad9bad',
      border: '#3d323d',
      borderStrong: '#544654',
      primary: '#9b7b95',
      primaryHover: '#b593ae',
      primarySoft: '#372b36',
      onPrimary: '#ffffff',
      danger: '#ef4444',
      dangerBorder: '#7f1d1d',
      dangerSoft: '#450a0a',
      accent: '#ac8ba5',
      accentSoft: '#3d303c',
      success: '#22c55e',
      successSoft: '#052e16',
      successBorder: '#14532d',
      warning: '#f59e0b',
      warningSoft: '#451a03',
      warningBorder: '#78350f',
      info: '#c084fc',
      infoSoft: '#2e1065',
      infoBorder: '#a855f7',
      railBg: '#130e13',
      railText: '#877686',
      railActive: '#251c25',
    },
  },
]

// isDarkTheme reports whether a palette key names a dark palette. An unknown
// key is treated as light, which is what a missing setting should draw.
export function isDarkTheme(key: string): boolean {
  return key === 'dark' || themes.find((theme) => theme.key === key)?.dark === true
}

export interface FontSizePreset {
  key: 'small' | 'medium' | 'large'
  label: string
  value: number
}

export const fontSizes: FontSizePreset[] = [
  { key: 'small', label: '小', value: 12 },
  { key: 'medium', label: '标准', value: 14 },
  { key: 'large', label: '大', value: 16 },
]

// The server normalizes legacy 12–20 values onto a preset on read, but a value
// cached before that migration can still sit between two presets. Ties go to
// the lower tier: 13 → small, 15 → medium.
export function resolveFontSizeKey(fontSize: number): 'small' | 'medium' | 'large' {
  if (fontSize <= 13) return 'small'
  if (fontSize >= 16) return 'large'
  return 'medium'
}
