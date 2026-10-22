// Unit tests for the sessions store's project list: a slow response for a project
// the user already left must never replace the list of the project now open, and
// the list is cleared when the project changes so a previous project's sessions are
// never shown under the new one.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useSessionsStore } from '../stores/sessions'
import { useAuthStore } from '../stores/auth'
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
    rawEvents: false,
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
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push(url)
      for (const [suffix, payload] of Object.entries(routes)) {
        if (url.endsWith(suffix)) return { status: 200, json: async () => payload } as Response
      }
      if (url.includes('/events/') && url.includes('/subscriptions')) {
        if (init?.method === 'DELETE') {
          return { status: 200, json: async () => ({ success: true }) } as Response
        }
        return {
          status: 200,
          json: async () => ({
            sessionId: 's1',
            turn: { id: 'turn-1', phase: 'ended', outcome: 'completed' },
            interactions: { permissions: [], forms: [] },
            resync: false,
          }),
        } as Response
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

    expect(calls.filter((c) => !c.includes('/events/'))).toEqual([
      '/api/projects/1/sessions/s1/continue',
      '/api/projects/1/sessions/s1/history',
    ])
    expect(calls.some((c) => c.includes('/events/') && c.includes('/subscriptions'))).toBe(true)
    expect(RecordingEventSource.instances).toHaveLength(1)
    expect(RecordingEventSource.instances[0].url).toContain('/api/events/')
  })

  it('replays history into the log as one continuous conversation', async () => {
    const sessions = useSessionsStore()
    stubRoutes({
      '/history': {
        events: [messageEvent('user', '历史里的问题', 1), messageEvent('assistant', '历史里的回答', 2)],
        truncated: false,
      },
    })

    await sessions.loadHistory(1, 's1')

    const entries = sessions.messagesFor('s1')
    expect(entries.map((entry) => entry.kind)).toEqual(['message', 'message'])
    expect(entries[0].text).toBe('历史里的问题')
    expect(entries[1].text).toBe('历史里的回答')
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
    expect(entries.map((entry) => entry.kind)).toEqual(['thinking', 'message'])
    expect(entries[0].text).toBe('历史推理')
    expect(entries[0].streaming).toBe(false)
  })
})

describe('sessions store tool entries', () => {  beforeEach(() => {
    setActivePinia(createPinia())
    RecordingEventSource.instances = []
    vi.stubGlobal('EventSource', RecordingEventSource)
    useSessionsStore().unsubscribeAll()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('keeps a tool call input so the view can summarise it', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]

    sessions.handleEvent(
      's1',
      JSON.stringify(
        historyEvent({
          type: 'tool_call',
          sequence: 1,
          replayed: false,
          toolCall: { callId: 'call-1', name: 'read', input: { path: '/docs/models.md' } },
        }),
      ),
    )

    const entries = sessions.messagesFor('s1')
    expect(entries).toHaveLength(1)
    expect(entries[0].kind).toBe('tool_call')
    expect(entries[0].input).toEqual({ path: '/docs/models.md' })
  })
})

describe('sessions store delete', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('deletes a session, drops its local state and reports success', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1), session('s2', 1)]
    sessions.pushEntry('s1', {
      id: 'm1',
      kind: 'message',
      role: 'assistant',
      text: 'hi',
      streaming: false,
    })
    sessions.streamStatus['s1'] = 'open'
    useAuthStore().csrfToken = 'csrf-1'
    const calls: Array<{ method?: string; url: string }> = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({ method: init?.method, url: String(input) })
        return { status: 204, json: async () => ({}) } as Response
      }),
    )

    const ok = await sessions.deleteSession(1, 's1')

    expect(ok).toBe(true)
    expect(calls[0].method).toBe('DELETE')
    expect(calls[0].url).toBe('/api/projects/1/sessions/s1')
    expect(sessions.sessions.map((entry) => entry.id)).toEqual(['s2'])
    expect(sessions.messagesFor('s1')).toEqual([])
    expect(sessions.streamStatus['s1']).toBeUndefined()
  })

  it('keeps the session and reports the error when delete fails', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    useAuthStore().csrfToken = 'csrf-1'
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ status: 500, json: async () => ({ error: 'server_error' }) }) as Response),
    )

    const ok = await sessions.deleteSession(1, 's1')

    expect(ok).toBe(false)
    expect(sessions.sessions.map((entry) => entry.id)).toEqual(['s1'])
    expect(sessions.error).toContain('服务内部错误')
  })
})

