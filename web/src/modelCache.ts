// The last model list seen per agent backend, kept in memory for the browser
// session so a newly opened task shows its models immediately while a fresh
// list loads in the background. It is a cache, not state: a page reload starts
// empty and the backend remains the source of truth.
import type { BackendId, ModelInfo } from './api/agents'

const cache = new Map<BackendId, ModelInfo[]>()

export function cachedModels(backend: BackendId): ModelInfo[] | undefined {
  return cache.get(backend)
}

export function rememberModels(backend: BackendId, models: ModelInfo[]): void {
  cache.set(backend, models)
}

// clearModelCache drops every cached list (used by tests between cases).
export function clearModelCache(): void {
  cache.clear()
}
