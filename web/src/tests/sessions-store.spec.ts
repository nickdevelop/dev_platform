// Unit tests for the sessions store's project list: a slow response for a project
// the user already left must never replace the list of the project now open, and
// the list is cleared when the project changes so a previous project's tasks are
// never shown under the new one.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useSessionsStore } from '../stores/sessions'
import type { AgentSession, Capabilities } from '../api/sessions'

function capabilities(): Capabilities {
  return {
    streaming: true,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: false,
    interrupt: false,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
  }
}

function session(id: string, projectId: number, overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id,
    projectId,
    backend: 'pi',
    instanceId: 'inst-1',
    nativeSessionId: `native-${id}`,
    title: id,
    status: 'active',
    turn: 'idle',
    capabilities: capabilities(),
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
    ...overrides,
  }
}

// Each list request resolves only when the test says so, which lets a test
// finish the requests in an order of its choosing.
function deferredLists() {
  const pending = new Map<number, (value: unknown) => void>()
  vi.stubGlobal(
    'fetch',
    vi.fn(
      (input: RequestInfo | URL) =>
        new Promise<Response>((resolve) => {
          const url = new URL(String(input), 'http://localhost')
          const projectId = Number(url.pathname.split('/')[3])
          pending.set(projectId, (sessions) =>
            resolve({ status: 200, json: async () => ({ sessions }) } as Response),
          )
        }),
    ),
  )
  return pending
}

describe('sessions store project list', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('ignores a slow response for a project the user has already left', async () => {
    const pending = deferredLists()
    const sessions = useSessionsStore()

    const first = sessions.loadSessions(1)
    const second = sessions.loadSessions(2)
    pending.get(2)!([session('b-1', 2)])
    await second
    pending.get(1)!([session('a-1', 1)])
    await first

    expect(sessions.projectId).toBe(2)
    expect(sessions.sessions.map((entry) => entry.id)).toEqual(['b-1'])
  })

  it('clears the previous project list when a different project is loaded', async () => {
    const pending = deferredLists()
    const sessions = useSessionsStore()

    const first = sessions.loadSessions(1)
    pending.get(1)!([session('a-1', 1)])
    await first
    expect(sessions.sessions.map((entry) => entry.id)).toEqual(['a-1'])

    const second = sessions.loadSessions(2)
    expect(sessions.sessions).toEqual([])
    pending.get(2)!([session('b-1', 2)])
    await second
    expect(sessions.sessions.map((entry) => entry.id)).toEqual(['b-1'])
  })
})

describe('sessions store turn actions', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('interrupt replaces the session with the server turn without an error', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1, { turn: 'running' }), session('s2', 1)]
    const calls: Array<{ url: string; method?: string }> = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({ url: String(input), method: init?.method })
        return { status: 200, json: async () => session('s1', 1, { turn: 'idle' }) } as Response
      }),
    )

    const result = await sessions.interrupt(1, 's1')

    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/projects/1/sessions/s1/interrupt')
    expect(calls[0].method).toBe('POST')
    expect(result?.turn).toBe('idle')
    expect(sessions.sessions.find((entry) => entry.id === 's1')?.turn).toBe('idle')
    expect(sessions.error).toBe('')
    // A sibling session is untouched.
    expect(sessions.sessions.find((entry) => entry.id === 's2')?.turn).toBe('idle')
  })

  it('refreshSessions replaces the list without clearing it or toggling loading', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    sessions.loading = false
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          ({
            status: 200,
            json: async () => ({ sessions: [session('s1', 1, { turn: 'running' }), session('s2', 1)] }),
          }) as Response,
      ),
    )

    await sessions.refreshSessions(1)

    expect(sessions.sessions.map((entry) => entry.id)).toEqual(['s1', 's2'])
    expect(sessions.sessions.find((entry) => entry.id === 's1')?.turn).toBe('running')
    expect(sessions.loading).toBe(false)
  })

  it('refreshSessions keeps the last good list and stays silent on failure', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('network down')
      }),
    )

    await sessions.refreshSessions(1)

    expect(sessions.sessions.map((entry) => entry.id)).toEqual(['s1'])
    expect(sessions.error).toBe('')
  })
})
