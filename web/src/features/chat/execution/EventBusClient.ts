// EventBusClient owns the single multiplexed SSE connection for the browser window
// and dispatches incoming events to their target sessions in the store.

import {
  eventBusPath,
  subscribeEventBusSession,
  unsubscribeEventBusSession,
  type SubscriptionSnapshot,
} from '../../../api/events'
import type { StreamStatus } from '../state/sessionLog'
import type { StreamStore } from './SessionEventStream'
import { consumeEventFrame } from './SessionEventStream'

let busId: string = ''

export function getBusId(): string {
  if (!busId) {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      busId = crypto.randomUUID()
    } else {
      busId = `bus-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
    }
  }
  return busId
}

export function setBusId(id: string) {
  busId = id
}

export function resetBusId() {
  busId = ''
}

// Global handle for the window-level EventSource bus.
let busSource: EventSource | null = null
let currentBusStatus: StreamStatus = 'idle'

// Map of subscribed sessions: sessionId -> projectId. The CSRF token is not
// cached here any more: the transport reads the current one when a
// subscription (re)connects, so a stale token can never be replayed.
const subscribedSessions = new Map<string, { projectId: number }>()

// Last event sequence applied per session
const lastEventIds = new Map<string, string>()

export function getSubscribedSessionIds(): string[] {
  return [...subscribedSessions.keys()]
}

export function hasSubscribedSession(sessionId: string): boolean {
  return subscribedSessions.has(sessionId)
}

export function getBusStatus(): StreamStatus {
  return currentBusStatus
}

export function forgetSessionResumePosition(sessionId: string) {
  lastEventIds.delete(sessionId)
}

export function clearAllResumePositions() {
  lastEventIds.clear()
}

// resubscribeActiveSessions re-registers all active sessions after a bus reconnect.
async function resubscribeActiveSessions(store: StreamStore) {
  const currentSubs = Array.from(subscribedSessions.entries())
  for (const [sid, { projectId }] of currentSubs) {
    let lastEventIdNum: number | undefined
    const lastIdStr = lastEventIds.get(sid)
    if (lastIdStr) {
      const parsed = parseInt(lastIdStr, 10)
      if (!isNaN(parsed) && parsed > 0) {
        lastEventIdNum = parsed
      }
    }
    const id = getBusId()
    try {
      const snapshot = await subscribeEventBusSession(id, projectId, sid, lastEventIdNum)
      if (snapshot.turn) {
        store.applyTurn(sid, snapshot.turn)
      }
      if (snapshot.interactions) {
        store.applyInteractionSnapshot(sid, snapshot.interactions)
      }
      if (snapshot.resync) {
        void store.rebuildFromHistory(projectId, sid)
      }
    } catch {
      // Re-registration may fail if backend is still recovering; stream status will stay reconnecting
    }
  }
}

// Stall repair: one automatic redial after 3s if reconnecting stalls
const stallMs = 3000
let repairTimer: ReturnType<typeof setTimeout> | null = null
let hasRepaired = false

export function clearBusRepair() {
  if (repairTimer) {
    clearTimeout(repairTimer)
    repairTimer = null
  }
}

// ensureBusConnection opens the window's multiplexed SSE connection if not yet active.
export function ensureBusConnection(store: StreamStore): EventSource {
  if (busSource && busSource.readyState !== EventSource.CLOSED) {
    if (currentBusStatus === 'reconnecting') {
      busSource.close()
      busSource = null
    } else {
      return busSource
    }
  }

  const id = getBusId()
  const source = new EventSource(eventBusPath(id))
  busSource = source
  currentBusStatus = 'connecting'

  // Update connecting state on all registered sessions
  for (const sid of subscribedSessions.keys()) {
    store.streamStatus[sid] = 'connecting'
  }

  source.onopen = () => {
    clearBusRepair()
    hasRepaired = false
    currentBusStatus = 'open'
    for (const sid of subscribedSessions.keys()) {
      store.streamStatus[sid] = 'open'
      const sub = subscribedSessions.get(sid)
      if (sub !== undefined) {
        void store.repairDanglingEntries(sub.projectId, sid)
      }
    }
    // Reconnected with existing active subscriptions: resync them with backend
    if (subscribedSessions.size > 0) {
      void resubscribeActiveSessions(store)
    }
  }

  source.onerror = () => {
    if (source.readyState === EventSource.CLOSED) {
      currentBusStatus = 'error'
      busSource = null
      clearBusRepair()
      for (const sid of subscribedSessions.keys()) {
        store.streamStatus[sid] = 'error'
      }
    } else {
      currentBusStatus = 'reconnecting'
      for (const sid of subscribedSessions.keys()) {
        store.streamStatus[sid] = 'reconnecting'
      }
      if (hasRepaired || repairTimer) return
      repairTimer = setTimeout(() => {
        repairTimer = null
        if (currentBusStatus !== 'reconnecting') return
        hasRepaired = true
        if (busSource) {
          busSource.close()
          busSource = null
        }
        ensureBusConnection(store)
      }, stallMs)
    }
  }

  // Listen for the initial ready frame
  source.addEventListener('ready', () => {
    clearBusRepair()
    hasRepaired = false
    currentBusStatus = 'open'
    for (const sid of subscribedSessions.keys()) {
      store.streamStatus[sid] = 'open'
    }
  })

  // Listen for interaction snapshot frames
  source.addEventListener('interactions', (event) => {
    const frame = event as MessageEvent
    try {
      const data = JSON.parse(frame.data) as { sessionId?: string; permissions?: unknown[]; forms?: unknown[] }
      let targetId = data.sessionId
      if (!targetId) {
        // If snapshot has no sessionId wrapper, apply to active session or the single subscribed session
        if (subscribedSessions.size === 1) {
          targetId = subscribedSessions.keys().next().value
        }
      }
      if (targetId && subscribedSessions.has(targetId)) {
        store.applyInteractionSnapshot(targetId, data as any)
      }
    } catch {
      // Ignore unparseable snapshot frame
    }
  })

  // Listen for session-targeted resync events
  source.addEventListener('resync', (event) => {
    const frame = event as MessageEvent
    try {
      const data = JSON.parse(frame.data) as { sessionId?: string }
      let targetId = data.sessionId
      if (!targetId && subscribedSessions.size === 1) {
        targetId = subscribedSessions.keys().next().value
      }
      if (targetId && subscribedSessions.has(targetId)) {
        const sub = subscribedSessions.get(targetId)
        if (sub !== undefined) {
          void store.rebuildFromHistory(sub.projectId, targetId)
        }
      }
    } catch {
      // Ignore unparseable resync frame
    }
  })

  // Dispatch all normalized event frames to their respective sessions
  for (const type of [
    'lifecycle',
    'message',
    'thinking',
    'tool_call',
    'tool_result',
    'error',
    'turn',
    'interaction',
    'unknown',
  ]) {
    source.addEventListener(type, (event) => {
      const frame = event as MessageEvent
      let targetSessionId = ''
      let sequenceNum: number | undefined
      try {
        const parsed = JSON.parse(frame.data) as { sessionId?: string; sequence?: number }
        if (parsed.sessionId) {
          targetSessionId = parsed.sessionId
        }
        if (typeof parsed.sequence === 'number' && parsed.sequence > 0) {
          sequenceNum = parsed.sequence
        }
      } catch {
        return
      }

      if (!targetSessionId || !subscribedSessions.has(targetSessionId)) {
        return
      }

      if (frame.lastEventId) {
        lastEventIds.set(targetSessionId, frame.lastEventId)
      } else if (sequenceNum !== undefined) {
        lastEventIds.set(targetSessionId, String(sequenceNum))
      }

      consumeEventFrame(store, targetSessionId, frame.data)
    })
  }

  return source
}

// subscribeSessionOnBus registers a session on the bus and syncs its initial snapshot.
export async function subscribeSessionOnBus(
  store: StreamStore,
  projectId: number,
  sessionId: string,
): Promise<SubscriptionSnapshot> {
  ensureBusConnection(store)

  subscribedSessions.set(sessionId, { projectId })
  store.streamStatus[sessionId] = currentBusStatus === 'open' ? 'open' : 'connecting'

  let lastEventIdNum: number | undefined
  const lastIdStr = lastEventIds.get(sessionId)
  if (lastIdStr) {
    const parsed = parseInt(lastIdStr, 10)
    if (!isNaN(parsed) && parsed > 0) {
      lastEventIdNum = parsed
    }
  }

  const id = getBusId()
  const snapshot = await subscribeEventBusSession(id, projectId, sessionId, lastEventIdNum)

  // Apply the authoritative initial snapshot
  if (snapshot.turn) {
    store.applyTurn(sessionId, snapshot.turn)
  }
  if (snapshot.interactions) {
    store.applyInteractionSnapshot(sessionId, snapshot.interactions)
  }
  if (snapshot.resync) {
    void store.rebuildFromHistory(projectId, sessionId)
  }

  return snapshot
}

// resubscribeSessionOnBus forces a fresh connection if stalled and re-subscribes.
export async function resubscribeSessionOnBus(
  store: StreamStore,
  projectId: number,
  sessionId: string,
): Promise<SubscriptionSnapshot> {
  if (busSource && (busSource.readyState !== EventSource.OPEN || currentBusStatus === 'reconnecting')) {
    busSource.close()
    busSource = null
  }
  return subscribeSessionOnBus(store, projectId, sessionId)
}

// unsubscribeSessionFromBus removes a session from the bus.
export async function unsubscribeSessionFromBus(
  store: StreamStore,
  sessionId: string,
): Promise<void> {
  if (!subscribedSessions.has(sessionId)) return

  subscribedSessions.delete(sessionId)
  store.streamStatus[sessionId] = 'idle'

  const id = getBusId()
  try {
    await unsubscribeEventBusSession(id, sessionId)
  } catch {
    // Non-fatal if already cleaned up on server
  }
}

// closeBus tears down the SSE connection and clears all subscriptions.
export function closeBus(store?: StreamStore) {
  clearBusRepair()
  hasRepaired = false
  if (busSource) {
    busSource.close()
    busSource = null
  }
  currentBusStatus = 'idle'
  if (store) {
    for (const sid of subscribedSessions.keys()) {
      store.streamStatus[sid] = 'idle'
    }
  }
  subscribedSessions.clear()
}
