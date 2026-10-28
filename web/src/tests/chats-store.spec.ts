// Pure-chat store tests: snapshot mapping, streaming deltas, message updates
// and the mutation endpoints. The EventSource is faked so the tests observe
// exactly what the store does with each frame.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useChatsStore } from '../stores/chats'
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
    this.readyState = FakeEventSource.OPEN
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data: JSON.stringify(data) } as MessageEvent)
    }
  }

  close() {
    this.readyState = FakeEventSource.CLOSED
  }
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
    text: 'hello',
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
      message({ id: 9, role: 'user', text: 'hi' }),
      message({ id: 10, reasoning: 'think', text: 'hello' }),
    ],
    running: false,
    truncated: false,
    ...overrides,
  }
}

function calls(fetchSpy: ReturnType<typeof vi.fn>): Array<{ url: string; init?: RequestInit }> {
  return (fetchSpy.mock.calls as unknown as [string, RequestInit?][]).map(([url, init]) => ({
    url: String(url),
    init,
  }))
}

function stubFetch(handler: (url: string, init?: RequestInit) => unknown) {
  const spy = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    return { status: 200, json: async () => handler(url, init) } as Response
  })
  vi.stubGlobal('fetch', spy)
  return spy
}

beforeEach(() => {
  resetChatBusId()
  closeChatBus()
  setActivePinia(createPinia())
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
  installTestCSRF()
})

afterEach(() => {
  closeChatBus()
  vi.unstubAllGlobals()
})

describe('chats store stream mapping', () => {
  it('opens a conversation and maps its snapshot into ChatEntry lines', async () => {
    stubFetch((url) => {
      if (url === '/api/chats/1') return chat()
      return {}
    })
    const chats = useChatsStore()
    await chats.openChat(1)

    const source = FakeEventSource.instances[0]
    expect(source.url).toContain('/api/chat-events/')
    source.dispatch('snapshot', { type: 'snapshot', chatId: 1, timestamp: '', snapshot: snapshot() })

    expect(chats.activeId).toBe(1)
    expect(chats.statusFor(1)).toBe('open')
    const entries = chats.messagesFor(1)
    expect(entries.map((entry) => entry.kind)).toEqual(['message', 'thinking', 'message'])
    expect(entries[0].text).toBe('hi')
    expect(entries[1].text).toBe('think')
    expect(entries[2].model?.id).toBe('Test Model')
  })

  it('appends reasoning before the assistant bubble and text to it', async () => {
    const chats = useChatsStore()
    chats.activeId = 1
    chats.messages = {
      1: [
        { id: 'msg:10', kind: 'message', role: 'assistant', text: '', streaming: true },
      ],
    }
    chats.applyDelta(1, { messageId: 10, reasoning: 'why' })
    expect(chats.messagesFor(1).map((entry) => entry.text)).toEqual(['why', ''])

    chats.applyDelta(1, { messageId: 10, text: 'Hello' })
    expect(chats.messagesFor(1).map((entry) => entry.text)).toEqual(['why', 'Hello'])
    expect(chats.messagesFor(1)[1].streaming).toBe(true)
  })

  it('settles a message update and surfaces a failure entry', async () => {
    const chats = useChatsStore()
    chats.activeId = 1
    chats.messages = { 1: [{ id: 'msg:10', kind: 'message', role: 'assistant', text: 'partial', streaming: true }] }
    chats.applyMessageUpdate(
      1,
      message({ text: 'partial', status: 'failed', errorCode: 'upstream_error', error: '模型服务返回错误' }),
    )
    const entries = chats.messagesFor(1)
    expect(entries).toHaveLength(2)
    expect(entries[0].streaming).toBe(false)
    expect(entries[1].kind).toBe('error')
    expect(entries[1].text).toContain('模型服务返回错误')
    expect(chats.runningFor(1)).toBe(false)
  })

  it('handles a deleted stream without leaving the conversation running', async () => {
    const chats = useChatsStore()
    chats.activeId = 1
    chats.running = { 1: true }
    chats.handleClosed(1, 'deleted')
    expect(chats.closedReason).toBe('deleted')
    expect(chats.runningFor(1)).toBe(false)
  })
})