describe('sessions store interleaved stream merging', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    RecordingEventSource.instances = []
    vi.stubGlobal('EventSource', RecordingEventSource)
    useSessionsStore().unsubscribeAll()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function liveThinking(text: string, final: boolean, sequence: number) {
    return historyEvent({ instanceId: 'inst-1', type: 'thinking', sequence, replayed: false, thinking: { text, final } })
  }

  function liveMessage(role: string, text: string, final: boolean, sequence: number) {
    return historyEvent({ instanceId: 'inst-1', type: 'message', sequence, replayed: false, message: { role, text, final } })
  }

  // pi's real event stream interleaves thinking_delta into a running text
  // delta and closes the thinking block after the text resumed (observed with
  // pi 1.1.0 --mode json). The merge must follow the open block, not the last
  // list position, or the log fills with fragments and empty blocks.
  it('merges thinking and message deltas that arrive interleaved', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]

    sessions.handleEvent('s1', JSON.stringify(liveThinking('甲', false, 1)))
    sessions.handleEvent('s1', JSON.stringify(liveMessage('assistant', 'A', false, 2)))
    sessions.handleEvent('s1', JSON.stringify(liveThinking('乙', false, 3)))
    sessions.handleEvent('s1', JSON.stringify(liveMessage('assistant', 'B', false, 4)))
    sessions.handleEvent('s1', JSON.stringify(liveThinking('', true, 5)))
    sessions.handleEvent('s1', JSON.stringify(liveMessage('assistant', '', true, 6)))

    const entries = sessions.messagesFor('s1')
    expect(entries.map((entry) => entry.kind)).toEqual(['thinking', 'message'])
    expect(entries[0].text).toBe('甲乙')
    expect(entries[0].streaming).toBe(false)
    expect(entries[1].text).toBe('AB')
    expect(entries[1].streaming).toBe(false)
  })

  it('drops an orphan empty thinking terminator instead of opening an empty block', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]

    sessions.handleEvent('s1', JSON.stringify(liveMessage('assistant', '答', true, 1)))
    sessions.handleEvent('s1', JSON.stringify(liveThinking('', true, 2)))

    expect(sessions.messagesFor('s1').map((entry) => entry.kind)).toEqual(['message'])
  })

  it('ignores empty non-final deltas', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]

    sessions.handleEvent('s1', JSON.stringify(liveThinking('', false, 1)))
    sessions.handleEvent('s1', JSON.stringify(liveMessage('assistant', '', false, 2)))

    expect(sessions.messagesFor('s1')).toHaveLength(0)
  })

  it('settles an open streaming entry when the turn goes idle', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]

    sessions.handleEvent('s1', JSON.stringify(liveMessage('assistant', '答到一半', false, 1)))
    sessions.handleEvent(
      's1',
      JSON.stringify(historyEvent({ instanceId: 'inst-1', type: 'turn', sequence: 2, replayed: false, turn: { id: 'turn-1', phase: 'ended', outcome: 'completed' } })),
    )

    const entries = sessions.messagesFor('s1')
    expect(entries[0].streaming).toBe(false)
  })

  it('settles an open streaming entry when the turn ends in a failure', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1, { turn: 'running' })]

    sessions.handleEvent('s1', JSON.stringify(liveMessage('assistant', '答到一半', false, 1)))
    sessions.handleEvent(
      's1',
      JSON.stringify(historyEvent({ instanceId: 'inst-1', type: 'turn', sequence: 2, replayed: false, turn: { id: 'turn-1', phase: 'ended', outcome: 'failed' } })),
    )

    expect(sessions.messagesFor('s1')[0].streaming).toBe(false)
    expect(sessions.sessionById('s1')?.turn).toBe('idle')
  })

  it('leaves the turn running on an error event alone', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1, { turn: 'running' })]

    sessions.handleEvent(
      's1',
      JSON.stringify(historyEvent({ instanceId: 'inst-1', type: 'error', sequence: 2, replayed: false, error: { code: 'execution_failed', message: 'boom' } })),
    )

    expect(sessions.sessionById('s1')?.turn).toBe('running')
  })

  // History replay must keep its old semantics: two complete thinking blocks
  // in the transcript are two entries, never one merged block.
  it('keeps two adjacent replayed thinking blocks as two entries', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    stubRoutes({
      '/history': {
        events: [
          historyEvent({ type: 'thinking', sequence: 1, thinking: { text: '第一段', final: true } }),
          historyEvent({ type: 'thinking', sequence: 2, thinking: { text: '第二段', final: true } }),
        ],
        truncated: false,
      },
    })

    await sessions.loadHistory(1, 's1')

    const entries = sessions.messagesFor('s1')
    expect(entries.filter((entry) => entry.kind === 'thinking').map((entry) => entry.text)).toEqual([
      '第一段',
      '第二段',
    ])
  })
})

