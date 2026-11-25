// @vitest-environment node
//
// Guard for the Element Plus boundary (docs/architecture/frontend-ui-system.md).
//
// Two ways the migration quietly fails:
//
//   1. A screen reaches into the library's internals — `--el-*` variables or
//      `.el-*` selectors — from a feature component. The library's cascade then
//      owns that one widget, a theme switch stops reaching it, and the next
//      Element Plus upgrade breaks it silently. Every intervention belongs in
//      `src/theme/element-tokens.css`, which is loaded last and is the one place
//      the selectors are documented.
//
//   2. The `Ui*` primitives stop shrinking. They are being retired page by page
//      (§六 of the spec), and a migration that also writes new ones never ends.
//      So the reference count is a ratchet: it may go down, never up.
//
// The ratchet is deliberately coarse (a plain count of `Ui*` mentions, imports
// and tags both). It is not measuring style, only direction. When a page
// migrates, lower the constant and never raise it again; raising it is a
// decision that should be visible in a diff.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const webRoot = fileURLToPath(new URL('../..', import.meta.url))
const srcRoot = join(webRoot, 'src')

// Measured at each migration step: 859 when Element Plus landed, 836 after the
// AI 提示词 page, 808 after the AI 助手 page, 798 after the AI 员工 page, 750
// after the AI 团队 page, 745 after the organization centre's last `UiAlert`
// banner (a failure now reports once, through `ElMessage`, instead of once in a
// toast and once in a banner that pushes the table down), 630 after the engines
// centre (sidebar el-menu, the three engine panels, EngineStepControl, the
// Agent 配置 page and its embedded BuiltinDataPanel / AgentUsagePanel). Lower
// it as pages move; see the
// header note.
const UI_PRIMITIVE_BASELINE = 630

// The primitives themselves are obviously allowed to mention their own names.
const EXCLUDED_DIRS = new Set(['ui', 'tests', 'fonts', 'node_modules'])

function collect(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      if (!EXCLUDED_DIRS.has(entry)) collect(full, out)
      continue
    }
    if (/\.(vue|ts|css)$/.test(entry)) out.push(full)
  }
  return out
}

const roots = [join(srcRoot, 'features'), join(srcRoot, 'shared')].filter((dir) => {
  try {
    return statSync(dir).isDirectory()
  } catch {
    return false
  }
})

const sources = roots.flatMap((root) => collect(root)).map((file) => ({
  file: relative(srcRoot, file).replace(/\\/g, '/'),
  text: readFileSync(file, 'utf8'),
}))

function lines(text: string, pattern: RegExp): number[] {
  return text
    .split('\n')
    .map((line, index) => ({ line, index }))
    .filter(({ line }) => pattern.test(line))
    .map(({ index }) => index + 1)
}

describe('Element Plus boundary', () => {
  it('scans the sources it is meant to police', () => {
    // An empty scan would make both assertions below vacuously true.
    expect(sources.length).toBeGreaterThan(50)
  })

  it('keeps library internals out of feature and shared components', () => {
    const offenders: string[] = []
    for (const { file, text } of sources) {
      for (const pattern of [/--el-[a-z0-9-]+/, /(^|[^-\w])\.el-[a-z]/]) {
        for (const line of lines(text, pattern)) {
          offenders.push(`${file}:${line}`)
        }
      }
    }
    expect(offenders).toEqual([])
  })

  it('does not add references to the retiring Ui* primitives', () => {
    const count = sources.reduce(
      (total, { text }) => total + [...text.matchAll(/\bUi[A-Z][A-Za-z]*\b/g)].length,
      0,
    )
    expect(count).toBeLessThanOrEqual(UI_PRIMITIVE_BASELINE)
  })
})