describe('chats store mutations', () => {
  it('creates a conversation on first send and returns its id', async () => {
    const fetchSpy = stubFetch(() => chat())
    const chats = useChatsStore()
    const created = await chats.createAndSend('hello', [], 1)
    expect(created?.id).toBe(1)
    expect(chats.chats[0].id).toBe(1)
    const call = calls(fetchSpy)[0]
    expect(call.url).toBe('/api/chats')
    expect(call.init?.method).toBe('POST')
    expect(JSON.parse(String(call.init?.body))).toEqual({ modelId: 1, text: 'hello', images: [] })
    expect((call.init?.headers as Record<string, string>)['X-CSRF-Token']).toBe(testCSRFToken)
  })

  it('sends, stops, regenerates and edits through their routes', async () => {
    const fetchSpy = stubFetch(() => chat())
    const chats = useChatsStore()
    chats.activeId = 1
    chats.active = chat()
    await chats.sendMessage('hi', [], 1)
    await chats.stop()
    await chats.regenerate()
    await chats.editAndResend(9, 'edited', [])
    const paths = calls(fetchSpy).map((call) => `${call.init?.method} ${call.url}`)
    expect(paths).toEqual([
      'POST /api/chats/1/messages',
      'POST /api/chats/1/stop',
      'POST /api/chats/1/regenerate',
      'POST /api/chats/1/messages/9/edit',
    ])
  })

  it('renames, switches model and deletes the active conversation', async () => {
    const fetchSpy = stubFetch((url, init) => {
      if (init?.method === 'DELETE') return undefined
      if (url === '/api/chats/1') return chat({ title: '新标题' })
      return chat()
    })
    const chats = useChatsStore()
    chats.activeId = 1
    chats.active = chat()
    chats.chats = [chat()]
    await chats.rename('新标题')
    await chats.setModel(2)
    expect(chats.active?.title).toBe('新标题')
    const removed = await chats.removeActive()
    expect(removed).toBe(true)
    expect(chats.activeId).toBeNull()
    expect(chats.chats).toHaveLength(0)
    const paths = calls(fetchSpy).map((call) => `${call.init?.method} ${call.url}`)
    expect(paths).toContain('DELETE /api/chats/1')
  })

  it('manages groups and moves chats between groups', async () => {
    const fetchSpy = stubFetch((url, init) => {
      if (url === '/api/chat-groups' && (!init?.method || init.method === 'GET')) {
        return { groups: [{ id: 10, name: '旧分组', createdAt: '2026-10-22T00:00:00Z', updatedAt: '2026-10-22T00:00:00Z' }] }
      }
      if (url === '/api/chat-groups' && init?.method === 'POST') {
        return { id: 11, name: '新分组', createdAt: '2026-10-22T00:00:00Z', updatedAt: '2026-10-22T00:00:00Z' }
      }
      if (url === '/api/chat-groups/11' && init?.method === 'PUT') {
        return { id: 11, name: '新分组改名', createdAt: '2026-10-22T00:00:00Z', updatedAt: '2026-10-22T00:00:00Z' }
      }
      if (url === '/api/chat-groups/11' && init?.method === 'DELETE') {
        return undefined
      }
      if (url === '/api/chats/1' && init?.method === 'PUT') {
        return chat({ id: 1, groupId: 11 })
      }
      return {}
    })

    const chats = useChatsStore()
    chats.chats = [chat({ id: 1, groupId: 10 })]

    // 1. loadGroups
    await chats.loadGroups()
    expect(chats.groups).toHaveLength(1)
    expect(chats.groups[0].name).toBe('旧分组')

    // 2. createGroup
    const created = await chats.createGroup('新分组')
    expect(created?.id).toBe(11)
    expect(chats.groups).toHaveLength(2)

    // 3. renameGroup
    const renamed = await chats.renameGroup(11, '新分组改名')
    expect(renamed).toBe(true)
    expect(chats.groupById(11)?.name).toBe('新分组改名')

    // 4. moveChatToGroup
    const moved = await chats.moveChatToGroup(1, 11)
    expect(moved).toBe(true)
    expect(chats.chats[0].groupId).toBe(11)

    // 5. deleteGroup
    const deleted = await chats.deleteGroup(11)
    expect(deleted).toBe(true)
    expect(chats.groups.some((g) => g.id === 11)).toBe(false)
    // Chats in this group should also be removed
    expect(chats.chats.some((c) => c.groupId === 11)).toBe(false)
  })
})
