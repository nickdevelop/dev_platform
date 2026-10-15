// Split-log tests: the left column shows only messages/errors, the right
// activity pane shows thinking/tool entries as a flat expanded list, and the
// middle divider collapses the pane (persisted) and drags to resize it.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import MessageView from '../features/chat/rendering/MessageView.vue'
import SessionChatPanel from '../features/chat/SessionChatPanel.vue'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore, type ChatEntry } from '../stores/sessions'
import { clearModelCache } from '../shared/modelCache'
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

  it('starts thinking, edit and shell expanded and folds only those', async () => {
    const entries: ChatEntry[] = [
      { id: 'u1', kind: 'message', role: 'user', text: 'hi', streaming: false },
      { id: 't1', kind: 'thinking', role: 'assistant', text: 'plan', streaming: false },
      {
        id: 'c1', kind: 'tool_call', role: 'assistant', text: 'read', name: 'read',
        callId: 'call-1', input: { path: '/a.md' }, streaming: false,
      },
      { id: 'r1', kind: 'tool_result', role: 'tool', text: '文件内容', callId: 'call-1', streaming: false },
      {
        id: 'c2', kind: 'tool_call', role: 'assistant', text: 'bash', name: 'bash',
        callId: 'call-2', input: { command: 'ls -la' }, streaming: false,
      },
      { id: 'r2', kind: 'tool_result', role: 'tool', text: '总用量 0', callId: 'call-2', streaming: false },
      { id: 'a1', kind: 'message', role: 'assistant', text: 'done', streaming: false },
    ]
    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'activity' },
    })
    // No step-group wrapper, no <details>: every entry is its own card.
    expect(wrapper.find('[data-test="step-group"]').exists()).toBe(false)
    expect(wrapper.findAll('details').length).toBe(0)
    expect(wrapper.find('[data-test="thinking"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="bubble"]').exists()).toBe(false)

    // Thinking and shell start expanded.
    expect(wrapper.find('[data-test="thinking"] .thinking-text').exists()).toBe(true)
    const calls = wrapper.findAll('[data-test="tool-call"]')
    expect(calls).toHaveLength(2)
    const read = calls[0]
    const shell = calls[1]

    // A non-priority tool (read) is a one-line summary: no fold, no body.
    expect(read.find('[data-test="flat-toggle"]').exists()).toBe(false)
    expect(read.find('.tool-arg').text()).toBe('/a.md')
    expect(read.find('.tool-output').exists()).toBe(false)

    // Shell folds and is already expanded with its output.
    expect(shell.find('[data-test="flat-toggle"]').exists()).toBe(true)
    expect(shell.find('.tool-output').text()).toContain('总用量 0')

    // A hand click folds the thinking card away.
    await wrapper.find('[data-test="thinking"] [data-test="flat-toggle"]').trigger('click')
    expect(wrapper.find('[data-test="thinking"] .thinking-text').exists()).toBe(false)

    // A hand click folds the shell card away too.
    await shell.find('[data-test="flat-toggle"]').trigger('click')
    expect(shell.find('.tool-output').exists()).toBe(false)
  })

  it('starts an edit call expanded and shows its diff', () => {
    const entries: ChatEntry[] = [
      {
        id: 'c1', kind: 'tool_call', role: 'assistant', text: 'edit', name: 'edit', callId: 'c1',
        input: { path: 'foo.ts', oldString: 'a', newString: 'b' }, streaming: false,
      },
      {
        id: 'r1', kind: 'tool_result', role: 'tool', text: 'Edited foo.ts', callId: 'c1', streaming: false,
        diff: { file: 'foo.ts', patch: 'x', additions: 1, deletions: 1 },
      },
    ]
    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'activity' },
    })
    const call = wrapper.find('[data-test="tool-call"]')
    expect(call.find('[data-test="flat-toggle"]').exists()).toBe(true)
    expect(call.find('[data-test="edit-tool"]').exists()).toBe(true)
  })

  it('reduces a subagent call to a named row and a detail button', async () => {
    const entries: ChatEntry[] = [
      {
        id: 'c1', kind: 'tool_call', role: 'assistant', text: 'task', name: 'task', callId: 'call-9',
        input: { description: 'sub job', prompt: 'do it' }, streaming: false,
      },
      { id: 'r1', kind: 'tool_result', role: 'tool', text: 'SECRET-DETAILS', callId: 'call-9', streaming: false },
    ]
    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'activity' },
    })
    const row = wrapper.find('[data-test="tool-call"]')
    expect(row.find('[data-test="subagent-open"]').exists()).toBe(true)
    expect(row.text()).toContain('sub job')
    // The subagent's detail is behind the button, never inline.
    expect(row.text()).not.toContain('SECRET-DETAILS')

    await row.find('[data-test="subagent-open"]').trigger('click')
    expect(wrapper.find('[data-test="subagent-dialog"]').text()).toContain('SECRET-DETAILS')
  })

  it('starts a question call expanded and renders its questions and options', async () => {
    const entries: ChatEntry[] = [
      {
        id: 'c1', kind: 'tool_call', role: 'assistant', text: 'question', name: 'question', callId: 'c1',
        input: {
          questions: [
            {
              header: '数据库',
              question: '用哪个数据库?',
              options: [
                { label: 'SQLite', description: '内嵌' },
                { label: 'Postgres', description: '服务端' },
              ],
            },
          ],
        },
        streaming: false,
      },
      { id: 'r1', kind: 'tool_result', role: 'tool', text: 'SQLite', callId: 'c1', streaming: false },
    ]
    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'activity' },
    })
    const call = wrapper.find('[data-test="tool-call"]')
    expect(call.find('[data-test="flat-toggle"]').exists()).toBe(true)
    expect(call.find('[data-test="question-text"]').text()).toContain('用哪个数据库?')
    expect(call.text()).toContain('SQLite')
    expect(call.text()).toContain('Postgres')
    // The answer comes through as the tool result.
    expect(call.find('.tool-output').text()).toContain('SQLite')

    // Collapsing hides the detail.
    await call.find('[data-test="flat-toggle"]').trigger('click')
    expect(call.find('[data-test="question-text"]').exists()).toBe(false)
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

  it('collapses from the middle divider and persists the choice', async () => {
    const wrapper = await mountPanel()
    // The toggle rides in the middle divider, not in the pane header.
    expect(wrapper.find('[data-test="split-divider"] [data-test="activity-collapse"]').exists()).toBe(true)
    await wrapper.find('[data-test="activity-collapse"]').trigger('click')
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    // Collapsing hides the pane entirely: no rail keeps occupying message width.
    expect(wrapper.find('[data-test="activity-rail"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="split-divider"]').exists()).toBe(true)
    expect(localStorage.getItem('dev-platform-activity-pane-v1')).toBe('collapsed')

    await wrapper.find('[data-test="activity-expand"]').trigger('click')
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)
    expect(localStorage.getItem('dev-platform-activity-pane-v1')).toBe('expanded')
  })

  it('starts collapsed when the stored choice says so', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'collapsed')
    const wrapper = await mountPanel()
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="activity-expand"]').exists()).toBe(true)
  })

  it('applies the stored divider width to the activity pane', async () => {
    localStorage.setItem('dev-platform-activity-width-v1', '420')
    const wrapper = await mountPanel()
    const pane = wrapper.find('[data-test="activity-pane"]')
    expect(pane.exists()).toBe(true)
    expect((pane.element as HTMLElement).style.width).toBe('420px')
  })

  it('restores the default width on divider double-click', async () => {
    localStorage.setItem('dev-platform-activity-width-v1', '500')
    const wrapper = await mountPanel()
    await wrapper.find('[data-test="split-divider"]').trigger('dblclick')
    expect((wrapper.find('[data-test="activity-pane"]').element as HTMLElement).style.width).toBe('340px')
    expect(localStorage.getItem('dev-platform-activity-width-v1')).toBe('340')
  })
})
