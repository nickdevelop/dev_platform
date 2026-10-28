// ChatEventBusClient owns the single multiplexed SSE connection for pure chat
// and dispatches incoming events to their target conversations in the stores.

import {
  chatEventBusPath,
  subscribeChatEventBus,
  unsubscribeChatEventBus,
  type ChatEvent,
  type ChatSnapshot,
} from '../../../api/chats'
import type { StreamStatus } from '../state/entry'
import type { ChatStreamStore } from './useChatStream'

let busId: string = ''

export function getChatBusId(): string {
  if (!busId) {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      busId = crypto.randomUUID()
    } else {
      busId = `chat-bus-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
    }
  }
  return busId
}

export function setChatBusId(id: string) {
  busId = id
}

export function resetChatBusId() {
  busId = ''
}

// Global handle for the window-level pure-chat EventSource bus.
let busSource: EventSource | null = null
let currentBusStatus: StreamStatus = 'idle'

// Map of subscribed chats: chatId -> Set of stores listening to it.
const subscribedChats = new Map<number, Set<ChatStreamStore>>()

export function getSubscribedChatIds(): number[] {
  return [...subscribedChats.keys()]
}

export function hasSubscribedChat(chatId: number): boolean {
  return subscribedChats.has(chatId)
}

export function getChatBusStatus(): StreamStatus {
  return currentBusStatus
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

// Stall repair: one automatic redial after 3s if reconnecting stalls
const stallMs = 3000
let repairTimer: ReturnType<typeof setTimeout> | null = null
let hasRepaired = false

export function clearChatBusRepair() {
  if (repairTimer) {
    clearTimeout(repairTimer)
    repairTimer = null
  }
}

// ensureChatBusConnection opens the multiplexed SSE connection if not yet active.
export function ensureChatBusConnection(): EventSource {
  if (busSource && busSource.readyState !== EventSource.CLOSED) {
    if (currentBusStatus === 'reconnecting') {
      busSource.close()
      busSource = null
    } else {
      return busSource
    }
  }

  const id = getChatBusId()
  const source = new EventSource(chatEventBusPath(id))
  busSource = source
  currentBusStatus = 'connecting'
  updateAllStoresStatus('connecting')

  source.onopen = () => {
    clearChatBusRepair()
    hasRepaired = false
    currentBusStatus = 'open'
    updateAllStoresStatus('open')

    if (subscribedChats.size > 0) {
      void resubscribeActiveChats()
    }
  }

  source.onerror = () => {
    if (source.readyState === EventSource.CLOSED) {
      currentBusStatus = 'error'
      busSource = null
      clearChatBusRepair()
      updateAllStoresStatus('error')
    } else {
      currentBusStatus = 'reconnecting'
      updateAllStoresStatus('reconnecting')
      if (hasRepaired || repairTimer) return
      repairTimer = setTimeout(() => {
        repairTimer = null
        if (currentBusStatus !== 'reconnecting') return
        hasRepaired = true
        if (busSource) {
          busSource.close()
          busSource = null
        }
        ensureChatBusConnection()
      }, stallMs)
    }
  }

  source.addEventListener('ready', () => {
    clearChatBusRepair()
    hasRepaired = false
    currentBusStatus = 'open'
    updateAllStoresStatus('open')
  })

  const listen = <T extends ChatEvent['type']>(type: T, apply: (store: ChatStreamStore, event: Extract<ChatEvent, { type: T }>) => void) => {
    source.addEventListener(type, (e) => {
      let parsed: ChatEvent | null = null
      try {
        parsed = JSON.parse((e as MessageEvent).data) as ChatEvent
      } catch {
        return
      }
      if (!parsed || parsed.type !== type || !parsed.chatId) return

      const stores = subscribedChats.get(parsed.chatId)
      if (!stores || stores.size === 0) return

      for (const store of stores) {
        apply(store, parsed as Extract<ChatEvent, { type: T }>)
      }
    })
  }

  listen('snapshot', (store, event) => {
    store.applySnapshot(event.snapshot)
    store.streamStatus = { ...store.streamStatus, [event.chatId]: 'open' }
  })
  listen('delta', (store, event) => store.applyDelta(event.chatId, event.delta))
  listen('message_update', (store, event) => store.applyMessageUpdate(event.chatId, event.message))
  listen('conversation', (store, event) => store.applyConversation(event.conversation))
  listen('turn', (store, event) => store.applyTurn(event.chatId, event.turn))
  listen('closed', (store, event) => {
    store.handleClosed(event.chatId, event.reason)
  })

  return source
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
    [chatId]: currentBusStatus === 'open' ? 'open' : 'connecting',
  }

  const id = getChatBusId()
  try {
    const snapshot = await subscribeChatEventBus(id, chatId)
    if (snapshot && snapshot.conversation) {
      store.applySnapshot(snapshot)
      store.streamStatus = {
        ...store.streamStatus,
        [chatId]: 'open',
      }
    }
    return snapshot
  } catch {
    // If subscription API call fails, stream connection will retry on next reconnect
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
    const id = getChatBusId()
    try {
      await unsubscribeChatEventBus(id, chatId)
    } catch {
      // Non-fatal if already cleaned up on server
    }
  }
}

// closeChatBus tears down the SSE connection and clears all subscriptions.
export function closeChatBus(store?: ChatStreamStore) {
  clearChatBusRepair()
  hasRepaired = false

  if (store) {
    for (const [chatId, stores] of subscribedChats.entries()) {
      if (stores.has(store)) {
        stores.delete(store)
        store.streamStatus = { ...store.streamStatus, [chatId]: 'idle' }
        if (stores.size === 0) {
          subscribedChats.delete(chatId)
        }
      }
    }
  } else {
    subscribedChats.clear()
  }

  if (!store || subscribedChats.size === 0) {
    if (busSource) {
      busSource.close()
      busSource = null
    }
    currentBusStatus = 'idle'
  }
}
