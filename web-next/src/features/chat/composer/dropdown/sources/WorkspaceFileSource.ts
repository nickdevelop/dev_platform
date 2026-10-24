import type { DropdownItem, DropdownSource, TriggerMatch } from '../types'
import { searchProjectFiles } from '../../../../../api/projects'

export class WorkspaceFileSource implements DropdownSource {
  readonly id = 'workspace-file'
  readonly trigger = '@' as const

  constructor(private readonly getProjectId: () => number) {}

  match(text: string, cursor: number): TriggerMatch | null {
    const before = text.slice(0, cursor)
    const atIndex = before.lastIndexOf('@')
    if (atIndex < 0) return null

    // Trigger must be preceded by whitespace or at the start of text
    if (atIndex > 0 && !/\s/.test(before[atIndex - 1])) return null

    const query = before.slice(atIndex + 1)
    // Whitespace inside query means mention trigger is closed
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

  async load(match: TriggerMatch, signal: AbortSignal): Promise<readonly DropdownItem[]> {
    const projectId = this.getProjectId()
    if (!projectId) return []

    try {
      const resp = await searchProjectFiles(projectId, match.query, 50)
      if (signal.aborted) return []

      return resp.files.map((file): DropdownItem => {
        if (file.isDir) {
          return {
            id: `folder:${file.path}`,
            label: `@${file.path}/`,
            detail: '工作区文件夹',
            kind: 'folder',
            badge: '目录',
            replacement: `@${file.path}/ `,
          }
        }
        return {
          id: `file:${file.path}`,
          label: `@${file.path}`,
          detail: '工作区文件',
          kind: 'file',
          badge: '文件',
          replacement: `@${file.path} `,
        }
      })
    } catch {
      return []
    }
  }

  select(item: DropdownItem, _match: TriggerMatch): string {
    return item.replacement
  }
}
