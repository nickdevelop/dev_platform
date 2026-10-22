// Unit tests for the unified agent session workbench: per-instance isolation,
// streamed messages, capability-gated controls, stop semantics and tool
// confirmation rendering. The stores consume the Session 5 JSON/SSE APIs; the SSE
// transport is faked with a controllable EventSource so event routing can be
// driven deterministically.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia, type Pinia } from 'pinia'
import CapabilityControls from '../features/chat/CapabilityControls.vue'
import MessageView from '../features/chat/rendering/MessageView.vue'
import SessionList from '../features/workspace/SessionList.vue'
import { useAgentsStore } from '../stores/agents'
import { backendFixtures } from './backend-fixtures'
import { useSessionsStore, type ChatEntry } from '../stores/sessions'
import { useAuthStore } from '../stores/auth'
import type { AgentInstance } from '../api/agents'
import type { AgentSession, Capabilities } from '../api/sessions'
import SessionChatPanel from '../features/chat/SessionChatPanel.vue'
import { installTestCSRF } from './csrf-fixture'

// A fully-false capability declaration: the workbench must expose no controls.
function noCapabilities(overrides: Partial<Capabilities> = {}): Capabilities {
  return {
    streaming: false,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: false,
    interrupt: false,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
    sessionHistory: false,
    rawEvents: false,
    ...overrides,
  }
}

function instance(overrides: Partial<AgentInstance> = {}): AgentInstance {
  return {
    id: 'inst-1',
    projectId: 1,
    backend: 'pi',
    directory: '/workspace/demo',
    status: 'running',
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
    ...overrides,
  }
}

function session(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 'sess-1',
    projectId: 1,
    backend: 'pi',
    instanceId: 'inst-1',
    nativeSessionId: 'native-1',
    title: 'Demo',
    status: 'active',
    turn: 'idle',
    capabilities: noCapabilities({ streaming: true, stop: true, sessionContinue: true }),
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
    ...overrides,
  }
}

function eventBody(overrides: Record<string, unknown> = {}) {
  return JSON.stringify({
    instanceId: 'inst-1',
    sessionId: 'sess-1',
    backend: 'pi',
    sequence: 1,
    timestamp: '2026-10-09T00:00:00Z',
    type: 'message',
    message: { role: 'assistant', text: '', final: true },
    ...overrides,
  })
}

// FakeEventSource records every subscription and lets a test drive open/error
// and emit named SSE events. It mirrors the readyState constants EventSource
// exposes so the store's reconnect logic can be exercised.
class FakeEventSource {
  static instances: FakeEventSource[] = []
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2

  readonly url: string
  readyState = FakeEventSource.CONNECTING
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  private listeners = new Map<string, ((event: MessageEvent) => void)[]>()

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    const list = this.listeners.get(type) ?? []
    list.push(listener)
    this.listeners.set(type, list)
  }

  close() {
    this.readyState = FakeEventSource.CLOSED
  }

  open() {
    this.readyState = FakeEventSource.OPEN
    this.onopen?.()
  }

  emit(type: string, data: string) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data } as MessageEvent)
    }
  }

  fail() {
    this.readyState = FakeEventSource.CONNECTING
    this.onerror?.()
  }

  end() {
    this.readyState = FakeEventSource.CLOSED
    this.onerror?.()
  }
}

// stubRoutes answers fetch calls by method + path prefix, independent of order,
// so a test can express the API surface a view or store will touch. It returns
// the recorded calls (method, path, parsed body) for assertions.
interface RouteResponse {
  status: number
  body?: unknown
}

interface RecordedRequest {
  method: string
  path: string
  body: unknown
}

function stubRoutes(routes: Array<[string, string, RouteResponse]>): RecordedRequest[] {
  const calls: RecordedRequest[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://localhost')
      const method = (init?.method ?? 'GET').toUpperCase()
      const body = init?.body ? JSON.parse(String(init.body)) : null
      calls.push({ method, path: url.pathname, body })
      const match = routes.find(
        ([routeMethod, prefix]) => routeMethod === method && url.pathname.startsWith(prefix),
      )
      if (match) {
        return {
          status: match[2].status,
          json: async () => match[2].body,
        } as Response
      }
      if (url.pathname.includes('/events/') && url.pathname.includes('/subscriptions')) {
        if (method === 'DELETE') {
          return {
            status: 200,
            json: async () => ({ success: true }),
          } as Response
        }
        return {
          status: 200,
          json: async () => ({
            sessionId: body?.sessionId ?? '',
            turn: { id: 'turn-1', phase: 'ended', outcome: 'completed' },
            interactions: { permissions: [], forms: [] },
            resync: false,
          }),
        } as Response
      }
      throw new Error(`unexpected ${method} ${url.pathname}`)
    }),
  )
  return calls
}

