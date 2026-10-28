import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ensureChatBusConnection,
  getChatBusId,
  setChatBusId,
  resetChatBusId,
  subscribeChatOnBus,
  unsubscribeChatFromBus,
  closeChatBus,
  getSubscribedChatIds,
  hasSubscribedChat,
} from '../features/chat/pure/ChatEventBusClient'
import type { ChatStreamStore } from '../features/chat/pure/useChatStream'
import type { ChatSnapshot } from '../api/chats'
import { installTestCSRF } from './csrf-fixture'

class MockEventSource {
  static instances: MockEventSource[] = []
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2

  url: string
  readyState = MockEventSource.CONNECTING
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  listeners = new Map<string, Array<(event: unknown) => void>>()

  constructor(url: string) {
    this.url = url
    MockEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: (event: unknown) => void) {
    const list = this.listeners.get(type) ?? []
    list.push(listener)
    this.listeners.set(type, list)
  }

  emit(type: string, data: unknown) {
    const list = this.listeners.get(type) ?? []
    for (const listener of list) {
      listener({ data: JSON.stringify(data) })
    }
  }

  open() {
    this.readyState = MockEventSource.OPEN
    this.onopen?.()
    this.emit('ready', {})
  }

  close() {
    this.closed = true
    this.readyState = MockEventSource.CLOSED
  }
}

function makeStore() {
  const calls: Array<{ method: string; args: unknown[] }> = []
  const store: ChatStreamStore = {
    streamStatus: {},
    applySnapshot: (...args) => calls.push({ method: 'snapshot', args }),
    applyDelta: (...args) => calls.push({ method: 'delta', args }),
    applyMessageUpdate: (...args) => calls.push({ method: 'message', args }),
    applyConversation: (...args) => calls.push({ method: 'conversation', args }),
    applyTurn: (...args) => calls.push({ method: 'turn', args }),
    handleClosed: (...args) => calls.push({ method: 'closed', args }),
  }
  return { store, calls }
}

installTestCSRF()

describe('ChatEventBusClient', () => {
  const fetchCalls: Array<{ method: string; url: string; body?: any }> = []

  beforeEach(() => {
    resetChatBusId()
    setChatBusId('test-chat-bus-uuid')
    closeChatBus()
    MockEventSource.instances = []
    vi.stubGlobal('EventSource', MockEventSource)
    fetchCalls.length = 0

    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      fetchCalls.push({
        method: init?.method ?? 'GET',
        url: String(url),
        body: init?.body ? JSON.parse(String(init.body)) : undefined,
      })

      if (url.includes('/subscriptions') && init?.method === 'POST') {
        const body = JSON.parse(String(init.body)) as { chatId: number }
        const snapshot: ChatSnapshot = {
          conversation: {
            id: body.chatId,
            groupId: 1,
            title: 'Chat ' + body.chatId,
            modelId: 1,
            modelName: 'Test',
            running: false,
            createdAt: '2026-01-01T00:00:00Z',
            updatedAt: '2026-01-01T00:00:00Z',
          },
          messages: [],
          running: false,
          truncated: false,
        }
        return {
          ok: true,
          status: 200,
          headers: new Headers({ 'Content-Type': 'application/json' }),
          json: async () => snapshot,
        } as unknown as Response
      }

      if (init?.method === 'DELETE') {
        return {
          ok: true,
          status: 200,
          headers: new Headers({ 'Content-Type': 'application/json' }),
          json: async () => ({ success: true }),
        } as unknown as Response
      }

      return {
        ok: true,
        status: 200,
        headers: new Headers({ 'Content-Type': 'application/json' }),
        json: async () => ({ subscriptions: [] }),
      } as unknown as Response
    })
  })

  afterEach(() => {
    closeChatBus()
    vi.unstubAllGlobals()
  })

  it('maintains a single EventSource connection for multiple chat subscriptions', async () => {
    const first = makeStore()
    const second = makeStore()

    await subscribeChatOnBus(first.store, 1)
    expect(MockEventSource.instances).toHaveLength(1)
    expect(MockEventSource.instances[0].url).toBe('/api/chat-events/test-chat-bus-uuid')
    expect(fetchCalls[0].url).toBe('/api/chat-events/test-chat-bus-uuid/subscriptions')
    expect(fetchCalls[0].body).toEqual({ chatId: 1 })
    expect(first.calls).toHaveLength(1)
    expect(first.calls[0].method).toBe('snapshot')

    // Second chat subscription reuses the same EventSource
    await subscribeChatOnBus(second.store, 2)
    expect(MockEventSource.instances).toHaveLength(1)
    expect(fetchCalls[1].url).toBe('/api/chat-events/test-chat-bus-uuid/subscriptions')
    expect(fetchCalls[1].body).toEqual({ chatId: 2 })
    expect(second.calls).toHaveLength(1)
    expect(second.calls[0].method).toBe('snapshot')

    expect(getSubscribedChatIds()).toEqual([1, 2])
    expect(hasSubscribedChat(1)).toBe(true)
    expect(hasSubscribedChat(2)).toBe(true)
  })

  it('routes incoming delta and turn events to matching chat subscriptions', async () => {
    const first = makeStore()
    const second = makeStore()

    await subscribeChatOnBus(first.store, 1)
    await subscribeChatOnBus(second.store, 2)

    const source = MockEventSource.instances[0]
    source.open()

    // Dispatch event for chat 1
    source.emit('delta', {
      type: 'delta',
      chatId: 1,
      timestamp: '2026-01-01T00:00:00Z',
      delta: { messageId: 10, text: 'hello 1' },
    })

    // Dispatch event for chat 2
    source.emit('turn', {
      type: 'turn',
      chatId: 2,
      timestamp: '2026-01-01T00:00:00Z',
      turn: { phase: 'ended', outcome: 'completed' },
    })

    // First store received delta for 1, but not turn for 2
    expect(first.calls.map((c) => c.method)).toEqual(['snapshot', 'delta'])
    expect(first.calls[1].args).toEqual([1, { messageId: 10, text: 'hello 1' }])

    // Second store received turn for 2, but not delta for 1
    expect(second.calls.map((c) => c.method)).toEqual(['snapshot', 'turn'])
    expect(second.calls[1].args).toEqual([2, { phase: 'ended', outcome: 'completed' }])
  })

  it('unsubscribes a single chat and only closes connection when all chats are removed', async () => {
    const first = makeStore()
    const second = makeStore()

    await subscribeChatOnBus(first.store, 1)
    await subscribeChatOnBus(second.store, 2)

    const source = MockEventSource.instances[0]
    expect(source.closed).toBe(false)

    // Unsubscribe chat 1
    await unsubscribeChatFromBus(first.store, 1)
    expect(fetchCalls.some((c) => c.method === 'DELETE' && c.url.includes('/subscriptions/1'))).toBe(true)
    expect(source.closed).toBe(false)
    expect(hasSubscribedChat(1)).toBe(false)
    expect(hasSubscribedChat(2)).toBe(true)

    // Unsubscribe chat 2
    await unsubscribeChatFromBus(second.store, 2)
    expect(fetchCalls.some((c) => c.method === 'DELETE' && c.url.includes('/subscriptions/2'))).toBe(true)
    expect(hasSubscribedChat(2)).toBe(false)

    // Closing bus closes the connection
    closeChatBus()
    expect(source.closed).toBe(true)
  })
})
