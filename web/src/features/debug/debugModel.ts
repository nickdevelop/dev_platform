// Pure logic for the SSE bus debug panel. No Vue and no store dependencies so
// every function is directly unit-testable (the rendering layer consumes these
// helpers, mirroring the chat rendering/logModel.ts pattern).

import type { EventEnvelope } from '../../api/events'

// Ring-buffer capacity: beyond this the oldest captured frames are evicted and
// counted, so a high-throughput stream can never grow the panel unbounded.
export const DEBUG_BUFFER_LIMIT = 500

export interface DebugEntry {
  // Monotonic capture index for this page session. It is only a local list key
  // and expansion id; it is NOT the number shown in the list.
  seq: number
  // The bus envelope's own SSE id / sequence. Null when the wire frame carried
  // no id (some broadcast frames); the list then prints a dash.
  sequence: number | null
  // Date.now() at receive time.
  receivedAt: number
  channel: string
  targetId: string
  eventType: string
  // Raw payload reference; pretty-printing is deferred to render time.
  payload: unknown
}

// Identity strings surfaced as badges on the row: the envelope's target plus the
// well-known identifiers carried inside the domain payloads.
export interface DebugEntryIdentities {
  targetId: string
  sessionIds: string[]
  chatIds: string[]
  instanceIds: string[]
  backends: string[]
  runIds: string[]
}

export interface DebugFilter {
  // 'all' or a concrete channel (sessions/chat/tasks/system...).
  channel: string
  // Substring matched against targetId, eventType and payload identity fields.
  keyword: string
  // Substring matched against eventType (case-insensitive).
  eventType: string
}

export function emptyDebugFilter(): DebugFilter {
  return { channel: 'all', keyword: '', eventType: '' }
}

// createEntry freezes one envelope into a capture record. The payload is kept
// by reference — envelopes are never mutated after dispatch.
export function createEntry(seq: number, envelope: EventEnvelope): DebugEntry {
  return {
    seq,
    sequence: typeof envelope.id === 'number' && Number.isFinite(envelope.id) ? envelope.id : null,
    receivedAt: typeof envelope.timestamp === 'number' ? envelope.timestamp : Date.now(),
    channel: envelope.channel,
    targetId: envelope.targetId,
    eventType: envelope.eventType,
    payload: envelope.payload,
  }
}

// appendEntry implements the ring buffer: returns the prune count so the caller
// can render a dropped-frames counter without keeping any history beyond the limit.
export function appendEntry(buffer: DebugEntry[], entry: DebugEntry, limit = DEBUG_BUFFER_LIMIT): number {
  buffer.push(entry)
  let dropped = 0
  while (buffer.length > limit) {
    buffer.shift()
    dropped++
  }
  return dropped
}

const IDENTITY_KEYS = ['sessionId', 'instanceId', 'chatId', 'runId', 'backend', 'messageId', 'senderName'] as const

// extractIdentities walks the payload (depth <= 2) and collects identifier-like
// values for badges and keyword matching. Numbers and strings both count.
export function extractIdentities(entry: DebugEntry): DebugEntryIdentities {
  const sessionIds = new Set<string>()
  const chatIds = new Set<string>()
  const instanceIds = new Set<string>()
  const backends = new Set<string>()
  const runIds = new Set<string>()

  function collect(value: unknown, isRoot: boolean) {
    if (!value || typeof value !== 'object') return
    for (const [key, raw] of Object.entries(value as Record<string, unknown>)) {
      if (raw === null || raw === undefined) continue
      if (key === 'sessionId' && typeof raw === 'string' && raw) sessionIds.add(raw)
      else if (key === 'instanceId' && typeof raw === 'string' && raw) instanceIds.add(raw)
      else if (key === 'backend' && typeof raw === 'string' && raw) backends.add(raw)
      else if (key === 'chatId' && (typeof raw === 'number' || typeof raw === 'string')) {
        chatIds.add(String(raw))
      } else if (key === 'runId' && (typeof raw === 'number' || typeof raw === 'string')) {
        runIds.add(String(raw))
      }
      // Only dive one extra level: domain payloads are flat-ish and deeper
      // recursion buys noise (long text chunks) instead of identifiers.
      if (typeof raw === 'object' && isRoot) collect(raw, false)
    }
  }

  collect(entry.payload, true)

  return {
    targetId: entry.targetId,
    sessionIds: [...sessionIds],
    chatIds: [...chatIds],
    instanceIds: [...instanceIds],
    backends: [...backends],
    runIds: [...runIds],
  }
}

