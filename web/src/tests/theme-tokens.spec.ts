// Guard for the theme token layer: every CSS custom property the UI consumes
// through var() must be defined somewhere — the global stylesheet, a component's
// own <style> block, or the appearance layer that projects the palette and
// density onto the document root.
//
// A var() with no definition silently falls back to its ad-hoc literal (or to
// nothing), which is exactly how the tasks feature stayed dark on a light theme:
// it consumed --color-sidebar-bg / --color-card-bg / --color-input-bg, which the
// theme never defined, so the fallback colors won every time. This test makes
// that class of drift impossible to reintroduce.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, extname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const srcDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')

// walk collects the stylesheets that can define or consume tokens. The vendored
// font assets and the tests themselves carry none.
function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    if (entry === 'fonts' || entry === 'tests' || entry === 'node_modules') return []
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) return walk(full)
    return ['.css', '.vue'].includes(extname(full)) ? [full] : []
  })
}

const sources = walk(srcDir).map((file) => ({ file, text: readFileSync(file, 'utf8') }))
// apply.ts holds the theme palette and density: its quoted strings are the
// variable names the appearance projection writes at runtime.
const applySource = readFileSync(resolve(srcDir, 'theme/apply.ts'), 'utf8')

const DEFINED_IN_APPLY = /'--[a-z0-9-]+'/g
const DECLARATION = /--[a-z0-9-]+\s*:/g
const USAGE = /var\(\s*(--[a-z0-9-]+)/g

function names(text: string, pattern: RegExp): string[] {
  return [...text.matchAll(pattern)].map((match) =>
    (match[1] ?? match[0]).replace(/[':\s]/g, ''),
  )
}

const defined = new Set<string>([
  ...names(applySource, DEFINED_IN_APPLY),
  ...sources.flatMap(({ text }) => names(text, DECLARATION)),
])

const used = new Map<string, string>()
for (const { file, text } of sources) {
  for (const name of names(text, USAGE)) {
    if (!used.has(name)) used.set(name, file.replace(`${srcDir}/`, 'src/'))
  }
}

describe('theme tokens', () => {
  it('finds the declarations and the consumers it is meant to compare', () => {
    // Sanity check on the scanner itself: an empty scan would make the assertion
    // below vacuously true.
    expect(defined.size).toBeGreaterThan(20)
    expect(used.size).toBeGreaterThan(20)
  })

  it('defines every CSS custom property the UI consumes', () => {
    const undefinedTokens = [...used]
      .filter(([name]) => !defined.has(name))
      .map(([name, file]) => `${name} (used by ${file})`)
    expect(undefinedTokens).toEqual([])
  })
})
