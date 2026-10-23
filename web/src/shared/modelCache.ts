// The last model snapshot seen per backend, kept in memory for the browser
// session so opening a session or the new-session panel shows its models
// immediately while the persisted platform snapshot loads. It is a cache, not
// state: a page reload starts empty and the platform database remains the
// source of truth. Keyed by backend because the catalog is platform-wide.
import type { BackendId, ModelInfo } from '../api/agents'

const cache = new Map<string, ModelInfo[]>()

export function cachedModels(backend: BackendId): ModelInfo[] | undefined {
  return cache.get(String(backend))
}

export function rememberModels(backend: BackendId, models: ModelInfo[]): void {
  cache.set(String(backend), models)
}

// clearModelCache drops every cached list (used by tests between cases).
export function clearModelCache(): void {
  cache.clear()
}
