// Tests for the built-in fonts. The platform bundles two Google Fonts families
// — Google Sans Code for everything except Chinese, Noto Sans SC for Chinese —
// ships them from its own origin, and gives the operator no face to choose.
import { existsSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// The font sources live next to the code, so they are addressed from disk by
// path: the assertions then run against what the build consumes, not against a
// build artifact.
const srcDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const fontsCSSPath = resolve(srcDir, 'fonts/fonts.css')

function read(...parts: string[]): string {
  return readFileSync(resolve(srcDir, ...parts), 'utf8')
}

const styleCSS = read('style.css')
const fontsCSS = read('fonts/fonts.css')

describe('built-in fonts', () => {
  it('loads the bundled stylesheet from the global stylesheet', () => {
    expect(styleCSS).toContain("@import './fonts/fonts.css'")
  })

  it('puts Google Sans Code first so non-Chinese text is monospaced', () => {
    const stack = (/--font-family:\s*([^;]+);/ as RegExp).exec(styleCSS)?.[1] ?? ''
    const families = stack
      .replace(/\s+/g, ' ')
      .split(',')
      .map((entry) => entry.trim().replace(/['"]/g, ''))
    expect(families[0]).toBe('Google Sans Code')
    expect(families[1]).toBe('Noto Sans SC')
  })

  it('declares @font-face rules for both bundled families', () => {
    expect(fontsCSS).toContain("font-family: 'Google Sans Code'")
    expect(fontsCSS).toContain("font-family: 'Noto Sans SC'")
  })

  it('slices the fonts by unicode-range so only the used subsets download', () => {
    const faces = fontsCSS.match(/@font-face\s*{[^}]*}/g) ?? []
    expect(faces.length).toBeGreaterThan(100)
    expect(faces.every((face) => face.includes('unicode-range:'))).toBe(true)
  })

  it('ships every declared font file from the build itself', () => {
    const urls = [...fontsCSS.matchAll(/url\(([^)]+)\)/g)].map((match) =>
      match[1].replace(/['"]/g, ''),
    )
    expect(urls.length).toBeGreaterThan(100)
    for (const url of urls) {
      expect(url).not.toMatch(/^https?:\/\//)
      const declared = resolve(dirname(fontsCSSPath), url)
      expect(existsSync(declared), `missing font file for ${url}`).toBe(true)
    }
  })

  it('never asks the platform to fetch a font from the network', () => {
    expect(read('main.ts')).not.toContain('fonts.googleapis.com')
    expect(read('theme/apply.ts')).not.toContain('fonts.googleapis.com')
  })
})
