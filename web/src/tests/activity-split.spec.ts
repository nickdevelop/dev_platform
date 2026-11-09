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
    if (new URL(url, 'http://localhost').pathname.endsWith('/history')) return { status: 200, json: async () => ({ events: [], truncated: false }) } as Response
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

  it('starts thinking directly and shows full tool input and output', () => {
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
    // No step-group wrapper, no <details>: every entry is its own block.
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

    // Tools expose their full input and output instead of a summary line.
    expect(read.find('[data-test="tool-input"]').text()).toContain('/a.md')
    expect(read.find('.tool-output').text()).toContain('读取了 /a.md 文件')
    expect(read.text()).not.toContain('运行中…')

    expect(shell.find('[data-test="tool-input"]').text()).toContain('ls -la')
    expect(shell.find('.tool-output').text()).toContain('执行了 ls -la 命令')
  })

  it('renders an edit call with its diff body', () => {
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
    expect(call.text()).toContain('foo.ts')
    expect(call.find('[data-test="edit-diff"]').exists()).toBe(true)
  })

  it('shows a subagent call with its full input and result inline', () => {
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
    expect(row.find('[data-test="subagent-open"]').exists()).toBe(false)
    expect(row.text()).toContain('sub job')
    expect(row.text()).toContain('do it')
    expect(row.text()).toContain('SECRET-DETAILS')
    expect(wrapper.find('[data-test="subagent-dialog"]').exists()).toBe(false)
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

  it('renders multiple questions and their corresponding answers after all questions', () => {
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

    // Questions do not contain answers inside their item blocks
    expect(questionBlocks[0].find('[data-test="question-text"]').text()).toContain('如果马上可以出发去旅行')
    expect(questionBlocks[0].find('[data-test="question-answer"]').exists()).toBe(false)

    expect(questionBlocks[1].find('[data-test="question-text"]').text()).toContain('如果时间完全自由')
    expect(questionBlocks[1].find('[data-test="question-answer"]').exists()).toBe(false)

    // Answers are displayed in the answer section after all questions
    const answerSection = card.find('[data-test="question-answer-section"]')
    expect(answerSection.exists()).toBe(true)

    const answerLines = answerSection.findAll('[data-test="question-answer"]')
    expect(answerLines).toHaveLength(2)
    expect(answerLines[0].text()).toContain('1.')
    expect(answerLines[0].text()).toContain('自然风景')
    expect(answerLines[1].text()).toContain('2.')
    expect(answerLines[1].text()).toContain('美食类')
  })

  it('renders answers after all questions and formats json answers line-by-line with 1 2 3 index', () => {
    const entries: ChatEntry[] = [
      {
        id: 'c-3q', kind: 'tool_call', role: 'assistant', text: 'question', name: 'question', callId: 'c-3q-1',
        input: {
          questions: [
            {
              id: 'task_topic',
              header: '任务方向',
              question: '你希望我接下来帮你做什么类型的事情？',
              options: [
                { label: '写代码 / 改项目', description: '修 bug、写功能' },
                { label: '禅道工时与工作总结', description: '生成日报周报' },
              ],
            },
            {
              id: 'reply_language',
              header: '回复语言',
              question: '之后回复你偏好使用哪种语言？',
              options: [
                { label: '中文（推荐）', description: '后续全部使用中文回复' },
                { label: 'English', description: 'Reply in English' },
              ],
            },
            {
              id: 'prompt_preference',
              header: '提问偏好',
              question: '以后干活时，你希望我以什么频率向你提问确认？',
              options: [
                { label: '多问、先确认再动手（推荐）', description: '遇到不确定的地方就询问' },
                { label: '只在必要时间', description: '仅在关键决策点提问' },
              ],
            },
          ],
        },
        streaming: false,
      },
      {
        id: 'r-3q', kind: 'tool_result', role: 'tool',
        text: '已回答: {"answers":[{"id":"task_topic","selected":["写代码 / 改项目"]},{"id":"reply_language","selected":["中文（推荐）"]},{"id":"prompt_preference","selected":["多问、先确认再动手（推荐）"]}]}',
        callId: 'c-3q-1', streaming: false,
      },
    ]

    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'messages' },
    })
    const card = wrapper.find('[data-test="question-dialog-card"]')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('共 3 题')

    const questionBlocks = card.findAll('[data-test="question-item"]')
    expect(questionBlocks).toHaveLength(3)

    // No answer displayed inside individual question blocks
    for (const qBlock of questionBlocks) {
      expect(qBlock.find('[data-test="question-answer"]').exists()).toBe(false)
    }

    // Answers are displayed in the section after all questions
    const answerSection = card.find('[data-test="question-answer-section"]')
    expect(answerSection.exists()).toBe(true)

    const answerLines = answerSection.findAll('[data-test="question-answer"]')
    expect(answerLines).toHaveLength(3)
    expect(answerLines[0].text()).toContain('1.')
    expect(answerLines[0].text()).toContain('写代码 / 改项目')
    expect(answerLines[1].text()).toContain('2.')
    expect(answerLines[1].text()).toContain('中文（推荐）')
    expect(answerLines[2].text()).toContain('3.')
    expect(answerLines[2].text()).toContain('多问、先确认再动手（推荐）')
  })

  it('shows unified waiting row after all questions when unanswered', () => {
    const entries: ChatEntry[] = [
      {
        id: 'c-wait', kind: 'tool_call', role: 'assistant', text: 'question', name: 'question', callId: 'c-wait-1',
        input: {
          questions: [
            { id: 'q1', question: '问题 1' },
            { id: 'q2', question: '问题 2' },
          ],
        },
        streaming: false,
      },
    ]

    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities(), display: 'messages' },
    })
    const card = wrapper.find('[data-test="question-dialog-card"]')
    expect(card.exists()).toBe(true)

    // Unified waiting row at the bottom, not repeated inside each question block
    const questionBlocks = card.findAll('[data-test="question-item"]')
    for (const qBlock of questionBlocks) {
      expect(qBlock.find('[data-test="question-waiting"]').exists()).toBe(false)
    }
    const waitingRow = card.find('[data-test="question-waiting"]')
    expect(waitingRow.exists()).toBe(true)
    expect(waitingRow.text()).toContain('等待回答…')
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

  it('defaults to collapsed with right-side trigger and collapsed preview state', async () => {
    const wrapper = await mountPanel()
    expect(wrapper.find('[data-test="conversation-scroll"]').exists()).toBe(true)
    // The message composer is in the message column.
    expect(wrapper.find('.message-column form.send').exists()).toBe(true)
    // The activity pane is hidden by default and represented by the right trigger.
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    const toggleBtn = wrapper.find('[data-test="toggle-preview-btn"]')
    expect(toggleBtn.exists()).toBe(true)
    expect(toggleBtn.attributes('aria-pressed')).toBe('false')
    expect(toggleBtn.attributes('title')).toBe('展开预览面板')
    // Bulky right rail has been removed in favor of top-bar layout button and clean collapsed edge
    expect(wrapper.find('[data-test="chat-side-rail-right"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="chat-side-rail-left"]').exists()).toBe(false)
  })

  it('opens in-tab popup on preview button click and persists expanded choice', async () => {
    const wrapper = await mountPanel()
    const toggleBtn = wrapper.find('[data-test="toggle-preview-btn"]')
    expect(toggleBtn.exists()).toBe(true)

    await toggleBtn.trigger('click')
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)
    // The workspace pane restores the fixed 链路 tab (its scroll area) on expand
    expect(wrapper.find('[data-test="activity-scroll"]').exists()).toBe(true)
    // Preview pane itself has no collapse/hide button (managed via session tabs layout toggle)
    expect(wrapper.find('[data-test="activity-hide"]').exists()).toBe(false)
    expect(localStorage.getItem('dev-platform-activity-pane-v1')).toBe('expanded')
  })

  it('has no self collapse button in preview pane and persists collapsed choice when collapsed', async () => {
    const wrapper = await mountPanel()
    await wrapper.find('[data-test="toggle-preview-btn"]').trigger('click')
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)

    // Verify preview pane header does not contain collapse button
    expect(wrapper.find('[data-test="activity-hide"]').exists()).toBe(false)

    // Press Escape to collapse
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()

    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="toggle-preview-btn"]').attributes('aria-pressed')).toBe('false')
    expect(localStorage.getItem('dev-platform-activity-pane-v1')).toBe('collapsed')
  })

  it('starts expanded when stored choice says so', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'expanded')
    const wrapper = await mountPanel()
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="activity-hide"]').exists()).toBe(false)
  })

  it('closes the open popup when Escape key is pressed', async () => {
    const wrapper = await mountPanel()
    await wrapper.find('[data-test="toggle-preview-btn"]').trigger('click')
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()

    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="toggle-preview-btn"]').attributes('aria-pressed')).toBe('false')
  })

  it('keeps conversation area clean without thinking and tool calls, restoring the 链路 tab', async () => {
    const sessionsStore = useSessionsStore()
    const wrapper = await mountPanel()

    // Push thinking and tool messages to sess-1
    sessionsStore.messages['sess-1'] = [
      { id: 'u1', kind: 'message', role: 'user', text: 'hi', streaming: false },
      { id: 't1', kind: 'thinking', role: 'assistant', text: 'thinking...', streaming: false },
      { id: 'c1', kind: 'tool_call', role: 'assistant', text: 'call', name: 'read', callId: '1', streaming: false },
    ]
    await flushPromises()

    // The main conversation area remains clean without thinking and tool calls
    const conversationArea = wrapper.find('.conversation-area')
    expect(conversationArea.text()).toContain('hi')
    expect(conversationArea.text()).not.toContain('thinking...')
    expect(conversationArea.text()).not.toContain('call')

    // The preview pane restores the fixed 链路 tab with a count badge
    await wrapper.find('[data-test="toggle-preview-btn"]').trigger('click')
    const pane = wrapper.find('[data-test="activity-pane"]')
    expect(pane.find('.activity-title').exists()).toBe(true)
    expect(pane.find('.activity-title').text()).toContain('链路')
    expect(pane.find('.activity-title').classes()).toContain('is-active')
    expect(pane.find('[data-test="activity-count"]').exists()).toBe(true)
    expect(pane.find('[data-test="activity-count"]').text()).toBe('2')

    // The activity scroll area inside the tab shows the flat activity stream
    expect(wrapper.find('[data-test="activity-scroll"]').exists()).toBe(true)
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

  it('renders the message column and unified workspace pane with preview on left and file tree on right', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'expanded')
    const wrapper = await mountPanel()

    // Bulky side rail is completely eliminated
    expect(wrapper.find('[data-test="chat-side-rail-left"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="chat-side-rail-right"]').exists()).toBe(false)

    // 1st column: message area
    expect(wrapper.find('.message-column').exists()).toBe(true)

    // 2nd column: unified workspace pane
    const preview = wrapper.find('[data-test="activity-pane"]')
    expect(preview.exists()).toBe(true)

    // The fixed 链路 tab is the default active tab
    expect(preview.find('[data-test="tab-activity"]').exists()).toBe(true)
    expect(preview.find('[data-test="tab-activity"]').text()).toContain('链路')
    expect(preview.find('[data-test="tab-activity"]').classes()).toContain('is-active')
    expect(preview.find('[data-test="activity-scroll"]').exists()).toBe(true)
    // File preview empty state only shows on the file tab side
    expect(preview.find('[data-test="preview-empty-state"]').exists()).toBe(true)

    // File tree drawer (docked by default, placed on the right inside workspace)
    const filetree = wrapper.find('[data-test="filetree-drawer"]')
    expect(filetree.exists()).toBe(true)
    expect(filetree.classes()).toContain('filetree-docked')
    expect(filetree.text()).toContain('工作区文件')

    // DOM order inside workspace: preview section is on the left, file tree section is on the right
    const workspaceChildren = Array.from(
      preview.find('.workspace-inner-split').element.children,
    ).map((child) => (child as HTMLElement).className)
    const previewSectionIdx = workspaceChildren.findIndex((name) => name.includes('workspace-preview-section'))
    const filetreeSectionIdx = workspaceChildren.findIndex((name) => name.includes('workspace-filetree-section'))
    expect(previewSectionIdx).toBeGreaterThanOrEqual(0)
    expect(previewSectionIdx).toBeLessThan(filetreeSectionIdx)
  })

  it('opens a file as a preview tab when selected in the file tree and auto-expands preview if collapsed', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'expanded')
    const wrapper = await mountPanel()

    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(true)

    // Select a file from file sidebar
    const sidebar = wrapper.findComponent(FileSidebar)
    expect(sidebar.exists()).toBe(true)
    sidebar.vm.$emit('select', 'src/main.go')
    await flushPromises()

    // Preview pane has active file tab
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

  it('switches between file tabs and falls back to empty state when all tabs are closed', async () => {
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

    // Close README.md tab -> falls back to the fixed 链路 tab
    const closeReadmeBtn = fileTabs[0].find('[data-test="close-file-tab"]')
    await closeReadmeBtn.trigger('click')
    await flushPromises()

    expect(preview.findAll('[data-test="tab-file"]').length).toBe(0)
    expect(preview.find('[data-test="tab-activity"]').classes()).toContain('is-active')
  })

  it('when preview pane is expanded, file tree expands and collapses eating preview width, not conversation width', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'expanded')
    localStorage.setItem('dev-platform-preview-width-v1', '600')
    localStorage.setItem('dev-platform-chat-filetree-collapsed-v1', 'collapsed')
    const wrapper = await mountPanel()

    const preview = wrapper.find('[data-test="activity-pane"]')
    expect(preview.exists()).toBe(true)

    // Initially file tree is collapsed
    expect(wrapper.find('[data-test="filetree-drawer"]').exists()).toBe(false)
    // Preview takes full previewWidth
    const fullWidth = (wrapper.vm as any).previewWidth
    expect((wrapper.vm as any).computedPreviewWidth).toBe(fullWidth)

    // Expand file tree
    const toggleFileTreeBtn = wrapper.find('[data-test="toggle-filetree-btn"]')
    await toggleFileTreeBtn.trigger('click')
    await flushPromises()

    // File tree is now docked and visible
    const filetree = wrapper.find('[data-test="filetree-drawer"]')
    expect(filetree.exists()).toBe(true)
    expect(filetree.classes()).toContain('filetree-docked')

    // Preview pane shrunk by exactly 240px (eating preview width, preserving total width)
    expect((wrapper.vm as any).computedPreviewWidth).toBe(fullWidth - 240)

    // Collapse file tree again
    const collapseBtn = filetree.find('[data-test="filetree-collapse-btn"]')
    await collapseBtn.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="filetree-drawer"]').exists()).toBe(false)
    // Preview pane re-claims full width
    expect((wrapper.vm as any).computedPreviewWidth).toBe(fullWidth)
  })

  it('supports file tree drawer pinning/unpinning, collapse and header toggle', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'expanded')
    const wrapper = await mountPanel()

    const drawer = wrapper.find('[data-test="filetree-drawer"]')
    expect(drawer.exists()).toBe(true)
    expect(drawer.classes()).toContain('filetree-docked')

    // Toggle pin to unpinned (floating mode)
    const pinBtn = drawer.find('[data-test="filetree-pin-btn"]')
    await pinBtn.trigger('click')
    await flushPromises()

    expect(drawer.classes()).toContain('filetree-floating')
    expect(wrapper.find('[data-test="activity-pane"]').classes()).toContain('has-floating-filetree')
    expect(wrapper.find('[data-test="filetree-backdrop"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="preview-floating-backdrop"]').exists()).toBe(true)
    expect(localStorage.getItem('dev-platform-chat-filetree-pinned-v1')).toBe('unpinned')

    // Click backdrop to close floating drawer
    await wrapper.find('[data-test="filetree-backdrop"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="filetree-drawer"]').exists()).toBe(false)

    // The header toggle can re-open the file tree drawer
    const toggleBtn = wrapper.find('[data-test="toggle-filetree-btn"]')
    expect(toggleBtn.exists()).toBe(true)
    expect(toggleBtn.attributes('aria-pressed')).toBe('false')
    await toggleBtn.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="filetree-drawer"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="filetree-drawer"]').classes()).toContain('filetree-floating')

    // Pin directly in workspace pane to dock
    await wrapper.find('[data-test="filetree-pin-btn"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="filetree-drawer"]').classes()).toContain('filetree-docked')
    expect(wrapper.find('[data-test="activity-pane"]').classes()).not.toContain('has-floating-filetree')
    expect(wrapper.find('[data-test="filetree-backdrop"]').exists()).toBe(false)
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

  it('keeps chat-header inside message-column and places preview and file tree in unified workspace without right rail', async () => {
    localStorage.setItem('dev-platform-activity-pane-v1', 'expanded')
    const wrapper = await mountPanel()

    const split = wrapper.find('[data-test="conversation-split"]')
    expect(split.exists()).toBe(true)

    // chat-header is inside message-column, matching the main chat column width
    const msgColumn = split.find('.message-column')
    expect(msgColumn.exists()).toBe(true)
    const header = msgColumn.find('.chat-header')
    expect(header.exists()).toBe(true)

    // Workspace pane is a sibling of message-column inside the split; rail is completely removed
    expect(split.find('[data-test="activity-pane"]').exists()).toBe(true)
    expect(split.find('[data-test="filetree-drawer"]').exists()).toBe(true)
    expect(split.find('[data-test="chat-side-rail-right"]').exists()).toBe(false)
  })

  it('renders thinking and tools as flat detail blocks with full arguments', async () => {
    const longCommand = 'git status --porcelain && npm test && cargo build --release --verbose --package dev-platform'
    const entries: ChatEntry[] = [
      { id: 't1', kind: 'thinking', role: 'assistant', text: 'detailed reasoning steps', streaming: false },
      {
        id: 'c1',
        kind: 'tool_call',
        role: 'assistant',
        text: 'bash',
        name: 'bash',
        callId: 'call-1',
        input: { command: longCommand },
        streaming: false,
      },
    ]

    const wrapper = mount(MessageView, {
      props: {
        messages: entries,
        capabilities: capabilities({ toolCallEvents: true }),
        display: 'activity',
      },
    })

    // Both kinds are flat activity blocks; neither is wrapped in a card.
    const thinkingEl = wrapper.find('[data-test="thinking"]')
    expect(thinkingEl.exists()).toBe(true)
    expect(thinkingEl.classes()).toContain('flat-thinking')

    const toolEl = wrapper.find('[data-test="tool-call"]')
    expect(toolEl.exists()).toBe(true)
    expect(toolEl.classes()).toContain('flat-tool')
    expect(toolEl.classes()).not.toContain('flat-thinking')

    // The complete command is present in the tool input, not just a summary.
    const inputEl = toolEl.find('[data-test="tool-input"]')
    expect(inputEl.exists()).toBe(true)
    expect(inputEl.text()).toContain(longCommand)
  })
})

