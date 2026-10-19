import type { TerminalServer, TerminalStatus } from '../../api/terminal'
import type { TerminalTab } from '../../stores/terminal'

export type { TerminalServer, TerminalStatus, TerminalTab }

export interface ServerFormData {
  name: string
  host: string
  port: number
  username: string
  auth_type: 'password' | 'key'
  secret: string
}