describe('sessions store per-message model', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    RecordingEventSource.instances = []
    vi.stubGlobal('EventSource', RecordingEventSource)
    useSessionsStore().unsubscribeAll()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function liveMessage(
    role: string,
    text: string,
    final: boolean,
    sequence: number,
    model?: { id: string; provider: string; variant?: string },
  ) {
    return historyEvent({
      instanceId: 'inst-1',
      type: 'message',
      sequence,
      replayed: false,
      message: { role, text, final, ...(model ? { model } : {}) },
    })
  }

  it('keeps the model an assistant message arrives with', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]

    sessions.handleEvent(
      's1',
      JSON.stringify(
        liveMessage('assistant', '答', true, 1, { id: 'prj/model-x', provider: 'prj', variant: 'fast' }),
      ),
    )

    const [entry] = sessions.messagesFor('s1')
    expect(entry.model).toEqual({ id: 'prj/model-x', provider: 'prj', variant: 'fast' })
  })

  it('keeps no model on a user message or a message sent without one', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]

    sessions.handleEvent('s1', JSON.stringify(liveMessage('user', '问', true, 1)))
    sessions.handleEvent('s1', JSON.stringify(liveMessage('assistant', '答', true, 2)))

    const entries = sessions.messagesFor('s1')
    expect(entries[0].model).toBeUndefined()
    expect(entries[1].model).toBeUndefined()
  })

  it('fills the model in when the final snapshot carries it', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]

    // The first delta may arrive before the adapter knows the session's
    // model; the final event carries the block's full snapshot, so it both
    // settles the text and completes the stamp.
    sessions.handleEvent('s1', JSON.stringify(liveMessage('assistant', 'A', false, 1)))
    sessions.handleEvent(
      's1',
      JSON.stringify(
        liveMessage('assistant', 'AB', true, 2, { id: 'prj/model-x', provider: 'prj' }),
      ),
    )

    const entries = sessions.messagesFor('s1')
    expect(entries).toHaveLength(1)
    expect(entries[0].text).toBe('AB')
    expect(entries[0].model).toEqual({ id: 'prj/model-x', provider: 'prj' })
  })

  it('replaces the accumulated prefix with the final full-text snapshot', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]

    // A viewer that joined mid-message holds a truncated prefix (from a
    // history snapshot or missed deltas); the final full-text snapshot
    // converges the block instead of appending to the fragment.
    sessions.handleEvent('s1', JSON.stringify(liveMessage('assistant', '答案的', false, 1)))
    sessions.handleEvent('s1', JSON.stringify(liveMessage('assistant', '答案的全文', true, 2)))

    const entries = sessions.messagesFor('s1')
    expect(entries).toHaveLength(1)
    expect(entries[0].text).toBe('答案的全文')
    expect(entries[0].streaming).toBe(false)
  })

  it('replays history with each message model', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    stubRoutes({
      '/history': {
        events: [
          historyEvent({ type: 'message', sequence: 1, message: { role: 'user', text: '历史问', final: true } }),
          historyEvent({
            type: 'message',
            sequence: 2,
            message: {
              role: 'assistant',
              text: '历史答',
              final: true,
              model: { id: 'acme/acme-code', provider: 'acme', variant: 'default' },
            },
          }),
        ],
        truncated: false,
      },
    })

    await sessions.loadHistory(1, 's1')

    const entries = sessions.messagesFor('s1')
    expect(entries[0].model).toBeUndefined()
    expect(entries[1].model).toEqual({ id: 'acme/acme-code', provider: 'acme', variant: 'default' })
  })
})

