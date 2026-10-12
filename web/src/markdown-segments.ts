// Agent replies are rendered as markdown, but a ```plantuml fence is not meant
// to be read as text: it becomes a diagram component (see components/
// PlantumlBlock.vue). marked cannot hand us that as a structured piece, so the
// reply is cut into segments first: markdown runs go through renderMarkdown
// unchanged, diagram fences become blocks the caller mounts a component for.
//
// The cut happens before marked on purpose, which means a diagram fence nested
// inside a list item or a blockquote is lifted to the top level. The spec
// (docs/superpowers/specs/2026-10-12-plantuml-preview-design.md §1) accepts
// that: a fence inside a list item is rare in chat, and the alternative —
// mounting components into already-rendered HTML — costs more than it buys.
//
// Segments lose nothing except the fence lines themselves: the newline that
// ended the closing fence stays at the head of the segment that follows, so
// joining every `text` returns the reply with exactly the diagram fences cut
// out of it.

export type Segment =
  | { kind: 'markdown'; text: string }
  | { kind: 'plantuml'; text: string; closed: boolean }

// Languages that mean "this is a diagram". Both spellings show up in agent
// output.
const DIAGRAM_LANGUAGES = ['plantuml', 'puml']

// A fence line: up to three leading spaces, three or more fence characters,
// then an optional info string.
const FENCE = /^ {0,3}(`{3,}|~{3,})[ \t]*(.*)$/

// The language tag of a diagram fence line, or null when the line is not one.
function diagramTag(line: string): boolean {
  const fence = FENCE.exec(line)
  if (!fence) return false
  const language = fence[2].trim().split(/\s+/)[0]?.toLowerCase() ?? ''
  return DIAGRAM_LANGUAGES.includes(language)
}

// The closing fence repeats the opening character, is at least as long, and
// carries no info string.
function closesFence(line: string, char: string, length: number): boolean {
  const fence = FENCE.exec(line)
  return fence !== null && fence[1][0] === char && fence[1].length >= length && fence[2].trim() === ''
}

// The bounds of one line starting at `from`: where its content ends and where
// the next line starts (the end of the source when there is no newline).
function lineBounds(source: string, from: number): { content: number; next: number } {
  const newline = source.indexOf('\n', from)
  const content = newline === -1 ? source.length : newline
  return { content, next: content + 1 }
}

// Where the fence opened at `open` ends: the start of its closing fence line,
// or -1 when it is never closed and therefore runs to the end of the reply.
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
  // marked normalizes line endings; doing it here keeps one grammar for the
  // fence scanner, so a reply that arrives over CRLF still gets its diagrams.
  const text = source.includes('\r') ? source.replace(/\r\n/g, '\n') : source
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
    if (!fence) {
      cursor = bounds.next
      continue
    }

    const char = fence[1][0]
    const length = fence[1].length
    const closing = closingFenceStart(text, cursor, char, length)
    // Everything a fence that is not a diagram holds — including a plantuml
    // fence quoted as an example inside it — stays markdown, and scanning
    // resumes after it. An unclosed one takes the rest of the reply with it.
    if (!diagramTag(line)) {
      cursor = closing === -1 ? text.length : lineBounds(text, closing).next
      continue
    }

    const bodyEnd = closing === -1 ? text.length : closing
    const body = text.slice(bounds.next, bodyEnd)
    // A diagram fence with nothing in it is not a diagram: leave it to markdown
    // rather than mount a block that has nothing to draw or read.
    if (closing !== -1 && body.trim() === '') {
      cursor = lineBounds(text, closing).next
      continue
    }

    flushMarkdown(cursor)
    // A closed fence drops the newline that ended its last body line; an open
    // one runs to the end of the reply and keeps everything it holds.
    segments.push({
      kind: 'plantuml',
      text: closing === -1 ? body : body.replace(/\n$/, ''),
      closed: closing !== -1,
    })
    // Stop in front of the newline that ended the closing fence, so that newline
    // stays with the segment that follows and nothing but the fence lines is
    // lost.
    cursor = closing === -1 ? text.length : lineBounds(text, closing).content
    markdownStart = cursor
  }

  flushMarkdown(text.length)
  return segments
}
