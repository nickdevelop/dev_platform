// @vitest-environment node
//
// Guard for template attribute names.
//
// An attribute name may not contain whitespace, quotes, '<', '>', '/' or '='
// (DOM: "InvalidCharacterError" from setAttribute). A typo such as
//     :worktree-id="activeWorktreeId""
// closes the value early and leaves a lone quote as the attribute name. The
// compiler accepts it and emits `"\"": ""`; vue-tsc and the build stay green,
// and Chromium 158 silently sets the bogus attribute — so browser runs look
// fine. Chromium 132 (the Electron 34 desktop client) throws while mounting the
// component, which aborted the session file tree drawer and it never appeared.
//
// Nothing else reports this class of typo, so this spec reads the template of
// every SFC and fails on any attribute name the DOM would reject.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// web/ is two levels up from src/tests.
const webRoot = fileURLToPath(new URL('../..', import.meta.url))
const srcRoot = join(webRoot, 'src')

export interface AttributeOffence {
  // Source path relative to web/, always with forward slashes.
  path: string
  // 1-based line of the offending attribute inside the file.
  line: number
  tag: string
  name: string
}

// Characters the DOM rejects in an attribute name. Whitespace, '=' and '/' end
// a name during tokenizing, so only quotes and angle brackets can survive here;
// the whole set is kept so the rule reads as the spec states it.
const INVALID_NAME_CHAR = /[\s"'<>/=]/

function mask(text: string, pattern: RegExp): string {
  // Replace matched spans with spaces, keeping newlines so line numbers stay
  // valid for everything that follows.
  return text.replace(pattern, (match) => match.replace(/[^\n]/g, ' '))
}

function lineAt(text: string, offset: number, baseLine: number): number {
  let line = baseLine
  for (let i = 0; i < offset; i++) {
    if (text[i] === '\n') line++
  }
  return line
}

// Attribute names of every start tag in the given template text. Quoted values
// are skipped as a unit, so '>' inside v-if="a > b" does not end the tag and a
// quote inside a value is never mistaken for a name.
export function invalidAttributeNames(
  template: string,
  path = 'fixture.vue',
  baseLine = 1,
): AttributeOffence[] {
  const text = mask(mask(template, /<!--[\s\S]*?-->/g), /\{\{[\s\S]*?\}\}/g)
  const offences: AttributeOffence[] = []
  let i = 0
  while (i < text.length) {
    const lt = text.indexOf('<', i)
    if (lt < 0) break
    let j = lt + 1
    // Closing tags, doctypes and processing instructions carry no attributes.
    if (text[j] === '/' || text[j] === '!' || text[j] === '?') {
      const end = text.indexOf('>', j)
      i = end < 0 ? text.length : end + 1
      continue
    }
    const tagStart = j
    while (j < text.length && !/[\s/>]/.test(text[j])) j++
    const tag = text.slice(tagStart, j)
    while (j < text.length) {
      while (j < text.length && /\s/.test(text[j])) j++
      if (j >= text.length) break
      const char = text[j]
      if (char === '>') {
        j++
        break
      }
      // The slash of a self-closing tag is the only thing left between
      // attributes.
      if (char === '/') {
        j++
        continue
      }
      const nameStart = j
      while (j < text.length && !/[\s=/>]/.test(text[j])) j++
      const name = text.slice(nameStart, j)
      if (name === '') {
        j++
        continue
      }
      if (INVALID_NAME_CHAR.test(name)) {
        offences.push({ path, line: lineAt(text, nameStart, baseLine), tag, name })
      }
      while (j < text.length && /\s/.test(text[j])) j++
      if (text[j] !== '=') continue
      j++
      while (j < text.length && /\s/.test(text[j])) j++
      const quote = text[j]
      if (quote === '"' || quote === "'") {
        j++
        while (j < text.length && text[j] !== quote) j++
        j++
        continue
      }
      while (j < text.length && !/[\s>]/.test(text[j])) j++
    }
    i = j
  }
  return offences
}

// templateBlock returns the root <template> … </template> region of an SFC,
// counting nested <template> tags (slots, v-slot, v-if templates).
export function templateBlock(
  source: string,
): { text: string; line: number } | undefined {
  const open = source.indexOf('<template')
  if (open < 0) return undefined
  let depth = 0
  let i = open
  while (i < source.length) {
    if (source.startsWith('<template', i) && /[\s>]/.test(source[i + 9] ?? '>')) {
      depth++
      i += '<template'.length
      continue
    }
    if (source.startsWith('</template>', i)) {
      depth--
      i += '</template>'.length
      if (depth === 0) {
        return { text: source.slice(open, i), line: lineAt(source, open, 1) }
      }
      continue
    }
    i++
  }
  return undefined
}

interface SfcFile {
  path: string
  source: string
}

function collect(dir: string, out: SfcFile[] = []): SfcFile[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      if (full === join(srcRoot, 'tests')) continue
      collect(full, out)
      continue
    }
    if (!entry.endsWith('.vue')) continue
    out.push({
      path: relative(webRoot, full).split(sep).join('/'),
      source: readFileSync(full, 'utf8'),
    })
  }
  return out
}

const files = collect(srcRoot)

function offencesInRepo(): AttributeOffence[] {
  const found: AttributeOffence[] = []
  for (const file of files) {
    const block = templateBlock(file.source)
    if (!block) continue
    found.push(...invalidAttributeNames(block.text, file.path, block.line))
  }
  return found
}

describe('template attribute names', () => {
  it('flags the doubled quote that hid the desktop file tree', () => {
    const offences = invalidAttributeNames(`<FileSidebar\n  :worktree-id="activeWorktreeId""\n/>`)
    expect(offences.map((o) => o.name)).toEqual(['"'])
    expect(offences[0].line).toBe(2)
  })

  it('accepts dynamic, shorthand, modifier and multi-line attributes', () => {
    const template = `<FileSidebar
      :project-id="props.projectId"
      :session-id="props.sessionId"
      #footer="{ close }"
      @click.stop="go()"
      v-if="a > b && c"
      :class="{ 'is-open': open }"
      hidden
    />`
    expect(invalidAttributeNames(template)).toEqual([])
  })

  it('ignores comments and mustaches, which are not markup', () => {
    const template = `<!-- <div class="a"" /> -->
      <p>{{ '<div class="b"" />' }}</p>`
    expect(invalidAttributeNames(template)).toEqual([])
  })

  it('finds the template block of an SFC, nested templates included', () => {
    const sfc = `<script setup lang="ts">\nconst a = 1\n</script>\n\n<template>\n  <div>\n    <template v-if="a"><span /></template>\n  </div>\n</template>\n\n<style scoped>\n.x {}\n</style>\n`
    const block = templateBlock(sfc)
    expect(block?.text.startsWith('<template>')).toBe(true)
    expect(block?.text.endsWith('</template>')).toBe(true)
    expect(block?.line).toBe(5)
  })

  it('scans actual SFCs', () => {
    // A broken collector would make the repo-wide check below pass vacuously.
    expect(files.length).toBeGreaterThan(20)
    expect(files.some((file) => file.path.endsWith('features/chat/SessionChatPanel.vue'))).toBe(true)
  })

  it('every SFC template uses attribute names the DOM accepts', () => {
    const offences = offencesInRepo()
    const report = offences.map((o) => `${o.path}:${o.line} <${o.tag} ${o.name}>`)
    expect(report).toEqual([])
  })
})
