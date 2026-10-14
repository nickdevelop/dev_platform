// Split-log tests: the left column shows only messages/errors, the right
// activity pane shows only thinking/tool step groups, and the pane collapses
// with the choice persisted in localStorage.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import MessageView from '../components/MessageView.vue'
import SessionChatPanel from '../components/SessionChatPanel.vue'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore, type ChatEntry } from '../stores/sessions'
import { clearModelCache } from '../modelCache'
import type { AgentSession, Capabilities } from '../api/sessions'

function capabilities(overrides: Partial<Capabilities> = {}): Capabilities {
  return {
    streaming: true,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: true,
    interrupt: false,
    modelSelection: false,
    toolCallEvents: true,
    toolConfirmation: false,
    sessionHistory: false,
    rawEvents: false,
    ...overrides,
  }
}

function session(id: string): AgentSession {
  return {
    id,
    projectId: 1,
    backend: 'opencode-v2',
    instanceId: 'inst-1',
    nativeSessionId: `native-${id}`,
    title: `Title ${id}`,
    status: 'active',
    turn: 'idle',
    capabilities: capabilities({ toolCallEvents: true }),
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
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
  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }
  addEventListener() {}
  close() {
    this.readyState = FakeEventSource.CLOSED
  }
}

function stubSessionList(list: AgentSession[]) {
  const fetchSpy = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input)
    if (url.endsWith('/history')) return { status: 200, json: async () => ({ events: [], truncated: false }) } as Response
    if (url.endsWith('/continue')) return { status: 200, json: async () => list[0] } as Response
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

describe('split message/activity log', () => {
  it('shows only messages and errors in the message column', () => {
    const entries: ChatEntry[] = [
      { id: 'u1', kind: 'message', role: 'user', text: 'hi', streaming: false },
      { id: 't1', kind: 'thinking', role: 'assistant', text: 'plan', streaming: false },
      {
        id: 'c1', kind: 'tool_call', role: 'assistant', text: 'read', name: 'read',
        callId: 'call-1', input: { path: '/a.md' }, streaming: false,
      },
      { id: 'a1', kind: 'message', role: 'assistant', text: 'done', streaming: false },
    ]
    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'messages' },
    })
    expect(wrapper.findAll('[data-test="bubble"]')).toHaveLength(2)
    expect(wrapper.find('[data-test="step-group"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="thinking"]').exists()).toBe(false)
  })

  it('shows only step groups in the activity pane', () => {
    const entries: ChatEntry[] = [
      { id: 'u1', kind: 'message', role: 'user', text: 'hi', streaming: false },
      { id: 't1', kind: 'thinking', role: 'assistant', text: 'plan', streaming: false },
      {
        id: 'c1', kind: 'tool_call', role: 'assistant', text: 'read', name: 'read',
        callId: 'call-1', input: { path: '/a.md' }, streaming: false,
      },
      { id: 'a1', kind: 'message', role: 'assistant', text: 'done', streaming: false },
    ]
    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'activity' },
    })
    expect(wrapper.find('[data-test="step-group"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="bubble"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="streaming"]').exists()).toBe(false)
  })

  it('shows an activity-specific empty note instead of the message one', () => {
    const wrapper = mount(MessageView, {
      props: { messages: [], capabilities: capabilities(), display: 'activity' },
    })
    expect(wrapper.text()).toContain('暂无思考与工具活动')
  })
})

describe('activity pane collapse', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    localStorage.clear()
    const agents = useAgentsStore()
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined as never)
  })

  afterEach(() => {
    useSessionsStore().unsubscribeAll()
    clearModelCache()
    vi.unstubAllGlobals()
  })

  async function mountPanel() {
    stubSessionList([session('sess-1')])
    const wrapper = mount(SessionChatPanel, {
      props: { projectId: 1, sessionId: 'sess-1' },
      global: { plugins: [testRouter()] },
    })
    await flushPromises()
    return wrapper
  }

  it('renders message and activity columns side by side', async () => {
    const wrapper = await mountPanel()
    expect(wrapper.find('[data-test="conversation-scroll"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="activity-scroll"]').exists()).toBe(true)
    // The composer stays in the left message column, not in a scroller.
    expect(wrapper.find('[data-test="conversation-scroll"]').find('form.send').exists()).toBe(false)
    expect(wrapper.find('.message-column form.send').exists()).toBe(true)
  })

  it('collapses to a rail and persists the choice', async () => {
    const wrapper = await mountPanel()
    await wrapper.find('[data-test="activity-collapse"]').trigger('click')
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="activity-rail"]').exists()).toBe(true)
    expect(localStorage.getItem('dev-platform-activity-pane-v1')).toBe('collapsed')

    await wrapper.find('[data-test="activity-expand"]').trigger('click')
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)
    expect(localStorage.getItem('dev-platform-activity-pane-v1')).toBe('expanded')
  })

  it('starts collapsed when the stored choice says so', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'collapsed')
    const wrapper = await mountPanel()
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="activity-rail"]').exists()).toBe(true)
  })
})
