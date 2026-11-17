// Agent replies and markdown documents are rendered as markdown, but diagram fences
// (```plantuml or ```mermaid) become interactive diagram components.
//
// The reply is cut into segments first: markdown runs go through renderMarkdown,
// while diagram fences become structured blocks the caller mounts a component for.

export type Segment =
  | { kind: 'markdown'; text: string }
  | { kind: 'plantuml'; text: string; closed: boolean }
  | { kind: 'mermaid'; text: string; closed: boolean }

const PLANTUML_LANGUAGES = new Set(['plantuml', 'puml', 'uml'])
const MERMAID_LANGUAGES = new Set(['mermaid', 'mmd'])

// A fence line: up to three leading spaces, three or more fence characters,
// then an optional info string.
const FENCE = /^ {0,3}(`{3,}|~{3,})[ \t]*(.*)$/

function diagramKind(line: string): 'plantuml' | 'mermaid' | null {
  const fence = FENCE.exec(line)
  if (!fence || !isFenceLine(fence)) return null
  const rawLang = fence[2].trim().split(/\s+/)[0]?.toLowerCase() ?? ''
  if (!rawLang) return null

  if (PLANTUML_LANGUAGES.has(rawLang)) return 'plantuml'
  if (MERMAID_LANGUAGES.has(rawLang)) return 'mermaid'

  // Support prefix notations like plantuml:sequence, puml-activity, mermaid:flowchart
  const baseLang = rawLang.split(/[:\-_]/)[0]
  if (PLANTUML_LANGUAGES.has(baseLang)) return 'plantuml'
  if (MERMAID_LANGUAGES.has(baseLang)) return 'mermaid'

  return null
}

function isFenceLine(fence: RegExpExecArray): boolean {
  return !(fence[1][0] === '`' && fence[2].includes('`'))
}

function closesFence(line: string, char: string, length: number): boolean {
  const fence = FENCE.exec(line)
  return (
    fence !== null && fence[1][0] === char && fence[1].length >= length && fence[2].trim() === ''
  )
}

function lineBounds(source: string, from: number): { content: number; next: number } {
  const newline = source.indexOf('\n', from)
  const content = newline === -1 ? source.length : newline
  return { content, next: content + 1 }
}

function closingFenceStart(source: string, open: number, char: string, length: number): number {
  const firstBodyLine = lineBounds(source, open).next
  for (let scan = firstBodyLine; scan < source.length; ) {
    const candidate = lineBounds(source, scan)
    if (closesFence(source.slice(scan, candidate.content), char, length)) return scan
    scan = candidate.next
  }
  return -1
}

export function splitDiagramBlocks(source: string): Segment[] {
  const text = source.includes('\r') ? source.replace(/\r\n?/g, '\n') : source
  if (text === '') return []

  const segments: Segment[] = []
  let cursor = 0
  let markdownStart = 0

  const flushMarkdown = function (end: number): void {
    if (end > markdownStart) segments.push({ kind: 'markdown', text: text.slice(markdownStart, end) })
  }

  while (cursor < text.length) {
    const bounds = lineBounds(text, cursor)
    const line = text.slice(cursor, bounds.content)
    const fence = FENCE.exec(line)
    if (!fence || !isFenceLine(fence)) {
      cursor = bounds.next
      continue
    }

    const char = fence[1][0]
    const length = fence[1].length
    const closing = closingFenceStart(text, cursor, char, length)
    const kind = diagramKind(line)

    if (!kind) {
      cursor = closing === -1 ? text.length : lineBounds(text, closing).next
      continue
    }

    const bodyEnd = closing === -1 ? text.length : closing
    const body = text.slice(bounds.next, bodyEnd)

    if (closing !== -1 && body.trim() === '') {
      cursor = lineBounds(text, closing).next
      continue
    }

    flushMarkdown(cursor)
    segments.push({
      kind,
      text: closing === -1 ? body : body.replace(/\n$/, ''),
      closed: closing !== -1,
    })

    cursor = closing === -1 ? text.length : lineBounds(text, closing).content
    markdownStart = cursor
  }

  flushMarkdown(text.length)
  return segments
}
