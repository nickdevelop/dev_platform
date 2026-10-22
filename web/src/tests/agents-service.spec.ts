// Unit tests for the agent-service API surface and store actions: the platform
// settings page is the only caller, and what it renders is what these actions
// leave in the store — including the failure wording, which is the same sentence
// the workbench shows when a session is refused.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAgentsStore } from '../stores/agents'
import { useAuthStore } from '../stores/auth'
import { installTestCSRF } from './csrf-fixture'
import { ApiError, describeApiError } from '../api/client'
import type { AgentServiceStatus } from '../api/agents'

function status(overrides: Partial<AgentServiceStatus> = {}): AgentServiceStatus {
  return {
    backend: 'opencode-v2',
    supported: true,
    state: 'running',
    endpoint: 'http://127.0.0.1:4096',
    pid: 4242,
    version: '2.0.24',
    executable: '/usr/local/bin/opencode',
    detail: '',
    checkedAt: '2026-10-13T00:00:00Z',
    ...overrides,
  }
}

// stubFetch answers each request with the queued payload in order and records
// how it was called.
function stubFetch(responses: unknown[]) {
  const calls: { url: string; method: string; headers: Record<string, string> }[] = []
  let index = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({
        url: String(input),
        method: (init?.method ?? 'GET').toUpperCase(),
        headers: (init?.headers ?? {}) as Record<string, string>,
      })
      const payload = responses[Math.min(index, responses.length - 1)]
      index += 1
      return { status: 200, json: async () => payload } as Response
    }),
  )
  return calls
}

describe('agent service api and store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useAuthStore().$patch({ csrfToken: 'csrf-abc' })
    installTestCSRF('csrf-abc')
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('detects, starts and stops through the documented routes', async () => {
    const store = useAgentsStore()
    const calls = stubFetch([
      { status: status({ state: 'stopped', endpoint: '', pid: 0 }) },
      { status: status({ state: 'running' }) },
      { status: status({ state: 'stopped', endpoint: '', pid: 0 }) },
    ])

    const detected = await store.detectService('opencode-v2')
    expect(detected?.state).toBe('stopped')
    expect(store.services['opencode-v2']?.state).toBe('stopped')

    await store.startService('opencode-v2')
    expect(store.services['opencode-v2']?.state).toBe('running')

    await store.stopService('opencode-v2')
    expect(store.services['opencode-v2']?.state).toBe('stopped')

    expect(calls.map((call) => `${call.method} ${call.url}`)).toEqual([
      'GET /api/agent-services/opencode-v2',
      'POST /api/agent-services/opencode-v2/start',
      'POST /api/agent-services/opencode-v2/stop',
    ])
    // Reads carry no CSRF header; every mutation must.
    expect(calls[0].headers['X-CSRF-Token']).toBeUndefined()
    expect(calls[1].headers['X-CSRF-Token']).toBe('csrf-abc')
    expect(calls[2].headers['X-CSRF-Token']).toBe('csrf-abc')
  })

  it('reports a refused start instead of pretending it worked', async () => {
    const store = useAgentsStore()
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ status: 409, json: async () => ({ error: 'service_already_running' }) } as Response)),
    )

    await expect(store.startService('opencode-v2')).resolves.toBeNull()
    expect(store.serviceError).toBe('opencode service 已在运行。')
  })

  it('names the settings page when a session is refused because the service is down', () => {
    expect(describeApiError(new ApiError(409, 'service_not_running'))).toBe(
      'opencode service 未启动，请到平台设置 → Agent 检测并启动。',
    )
    expect(describeApiError(new ApiError(409, 'service_already_running'))).toBe(
      'opencode service 已在运行。',
    )
  })
})
