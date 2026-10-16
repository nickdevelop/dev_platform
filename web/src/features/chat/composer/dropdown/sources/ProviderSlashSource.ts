import type { AgentCommand } from '../../../../../api/sessions'
import { listSessionCommands } from '../../../../../api/sessions'
import type { DropdownItem, DropdownSource, TriggerMatch } from '../types'

export class ProviderSlashSource implements DropdownSource {
  readonly id = 'provider-slash'
  readonly trigger = '/' as const

  private readonly cache = new Map<string, AgentCommand[]>()

  constructor(
    private readonly getProjectId: () => number,
    private readonly getSessionId: () => string,
  ) {}

  match(text: string, cursor: number): TriggerMatch | null {
    const before = text.slice(0, cursor)
    const lastNewline = before.lastIndexOf('\n')
    const lineStart = lastNewline === -1 ? 0 : lastNewline + 1
    const lineBefore = before.slice(lineStart)

    if (!lineBefore.startsWith('/')) return null

    const query = lineBefore.slice(1)
    if (/\s/.test(query)) return null

    return {
      trigger: '/',
      query,
      start: lineStart,
      end: cursor,
      atLineStart: true,
    }
  }

  async load(match: TriggerMatch, signal: AbortSignal): Promise<readonly DropdownItem[]> {
    const projectId = this.getProjectId()
    const sessionId = this.getSessionId()
    if (!projectId || !sessionId) return []

    const cacheKey = `${projectId}:${sessionId}`
    let commands = this.cache.get(cacheKey)

    if (!commands) {
      try {
        const resp = await listSessionCommands(projectId, sessionId)
        if (signal.aborted) return []
        commands = resp.commands || []
        this.cache.set(cacheKey, commands)
      } catch {
        return []
      }
    }

    if (signal.aborted) return []

    const q = match.query.toLowerCase().trim()
    return commands
      .filter((cmd) => {
        if (!q) return true
        return (
          cmd.name.toLowerCase().includes(q) ||
          cmd.description.toLowerCase().includes(q)
        )
      })
      .map((cmd): DropdownItem => {
        const badge = cmd.kind === 'skill' ? '技能' : 'Agent'
        const detail = cmd.argumentHint
          ? `${cmd.description} · ${cmd.argumentHint}`
          : cmd.description

        return {
          id: `provider:${cmd.name}`,
          label: `/${cmd.name}`,
          detail,
          kind: 'command',
          badge,
          replacement: `/${cmd.name} `,
        }
      })
  }

  select(item: DropdownItem, _match: TriggerMatch): string {
    return item.replacement
  }

  clearCache(sessionId?: string): void {
    if (sessionId) {
      for (const key of this.cache.keys()) {
        if (key.endsWith(`:${sessionId}`)) {
          this.cache.delete(key)
        }
      }
    } else {
      this.cache.clear()
    }
  }
}
