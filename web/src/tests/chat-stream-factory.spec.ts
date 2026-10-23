// Each pure-chat store owns its own EventSource. The main Chat page and the
// quick-assistant popover must be able to stream different conversations at the
// same time; closing one stream cannot close the other.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createChatStream, type ChatStreamStore } from '../features/chat/pure/useChatStream'

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

beforeEach(() => {
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('chat stream factory', () => {
  it('keeps one EventSource per store and closes only its own stream', () => {
    const first = makeStore()
    const second = makeStore()
    const firstStream = createChatStream(first.store)
    const secondStream = createChatStream(second.store)

    firstStream.open(1)
    secondStream.open(2)

    expect(FakeEventSource.instances.map((source) => source.url)).toEqual([
      '/api/chats/1/events',
      '/api/chats/2/events',
    ])

    firstStream.close()
    expect(FakeEventSource.instances[0].closed).toBe(true)
    expect(FakeEventSource.instances[1].closed).toBe(false)

    secondStream.close()
    expect(FakeEventSource.instances[1].closed).toBe(true)
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
