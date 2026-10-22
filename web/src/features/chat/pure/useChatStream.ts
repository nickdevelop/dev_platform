// EventSource lifecycle for one pure-chat conversation. Browser resources stay
// outside Vue's reactive proxies; the store receives already-parsed events.

import { chatEventsPath, type Chat, type ChatDelta, type ChatEvent, type ChatMessage, type ChatSnapshot, type ChatTurn } from '../../../api/chats'
import type { StreamStatus } from '../state/entry'

export interface ChatStreamStore {
  streamStatus: Record<number, StreamStatus>
  applySnapshot: (snapshot: ChatSnapshot) => void
  applyDelta: (delta: ChatDelta) => void
  applyMessageUpdate: (message: ChatMessage) => void
  applyConversation: (chat: Chat) => void
  applyTurn: (turn: ChatTurn) => void
  handleClosed: (reason: string) => void
}

let source: EventSource | null = null
let openChatId: number | null = null

function closeSource() {
  if (source) {
    source.close()
    source = null
  }
  openChatId = null
}

function setStatus(store: ChatStreamStore, chatId: number, status: StreamStatus) {
  store.streamStatus = { ...store.streamStatus, [chatId]: status }
}

function parseEvent(event: MessageEvent): ChatEvent | null {
  try {
    return JSON.parse(event.data) as ChatEvent
  } catch {
    return null
  }
}

export function openChatStream(store: ChatStreamStore, chatId: number) {
  if (openChatId === chatId && source) return
  closeSource()
  openChatId = chatId
  setStatus(store, chatId, 'connecting')

  const next = new EventSource(chatEventsPath(chatId))
  source = next
  next.onopen = () => setStatus(store, chatId, 'open')
  next.onerror = () => {
    if (source !== next) return
    setStatus(store, chatId, next.readyState === EventSource.CLOSED ? 'error' : 'reconnecting')
  }

  const listen = <T extends ChatEvent['type']>(type: T, apply: (event: Extract<ChatEvent, { type: T }>) => void) => {
    next.addEventListener(type, (event) => {
      if (source !== next) return
      const parsed = parseEvent(event as MessageEvent)
      if (!parsed || parsed.type !== type) return
      apply(parsed as Extract<ChatEvent, { type: T }>)
    })
  }

  listen('snapshot', (event) => {
    store.applySnapshot(event.snapshot)
    setStatus(store, chatId, 'open')
  })
  listen('delta', (event) => store.applyDelta(event.delta))
  listen('message_update', (event) => store.applyMessageUpdate(event.message))
  listen('conversation', (event) => store.applyConversation(event.conversation))
  listen('turn', (event) => store.applyTurn(event.turn))
  listen('closed', (event) => {
    store.handleClosed(event.reason)
    closeSource()
  })
}

export function reconnectChatStream(store: ChatStreamStore, chatId: number) {
  closeSource()
  openChatStream(store, chatId)
}

export function closeChatStream() {
  closeSource()
}
