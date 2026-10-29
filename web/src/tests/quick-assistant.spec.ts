// Quick-assistant launcher and popover: opens without a route change, renders
// the latest reserved-group conversation, creates new conversations in that
// group, and keeps the 480x600 footprint.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import QuickAssistantLauncher from '../features/chat/quick/QuickAssistantLauncher.vue'
import { useModelsStore } from '../stores/models'
import { useQuickAssistantStore } from '../stores/quickAssistant'
import { installTestCSRF } from './csrf-fixture'
import type { Chat, ChatMessage, ChatSnapshot } from '../api/chats'
import { closeChatBus, resetChatBusId } from '../features/chat/pure/ChatEventBusClient'

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
    this.onopen?.()
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data: JSON.stringify(data) } as MessageEvent)
    }
  }

  close() {}
}

const GROUP = {
  id: 7,
  name: '快捷助手',
  systemKey: 'quick-assistant',
  createdAt: '2026-10-23T00:00:00Z',
  updatedAt: '2026-10-23T00:00:00Z',
}

function chat(overrides: Partial<Chat> = {}): Chat {
  return {
    id: 1,
    groupId: 7,
    title: '快捷对话',
    modelId: 1,
    modelName: 'Test Model',
    running: false,
    createdAt: '2026-10-23T00:00:00Z',
    updatedAt: '2026-10-23T00:00:00Z',
    ...overrides,
  }
}

function message(overrides: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id: 10,
    chatId: 1,
    role: 'assistant',
    text: '你好，我在。',
    status: 'complete',
    modelId: 1,
    modelName: 'Test Model',
    createdAt: '2026-10-23T00:00:00Z',
    updatedAt: '2026-10-23T00:00:00Z',
    ...overrides,
  }
}

function snapshot(overrides: Partial<ChatSnapshot> = {}): ChatSnapshot {
  return {
    conversation: chat(),
    messages: [message({ id: 9, role: 'user', text: '你好' }), message()],
    running: false,
    truncated: false,
    ...overrides,
  }
}

function setupModels() {
  const models = useModelsStore()
  models.services = [
    {
      id: 1,
      name: '测试服务',
      baseUrl: 'http://example.invalid/v1',
      hasKey: true,
      secretRef: 'sec',
      enabled: true,
      models: [
        {
          id: 1,
          serviceId: 1,
          modelId: 'test-model',
          name: 'Test Model',
          supportsText: true,
          supportsImage: true,
          supportsTools: false,
          contextTokens: 128000,
          outputTokens: 4096,
          enabled: true,
          usable: true,
        },
      ],
    },
  ]
}

function stubFetch(options: { existing?: boolean; chats?: Chat[] } = {}) {
  const defaultChats = options.chats ?? (options.existing ? [chat()] : [])
  const calls: Array<{ url: string; init?: RequestInit }> = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/chat-groups') {
        return { status: 200, json: async () => ({ groups: [GROUP] }) } as Response
      }
      if (url === '/api/chats?limit=5&groupId=7' || url === '/api/chats?limit=1&groupId=7') {
        return {
          status: 200,
          json: async () => ({ chats: defaultChats, total: defaultChats.length }),
        } as Response
      }
      const matchChat = url.match(/^\/api\/chats\/(\d+)$/)
      if (matchChat && (init?.method ?? 'GET') === 'GET') {
        const id = Number(matchChat[1])
        const found = defaultChats.find((c) => c.id === id) ?? chat({ id })
        return { status: 200, json: async () => found } as Response
      }
      if (url === '/api/chats' && init?.method === 'POST') {
        return { status: 201, json: async () => chat() } as Response
      }
      if (url.match(/^\/api\/chats\/\d+\/messages$/) && init?.method === 'POST') {
        return { status: 202, json: async () => chat({ running: true }) } as Response
      }
      return { status: 200, json: async () => ({}) } as Response
    }),
  )
  return calls
}

async function mountLauncher() {
  const pinia = createPinia()
  setActivePinia(pinia)
  setupModels()
  const wrapper = mount(QuickAssistantLauncher, { global: { plugins: [pinia] } })
  await flushPromises()
  return { wrapper, quick: useQuickAssistantStore() }
}

