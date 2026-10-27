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
import * as filesApi from '../api/files'
import FileSidebar from '../features/files/FileSidebar.vue'
import FileViewer from '../features/files/FileViewer.vue'
import { installTestCSRF } from './csrf-fixture'

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

// The chat panel opens session streams in the background; the transport must
// have a token source even when the test never sends a request itself.
installTestCSRF()

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

  it('starts thinking directly and displays tools as concise summaries', async () => {
    const entries: ChatEntry[] = [
      { id: 'u1', kind: 'message', role: 'user', text: 'hi', streaming: false },
      { id: 't1', kind: 'thinking', role: 'assistant', text: 'plan', streaming: false },
      {
        id: 'c1', kind: 'tool_call', role: 'assistant', text: 'read', name: 'read',
        callId: 'call-1', input: { path: '/a.md' }, streaming: false,
      },
      { id: 'r1', kind: 'tool_result', role: 'tool', text: '读取了 /a.md 文件', callId: 'call-1', streaming: false },
      {
        id: 'c2', kind: 'tool_call', role: 'assistant', text: 'bash', name: 'bash',
        callId: 'call-2', input: { command: 'ls -la' }, streaming: false,
      },
      { id: 'r2', kind: 'tool_result', role: 'tool', text: '执行了 ls -la 命令', callId: 'call-2', streaming: false },
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

    // Thinking is output directly without a fold toggle.
    expect(wrapper.find('[data-test="thinking"] .thinking-text').exists()).toBe(true)
    expect(wrapper.find('[data-test="thinking"] [data-test="flat-toggle"]').exists()).toBe(false)
    const calls = wrapper.findAll('[data-test="tool-call"]')
    expect(calls).toHaveLength(2)
    const read = calls[0]
    const shell = calls[1]

    // Tools show as a one-line summary: no fold, no raw body.
    expect(read.find('[data-test="flat-toggle"]').exists()).toBe(false)
    expect(read.text()).toContain('读取了 /a.md 文件')
    expect(read.find('.tool-output').exists()).toBe(false)

    expect(shell.find('[data-test="flat-toggle"]').exists()).toBe(false)
    expect(shell.text()).toContain('执行了 ls -la 命令')
    expect(shell.find('.tool-output').exists()).toBe(false)
  })

  it('renders an edit call as a concise summary', () => {
    const entries: ChatEntry[] = [
      {
        id: 'c1', kind: 'tool_call', role: 'assistant', text: 'edit', name: 'edit', callId: 'c1',
        input: { path: 'foo.ts', oldString: 'a', newString: 'b' }, streaming: false,
      },
      {
        id: 'r1', kind: 'tool_result', role: 'tool', text: '编辑了 foo.ts 文件', callId: 'c1', streaming: false,
        diff: { file: 'foo.ts', patch: 'x', additions: 1, deletions: 1 },
      },
    ]
    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'activity' },
    })
    const call = wrapper.find('[data-test="tool-call"]')
    expect(call.text()).toContain('编辑了 foo.ts 文件')
    expect(call.find('[data-test="edit-tool"]').exists()).toBe(false)
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

  it('renders question tool in the message column and excludes it from activity pane', async () => {
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
      { id: 'r1', kind: 'tool_result', role: 'tool', text: '已回答: SQLite', callId: 'c1', streaming: false },
    ]
    // 1. Excluded from activity pane
    const activityWrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'activity' },
    })
    expect(activityWrapper.find('[data-test="tool-call"]').exists()).toBe(false)

    // 2. Rendered in message column
    const messageWrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'messages' },
    })
    const card = messageWrapper.find('[data-test="question-dialog-card"]')
    expect(card.exists()).toBe(true)
    expect(card.find('[data-test="question-text"]').text()).toContain('用哪个数据库?')
    expect(card.text()).toContain('SQLite')
    expect(card.text()).toContain('Postgres')
    expect(card.find('[data-test="question-answer"]').text()).toContain('SQLite')
  })

  it('renders deepseek ask_user_question in the message column as a question card', () => {
    const entries: ChatEntry[] = [
      {
        id: 'c-dsh', kind: 'tool_call', role: 'assistant', text: 'ask_user_question', name: 'ask_user_question', callId: 'c-dsh-1',
        // Support arguments serialized as JSON string (DeepSeek wire protocol)
        input: JSON.stringify({
          header: '云平台',
          question: '请选择部署目标云平台',
          options: ['AWS', 'GCP', '阿里云'],
          multi_select: false,
        }),
        streaming: false,
      },
      { id: 'r-dsh', kind: 'tool_result', role: 'tool', text: '已回答: 阿里云', callId: 'c-dsh-1', streaming: false },
    ]

    // 1. Excluded from activity pane
    const activityWrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'activity' },
    })
    expect(activityWrapper.find('[data-test="tool-call"]').exists()).toBe(false)

    // 2. Rendered in message column
    const messageWrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'messages' },
    })
    const card = messageWrapper.find('[data-test="question-dialog-card"]')
    expect(card.exists()).toBe(true)
    expect(card.find('[data-test="question-text"]').text()).toContain('请选择部署目标云平台')
    expect(card.text()).toContain('AWS')
    expect(card.text()).toContain('阿里云')
    expect(card.find('[data-test="question-answer"]').text()).toContain('阿里云')
  })

  it('renders multiple questions and their corresponding answers line-by-line', () => {
    const entries: ChatEntry[] = [
      {
        id: 'c-multi', kind: 'tool_call', role: 'assistant', text: 'question', name: 'question', callId: 'c-multi-1',
        input: {
          questions: [
            {
              id: 'q1',
              header: '旅行偏好',
              question: '如果马上可以出发去旅行，您更倾向哪种？',
              options: [
                { label: '自然风景', description: '山海湖草' },
                { label: '人文历史', description: '古城博物馆' },
              ],
            },
            {
              id: 'q2',
              header: '兴趣爱好',
              question: '如果时间完全自由，您最想培养什么爱好？',
              options: [
                { label: '音乐类', description: '吉他钢琴' },
                { label: '美食类', description: '下厨调酒' },
              ],
            },
          ],
        },
        streaming: false,
      },
      {
        id: 'r-multi', kind: 'tool_result', role: 'tool',
        // DeepSeek structured answers JSON
        text: '已回答: {"answers":[{"id":"q1","selected":["自然风景"]},{"id":"q2","selected":["美食类"]}]}',
        callId: 'c-multi-1', streaming: false,
      },
    ]

    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'messages' },
    })
    const card = wrapper.find('[data-test="question-dialog-card"]')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('共 2 题')

    const questionBlocks = card.findAll('[data-test="question-item"]')
    expect(questionBlocks).toHaveLength(2)

    // Question 1 and its corresponding answer
    expect(questionBlocks[0].find('[data-test="question-text"]').text()).toContain('如果马上可以出发去旅行')
    expect(questionBlocks[0].find('[data-test="question-answer"]').text()).toContain('自然风景')

    // Question 2 and its corresponding answer
    expect(questionBlocks[1].find('[data-test="question-text"]').text()).toContain('如果时间完全自由')
    expect(questionBlocks[1].find('[data-test="question-answer"]').text()).toContain('美食类')
  })

  it('shows an activity-specific empty note instead of the message one', () => {
    const wrapper = mount(MessageView, {
      props: { messages: [], capabilities: capabilities(), display: 'activity' },
    })
    expect(wrapper.text()).toContain('暂无活动')
  })
})

