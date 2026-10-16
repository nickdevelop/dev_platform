// Dropdown contracts for chat composer completion (slash commands and mentions).
// Provider-neutral and independent of backend identifiers.

export type TriggerKind = '/' | '@'

export interface TriggerMatch {
  trigger: TriggerKind
  query: string
  start: number
  end: number
  atLineStart: boolean
}

export type DropdownItemKind = 'command' | 'file' | 'folder' | 'session' | 'status'

export interface DropdownItem {
  id: string
  label: string
  detail?: string
  icon?: string
  kind: DropdownItemKind
  replacement: string
  badge?: string
  action?: () => void | Promise<void>
}

export interface DropdownSource {
  id: string
  trigger: TriggerKind
  match(text: string, cursor: number): TriggerMatch | null
  load(match: TriggerMatch, signal: AbortSignal): Promise<readonly DropdownItem[]> | readonly DropdownItem[]
  select(item: DropdownItem, match: TriggerMatch): string | void
}
