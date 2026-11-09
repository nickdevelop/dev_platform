// Debug panel unit tests: the pure capture/filter model, the ring buffer and
// the envelope tap that feeds it. Component rendering is covered by manual
// acceptance since the panel is a developer aid, but every behavior rule here
// is asserted in isolation without network dependencies.
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import { installTestCSRF } from './csrf-fixture'
import { createPinia, setActivePinia } from 'pinia'
import {
  DEBUG_BUFFER_LIMIT,
  appendEntry,
  createEntry,
  emptyDebugFilter,
  extractIdentities,
  filterEntries,
  formatClock,
  matchesFilter,
  prettyJSON,
  type DebugEntry,
} from '../features/debug/debugModel'
import { useDebugPanelStore } from '../stores/debugPanel'
import { UnifiedEventBusClient } from '../shared/events/UnifiedEventBusClient'
import type { EventEnvelope } from '../api/events'

function envelope(overrides: Partial<EventEnvelope> = {}): EventEnvelope {
  return {
    id: 1,
    channel: 'sessions',
    targetId: 'session-1',
    eventType: 'message',
    timestamp: 1730770800123,
    payload: {},
    ...overrides,
  }
}

describe('debugModel · capture ring buffer', () => {
  it('createEntry snapshots the envelope identity and receive time', () => {
    const entry = createEntry(7, envelope())
    expect(entry.seq).toBe(7)
    expect(entry.channel).toBe('sessions')
    expect(entry.targetId).toBe('session-1')
    expect(entry.eventType).toBe('message')
    expect(entry.receivedAt).toBe(1730770800123)
  })

  it('createEntry falls back to wall time when the envelope lacks a timestamp', () => {
    const entry = createEntry(1, envelope({ timestamp: undefined }))
    expect(Number.isFinite(entry.receivedAt)).toBe(true)
  })

  it('appendEntry prunes beyond the limit and reports the dropped count', () => {
    const buffer: DebugEntry[] = []
    let dropped = 0
    for (let i = 1; i <= DEBUG_BUFFER_LIMIT + 3; i++) {
      dropped += appendEntry(buffer, createEntry(i, envelope({ targetId: `s-${i}` })), DEBUG_BUFFER_LIMIT)
    }
    expect(buffer.length).toBe(DEBUG_BUFFER_LIMIT)
    expect(dropped).toBe(3)
    // Oldest-no-history rule: only the newest frames survive.
    expect(buffer[0].targetId).toBe(`s-4`)
    expect(buffer[buffer.length - 1].targetId).toBe(`s-${DEBUG_BUFFER_LIMIT + 3}`)
  })
})

describe('debugModel · identity extraction', () => {
  it('collects session/chat/instance/backend identifiers from the payload', () => {
    const entry = createEntry(1, envelope({
      payload: {
        sessionId: 'abc',
        instanceId: 'inst-9',
        backend: 'opencode',
        nested: { chatId: 42 },
      },
    }))
    const identities = extractIdentities(entry)
    expect(identities.targetId).toBe('session-1')
    expect(identities.sessionIds).toEqual(['abc'])
    expect(identities.instanceIds).toEqual(['inst-9'])
    expect(identities.backends).toEqual(['opencode'])
    expect(identities.chatIds).toEqual(['42'])
  })

  it('does not explode on scalar or missing payloads', () => {
    const identities = extractIdentities(createEntry(1, envelope({ payload: 'plain text' })))
    expect(identities.sessionIds).toEqual([])
    expect(identities.targetId).toBe('session-1')
  })
})

describe('debugModel · filtering', () => {
  const entries: DebugEntry[] = [
    { seq: 1, receivedAt: 0, channel: 'sessions', targetId: 'session-a', eventType: 'delta', payload: {} },
    { seq: 2, receivedAt: 0, channel: 'sessions', targetId: 'session-b', eventType: 'turn', payload: { sessionId: 'session-a' } },
    { seq: 3, receivedAt: 0, channel: 'chat', targetId: '17', eventType: 'delta', payload: { chatId: 17 } },
    { seq: 4, receivedAt: 0, channel: 'tasks', targetId: '9', eventType: 'chunk', payload: {} },
  ]

  it('an empty filter passes everything through by reference', () => {
    expect(filterEntries(entries, emptyDebugFilter())).toBe(entries)
  })

  it('channel过滤为精确匹配', () => {
    expect(filterEntries(entries, { ...emptyDebugFilter(), channel: 'chat' })).toHaveLength(1)
    expect(filterEntries(entries, { ...emptyDebugFilter(), channel: 'sessions' })).toHaveLength(2)
  })

  it('关键字命中 targetId 与 payload 内会话/chat 标识字段', () => {
    expect(filterEntries(entries, { ...emptyDebugFilter(), keyword: 'session-a' })).toHaveLength(2)
    expect(filterEntries(entries, { ...emptyDebugFilter(), keyword: '17' })).toHaveLength(1)
    expect(filterEntries(entries, { ...emptyDebugFilter(), keyword: '不存在的标识' })).toHaveLength(0)
  })

  it('事件类型过滤区分大小写不敏感的部分匹配', () => {
    expect(filterEntries(entries, { ...emptyDebugFilter(), eventType: 'DELTA' })).toHaveLength(2)
  })

  it('三个维度是 AND 关系', () => {
    expect(
      filterEntries(entries, { channel: 'chat', keyword: '17', eventType: 'turn' }),
    ).toHaveLength(0)
    expect(
      filterEntries(entries, { channel: 'chat', keyword: '17', eventType: 'delta' }),
    ).toHaveLength(1)
  })

  it('matchesFilter respects the channel=all escape hatch', () => {
    expect(matchesFilter(entries[0], { ...emptyDebugFilter(), channel: 'all' })).toBe(true)
  })
})

