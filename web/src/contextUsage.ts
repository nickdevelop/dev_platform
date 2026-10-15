// Composer context line: exact backend anchor plus a trailing estimate.
//
// The backend reports exact per-turn token usage on final assistant messages
// (and opencode turn finishes); anything after the newest anchor — the turn
// in flight — is estimated with the same chars/4 heuristic pi uses for its
// own trailing tokens. estimated is true exactly when any part of used is
// estimated, and the view prefixes the figure with ~ and an 估算 tag then.
import type { MessageUsage } from './api/sessions'
import type { ChatEntry } from './stores/sessions'

export interface UsageParts {
  input: number
  output: number
  reasoning: number
  cacheRead: number
  cacheWrite: number
  total: number
}

export interface ContextUsage {
  used: number
  limit?: number
  // Percent is present only when the model reports a context window; without
  // one the view shows the used figure alone.
  percent?: number
  estimated: boolean
  parts: UsageParts
}

function emptyParts(): UsageParts {
  return { input: 0, output: 0, reasoning: 0, cacheRead: 0, cacheWrite: 0, total: 0 }
}

function partsOf(usage: MessageUsage): UsageParts {
  const parts: UsageParts = {
    input: usage.input,
    output: usage.output,
    reasoning: usage.reasoning ?? 0,
    cacheRead: usage.cacheRead ?? 0,
    cacheWrite: usage.cacheWrite ?? 0,
    total: 0,
  }
  parts.total = parts.input + parts.output + parts.reasoning + parts.cacheRead + parts.cacheWrite
  return parts
}

// estimateTokens approximates one entry's tokens from its text length.
// Mirrors pi's chars/4 heuristic; images and non-text payloads are out of
// scope for the composer line and intentionally uncounted.
export function estimateTokens(text: string): number {
  if (text.length === 0) return 0
  return Math.ceil(text.length / 4)
}

// computeContextUsage anchors on the freshest exact usage — an explicit
// turn-level report first, else the newest assistant message carrying usage —
// and estimates everything after it. limit is the selected model's context
// window; without one no percentage is derived.
export function computeContextUsage(
  entries: ChatEntry[],
  turnUsage: MessageUsage | undefined,
  limit: number | undefined,
): ContextUsage {
  let anchor: UsageParts | null = null
  let anchorIndex = -1
  if (turnUsage && partsOf(turnUsage).total > 0) {
    anchor = partsOf(turnUsage)
    anchorIndex = entries.length
  } else {
    for (let i = entries.length - 1; i >= 0; i -= 1) {
      const entry = entries[i]
      if (entry.kind === 'message' && entry.role === 'assistant' && entry.usage) {
        const parts = partsOf(entry.usage)
        if (parts.total > 0) {
          anchor = parts
          anchorIndex = i
          break
        }
      }
    }
  }

  const parts = anchor ?? emptyParts()
  let estimated = anchor === null
  for (let i = anchorIndex + 1; i < entries.length; i += 1) {
    const entry = entries[i]
    const tokens = estimateTokens(entry.text)
    if (tokens === 0) continue
    estimated = true
    // Trailing content becomes the next prompt's input, except reasoning
    // and in-flight assistant text which read as output.
    if (entry.kind === 'thinking') {
      parts.reasoning += tokens
    } else if (entry.kind === 'message' && entry.role === 'assistant') {
      parts.output += tokens
    } else {
      parts.input += tokens
    }
    parts.total += tokens
  }

  const usage: ContextUsage = { used: parts.total, estimated, parts }
  if (limit !== undefined && Number.isFinite(limit) && limit > 0) {
    usage.limit = limit
    usage.percent = Math.round((parts.total / limit) * 100)
  }
  return usage
}
