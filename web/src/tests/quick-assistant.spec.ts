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

function stubFetch(options: { existing?: boolean } = {}) {
  const calls: Array<{ url: string; init?: RequestInit }> = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push({ url, init })
      if (url === '/api/chat-groups') {
        return { status: 200, json: async () => ({ groups: [GROUP] }) } as Response
      }
      if (url === '/api/chats?limit=1&groupId=7') {
        return {
          status: 200,
          json: async () => ({ chats: options.existing ? [chat()] : [], total: options.existing ? 1 : 0 }),
        } as Response
      }
      if (url === '/api/chats/1' && (init?.method ?? 'GET') === 'GET') {
        return { status: 200, json: async () => chat() } as Response
      }
      if (url === '/api/chats' && init?.method === 'POST') {
        return { status: 201, json: async () => chat() } as Response
      }
      if (url === '/api/chats/1/messages' && init?.method === 'POST') {
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
  localStorage.clear()
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
  installTestCSRF()
})

afterEach(() => {
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

  it('loads the latest reserved-group conversation and renders it', async () => {
    stubFetch({ existing: true })
    const { wrapper } = await mountLauncher()

    await wrapper.find('[data-test="quick-assistant-button"]').trigger('click')
    await flushPromises()

    expect(FakeEventSource.instances[0]?.url).toBe('/api/chats/1/events')
    FakeEventSource.instances[0].dispatch('snapshot', {
      type: 'snapshot',
      chatId: 1,
      timestamp: '',
      snapshot: snapshot(),
    })
    await flushPromises()

    expect(wrapper.text()).toContain('你好，我在。')
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

    const style = wrapper.find('[data-test="quick-assistant-button"]').attributes('style')
    expect(style).toContain('left: 400px')
    expect(style).toContain('top: 120px')
  })
})
