// Appearance presets: the single source of truth for the theme palettes, font
// stacks and density scales the settings UI offers. applyAppearance projects
// these onto the document's CSS custom properties, so adding a preset here and
// a label is all it takes to make it selectable.

// Appearance is the shared visual configuration. It mirrors the server's
// settings.Appearance DTO; api/settings.ts re-exports it.
export interface Appearance {
  theme: string
  font: string
  customFont: string
  fontSize: number
  density: string
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
  onPrimary: string
  danger: string
  accent: string
}

export interface ThemePreset {
  key: string
  label: string
  colors: ThemeColors
}

export interface FontPreset {
  key: string
  label: string
  stack: string
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
    colors: {
      bg: '#f4f5f7',
      surface: '#ffffff',
      surfaceMuted: '#f8fafc',
      text: '#1f2328',
      textMuted: '#64748b',
      border: '#e3e6eb',
      borderStrong: '#d0d5dd',
      primary: '#2563eb',
      primaryHover: '#1d4ed8',
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      accent: '#7c3aed',
    },
  },
  {
    key: 'dark',
    label: '深色',
    colors: {
      bg: '#0f1115',
      surface: '#171a21',
      surfaceMuted: '#1e222b',
      text: '#e6e8ef',
      textMuted: '#9aa3b2',
      border: '#2a2f3a',
      borderStrong: '#3a4150',
      primary: '#4f8cff',
      primaryHover: '#6ba0ff',
      onPrimary: '#0b0d11',
      danger: '#f87171',
      accent: '#a78bfa',
    },
  },
  {
    key: 'warm',
    label: '暖阳',
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
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      accent: '#9333ea',
    },
  },
  {
    key: 'cool',
    label: '冷灰',
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
      onPrimary: '#ffffff',
      danger: '#b91c1c',
      accent: '#6366f1',
    },
  },
]

const systemStack =
  "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif"

export const fonts: FontPreset[] = [
  { key: 'system', label: '系统默认', stack: systemStack },
  {
    key: 'sans',
    label: '无衬线',
    stack: "'Noto Sans SC', 'PingFang SC', 'Microsoft YaHei', 'Segoe UI', sans-serif",
  },
  {
    key: 'serif',
    label: '衬线',
    stack: "'Noto Serif SC', 'Songti SC', Georgia, 'Times New Roman', serif",
  },
  {
    key: 'mono',
    label: '等宽',
    stack: "'JetBrains Mono', 'Fira Code', 'SFMono-Regular', Consolas, monospace",
  },
]

export const densities: DensityPreset[] = [
  { key: 'compact', label: '紧凑', spaceUnit: '4px', controlHeight: '28px', radius: '5px' },
  { key: 'standard', label: '标准', spaceUnit: '6px', controlHeight: '34px', radius: '6px' },
  { key: 'relaxed', label: '宽松', spaceUnit: '8px', controlHeight: '40px', radius: '8px' },
]

// googleFontURL builds the stylesheet URL for one Google Fonts family. Spaces
// become `+`; everything else is percent-encoded so a family name can never
// inject query parameters.
export function googleFontURL(family: string): string {
  const encoded = encodeURIComponent(family.trim()).replace(/%20/g, '+')
  return `https://fonts.googleapis.com/css2?family=${encoded}&display=swap`
}

// resolveFontFamily returns the CSS font-family value for an appearance: the
// custom family (with the system stack as fallback) or the selected preset's
// stack. An unknown key falls back to the system stack.
export function resolveFontFamily(appearance: Appearance): string {
  if (appearance.font === 'custom' && appearance.customFont.trim() !== '') {
    return `'${appearance.customFont.trim()}', ${systemStack}`
  }
  const preset = fonts.find((font) => font.key === appearance.font)
  return preset?.stack ?? systemStack
}
