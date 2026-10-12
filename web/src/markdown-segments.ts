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

export function splitDiagramBlocks(source: string): Segment[] {
  if (source === '') return []

  const segments: Segment[] = []
  let cursor = 0
  let markdownStart = 0

  const flushMarkdown = function (end: number): void {
    if (end > markdownStart) segments.push({ kind: 'markdown', text: source.slice(markdownStart, end) })
  }

  while (cursor < source.length) {
    const bounds = lineBounds(source, cursor)
    const line = source.slice(cursor, bounds.content)
    if (!diagramTag(line)) {
      cursor = bounds.next
      continue
    }

    const fence = FENCE.exec(line)!
    const body = bounds.next
    let open = true
    let bodyEnd = source.length
    let after = source.length
    for (let scan = body; scan < source.length; ) {
      const candidate = lineBounds(source, scan)
      if (closesFence(source.slice(scan, candidate.content), fence[1][0], fence[1].length)) {
        open = false
        bodyEnd = scan
        // Stop in front of the newline that ended the closing fence, so that
        // newline stays with the segment that follows and nothing but the fence
        // lines is lost.
        after = candidate.content
        break
      }
      scan = candidate.next
    }

    flushMarkdown(cursor)
    // A closed fence drops the newline that ended its last body line; an open
    // one runs to the end of the reply and keeps everything it holds.
    const text = source.slice(body, bodyEnd)
    segments.push({
      kind: 'plantuml',
      text: open ? text : text.replace(/\n$/, ''),
      closed: !open,
    })
    cursor = after
    markdownStart = after
  }

  flushMarkdown(source.length)
  return segments
}
