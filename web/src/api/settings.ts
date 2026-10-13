// Appearance settings API calls. The Appearance DTO mirrors the server's
// settings.Appearance and docs/api/openapi.yaml; the preset metadata lives in
// theme/presets.ts so the client and the theme application share one type.
import { apiRequest } from './client'
import type { Appearance } from '../theme/presets'

export type { Appearance }

// defaultAppearance mirrors the server's default, used before the first load.
export const defaultAppearance: Appearance = {
  theme: 'light',
  fontSize: 14,
  density: 'standard',
}

export function getAppearance(): Promise<Appearance> {
  return apiRequest<Appearance>('GET', '/settings/appearance')
}

export function putAppearance(appearance: Appearance, csrfToken: string): Promise<Appearance> {
  return apiRequest<Appearance>('PUT', '/settings/appearance', { body: appearance, csrfToken })
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
  return apiRequest<AgentList>('GET', '/agents')
}

export function setAgentEnabled(
  backend: string,
  enabled: boolean,
  csrfToken: string,
): Promise<AgentDto> {
  return apiRequest<AgentDto>('PUT', `/agents/${encodeURIComponent(backend)}`, {
    body: { enabled },
    csrfToken,
  })
}

export function setAgentPath(backend: string, path: string, csrfToken: string): Promise<AgentDto> {
  return apiRequest<AgentDto>('PUT', `/agents/${encodeURIComponent(backend)}/path`, {
    body: { path },
    csrfToken,
  })
}

export function setAgentMode(backend: string, mode: string, csrfToken: string): Promise<AgentDto> {
  return apiRequest<AgentDto>('PUT', `/agents/${encodeURIComponent(backend)}/mode`, {
    body: { mode },
    csrfToken,
  })
}

export function detectAgent(backend: string, csrfToken: string): Promise<AgentDetection> {
  return apiRequest<AgentDetection>('POST', `/agents/${encodeURIComponent(backend)}/detect`, {
    csrfToken,
  })
}