describe('sessions store user echo dedupe', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    RecordingEventSource.instances = []
    vi.stubGlobal('EventSource', RecordingEventSource)
    useSessionsStore().unsubscribeAll()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function stubSend() {
    stubRoutes({
      '/messages': {
        id: 's1',
        projectId: 1,
        backend: 'pi',
        instanceId: 'inst-1',
        nativeSessionId: 'native-s1',
        title: 's1',
        status: 'active',
        turn: 'running',
        capabilities: capabilities(),
        createdAt: '2026-10-09T00:00:00Z',
        updatedAt: '2026-10-09T00:00:00Z',
      },
    })
  }

  function liveUser(text: string, sequence: number) {
    return historyEvent({ instanceId: 'inst-1', type: 'message', sequence, replayed: false, message: { role: 'user', text, final: true } })
  }

  function userEntries(sessions: ReturnType<typeof useSessionsStore>) {
    return sessions.messagesFor('s1').filter((entry) => entry.role === 'user')
  }

  it('drops the backend echo of the turn just sent', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    useAuthStore().csrfToken = 'token'
    stubSend()

    await sessions.sendMessage(1, 's1', 'hello pi')
    sessions.handleEvent('s1', JSON.stringify(liveUser('hello pi', 1)))

    expect(userEntries(sessions)).toHaveLength(1)
  })

  it('still drops the echo when another entry landed in between', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    useAuthStore().csrfToken = 'token'
    stubSend()

    await sessions.sendMessage(1, 's1', 'hello pi')
    // pi reports a diagnostic on stderr before echoing the user turn; an
    // adjacency check lost the echo behind it and rendered the turn twice.
    sessions.handleEvent(
      's1',
      JSON.stringify(historyEvent({ instanceId: 'inst-1', type: 'error', sequence: 1, replayed: false, error: { code: 'stderr', message: 'warning: noisy' } })),
    )
    sessions.handleEvent('s1', JSON.stringify(liveUser('hello pi', 2)))

    expect(userEntries(sessions)).toHaveLength(1)
  })

  it('shows a later user turn with the same text once the echo was consumed', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    useAuthStore().csrfToken = 'token'
    stubSend()

    await sessions.sendMessage(1, 's1', 'again')
    sessions.handleEvent('s1', JSON.stringify(liveUser('again', 1)))
    // A steering/follow-up message with identical text is a real second turn.
    sessions.handleEvent('s1', JSON.stringify(liveUser('again', 2)))

    expect(userEntries(sessions)).toHaveLength(2)
  })

  it('updates session and sessionsByProject with the fresh updatedAt returned by sendMessage', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    const oldSession = session('s1', 1, { updatedAt: '2026-10-09T00:00:00Z' })
    sessions.sessions = [oldSession]
    sessions.sessionsByProject[1] = [oldSession]
    useAuthStore().csrfToken = 'token'

    const freshTime = '2026-10-20T15:30:00Z'
    stubRoutes({
      '/messages': {
        ...oldSession,
        updatedAt: freshTime,
      },
    })

    await sessions.sendMessage(1, 's1', 'hello world')

    expect(sessions.sessionById('s1')?.updatedAt).toBe(freshTime)
    expect(sessions.sessionsByProject[1][0].updatedAt).toBe(freshTime)
  })

  it('aligns session updatedAt from the latest timestamp in loaded history', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    const oldSession = session('s1', 1, { updatedAt: '2026-10-09T00:00:00Z' })
    sessions.sessions = [oldSession]
    sessions.sessionsByProject[1] = [oldSession]

    const latestMessageTime = '2026-10-20T16:00:00Z'
    stubRoutes({
      '/history': {
        events: [
          historyEvent({
            instanceId: 'inst-1',
            type: 'message',
            sequence: 1,
            timestamp: '2026-10-20T15:00:00Z',
            message: { role: 'user', text: 'hi', final: true },
          }),
          historyEvent({
            instanceId: 'inst-1',
            type: 'message',
            sequence: 2,
            timestamp: latestMessageTime,
            message: { role: 'assistant', text: 'hello!', final: true },
          }),
        ],
        truncated: false,
      },
    })

    await sessions.loadHistory(1, 's1')

    expect(sessions.sessionById('s1')?.updatedAt).toBe(latestMessageTime)
    expect(sessions.sessionsByProject[1][0].updatedAt).toBe(latestMessageTime)
  })
})

// The interrupt action answers from the session row as it stood when the
// interruption was accepted, while the event stream reports the interruption as
// soon as the agent stops. The stream is the fresher witness, and an interrupt
// can only end a turn — so the response must never rewind one.
describe('sessions store interrupt', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    RecordingEventSource.instances = []
    vi.stubGlobal('EventSource', RecordingEventSource)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  function interruptedEvent() {
    return historyEvent({
      instanceId: 'inst-1',
      type: 'turn',
      sequence: 3,
      replayed: false,
      turn: { id: 'turn-1', phase: 'ended', outcome: 'cancelled' },
    })
  }

  it('keeps a turn the stream settled while the response was still in flight', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1, { turn: 'running' })]
    useAuthStore().csrfToken = 'token'

    let settle!: (dto: AgentSession) => void
    const response = new Promise<AgentSession>((resolve) => {
      settle = resolve
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ status: 200, json: async () => response }) as unknown as Response),
    )

    const inFlight = sessions.interrupt(1, 's1')
    sessions.handleEvent('s1', JSON.stringify(interruptedEvent()))
    expect(sessions.sessionById('s1')?.turn).toBe('idle')

    // The stale answer arrives last and still claims the turn is running.
    settle(session('s1', 1, { turn: 'running' }))
    await inFlight

    expect(sessions.sessionById('s1')?.turn).toBe('idle')
  })

  it('takes the settled turn the response carries when the stream has not spoken', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1, { turn: 'running' })]
    useAuthStore().csrfToken = 'token'
    stubRoutes({ '/interrupt': session('s1', 1, { turn: 'idle' }) })

    await sessions.interrupt(1, 's1')

    expect(sessions.sessionById('s1')?.turn).toBe('idle')
  })
})

