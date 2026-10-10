// Unit tests for the task chat panel: a directly opened task URL loads its
// project's sessions first, an unknown task is reported rather than subscribed,
// switching tasks closes the previous stream and hides its messages, and
// unsupported controls are not rendered.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import BackendTag from '../components/BackendTag.vue'
import TaskChatPanel from '../components/TaskChatPanel.vue'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore, type ChatEntry } from '../stores/sessions'
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

function stubSessionList(list: AgentSession[]) {
  const fetchSpy = vi.fn(
    async () => ({ status: 200, json: async () => ({ sessions: list }) }) as Response,
  )
  vi.stubGlobal('fetch', fetchSpy)
  return fetchSpy
}

// The panel links back to its project, so the test router knows both shapes.
function testRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/projects/:projectId', name: 'project', component: { render: () => null } },
      { path: '/projects/:projectId/tasks/:sessionId', name: 'task', component: { render: () => null } },
    ],
  })
}

function mountPanel(sessionId: string) {
  return mount(TaskChatPanel, {
    props: { projectId: 1, sessionId },
    global: { plugins: [testRouter()] },
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

describe('TaskChatPanel', () => {
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
    vi.unstubAllGlobals()
  })

  it('loads the project sessions first when a task URL is opened directly', async () => {
    const fetchSpy = stubSessionList([session('sess-1'), session('sess-2')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    expect(fetchSpy).toHaveBeenCalled()
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(FakeEventSource.instances[0].url).toContain('/sessions/sess-1/')
    expect(wrapper.text()).toContain('Title sess-1')
  })

  it('reports an unknown task and opens no stream', async () => {
    stubSessionList([session('sess-1')])
    const wrapper = mountPanel('missing')
    await flushPromises()

    expect(wrapper.text()).toContain('任务不存在')
    expect(wrapper.find('a').attributes('href')).toBe('/projects/1')
    expect(FakeEventSource.instances).toHaveLength(0)
  })

  it('shows a failed project load as an error with a way back, not as a missing task', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ status: 500, json: async () => ({}) }) as Response),
    )
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    expect(wrapper.find('[role="alert"]').text()).toContain('发生未知错误。')
    expect(wrapper.text()).not.toContain('任务不存在')
    expect(wrapper.find('a').attributes('href')).toBe('/projects/1')
  })

  it('disables sending and explains when the task instance is stopped', async () => {
    stubSessionList([session('sess-1')])
    useAgentsStore().instances = [instance('stopped')]
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    expect(wrapper.text()).toContain('实例已停止，请在项目设置中启动')
    expect(wrapper.find('button.send-button').attributes('disabled')).toBeDefined()
  })

  it('keeps the typed message and reports the error when sending fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if ((init?.method ?? 'GET') === 'POST') {
          return { status: 500, json: async () => ({}) } as Response
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

  it('closes the previous stream and hides its messages when the task switches', async () => {
    stubSessionList([session('sess-1'), session('sess-2')])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    useSessionsStore().pushEntry('sess-1', entry('m1', 'OLD MESSAGE'))
    await flushPromises()
    expect(wrapper.text()).toContain('OLD MESSAGE')

    await wrapper.setProps({ sessionId: 'sess-2' })
    await flushPromises()

    expect(FakeEventSource.instances[0].readyState).toBe(FakeEventSource.CLOSED)
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
        if (String(input).endsWith('/models')) {
          return { status: 500, json: async () => ({}) } as Response
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
