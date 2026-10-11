// Presentation tests for the message log: agent replies render as sanitized
// markdown, while user turns, reasoning and tool output stay literal text.
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import MessageView from '../components/MessageView.vue'
import type { Capabilities } from '../api/sessions'
import type { ChatEntry } from '../stores/sessions'

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
    ...overrides,
  }
}

function message(role: string, text: string): ChatEntry {
  return { id: `${role}-1`, kind: 'message', role, text, streaming: false }
}

describe('MessageView markdown', () => {
  it('renders an assistant reply as markdown', () => {
    const wrapper = mount(MessageView, {
      props: { messages: [message('assistant', '**粗体** 与 `代码`')], capabilities: capabilities() },
    })

    const html = wrapper.find('.bubble-agent .message-text').html()
    expect(html).toContain('<strong>粗体</strong>')
    expect(html).toContain('<code>代码</code>')
  })

  it('keeps a user turn as literal text', () => {
    const wrapper = mount(MessageView, {
      props: { messages: [message('user', '**粗体**')], capabilities: capabilities() },
    })

    const html = wrapper.find('.bubble-user .message-text').html()
    expect(html).not.toContain('<strong>')
    expect(wrapper.find('.bubble-user .message-text').text()).toBe('**粗体**')
  })

  it('strips markup injected through an assistant reply', () => {
    const wrapper = mount(MessageView, {
      props: { messages: [message('assistant', '<img src=x onerror="alert(1)">')], capabilities: capabilities() },
    })

    expect(wrapper.find('.bubble-agent .message-text').html()).not.toContain('onerror')
  })

  it('leaves reasoning and tool output as plain text', () => {
    const entries: ChatEntry[] = [
      { id: 't1', kind: 'thinking', role: 'assistant', text: '**推理**', streaming: false },
      { id: 'r1', kind: 'tool_result', role: 'tool', text: '**输出**', callId: 'c1', streaming: false },
    ]
    const wrapper = mount(MessageView, { props: { messages: entries, capabilities: capabilities() } })

    expect(wrapper.find('.thinking-text').text()).toBe('**推理**')
    expect(wrapper.find('.tool-output').text()).toBe('**输出**')
  })
})

describe('MessageView tool and thinking blocks', () => {
  it('merges a tool call and its result into one expandable block', async () => {
    const entries: ChatEntry[] = [
      {
        id: 'c1',
        kind: 'tool_call',
        role: 'assistant',
        text: 'read',
        name: 'read',
        callId: 'call-1',
        input: { path: '/usr/docs/models.md' },
        streaming: false,
      },
      { id: 'r1', kind: 'tool_result', role: 'tool', text: '文件内容', callId: 'call-1', streaming: false },
    ]
    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities({ toolCallEvents: true }) },
    })

    const blocks = wrapper.findAll('[data-test="tool-call"]')
    expect(blocks).toHaveLength(1)
    // The result is not a block of its own.
    expect(wrapper.find('[data-test="tool-result"]').exists()).toBe(false)
    const summary = blocks[0].find('summary')
    expect(summary.text()).toContain('🔧')
    expect(summary.text()).toContain('read')
    expect(summary.text()).toContain('/usr/docs/models.md')
    expect(summary.text()).not.toContain('点击展开')

    await summary.trigger('click')
    // The expanded body shows only the summary argument and the result, never
    // the whole argument payload.
    expect(blocks[0].find('.tool-args').text()).toBe('/usr/docs/models.md')
    expect(blocks[0].find('.tool-output').text()).toContain('文件内容')
  })

  it('does not expose the other arguments in the expanded body', async () => {
    const entries: ChatEntry[] = [
      {
        id: 'c1',
        kind: 'tool_call',
        role: 'assistant',
        text: 'write',
        name: 'write',
        callId: 'call-1',
        input: { path: '/usr/a.md', content: 'SECRET-BODY' },
        streaming: false,
      },
      { id: 'r1', kind: 'tool_result', role: 'tool', text: 'ok', callId: 'call-1', streaming: false },
    ]
    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities({ toolCallEvents: true }) },
    })

    const block = wrapper.find('[data-test="tool-call"]')
    expect(block.find('.tool-args').text()).toBe('/usr/a.md')
    expect(block.text()).not.toContain('SECRET-BODY')
  })

  it('shows only the last result when one tool call reports several', async () => {
    const entries: ChatEntry[] = [
      {
        id: 'c1',
        kind: 'tool_call',
        role: 'assistant',
        text: 'bash',
        name: 'bash',
        callId: 'call-1',
        input: { command: 'ls' },
        streaming: false,
      },
      { id: 'r1', kind: 'tool_result', role: 'tool', text: '第一次输出', callId: 'call-1', streaming: false },
      { id: 'r2', kind: 'tool_result', role: 'tool', text: '第二次输出', callId: 'call-1', streaming: false },
    ]
    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities({ toolCallEvents: true }) },
    })

    const block = wrapper.find('[data-test="tool-call"]')
    await block.find('summary').trigger('click')
    expect(block.text()).toContain('第二次输出')
    expect(block.text()).not.toContain('第一次输出')
  })

  it('shows reasoning behind a brain icon and Thinking label', async () => {
    const entries: ChatEntry[] = [
      { id: 't1', kind: 'thinking', role: 'assistant', text: '先分析再回答', streaming: false },
    ]
    const wrapper = mount(MessageView, {
      props: { messages: entries, capabilities: capabilities({ toolCallEvents: true }) },
    })

    const thinking = wrapper.find('[data-test="thinking"]')
    expect(thinking.find('summary').text()).toContain('🧠')
    expect(thinking.find('summary').text()).toContain('Thinking')
    await thinking.find('summary').trigger('click')
    expect(thinking.find('.thinking-text').text()).toContain('先分析再回答')
  })
})
