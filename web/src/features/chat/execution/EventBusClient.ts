// EventBusClient adapts workspace agent sessions to the single unified event bus (unifiedEventBus).
// It no longer maintains an independent EventSource or private busId.

import {
  type SubscriptionSnapshot,
} from '../../../api/events'
import type { StreamStatus } from '../state/sessionLog'
import type { StreamStore } from './SessionEventStream'
import { consumeEventFrame } from './SessionEventStream'
import { unifiedEventBus } from '../../../shared/events/UnifiedEventBusClient'
import type { EventEnvelope } from '../../../api/events'

export function getBusId(): string {
  return unifiedEventBus.getBusId()
}

export function setBusId(id: string) {
  unifiedEventBus.setBusId(id)
}

export function resetBusId() {
  unifiedEventBus.resetBusId()
  subscribedSessions.clear()
  sessionHandlers.clear()
  sessionStores.clear()
}

// Map of subscribed sessions: sessionId -> projectId.
const subscribedSessions = new Map<string, { projectId: number }>()
const sessionStores = new Map<string, StreamStore>()
const sessionHandlers = new Map<string, (env: EventEnvelope) => void>()

// Last event sequence applied per session
const lastEventIds = new Map<string, string>()

export function getSubscribedSessionIds(): string[] {
  return [...subscribedSessions.keys()]
}

export function hasSubscribedSession(sessionId: string): boolean {
  return subscribedSessions.has(sessionId)
}

export function getBusStatus(): StreamStatus {
  return unifiedEventBus.getStatus()
}

export function forgetSessionResumePosition(sessionId: string) {
  lastEventIds.delete(sessionId)
}

export function clearAllResumePositions() {
  lastEventIds.clear()
}

export function clearBusRepair() {
  // Maintained for backward compatibility
}

// ensureBusConnection delegates to the unified bus
export function ensureBusConnection(store?: StreamStore): EventSource | null {
  const conn = unifiedEventBus.ensureConnection()
  if (conn) {
    conn.addEventListener('resync', (event) => {
      const frame = event as MessageEvent
      try {
        const data = JSON.parse(frame.data) as { sessionId?: string }
        let targetId = data.sessionId
        if (!targetId && subscribedSessions.size === 1) {
          targetId = subscribedSessions.keys().next().value
        }
        if (targetId && subscribedSessions.has(targetId)) {
          const sub = subscribedSessions.get(targetId)
          const s = (targetId ? sessionStores.get(targetId) : undefined) || store
          if (sub !== undefined && s) {
            void s.rebuildFromHistory(sub.projectId, targetId)
          }
        }
      } catch {
        // Ignore unparseable resync frame
      }
    })
  }
  if (store) {
    for (const sid of subscribedSessions.keys()) {
      store.streamStatus[sid] = unifiedEventBus.getStatus() === 'open' ? 'open' : 'connecting'
    }
  }
  return conn
}

// Listen for unified bus status changes to update all registered sessions
unifiedEventBus.onStatusChange((status) => {
  for (const [sid] of subscribedSessions.entries()) {
    const store = sessionStores.get(sid)
    if (store) {
      store.streamStatus[sid] = status
    }
  }
})

// Listen for unified bus reconnect to resync active sessions (strategy A)
unifiedEventBus.onReconnect(() => {
  for (const [sid, sub] of subscribedSessions.entries()) {
    const store = sessionStores.get(sid)
    if (store) {
      void store.repairDanglingEntries(sub.projectId, sid)
      void resubscribeSessionOnBus(store, sub.projectId, sid)
    }
  }
})

