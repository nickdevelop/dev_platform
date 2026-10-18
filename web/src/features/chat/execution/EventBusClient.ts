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

// Map of subscribed sessions: sessionId -> projectId
const subscribedSessions = new Map<string, number>()

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

// ensureBusConnection opens the window's multiplexed SSE connection if not yet active.
export function ensureBusConnection(store: StreamStore): EventSource {
  if (busSource && busSource.readyState !== EventSource.CLOSED) {
    return busSource
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
    currentBusStatus = 'open'
    for (const sid of subscribedSessions.keys()) {
      store.streamStatus[sid] = 'open'
      const projId = subscribedSessions.get(sid)
      if (projId !== undefined) {
        void store.repairDanglingEntries(projId, sid)
      }
    }
  }

  source.onerror = () => {
    if (source.readyState === EventSource.CLOSED) {
      currentBusStatus = 'error'
      busSource = null
      for (const sid of subscribedSessions.keys()) {
        store.streamStatus[sid] = 'error'
      }
    } else {
      currentBusStatus = 'reconnecting'
      for (const sid of subscribedSessions.keys()) {
        store.streamStatus[sid] = 'reconnecting'
      }
    }
  }

  // Listen for the initial ready frame
  source.addEventListener('ready', () => {
    currentBusStatus = 'open'
    for (const sid of subscribedSessions.keys()) {
      store.streamStatus[sid] = 'open'
    }
  })

  // Listen for session-targeted resync events
  source.addEventListener('resync', (event) => {
    const frame = event as MessageEvent
    try {
      const data = JSON.parse(frame.data) as { sessionId?: string }
      if (data.sessionId && subscribedSessions.has(data.sessionId)) {
        const projId = subscribedSessions.get(data.sessionId)
        if (projId !== undefined) {
          void store.rebuildFromHistory(projId, data.sessionId)
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
      try {
        const parsed = JSON.parse(frame.data) as { sessionId?: string }
        if (parsed.sessionId) {
          targetSessionId = parsed.sessionId
        }
      } catch {
        return
      }

      if (!targetSessionId || !subscribedSessions.has(targetSessionId)) {
        return
      }

      if (frame.lastEventId) {
        lastEventIds.set(targetSessionId, frame.lastEventId)
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
  csrfToken?: string,
): Promise<SubscriptionSnapshot> {
  ensureBusConnection(store)

  subscribedSessions.set(sessionId, projectId)
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
  const snapshot = await subscribeEventBusSession(id, projectId, sessionId, lastEventIdNum, csrfToken)

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

// unsubscribeSessionFromBus removes a session from the bus.
export async function unsubscribeSessionFromBus(
  store: StreamStore,
  sessionId: string,
  csrfToken?: string,
): Promise<void> {
  if (!subscribedSessions.has(sessionId)) return

  subscribedSessions.delete(sessionId)
  store.streamStatus[sessionId] = 'idle'

  const id = getBusId()
  try {
    await unsubscribeEventBusSession(id, sessionId, csrfToken)
  } catch {
    // Non-fatal if already cleaned up on server
  }
}

// closeBus tears down the SSE connection and clears all subscriptions.
export function closeBus(store?: StreamStore) {
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
