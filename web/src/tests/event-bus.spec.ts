import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ensureBusConnection,
  getBusId,
  setBusId,
  resetBusId,
  subscribeSessionOnBus,
  unsubscribeSessionFromBus,
  closeBus,
} from '../features/chat/execution/EventBusClient'
import type { StreamStore } from '../features/chat/execution/SessionEventStream'
import type { SubscriptionSnapshot } from '../api/events'
import { installTestCSRF } from './csrf-fixture'

class MockEventSource {
  static instances: MockEventSource[] = []
  url: string
  readyState = 0 // CONNECTING
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

  emit(type: string, data: string, lastEventId?: string) {
    const list = this.listeners.get(type) ?? []
    for (const listener of list) {
      listener({ data, lastEventId })
    }
  }

  open() {
    this.readyState = 1 // OPEN
    this.emit('ready', '{}')
  }

  close() {
    this.readyState = 2 // CLOSED
  }
}

function createMockStore(): StreamStore {
  return {
    streamStatus: {},
    messages: {},
    historyTailGuard: {},
    echoPending: {},
    sessionById: (id: string) => ({
      id,
      projectId: 1,
      instanceId: 'inst-1',
      nativeSessionId: 'native-' + id,
      title: 'Session ' + id,
      backend: 'pi',
      status: 'active',
      turn: { phase: 'ended', outcome: 'completed' },
      capabilities: { streaming: true },
    } as any),
    pushEntry: vi.fn(),
    setTurn: vi.fn(),
    settleStreaming: vi.fn(),
    applyTurn: vi.fn(),
    applyInteraction: vi.fn(),
    applyInteractionSnapshot: vi.fn(),
    subscribe: vi.fn(),
    appendMessage: vi.fn(),
    appendThinking: vi.fn(),
    applyLifecycle: vi.fn(),
    repairDanglingEntries: vi.fn().mockResolvedValue(undefined),
    rebuildFromHistory: vi.fn().mockResolvedValue(undefined),
  }
}

// The transport refuses mutating requests without a token source; the app
// installs the real provider in main.ts, which these specs never run.
installTestCSRF()

describe('EventBusClient', () => {
  let store: StreamStore
  const fetchCalls: Array<{ method: string; url: string; body?: any }> = []

  beforeEach(() => {
    resetBusId()
    setBusId('test-bus-uuid-123')
    closeBus()
    MockEventSource.instances = []
    vi.stubGlobal('EventSource', MockEventSource)
    store = createMockStore()
    fetchCalls.length = 0

    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      fetchCalls.push({
        method: init?.method ?? 'GET',
        url,
        body: init?.body ? JSON.parse(init.body as string) : undefined,
      })

      if (url.includes('/subscriptions') && init?.method === 'POST') {
        const body = init.body ? JSON.parse(init.body as string) : {}
        const snapshot: SubscriptionSnapshot = {
          sessionId: body.sessionId,
          turn: { id: 'turn-1', phase: 'ended', outcome: 'completed' },
          interactions: { permissions: [], forms: [] },
          resync: false,
        }
        return {
          ok: true,
          status: 200,
          json: async () => snapshot,
        } as Response
      }

      if (init?.method === 'DELETE') {
        return {
          ok: true,
          status: 200,
          json: async () => ({ success: true }),
        } as Response
      }

      return {
        ok: true,
        status: 200,
        json: async () => ({}),
      } as Response
    })
  })

  it('maintains a single EventSource connection for multiple session subscriptions', async () => {
    // 1. Subscribe session 1
    await subscribeSessionOnBus(store, 1, 's1')

    expect(MockEventSource.instances).toHaveLength(1)
    expect(MockEventSource.instances[0].url).toBe('/api/events/test-bus-uuid-123')
    expect(fetchCalls[0].url).toBe('/api/events/test-bus-uuid-123/subscriptions')
    expect(fetchCalls[0].body).toEqual({ projectId: 1, sessionId: 's1' })

    // 2. Subscribe session 2 on the same window
    await subscribeSessionOnBus(store, 1, 's2')

    // Still only ONE EventSource instance
    expect(MockEventSource.instances).toHaveLength(1)
    expect(fetchCalls[1].url).toBe('/api/events/test-bus-uuid-123/subscriptions')
    expect(fetchCalls[1].body).toEqual({ projectId: 1, sessionId: 's2' })

    // 3. Subscribe session 3
    await subscribeSessionOnBus(store, 1, 's3')
    expect(MockEventSource.instances).toHaveLength(1)
  })

  it('routes incoming events to their respective sessions by sessionId', async () => {
    await subscribeSessionOnBus(store, 1, 's1')
    await subscribeSessionOnBus(store, 1, 's2')

    const source = MockEventSource.instances[0]
    source.open()

    // Emit event for s1
    const eventS1 = {
      instanceId: 'inst-1',
      sessionId: 's1',
      backend: 'pi',
      sequence: 10,
      timestamp: new Date().toISOString(),
      type: 'message',
      message: { role: 'assistant', text: 'hello s1', final: true },
    }
    source.emit('message', JSON.stringify(eventS1), '10')

    expect(store.appendMessage).toHaveBeenCalledWith(
      's1',
      'assistant',
      'hello s1',
      true,
      10,
      undefined,
      undefined,
      undefined,
      expect.any(String),
    )
    expect(store.appendMessage).not.toHaveBeenCalledWith(
      's2',
      expect.anything(),
      expect.anything(),
      expect.anything(),
      expect.anything(),
      expect.anything(),
      expect.anything(),
      expect.anything(),
      expect.anything(),
    )

    // Emit event for s2
    const eventS2 = {
      instanceId: 'inst-1',
      sessionId: 's2',
      backend: 'pi',
      sequence: 20,
      timestamp: new Date().toISOString(),
      type: 'thinking',
      thinking: { text: 'thinking for s2', final: true },
    }
    source.emit('thinking', JSON.stringify(eventS2), '20')

    expect(store.appendThinking).toHaveBeenCalledWith('s2', 'thinking for s2', true, 20)
  })

  it('unsubscribes a session and issues DELETE call', async () => {
    await subscribeSessionOnBus(store, 1, 's1')
    await unsubscribeSessionFromBus(store, 's1')

    expect(fetchCalls[1].method).toBe('DELETE')
    expect(fetchCalls[1].url).toBe('/api/events/test-bus-uuid-123/subscriptions/s1')
    expect(store.streamStatus['s1']).toBe('idle')
  })

  it('triggers rebuildFromHistory on resync frame', async () => {
    await subscribeSessionOnBus(store, 1, 's1')
    const source = MockEventSource.instances[0]

    source.emit('resync', JSON.stringify({ sessionId: 's1' }))
    expect(store.rebuildFromHistory).toHaveBeenCalledWith(1, 's1')
  })
})
