// EventSource lifecycle for one pure-chat conversation. Browser resources stay
// outside Vue's reactive proxies; the store receives already-parsed events.
//
// A stream is created per store through createChatStream, so the main Chat
// panel and the quick-assistant popover can stream different conversations at
// the same time without closing each other. The legacy module-level helpers
// keep the main chats store's original call sites unchanged.

import { chatEventsPath, type Chat, type ChatDelta, type ChatEvent, type ChatMessage, type ChatSnapshot, type ChatTurn } from '../../../api/chats'
import type { StreamStatus } from '../state/entry'

export interface ChatStreamStore {
  streamStatus: Record<number, StreamStatus>
  setStreamStatus: (chatId: number, status: StreamStatus) => void
  applySnapshot: (snapshot: ChatSnapshot) => void
  // Callbacks carry the conversation id so a store can keep several
  // conversations' state without relying on a global "active" pointer.
  applyDelta: (chatId: number, delta: ChatDelta) => void
  applyMessageUpdate: (chatId: number, message: ChatMessage) => void
  applyConversation: (chat: Chat) => void
  applyTurn: (chatId: number, turn: ChatTurn) => void
  handleClosed: (chatId: number, reason: string) => void
}

export interface ChatStream {
  open: (chatId: number) => void
  reconnect: (chatId: number) => void
  close: () => void
}

function setStatus(store: ChatStreamStore, chatId: number, status: StreamStatus) {
  store.setStreamStatus(chatId, status)
}

function parseEvent(event: MessageEvent): ChatEvent | null {
  try {
    return JSON.parse(event.data) as ChatEvent
  } catch {
    return null
  }
}

// createChatStream owns exactly one EventSource. Each store gets its own
// instance, which is what allows the workbench and the quick-assistant popover
// to stay live at the same time.
export function createChatStream(store: ChatStreamStore): ChatStream {
  let source: EventSource | null = null
  let openChatId: number | null = null

  function closeSource() {
    if (source) {
      source.close()
      source = null
    }
    openChatId = null
  }

  function open(chatId: number) {
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
    listen('delta', (event) => store.applyDelta(chatId, event.delta))
    listen('message_update', (event) => store.applyMessageUpdate(chatId, event.message))
    listen('conversation', (event) => store.applyConversation(event.conversation))
    listen('turn', (event) => store.applyTurn(chatId, event.turn))
    listen('closed', (event) => {
      store.handleClosed(chatId, event.reason)
      closeSource()
    })
  }

  return {
    open,
    reconnect(chatId: number) {
      closeSource()
      open(chatId)
    },
    close: closeSource,
  }
}

// Main pure-chat store stream. Kept outside reactive state in a WeakMap so each
// Pinia instance (including a fresh one in a unit test) gets its own
// EventSource.
const mainStreams = new WeakMap<object, ChatStream>()

function mainStreamFor(store: ChatStreamStore): ChatStream {
  let stream = mainStreams.get(store)
  if (!stream) {
    stream = createChatStream(store)
    mainStreams.set(store, stream)
  }
  return stream
}

export function openChatStream(store: ChatStreamStore, chatId: number) {
  mainStreamFor(store).open(chatId)
}

export function reconnectChatStream(store: ChatStreamStore, chatId: number) {
  mainStreamFor(store).reconnect(chatId)
}

export function closeChatStream(store: ChatStreamStore) {
  mainStreams.get(store)?.close()
}
