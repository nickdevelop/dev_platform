// EventSource lifecycle for pure-chat conversations using the multiplexed event bus.
// Browser resources stay outside Vue's reactive proxies; the store receives
// already-parsed events.
//
// Individual stores create a stream handle through createChatStream, which binds
// to the window-level ChatEventBusClient. Multiple stores (e.g. the main Chat
// page and the quick-assistant popover) can stream different conversations simultaneously
// over a single multiplexed SSE connection without closing each other.

import type { Chat, ChatDelta, ChatMessage, ChatSnapshot, ChatTurn } from '../../../api/chats'
import type { StreamStatus } from '../state/entry'
import {
  subscribeChatOnBus,
  unsubscribeChatFromBus,
} from './ChatEventBusClient'

export interface ChatStreamStore {
  streamStatus: Record<number, StreamStatus>
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

// createChatStream delegates conversation subscription to the shared ChatEventBusClient.
export function createChatStream(store: ChatStreamStore): ChatStream {
  let openChatId: number | null = null

  function closeSource() {
    if (openChatId !== null) {
      const prev = openChatId
      openChatId = null
      void unsubscribeChatFromBus(store, prev)
    }
  }

  function open(chatId: number) {
    if (openChatId === chatId) return
    if (openChatId !== null) {
      void unsubscribeChatFromBus(store, openChatId)
    }
    openChatId = chatId
    void subscribeChatOnBus(store, chatId)
  }

  return {
    open,
    reconnect(chatId: number) {
      openChatId = chatId
      void subscribeChatOnBus(store, chatId)
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
