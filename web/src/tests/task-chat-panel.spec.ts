// Unit tests for the task chat panel: a directly opened task URL loads its
// project's sessions first, an unknown task is reported rather than subscribed,
// switching tasks closes the previous stream and hides its messages, and
// unsupported controls are not rendered.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import BackendTag from '../components/BackendTag.vue'
import TaskChatPanel from '../components/TaskChatPanel.vue'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore, type ChatEntry } from '../stores/sessions'
import type { AgentSession, Capabilities } from '../api/sessions'

function capabilities(overrides: Partial<Capabilities> = {}): Capabilities {
  return {
    streaming: true,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: true,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
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

function mountPanel(sessionId: string) {
  return mount(TaskChatPanel, { props: { projectId: 1, sessionId } })
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
    expect(FakeEventSource.instances).toHaveLength(0)
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

  it('shows the backend tag and no rename control in the header', async () => {
    stubSessionList([session('sess-1', { backend: 'pi' })])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    const tag = wrapper.findComponent(BackendTag)
    expect(tag.exists()).toBe(true)
    expect(tag.props('backend')).toBe('pi')
    expect(wrapper.findAll('button').some((button) => button.text() === '重命名')).toBe(false)
  })

  it('hides the model input when the session does not declare modelSelection', async () => {
    stubSessionList([session('sess-1', { capabilities: capabilities({ modelSelection: false, stop: true }) })])
    const wrapper = mountPanel('sess-1')
    await flushPromises()

    expect(wrapper.find('#session-model').exists()).toBe(false)
    expect(wrapper.find('.stop').exists()).toBe(true)
  })
})
