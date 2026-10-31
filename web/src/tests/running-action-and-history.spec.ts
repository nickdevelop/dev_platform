import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { deriveRunningAction } from '../features/chat/rendering/logModel'
import MessageView from '../features/chat/rendering/MessageView.vue'
import type { ChatEntry } from '../features/chat/state/entry'

function createEntry(overrides: Partial<ChatEntry>): ChatEntry {
  return {
    id: 'entry-1',
    kind: 'message',
    role: 'user',
    text: '',
    streaming: false,
    timestamp: '2026-10-10T12:00:00Z',
    ...overrides,
  }
}

describe('deriveRunningAction', () => {
  it('returns default thinking state when no entries or only user message exists', () => {
    expect(deriveRunningAction([])).toEqual({ label: '正在思考...', kind: 'thinking' })

    const userOnly = [createEntry({ kind: 'message', role: 'user', text: '你好' })]
    expect(deriveRunningAction(userOnly)).toEqual({ label: '正在思考...', kind: 'thinking' })
  })

  it('detects reasoning thinking state', () => {
    const entries = [
      createEntry({ kind: 'message', role: 'user', text: '请分析代码' }),
      createEntry({ kind: 'thinking', text: '正在深入思考算法逻辑...' }),
    ]
    expect(deriveRunningAction(entries)).toEqual({ label: '正在思考...', kind: 'thinking' })
  })

  it('detects bash and shell execution commands', () => {
    const entries = [
      createEntry({ kind: 'message', role: 'user', text: '执行测试' }),
      createEntry({
        kind: 'tool_call',
        name: 'bash',
        input: { command: 'npm test -- --coverage' },
      }),
    ]
    const action = deriveRunningAction(entries)
    expect(action.label).toBe('执行')
    expect(action.kind).toBe('bash')
    expect(action.code).toBe('npm test -- --coverage')
  })

  it('detects file edit tool', () => {
    const entries = [
      createEntry({ kind: 'message', role: 'user', text: '修复 bug' }),
      createEntry({
        kind: 'tool_call',
        name: 'edit',
        input: { path: 'src/features/chat/MessageView.vue' },
      }),
    ]
    const action = deriveRunningAction(entries)
    expect(action.label).toBe('编辑')
    expect(action.kind).toBe('edit')
    expect(action.code).toBe('src/features/chat/MessageView.vue')
  })

  it('detects file read tool', () => {
    const entries = [
      createEntry({ kind: 'message', role: 'user', text: '查看文件' }),
      createEntry({
        kind: 'tool_call',
        name: 'read',
        input: { path: 'package.json' },
      }),
    ]
    const action = deriveRunningAction(entries)
    expect(action.label).toBe('读取')
    expect(action.kind).toBe('read')
    expect(action.code).toBe('package.json')
  })

  it('detects search tool', () => {
    const entries = [
      createEntry({ kind: 'message', role: 'user', text: '全局搜索' }),
      createEntry({
        kind: 'tool_call',
        name: 'grep',
        input: { pattern: 'deriveRunningAction' },
      }),
    ]
    const action = deriveRunningAction(entries)
    expect(action.label).toBe('搜索')
    expect(action.kind).toBe('search')
    expect(action.code).toBe('deriveRunningAction')
  })

  it('detects subagent task tool', () => {
    const entries = [
      createEntry({ kind: 'message', role: 'user', text: '委派任务' }),
      createEntry({
        kind: 'tool_call',
        name: 'subagent',
        input: { description: '探索项目文档' },
      }),
    ]
    const action = deriveRunningAction(entries)
    expect(action.label).toBe('执行子任务')
    expect(action.kind).toBe('task')
    expect(action.code).toBe('探索项目文档')
  })

  it('detects generic tool calls', () => {
    const entries = [
      createEntry({ kind: 'message', role: 'user', text: '抓取网页' }),
      createEntry({
        kind: 'tool_call',
        name: 'webfetch',
        input: { url: 'https://example.com' },
      }),
    ]
    const action = deriveRunningAction(entries)
    expect(action.label).toBe('调用 webfetch')
    expect(action.kind).toBe('tool')
    expect(action.code).toBe('https://example.com')
  })

  it('transitions back to thinking when a tool call has completed but turn is running', () => {
    const entries = [
      createEntry({ kind: 'message', role: 'user', text: '执行测试' }),
      createEntry({
        kind: 'tool_call',
        callId: 'c1',
        name: 'bash',
        input: { command: 'npm test' },
      }),
      createEntry({
        kind: 'tool_result',
        callId: 'c1',
        text: 'PASS',
      }),
    ]
    const action = deriveRunningAction(entries)
    expect(action).toEqual({ label: '正在思考...', kind: 'thinking' })
  })
})

describe('MessageView dynamic running indicator rendering', () => {
  it('renders dynamic thinking action when running with reasoning', () => {
    const messages = [
      createEntry({ kind: 'message', role: 'user', text: '你好' }),
      createEntry({ kind: 'thinking', text: '思考中...' }),
    ]
    const wrapper = mount(MessageView, {
      props: {
        messages,
        running: true,
        display: 'messages',
      },
    })

    const runningEntry = wrapper.find('[data-test="running-entry"]')
    expect(runningEntry.exists()).toBe(true)
    expect(runningEntry.text()).toContain('正在思考...')
    expect(runningEntry.find('[data-test="running-code"]').exists()).toBe(false)
  })

  it('renders dynamic bash execution action with command code block', () => {
    const messages = [
      createEntry({ kind: 'message', role: 'user', text: '执行构建' }),
      createEntry({
        kind: 'tool_call',
        name: 'bash',
        input: { command: 'go build ./...' },
      }),
    ]
    const wrapper = mount(MessageView, {
      props: {
        messages,
        running: true,
        display: 'messages',
      },
    })

    const runningEntry = wrapper.find('[data-test="running-entry"]')
    expect(runningEntry.exists()).toBe(true)
    const label = wrapper.find('[data-test="running-label"]')
    expect(label.text()).toBe('执行')
    const code = wrapper.find('[data-test="running-code"]')
    expect(code.exists()).toBe(true)
    expect(code.text()).toBe('go build ./...')
  })

  it('does not render running indicator when running is false', () => {
    const messages = [
      createEntry({ kind: 'message', role: 'user', text: '你好' }),
      createEntry({ kind: 'message', role: 'assistant', text: '你好！' }),
    ]
    const wrapper = mount(MessageView, {
      props: {
        messages,
        running: false,
        display: 'messages',
      },
    })

    expect(wrapper.find('[data-test="running-entry"]').exists()).toBe(false)
  })
})
