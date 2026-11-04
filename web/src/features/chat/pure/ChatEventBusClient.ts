// ChatEventBusClient adapts pure-chat streaming to the single unified event bus (unifiedEventBus).
// It no longer maintains an independent EventSource or private busId.

import {
  subscribeChatEventBus,
  type ChatSnapshot,
} from '../../../api/chats'
import type { StreamStatus } from '../state/entry'
import type { ChatStreamStore } from './useChatStream'
import { unifiedEventBus } from '../../../shared/events/UnifiedEventBusClient'
import type { EventEnvelope } from '../../../api/events'

export function getChatBusId(): string {
  return unifiedEventBus.getBusId()
}

export function setChatBusId(id: string) {
  unifiedEventBus.setBusId(id)
}

export function resetChatBusId() {
  unifiedEventBus.resetBusId()
  subscribedChats.clear()
  chatHandlers.clear()
}

export function closeChatBus() {
  unifiedEventBus.close()
  subscribedChats.clear()
  chatHandlers.clear()
}

// Map of subscribed chats: chatId -> Set of stores listening to it.
const subscribedChats = new Map<number, Set<ChatStreamStore>>()
// Handler registry per chatId on the unifiedEventBus
const chatHandlers = new Map<number, (env: EventEnvelope) => void>()

export function getSubscribedChatIds(): number[] {
  return [...subscribedChats.keys()]
}

export function hasSubscribedChat(chatId: number): boolean {
  return subscribedChats.has(chatId)
}

export function getChatBusStatus(): StreamStatus {
  return unifiedEventBus.getStatus()
}

function updateStoreStatus(chatId: number, status: StreamStatus) {
  const stores = subscribedChats.get(chatId)
  if (!stores) return
  for (const store of stores) {
    store.streamStatus = { ...store.streamStatus, [chatId]: status }
  }
}

function updateAllStoresStatus(status: StreamStatus) {
  for (const chatId of subscribedChats.keys()) {
    updateStoreStatus(chatId, status)
  }
}

// Ensure bus status listener updates store statuses
unifiedEventBus.onStatusChange((status) => {
  updateAllStoresStatus(status)
})

// Ensure bus reconnect triggers strategy A snapshot resync
unifiedEventBus.onReconnect(() => {
  void resubscribeActiveChats()
})

// resubscribeActiveChats re-registers all active chats after a bus reconnect.
async function resubscribeActiveChats() {
  const chatIds = [...subscribedChats.keys()]
  const id = getChatBusId()
  for (const chatId of chatIds) {
    try {
      const snapshot = await subscribeChatEventBus(id, chatId)
      if (snapshot && snapshot.conversation) {
        const stores = subscribedChats.get(chatId)
        if (stores) {
          for (const store of stores) {
            store.applySnapshot(snapshot)
            store.streamStatus = { ...store.streamStatus, [chatId]: 'open' }
          }
        }
      }
    } catch {
      // Re-registration may fail if backend is still recovering; status remains reconnecting
    }
  }
}

export function clearChatBusRepair() {
  // Maintained for backward compatibility
}

// ensureChatBusConnection delegates to the unified bus
export function ensureChatBusConnection(): EventSource | null {
  return unifiedEventBus.ensureConnection()
}

function getOrCreateChatHandler(chatId: number): (env: EventEnvelope) => void {
  let handler = chatHandlers.get(chatId)
  if (!handler) {
    handler = (env: EventEnvelope) => {
      const raw = env.payload as any
      if (!raw) return

      const stores = subscribedChats.get(chatId)
      if (!stores || stores.size === 0) return

      const eventType = env.eventType || raw.type
      for (const store of stores) {
        if (eventType === 'snapshot') {
          store.applySnapshot(raw.snapshot || raw)
          store.streamStatus = { ...store.streamStatus, [chatId]: 'open' }
        } else if (eventType === 'delta') {
          store.applyDelta(chatId, raw.delta || raw)
        } else if (eventType === 'message_update') {
          store.applyMessageUpdate(chatId, raw.message || raw)
        } else if (eventType === 'conversation') {
          store.applyConversation(raw.conversation || raw)
        } else if (eventType === 'turn') {
          store.applyTurn(chatId, raw.turn || raw)
        } else if (eventType === 'closed') {
          store.handleClosed(chatId, raw.reason)
        }
      }
    }
    chatHandlers.set(chatId, handler)
  }
  return handler
}

// subscribeChatOnBus attaches a chat to the client bus and syncs its initial snapshot.
export async function subscribeChatOnBus(
  store: ChatStreamStore,
  chatId: number,
): Promise<ChatSnapshot> {
  ensureChatBusConnection()

  let stores = subscribedChats.get(chatId)
  if (!stores) {
    stores = new Set<ChatStreamStore>()
    subscribedChats.set(chatId, stores)
  }
  stores.add(store)

  store.streamStatus = {
    ...store.streamStatus,
    [chatId]: unifiedEventBus.getStatus() === 'open' ? 'open' : 'connecting',
  }

  const handler = getOrCreateChatHandler(chatId)

  try {
    const snapshot = await unifiedEventBus.subscribe<ChatSnapshot>('chat', String(chatId), handler)
    if (snapshot && snapshot.conversation) {
      store.applySnapshot(snapshot)
      store.streamStatus = {
        ...store.streamStatus,
        [chatId]: 'open',
      }
    }
    return snapshot || ({} as ChatSnapshot)
  } catch {
    return undefined as unknown as ChatSnapshot
  }
}

// unsubscribeChatFromBus removes a store's subscription for a chat.
export async function unsubscribeChatFromBus(
  store: ChatStreamStore,
  chatId: number,
): Promise<void> {
  const stores = subscribedChats.get(chatId)
  if (!stores || !stores.has(store)) return

  stores.delete(store)
  store.streamStatus = {
    ...store.streamStatus,
    [chatId]: 'idle',
  }

  if (stores.size === 0) {
    subscribedChats.delete(chatId)
    const handler = chatHandlers.get(chatId)
    if (handler) {
      chatHandlers.delete(chatId)
      await unifiedEventBus.unsubscribe('chat', String(chatId), handler)
    }
  }
}