// What the browser actually does when the agent process behind a session dies:
// it fires one error and then sits in CONNECTING forever, dialling never again.
// A stream in that state is not a live subscription, so the next time the user
// opens the session it must be replaced rather than reused.
describe('sessions store stale stream', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    RecordingEventSource.instances = []
    vi.stubGlobal('EventSource', RecordingEventSource)
    useSessionsStore().unsubscribeAll()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('replaces the stream of a session the user comes back to after a drop', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    stubRoutes({
      '/continue': session('s1', 1),
      '/history': { events: [], truncated: false },
    })

    await sessions.openSession(1, 's1')
    const first = RecordingEventSource.instances[0]
    first.readyState = RecordingEventSource.OPEN
    first.onopen?.()
    expect(sessions.statusFor('s1')).toBe('open')

    // The drop: one error, the browser still claims it is connecting.
    first.readyState = RecordingEventSource.CONNECTING
    first.onerror?.()
    expect(sessions.statusFor('s1')).toBe('reconnecting')

    // The user opens the session again.
    await sessions.openSession(1, 's1')

    expect(first.readyState).toBe(RecordingEventSource.CLOSED)
    expect(RecordingEventSource.instances).toHaveLength(2)
    expect(RecordingEventSource.instances[1].url).toContain('/api/events/')
  })

  it('leaves a stream that never dropped alone when the session is reopened', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    stubRoutes({
      '/continue': session('s1', 1),
      '/history': { events: [], truncated: false },
    })

    await sessions.openSession(1, 's1')
    const first = RecordingEventSource.instances[0]
    first.readyState = RecordingEventSource.OPEN
    first.onopen?.()

    await sessions.openSession(1, 's1')

    expect(RecordingEventSource.instances).toHaveLength(1)
    expect(first.readyState).toBe(RecordingEventSource.OPEN)
  })
})

// The drop does not always arrive while the user is away: the agent process can
// die a moment after the session was opened, while they are looking right at it.
// The browser will not dial again, so the store makes one repair attempt of its
// own — and only one, because an operator who stopped a service deliberately
// must not find the platform dialling it on a loop. After that the user decides.
describe('sessions store stall repair', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    RecordingEventSource.instances = []
    vi.stubGlobal('EventSource', RecordingEventSource)
    vi.useFakeTimers()
    useSessionsStore().unsubscribeAll()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('re-dials a stalled stream once, then leaves the decision to the user', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    stubRoutes({
      '/continue': session('s1', 1),
      '/history': { events: [], truncated: false },
    })

    await sessions.openSession(1, 's1')
    const first = RecordingEventSource.instances[0]
    first.readyState = RecordingEventSource.OPEN
    first.onopen?.()

    first.readyState = RecordingEventSource.CONNECTING
    first.onerror?.()
    expect(RecordingEventSource.instances).toHaveLength(1)

    // Nothing happens before the stall window closes...
    vi.advanceTimersByTime(2000)
    expect(RecordingEventSource.instances).toHaveLength(1)

    // ...and one repair after it.
    vi.advanceTimersByTime(2000)
    await vi.advanceTimersByTimeAsync(0)
    expect(RecordingEventSource.instances).toHaveLength(2)
    expect(first.readyState).toBe(RecordingEventSource.CLOSED)

    // The repair itself fails: no second attempt behind the user's back.
    const second = RecordingEventSource.instances[1]
    second.readyState = RecordingEventSource.CONNECTING
    second.onerror?.()
    vi.advanceTimersByTime(30_000)
    await vi.advanceTimersByTimeAsync(0)
    expect(RecordingEventSource.instances).toHaveLength(2)
    expect(sessions.statusFor('s1')).toBe('reconnecting')
  })

  it('does not re-dial a stream that was closed on purpose', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    stubRoutes({
      '/continue': session('s1', 1),
      '/history': { events: [], truncated: false },
    })
    await sessions.openSession(1, 's1')
    sessions.unsubscribe('s1')

    vi.advanceTimersByTime(30_000)
    await vi.advanceTimersByTimeAsync(0)

    expect(RecordingEventSource.instances).toHaveLength(1)
    expect(sessions.statusFor('s1')).toBe('idle')
  })
})

