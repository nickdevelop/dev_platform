import type { DropdownItem, DropdownSource, TriggerMatch } from '../types'
import { BUILT_IN_COMMANDS } from '../../builtInCommands'

export class BuiltInSlashSource implements DropdownSource {
  readonly id = 'builtin-slash'
  readonly trigger = '/' as const

  match(text: string, cursor: number): TriggerMatch | null {
    const before = text.slice(0, cursor)
    // Find the last newline before cursor to determine start of current line
    const lastNewline = before.lastIndexOf('\n')
    const lineStart = lastNewline === -1 ? 0 : lastNewline + 1
    const lineBefore = before.slice(lineStart)

    // Trigger only if the line starts with '/'
    if (!lineBefore.startsWith('/')) return null

    // If there is whitespace between '/' and cursor, slash command name is already entered
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

  load(match: TriggerMatch, signal: AbortSignal): readonly DropdownItem[] {
    if (signal.aborted) return []
    const q = match.query.toLowerCase().trim()

    return BUILT_IN_COMMANDS.filter((cmd) => {
      if (!q) return true
      return (
        cmd.name.toLowerCase().includes(q) ||
        cmd.description.toLowerCase().includes(q) ||
        cmd.aliases?.some((a) => a.toLowerCase().includes(q))
      )
    }).map((cmd) => {
      const aliasSuffix = cmd.aliases?.length ? ` (${cmd.aliases.join(', ')})` : ''
      return {
        id: `builtin:${cmd.name}`,
        label: `/${cmd.name}`,
        detail: cmd.argumentHint
          ? `${cmd.description} · ${cmd.argumentHint}${aliasSuffix}`
          : `${cmd.description}${aliasSuffix}`,
        kind: 'command',
        badge: '内置',
        replacement: `/${cmd.name} `,
      }
    })
  }

  select(item: DropdownItem, _match: TriggerMatch): string {
    return item.replacement
  }
}