describe('activity pane floating button and in-tab popup', () => {
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

  it('defaults to collapsed with a header preview button and a full-width message area', async () => {
    const wrapper = await mountPanel()
    expect(wrapper.find('[data-test="conversation-scroll"]').exists()).toBe(true)
    // The message composer is in the message column.
    expect(wrapper.find('.message-column form.send').exists()).toBe(true)
    // The activity pane is hidden by default and represented by a header preview button.
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    const toggleBtn = wrapper.find('[data-test="toggle-preview-btn"]')
    expect(toggleBtn.exists()).toBe(true)
    expect(toggleBtn.text()).toContain('预览')
    // Preview comes before the file tree in the title row.
    const headerTools = wrapper.findAll('.title-row .header-tool-btn')
    expect(headerTools.map((button) => button.attributes('data-test'))).toEqual([
      'toggle-preview-btn',
      'toggle-filetree-btn',
    ])
  })

  it('opens in-tab popup on preview button click and persists expanded choice', async () => {
    const wrapper = await mountPanel()
    const toggleBtn = wrapper.find('[data-test="toggle-preview-btn"]')
    expect(toggleBtn.exists()).toBe(true)

    await toggleBtn.trigger('click')
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="activity-scroll"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="toggle-preview-btn"]').exists()).toBe(false)
    expect(localStorage.getItem('dev-platform-activity-pane-v1')).toBe('expanded')
  })

  it('collapses back to preview button on hide click and persists collapsed choice', async () => {
    const wrapper = await mountPanel()
    await wrapper.find('[data-test="toggle-preview-btn"]').trigger('click')
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)

    const hideBtn = wrapper.find('[data-test="activity-hide"]')
    expect(hideBtn.exists()).toBe(true)
    await hideBtn.trigger('click')

    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="toggle-preview-btn"]').exists()).toBe(true)
    expect(localStorage.getItem('dev-platform-activity-pane-v1')).toBe('collapsed')
  })

  it('starts expanded when stored choice says so', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'expanded')
    const wrapper = await mountPanel()
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="toggle-preview-btn"]').exists()).toBe(false)
  })

  it('closes the open popup when Escape key is pressed', async () => {
    const wrapper = await mountPanel()
    await wrapper.find('[data-test="toggle-preview-btn"]').trigger('click')
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()

    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="toggle-preview-btn"]').exists()).toBe(true)
  })

  it('displays activity count badge on header preview button and popup title', async () => {
    const sessionsStore = useSessionsStore()
    const wrapper = await mountPanel()

    // Push thinking and tool messages to sess-1
    sessionsStore.messages['sess-1'] = [
      { id: 'u1', kind: 'message', role: 'user', text: 'hi', streaming: false },
      { id: 't1', kind: 'thinking', role: 'assistant', text: 'thinking...', streaming: false },
      { id: 'c1', kind: 'tool_call', role: 'assistant', text: 'call', name: 'read', callId: '1', streaming: false },
    ]
    await flushPromises()

    const toggleBtn = wrapper.find('[data-test="toggle-preview-btn"]')
    expect(toggleBtn.find('.mini-count').text()).toBe('2')

    await toggleBtn.trigger('click')
    const pane = wrapper.find('[data-test="activity-pane"]')
    expect(pane.find('.activity-title [data-test="activity-count"]').text()).toBe('2')
  })
})

