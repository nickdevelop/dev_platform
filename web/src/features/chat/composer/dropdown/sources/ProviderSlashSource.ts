import type { PlatformCommand } from '../../../../../api/sessions'
import { listSessionCommands } from '../../../../../api/sessions'
import type { DropdownItem, DropdownSource, TriggerMatch } from '../types'

// Default platform-designed commands available offline or before session commands load.
// Currently, only the functional "compact" action command is implemented.
export const DEFAULT_PLATFORM_COMMANDS: PlatformCommand[] = [
  {
    name: 'compact',
    description: '压缩当前会话上下文历史，减少 Token 占用',
    category: 'action',
    argumentHint: '[可选说明]',
  },
]

// Backward-compatible alias for any legacy references
export const DEFAULT_BACKEND_COMMANDS: Record<string, PlatformCommand[]> = {
  'opencode-v2': DEFAULT_PLATFORM_COMMANDS,
  'pi': DEFAULT_PLATFORM_COMMANDS,
  'deepseek': DEFAULT_PLATFORM_COMMANDS,
}

export class ProviderSlashSource implements DropdownSource {
  readonly id = 'platform-slash'
  readonly trigger = '/' as const

  private readonly cache = new Map<string, PlatformCommand[]>()

  constructor(
    private readonly getProjectId: () => number,
    private readonly getSessionId: () => string,
    private readonly getBackend?: () => string | undefined,
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

    let commands: PlatformCommand[] | undefined

    if (projectId && sessionId) {
      const cacheKey = `${projectId}:${sessionId}`
      commands = this.cache.get(cacheKey)

      if (!commands) {
        try {
          const resp = await listSessionCommands(projectId, sessionId)
          if (signal.aborted) return []
          commands = resp.commands || []
          this.cache.set(cacheKey, commands)
        } catch {
          commands = DEFAULT_PLATFORM_COMMANDS
        }
      }
    } else {
      commands = DEFAULT_PLATFORM_COMMANDS
    }

    if (!commands || signal.aborted) return []

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
        const badge = cmd.category === 'prompt' ? '提示词' : '功能'
        const detail = cmd.argumentHint
          ? `${cmd.description} · ${cmd.argumentHint}`
          : cmd.description

        return {
          id: `platform:${cmd.name}`,
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

  findCommand(name: string): PlatformCommand | undefined {
    const cleanName = name.toLowerCase()
    const projectId = this.getProjectId()
    const sessionId = this.getSessionId()

    const cacheKey = `${projectId}:${sessionId}`
    const list = this.cache.get(cacheKey) || DEFAULT_PLATFORM_COMMANDS

    return list.find((c) => c.name.toLowerCase() === cleanName)
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

// PlatformSlashSource is the alias for ProviderSlashSource under the new platform-owned command model.
export const PlatformSlashSource = ProviderSlashSource
