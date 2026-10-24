// @vitest-environment node
//
// Guard for the CSRF transport contract.
//
// The whole point of get()/mutate() is that no call site can reach the network
// without them: a bare fetch(), a hand-written request or a locally stored
// token would bypass the injection that api/client.ts performs. Linting the
// sources is the only way to keep that property true for code that has not been
// written yet, so this spec reads the source tree and fails on any shortcut.
//
// It runs against the real files, not a fixture: adding an API module with a
// hand-rolled request fails here before it can ship.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// web/ is two levels up from src/tests.
const webRoot = fileURLToPath(new URL('../..', import.meta.url))
const srcRoot = join(webRoot, 'src')
const clientModule = join('src', 'api', 'client.ts')
const authStore = join('src', 'stores', 'auth.ts')
const compositionRoot = join('src', 'main.tsx')
// The PlantUML renderer talks to an operator-configured external diagram
// server, never to this platform's /api: it sends no cookies and is not part of
// the CSRF boundary.
const plantumlRenderer = join('src', 'plantuml', 'renderer.ts')

interface SourceFile {
  // Path relative to web/, always with forward slashes.
  path: string
  lines: string[]
}

function collect(dir: string, out: SourceFile[] = []): SourceFile[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      // Tests are allowed to stub fetch and to mention the token: they are not
      // production call sites.
      if (full === join(srcRoot, 'tests')) continue
      collect(full, out)
      continue
    }
    if (!entry.endsWith('.ts') && !entry.endsWith('.tsx') && !entry.endsWith('.jsx')) continue
    out.push({
      path: relative(webRoot, full).split(sep).join('/'),
      lines: readFileSync(full, 'utf8').split('\n'),
    })
  }
  return out
}

const sources = collect(srcRoot)

function offences(pattern: RegExp, allowed: string[]): string[] {
  const found: string[] = []
  for (const file of sources) {
    if (allowed.includes(file.path)) continue
    file.lines.forEach((line, index) => {
      if (pattern.test(line)) found.push(`${file.path}:${index + 1}: ${line.trim()}`)
    })
  }
  return found
}

describe('CSRF transport coverage', () => {
  it('scans a plausible number of sources', () => {
    // Guards the walker itself: an empty walk would make every check below pass
    // vacuously.
    expect(sources.length).toBeGreaterThan(50)
  })

  it('reaches the network only through the transport', () => {
    expect(offences(/\bfetch\s*\(/, [clientModule, plantumlRenderer])).toEqual([])
  })

  it('sets the CSRF header only in the transport', () => {
    expect(offences(/X-CSRF-Token/, [clientModule])).toEqual([])
  })

  it('keeps the token in the session store and the transport only', () => {
    expect(offences(/csrfToken/, [authStore, clientModule, compositionRoot])).toEqual([])
  })

  it('has no second request entry point', () => {
    expect(offences(/\bapiRequest\b/, [])).toEqual([])
  })

  it('sends only mutating verbs through mutate()', () => {
    const allowed = new Set(['POST', 'PUT', 'PATCH', 'DELETE'])
    const offenders: string[] = []
    for (const file of sources) {
      if (!file.path.startsWith('src/api/') || file.path === clientModule) continue
      const source = file.lines.join('\n')
      for (const match of source.matchAll(/mutate<[^>]*>\(\s*['"]([A-Z]+)['"]/g)) {
        if (!allowed.has(match[1])) offenders.push(`${file.path}: mutate('${match[1]}')`)
      }
    }
    expect(offenders).toEqual([])
  })
})