describe('three-column chat layout with preview tabs and file tree drawer', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    localStorage.clear()
    const agents = useAgentsStore()
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined as never)
    vi.spyOn(filesApi, 'getFileTree').mockResolvedValue([
      { name: 'main.go', path: 'src/main.go', isDir: false },
      { name: 'README.md', path: 'README.md', isDir: false },
    ])
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])
    vi.spyOn(filesApi, 'getFileContent').mockImplementation(async (_pid, path) => ({
      path,
      size: 100,
      lines: 10,
      mimeType: 'text/plain',
      isBinary: false,
      content: `content of ${path}`,
    }))
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

  it('renders three columns: message column, preview column, and filetree drawer', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'expanded')
    const wrapper = await mountPanel()

    // 1st column: message area
    expect(wrapper.find('.message-column').exists()).toBe(true)

    // 2nd column: preview pane
    const preview = wrapper.find('[data-test="activity-pane"]')
    expect(preview.exists()).toBe(true)

    // Fixed first tab is "思考与工具"
    const fixedTab = preview.find('[data-test="tab-activity"]')
    expect(fixedTab.exists()).toBe(true)
    expect(fixedTab.text()).toContain('思考与工具')
    expect(fixedTab.classes()).toContain('is-active')

    // 3rd column: file tree drawer (docked by default)
    const filetree = wrapper.find('[data-test="filetree-drawer"]')
    expect(filetree.exists()).toBe(true)
    expect(filetree.classes()).toContain('filetree-docked')
    expect(filetree.text()).toContain('工作区文件')
  })

  it('opens a file as a preview tab when selected in the file tree and auto-expands preview if collapsed', async () => {
    // Start with preview collapsed
    localStorage.setItem('dev-platform-activity-pane-v1', 'collapsed')
    const wrapper = await mountPanel()

    // Preview is initially collapsed
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="toggle-preview-btn"]').exists()).toBe(true)

    // Select a file from file sidebar
    const sidebar = wrapper.findComponent(FileSidebar)
    expect(sidebar.exists()).toBe(true)
    sidebar.vm.$emit('select', 'src/main.go')
    await flushPromises()

    // Preview pane should now be auto-expanded
    const preview = wrapper.find('[data-test="activity-pane"]')
    expect(preview.exists()).toBe(true)

    // A file tab should be created and activated
    const fileTabs = preview.findAll('[data-test="tab-file"]')
    expect(fileTabs.length).toBe(1)
    expect(fileTabs[0].text()).toContain('main.go')
    expect(fileTabs[0].classes()).toContain('is-active')

    // FileViewer is mounted for the active file
    const viewer = preview.findComponent(FileViewer)
    expect(viewer.exists()).toBe(true)
    expect(viewer.props('activePath')).toBe('src/main.go')
    await vi.waitFor(() => expect(viewer.find('.empty-state').exists()).toBe(false))
    expect(viewer.text()).toContain('content of src/main.go')
  })

  it('switches between 思考与工具 and file tabs, and handles closing file tabs with proper fallback', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'expanded')
    const wrapper = await mountPanel()
    const sidebar = wrapper.findComponent(FileSidebar)

    // Open two files
    sidebar.vm.$emit('select', 'src/main.go')
    await flushPromises()
    sidebar.vm.$emit('select', 'README.md')
    await flushPromises()

    const preview = wrapper.find('[data-test="activity-pane"]')
    let fileTabs = preview.findAll('[data-test="tab-file"]')
    expect(fileTabs.length).toBe(2)
    expect(fileTabs[1].classes()).toContain('is-active') // README.md is active

    // Switch back to 思考与工具
    const fixedTab = preview.find('[data-test="tab-activity"]')
    await fixedTab.trigger('click')
    expect(fixedTab.classes()).toContain('is-active')
    expect(wrapper.find('[data-test="activity-scroll"]').exists()).toBe(true)

    // Switch to main.go tab
    await fileTabs[0].trigger('click')
    expect(fileTabs[0].classes()).toContain('is-active')

    // Close main.go tab
    const closeMainBtn = fileTabs[0].find('[data-test="close-file-tab"]')
    await closeMainBtn.trigger('click')
    await flushPromises()

    // Remaining file tab is README.md and becomes active
    fileTabs = preview.findAll('[data-test="tab-file"]')
    expect(fileTabs.length).toBe(1)
    expect(fileTabs[0].text()).toContain('README.md')
    expect(fileTabs[0].classes()).toContain('is-active')

    // Close README.md tab -> falls back to 思考与工具
    const closeReadmeBtn = fileTabs[0].find('[data-test="close-file-tab"]')
    await closeReadmeBtn.trigger('click')
    await flushPromises()

    expect(preview.findAll('[data-test="tab-file"]').length).toBe(0)
    expect(fixedTab.classes()).toContain('is-active')
    expect(wrapper.find('[data-test="activity-scroll"]').exists()).toBe(true)
  })

  it('scrolls activity pane to bottom when switching back to 思考与工具 tab', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'expanded')
    const wrapper = await mountPanel()
    const sessionsStore = useSessionsStore()

    // Add activity entries
    sessionsStore.messages['sess-1'] = [
      { id: 't1', kind: 'thinking', role: 'assistant', text: 'thinking 1', streaming: false },
      { id: 'c1', kind: 'tool_call', role: 'assistant', text: 'tool 1', name: 'read', callId: '1', streaming: false },
      { id: 't2', kind: 'thinking', role: 'assistant', text: 'thinking 2', streaming: false },
    ]
    await flushPromises()

    const preview = wrapper.find('[data-test="activity-pane"]')
    const activityScrollEl = preview.find('[data-test="activity-scroll"]').element as HTMLElement
    Object.defineProperty(activityScrollEl, 'scrollHeight', { configurable: true, writable: true, value: 800 })
    Object.defineProperty(activityScrollEl, 'clientHeight', { configurable: true, writable: true, value: 300 })

    const sidebar = wrapper.findComponent(FileSidebar)
    sidebar.vm.$emit('select', 'src/main.go')
    await flushPromises()

    // Simulate scroll offset changed while away
    activityScrollEl.scrollTop = 0

    // Switch back to 思考与工具
    const fixedTab = preview.find('[data-test="tab-activity"]')
    await fixedTab.trigger('click')
    await flushPromises()

    expect(activityScrollEl.scrollTop).toBe(800)
  })

  it('supports file tree drawer pinning/unpinning, collapse and header toggle', async () => {
    const wrapper = await mountPanel()

    const drawer = wrapper.find('[data-test="filetree-drawer"]')
    expect(drawer.exists()).toBe(true)
    expect(drawer.classes()).toContain('filetree-docked')

    // Toggle pin to unpinned (floating mode)
    const pinBtn = drawer.find('[data-test="filetree-pin-btn"]')
    await pinBtn.trigger('click')
    await flushPromises()

    expect(drawer.classes()).toContain('filetree-floating')
    expect(wrapper.find('[data-test="filetree-backdrop"]').exists()).toBe(true)
    expect(localStorage.getItem('dev-platform-chat-filetree-pinned-v1')).toBe('unpinned')

    // Click backdrop to close floating drawer
    await wrapper.find('[data-test="filetree-backdrop"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="filetree-drawer"]').exists()).toBe(false)

    // Header toggle button can re-open the file tree drawer
    const toggleBtn = wrapper.find('[data-test="toggle-filetree-btn"]')
    expect(toggleBtn.exists()).toBe(true)
    await toggleBtn.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="filetree-drawer"]').exists()).toBe(true)

    // Pin again to dock
    await wrapper.find('[data-test="filetree-pin-btn"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="filetree-drawer"]').classes()).toContain('filetree-docked')
    expect(localStorage.getItem('dev-platform-chat-filetree-pinned-v1')).toBe('pinned')

    // Collapse drawer
    const collapseBtn = wrapper.find('[data-test="filetree-collapse-btn"]')
    await collapseBtn.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="filetree-drawer"]').exists()).toBe(false)
  })

  it('supports resizing preview pane width via resizer handle and persists width', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'expanded')
    localStorage.setItem('dev-platform-preview-width-v1', '460')
    const wrapper = await mountPanel()

    const preview = wrapper.find('[data-test="activity-pane"]')
    expect(preview.exists()).toBe(true)
    const resizer = preview.find('[data-test="preview-resizer"]')
    expect(resizer.exists()).toBe(true)

    // Trigger mousedown on resizer
    await resizer.trigger('mousedown', { button: 0, clientX: 500 })

    // Simulate drag mousemove (clientX from 500 to 450 -> delta = 50px wider)
    window.dispatchEvent(new MouseEvent('mousemove', { clientX: 450 }))
    await flushPromises()

    // Trigger mouseup
    window.dispatchEvent(new MouseEvent('mouseup'))
    await flushPromises()

    expect(localStorage.getItem('dev-platform-preview-width-v1')).toBe('510')
  })
})
