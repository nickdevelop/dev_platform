import type { AgentSession } from '../../../../../api/sessions'
import type { DropdownItem, DropdownSource, TriggerMatch } from '../types'
import { formatSessionMention } from '../../sessionMentionResolver'

export class SessionMentionSource implements DropdownSource {
  readonly id = 'session-mention'
  readonly trigger = '@' as const

  constructor(
    private readonly getSessions: () => AgentSession[],
    private readonly getCurrentSessionId: () => string,
  ) {}

  match(text: string, cursor: number): TriggerMatch | null {
    const before = text.slice(0, cursor)
    const atIndex = before.lastIndexOf('@')
    if (atIndex < 0) return null

    // Trigger must be preceded by whitespace or at the start of text
    if (atIndex > 0 && !/\s/.test(before[atIndex - 1])) return null

    const query = before.slice(atIndex + 1)
    if (/\s/.test(query)) return null

    // If query looks like completed markdown mention token @[...](...), do not match
    if (/^\[(?:\\[\\\]]|[^\]\\\r\n])*\]\([^\s)]+\)/.test(query)) return null

    return {
      trigger: '@',
      query,
      start: atIndex,
      end: cursor,
      atLineStart: atIndex === 0 || before[atIndex - 1] === '\n',
    }
  }

  load(match: TriggerMatch, signal: AbortSignal): readonly DropdownItem[] {
    if (signal.aborted) return []

    const currentId = this.getCurrentSessionId()
    const allSessions = this.getSessions()
    const q = match.query.toLowerCase().trim()

    return allSessions
      .filter((s) => s.id !== currentId)
      .filter((s) => {
        if (!q) return true
        const titleMatch = s.title?.toLowerCase().includes(q)
        const idMatch = s.id.toLowerCase().includes(q)
        return titleMatch || idMatch
      })
      .slice(0, 20)
      .map((s): DropdownItem => {
        const title = s.title || `会话 ${s.id.slice(0, 8)}`
        return {
          id: `session:${s.id}`,
          label: `@${title}`,
          detail: `ID: ${s.id.slice(0, 8)} · ${s.backend}`,
          kind: 'session',
          badge: '会话',
          replacement: formatSessionMention(title, s.id),
        }
      })
  }

  select(item: DropdownItem, _match: TriggerMatch): string {
    return item.replacement
  }
}
