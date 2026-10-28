// Multiplexed pure-chat stream factory. The main Chat page and the
// quick-assistant popover share the window-level ChatEventBusClient and can
// stream different conversations simultaneously over a single SSE connection.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createChatStream, type ChatStreamStore } from '../features/chat/pure/useChatStream'
import { closeChatBus, resetChatBusId, setChatBusId } from '../features/chat/pure/ChatEventBusClient'
import { installTestCSRF } from './csrf-fixture'

class FakeEventSource {
  static instances: FakeEventSource[] = []
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2

  readonly url: string
  readyState = FakeEventSource.CONNECTING
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false
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

  close() {
    this.closed = true
    this.readyState = FakeEventSource.CLOSED
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

beforeEach(() => {
  resetChatBusId()
  setChatBusId('test-bus-factory')
  closeChatBus()
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)

  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    if (String(url).includes('/subscriptions') && init?.method === 'POST') {
      const body = JSON.parse(String(init.body)) as { chatId: number }
      return {
        ok: true,
        status: 200,
        headers: new Headers({ 'Content-Type': 'application/json' }),
        json: async () => ({
          conversation: { id: body.chatId, groupId: 1, title: 'Chat ' + body.chatId },
          messages: [],
          running: false,
          truncated: false,
        }),
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
      json: async () => ({}),
    } as unknown as Response
  })
})

afterEach(() => {
  closeChatBus()
  vi.unstubAllGlobals()
})

describe('chat stream factory', () => {
  it('shares one EventSource connection across stores on the bus', () => {
    const first = makeStore()
    const second = makeStore()
    const firstStream = createChatStream(first.store)
    const secondStream = createChatStream(second.store)

    firstStream.open(1)
    secondStream.open(2)

    expect(FakeEventSource.instances).toHaveLength(1)
    expect(FakeEventSource.instances[0].url).toBe('/api/chat-events/test-bus-factory')

    firstStream.close()
    // Bus connection remains active because secondStream is still listening
    expect(FakeEventSource.instances[0].closed).toBe(false)

    secondStream.close()
  })

  it('passes the conversation id with delta and turn callbacks', () => {
    const { store, calls } = makeStore()
    const stream = createChatStream(store)
    stream.open(9)
    const source = FakeEventSource.instances[0]

    source.dispatch('delta', {
      type: 'delta',
      chatId: 9,
      timestamp: '',
      delta: { messageId: 10, text: 'hi' },
    })
    source.dispatch('turn', {
      type: 'turn',
      chatId: 9,
      timestamp: '',
      turn: { phase: 'ended', outcome: 'completed' },
    })

    expect(calls).toEqual([
      { method: 'delta', args: [9, { messageId: 10, text: 'hi' }] },
      { method: 'turn', args: [9, { phase: 'ended', outcome: 'completed' }] },
    ])
  })
})
