// Presentation tests for the message log: agent replies render as sanitized
// markdown, while user turns, reasoning and tool output stay literal text.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import MessageView from '../features/chat/rendering/MessageView.vue'
import { clearDiagramCache, configureEngineLoader, type Engine } from '../plantuml/renderer'
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
    rawEvents: false,
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

describe('MessageView collapsible state', () => {
  function toolCall(id: string, name: string): ChatEntry {
    return {
      id,
      kind: 'tool_call',
      role: 'assistant',
      text: name,
      name,
      callId: `call-${id}`,
      input: { path: `/${id}` },
      streaming: false,
    }
  }

  // jsdom toggles a <details> on a summary click but, unlike a browser, fires
  // no `toggle` event, so the state is applied the way the view observes it:
  // flip `open`, then dispatch the event the view listens for.
  function openByHand(details: Element) {
    const el = details as HTMLDetailsElement
    el.open = true
    el.dispatchEvent(new Event('toggle'))
  }

  // A step group is re-created whenever its key changes; that reset the
  // user's expansion on the next streamed tool call. The key is now stable, so
  // the element is patched in place and stays open.
  it('keeps a hand-opened step group open when the turn appends more activity', async () => {
    const wrapper = mount(MessageView, {
      props: { messages: [toolCall('c1', 'read')], capabilities: capabilities({ toolCallEvents: true }) },
    })
    const before = wrapper.find('[data-test="step-group"]')
    openByHand(before.element)

    await wrapper.setProps({ messages: [toolCall('c1', 'read'), toolCall('c2', 'grep')] })

    const after = wrapper.find('[data-test="step-group"]')
    expect(after.element).toBe(before.element)
    expect((after.element as HTMLDetailsElement).open).toBe(true)
  })

  it('keeps a hand-expanded thinking block open when the turn appends more activity', async () => {
    const thinking: ChatEntry = {
      id: 't1',
      kind: 'thinking',
      role: 'assistant',
      text: 'plan',
      streaming: false,
    }
    const wrapper = mount(MessageView, {
      props: {
        messages: [thinking, toolCall('c1', 'read')],
        capabilities: capabilities({ toolCallEvents: true }),
      },
    })
    const before = wrapper.find('[data-test="thinking"]')
    openByHand(before.element)

    await wrapper.setProps({
      messages: [thinking, toolCall('c1', 'read'), toolCall('c2', 'grep')],
    })

    const after = wrapper.find('[data-test="thinking"]')
    expect(after.element).toBe(before.element)
    expect((after.element as HTMLDetailsElement).open).toBe(true)
  })

  it('keeps a hand-expanded tool block open when a later block joins the group', async () => {
    const wrapper = mount(MessageView, {
      props: { messages: [toolCall('c1', 'read')], capabilities: capabilities({ toolCallEvents: true }) },
    })
    const before = wrapper.find('[data-test="tool-call"]')
    openByHand(before.element)

    await wrapper.setProps({ messages: [toolCall('c1', 'read'), toolCall('c2', 'grep')] })

    const after = wrapper.find('[data-test="tool-call"]')
    expect(after.element).toBe(before.element)
    expect((after.element as HTMLDetailsElement).open).toBe(true)
  })
})

