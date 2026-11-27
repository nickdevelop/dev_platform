// Extension filter model for the quick file/content search dialog. The dialog
// offers a single choice: the "全部后缀" entry (no filter), one of the preset
// groups (each mapping to a set of extensions) or a manually typed raw value
// ("*.kt", "kt,json"). Parsing lives here so the dialog stays template-only
// and the mapping is unit-testable.

export interface ExtFilterGroup {
  label: string
  exts: string[]
}

export const EXT_FILTER_GROUPS: ExtFilterGroup[] = [
  { label: 'Java', exts: ['.java'] },
  { label: '前端', exts: ['.js', '.jsx', '.ts', '.tsx', '.vue', '.css', '.scss', '.less'] },
  { label: '前端 JS/TS', exts: ['.js', '.jsx', '.ts', '.tsx', '.vue'] },
  { label: '样式', exts: ['.css', '.scss', '.less'] },
  { label: 'Go', exts: ['.go'] },
  { label: 'Python', exts: ['.py'] },
  { label: 'SQL', exts: ['.sql'] },
  { label: 'Shell 脚本', exts: ['.sh', '.bash', '.ps1'] },
  { label: 'C/C++', exts: ['.c', '.h', '.cpp', '.hpp'] },
  { label: 'Rust', exts: ['.rs'] },
  { label: '配置数据', exts: ['.json', '.yaml', '.yml', '.toml', '.xml', '.properties'] },
  { label: '文档', exts: ['.md', '.txt'] },
]

// The empty selection means "全部后缀 (*.*)": every text file is searched.
export const EXT_FILTER_ALL = ''

export interface ExtFilterOption {
  value: string
  label: string
}

export const EXT_FILTER_OPTIONS: ExtFilterOption[] = [
  { value: EXT_FILTER_ALL, label: '全部后缀 (*.*)' },
  ...EXT_FILTER_GROUPS.map((g) => ({
    value: g.label,
    label: `${g.label} (${g.exts.map((e) => `*${e}`).join(' ')})`,
  })),
]

// resolveExtFilter maps the current selection onto dotted extensions for the
// backend (empty selection → undefined, i.e. every text file is searched).
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

// normalizeCustomExt turns a raw custom-ext draft ("vue", ".vue", "*.vue",
// "kt,json") onto the canonical selection string (dotted extensions joined by
// commas, e.g. ".vue" or ".kt,.json"). "*" / "*.*" / "all" mean the unfiltered
// selection (EXT_FILTER_ALL); junk returns undefined.
export function normalizeCustomExt(raw: string): string | undefined {
  const val = raw.trim().toLowerCase()
  if (!val) return undefined
  if (val === '*' || val === '*.*' || val === 'all') return EXT_FILTER_ALL
  const tokens = Array.from(
    new Set(
      val
        .split(/[,;\s]+/)
        .map((tok) => tok.trim().replace(/^\*+\.?/, '').replace(/^\.+/, ''))
        .filter((tok) => tok !== '' && tok !== '*'),
    ),
  )
  if (tokens.length === 0) return undefined
  if (tokens.some((tok) => !/^[a-z0-9][a-z0-9+_-]*$/.test(tok))) return undefined
  return tokens.map((tok) => `.${tok}`).join(',')
}

// extSelectionLabel renders a selection as the compact chip label: the all
// entry shows "*.*", a single-extension group shows "*.java", a multi-group
// shows its name, and a custom selection shows its first extension plus a
// "+N" remainder.
export function extSelectionLabel(selection: string): string {
  const sel = selection.trim()
  if (!sel) return '*.*'
  const group = EXT_FILTER_GROUPS.find((g) => g.label === sel)
  if (group) return group.exts.length === 1 ? `*${group.exts[0]}` : group.label
  const tokens = resolveExtFilter(sel)
  if (!tokens || tokens.length === 0) return '*.*'
  if (tokens.length === 1) return `*${tokens[0]}`
  return `*${tokens[0]} +${tokens.length - 1}`
}
