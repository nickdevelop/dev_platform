// The last model list seen per project and backend, kept in memory for the
// browser session so opening a session or the new-session panel shows its models
// immediately while a fresh list loads. It is a cache, not state: a page reload
// starts empty and the platform's own per-(backend, directory) cache stays the
// source of truth. Keyed by project because a backend's model list is scoped to
// the project's directory.
import type { BackendId, ModelInfo } from './api/agents'

const cache = new Map<string, ModelInfo[]>()

function key(projectId: number, backend: BackendId): string {
  return `${projectId}:${backend}`
}

export function cachedModels(projectId: number, backend: BackendId): ModelInfo[] | undefined {
  return cache.get(key(projectId, backend))
}

export function rememberModels(projectId: number, backend: BackendId, models: ModelInfo[]): void {
  cache.set(key(projectId, backend), models)
}

// forgetModels drops one entry, so a caller that just learned the list was wrong
// (the backend refused the model it offered) stops offering it.
export function forgetModels(projectId: number, backend: BackendId): void {
  cache.delete(key(projectId, backend))
}

// clearModelCache drops every cached list (used by tests between cases).
export function clearModelCache(): void {
  cache.clear()
}