describe('debugModel · formatting', () => {
  it('formatClock renders HH:MM:SS.mmm with zero padding', () => {
    // Local 10:40:00.123 is asserted timezone-free by re-deriving from the input.
    const ts = new Date(2024, 0, 1, 10, 40, 0, 123).getTime()
    expect(formatClock(ts)).toBe('10:40:00.123')
  })

  it('prettyJSON pretty-prints and degrades gracefully', () => {
    expect(prettyJSON({ a: 1 })).toBe('{\n  "a": 1\n}')
    const cyclic: Record<string, unknown> = {}
    cyclic.self = cyclic
    expect(prettyJSON(cyclic)).toContain('无法序列化')
  })
})

describe('UnifiedEventBusClient · envelope taps', () => {
  class StubEventSource {
    listeners = new Map<string, Array<(event: unknown) => void>>()
    static instances: StubEventSource[] = []
    constructor(public url: string) {
      StubEventSource.instances.push(this)
    }
    addEventListener(type: string, listener: (event: unknown) => void) {
      const list = this.listeners.get(type) ?? []
      list.push(listener)
      this.listeners.set(type, list)
    }
    close() {}
  }

  beforeAll(() => {
    vi.stubGlobal('EventSource', StubEventSource)
    installTestCSRF()
    vi.stubGlobal('fetch', async () => ({ ok: true, status: 200, json: async () => ({}) }))
  })
  afterAll(() => {
    vi.unstubAllGlobals()
    sessionStorage.clear()
  })

  it('taps observe every dispatched envelope before the handlers, once each', () => {
    const client = new UnifiedEventBusClient()
    const seen: EventEnvelope[] = []
    const handled: EventEnvelope[] = []
    client.addEnvelopeTap((env) => seen.push(env))
    client.subscribe('sessions', 'session-1', (env) => handled.push(env))
    const env = envelope()
    client.dispatchEnvelope(env)
    expect(seen).toHaveLength(1)
    expect(handled).toHaveLength(1)
  })

  it('detach removes the tap without disturbing dispatch', () => {
    const client = new UnifiedEventBusClient()
    const seen: EventEnvelope[] = []
    const detach = client.addEnvelopeTap((env) => seen.push(env))
    detach()
    client.dispatchEnvelope(envelope())
    expect(seen).toHaveLength(0)
  })

  it('a throwing tap is isolated from the real consumers', () => {
    const client = new UnifiedEventBusClient()
    client.addEnvelopeTap(() => {
      throw new Error('debug tap blew up')
    })
    const handled: EventEnvelope[] = []
    client.subscribe('sessions', 'session-1', (env) => handled.push(env))
    client.dispatchEnvelope(envelope())
    expect(handled).toHaveLength(1)
  })

  it('invalid envelopes are rejected before taps run', () => {
    const client = new UnifiedEventBusClient()
    const seen: EventEnvelope[] = []
    client.addEnvelopeTap((env) => seen.push(env))
    client.dispatchEnvelope(envelope({ channel: '' }))
    expect(seen).toHaveLength(0)
  })
})

describe('debugPanel store · capture lifecycle', () => {
  setActivePinia(createPinia())

  it('closeDrawer wipes entries, dropped counter and filter — no history survives', () => {
    const store = useDebugPanelStore()
    store.setEnabled(true)
    store.openDrawer()
    store.recordEnvelope(envelope())
    expect(store.entries).toHaveLength(1)
    store.closeDrawer()
    expect(store.entries).toHaveLength(0)
    expect(store.dropped).toBe(0)
    expect(store.filter.channel).toBe('all')
    expect(store.open).toBe(false)
  })

  it('setEnabled(false) closes an open drawer and detaches capture', () => {
    const store = useDebugPanelStore()
    store.setEnabled(true)
    store.openDrawer()
    store.setEnabled(false)
    expect(store.open).toBe(false)
    // Closed panels capture nothing even when frames keep flowing.
    const before = store.entries.length
    store.recordEnvelope(envelope())
    expect(store.entries).toHaveLength(before)
  })

  it('clearBuffer resets visible data without closing the drawer', () => {
    const store = useDebugPanelStore()
    store.setEnabled(true)
    store.openDrawer()
    store.recordEnvelope(envelope())
    store.clearBuffer()
    expect(store.entries).toHaveLength(0)
    expect(store.open).toBe(true)
    store.closeDrawer()
  })

  it('an inactive drawer never records frames (open-only capture rule)', () => {
    const store = useDebugPanelStore()
    store.setEnabled(true)
    store.recordEnvelope(envelope())
    expect(store.entries).toHaveLength(0)
    store.openDrawer()
    store.recordEnvelope(envelope())
    expect(store.entries).toHaveLength(1)
    store.closeDrawer()
  })
})