let pinia: Pinia
let liveSessions: ReturnType<typeof useSessionsStore> | null = null

function setupStores() {
  pinia = createPinia()
  setActivePinia(pinia)
  liveSessions = useSessionsStore()
  const agents = useAgentsStore()
  agents.backends = backendFixtures()
  return {
    agents,
    sessions: liveSessions,
    auth: useAuthStore(),
  }
}

const mounted: ReturnType<typeof mount>[] = []

afterEach(() => {
  // Module-level EventSource handles are shared across store instances; close
  // any left open so a later test starts from a clean subscription map.
  liveSessions?.unsubscribeAll()
  liveSessions = null
  for (const wrapper of mounted) wrapper.unmount()
  mounted.length = 0
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  FakeEventSource.instances = []
})

// The transport refuses mutating requests without a token source; the app
// installs the real provider in main.ts, which these specs never run.
installTestCSRF()

describe('session list', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useAgentsStore().backends = backendFixtures()
  })

  it('opens a session with the keyboard (Enter and Space) as well as the pointer', async () => {
    const wrapper = mount(SessionList, {
      props: { sessions: [session({ id: 'sess-1', title: 'Demo' })] },
      // Attach so focus behaves like a browser (a detached tree cannot focus).
      attachTo: document.body,
    })
    mounted.push(wrapper)

    // The row exposes a focusable, keyboard-operable control.
    const open = wrapper.find('.session-open')
    expect(open.exists()).toBe(true)
    expect(open.attributes('role')).toBe('button')
    expect(open.attributes('tabindex')).toBe('0')

    ;(open.element as HTMLElement).focus()
    expect(document.activeElement).toBe(open.element)

    await open.trigger('keydown', { key: 'Enter' })
    await open.trigger('keydown', { key: ' ' })
    await open.trigger('click')

    const selections = wrapper.emitted('select') ?? []
    expect(selections.map((entry) => entry[0])).toEqual(['sess-1', 'sess-1', 'sess-1'])
  })

  it('shows the untitled placeholder instead of leaking the internal session id', () => {
    const wrapper = mount(SessionList, {
      props: { sessions: [session({ id: 'sess-untitled', title: '' })] },
    })
    mounted.push(wrapper)

    expect(wrapper.text()).toContain('未命名会话')
    expect(wrapper.text()).not.toContain('sess-untitled')
  })
})

describe('capability-gated controls', () => {
  it('omits the tool-confirmation control when unsupported', () => {
    const wrapper = mount(CapabilityControls, {
      props: { capabilities: noCapabilities(), pendingToolCall: { callId: 'c1', name: 'bash' } },
    })
    mounted.push(wrapper)

    expect(wrapper.find('[data-test="tool-confirmation"]').exists()).toBe(false)
  })

  it('renders tool confirmation only when the capability is advertised', async () => {
    const advertised = mount(CapabilityControls, {
      props: {
        capabilities: noCapabilities({ toolConfirmation: true }),
        pendingToolCall: { callId: 'c1', name: 'bash' },
      },
    })
    mounted.push(advertised)
    expect(advertised.find('[data-test="tool-confirmation"]').exists()).toBe(true)

    await advertised.find('button.confirm-tool').trigger('click')
    expect(advertised.emitted('confirm-tool')?.[0]).toEqual(['c1'])

    const unsupported = mount(CapabilityControls, {
      props: {
        capabilities: noCapabilities({ toolConfirmation: false }),
        pendingToolCall: { callId: 'c1', name: 'bash' },
      },
    })
    mounted.push(unsupported)
    expect(unsupported.find('[data-test="tool-confirmation"]').exists()).toBe(false)
  })
})

