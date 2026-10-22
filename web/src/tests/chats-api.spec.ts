// Contract tests for the pure-chat API module: paths, verbs, JSON bodies and
// the CSRF header on mutations.
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  chatEventsPath,
  createChat,
  editChatMessage,
  getChat,
  listChats,
  regenerateChat,
  sendChatMessage,
  stopChat,
  updateChat,
  deleteChat,
} from '../api/chats'
import { installTestCSRF, testCSRFToken } from './csrf-fixture'

interface Call {
  url: string
  init?: RequestInit
}

function stub(payload: unknown = {}): Call[] {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: String(input), init })
      return { status: 200, json: async () => payload } as Response
    }),
  )
  return calls
}

function headers(call: Call): Record<string, string> {
  return (call.init?.headers ?? {}) as Record<string, string>
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('pure chat API contracts', () => {
  it('lists chats with paging and reads one chat without CSRF', async () => {
    const calls = stub({ chats: [], total: 0 })
    await listChats(10, 20)
    await getChat(7)
    expect(calls[0].url).toBe('/api/chats?limit=10&offset=20')
    expect(calls[0].init?.method).toBe('GET')
    expect(calls[1].url).toBe('/api/chats/7')
    expect(headers(calls[1])['X-CSRF-Token']).toBeUndefined()
  })

  it('creates, updates and deletes a chat through mutate()', async () => {
    installTestCSRF()
    const calls = stub({ id: 1 })
    await createChat({ modelId: 1, text: 'hi', images: [] })
    await updateChat(1, { title: 'new' })
    await deleteChat(1)

    expect(calls.map((call) => `${call.init?.method} ${call.url}`)).toEqual([
      'POST /api/chats',
      'PUT /api/chats/1',
      'DELETE /api/chats/1',
    ])
    expect(JSON.parse(String(calls[0].init?.body))).toEqual({ modelId: 1, text: 'hi', images: [] })
    for (const call of calls) {
      expect(headers(call)['X-CSRF-Token']).toBe(testCSRFToken)
    }
  })

  it('sends messages, stops, regenerates and edits through their routes', async () => {
    installTestCSRF()
    const calls = stub({ id: 1 })
    await sendChatMessage(1, { text: 'hello' })
    await stopChat(1)
    await regenerateChat(1)
    await editChatMessage(1, 9, { text: 'edited' })

    expect(calls.map((call) => `${call.init?.method} ${call.url}`)).toEqual([
      'POST /api/chats/1/messages',
      'POST /api/chats/1/stop',
      'POST /api/chats/1/regenerate',
      'POST /api/chats/1/messages/9/edit',
    ])
    expect(JSON.parse(String(calls[3].init?.body))).toEqual({ text: 'edited' })
  })

  it('exposes the same-origin SSE path', () => {
    expect(chatEventsPath(12)).toBe('/api/chats/12/events')
  })
})
