// Unit tests for the session chat panel: a directly opened session URL loads its
// project's sessions first, an unknown session is reported rather than subscribed,
// switching sessions closes the previous stream and hides its messages, and
// unsupported controls are not rendered.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import BackendTag from '../components/BackendTag.vue'
import SessionChatPanel from '../components/SessionChatPanel.vue'
import { useAgentsStore } from '../stores/agents'
import { backendFixtures } from './backend-fixtures'
import { useSessionsStore, type ChatEntry } from '../stores/sessions'
import { useOpencodeRawStore } from '../stores/opencodeRaw'
import { clearModelCache } from '../modelCache'
import type { AgentInstance } from '../api/agents'
import type { AgentSession, Capabilities } from '../api/sessions'

function capabilities(overrides: Partial<Capabilities> = {}): Capabilities {
  return {
    streaming: true,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: true,
    interrupt: false,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
    sessionHistory: false,
    rawEvents: false,
    ...overrides,
  }
}

// continues lists the continue calls a stub fetch recorded, so a test can count
// reconnects rather than total traffic.
function continues(fetchSpy: ReturnType<typeof vi.fn>): string[] {
  return (fetchSpy.mock.calls as unknown as [string][]).map((call) => String(call[0])).filter((url) => url.endsWith('/continue'))
}

function session(id: string, overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id,
    projectId: 1,
    backend: 'opencode-v2',
    instanceId: 'inst-1',
    nativeSessionId: `native-${id}`,
    title: `Title ${id}`,
    status: 'active',
    turn: 'idle',
    capabilities: capabilities(),
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
    ...overrides,
  }
}

// Records every EventSource the store opens, so a test can prove which streams
// are live and which were closed on switch.
class FakeEventSource {
  static instances: FakeEventSource[] = []
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2

  readonly url: string
  readyState = FakeEventSource.CONNECTING
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }

  addEventListener() {}

  close() {
    this.readyState = FakeEventSource.CLOSED
  }
}

// stubSessionList serves every request opening a session makes: the project's
// session list, the continue that re-attaches the session, and the history
// replay. Routing by path keeps each response shaped as its endpoint promises.
function stubSessionList(
  list: AgentSession[],
  history: { events: unknown[]; truncated: boolean } = { events: [], truncated: false },
) {
  const fetchSpy = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input)
    if (url.endsWith('/history')) return { status: 200, json: async () => history } as Response
    if (url.endsWith('/continue')) {
      const id = url.split('/')[5]
      const found = list.find((entry) => entry.id === id) ?? list[0]
      return { status: 200, json: async () => found } as Response
    }
    return { status: 200, json: async () => ({ sessions: list }) } as Response
  })
  vi.stubGlobal('fetch', fetchSpy)
  return fetchSpy
}

// The panel links back to the workbench (a project has no page of its own), so
// the test router knows both shapes.
function testRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { render: () => null } },
      { path: '/projects/:projectId/settings', name: 'project-settings', component: { render: () => null } },
      { path: '/projects/:projectId/sessions/:sessionId', name: 'session', component: { render: () => null } },
    ],
  })
}

function mountPanel(sessionId: string, router = testRouter()) {
  return mount(SessionChatPanel, {
    props: { projectId: 1, sessionId },
    global: { plugins: [router] },
  })
}

function instance(status: AgentInstance['status']): AgentInstance {
  return {
    id: 'inst-1',
    projectId: 1,
    backend: 'opencode-v2',
    directory: '/workspace/alpha',
    status,
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
  }
}

function entry(id: string, text: string): ChatEntry {
  return { id, kind: 'message', role: 'assistant', text, streaming: false } as ChatEntry
}

// jsdom performs no layout, so the tests hand the scroller the geometry it
// would have in a browser, and change scrollHeight to simulate growth.
function setGeometry(
  el: HTMLElement,
  geometry: { scrollHeight: number; clientHeight: number },
) {
  Object.defineProperty(el, 'scrollHeight', {
    configurable: true,
    writable: true,
    value: geometry.scrollHeight,
  })
  Object.defineProperty(el, 'clientHeight', {
    configurable: true,
    writable: true,
    value: geometry.clientHeight,
  })
}