describe('message rendering', () => {
  it('renders a streamed assistant reply as sanitized markdown', () => {
    const wrapper = mount(MessageView, {
      props: {
        messages: [
          {
            id: 'm1',
            kind: 'message',
            role: 'assistant',
            text: 'hello **world** <script>alert(1)</script>',
            streaming: true,
          },
        ],
        capabilities: noCapabilities({ streaming: true }),
      },
    })
    mounted.push(wrapper)

    // Markdown renders, but backend markup cannot inject a live script.
    expect(wrapper.find('.bubble-agent .message-text').html()).toContain('<strong>world</strong>')
    expect(wrapper.text()).toContain('hello world')
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.find('[data-test="streaming"]').exists()).toBe(true)
  })
})

describe('session store streaming and isolation', () => {
  beforeEach(() => {
    vi.stubGlobal('EventSource', FakeEventSource)
  })

  it('accumulates streamed message chunks for one session', async () => {
    stubRoutes([
      [
        'GET',
        '/api/projects/1/sessions',
        { status: 200, body: { sessions: [session({ capabilities: noCapabilities({ streaming: true }) })] } },
      ],
    ])
    const { sessions } = setupStores()

    await sessions.loadSessions(1)
    await sessions.subscribe(1, 'sess-1')
    const source = FakeEventSource.instances[0]
    source.open()

    source.emit(
      'message',
      eventBody({ sequence: 1, message: { role: 'assistant', text: 'Hel', final: false } }),
    )
    source.emit(
      'message',
      eventBody({ sequence: 2, message: { role: 'assistant', text: 'lo', final: false } }),
    )
    source.emit(
      'message',
      eventBody({ sequence: 3, message: { role: 'assistant', text: '', final: true } }),
    )

    const messages = sessions.messagesFor('sess-1')
    expect(messages).toHaveLength(1)
    expect(messages[0].text).toBe('Hello')
    expect(messages[0].streaming).toBe(false)
    expect(sessions.statusFor('sess-1')).toBe('open')
  })

  it('never renders another instance\'s events into a session view', async () => {
    stubRoutes([
      [
        'GET',
        '/api/projects/1/sessions',
        {
          status: 200,
          body: {
            sessions: [
              session({ id: 'sess-1', instanceId: 'inst-1', nativeSessionId: 'n1' }),
              session({ id: 'sess-2', instanceId: 'inst-2', nativeSessionId: 'n2' }),
            ],
          },
        },
      ],
    ])
    const { sessions } = setupStores()
    await sessions.loadSessions(1)
    await sessions.subscribe(1, 'sess-1')
    await sessions.subscribe(1, 'sess-2')

    // Event bus is multiplexed: single EventSource connection for the window
    expect(FakeEventSource.instances).toHaveLength(1)
    const source = FakeEventSource.instances[0]
    source.open()

    // An event addressed to instance 2 arriving for session 1 is dropped.
    source.emit(
      'message',
      eventBody({
        instanceId: 'inst-2',
        sessionId: 'sess-1',
        message: { role: 'assistant', text: 'wrong', final: true },
      }),
    )
    // A genuine event for session 1 renders only in session 1.
    source.emit(
      'message',
      eventBody({
        instanceId: 'inst-1',
        sessionId: 'sess-1',
        message: { role: 'assistant', text: 'right', final: true },
      }),
    )
    // Session 2 is a different instance and stays untouched by session 1.
    source.emit(
      'message',
      eventBody({
        instanceId: 'inst-2',
        sessionId: 'sess-2',
        message: { role: 'assistant', text: 'two', final: true },
      }),
    )

    expect(sessions.messagesFor('sess-1').map((m) => m.text)).toEqual(['right'])
    expect(sessions.messagesFor('sess-2').map((m) => m.text)).toEqual(['two'])
  })

  it('reports a reconnecting stream and a terminal error without affecting other sessions', async () => {
    stubRoutes([
      [
        'GET',
        '/api/projects/1/sessions',
        { status: 200, body: { sessions: [session()] } },
      ],
    ])
    const { sessions } = setupStores()
    await sessions.loadSessions(1)
    await sessions.subscribe(1, 'sess-1')
    const source = FakeEventSource.instances[0]

    source.fail()
    expect(sessions.statusFor('sess-1')).toBe('reconnecting')
    source.end()
    expect(sessions.statusFor('sess-1')).toBe('error')
  })

  it('keeps both streams open when switching sessions', async () => {
    stubRoutes([
      [
        'GET',
        '/api/projects/1/sessions',
        {
          status: 200,
          body: {
            sessions: [
              session({ id: 'sess-1', instanceId: 'inst-1', nativeSessionId: 'n1' }),
              session({ id: 'sess-2', instanceId: 'inst-2', nativeSessionId: 'n2' }),
            ],
          },
        },
      ],
    ])
    const { sessions } = setupStores()
    await sessions.loadSessions(1)

    sessions.activateSession(1, 'sess-1')
    expect(FakeEventSource.instances).toHaveLength(1)
    const first = FakeEventSource.instances[0]
    first.open()

    sessions.activateSession(1, 'sess-2')
    // Tabs keep both sessions live on the shared event bus: the bus stays open
    expect(first.readyState).not.toBe(FakeEventSource.CLOSED)
    expect(sessions.statusFor('sess-1')).toBe('open')
    expect(sessions.statusFor('sess-2')).toBe('open')
    // Multiplexed bus: still exactly ONE EventSource connection
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(sessions.activeSessionId).toBe('sess-2')

    // Re-activating the current session is a no-op (no duplicate stream).
    sessions.activateSession(1, 'sess-2')
    expect(FakeEventSource.instances).toHaveLength(1)
  })
})

