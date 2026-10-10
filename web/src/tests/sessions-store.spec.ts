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
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
  }
}

function session(id: string, projectId: number): AgentSession {
  return {
    id,
    projectId,
    backend: 'pi',
    instanceId: 'inst-1',
    nativeSessionId: `native-${id}`,
    title: id,
    status: 'active',
    capabilities: capabilities(),
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
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