describe('SessionChatPanel', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    const agents = useAgentsStore()
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined as never)
  })

  afterEach(() => {
    // The store keeps open streams in module scope; close them so a stream
    // opened by one test cannot make the next test skip its subscription.
    useSessionsStore().unsubscribeAll()
    clearModelCache()
    vi.unstubAllGlobals()
  })

  it('loads the project sessions first when a session URL is opened directly', async () => {
    const fetchSpy = stubSessionList([session('sess-1'), session('sess-2')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    expect(fetchSpy).toHaveBeenCalled()
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(FakeEventSource.instances[0].url).toContain('/sessions/sess-1/')
    expect(wrapper.text()).toContain('Title sess-1')
  })

describe('SessionChatPanel opencode approvals', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    const agents = useAgentsStore()
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined as never)
    // Keep the raw fetch stream out of these tests: approval state is driven
    // directly on the store.
    vi.spyOn(useOpencodeRawStore(), 'subscribeRaw').mockImplementation(() => {})
  })

  afterEach(() => {
    useSessionsStore().unsubscribeAll()
    useOpencodeRawStore().unsubscribeAllRaw()
    clearModelCache()
    vi.unstubAllGlobals()
  })

  function opencodeSession(id: string): AgentSession {
    return session(id, { capabilities: capabilities({ toolConfirmation: true, rawEvents: true }) })
  }

  it('opens the raw event stream only when the backend declares it', async () => {
    const subscribe = vi.spyOn(useOpencodeRawStore(), 'subscribeRaw')
    stubSessionList([session('sess-1', { capabilities: capabilities({ toolConfirmation: true, rawEvents: false }) })])
    mountPanel('sess-1')
    await flushPromises()
    expect(subscribe).not.toHaveBeenCalled()
  })

  it('polls approvals while a turn runs for a backend without raw events', async () => {
    vi.useFakeTimers()
    try {
      stubSessionList([
        session('sess-1', { turn: 'running', capabilities: capabilities({ toolConfirmation: true, rawEvents: false }) }),
      ])
      const load = vi.spyOn(useOpencodeRawStore(), 'loadApprovals')
      mountPanel('sess-1')
      await vi.advanceTimersByTimeAsync(0)
      const initial = load.mock.calls.length
      await vi.advanceTimersByTimeAsync(1600)
      expect(load.mock.calls.length).toBeGreaterThan(initial)
    } finally {
      vi.useRealTimers()
    }
  })

  it('subscribes to the raw event stream when the backend declares raw events', async () => {
    const subscribe = vi.spyOn(useOpencodeRawStore(), 'subscribeRaw')
    stubSessionList([opencodeSession('sess-1')])
    mountPanel('sess-1')
    await flushPromises()
    expect(subscribe).toHaveBeenCalledWith(expect.any(Number), 'sess-1', expect.any(String))
  })

  it('shows no confirmation for a dangling tool_call on opencode sessions', async () => {
    stubSessionList([opencodeSession('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    // A tool_call with no result is NOT a permission: replying with its id
    // can never succeed (server 404/400, surfaced as 502), so no control.
    // With no pending approval the confirmation section hides entirely.
    useSessionsStore().pushEntry('sess-1', {
      id: 't1',
      kind: 'tool_call',
      role: 'assistant',
      text: 'read',
      streaming: false,
      callId: 'call_1',
      name: 'read',
    } as ChatEntry)
    await flushPromises()

    expect(wrapper.find('[data-test="tool-confirmation"]').exists()).toBe(false)
    expect(wrapper.find('button.confirm-tool').exists()).toBe(false)
    expect(wrapper.find('button.deny-tool').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('没有待确认')
  })

  it('shows confirmation only for real pending approvals', async () => {
    stubSessionList([opencodeSession('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()
    // No pending approval: no confirmation UI and no tip.
    expect(wrapper.find('[data-test="tool-confirmation"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('没有待确认')

    useOpencodeRawStore().permissions['sess-1'] = [
      { id: 'per_1', sessionId: 'sess-1', action: 'write', resources: ['/tmp/x'] },
    ]
    await flushPromises()

    expect(wrapper.find('[data-test="tool-confirmation"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('write')
  })

  it('renders pending question forms as answerable cards', async () => {
    stubSessionList([opencodeSession('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()
    expect(wrapper.find('[data-test="question-form"]').exists()).toBe(false)

    useOpencodeRawStore().forms['sess-1'] = [
      {
        id: 'frm_1',
        sessionId: 'sess-1',
        title: '选一个',
        fields: [{ key: 'scope', type: 'string', title: '范围', required: true }],
      },
    ]
    await flushPromises()

    const card = wrapper.find('[data-test="question-form"]')
    expect(card.exists()).toBe(true)
    await card.find('.text-input').setValue('大')
    await card.find('[data-test="submit-answer"]').trigger('click')
    await flushPromises()

    expect(useOpencodeRawStore().formsFor('sess-1')).toHaveLength(0)
  })
})

  it('reports an unknown session and opens no stream', async () => {
    stubSessionList([session('sess-1')])
    const wrapper = mountPanel('missing')
    await flushPromises()

    expect(wrapper.text()).toContain('会话不存在')
    expect(wrapper.find('a').attributes('href')).toBe('/')
    expect(FakeEventSource.instances).toHaveLength(0)
  })

  it('shows a failed project load as an error with a way back, not as a missing session', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ status: 500, json: async () => ({}) }) as Response),
    )
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    expect(wrapper.find('[role="alert"]').text()).toContain('发生未知错误。')
    expect(wrapper.text()).not.toContain('会话不存在')
    expect(wrapper.find('a').attributes('href')).toBe('/')
  })

  it('does not block the composer on an instance the project list calls stopped', async () => {
    stubSessionList([session('sess-1')])
    useAgentsStore().instances = [instance('stopped')]
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    // Opening the session repairs the session, so a stale instance status in the
    // project list no longer gates sending.
    expect(wrapper.text()).not.toContain('实例已停止')
    await wrapper.find('#message-text').setValue('hello')
    expect(wrapper.find('button.send-button').attributes('disabled')).toBeUndefined()
  })

  it('opens the session with openSession rather than subscribing directly', async () => {
    const fetchSpy = stubSessionList([session('sess-1')])
    const store = useSessionsStore()
    const openSession = vi.spyOn(store, 'openSession')

    mountPanel('sess-1')
    await flushPromises()

    expect(openSession).toHaveBeenCalledWith(1, 'sess-1')
    const urls = fetchSpy.mock.calls.map((call) => String(call[0]))
    expect(urls.some((url) => url.endsWith('/continue'))).toBe(true)
    expect(urls.some((url) => url.endsWith('/history'))).toBe(true)
  })

  it('renders no instance identifier in the header', async () => {
    stubSessionList([session('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    expect(wrapper.text()).not.toContain('实例：')
    expect(wrapper.text()).not.toContain('inst-1')
  })

  it('shows the replayed history with a truncation note', async () => {
    stubSessionList([session('sess-1')], {
      events: [
        {
          instanceId: 'inst-1',
          sessionId: 'native-sess-1',
          backend: 'opencode-v2',
          sequence: 1,
          timestamp: '2026-10-09T00:00:00Z',
          type: 'message',
          replayed: true,
          message: { role: 'assistant', text: 'OLD REPLAYED', final: true },
        },
      ],
      truncated: true,
    })
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    expect(wrapper.text()).toContain('OLD REPLAYED')
    expect(wrapper.find('[data-test="history-truncated"]').text()).toContain('历史')
  })

  it('offers a retry when reading history fails', async () => {
    let historyCalls = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.endsWith('/history')) {
          historyCalls += 1
          if (historyCalls === 1) {
            return { status: 502, json: async () => ({ error: 'agent_operation_failed' }) } as Response
          }
          return { status: 200, json: async () => ({ events: [], truncated: false }) } as Response
        }
        if (url.endsWith('/continue')) return { status: 200, json: async () => session('sess-1') } as Response
        return { status: 200, json: async () => ({ sessions: [session('sess-1')] }) } as Response
      }),
    )
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const retry = wrapper.find('[data-test="history-retry"]')
    expect(retry.exists()).toBe(true)

    await retry.trigger('click')
    await flushPromises()

    expect(historyCalls).toBe(2)
  })

  it('explains a session that stays disconnected and offers to retry', async () => {
    const fetchSpy = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/history')) {
        return { status: 200, json: async () => ({ events: [], truncated: false }) } as Response
      }
      if (url.endsWith('/continue')) {
        return { status: 502, json: async () => ({ error: 'instance_start_failed' }) } as Response
      }
      return {
        status: 200,
        json: async () => ({ sessions: [session('sess-1', { status: 'disconnected', backend: 'pi' })] }),
      } as Response
    })
    vi.stubGlobal('fetch', fetchSpy)
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    // A pi session has no service to blame, so it keeps the plain retry affordance:
    // the backend reports a code, not prose, and the note shows the platform's
    // message for that code.
    const note = wrapper.find('[data-test="session-unavailable"]')
    expect(note.text()).toContain('Agent 实例启动失败')
    expect(note.text()).not.toContain('service')
    expect(wrapper.find('[data-test="session-retry"]').exists()).toBe(true)

    const before = fetchSpy.mock.calls.length
    await wrapper.find('[data-test="session-retry"]').trigger('click')
    await flushPromises()
    expect(fetchSpy.mock.calls.length).toBeGreaterThan(before)
  })

  // The opencode service is a third-party process the platform never starts.
  // So the panel says what is missing, points at the one page that can start it,
  // and offers a reconnect the user clicks — it never retries by itself.
  it('points an unreachable opencode service at platform settings and offers reconnect', async () => {
    useAgentsStore().backends = backendFixtures()
    const fetchSpy = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/history')) {
        return { status: 200, json: async () => ({ events: [], truncated: false }) } as Response
      }
      if (url.endsWith('/continue')) {
        return { status: 409, json: async () => ({ error: 'service_not_running' }) } as Response
      }
      return {
        status: 200,
        json: async () => ({ sessions: [session('sess-1', { status: 'disconnected' })] }),
      } as Response
    })
    vi.stubGlobal('fetch', fetchSpy)
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const note = wrapper.find('[data-test="session-unavailable"]')
    expect(note.text()).toContain('无法连接 opencode service')
    expect(note.text()).toContain('平台设置 → Agent')
    expect(note.text()).not.toContain('实例')
    const reconnect = wrapper.find('[data-test="session-reconnect"]')
    expect(reconnect.text()).toBe('重新连接')

    const continuedBefore = continues(fetchSpy)
    await reconnect.trigger('click')
    await flushPromises()

    // Exactly one reconnect per click, on this session — the same endpoint the
    // platform already used to open it.
    expect(continues(fetchSpy).length).toBe(continuedBefore.length + 1)
    expect(continues(fetchSpy)).toContain('/api/projects/1/sessions/sess-1/continue')
  })

  it('keeps the typed message and reports the error when sending fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        if (url.endsWith('/messages')) {
          return { status: 500, json: async () => ({}) } as Response
        }
        if (url.endsWith('/continue')) {
          return { status: 200, json: async () => session('sess-1') } as Response
        }
        if (url.endsWith('/history')) {
          return { status: 200, json: async () => ({ events: [], truncated: false }) } as Response
        }
        return { status: 200, json: async () => ({ sessions: [session('sess-1')] }) } as Response
      }),
    )
    useAgentsStore().instances = [instance('running')]
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    await wrapper.find('#message-text').setValue('hello')
    await wrapper.find('form.send').trigger('submit')
    await flushPromises()

    expect((wrapper.find('#message-text').element as HTMLTextAreaElement).value).toBe('hello')
    expect(wrapper.find('[role="alert"]').text()).toContain('发生未知错误。')
  })

  // The browser does not always recover a dropped stream by itself: when the
  // process behind the session dies it fires one error and then sits in
  // CONNECTING forever. A conversation stuck on 正在重连 with no way out is a
  // dead end, so the stalled state offers the same reconnect the failed one
  // does, and clicking it opens a fresh stream rather than reusing the old.
  it('offers a reconnect while the stream is stalled in reconnecting', async () => {
    stubSessionList([session('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const source = FakeEventSource.instances[0]
    source.readyState = FakeEventSource.OPEN
    source.onopen?.()
    source.readyState = FakeEventSource.CONNECTING
    source.onerror?.()
    await flushPromises()
    expect(wrapper.find('[data-test=\"conversation\"]').attributes('data-stream-status')).toBe(
      'reconnecting',
    )

    const reconnect = wrapper.find('[data-test=\"stream-reconnect\"]')
    expect(reconnect.exists()).toBe(true)
    await reconnect.trigger('click')
    await flushPromises()

    expect(source.readyState).toBe(FakeEventSource.CLOSED)
    expect(FakeEventSource.instances).toHaveLength(2)
    expect(wrapper.find('[data-test=\"conversation\"]').attributes('data-stream-status')).toBe(
      'connecting',
    )
  })

  it('keeps the previous stream open and shows the new messages when the session switches', async () => {
    stubSessionList([session('sess-1'), session('sess-2')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    useSessionsStore().pushEntry('sess-1', entry('m1', 'OLD MESSAGE'))
    await flushPromises()
    expect(wrapper.text()).toContain('OLD MESSAGE')

    await wrapper.setProps({ sessionId: 'sess-2' })
    await flushPromises()

    // Tabs keep both sessions live: the previous stream stays open for its tab.
    expect(FakeEventSource.instances).toHaveLength(2)
    expect(FakeEventSource.instances[1].url).toContain('/sessions/sess-2/')
    expect(wrapper.text()).not.toContain('OLD MESSAGE')
  })

  it('shows the backend tag and no rename or continue controls in the header', async () => {
    stubSessionList([session('sess-1', { backend: 'pi' })])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const tag = wrapper.findComponent(BackendTag)
    expect(tag.exists()).toBe(true)
    expect(tag.props('backend')).toBe('pi')
    expect(wrapper.findAll('button').some((button) => button.text() === '重命名')).toBe(false)
    expect(wrapper.findAll('button').some((button) => button.text() === '继续会话')).toBe(false)
  })

  it('renders no model picker when the session does not declare modelSelection', async () => {
    stubSessionList([
      session('sess-1', { capabilities: capabilities({ modelSelection: false, interrupt: true }) }),
    ])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    expect(wrapper.find('.model-select').exists()).toBe(false)
  })

  it('offers send while the turn is idle', async () => {
    stubSessionList([session('sess-1', { turn: 'idle' })])
    useAgentsStore().instances = [instance('running')]
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const button = wrapper.find('button.send-button')
    expect(button.exists()).toBe(true)
    expect(button.text()).toBe('发送')
    expect(wrapper.find('button.stop-button').exists()).toBe(false)
  })

  it('offers stop on a running turn and interrupts the session on click', async () => {
    stubSessionList([
      session('sess-1', { turn: 'running', capabilities: capabilities({ interrupt: true }) }),
    ])
    useAgentsStore().instances = [instance('running')]
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const store = useSessionsStore()
    const interrupt = vi.spyOn(store, 'interrupt').mockResolvedValue(null)

    const button = wrapper.find('button.stop-button')
    expect(button.exists()).toBe(true)
    expect(button.text()).toBe('停止')

    await button.trigger('click')
    expect(interrupt).toHaveBeenCalledWith(1, 'sess-1')
  })

  it('disables sending when the model list fails to load', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.endsWith('/models')) return { status: 500, json: async () => ({}) } as Response
        if (url.endsWith('/continue')) {
          return {
            status: 200,
            json: async () => session('sess-1', { capabilities: capabilities({ modelSelection: true }) }),
          } as Response
        }
        if (url.endsWith('/history')) {
          return { status: 200, json: async () => ({ events: [], truncated: false }) } as Response
        }
        return {
          status: 200,
          json: async () => ({
            sessions: [session('sess-1', { capabilities: capabilities({ modelSelection: true }) })],
          }),
        } as Response
      }),
    )
    useAgentsStore().instances = [instance('running')]
    const wrapper = mountPanel('sess-1')
    await flushPromises()
    await wrapper.find('#message-text').setValue('hello')
    await flushPromises()

    expect(wrapper.text()).toContain('模型列表加载失败')
    expect(wrapper.find('button.send-button').attributes('disabled')).toBeDefined()
  })

  it('keeps each session’s chosen model when switching sessions', async () => {
    const models = [
      { id: 'pi/a', name: 'Model A', provider: 'pi' },
      { id: 'pi/b', name: 'Model B', provider: 'pi' },
    ]
    const selectable = capabilities({ modelSelection: true })
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.endsWith('/models')) return { status: 200, json: async () => ({ models }) } as Response
        if (url.endsWith('/history')) {
          return { status: 200, json: async () => ({ events: [], truncated: false }) } as Response
        }
        if (url.endsWith('/continue')) {
          const id = url.split('/')[5]
          return { status: 200, json: async () => session(id, { capabilities: selectable }) } as Response
        }
        return {
          status: 200,
          json: async () => ({
            sessions: [
              session('s1', { capabilities: selectable }),
              session('s2', { capabilities: selectable }),
            ],
          }),
        } as Response
      }),
    )
    useAgentsStore().instances = [instance('running')]
    const store = useSessionsStore()
    const wrapper = mountPanel('s1')
    await flushPromises()

    // A session opened for the first time defaults to the backend's first model.
    expect(store.modelFor('s1')).toBe('pi/a')

    // The user chooses the second model for this session.
    await wrapper.find('.model-trigger').trigger('click')
    await wrapper.find('[data-model-id="pi/b"]').trigger('click')
    expect(store.modelFor('s1')).toBe('pi/b')

    // Opening another session does not disturb the first session's choice.
    await wrapper.setProps({ sessionId: 's2' })
    await flushPromises()
    expect(store.modelFor('s1')).toBe('pi/b')
    expect(store.modelFor('s2')).toBe('pi/a')

    // Coming back restores the model this session chose, not the first one.
    await wrapper.setProps({ sessionId: 's1' })
    await flushPromises()
    expect(store.modelFor('s1')).toBe('pi/b')
    expect(wrapper.find('.model-trigger').text()).toContain('Model B')
  })

  it('opens a session on the model it actually used, not the backend’s first', async () => {
    const models = [
      { id: 'pi/a', name: 'Model A', provider: 'pi' },
      { id: 'pi/b', name: 'Model B', provider: 'pi' },
      { id: 'pi/c', name: 'Model C', provider: 'pi' },
    ]
    const selectable = capabilities({ modelSelection: true })
    const used: Record<string, string> = { s1: 'pi/b', s2: 'pi/c' }
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.endsWith('/models')) return { status: 200, json: async () => ({ models }) } as Response
        if (url.endsWith('/history')) {
          const id = url.split('/')[5]
          const model = used[id] ?? 'pi/a'
          return {
            status: 200,
            json: async () => ({
              events: [
                {
                  instanceId: '',
                  backend: 'opencode-v2',
                  sequence: 1,
                  timestamp: '2026-10-09T00:00:00Z',
                  replayed: true,
                  type: 'message',
                  message: {
                    role: 'assistant',
                    text: '历史回复',
                    final: true,
                    model: { id: model, provider: 'pi' },
                  },
                },
              ],
              truncated: false,
            }),
          } as Response
        }
        if (url.endsWith('/continue')) {
          const id = url.split('/')[5]
          return { status: 200, json: async () => session(id, { capabilities: selectable }) } as Response
        }
        return {
          status: 200,
          json: async () => ({
            sessions: [
              session('s1', { capabilities: selectable }),
              session('s2', { capabilities: selectable }),
            ],
          }),
        } as Response
      }),
    )
    useAgentsStore().instances = [instance('running')]
    const store = useSessionsStore()
    const wrapper = mountPanel('s1')
    await flushPromises()

    // The session replayed a model-B reply, so the composer opens on model B even
    // though the backend lists model A first.
    expect(store.modelFor('s1')).toBe('pi/b')
    expect(wrapper.find('.model-trigger').text()).toContain('Model B')

    // Switching sessions follows each session's own model.
    await wrapper.setProps({ sessionId: 's2' })
    await flushPromises()
    expect(store.modelFor('s2')).toBe('pi/c')
    expect(wrapper.find('.model-trigger').text()).toContain('Model C')

    // Coming back restores the first session's model, not the backend's first.
    await wrapper.setProps({ sessionId: 's1' })
    await flushPromises()
    expect(wrapper.find('.model-trigger').text()).toContain('Model B')
  })

  it('keeps an aborted pi turn idle without a process_exit error entry', () => {
    const store = useSessionsStore()
    store.sessions = [session('sess-1', { turn: 'running' })]
    store.handleEvent(
      'sess-1',
      JSON.stringify({
        instanceId: 'inst-1',
        sessionId: 'sess-1',
        backend: 'pi',
        sequence: 2,
        timestamp: '2026-10-09T00:00:00Z',
        type: 'lifecycle',
        lifecycle: { state: 'agent_settled', detail: 'aborted' },
      }),
    )

    expect(store.sessionById('sess-1')?.turn).toBe('idle')
    expect(store.messagesFor('sess-1')).toHaveLength(0)
  })
})

