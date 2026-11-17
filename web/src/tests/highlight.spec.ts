import { describe, expect, it } from 'vitest'
import {
  detectLanguage,
  escapeHtml,
  highlightCode,
  splitHighlightedHtmlIntoLines,
} from '../shared/highlight'

describe('highlight utilities', () => {
  it('escapes special characters', () => {
    expect(escapeHtml('<div>"a" & \'b\'</div>')).toBe('&lt;div&gt;&quot;a&quot; &amp; &#39;b&#39;&lt;/div&gt;')
  })

  it('detects language from language name or extension', () => {
    expect(detectLanguage('ts')).toBe('typescript')
    expect(detectLanguage('typescript')).toBe('typescript')
    expect(detectLanguage('main.go')).toBe('go')
    expect(detectLanguage('App.vue')).toBe('xml')
    expect(detectLanguage('script.py')).toBe('python')
    expect(detectLanguage('styles.css')).toBe('css')
    expect(detectLanguage('config.yaml')).toBe('yaml')
    expect(detectLanguage('unknown.xyzabc')).toBeUndefined()
  })

  it('highlights code when language is supported', () => {
    const tsCode = 'const x: number = 42;'
    const highlighted = highlightCode(tsCode, 'typescript')
    expect(highlighted).toContain('class="hljs-keyword"')
    expect(highlighted).toContain('const')
    expect(highlighted).toContain('class="hljs-number"')
    expect(highlighted).toContain('42')
  })

  it('falls back to safe escaped text for unknown languages', () => {
    const raw = '<div>test & code</div>'
    const res = highlightCode(raw, 'nonexistent_lang')
    expect(res).toBe('&lt;div&gt;test &amp; code&lt;/div&gt;')
  })

  it('splits multiline highlighted code and maintains valid tags across lines', () => {
    const code = '/*\n * multi-line comment\n */\nconst a = 1;'
    const highlighted = highlightCode(code, 'javascript')
    const lines = splitHighlightedHtmlIntoLines(highlighted)

    expect(lines.length).toBe(4)
    expect(lines[0]).toContain('<span class="hljs-comment">/*</span>')
    expect(lines[1]).toContain('<span class="hljs-comment"> * multi-line comment</span>')
    expect(lines[2]).toContain('<span class="hljs-comment"> */</span>')
    expect(lines[3]).toContain('<span class="hljs-keyword">const</span>')
  })
})