describe('sessions store composer model', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('remembers a model per session so switching sessions restores it', () => {
    const sessions = useSessionsStore()

    sessions.setModel('s1', 'pi/a')
    sessions.setModel('s2', 'pi/b')

    expect(sessions.modelFor('s1')).toBe('pi/a')
    expect(sessions.modelFor('s2')).toBe('pi/b')
    expect(sessions.modelFor('never-opened')).toBe('')
  })

  it('drops the remembered model when the session is deleted', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    sessions.setModel('s1', 'pi/a')
    stubRoutes({ '/sessions/s1': {} })

    await sessions.deleteSession(1, 's1')

    expect(sessions.modelFor('s1')).toBe('')
  })

  it('clears remembered models when a different project is loaded', async () => {
    const pending = deferredLists()
    const sessions = useSessionsStore()
    const first = sessions.loadSessions(1)
    pending.get(1)!([session('a-1', 1)])
    await first
    sessions.setModel('a-1', 'pi/a')

    const second = sessions.loadSessions(2)
    expect(sessions.modelFor('a-1')).toBe('')
    pending.get(2)!([session('b-1', 2)])
    await second
  })

  // A session's own model comes from the per-message stamp the backend now
  // sends: opening a session shows the model it actually ran, not the backend's
  // first, even before the user has picked anything in the composer.
  it('defaults to the model the session actually used', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    // The picker's own first-model fallback must not outrank the used model.
    sessions.setAutoModel('s1', 'pi/first')
    sessions.handleEvent(
      's1',
      JSON.stringify(
        historyEvent({
          instanceId: 'inst-1',
          type: 'message',
          sequence: 1,
          message: {
            role: 'assistant',
            text: '答',
            final: true,
            model: { id: 'pi/used', provider: 'pi' },
          },
        }),
      ),
    )

    expect(sessions.modelFor('s1')).toBe('pi/used')
  })

  it('keeps the latest assistant model and lets an explicit choice outrank it', () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    const assistant = (sequence: number, id: string) =>
      JSON.stringify(
        historyEvent({
          instanceId: 'inst-1',
          type: 'message',
          sequence,
          message: { role: 'assistant', text: '答', final: true, model: { id, provider: 'pi' } },
        }),
      )
    sessions.handleEvent('s1', assistant(1, 'pi/old'))
    sessions.handleEvent('s1', assistant(2, 'pi/new'))
    expect(sessions.modelFor('s1')).toBe('pi/new')

    // The user picked a model for this session: their choice wins over the
    // session's, even after another model answered.
    sessions.setModel('s1', 'pi/picked')
    expect(sessions.modelFor('s1')).toBe('pi/picked')
  })

  it('drops the picker fallback with the session too', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    sessions.setAutoModel('s1', 'pi/a')
    stubRoutes({ '/sessions/s1': {} })

    await sessions.deleteSession(1, 's1')

    expect(sessions.modelFor('s1')).toBe('')
  })

  // The new-session form chooses the model before the session exists, so no
  // assistant message carries one yet. The choice must still be what the
  // composer shows, rather than the picker's own first-model fallback.
  it('shows the model chosen at creation before the agent replies', async () => {
    const sessions = useSessionsStore()
    stubRoutes({ '/sessions': { session: session('s1', 1) } })
    // Opening the created conversation makes the picker fall back to the
    // backend's first model; that must not outrank the created model.
    sessions.setAutoModel('s1', 'pi/first')

    const created = await sessions.createSession(1, { backend: 'pi', model: 'pi/chosen' })

    expect(created?.id).toBe('s1')
    expect(sessions.modelFor('s1')).toBe('pi/chosen')
  })

  // Opening the session loads its project, which resets the composer's
  // per-session choices. The created session's model belongs to that project
  // and must survive the switch to it.
  it('keeps the creation model when the session project is loaded', async () => {
    const sessions = useSessionsStore()
    stubRoutes({ '/sessions': { session: session('s1', 1) } })
    await sessions.createSession(1, { backend: 'pi', model: 'pi/chosen' })

    const pending = deferredLists()
    const opened = sessions.loadSessions(1)
    pending.get(1)!([session('s1', 1)])
    await opened

    expect(sessions.modelFor('s1')).toBe('pi/chosen')
  })
})

