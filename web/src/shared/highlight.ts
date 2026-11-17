// Syntax highlighting support using highlight.js common languages.
import hljs from 'highlight.js/lib/common'

const EXTENSION_MAP: Record<string, string> = {
  ts: 'typescript',
  tsx: 'typescript',
  mts: 'typescript',
  cts: 'typescript',
  js: 'javascript',
  jsx: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  vue: 'xml',
  html: 'xml',
  htm: 'xml',
  svg: 'xml',
  xml: 'xml',
  css: 'css',
  scss: 'scss',
  less: 'less',
  json: 'json',
  yaml: 'yaml',
  yml: 'yaml',
  md: 'markdown',
  markdown: 'markdown',
  py: 'python',
  pyw: 'python',
  go: 'go',
  rs: 'rust',
  java: 'java',
  c: 'c',
  h: 'c',
  cpp: 'cpp',
  hpp: 'cpp',
  cc: 'cpp',
  cxx: 'cpp',
  cs: 'csharp',
  sh: 'bash',
  bash: 'bash',
  zsh: 'bash',
  sql: 'sql',
  diff: 'diff',
  patch: 'diff',
  lua: 'lua',
  rb: 'ruby',
  php: 'php',
  kt: 'kotlin',
  kts: 'kotlin',
  swift: 'swift',
  ini: 'ini',
  conf: 'ini',
  env: 'ini',
  makefile: 'makefile',
  mk: 'makefile',
}

export function escapeHtml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

export function detectLanguage(nameOrPath?: string): string | undefined {
  if (!nameOrPath) return undefined
  const clean = nameOrPath.trim().toLowerCase()
  if (EXTENSION_MAP[clean]) return EXTENSION_MAP[clean]
  const ext = clean.replace(/\\/g, '/').split('/').pop()?.split('.').pop()
  if (ext && EXTENSION_MAP[ext]) return EXTENSION_MAP[ext]
  if (hljs.getLanguage(clean)) return clean
  return undefined
}

export function highlightCode(code: string, languageOrExt?: string): string {
  if (!code) return ''
  const lang = detectLanguage(languageOrExt)
  if (lang) {
    try {
      return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value
    } catch {
      // Fallback to escaping on any highlighting error
    }
  }
  return escapeHtml(code)
}

// Split an HTML string from highlightCode into safe, self-contained lines
// by ensuring any open <span> tags are closed at end of line and reopened on the next.
export function splitHighlightedHtmlIntoLines(html: string): string[] {
  if (!html) return []
  const lines: string[] = []
  const openTags: string[] = []
  let currentLine = ''
  let i = 0

  while (i < html.length) {
    if (html[i] === '\n') {
      let closed = currentLine
      for (let j = openTags.length - 1; j >= 0; j--) {
        closed += '</span>'
      }
      lines.push(closed)
      let reopened = ''
      for (let j = 0; j < openTags.length; j++) {
        reopened += openTags[j]
      }
      currentLine = reopened
      i++
      continue
    }

    if (html.startsWith('<span', i)) {
      const closeIdx = html.indexOf('>', i)
      if (closeIdx !== -1) {
        const tag = html.slice(i, closeIdx + 1)
        openTags.push(tag)
        currentLine += tag
        i = closeIdx + 1
        continue
      }
    } else if (html.startsWith('</span>', i)) {
      openTags.pop()
      currentLine += '</span>'
      i += 7
      continue
    }

    currentLine += html[i]
    i++
  }

  lines.push(currentLine)
  return lines
}
