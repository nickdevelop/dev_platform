// @vitest-environment node
//
// Guard for the platform dialog contract.
//
// Browser-native confirm/alert dialogs cannot follow the platform theme, add
// focus trapping, or show loading/error state. PROJECT.md requires every
// confirmation through UiConfirmDialog and every error through UiAlert, so this
// spec reads the real source tree and fails when a native dialog is introduced.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const webRoot = fileURLToPath(new URL('../..', import.meta.url))
const srcRoot = join(webRoot, 'src')

interface SourceFile {
  path: string
  lines: string[]
}

function collect(dir: string, out: SourceFile[] = []): SourceFile[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      if (full === join(srcRoot, 'tests')) continue
      collect(full, out)
      continue
    }
    if (!entry.endsWith('.ts') && !entry.endsWith('.vue')) continue
    out.push({
      path: relative(webRoot, full).split(sep).join('/'),
      lines: readFileSync(full, 'utf8').split('\n'),
    })
  }
  return out
}

const sources = collect(srcRoot)

const patterns = [
  { label: 'bare confirm()/alert()', re: /(?:^|[^.\w$])(confirm|alert)\s*\(/ },
  { label: 'window/globalThis dialog', re: /\b(?:window|globalThis)\.(confirm|alert|prompt)\s*\(/ },
]

function isComment(line: string): boolean {
  const trimmed = line.trim()
  return (
    trimmed.startsWith('//') ||
    trimmed.startsWith('*') ||
    trimmed.startsWith('/*') ||
    trimmed.startsWith('<!--')
  )
}

describe('native dialog guard', () => {
  it('scans a plausible number of sources', () => {
    expect(sources.length).toBeGreaterThan(50)
  })

  it('never calls browser-native confirm/alert/prompt dialogs', () => {
    const offences: string[] = []
    for (const file of sources) {
      file.lines.forEach((line, index) => {
        if (isComment(line)) return
        for (const pattern of patterns) {
          if (pattern.re.test(line)) {
            offences.push(`${file.path}:${index + 1}: ${pattern.label}: ${line.trim()}`)
          }
        }
      })
    }
    expect(offences).toEqual([])
  })
})
