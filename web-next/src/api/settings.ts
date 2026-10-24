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
  density: 'standard',
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
  pathSource: 'auto' | 'manual' | 'none'
  available: boolean
  // mode selects a backend transport when the backend has more than one
  // (pi: cli or rpc). modes lists the choices; both are absent otherwise.
  mode?: string
  modes?: string[]
  version?: string
  checkedAt?: string
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

export function detectAgent(backend: string): Promise<AgentDetection> {
  return mutate<AgentDetection>('POST', `/agents/${encodeURIComponent(backend)}/detect`)
}