describe('SessionChatPanel conversation layout', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    const agents = useAgentsStore()
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined as never)
  })

  afterEach(() => {
    useSessionsStore().unsubscribeAll()
    clearModelCache()
    vi.unstubAllGlobals()
  })

  it('puts the agent tag before the session title', async () => {
    stubSessionList([session('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const header = wrapper.find('.chat-header')
    const tag = wrapper.findComponent(BackendTag)
    expect(header.exists()).toBe(true)
    expect(tag.exists()).toBe(true)
    expect(header.element.firstElementChild).toBe(tag.element)
    // And the title follows it, rather than preceding it.
    expect(header.find('.session-heading').exists()).toBe(true)
  })

  it('keeps the composer outside the scrolling message area', async () => {
    stubSessionList([session('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    // The message log scrolls; the composer is pinned below it, so it is not a
    // descendant of the scrolling region.
    const scroller = wrapper.find('[data-test="conversation-scroll"]')
    expect(scroller.exists()).toBe(true)
    expect(scroller.find('form.send').exists()).toBe(false)
    expect(wrapper.find('form.send').exists()).toBe(true)
  })

  it('marks only a running session with a status chip', async () => {
    stubSessionList([session('sess-1', { status: 'disconnected', turn: 'idle' })])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    expect(wrapper.find('.turn-status').exists()).toBe(false)
  })

  it('deletes the session after a confirmation and returns to the project', async () => {
    stubSessionList([session('sess-1')])
    const store = useSessionsStore()
    const remove = vi.spyOn(store, 'deleteSession').mockResolvedValue(true)
    const router = testRouter()
    const wrapper = mountPanel('sess-1', router)
    await flushPromises()

    await wrapper.find('[data-test="delete-session"]').trigger('click')
    const dialog = wrapper.find('[data-test="delete-session-dialog"]')
    expect(dialog.exists()).toBe(true)
    // Nothing is removed until the user confirms.
    expect(remove).not.toHaveBeenCalled()

    await dialog.find('[data-test="confirm-delete"]').trigger('click')
    await flushPromises()

    expect(remove).toHaveBeenCalledWith(1, 'sess-1')
    // The project has no page to return to; the workbench does.
    expect(router.currentRoute.value.name).toBe('home')
  })

  it('cancels the delete without removing the session', async () => {
    stubSessionList([session('sess-1')])
    const store = useSessionsStore()
    const remove = vi.spyOn(store, 'deleteSession').mockResolvedValue(true)
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    await wrapper.find('[data-test="delete-session"]').trigger('click')
    await wrapper.find('.confirm-cancel').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="delete-session-dialog"]').exists()).toBe(false)
    expect(remove).not.toHaveBeenCalled()
  })
})

describe('SessionChatPanel scroll following', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    const agents = useAgentsStore()
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined as never)
  })

  afterEach(() => {
    useSessionsStore().unsubscribeAll()
    clearModelCache()
    vi.unstubAllGlobals()
  })

  it('follows new entries while the view is at the bottom, with no jump button', async () => {
    stubSessionList([session('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const el = wrapper.find('[data-test="conversation-scroll"]').element as HTMLElement
    setGeometry(el, { scrollHeight: 1000, clientHeight: 400 })

    useSessionsStore().pushEntry('sess-1', entry('m1', 'FOLLOWED'))
    await flushPromises()

    // The view was following, so the new entry pulled it to the bottom.
    expect(el.scrollTop).toBe(1000)
    expect(wrapper.find('[data-test="jump-latest"]').exists()).toBe(false)
  })

  it('holds the position when scrolled away and offers a button to the newest entry', async () => {
    stubSessionList([session('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const el = wrapper.find('[data-test="conversation-scroll"]').element as HTMLElement
    setGeometry(el, { scrollHeight: 1000, clientHeight: 400 })
    // Park mid-history: 700px from the bottom, well past the follow threshold.
    el.scrollTop = 300
    await wrapper.find('[data-test="conversation-scroll"]').trigger('scroll')

    setGeometry(el, { scrollHeight: 1200, clientHeight: 400 })
    useSessionsStore().pushEntry('sess-1', entry('m2', 'WHILE AWAY'))
    await flushPromises()

    // The position is untouched, and the button appeared instead.
    expect(el.scrollTop).toBe(300)
    const jump = wrapper.find('[data-test="jump-latest"]')
    expect(jump.exists()).toBe(true)

    // Clicking it scrolls to the newest entry and retires the button. jsdom
    // has no Element.scrollTo, so the test supplies one that behaves.
    const scrollTo = vi.fn((options: { top: number }) => {
      el.scrollTop = options.top
    })
    el.scrollTo = scrollTo as unknown as typeof el.scrollTo
    await jump.trigger('click')
    await flushPromises()

    expect(scrollTo).toHaveBeenCalledWith({ top: 1200, behavior: 'smooth' })
    expect(el.scrollTop).toBe(1200)
    expect(wrapper.find('[data-test="jump-latest"]').exists()).toBe(false)
  })

  it('resumes following once the user scrolls back to the bottom', async () => {
    stubSessionList([session('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const el = wrapper.find('[data-test="conversation-scroll"]').element as HTMLElement
    setGeometry(el, { scrollHeight: 1000, clientHeight: 400 })
    el.scrollTop = 300
    await wrapper.find('[data-test="conversation-scroll"]').trigger('scroll')
    useSessionsStore().pushEntry('sess-1', entry('m1', 'FIRST'))
    await flushPromises()
    expect(wrapper.find('[data-test="jump-latest"]').exists()).toBe(true)

    // Scrolling back to the bottom hides the button and resumes following.
    el.scrollTop = 1000
    await wrapper.find('[data-test="conversation-scroll"]').trigger('scroll')
    expect(wrapper.find('[data-test="jump-latest"]').exists()).toBe(false)

    setGeometry(el, { scrollHeight: 1300, clientHeight: 400 })
    useSessionsStore().pushEntry('sess-1', entry('m2', 'SECOND'))
    await flushPromises()
    expect(el.scrollTop).toBe(1300)
  })

  it('follows streaming text of the open entry, not only new entries', async () => {
    stubSessionList([session('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const el = wrapper.find('[data-test="conversation-scroll"]').element as HTMLElement
    setGeometry(el, { scrollHeight: 800, clientHeight: 400 })

    const store = useSessionsStore()
    store.pushEntry('sess-1', {
      id: 's1',
      kind: 'message',
      role: 'assistant',
      text: 'partial',
      streaming: true,
    } as ChatEntry)
    await flushPromises()
    expect(el.scrollTop).toBe(800)

    // A delta grows the open entry without appending one; the view follows.
    setGeometry(el, { scrollHeight: 900, clientHeight: 400 })
    store.messagesFor('sess-1')[0].text += ' and more'
    await flushPromises()
    expect(el.scrollTop).toBe(900)
  })
})
