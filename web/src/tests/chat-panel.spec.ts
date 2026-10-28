// Pure-chat panel tests: the shared renderer is used, agent-only controls are
// absent, streaming state drives the stop button, and edit/regenerate/send hit
// their chat routes.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import ChatPanel from '../features/chat/pure/ChatPanel.vue'
import { useChatsStore } from '../stores/chats'
import { useModelsStore } from '../stores/models'
import { installTestCSRF, testCSRFToken } from './csrf-fixture'
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

function chat(overrides: Partial<Chat> = {}): Chat {
  return {
    id: 1,
    title: '测试对话',
    modelId: 1,
    modelName: 'Test Model',
    running: false,
    createdAt: '2026-10-22T00:00:00Z',
    updatedAt: '2026-10-22T00:00:00Z',
    ...overrides,
  }
}

function message(overrides: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id: 10,
    chatId: 1,
    role: 'assistant',
    text: '你好，有什么可以帮你？',
    status: 'complete',
    modelId: 1,
    modelName: 'Test Model',
    createdAt: '2026-10-22T00:00:00Z',
    updatedAt: '2026-10-22T00:00:00Z',
    ...overrides,
  }
}

function snapshot(overrides: Partial<ChatSnapshot> = {}): ChatSnapshot {
  return {
    conversation: chat(),
    messages: [
      message({ id: 9, role: 'user', text: '你好' }),
      message({ id: 10, reasoning: '先打招呼' }),
    ],
    running: false,
    truncated: false,
    ...overrides,
  }
}

function setupModelStore() {
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

function stubFetch() {
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const spy = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    calls.push({ url, init })
    if (url === '/api/chats/1' && init?.method === undefined) {
      return { status: 200, json: async () => chat() } as Response
    }
    if (url === '/api/chats' && init?.method === 'POST') {
      return { status: 201, json: async () => chat() } as Response
    }
    if (url === '/api/chats/1' && init?.method === 'PUT') {
      return { status: 200, json: async () => chat() } as Response
    }
    if (url === '/api/chats/1' && init?.method === 'DELETE') {
      return { status: 204, json: async () => undefined } as Response
    }
    return { status: 200, json: async () => chat() } as Response
  })
  vi.stubGlobal('fetch', spy)
  return { spy, calls }
}

async function mountPanel(chatId?: number) {
  const pinia = createPinia()
  setActivePinia(pinia)
  setupModelStore()
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/chat', name: 'chat', component: { template: '<div />' } },
      { path: '/chat/:chatId', name: 'chat-detail', component: { template: '<div />' } },
      { path: '/settings/models', name: 'settings-models', component: { template: '<div />' } },
    ],
  })
  await router.push(chatId === undefined ? '/chat' : `/chat/${chatId}`)
  await router.isReady()
  const wrapper = mount(ChatPanel, {
    props: chatId === undefined ? {} : { chatId },
    global: {
      plugins: [pinia, router],
      stubs: { RouterLink: { template: '<a><slot /></a>' } },
    },
  })
  await flushPromises()
  return { wrapper, router, chats: useChatsStore() }
}

beforeEach(() => {
  resetChatBusId()
  closeChatBus()
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
  installTestCSRF()
})

afterEach(() => {
  closeChatBus()
  vi.unstubAllGlobals()
})

describe('pure chat panel', () => {
  it('shows the welcome state and omits every agent-only control', async () => {
    stubFetch()
    const { wrapper } = await mountPanel()
    expect(wrapper.text()).toContain('开始新的对话')
    expect(wrapper.find('[data-test="pure-chat"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="chat-input"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="activity-pane"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="workdir-row"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="toggle-filetree-btn"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="chat-side-rail-left"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="chat-side-rail-right"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="approval-mode-select"]').exists()).toBe(false)
  })

  it('renders a snapshot, thinking, and the edit/regenerate actions', async () => {
    stubFetch()
    const { wrapper } = await mountPanel(1)
    FakeEventSource.instances[0].dispatch('snapshot', {
      type: 'snapshot',
      chatId: 1,
      timestamp: '',
      snapshot: snapshot(),
    })
    await flushPromises()

    expect(wrapper.text()).toContain('你好')
    expect(wrapper.text()).toContain('先打招呼')
    expect(wrapper.find('[data-test="message-edit-btn"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="message-regenerate-btn"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="chat-model-trigger"]').text()).toContain('Test Model')
  })

  it('prefills the composer when editing and warns that later messages are removed', async () => {
    stubFetch()
    const { wrapper } = await mountPanel(1)
    FakeEventSource.instances[0].dispatch('snapshot', {
      type: 'snapshot',
      chatId: 1,
      timestamp: '',
      snapshot: snapshot(),
    })
    await flushPromises()

    await wrapper.find('[data-test="message-edit-btn"]').trigger('click')
    expect((wrapper.find('[data-test="chat-input"]').element as HTMLTextAreaElement).value).toBe('你好')
    expect(wrapper.find('[data-test="chat-edit-banner"]').text()).toContain('删除该消息之后的内容')
    await wrapper.find('[data-test="chat-edit-banner"] button').trigger('click')
    expect(wrapper.find('[data-test="chat-edit-banner"]').exists()).toBe(false)
  })

  it('creates a conversation on the first send and navigates to it', async () => {
    const { calls } = stubFetch()
    const { wrapper, router } = await mountPanel()
    await wrapper.find('[data-test="chat-input"]').setValue('新对话')
    await wrapper.find('[data-test="chat-send-btn"]').trigger('click')
    await flushPromises()

    expect(calls.some((call) => call.url === '/api/chats' && call.init?.method === 'POST')).toBe(true)
    expect(router.currentRoute.value.name).toBe('chat-detail')
    const send = calls.find((call) => call.url === '/api/chats' && call.init?.method === 'POST')
    expect((send?.init?.headers as Record<string, string>)['X-CSRF-Token']).toBe(testCSRFToken)
  })

  it('sends, stops and regenerates through the chat routes', async () => {
    const { calls } = stubFetch()
    const { wrapper } = await mountPanel(1)
    FakeEventSource.instances[0].dispatch('snapshot', {
      type: 'snapshot',
      chatId: 1,
      timestamp: '',
      snapshot: snapshot(),
    })
    await flushPromises()

    await wrapper.find('[data-test="chat-input"]').setValue('继续')
    await wrapper.find('[data-test="chat-send-btn"]').trigger('click')
    await flushPromises()
    expect(calls.some((call) => call.url === '/api/chats/1/messages' && call.init?.method === 'POST')).toBe(true)

    FakeEventSource.instances[0].dispatch('snapshot', {
      type: 'snapshot',
      chatId: 1,
      timestamp: '',
      snapshot: snapshot({ running: true }),
    })
    await flushPromises()
    expect(wrapper.find('[data-test="chat-send-btn"]').text()).toBe('停止')
    await wrapper.find('[data-test="chat-send-btn"]').trigger('click')
    await flushPromises()
    expect(calls.some((call) => call.url === '/api/chats/1/stop' && call.init?.method === 'POST')).toBe(true)

    FakeEventSource.instances[0].dispatch('snapshot', {
      type: 'snapshot',
      chatId: 1,
      timestamp: '',
      snapshot: snapshot(),
    })
    await flushPromises()
    await wrapper.find('[data-test="message-regenerate-btn"]').trigger('click')
    await flushPromises()
    expect(calls.some((call) => call.url === '/api/chats/1/regenerate' && call.init?.method === 'POST')).toBe(true)
  })
})
