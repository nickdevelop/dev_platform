// The session log's vocabulary and pure derivations, provider-neutral: what a
// rendered log line is, how a normalized event becomes one, and how an open
// streaming block is located. Kept free of the store so every consumer (the
// store's live path, history replay, the rendering feature) shares one model.

import type {
  AgentSession,
  ImageAttachment,
  MessageModel,
  MessageUsage,
  NormalizedEvent,
} from '../../../api/sessions'

// One rendered line in a session's log. Agent replies are rendered as sanitized
// markdown in the view (see ../shared/markdown); every other kind is plain text.
// A `thinking` line is the agent's reasoning, which the pane shows collapsed.
// Replayed history and the live stream share this log directly: there is no
// separator between them, so a resumed turn appends to the same blocks.
export interface ChatEntry {
  id: string
  kind: 'message' | 'thinking' | 'tool_call' | 'tool_result' | 'error' | 'notice'
  role: string
  text: string
  images?: ImageAttachment[]
  streaming: boolean
  // The model that produced an assistant message, when the backend reports
  // one. Absent on user messages and on backends without per-message models.
  model?: MessageModel
  // Exact token usage of a final assistant message, when the backend reports
  // it. The context line anchors on the newest such entry; streaming chunks
  // never carry it.
  usage?: MessageUsage
  callId?: string
  name?: string
  // Tool calls carry the raw argument payload so the view can summarise it
  // (name + representative argument) without the store knowing tool schemas.
  input?: unknown
  isError?: boolean
  // Normalized edit summary for file tools (edit): the desktop-style
  // badge/body render from this rather than parsing output text. Absent on
  // non-edit tools, failed results without diffs, and old transcripts.
  diff?: { file: string; patch?: string; additions: number; deletions: number }
}

export type StreamStatus = 'idle' | 'connecting' | 'open' | 'reconnecting' | 'error' | 'ended'

let entryCounter = 0

export function nextEntryID(sessionId: string, sequence: number, kind: string): string {
  entryCounter += 1
  return `${sessionId}:${sequence}:${kind}:${entryCounter}`
}

// findOpenStreaming returns the most recent still-streaming entry of a kind
// (and, for messages, of the same role), scanning past anything appended in
// between. Backends interleave their streams — pi emits a thinking_delta in
// the middle of a running text delta and closes the thinking block after text
// resumed — so the open block, not the last list position, is what a delta
// belongs to. Without this, interleaving fragments a block into several
// entries and its terminator opens an empty one.
export function findOpenStreaming(
  list: ChatEntry[],
  kind: ChatEntry['kind'],
  role?: string,
): ChatEntry | undefined {
  for (let i = list.length - 1; i >= 0; i -= 1) {
    const entry = list[i]
    if (entry.kind === kind && entry.streaming && (role === undefined || entry.role === role)) {
      return entry
    }
  }
  return undefined
}

// keepSettledTurn merges an interrupt's response into the entry the live stream
// may have settled first. The backend answers from the session row as it stood
// when the interrupt was accepted; the stream reports the interruption as soon
// as the agent stops. When the stream already settled the turn, the entry keeps
// it settled — an interrupt can only end a turn, never restart one.
export function keepSettledTurn(current: AgentSession, incoming: AgentSession): AgentSession {
  if (current.turn !== 'running' && incoming.turn === 'running') {
    return { ...incoming, turn: current.turn }
  }
  return incoming
}

export function firstChangedFile(metadata: unknown): ChatEntry['diff'] {
  if (!metadata || typeof metadata !== 'object') return undefined
  const files = (metadata as { files?: unknown }).files
  if (!Array.isArray(files)) return undefined
  for (const item of files) {
    if (!item || typeof item !== 'object') continue
    const file = item as Record<string, unknown>
    if (typeof file.file !== 'string' || file.file === '') continue
    if (typeof file.additions !== 'number' || typeof file.deletions !== 'number') continue
    if (file.additions <= 0 && file.deletions <= 0) continue
    if (typeof file.patch !== 'string' && file.patch !== undefined) continue
    return {
      file: file.file,
      patch: typeof file.patch === 'string' ? file.patch : undefined,
      additions: file.additions,
      deletions: file.deletions,
    }
  }
  return undefined
}

// entryFromEvent maps one non-message event to its log line, or null when the
// event renders nothing (lifecycle events are handled separately because they
// carry no log line of their own). Both the live path and the history replay
// go through it, so a replayed tool call looks exactly like a live one.
export function entryFromEvent(sessionId: string, event: NormalizedEvent): ChatEntry | null {
  switch (event.type) {
    case 'tool_call':
      if (!event.toolCall) return null
      return {
        id: nextEntryID(sessionId, event.sequence, 'tool_call'),
        kind: 'tool_call',
        role: 'assistant',
        text: event.toolCall.name,
        streaming: false,
        callId: event.toolCall.callId,
        name: event.toolCall.name,
        input: event.toolCall.input,
      }
    case 'tool_result':
      if (!event.toolResult) return null
      return {
        id: nextEntryID(sessionId, event.sequence, 'tool_result'),
        kind: 'tool_result',
        role: 'tool',
        text: event.toolResult.output ?? '',
        streaming: false,
        callId: event.toolResult.callId,
        isError: event.toolResult.isError,
        diff: firstChangedFile(event.toolResult.metadata),
      }
    case 'error':
      if (!event.error) return null
      return {
        id: nextEntryID(sessionId, event.sequence, 'error'),
        kind: 'error',
        role: 'system',
        text: event.error.message,
        streaming: false,
      }
    default:
      return null
  }
}

// sessionUsedModel returns the model a session actually ran, taken from its most
// recent assistant message that reported one. '' when the transcript has no such
// message yet — a brand-new session, or a backend that records no per-message
// model (pi) — so callers can fall back to another default.
export function sessionUsedModel(list: ChatEntry[]): string {
  for (let i = list.length - 1; i >= 0; i -= 1) {
    const entry = list[i]
    if (entry.kind === 'message' && entry.role === 'assistant' && entry.model?.id) {
      return entry.model.id
    }
  }
  return ''
}