function getOrCreateSessionHandler(
  sessionId: string,
  projectId: number,
  fallbackStore: StreamStore,
): (env: EventEnvelope) => void {
  let handler = sessionHandlers.get(sessionId)
  if (!handler) {
    handler = (env: EventEnvelope) => {
      const store = sessionStores.get(sessionId) || fallbackStore
      const raw = env.payload as any
      if (!raw) return

      const eventType = env.eventType || raw.type

      if (eventType === 'ready') {
        store.streamStatus[sessionId] = 'open'
        return
      }

      if (eventType === 'interactions') {
        store.applyInteractionSnapshot(sessionId, raw)
        return
      }

      if (eventType === 'resync') {
        void store.rebuildFromHistory(projectId, sessionId)
        return
      }

      if (env.id !== undefined && env.id > 0) {
        lastEventIds.set(sessionId, String(env.id))
      } else if (raw.sequence !== undefined && raw.sequence > 0) {
        lastEventIds.set(sessionId, String(raw.sequence))
      }

      // Serialize payload as event JSON frame to consumeEventFrame
      const rawFrame = typeof raw === 'string' ? raw : JSON.stringify(raw)
      consumeEventFrame(store, sessionId, rawFrame)
    }
    sessionHandlers.set(sessionId, handler)
  }
  return handler
}

// subscribeSessionOnBus registers a session on the unified bus and syncs its initial snapshot.
export async function subscribeSessionOnBus(
  store: StreamStore,
  projectId: number,
  sessionId: string,
): Promise<SubscriptionSnapshot> {
  ensureBusConnection(store)

  subscribedSessions.set(sessionId, { projectId })
  sessionStores.set(sessionId, store)
  store.streamStatus[sessionId] = unifiedEventBus.getStatus() === 'open' ? 'open' : 'connecting'

  let lastEventIdNum: number | undefined
  const lastIdStr = lastEventIds.get(sessionId)
  if (lastIdStr) {
    const parsed = parseInt(lastIdStr, 10)
    if (!isNaN(parsed) && parsed > 0) {
      lastEventIdNum = parsed
    }
  }

  const handler = getOrCreateSessionHandler(sessionId, projectId, store)

  try {
    const snapshot = await unifiedEventBus.subscribe<SubscriptionSnapshot>(
      'sessions',
      sessionId,
      handler,
      { projectId, lastEventId: lastEventIdNum },
    )

    // Apply the authoritative initial snapshot
    if (snapshot?.turn) {
      store.applyTurn(sessionId, snapshot.turn)
    }
    if (snapshot?.interactions) {
      store.applyInteractionSnapshot(sessionId, snapshot.interactions)
    }
    if (snapshot?.resync) {
      void store.rebuildFromHistory(projectId, sessionId)
    }

    return (
      snapshot || {
        sessionId,
        turn: null,
        interactions: { permissions: [], forms: [] },
        resync: false,
      }
    )
  } catch {
    return {
      sessionId,
      turn: null,
      interactions: { permissions: [], forms: [] },
      resync: false,
    }
  }
}

// resubscribeSessionOnBus forces a fresh connection if stalled and re-subscribes.
export async function resubscribeSessionOnBus(
  store: StreamStore,
  projectId: number,
  sessionId: string,
): Promise<SubscriptionSnapshot> {
  return subscribeSessionOnBus(store, projectId, sessionId)
}

// unsubscribeSessionFromBus removes a session from the unified bus.
export async function unsubscribeSessionFromBus(
  store: StreamStore,
  sessionId: string,
): Promise<void> {
  subscribedSessions.delete(sessionId)
  sessionStores.delete(sessionId)
  store.streamStatus[sessionId] = 'idle'

  const handler = sessionHandlers.get(sessionId)
  if (handler) {
    sessionHandlers.delete(sessionId)
    await unifiedEventBus.unsubscribe('sessions', sessionId, handler)
  }
}

// closeBus tears down the SSE connection and clears all session subscriptions.
export function closeBus(store?: StreamStore) {
  clearBusRepair()
  unifiedEventBus.close()
  if (store) {
    for (const sid of subscribedSessions.keys()) {
      store.streamStatus[sid] = 'idle'
    }
  }
  subscribedSessions.clear()
  sessionStores.clear()
  sessionHandlers.clear()
}