// A first message admitted at create time rides the create request, so its
// echo arrives on the stream without a composer send ever happening: the
// echo dedupe must hold it all the same, and a refused message must keep
// its text for the composer to offer again.
describe('sessions store create with first message', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function stubCreate(payload: unknown) {
    stubRoutes({ '/sessions': payload })
  }

  it('holds the echo of a first message admitted at create time', async () => {
    const sessions = useSessionsStore()
    stubCreate({ session: session('s1', 1) })

    await sessions.createSession(1, { backend: 'pi', message: 'hello pi' })
    expect(sessions.echoPending['s1']).toBe('hello pi')

    // The user turn in the log comes from the history replay, not from an
    // optimistic composer entry.
    sessions.messages['s1'] = [
      { id: 'e1', kind: 'message', role: 'user', text: 'hello pi', streaming: false },
    ]
    sessions.handleEvent(
      's1',
      JSON.stringify(historyEvent({ instanceId: 'inst-1', type: 'message', sequence: 1, replayed: false, message: { role: 'user', text: 'hello pi', final: true } })),
    )
    expect(sessions.messagesFor('s1').filter((entry) => entry.role === 'user')).toHaveLength(1)
  })

  it('records a refused first message and consumes it once', async () => {
    const sessions = useSessionsStore()
    stubCreate({ session: session('s1', 1), messageError: 'agent_operation_failed' })

    const created = await sessions.createSession(1, { backend: 'pi', message: 'hello pi' })
    expect(created?.id).toBe('s1')
    expect(sessions.firstMessageFailures['s1']).toEqual({
      text: 'hello pi',
      error: 'agent_operation_failed',
    })

    const record = sessions.consumeFirstMessageFailure('s1')
    expect(record).toEqual({ text: 'hello pi', error: 'agent_operation_failed' })
    expect(sessions.consumeFirstMessageFailure('s1')).toBeNull()
  })
})

// A turn that finished before the stream opened leaves entries streaming:
// history replayed an in-flight snapshot and the final full-text event was
// broadcast to nobody. One history re-fetch settles them.
describe('sessions store dangling entry repair', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('settles a streamed assistant entry from the final history snapshot', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    sessions.messages['s1'] = [
      { id: 'e1', kind: 'message', role: 'user', text: '问', streaming: false },
      { id: 'e2', kind: 'message', role: 'assistant', text: '部分的', streaming: true },
    ]
    stubRoutes({
      '/history': {
        events: [
          messageEvent('user', '问', 1),
          messageEvent('assistant', '部分的回答全文', 2),
        ],
        truncated: false,
      },
    })

    await sessions.repairDanglingEntries(1, 's1')

    const entries = sessions.messagesFor('s1')
    expect(entries[1].text).toBe('部分的回答全文')
    expect(entries[1].streaming).toBe(false)
  })

  it('leaves entries alone while the turn is still running', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1, { turn: 'running' })]
    sessions.messages['s1'] = [
      { id: 'e1', kind: 'message', role: 'assistant', text: '部分的', streaming: true },
    ]
    const calls = stubRoutes({ '/history': { events: [], truncated: false } })

    await sessions.repairDanglingEntries(1, 's1')

    expect(calls).toHaveLength(0)
    expect(sessions.messagesFor('s1')[0].streaming).toBe(true)
  })

  it('does nothing when every entry already settled', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    sessions.messages['s1'] = [
      { id: 'e1', kind: 'message', role: 'assistant', text: '完成', streaming: false },
    ]
    const calls = stubRoutes({ '/history': { events: [], truncated: false } })

    await sessions.repairDanglingEntries(1, 's1')

    expect(calls).toHaveLength(0)
  })

  it('aborts the repair when entries changed while history was in flight', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    sessions.messages['s1'] = [
      { id: 'e1', kind: 'message', role: 'assistant', text: '部分的', streaming: true },
    ]
    // The history response resolves only after a live entry landed, so the
    // stale snapshot must never overwrite what the stream just delivered.
    let resolveHistory: (value: unknown) => void = () => {}
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            resolveHistory = (payload) =>
              resolve({ status: 200, json: async () => payload } as Response)
          }),
      ),
    )

    const repair = sessions.repairDanglingEntries(1, 's1')
    sessions.messages['s1']!.push({
      id: 'e2',
      kind: 'message',
      role: 'user',
      text: '新消息',
      streaming: false,
    })
    resolveHistory({ events: [messageEvent('assistant', '过期的全文', 2)], truncated: false })
    await repair

    const entries = sessions.messagesFor('s1')
    expect(entries[0].text).toBe('部分的')
    expect(entries[0].streaming).toBe(true)
  })
})

// A listener-driven EventSource stand-in: tests can deliver frames shaped
// like the browser's MessageEvents, including the SSE id the browser exposes
// as lastEventId and the server turns back into a resume position.
class StreamingEventSource {
  static instances: StreamingEventSource[] = []
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 2

  readyState = StreamingEventSource.OPEN
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  private listeners = new Map<string, ((event: MessageEvent) => void)[]>()

  constructor(readonly url: string) {
    StreamingEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    const list = this.listeners.get(type) ?? []
    list.push(listener)
    this.listeners.set(type, list)
  }

  close() {
    this.readyState = StreamingEventSource.CLOSED
  }

