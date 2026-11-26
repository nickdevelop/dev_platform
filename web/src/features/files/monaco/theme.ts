import * as monaco from 'monaco-editor'
import { themes } from '../../../theme/presets'

const registeredThemes = new Set<string>()

function normalizeHex(color: string, fallback: string): string {
  const c = color.trim()
  if (/^#[0-9a-fA-F]{6}$/.test(c) || /^#[0-9a-fA-F]{8}$/.test(c)) {
    return c
  }
  if (/^#[0-9a-fA-F]{3}$/.test(c)) {
    return `#${c[1]}${c[1]}${c[2]}${c[2]}${c[3]}${c[3]}`
  }
  return fallback
}

export function getMonacoThemeName(themeKey?: string): string {
  const preset = themes.find((t) => t.key === themeKey) ?? themes[0]
  const name = `platform-${preset.key}`

  if (!registeredThemes.has(name)) {
    const c = preset.colors
    const surface = normalizeHex(c.surface, preset.dark ? '#18181b' : '#ffffff')
    const surfaceMuted = normalizeHex(c.surfaceMuted, preset.dark ? '#27272a' : '#f8fafc')
    const text = normalizeHex(c.text, preset.dark ? '#e4e4e7' : '#1f2328')
    const textMuted = normalizeHex(c.textMuted, preset.dark ? '#a1a1aa' : '#64748b')
    const border = normalizeHex(c.border, preset.dark ? '#27272a' : '#e3e6eb')
    const primary = normalizeHex(c.primary, '#2563eb')
    const primarySoft = normalizeHex(c.primarySoft, preset.dark ? '#1e3a5f' : '#e8effd')

    monaco.editor.defineTheme(name, {
      base: preset.dark ? 'vs-dark' : 'vs',
      inherit: true,
      rules: [],
      colors: {
        'editor.background': surface,
        'editor.foreground': text,
        'editorLineNumber.foreground': textMuted,
        'editorLineNumber.activeForeground': text,
        'editorCursor.foreground': primary,
        'editor.selectionBackground': primarySoft,
        'editor.lineHighlightBackground': surfaceMuted,
        'editorGutter.background': surface,
        'editorWidget.background': surface,
        'editorWidget.border': border,
        'diffEditor.insertedTextBackground': preset.dark ? '#10b98126' : '#10b9811f',
        'diffEditor.removedTextBackground': preset.dark ? '#ef444426' : '#ef44441f',
      },
    })
    registeredThemes.add(name)
  }

  return name
}

export function applyMonacoTheme(themeKey?: string): string {
  const name = getMonacoThemeName(themeKey)
  monaco.editor.setTheme(name)
  return name
}

export function getMonacoFontFamily(): string {
  if (typeof window !== 'undefined' && typeof document !== 'undefined') {
    const v = getComputedStyle(document.documentElement).getPropertyValue('--font-mono').trim()
    if (v) return v
  }
  return '"Google Sans Code", "Noto Sans SC", monospace'
}

// Match the pre-Monaco CodeView scale (`0.82rem`; 11.48px at the default 14px
// root font size) and scale proportionally with the appearance fontSize setting.
export function getMonacoFontSize(baseFontSize?: number): number {
  const base = typeof baseFontSize === 'number' && baseFontSize > 0 ? baseFontSize : 14
  return Math.round(base * 0.82 * 100) / 100
}
