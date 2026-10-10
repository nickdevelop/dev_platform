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
    sessionHistory: false,
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

  it('discoverNativeSessions merges results without duplicating a known session', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          ({ status: 200, json: async () => ({ sessions: [session('s2', 1), session('s1', 1)] }) }) as Response,
      ),
    )

    const found = await sessions.discoverNativeSessions(1, 'inst-1')

    expect(found?.map((entry) => entry.id)).toEqual(['s2', 's1'])
    expect(sessions.sessions.map((entry) => entry.id).sort()).toEqual(['s1', 's2'])
    expect(sessions.error).toBe('')
  })
})

// A minimal EventSource stand-in: the history tests only need to prove that a
// subscription was opened, not to drive the stream.
class RecordingEventSource {
  static instances: RecordingEventSource[] = []
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 2

  readyState = RecordingEventSource.CONNECTING
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null

  constructor(readonly url: string) {
    RecordingEventSource.instances.push(this)
  }

  addEventListener() {}
  close() {
    this.readyState = RecordingEventSource.CLOSED
  }
}

function historyEvent(overrides: Record<string, unknown> = {}) {
  return {
    instanceId: '',
    backend: 'pi',
    sequence: 1,
    timestamp: '2026-10-10T12:00:00Z',
    replayed: true,
    ...overrides,
  }
}

function messageEvent(role: string, text: string, sequence: number) {
  return historyEvent({ type: 'message', sequence, message: { role, text, final: true } })
}

function stubRoutes(routes: Record<string, unknown>): string[] {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      calls.push(url)
      for (const [suffix, payload] of Object.entries(routes)) {
        if (url.endsWith(suffix)) return { status: 200, json: async () => payload } as Response
      }
      return { status: 404, json: async () => ({ code: 'not_found', message: 'missing' }) } as Response
    }),
  )
  return calls
}

describe('sessions store history', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    RecordingEventSource.instances = []
    vi.stubGlobal('EventSource', RecordingEventSource)
    // The live subscriptions live in a module-level map, so a source opened by
    // an earlier test would make the next subscribe a no-op.
    useSessionsStore().unsubscribeAll()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('opens a session in continue → history → subscribe order', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    const calls = stubRoutes({
      '/continue': session('s1', 1),
      '/history': { events: [messageEvent('user', '历史', 1)], truncated: false },
    })

    await sessions.openSession(1, 's1')

    expect(calls).toEqual([
      '/api/projects/1/sessions/s1/continue',
      '/api/projects/1/sessions/s1/history',
    ])
    expect(RecordingEventSource.instances).toHaveLength(1)
    expect(RecordingEventSource.instances[0].url).toBe('/api/projects/1/sessions/s1/events')
  })

  it('replays history into the log and marks where it ends', async () => {
    const sessions = useSessionsStore()
    stubRoutes({
      '/history': {
        events: [messageEvent('user', '历史里的问题', 1), messageEvent('assistant', '历史里的回答', 2)],
        truncated: false,
      },
    })

    await sessions.loadHistory(1, 's1')

    const entries = sessions.messagesFor('s1')
    expect(entries.map((entry) => entry.kind)).toEqual(['message', 'message', 'separator'])
    expect(entries[0].text).toBe('历史里的问题')
    expect(entries[1].text).toBe('历史里的回答')
    expect(entries[2].text).toContain('历史')
  })

  it('loads history only once per session', async () => {
    const sessions = useSessionsStore()
    const calls = stubRoutes({ '/history': { events: [messageEvent('user', '一次', 1)], truncated: false } })

    await sessions.loadHistory(1, 's1')
    await sessions.loadHistory(1, 's1')

    expect(calls).toHaveLength(1)
  })

  it('records a load failure and still opens the live stream', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.endsWith('/history')) {
          return { status: 502, json: async () => ({ code: 'agent_operation_failed', message: '读不到历史' }) } as Response
        }
        return { status: 200, json: async () => session('s1', 1) } as Response
      }),
    )

    await sessions.openSession(1, 's1')

    expect(sessions.historyError['s1']).not.toBe('')
    expect(sessions.messagesFor('s1')).toEqual([])
    expect(RecordingEventSource.instances).toHaveLength(1)
  })

  it('records the truncation verdict', async () => {
    const sessions = useSessionsStore()
    stubRoutes({ '/history': { events: [messageEvent('user', '最近一条', 1)], truncated: true } })

    await sessions.loadHistory(1, 's1')

    expect(sessions.historyTruncated['s1']).toBe(true)
  })

  it('drops a live echo of the message history already ended with', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    stubRoutes({ '/history': { events: [messageEvent('assistant', '最终回复', 1)], truncated: false } })

    await sessions.loadHistory(1, 's1')
    sessions.handleEvent(
      's1',
      JSON.stringify(historyEvent({ instanceId: 'inst-1', type: 'message', sequence: 99, replayed: false, message: { role: 'assistant', text: '最终回复', final: true } })),
    )

    const messages = sessions.messagesFor('s1').filter((entry) => entry.kind === 'message')
    expect(messages).toHaveLength(1)
  })
})

describe('sessions store thinking entries', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    RecordingEventSource.instances = []
    vi.stubGlobal('EventSource', RecordingEventSource)
    useSessionsStore().unsubscribeAll()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function thinkingEvent(text: string, final: boolean, sequence: number) {
    return historyEvent({ instanceId: 'inst-1', type: 'thinking', sequence, replayed: false, thinking: { text, final } })
  }

  it('accumulates streamed reasoning into one entry and closes it', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]

    sessions.handleEvent('s1', JSON.stringify(thinkingEvent('先看', false, 1)))
    sessions.handleEvent('s1', JSON.stringify(thinkingEvent('再看', false, 2)))
    sessions.handleEvent('s1', JSON.stringify(thinkingEvent('', true, 3)))

    const entries = sessions.messagesFor('s1')
    expect(entries).toHaveLength(1)
    expect(entries[0].kind).toBe('thinking')
    expect(entries[0].text).toBe('先看再看')
    expect(entries[0].streaming).toBe(false)
  })

  it('keeps reasoning as its own line before the reply', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]

    sessions.handleEvent('s1', JSON.stringify(thinkingEvent('推理', false, 1)))
    sessions.handleEvent('s1', JSON.stringify(thinkingEvent('', true, 2)))
    sessions.handleEvent('s1', JSON.stringify({ ...historyEvent({ instanceId: 'inst-1', type: 'message', sequence: 3 }), message: { role: 'assistant', text: '答案', final: true } }))

    expect(sessions.messagesFor('s1').map((entry) => entry.kind)).toEqual(['thinking', 'message'])
    expect(sessions.messagesFor('s1')[1].text).toBe('答案')
  })

  it('replays reasoning from history as a closed entry', async () => {
    const sessions = useSessionsStore()
    stubRoutes({
      '/history': {
        events: [
          historyEvent({ type: 'thinking', sequence: 1, thinking: { text: '历史推理', final: true } }),
          messageEvent('assistant', '历史回答', 2),
        ],
        truncated: false,
      },
    })

    await sessions.loadHistory(1, 's1')

    const entries = sessions.messagesFor('s1')
    expect(entries.map((entry) => entry.kind)).toEqual(['thinking', 'message', 'separator'])
    expect(entries[0].text).toBe('历史推理')
    expect(entries[0].streaming).toBe(false)
  })
})
