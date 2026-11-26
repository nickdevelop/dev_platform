// Extension filter model for the quick file/content search dialog. The dialog
// offers a single choice: one of the preset groups (each mapping to a set of
// extensions) or a manually typed raw value ("*.kt", "kt,json"). Parsing lives
// here so the dialog stays template-only and the mapping is unit-testable.

export interface ExtFilterGroup {
  label: string
  exts: string[]
}

export const EXT_FILTER_GROUPS: ExtFilterGroup[] = [
  { label: 'Java', exts: ['.java'] },
  { label: '前端 JS/TS', exts: ['.js', '.jsx', '.ts', '.tsx', '.vue'] },
  { label: 'Go', exts: ['.go'] },
  { label: 'Python', exts: ['.py'] },
  { label: 'SQL', exts: ['.sql'] },
  { label: 'Shell 脚本', exts: ['.sh', '.bash', '.ps1'] },
  { label: 'C/C++', exts: ['.c', '.h', '.cpp', '.hpp'] },
  { label: 'Rust', exts: ['.rs'] },
  { label: '样式', exts: ['.css', '.scss', '.less'] },
  { label: '配置数据', exts: ['.json', '.yaml', '.yml', '.toml', '.xml', '.properties'] },
  { label: '文档', exts: ['.md', '.txt'] },
]

export const EXT_FILTER_OPTIONS = EXT_FILTER_GROUPS.map((g) => ({
  value: g.label,
  label: `${g.label}（${g.exts.join(' ')}）`,
}))

// resolveExtFilter maps the current selection onto dotted extensions for the
// backend ("不限制" / empty → undefined, i.e. every text file is searched).
// Manual input accepts "*.kt", "kt", or several tokens separated by commas,
// semicolons or spaces (e.g. "kt,json").
export function resolveExtFilter(selection: string): string[] | undefined {
  const sel = selection.trim()
  if (!sel) return undefined
  const group = EXT_FILTER_GROUPS.find((g) => g.label === sel)
  if (group) return group.exts
  const tokens = Array.from(
    new Set(
      sel
        .toLowerCase()
        .split(/[,;\s]+/)
        .map((tok) => tok.trim().replace(/^\*\.?/, '').replace(/^\.+/, ''))
        .filter((tok) => tok !== '')
        .map((tok) => `.${tok}`),
    ),
  )
  return tokens.length > 0 ? tokens : undefined
}