beforeEach(() => {
  resetChatBusId()
  closeChatBus()
  localStorage.clear()
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
  installTestCSRF()
})

afterEach(() => {
  closeChatBus()
  vi.unstubAllGlobals()
})

describe('quick assistant launcher', () => {
  it('starts closed and opens a 480x600 popover with a welcome state', async () => {
    stubFetch()
    const { wrapper } = await mountLauncher()

    expect(wrapper.find('[data-test="quick-assistant-panel"]').exists()).toBe(false)
    const button = wrapper.find('[data-test="quick-assistant-button"]')
    expect(button.attributes('aria-expanded')).toBe('false')
    expect(button.text()).toBe('快')

    await button.trigger('click')
    await flushPromises()

    expect(button.attributes('aria-expanded')).toBe('true')
    const popover = wrapper.find('[data-test="quick-assistant-popover"]')
    expect(popover.exists()).toBe(true)
    expect(popover.attributes('style')).toContain('width: 480px')
    expect(popover.attributes('style')).toContain('height: 600px')
    expect(wrapper.text()).toContain('你好，我是快捷助手')
  })

  it('opens a fresh draft instead of restoring the latest conversation', async () => {
    const calls = stubFetch({ existing: true })
    const { wrapper, quick } = await mountLauncher()

    await wrapper.find('[data-test="quick-assistant-button"]').trigger('click')
    await flushPromises()

    expect(quick.chatId).toBeNull()
    expect(wrapper.text()).toContain('你好，我是快捷助手')
    expect(calls.some((call) => call.url === '/api/chats/1')).toBe(false)
  })

  it('creates the first message inside the reserved group', async () => {
    const calls = stubFetch()
    const { wrapper } = await mountLauncher()

    await wrapper.find('[data-test="quick-assistant-button"]').trigger('click')
    await flushPromises()

    await wrapper.find('[data-test="chat-input"]').setValue('帮我写一个函数')
    await wrapper.find('[data-test="chat-send-btn"]').trigger('click')
    await flushPromises()

    const create = calls.find((call) => call.url === '/api/chats' && call.init?.method === 'POST')
    expect(create).toBeTruthy()
    expect(JSON.parse(String(create?.init?.body))).toMatchObject({
      groupId: 7,
      modelId: 1,
      text: '帮我写一个函数',
    })
  })

  it('closes with the close button and Escape', async () => {
    stubFetch()
    const { wrapper } = await mountLauncher()

    await wrapper.find('[data-test="quick-assistant-button"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-test="quick-assistant-close"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="quick-assistant-panel"]').exists()).toBe(false)

    await wrapper.find('[data-test="quick-assistant-button"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-test="quick-assistant-panel"]').trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(wrapper.find('[data-test="quick-assistant-panel"]').exists()).toBe(false)
  })

  it('restores a persisted position and clamps it into the viewport', async () => {
    localStorage.setItem(
      'dev-platform-quick-assistant-position',
      JSON.stringify({ left: 400, top: 120 }),
    )
    stubFetch()
    const { wrapper } = await mountLauncher()

    const style = wrapper.find('[data-test="quick-assistant-toolbar"]').attributes('style')
    expect(style).toContain('left: 400px')
    expect(style).toContain('top: 120px')
  })

  it('does not render a dedicated drag handle and enables direct button interaction', async () => {
    stubFetch()
    const { wrapper } = await mountLauncher()

    expect(wrapper.find('.toolbar-drag-handle').exists()).toBe(false)
    const toolbar = wrapper.find('[data-test="quick-assistant-toolbar"]')
    expect(toolbar.exists()).toBe(true)

    const button = wrapper.find('[data-test="quick-assistant-button"]')
    expect(button.exists()).toBe(true)

    // Regular click opens popover
    await button.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="quick-assistant-popover"]').exists()).toBe(true)
  })

  it('suppresses click toggling when user drags the toolbar', async () => {
    stubFetch()
    const { wrapper } = await mountLauncher()
    const toolbar = wrapper.find('[data-test="quick-assistant-toolbar"]')
    const button = wrapper.find('[data-test="quick-assistant-button"]')

    // Simulate drag start and move by > 4px via standard PointerEvents
    const el = toolbar.element
    el.dispatchEvent(new PointerEvent('pointerdown', { pointerId: 1, clientX: 100, clientY: 100, button: 0 }))
    el.dispatchEvent(new PointerEvent('pointermove', { pointerId: 1, clientX: 120, clientY: 120 }))
    el.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, clientX: 120, clientY: 120 }))

    // Trailing click is suppressed
    await button.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="quick-assistant-popover"]').exists()).toBe(false)
  })

  it('toggles open/close when clicking the same active assistant in multi-assistant mode', async () => {
    const multiGroups = [
      { id: 1, name: '助手一', showInFloat: true, createdAt: '', updatedAt: '' },
      { id: 2, name: '助手二', showInFloat: true, createdAt: '', updatedAt: '' },
    ]
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url === '/api/chat-groups') {
          return { status: 200, json: async () => ({ groups: multiGroups }) } as Response
        }
        if (url === '/api/models/services') {
          return { status: 200, json: async () => ({ services: [] }) } as Response
        }
        return { status: 200, json: async () => ({ chats: [], total: 0 }) } as Response
      }),
    )

    const { wrapper, quick } = await mountLauncher()
    const buttons = wrapper.findAll('.quick-assistant-button')
    expect(buttons).toHaveLength(2)

    // Click assistant 1 to open
    await buttons[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="quick-assistant-popover"]').exists()).toBe(true)
    expect(quick.currentAssistant?.id).toBe(1)

    // Click assistant 1 again toggles it closed
    await buttons[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="quick-assistant-popover"]').exists()).toBe(false)

    // Click assistant 2 opens it
    await buttons[1].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="quick-assistant-popover"]').exists()).toBe(true)
    expect(quick.currentAssistant?.id).toBe(2)
  })

  it('renders recent 5 conversations in the history dropdown without opening one', async () => {
    const historyChats = [
      chat({ id: 101, title: '对话 1' }),
      chat({ id: 102, title: '对话 2' }),
      chat({ id: 103, title: '对话 3' }),
      chat({ id: 104, title: '对话 4' }),
      chat({ id: 105, title: '对话 5' }),
    ]
    stubFetch({ chats: historyChats })
    const { wrapper, quick } = await mountLauncher()

    await wrapper.find('[data-test="quick-assistant-button"]').trigger('click')
    await flushPromises()

    const select = wrapper.find('[data-test="quick-assistant-history-select"]')
    expect(select.exists()).toBe(true)

    const options = select.findAll('option')
    // 1 placeholder ("最近对话…") + 5 conversations
    expect(options).toHaveLength(6)
    expect(options[0].text()).toContain('最近对话')
    expect(options[1].text()).toBe('对话 1')
    expect(options[2].text()).toBe('对话 2')
    expect(options[5].text()).toBe('对话 5')

    // The popover starts on a blank draft; history is opt-in.
    expect(quick.chatId).toBeNull()
    expect((select.element as HTMLSelectElement).value).toBe('')
  })

  it('switches conversation when selecting an item from the history dropdown', async () => {
    const historyChats = [
      chat({ id: 201, title: '对话 A' }),
      chat({ id: 202, title: '对话 B' }),
    ]
    stubFetch({ chats: historyChats })
    const { wrapper, quick } = await mountLauncher()

    await wrapper.find('[data-test="quick-assistant-button"]').trigger('click')
    await flushPromises()

    // Opening the popover no longer selects the newest conversation.
    expect(quick.chatId).toBeNull()

    const select = wrapper.find('[data-test="quick-assistant-history-select"]')
    await select.setValue('202')
    await flushPromises()

    expect(quick.chatId).toBe(202)
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(FakeEventSource.instances[0]?.url).toContain('/api/chat-events/')

    FakeEventSource.instances[0].dispatch('snapshot', {
      type: 'snapshot',
      chatId: 202,
      timestamp: '',
      snapshot: snapshot({ conversation: chat({ id: 202, title: '对话 B' }) }),
    })
    await flushPromises()
    expect(wrapper.text()).toContain('你好，我在。')
  })

  it('starts a new draft every time the popover is opened', async () => {
    const historyChats = [chat({ id: 301, title: '历史对话' })]
    const calls = stubFetch({ chats: historyChats })
    const { wrapper, quick } = await mountLauncher()

    // 1. Opening goes straight to a blank draft even though history exists.
    await wrapper.find('[data-test="quick-assistant-button"]').trigger('click')
    await flushPromises()
    expect(quick.chatId).toBeNull()
    expect(wrapper.text()).toContain('你好，我是快捷助手')

    // 2. The user deliberately picks the historical conversation.
    const select = wrapper.find('[data-test="quick-assistant-history-select"]')
    await select.setValue('301')
    await flushPromises()
    expect(quick.chatId).toBe(301)

    // 3. Closing and reopening must NOT restore that conversation.
    await wrapper.find('[data-test="quick-assistant-close"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="quick-assistant-panel"]').exists()).toBe(false)

    await wrapper.find('[data-test="quick-assistant-button"]').trigger('click')
    await flushPromises()

    expect(quick.chatId).toBeNull()
    expect(wrapper.text()).toContain('你好，我是快捷助手')
    // Only the explicit dropdown selection fetched the conversation.
    expect(calls.filter((call) => call.url === '/api/chats/301')).toHaveLength(1)
  })

  it('does not restore the conversation after switching assistants', async () => {
    const multiGroups = [
      { id: 1, name: '助手一', showInFloat: true, createdAt: '', updatedAt: '' },
      { id: 2, name: '助手二', showInFloat: true, createdAt: '', updatedAt: '' },
    ]
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url === '/api/chat-groups') {
          return { status: 200, json: async () => ({ groups: multiGroups }) } as Response
        }
        if (url === '/api/chats?limit=5&groupId=1') {
          return {
            status: 200,
            json: async () => ({ chats: [chat({ id: 501, groupId: 1 })], total: 1 }),
          } as Response
        }
        if (url === '/api/chats?limit=5&groupId=2') {
          return { status: 200, json: async () => ({ chats: [], total: 0 }) } as Response
        }
        if (url === '/api/chats/501') {
          return { status: 200, json: async () => chat({ id: 501, groupId: 1 }) } as Response
        }
        return { status: 200, json: async () => ({}) } as Response
      }),
    )

    const { wrapper, quick } = await mountLauncher()
    const buttons = wrapper.findAll('.quick-assistant-button')

    await buttons[0].trigger('click')
    await flushPromises()
    const select = wrapper.find('[data-test="quick-assistant-history-select"]')
    await select.setValue('501')
    await flushPromises()
    expect(quick.chatId).toBe(501)

    // Switching assistants remounts the panel and starts a new draft.
    await buttons[1].trigger('click')
    await flushPromises()
    expect(quick.currentAssistant?.id).toBe(2)
    expect(quick.chatId).toBeNull()
    expect(wrapper.text()).toContain('你好，我是助手二')
  })

  it('starts a new draft after sending the first message and reopening', async () => {
    stubFetch()
    const { wrapper, quick } = await mountLauncher()

    // 1. Open launcher: it is already a new draft.
    await wrapper.find('[data-test="quick-assistant-button"]').trigger('click')
    await flushPromises()
    expect(quick.chatId).toBeNull()

    // 2. Send message -> creates chat 1
    await wrapper.find('[data-test="chat-input"]').setValue('Hello new chat')
    await wrapper.find('[data-test="chat-send-btn"]').trigger('click')
    await flushPromises()
    expect(quick.chatId).toBe(1)

    // 3. Close and reopen -> a fresh draft, not chat 1
    await wrapper.find('[data-test="quick-assistant-close"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-test="quick-assistant-button"]').trigger('click')
    await flushPromises()
    expect(quick.chatId).toBeNull()
    expect(wrapper.text()).toContain('你好，我是快捷助手')
  })
})
