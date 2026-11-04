// SharedWorker for multiplexed Server-Sent Events across browser tabs.
// It maintains exactly ONE EventSource connection to /api/events/bus/:uuid
// and dispatches incoming envelopes to all connected browser tabs.

const ports = new Set<MessagePort>()
let eventSource: EventSource | null = null
let currentBusId: string = ''
let connectionStatus: 'idle' | 'connecting' | 'open' | 'error' | 'reconnecting' = 'idle'
let closeTimer: ReturnType<typeof setTimeout> | null = null

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
  if (closeTimer) {
    clearTimeout(closeTimer)
    closeTimer = null
  }

  // If busId changed, close existing connection to reopen with the authoritative one
  if (busIdToUse && currentBusId && currentBusId !== busIdToUse && eventSource) {
    eventSource.close()
    eventSource = null
  }

  if (busIdToUse) {
    currentBusId = busIdToUse
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
    connectionStatus = 'open'
    broadcastToAll({ type: 'status', status: connectionStatus, busId: currentBusId })
  }

  es.onerror = () => {
    if (es.readyState === EventSource.CLOSED) {
      connectionStatus = 'error'
      eventSource = null
    } else {
      connectionStatus = 'reconnecting'
    }
    broadcastToAll({ type: 'status', status: connectionStatus, busId: currentBusId })
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

  const knownEventTypes = [
    'delta',
    'message',
    'thinking',
    'tool_call',
    'tool_result',
    'turn',
    'interactions',
    'interaction',
    'resync',
    'status_change',
    'step_update',
    'chunk',
    'status',
    'conversation',
    'snapshot',
    'message_update',
    'closed',
    'error',
    'ready',
  ]

  for (const evtType of knownEventTypes) {
    es.addEventListener(evtType, handleIncomingFrame)
  }
}

function handleTabDisconnect(port: MessagePort) {
  ports.delete(port)

  if (ports.size === 0) {
    // If no tabs remain, schedule disconnect after 5 seconds to free backend resources
    closeTimer = setTimeout(() => {
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
      if (data.busId) {
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