// matchTexts is the keyword engine: empty needle matches everything, otherwise
// all haystacks are searched case-insensitively for one hit.
function matchTexts(needle: string, haystacks: string[]): boolean {
  const trimmed = needle.trim().toLowerCase()
  if (!trimmed) return true
  return haystacks.some((text) => text.toLowerCase().includes(trimmed))
}

// matchesFilter is the AND of three independent dimensions:
//   1. channel: exact ('all' disables).
//   2. keyword: targetId + eventType + payload identity fields.
//   3. eventType: substring on the event type only.
export function matchesFilter(entry: DebugEntry, filter: DebugFilter): boolean {
  if (filter.channel && filter.channel !== 'all' && entry.channel !== filter.channel) return false

  const identities = extractIdentities(entry)
  const keywordHaystacks = [
    entry.targetId,
    entry.eventType,
    ...identities.sessionIds,
    ...identities.chatIds,
    ...identities.instanceIds,
    ...identities.backends,
    ...identities.runIds,
  ]
  if (!matchTexts(filter.keyword, keywordHaystacks)) return false

  return matchTexts(filter.eventType, [entry.eventType])
}

// filterEntries returns the visible subset in insertion (oldest-first) order.
export function filterEntries(entries: DebugEntry[], filter: DebugFilter): DebugEntry[] {
  if (filter.channel === 'all' && !filter.keyword.trim() && !filter.eventType.trim()) return entries
  return entries.filter((entry) => matchesFilter(entry, filter))
}

// channelCounts counts the current buffer per channel, plus the 'all' total.
// The channel tabs read these counts so a channel with a real zero prints 0
// instead of disappearing.
export function channelCounts(entries: ReadonlyArray<DebugEntry>): Record<string, number> {
  const counts: Record<string, number> = { all: entries.length }
  for (const entry of entries) {
    counts[entry.channel] = (counts[entry.channel] ?? 0) + 1
  }
  return counts
}

// A badge is one identity string shown on a row: session / chat / run /
// instance / backend. backend is flagged so the row can style it apart without
// hardcoding provider ids.
export interface DebugBadge {
  key: string
  value: string
  label: string
  backend: boolean
}

export interface DebugBadgeSet {
  badges: DebugBadge[]
  hidden: number
}

// identityBadges flattens the identity groups into display order and folds the
// tail into a count. The full values remain available in the expanded payload.
export function identityBadges(identity: DebugEntryIdentities, limit = 3): DebugBadgeSet {
  const badges: DebugBadge[] = []
  const push = (key: string, values: string[], prefix = '') => {
    for (const value of values) {
      badges.push({ key, value, label: prefix ? `${prefix} ${value}` : value, backend: false })
    }
  }

  push('sessionId', identity.sessionIds)
  push('chatId', identity.chatIds, 'chat')
  push('runId', identity.runIds, 'run')
  push('instanceId', identity.instanceIds, 'inst')
  for (const backend of identity.backends) {
    badges.push({ key: 'backend', value: backend, label: backend, backend: true })
  }

  const shown = badges.slice(0, Math.max(0, limit))
  return { badges: shown, hidden: Math.max(0, badges.length - shown.length) }
}

// formatClock renders receive time as local HH:MM:SS.mmm with tabular digits.
export function formatClock(timestamp: number): string {
  if (!Number.isFinite(timestamp)) return '--:--:--.---'
  const date = new Date(timestamp)
  const pad = (value: number, width = 2) => String(value).padStart(width, '0')
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}.${pad(date.getMilliseconds(), 3)}`
}

// prettyJSON formats a payload for the expanded row view; failures (cyclic or
// hostile payloads) degrade to a diagnostic string instead of throwing.
export function prettyJSON(payload: unknown): string {
  try {
    return JSON.stringify(payload, null, 2) ?? 'null'
  } catch {
    return '[无法序列化的载荷]'
  }
}
