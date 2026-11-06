import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import SessionChatPanel from '../features/chat/SessionChatPanel.vue'
import { useAgentsStore } from '../stores/agents'
import { backendFixtures } from './backend-fixtures'
import { useSessionsStore } from '../stores/sessions'
import { clearModelCache } from '../shared/modelCache'
import type { AgentInstance } from '../api/agents'
import type { AgentSession, Capabilities } from '../api/sessions'
import { installTestCSRF } from './csrf-fixture'

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
    capabilities: capabilities({ toolConfirmation: true, rawEvents: true }),
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
    ...overrides,
  }
}

class FakeEventSource {
  static instances: FakeEventSource[] = []
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2

  readonly url: string
  readyState = FakeEventSource.CONNECTING
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  private readonly listeners = new Map<string, Array<(event: MessageEvent) => void>>()

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener])
  }

  dispatch(type: string, data: unknown) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data: JSON.stringify(data), lastEventId: '' } as MessageEvent)
    }
  }

  close() {
    this.readyState = FakeEventSource.CLOSED
  }
}

function stubSessionList(list: AgentSession[]) {
  const defaultInstances: AgentInstance[] = [
    { id: 'inst-1', projectId: 1, backend: 'opencode-v2', directory: '/workspace', status: 'running', createdAt: '', updatedAt: '' },
  ]
  const fetchSpy = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    if (url.endsWith('/instances')) {
      return { status: 200, json: async () => ({ instances: defaultInstances }) } as Response
    }
    if (url.endsWith('/history')) return { status: 200, json: async () => ({ events: [], truncated: false }) } as Response
    if (url.endsWith('/continue')) {
      const id = url.split('/')[5]
      const found = list.find((entry) => entry.id === id) ?? list[0]
      return { status: 200, json: async () => found } as Response
    }
    if (url.includes('/events/') && url.includes('/subscriptions')) {
      if (init?.method === 'DELETE') {
        return { status: 200, json: async () => ({ success: true }) } as Response
      }
      return {
        status: 200,
        json: async () => ({
          sessionId: 'sess-1',
          turn: { id: 'turn-1', phase: 'ended', outcome: 'completed' },
          interactions: { permissions: [], forms: [] },
          resync: false,
        }),
      } as Response
    }
    if (url.includes('/permission/') && url.endsWith('/reply')) {
      return { status: 200, json: async () => ({}) } as Response
    }
    if (url.includes('/form/') && url.endsWith('/reply')) {
      return { status: 200, json: async () => ({}) } as Response
    }
    if (url.endsWith('/messages')) {
      const found = list.find((entry) => entry.id === 'sess-1') ?? list[0]
      return { status: 200, json: async () => ({ ...found, turn: 'idle' }) } as Response
    }
    return { status: 200, json: async () => ({ sessions: list }) } as Response
  })
  vi.stubGlobal('fetch', fetchSpy)
  return fetchSpy
}

function testRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { render: () => null } },
      { path: '/projects/:projectId/sessions/:sessionId', name: 'session', component: { render: () => null } },
    ],
  })
}

function mountPanel(sessionId: string) {
  return mount(SessionChatPanel, {
    props: { projectId: 1, sessionId },
    global: { plugins: [testRouter()] },
  })
}

// The transport refuses mutating requests without a token source; the app
// installs the real provider in main.ts, which these specs never run.
installTestCSRF()

describe('SessionChatPanel - Send Shortcuts', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    const agentsStore = useAgentsStore()
    vi.spyOn(agentsStore, 'loadBackends').mockResolvedValue(undefined as never)
    vi.spyOn(agentsStore, 'loadInstances').mockResolvedValue(undefined as never)
    agentsStore.instances = [{ id: 'inst-1', projectId: 1, backend: 'opencode-v2', directory: '/workspace', status: 'running', createdAt: '', updatedAt: '' }]
    agentsStore.backends = backendFixtures()
  })

  afterEach(() => {
    useSessionsStore().unsubscribeAll()
    clearModelCache()
    vi.unstubAllGlobals()
  })

  it('supports toggling send shortcuts and respects enter vs ctrl+enter', async () => {
    const fetchSpy = stubSessionList([session('sess-1')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const textarea = wrapper.find<HTMLTextAreaElement>('#message-text')
    await textarea.setValue('Hello agent')

    // Initial default is 'enter'
    const toggleBtn = wrapper.find('[data-test="send-shortcut-toggle"]')
    expect(toggleBtn.exists()).toBe(true)
    expect(wrapper.find('[data-test="send-shortcut-menu"]').exists()).toBe(false)

    // Press Enter -> should send
    await textarea.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    let sendCall = fetchSpy.mock.calls.find((call) => String(call[0]).endsWith('/messages'))
    expect(sendCall).toBeDefined()

    // Open shortcut menu and switch to ctrl-enter
    await toggleBtn.trigger('click')
    expect(wrapper.find('[data-test="send-shortcut-menu"]').exists()).toBe(true)

    const ctrlEnterOption = wrapper.find('[data-test="shortcut-ctrl-enter"]')
    await ctrlEnterOption.trigger('click')
    await flushPromises()

    // Menu should close
    expect(wrapper.find('[data-test="send-shortcut-menu"]').exists()).toBe(false)

    // Now in ctrl-enter mode
    fetchSpy.mockClear()
    await textarea.setValue('Second message')

    // Plain Enter should NOT send in ctrl-enter mode
    await textarea.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(fetchSpy.mock.calls.some((call) => String(call[0]).endsWith('/messages'))).toBe(false)

    // Ctrl+Enter should send
    await textarea.trigger('keydown', { key: 'Enter', ctrlKey: true })
    await flushPromises()
    sendCall = fetchSpy.mock.calls.find((call) => String(call[0]).endsWith('/messages'))
    expect(sendCall).toBeDefined()
  })
})
