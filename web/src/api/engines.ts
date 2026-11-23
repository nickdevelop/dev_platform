// Engine settings DTOs and client calls.
// Mirrors internal/app/settings/engines.go
import { get, mutate } from './client'

export interface StewardConfig {
  model: string
  maxSteps: number
  enableThinking: boolean
  systemPrompt: string
  promptId?: number
  enablePlatformTools: boolean
  enabledTools: string[]
}

export interface BuiltinCoderConfig {
  model: string
  maxSteps: number
  permissionMode: string
  systemPrompt: string
  injectPlatformDoc: boolean
  injectUserDoc: boolean
  injectProjectDoc: boolean
  enableSkills: boolean
  enabledTools: string[]
}

export interface OrchestratorConfig {
  model: string
  maxSteps: number
  timeoutSeconds: number
  systemPrompt: string
  autoInjectRoles: boolean
  enabledTools: string[]
}

export interface CoreEngineConfig {
  fallbackModel: string
  enableDebugLog: boolean
  logRetentionDays: number
  searchEngine: string
}

export interface EnginesSetting {
  steward: StewardConfig
  coder: BuiltinCoderConfig
  orchestrator: OrchestratorConfig
  core: CoreEngineConfig
  updatedAt?: string
}

export function getEnginesSettings(): Promise<EnginesSetting> {
  return get<EnginesSetting>('/settings/engines')
}

export function putEnginesSettings(setting: EnginesSetting): Promise<EnginesSetting> {
  return mutate<EnginesSetting>('PUT', '/settings/engines', setting)
}