  emit(type: string, data: string, lastEventId = '') {
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data, lastEventId } as MessageEvent)
    }
  }
}

function liveMessage(text: string, sequence: number) {
  return JSON.stringify({
    instanceId: 'inst-1',
    sessionId: 's1',
    backend: 'pi',
    sequence,
    timestamp: '2026-10-14T12:00:00Z',
    type: 'message',
    message: { role: 'assistant', text, final: true },
  })
}

describe('sessions store stream resume', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    StreamingEventSource.instances = []
    vi.stubGlobal('EventSource', StreamingEventSource)
    useSessionsStore().unsubscribeAll()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('re-dials with the last applied frame id so the server replays the gap', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    const subscriptionCalls: Array<{ lastEventId?: number }> = []

    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        if (url.includes('/subscriptions') && init?.method === 'POST') {
          const body = init.body ? JSON.parse(String(init.body)) : {}
          subscriptionCalls.push(body)
          return {
            status: 200,
            json: async () => ({
              sessionId: 's1',
              turn: { id: 'turn-1', phase: 'ended', outcome: 'completed' },
              interactions: { permissions: [], forms: [] },
              resync: false,
            }),
          } as Response
        }
        return { status: 200, json: async () => ({ success: true }) } as Response
      }),
    )

    await sessions.subscribe(1, 's1')
    StreamingEventSource.instances[0].emit('message', liveMessage('七', 7), '7')
    await sessions.unsubscribe('s1')
    await sessions.subscribe(1, 's1')

    // On re-subscription, the last applied frame id (7) is passed to the server so it can replay
    expect(subscriptionCalls).toHaveLength(2)
    expect(subscriptionCalls[1].lastEventId).toBe(7)
    // The replayed entry still renders into the same log.
    expect(sessions.messagesFor('s1').map((entry) => entry.text)).toContain('七')
  })

  it('rebuilds the log from history when the server reports an unreplayable gap', async () => {
    const sessions = useSessionsStore()
    sessions.projectId = 1
    sessions.sessions = [session('s1', 1)]
    const calls = stubRoutes({
      '/continue': session('s1', 1),
      '/history': { events: [messageEvent('assistant', '完整的历史', 9)], truncated: false },
    })

    await sessions.subscribe(1, 's1')
    const first = StreamingEventSource.instances[0]
    first.emit('message', liveMessage('断流前的半句', 10), '10')
    expect(sessions.messagesFor('s1').map((entry) => entry.text)).toContain('断流前的半句')

    first.emit('resync', '{}')
    await vi.waitFor(() => {
      expect(sessions.messagesFor('s1').map((entry) => entry.text)).toEqual(['完整的历史'])
    })

    // The broken log and its stale resume position are gone: the reopened
    // stream dials fresh and history was re-fetched.
    expect(calls).toContain('/api/projects/1/sessions/s1/history')
    expect(StreamingEventSource.instances[0].url).toContain('/api/events/')
  })
})

describe('sessions store cross-project turn state', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('keeps running session ids across projects and updates via setTurn', async () => {
    const sessions = useSessionsStore()

    // Load project 1 with a running session A1 and idle session A2
    sessions.loadSessions = vi.fn().mockImplementation(async (projectId: number) => {
      sessions.projectId = projectId
      if (projectId === 1) {
        const list = [
          session('A1', 1, { turn: 'running' }),
          session('A2', 1, { turn: 'idle' }),
        ]
        sessions.recordTurns(list)
        sessions.sessions = list
        sessions.sessionsByProject[1] = list
      } else if (projectId === 2) {
        const list = [
          session('B1', 2, { turn: 'running' }),
          session('B2', 2, { turn: 'running' }),
        ]
        sessions.recordTurns(list)
        sessions.sessions = list
        sessions.sessionsByProject[2] = list
      }
    })

    await sessions.loadSessions(1)
    expect(sessions.runningSessionIds).toEqual(['A1'])

    // Now switch to project 2
    await sessions.loadSessions(2)
    // Both project 1's A1 and project 2's B1, B2 should be included in runningSessionIds!
    expect(sessions.runningSessionIds).toContain('A1')
    expect(sessions.runningSessionIds).toContain('B1')
    expect(sessions.runningSessionIds).toContain('B2')

    // Background session A1 finishes in project 1
    sessions.setTurn('A1', 'idle')
    expect(sessions.runningSessionIds).not.toContain('A1')
    expect(sessions.runningSessionIds).toContain('B1')
    expect(sessions.runningSessionIds).toContain('B2')

    // And sessionById for background session A1 reflects idle
    expect(sessions.sessionById('A1')?.turn).toBe('idle')
  })
})