describe('stop semantics', () => {
  it('marks only the target session stopped, leaving its siblings and the instance active', async () => {
    const stopped = session({ id: 'sess-1', status: 'stopped' })
    stubRoutes([
      [
        'GET',
        '/api/projects/1/sessions',
        {
          status: 200,
          body: {
            sessions: [
              session({ id: 'sess-1', instanceId: 'inst-1', nativeSessionId: 'n1' }),
              session({ id: 'sess-2', instanceId: 'inst-1', nativeSessionId: 'n2' }),
              session({ id: 'sess-3', instanceId: 'inst-2', nativeSessionId: 'n3' }),
            ],
          },
        },
      ],
      ['POST', '/api/projects/1/sessions/sess-1/stop', { status: 200, body: stopped }],
    ])
    const { sessions, auth } = setupStores()
    auth.csrfToken = 'csrf-1'
    await sessions.loadSessions(1)

    await sessions.stopSession(1, 'sess-1')

    expect(sessions.sessionById('sess-1')?.status).toBe('stopped')
    // A session stop is session-scoped: same-instance siblings keep their own
    // runs, and so does a session on another instance.
    expect(sessions.sessionById('sess-2')?.status).toBe('active')
    expect(sessions.sessionById('sess-3')?.status).toBe('active')
  })
})

describe('session chat and new session panels', () => {
  async function mountPanel(component: Parameters<typeof mount>[0], props: Record<string, unknown>) {
    const stores = setupStores()
    const wrapper = mount(component, { props, global: { plugins: [pinia] } })
    mounted.push(wrapper)
    await flushPromises()
    return { wrapper, ...stores }
  }

  it('opens a session and renders its stream controls', async () => {
    stubRoutes([
      ['GET', '/api/projects/1/sessions', { status: 200, body: { sessions: [session({ title: 'Demo session' })] } }],
    ])
    vi.stubGlobal('EventSource', FakeEventSource)

    const { wrapper, sessions } = await mountPanel(SessionChatPanel, { projectId: 1, sessionId: 'sess-1' })

    expect(wrapper.text()).toContain('Demo session')
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(wrapper.find('button.send-button').exists()).toBe(true)
    expect(sessions.activeSessionId).toBe('sess-1')
  })

  it('derives send-time model support from the open session, not the create instance', async () => {
    const piSession = session({
      id: 'sess-pi',
      backend: 'pi',
      instanceId: 'pi-1',
      capabilities: noCapabilities({ streaming: true, stop: true }),
    })
    const calls = stubRoutes([
      ['GET', '/api/projects/1/sessions', { status: 200, body: { sessions: [piSession] } }],
      ['GET', '/api/projects/1/instances', { status: 200, body: { instances: [instance({ id: 'pi-1', backend: 'pi' })] } }],
      ['POST', '/api/projects/1/sessions/sess-pi/messages', { status: 202, body: piSession }],
    ])
    vi.stubGlobal('EventSource', FakeEventSource)

    const { wrapper, auth } = await mountPanel(SessionChatPanel, { projectId: 1, sessionId: 'sess-pi' })
    auth.csrfToken = 'csrf-1'

    // The open session's backend does not advertise modelSelection, so no model picker is shown.
    expect(wrapper.find('.model-select').exists()).toBe(false)

    await wrapper.find('#message-text').setValue('hello')
    await wrapper.find('form.send').trigger('submit')
    await flushPromises()

    const sent = calls.find((call) => call.method === 'POST' && call.path.endsWith('/messages'))
    expect(sent?.body).toEqual({ text: 'hello' })
  })

})