describe('MessageView per-message model', () => {
  it('labels an assistant reply with the model that produced it', () => {
    const entry: ChatEntry = {
      ...message('assistant', '回复'),
      model: { id: 'acme/acme-code', provider: 'acme', variant: 'default' },
    }
    const wrapper = mount(MessageView, {
      props: { messages: [entry], capabilities: capabilities() },
    })

    const tag = wrapper.find('[data-test="message-model"]')
    expect(tag.exists()).toBe(true)
    expect(tag.text()).toBe('acme/acme-code#default')
  })

  it('omits the variant suffix when the model ran without one', () => {
    const entry: ChatEntry = {
      ...message('assistant', '回复'),
      model: { id: 'acme/acme-code', provider: 'acme' },
    }
    const wrapper = mount(MessageView, {
      props: { messages: [entry], capabilities: capabilities() },
    })

    expect(wrapper.find('[data-test="message-model"]').text()).toBe('acme/acme-code')
  })

  it('renders no model tag when the message carries none', () => {
    const wrapper = mount(MessageView, {
      props: { messages: [message('assistant', '回复')], capabilities: capabilities() },
    })

    expect(wrapper.find('[data-test="message-model"]').exists()).toBe(false)
  })

  it('never labels a user turn even if it somehow carries a model', () => {
    const entry: ChatEntry = {
      ...message('user', '你好'),
      model: { id: 'acme/acme-code', provider: 'acme' },
    }
    const wrapper = mount(MessageView, {
      props: { messages: [entry], capabilities: capabilities() },
    })

    expect(wrapper.find('[data-test="message-model"]').exists()).toBe(false)
  })

  it('renders timestamp for assistant and user messages', () => {
    const now = new Date('2026-10-18T14:30:00Z').toISOString()
    const assistantEntry: ChatEntry = {
      ...message('assistant', '助手回答'),
      model: { id: 'acme/acme-code', provider: 'acme' },
      timestamp: now,
    }
    const userEntry: ChatEntry = {
      ...message('user', '用户提问'),
      timestamp: now,
    }
    const wrapper = mount(MessageView, {
      props: { messages: [userEntry, assistantEntry], capabilities: capabilities() },
    })

    expect(wrapper.find('[data-test="user-message-time"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="message-time"]').exists()).toBe(true)
  })
})

describe('MessageView copy button', () => {
  const originalClipboard = navigator.clipboard

  afterEach(() => {
    Object.defineProperty(navigator, 'clipboard', {
      value: originalClipboard,
      writable: true,
      configurable: true,
    })
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('renders a copy button for assistant replies but not user messages', () => {
    const assistant = message('assistant', '这是助手消息')
    const user = message('user', '这是用户消息')
    const wrapper = mount(MessageView, {
      props: { messages: [user, assistant], capabilities: capabilities() },
    })

    const buttons = wrapper.findAll('[data-test="message-copy-btn"]')
    expect(buttons).toHaveLength(1)
    expect(buttons[0].attributes('title')).toBe('复制本条消息')
    expect(buttons[0].find('.copy-icon').exists()).toBe(true)

    // Confirm it resides inside the assistant message-meta
    const meta = wrapper.find('[data-test="message-meta"]')
    expect(meta.find('[data-test="message-copy-btn"]').exists()).toBe(true)
  })

  it('copies message text to clipboard and toggles status to 已复制', async () => {
    vi.useFakeTimers()
    const writeTextMock = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: writeTextMock },
      writable: true,
      configurable: true,
    })

    const assistant = message('assistant', '复制这行文字')
    const wrapper = mount(MessageView, {
      props: { messages: [assistant], capabilities: capabilities() },
    })

    const btn = wrapper.get('[data-test="message-copy-btn"]')
    expect(btn.attributes('title')).toBe('复制本条消息')
    expect(btn.classes()).not.toContain('is-copied')

    await btn.trigger('click')
    await flushPromises()

    expect(writeTextMock).toHaveBeenCalledWith('复制这行文字')
    expect(btn.attributes('title')).toBe('已复制')
    expect(btn.classes()).toContain('is-copied')

    vi.advanceTimersByTime(1500)
    await wrapper.vm.$nextTick()

    expect(btn.attributes('title')).toBe('复制本条消息')
    expect(btn.classes()).not.toContain('is-copied')
  })

  it('places the copy button after model and timestamp when present', () => {
    const entry: ChatEntry = {
      ...message('assistant', '带元数据的消息'),
      model: { id: 'test-provider/test-model', provider: 'test-provider' },
      timestamp: new Date('2026-10-18T14:30:00Z').toISOString(),
    }
    const wrapper = mount(MessageView, {
      props: { messages: [entry], capabilities: capabilities() },
    })

    const meta = wrapper.get('[data-test="message-meta"]')
    expect(meta.find('[data-test="message-model"]').exists()).toBe(true)
    expect(meta.find('[data-test="message-time"]').exists()).toBe(true)
    expect(meta.find('[data-test="message-copy-btn"]').exists()).toBe(true)

    const children = meta.element.children
    expect(children[0].getAttribute('data-test')).toBe('message-model')
    expect(children[1].getAttribute('data-test')).toBe('message-time')
    expect(children[2].getAttribute('data-test')).toBe('message-copy-btn')
  })
})

