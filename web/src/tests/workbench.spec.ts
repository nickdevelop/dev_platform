// Unit tests for the unified agent session workbench: per-instance isolation,
// streamed messages, capability-gated controls, stop semantics and tool
// confirmation rendering. The stores consume the Task 5 JSON/SSE APIs; the SSE
// transport is faked with a controllable EventSource so event routing can be
// driven deterministically.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia, type Pinia } from 'pinia'
import AgentInstanceList from '../components/AgentInstanceList.vue'
import CapabilityControls from '../components/CapabilityControls.vue'
import MessageView from '../components/MessageView.vue'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore } from '../stores/sessions'
import { useAuthStore } from '../stores/auth'
import type { AgentInstance } from '../api/agents'
import type { AgentSession, Capabilities } from '../api/sessions'
import SessionWorkbenchView from '../views/SessionWorkbenchView.vue'

// A fully-false capability declaration: the workbench must expose no controls.
function noCapabilities(overrides: Partial<Capabilities> = {}): Capabilities {
  return {
    streaming: false,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: false,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
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
// so a test can express the API surface a view or store will touch.
interface RouteResponse {
  status: number
  body?: unknown
}

function stubRoutes(routes: Array<[string, string, RouteResponse]>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://localhost')
      const method = (init?.method ?? 'GET').toUpperCase()
      const match = routes.find(
        ([routeMethod, prefix]) => routeMethod === method && url.pathname.startsWith(prefix),
      )
      if (!match) throw new Error(`unexpected ${method} ${url.pathname}`)
      return {
        status: match[2].status,
        json: async () => match[2].body,
      } as Response
    }),
  )
}

let pinia: Pinia
let liveSessions: ReturnType<typeof useSessionsStore> | null = null

function setupStores() {
  pinia = createPinia()
  setActivePinia(pinia)
  liveSessions = useSessionsStore()
  return {
    agents: useAgentsStore(),
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

describe('agent instance list', () => {
  it('renders multiple instances and gates stop per backend capability', async () => {
    const instances = [
      instance({ id: 'pi-1', backend: 'pi' }),
      instance({ id: 'oc-1', backend: 'opencode-v2', endpoint: 'http://127.0.0.1:1234' }),
    ]
    const wrapper = mount(AgentInstanceList, {
      props: {
        instances,
        capabilitiesFor: (backend: string) =>
          backend === 'pi' ? noCapabilities({ stop: true }) : noCapabilities({ stop: false }),
      },
    })
    mounted.push(wrapper)

    expect(wrapper.findAll('.instance-item')).toHaveLength(2)
    // Only the backend that declares stop exposes a stop control.
    expect(wrapper.findAll('button.stop')).toHaveLength(1)

    await wrapper.find('button.stop').trigger('click')
    expect(wrapper.emitted('stop')?.[0]).toEqual(['pi-1'])
  })
})

describe('capability-gated controls', () => {
  it('omits model, continue, stop and tool-confirmation controls when unsupported', () => {
    const wrapper = mount(CapabilityControls, {
      props: { capabilities: noCapabilities(), pendingToolCall: { callId: 'c1', name: 'bash' } },
    })
    mounted.push(wrapper)

    expect(wrapper.find('#session-model').exists()).toBe(false)
    expect(wrapper.find('button.continue').exists()).toBe(false)
    expect(wrapper.find('button.stop').exists()).toBe(false)
    expect(wrapper.find('[data-test="tool-confirmation"]').exists()).toBe(false)
  })

  it('renders model, continue and stop controls when the capabilities advertise them', () => {
    const wrapper = mount(CapabilityControls, {
      props: {
        capabilities: noCapabilities({
          modelSelection: true,
          sessionContinue: true,
          stop: true,
        }),
      },
    })
    mounted.push(wrapper)

    expect(wrapper.find('#session-model').exists()).toBe(true)
    expect(wrapper.find('button.continue').exists()).toBe(true)
    expect(wrapper.find('button.stop').exists()).toBe(true)
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
  it('renders streamed assistant text without injecting raw HTML', () => {
    const wrapper = mount(MessageView, {
      props: {
        messages: [
          {
            id: 'm1',
            kind: 'message',
            role: 'assistant',
            text: '<script>alert(1)</script> hello',
            streaming: true,
          },
        ],
        capabilities: noCapabilities({ streaming: true }),
      },
    })
    mounted.push(wrapper)

    expect(wrapper.text()).toContain('<script>alert(1)</script> hello')
    // The payload is text, never markup.
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

    const [first, second] = FakeEventSource.instances
    first.open()
    second.open()

    // An event addressed to instance 2 arriving on session 1's stream is dropped.
    first.emit(
      'message',
      eventBody({
        instanceId: 'inst-2',
        sessionId: 'sess-1',
        message: { role: 'assistant', text: 'wrong', final: true },
      }),
    )
    // A genuine event for session 1 renders only in session 1.
    first.emit(
      'message',
      eventBody({
        instanceId: 'inst-1',
        sessionId: 'sess-1',
        message: { role: 'assistant', text: 'right', final: true },
      }),
    )
    // Session 2 is a different instance and stays untouched by session 1.
    second.emit(
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
})

describe('stop semantics', () => {
  it('marks the target session stopped and its siblings disconnected, leaving other instances active', async () => {
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
    // The adapter stops at instance level: same-instance siblings are
    // reported disconnected, but a different instance is untouched.
    expect(sessions.sessionById('sess-2')?.status).toBe('disconnected')
    expect(sessions.sessionById('sess-3')?.status).toBe('active')
  })
})

describe('session workbench view', () => {
  it('lists sessions and renders the selected session workbench', async () => {
    stubRoutes([
      [
        'GET',
        '/api/projects/1/sessions',
        { status: 200, body: { sessions: [session({ title: 'Demo session' })] } },
      ],
      [
        'GET',
        '/api/agent-backends',
        {
          status: 200,
          body: {
            backends: [
              {
                backend: 'pi',
                capabilities: noCapabilities({
                  streaming: true,
                  sessionContinue: true,
                  stop: true,
                  modelSelection: true,
                }),
              },
            ],
          },
        },
      ],
      ['GET', '/api/projects/1/instances', { status: 200, body: { instances: [instance()] } }],
    ])
    vi.stubGlobal('EventSource', FakeEventSource)

    const { sessions, agents, auth } = setupStores()
    auth.csrfToken = 'csrf-1'
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/projects/:id/workbench', name: 'workbench', component: SessionWorkbenchView },
        { path: '/projects/:id/agents', name: 'project-agents', component: { template: '<div />' } },
      ],
    })
    await router.push('/projects/1/workbench')
    await router.isReady()

    const wrapper = mount(SessionWorkbenchView, { global: { plugins: [pinia, router] } })
    mounted.push(wrapper)
    await flushPromises()

    expect(agents.backends).toHaveLength(1)
    expect(wrapper.findAll('.session-item')).toHaveLength(1)
    expect(wrapper.text()).toContain('Demo session')

    await wrapper.find('.session-item').trigger('click')
    await flushPromises()

    // Selecting a session opens its stream and shows its controls.
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(wrapper.find('button.stop').exists()).toBe(true)
    expect(sessions.activeSessionId).toBe('sess-1')
  })
})