describe('platform agent switch in the agents store', () => {
  it('reflects a platform-level disable once the backend list is reloaded', async () => {
    const { agents } = setupStores()
    expect(agents.backendEnabledFor('pi')).toBe(true)

    const calls = stubRoutes([
      [
        'GET',
        '/api/agent-backends',
        {
          status: 200,
          body: {
            backends: backendFixtures().map((entry) =>
              entry.backend === 'pi' ? { ...entry, enabled: false } : entry,
            ),
          },
        },
      ],
    ])
    await agents.refreshBackends()

    expect(calls.map((call) => call.path)).toEqual(['/api/agent-backends'])
    expect(agents.backendEnabledFor('pi')).toBe(false)
    expect(agents.backendEnabledFor('opencode-v2')).toBe(true)
  })
})

describe('MessageView presentation', () => {
  const messages: ChatEntry[] = [
    { id: 'm1', kind: 'message', role: 'user', text: '我的问题', streaming: false },
    { id: 'm2', kind: 'thinking', role: 'assistant', text: '推理内容', streaming: false },
    { id: 'm3', kind: 'message', role: 'assistant', text: '答复内容', streaming: false },
    { id: 'm4', kind: 'tool_call', role: 'assistant', text: 'bash', name: 'bash', callId: 'c1', streaming: false },
    { id: 'm5', kind: 'tool_result', role: 'tool', text: '输出内容', callId: 'c1', isError: false, streaming: false },
  ]

  it('aligns user turns to the right and agent output to the left', () => {
    const wrapper = mount(MessageView, {
      props: { messages, capabilities: noCapabilities({ toolCallEvents: true }) },
    })
    mounted.push(wrapper)

    const bubbles = wrapper.findAll('.bubble')
    expect(bubbles[0].classes()).toContain('bubble-user')
    expect(bubbles[1].classes()).toContain('bubble-agent')
  })

  it('keeps reasoning collapsed and shows it when expanded', async () => {
    const wrapper = mount(MessageView, {
      props: { messages, capabilities: noCapabilities({ toolCallEvents: true }) },
    })
    mounted.push(wrapper)

    const thinking = wrapper.find('[data-test="thinking"]')
    expect(thinking.exists()).toBe(true)
    expect(thinking.attributes('open')).toBeUndefined()
    expect(thinking.find('summary').text()).toContain('Thinking')

    await thinking.find('summary').trigger('click')
    expect(thinking.text()).toContain('推理内容')
  })

  it('merges a tool call and its result into one collapsed block', async () => {
    const wrapper = mount(MessageView, {
      props: { messages, capabilities: noCapabilities({ toolCallEvents: true }) },
    })
    mounted.push(wrapper)

    // One block per call: the result is not a block of its own.
    const call = wrapper.find('[data-test="tool-call"]')
    expect(call.exists()).toBe(true)
    expect(call.attributes('open')).toBeUndefined()
    expect(call.find('summary').text()).toContain('bash')
    expect(wrapper.find('[data-test="tool-result"]').exists()).toBe(false)

    await call.find('summary').trigger('click')
    expect(call.find('.tool-output').text()).toContain('输出内容')
  })

  it('renders no tool or reasoning entries when the backend declares neither', () => {
    const wrapper = mount(MessageView, {
      props: {
        messages: [
          { id: 'm4', kind: 'tool_call', role: 'assistant', text: 'bash', name: 'bash', streaming: false },
          { id: 'm5', kind: 'tool_result', role: 'tool', text: '输出', streaming: false },
        ],
        capabilities: noCapabilities({ toolCallEvents: false }),
      },
    })
    mounted.push(wrapper)

    expect(wrapper.find('[data-test="tool-call"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="tool-result"]').exists()).toBe(false)
  })
})
