import type { SessionHistory } from '../../../api/sessions'

export interface ParsedSessionMention {
  fullMatch: string
  title: string
  sessionId: string
  index: number
}

/**
 * Formats a session mention into a markdown-like token.
 */
export function formatSessionMention(title: string, sessionId: string): string {
  const safeTitle = (title || '未命名会话')
    .replace(/\\/g, '\\\\')
    .replace(/\[/g, '\\[')
    .replace(/\]/g, '\\]')
  return `@[${safeTitle}](session:${sessionId}) `
}

/**
 * Finds all session mention tokens within the input text.
 */
export function parseSessionMentions(text: string): ParsedSessionMention[] {
  if (!text.includes('@[')) return []
  const pattern = /@\[((?:\\[\\\[\]]|[^\]\\\r\n])*)\]\(session:([^)\r\n]+)\)/g
  const matches: ParsedSessionMention[] = []

  let match: RegExpExecArray | null
  while ((match = pattern.exec(text)) !== null) {
    matches.push({
      fullMatch: match[0],
      title: match[1].replace(/\\([\\\[\]])/g, '$1'),
      sessionId: match[2],
      index: match.index,
    })
  }

  return matches
}

type MentionHistoryEntry = { kind?: string; role?: string; text?: string }

// historyEntries accepts the current event-shaped history and the older
// entries-shaped payload some clients still send.
function historyEntries(history: any): MentionHistoryEntry[] {
  return (
    history?.entries ||
    (history?.events || [])
      .filter((e: any) => e.type === 'message')
      .map((e: any) => ({
        role: e.message?.role,
        text: e.message?.text,
      }))
  )
}

function relevantCount(entries: MentionHistoryEntry[]): number {
  return entries.filter((e) => e.kind === 'user' || e.kind === 'agent' || e.role === 'user' || e.role === 'assistant').length
}

/**
 * Resolves session mentions in the user text, fetches referenced histories,
 * rewrites references into natural titles, and appends reference context blocks.
 */
export interface SessionHistoryFetchOptions {
  limit?: number
  before?: string
  includeImages?: boolean
}

export async function resolveSessionMentions(
  projectId: number,
  text: string,
  fetchHistory: (sessionId: string, options?: SessionHistoryFetchOptions) => Promise<SessionHistory>,
): Promise<string> {
  const mentions = parseSessionMentions(text)
  if (mentions.length === 0) return text

  const sessionContexts: string[] = []
  const seenSessions = new Set<string>()

  for (const mention of mentions) {
    if (seenSessions.has(mention.sessionId)) continue
    seenSessions.add(mention.sessionId)

    try {
      // A mention only needs the last few messages, not the whole transcript.
      // Start from a bounded tail and walk backwards only when the window did
      // not contain enough messages.
      let history = await fetchHistory(mention.sessionId, {
        limit: 100,
        includeImages: false,
      })
      let entries = historyEntries(history)
      for (let page = 1; page < 3 && relevantCount(entries) < 4 && history?.hasMore && history?.nextCursor; page += 1) {
        try {
          history = await fetchHistory(mention.sessionId, {
            limit: 100,
            before: history.nextCursor,
            includeImages: false,
          })
          entries = [...historyEntries(history), ...entries]
        } catch {
          break
        }
      }

      if (entries.length > 0) {
        // Extract up to the last 4 user/agent messages as summary context
        const relevantEntries = entries
          .filter((e) => e.kind === 'user' || e.kind === 'agent' || e.role === 'user' || e.role === 'assistant')
          .slice(-4)

        if (relevantEntries.length > 0) {
          const dialogue = relevantEntries
            .map((e) => `[${e.kind === 'user' || e.role === 'user' ? '用户' : 'Agent'}]: ${(e.text || '').trim()}`)
            .join('\n')

          sessionContexts.push(
            `<referenced_session id="${mention.sessionId}" title="${mention.title}">\n${dialogue}\n</referenced_session>`,
          )
        }
      }
    } catch {
      // If fetching fails, still keep basic mention indicator without breaking sending
      sessionContexts.push(
        `<referenced_session id="${mention.sessionId}" title="${mention.title}" unavailable="true" />`,
      )
    }
  }

  // Rewrite mentions backwards so character indexes remain valid
  let rewritten = text
  for (const mention of [...mentions].reverse()) {
    rewritten =
      rewritten.slice(0, mention.index) +
      `@"${mention.title}"` +
      rewritten.slice(mention.index + mention.fullMatch.length)
  }

  if (sessionContexts.length > 0) {
    return `${rewritten.trim()}\n\n---\n**引用会话上下文：**\n${sessionContexts.join('\n\n')}`
  }

  return rewritten
}
