// SharedWorker for multiplexed Server-Sent Events across browser tabs.
// It maintains exactly ONE EventSource connection to /api/events/bus/:uuid
// and dispatches incoming envelopes to all connected browser tabs.

import { BUS_EVENT_TYPES } from './event-types'

const ports = new Set<MessagePort>()
let eventSource: EventSource | null = null
let currentBusId: string = ''
let connectionStatus: 'idle' | 'connecting' | 'open' | 'error' | 'reconnecting' = 'idle'
let closeTimer: ReturnType<typeof setTimeout> | null = null

// Stall repair: a SharedWorker must not depend on the browser's implicit
// EventSource auto-retry alone. When a connection errors or drops, it is
// force-redialed after repairMs — the status broadcast flows through the
// main-thread client as "reconnecting" → "open", which is exactly what
// triggers the reconnectListeners (re-subscribe with lastEventId → server
// replay) chain. Without this, one CLOSED connection kills live output for
// every tab until a manual page refresh.
const repairMs = 3000
let repairTimer: ReturnType<typeof setTimeout> | null = null

function clearRepair() {
  if (repairTimer) {
    clearTimeout(repairTimer)
    repairTimer = null
  }
}

function scheduleRepair() {
  if (repairTimer || ports.size === 0) return
  repairTimer = setTimeout(() => {
    repairTimer = null
    if (ports.size === 0 || !currentBusId) return
    if (eventSource && eventSource.readyState === EventSource.OPEN) {
      connectionStatus = 'open'
      broadcastToAll({ type: 'status', status: connectionStatus, busId: currentBusId })
      return
    }
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    ensureEventSource(currentBusId)
  }, repairMs)
}

function broadcastToAll(message: unknown) {
  for (const port of ports) {
    try {
      port.postMessage(message)
    } catch {
      ports.delete(port)
    }
  }
}

function ensureEventSource(busIdToUse: string) {
  clearRepair()
  if (closeTimer) {
    clearTimeout(closeTimer)
    closeTimer = null
  }

  if (eventSource && eventSource.readyState !== EventSource.CLOSED) {
    return
  }

  if (!currentBusId) {
    return
  }

  connectionStatus = 'connecting'
  broadcastToAll({ type: 'status', status: connectionStatus, busId: currentBusId })

  const es = new EventSource(`/api/events/bus/${encodeURIComponent(currentBusId)}`)
  eventSource = es

  es.onopen = () => {
    clearRepair()
    connectionStatus = 'open'
    broadcastToAll({ type: 'status', status: connectionStatus, busId: currentBusId })
  }

  es.onerror = () => {
    if (es.readyState === EventSource.CLOSED) {
      // A CLOSED source never retries by itself: drop it so the repair
      // timer redials. The status stays 'reconnecting' (never a terminal
      // 'error') so the main-thread client treats the next 'open' as a
      // reconnect and re-subscribes every session with its last sequence.
      eventSource = null
    }
    connectionStatus = 'reconnecting'
    broadcastToAll({ type: 'status', status: connectionStatus, busId: currentBusId })
    scheduleRepair()
  }

  // Forward all received envelopes to all connected tabs without dropping frames.
  // The tabs themselves do precise local filtering by channel and targetId.
  const handleIncomingFrame = (event: MessageEvent) => {
    try {
      const envelope = JSON.parse(event.data)
      broadcastToAll({ type: 'event', envelope })
    } catch {
      // Ignore unparseable
    }
  }

  for (const evtType of BUS_EVENT_TYPES) {
    es.addEventListener(evtType, handleIncomingFrame)
  }
}

function handleTabDisconnect(port: MessagePort) {
  ports.delete(port)

  if (ports.size === 0) {
    // If no tabs remain, schedule disconnect after 5 seconds to free backend resources
    closeTimer = setTimeout(() => {
      clearRepair()
      closeTimer = null
      if (ports.size === 0 && eventSource) {
        eventSource.close()
        eventSource = null
        connectionStatus = 'idle'
      }
    }, 5000)
  }
}

// Global scope in SharedWorker
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const ctx = self as any

ctx.onconnect = (event: MessageEvent) => {
  const port = event.ports[0]
  if (!port) return

  ports.add(port)

  port.onmessage = (msgEvent: MessageEvent) => {
    const data = msgEvent.data as
      | { type: 'init_bus'; busId: string }
      | { type: 'ping' }
      | { type: 'disconnect' }

    if (!data || !data.type) return

    if (data.type === 'init_bus') {
      // Bus ownership arbitration in the worker: the first tab whose init
      // reaches the (shared) worker establishes the bus; every later tab
      // adopts the winner's busId through the init reply. Without this, each
      // tab's own sessionStorage busId would fight over the single shared
      // EventSource (last-init wins) and leave earlier tabs' subscriptions
      // on a bus that no SSE connection drains — their frames silently
      // overflow and drop server-side.
      if (data.busId && !currentBusId) {
        currentBusId = data.busId
        ensureEventSource(data.busId)
      }
      port.postMessage({ type: 'init', busId: currentBusId, status: connectionStatus })
    } else if (data.type === 'disconnect') {
      handleTabDisconnect(port)
    }
  }

  port.start()
}