describe('MessageView plantuml blocks', () => {
  function fakeEngine(svg: string): Engine {
    return (_lines, _opts, onSuccess) => onSuccess(svg)
  }

  beforeEach(() => {
    setActivePinia(createPinia())
    clearDiagramCache()
    configureEngineLoader(null)
  })

  it('renders a plantuml fence as a diagram block, not as a code block', async () => {
    configureEngineLoader(fakeEngine('<svg><text>seq</text></svg>'))
    const wrapper = mount(MessageView, {
      props: {
        messages: [message('assistant', '看图\n\n```plantuml\n@startuml\nA->B\n@enduml\n```\n\n完毕')],
        capabilities: capabilities(),
      },
    })
    await flushPromises()

    const block = wrapper.find('[data-test="plantuml-block"]')
    expect(block.exists()).toBe(true)
    expect(block.find('[data-test="plantuml-canvas"] svg').exists()).toBe(true)
    expect(wrapper.find('.bubble-agent .message-text').text()).toContain('看图')
    expect(wrapper.find('.bubble-agent .message-text').text()).toContain('完毕')
    expect(wrapper.find('.bubble-agent').html()).not.toContain('<code>@startuml')
  })

  it('keeps an unterminated fence waiting while the reply streams', () => {
    const entry: ChatEntry = {
      ...message('assistant', '```plantuml\n@startuml\nA->B'),
      streaming: true,
    }
    const wrapper = mount(MessageView, { props: { messages: [entry], capabilities: capabilities() } })

    expect(wrapper.find('[data-test="plantuml-waiting"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="plantuml-canvas"]').exists()).toBe(false)
  })

  it('leaves non-plantuml code blocks alone', () => {
    const wrapper = mount(MessageView, {
      props: { messages: [message('assistant', '```go\nfmt.Println("hi")\n```')], capabilities: capabilities() },
    })

    expect(wrapper.find('[data-test="plantuml-block"]').exists()).toBe(false)
    expect(wrapper.find('.bubble-agent code').text()).toContain('fmt.Println')
  })

  it('draws a closed fence whose event stream died mid-reply', async () => {
    // An EventSource that closes without a turn-ending event leaves the entry
    // flagged as streaming forever; the text is all there is and all there will be.
    configureEngineLoader((_lines, _opts, onSuccess) => onSuccess('<svg><text>x</text></svg>'))
    const entry: ChatEntry = {
      ...message('assistant', '```plantuml\n@startuml\nA->B\n@enduml\n```'),
      streaming: true,
    }
    const wrapper = mount(MessageView, {
      props: { messages: [entry], capabilities: capabilities(), streamStatus: 'error' },
    })
    await flushPromises()
    expect(wrapper.find('[data-test="plantuml-canvas"] svg').exists()).toBe(true)
  })

  it('draws every diagram in one reply, in order', async () => {
    let calls = 0
    configureEngineLoader((_lines, _opts, onSuccess) => {
      calls++
      onSuccess('<svg><text>d</text></svg>')
    })
    const wrapper = mount(MessageView, {
      props: {
        messages: [
          message('assistant', '```plantuml\n@startuml\nA->B\n@enduml\n```\n\n中\n\n```puml\n@startuml\nC->D\n@enduml\n```'),
        ],
        capabilities: capabilities(),
      },
    })
    await flushPromises()

    const blocks = wrapper.findAll('[data-test="plantuml-block"]')
    expect(blocks).toHaveLength(2)
    expect(blocks.map((block) => block.find('[data-test="plantuml-canvas"] svg').exists())).toEqual([true, true])
    expect(calls).toBe(2)
    expect(wrapper.text()).toContain('中')
  })

  it('shows the source of a diagram the reply never finished', async () => {
    const wrapper = mount(MessageView, {
      props: {
        messages: [message('assistant', '```plantuml\n@startuml\nA->B\n')],
        capabilities: capabilities(),
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-test="plantuml-source"]').text()).toContain('A->B')
    expect(wrapper.find('[data-test="plantuml-waiting"]').text()).toContain('没画完')
  })
})
