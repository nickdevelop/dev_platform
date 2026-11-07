// Appearance settings API calls. The Appearance DTO mirrors the server's
// settings.Appearance and docs/api/openapi.yaml; the preset metadata lives in
// theme/presets.ts so the client and the theme application share one type.
import { get, mutate } from './client'
import type { Appearance } from '../theme/presets'

export type { Appearance }

// defaultAppearance mirrors the server's default, used before the first load.
export const defaultAppearance: Appearance = {
  theme: 'light',
  fontSize: 14,
  plantumlMode: 'local',
  plantumlServerUrl: 'https://www.plantuml.com/plantuml',
}

export function getAppearance(): Promise<Appearance> {
  return get<Appearance>('/settings/appearance')
}

export function putAppearance(appearance: Appearance): Promise<Appearance> {
  return mutate<Appearance>('PUT', '/settings/appearance', appearance)
}

// QuickAssistant toggles the floating quick-assistant surface. It is a
// platform feature switch stored separately from the visual appearance config.
export interface QuickAssistantSettings {
  enabled: boolean
}

export function getQuickAssistantSettings(): Promise<QuickAssistantSettings> {
  return get<QuickAssistantSettings>('/settings/quick-assistant')
}

export function putQuickAssistantSettings(
  settings: QuickAssistantSettings,
): Promise<QuickAssistantSettings> {
  return mutate<QuickAssistantSettings>('PUT', '/settings/quick-assistant', settings)
}

// Agent management DTOs and calls. enabled gates new session creation; pathSource
// is auto when the executable was found on PATH, manual when the operator set a
// path, none when neither applies.
export interface AgentDto {
  backend: string
  enabled: boolean
  path: string
  // pathSource is auto when the executable was found on PATH, manual when the
  // operator set a path, none when neither applies, and embedded for an
  // in-process backend (no path controls apply).
  pathSource: 'auto' | 'manual' | 'none' | 'embedded'
  available: boolean
  // mode selects a backend transport when the backend has more than one
  // (pi: cli or rpc). modes lists the choices; both are absent otherwise.
  mode?: string
  modes?: string[]
  version?: string
  checkedAt?: string
  idleTimeoutMinutes?: number
  // permissionMode gates the tool confirmations of backends that support the
  // control; permissionModes lists the choices. Both are absent otherwise.
  permissionMode?: string
  permissionModes?: string[]
  // sessionDataOwner marks a backend whose transcript the platform persists,
  // so the settings center offers its session data management.
  sessionDataOwner?: boolean
}

export interface AgentList {
  agents: AgentDto[]
}

export interface AgentDetection {
  backend: string
  status: 'ok' | 'error'
  path?: string
  version?: string
  error?: string
}

export function listAgents(): Promise<AgentList> {
  return get<AgentList>('/agents')
}

export function setAgentEnabled(backend: string, enabled: boolean): Promise<AgentDto> {
  return mutate<AgentDto>('PUT', `/agents/${encodeURIComponent(backend)}`, { enabled })
}

export function setAgentPath(backend: string, path: string): Promise<AgentDto> {
  return mutate<AgentDto>('PUT', `/agents/${encodeURIComponent(backend)}/path`, { path })
}

export function setAgentMode(backend: string, mode: string): Promise<AgentDto> {
  return mutate<AgentDto>('PUT', `/agents/${encodeURIComponent(backend)}/mode`, { mode })
}

export function setAgentIdleTimeout(backend: string, minutes: number): Promise<AgentDto> {
  return mutate<AgentDto>('PUT', `/agents/${encodeURIComponent(backend)}/idle-timeout`, { minutes })
}

export function setAgentPermissionMode(backend: string, mode: string): Promise<AgentDto> {
  return mutate<AgentDto>('PUT', `/agents/${encodeURIComponent(backend)}/permission-mode`, { mode })
}

// Transcript data management of the backend that persists its session data in
// the platform database (the embedded agent).
export interface AgentDataSession {
  sessionId: string
  title: string
  directory: string
  model?: string
  messageCount: number
  createdAt: string
  updatedAt: string
}

export interface AgentDataSessionList {
  sessions: AgentDataSession[]
  total: number
}

export function listAgentDataSessions(page = 1, pageSize = 20): Promise<AgentDataSessionList> {
  const query = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
  return get<AgentDataSessionList>(`/settings/agent-data/sessions?${query.toString()}`)
}

export function deleteAgentDataSession(sessionId: string): Promise<{ deleted: boolean }> {
  return mutate<{ deleted: boolean }>('DELETE', `/settings/agent-data/sessions/${encodeURIComponent(sessionId)}`)
}

export function clearAgentDataSessions(): Promise<{ deleted: number }> {
  return mutate<{ deleted: number }>('DELETE', '/settings/agent-data/sessions')
}

export function detectAgent(backend: string): Promise<AgentDetection> {
  return mutate<AgentDetection>('POST', `/agents/${encodeURIComponent(backend)}/detect`)
}

// Global Agent settings: shared across all agent backends (system prompt / AGENTS.md).
export interface AgentGeneralSettings {
  systemPrompt: string
  path?: string
  updatedAt?: string
}

export function getAgentGeneralSettings(): Promise<AgentGeneralSettings> {
  return get<AgentGeneralSettings>('/settings/agent-general')
}

export function putAgentGeneralSettings(
  settings: { systemPrompt: string },
): Promise<AgentGeneralSettings> {
  return mutate<AgentGeneralSettings>('PUT', '/settings/agent-general', settings)
}
