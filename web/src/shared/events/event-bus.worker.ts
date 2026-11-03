// SharedWorker for multiplexed Server-Sent Events across browser tabs.
// It maintains exactly ONE EventSource connection to /api/events/bus/:uuid
// and dispatches incoming envelopes to connected browser tabs based on subscriptions.

interface TabSubscription {
  channel: string
  targetId: string
}

const ports = new Set<MessagePort>()
let eventSource: EventSource | null = null
let busId: string = ''
let connectionStatus: 'idle' | 'connecting' | 'open' | 'error' | 'reconnecting' = 'idle'
let closeTimer: ReturnType<typeof setTimeout> | null = null

// portToSubs maps each port to the set of "channel:targetId" strings it is listening to.
const portToSubs = new Map<MessagePort, Set<string>>()

function getOrCreateBusId(): string {
  if (!busId) {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      busId = crypto.randomUUID()
    } else {
      busId = `bus-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
    }
  }
  return busId
}

function broadcastToAll(message: unknown) {
  for (const port of ports) {
    try {
      port.postMessage(message)
    } catch {
      ports.delete(port)
      portToSubs.delete(port)
    }
  }
}

function broadcastEnvelope(envelope: { channel: string; targetId: string; eventType: string }) {
  const subKey = `${envelope.channel}:${envelope.targetId}`
  for (const [port, subs] of portToSubs.entries()) {
    if (envelope.channel === 'system' || subs.has(subKey) || subs.has(`${envelope.channel}:*`)) {
      try {
        port.postMessage({ type: 'event', envelope })
      } catch {
        ports.delete(port)
        portToSubs.delete(port)
      }
    }
  }
}

function ensureEventSource() {
  if (closeTimer) {
    clearTimeout(closeTimer)
    closeTimer = null
  }

  if (eventSource && eventSource.readyState !== EventSource.CLOSED) {
    return
  }

  const id = getOrCreateBusId()
  connectionStatus = 'connecting'
  broadcastToAll({ type: 'status', status: connectionStatus, busId: id })

  const es = new EventSource(`/api/events/bus/${encodeURIComponent(id)}`)
  eventSource = es

  es.onopen = () => {
    connectionStatus = 'open'
    broadcastToAll({ type: 'status', status: connectionStatus, busId: id })
  }

  es.onerror = () => {
    if (es.readyState === EventSource.CLOSED) {
      connectionStatus = 'error'
      eventSource = null
    } else {
      connectionStatus = 'reconnecting'
    }
    broadcastToAll({ type: 'status', status: connectionStatus, busId: id })
  }

  // Handle ready frame
  es.addEventListener('ready', (event) => {
    connectionStatus = 'open'
    broadcastToAll({ type: 'status', status: connectionStatus, busId: id })
    try {
      const data = JSON.parse(event.data)
      broadcastEnvelope(data)
    } catch {
      // Ignore
    }
  })

  // Listen to all standard event types or fallback to onmessage
  const knownEventTypes = [
    'delta',
    'message',
    'turn',
    'interactions',
    'resync',
    'status_change',
    'step_update',
    'chunk',
    'status',
  ]

  for (const evtType of knownEventTypes) {
    es.addEventListener(evtType, (event) => {
      try {
        const env = JSON.parse(event.data)
        broadcastEnvelope(env)
      } catch {
        // Ignore unparseable
      }
    })
  }

  es.onmessage = (event) => {
    try {
      const env = JSON.parse(event.data)
      broadcastEnvelope(env)
    } catch {
      // Ignore unparseable
    }
  }
}

function handleTabDisconnect(port: MessagePort) {
  ports.delete(port)
  portToSubs.delete(port)

  if (ports.size === 0) {
    // If no tabs remain, schedule disconnect after 5 seconds
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
  portToSubs.set(port, new Set())

  // Send initial busId and connection status to the newly connected tab
  const id = getOrCreateBusId()
  port.postMessage({ type: 'init', busId: id, status: connectionStatus })

  ensureEventSource()

  port.onmessage = (msgEvent: MessageEvent) => {
    const data = msgEvent.data as
      | { type: 'register_sub'; channel: string; targetId: string }
      | { type: 'unregister_sub'; channel: string; targetId: string }
      | { type: 'ping' }
      | { type: 'disconnect' }

    if (!data || !data.type) return

    if (data.type === 'register_sub') {
      const subs = portToSubs.get(port)
      if (subs) {
        subs.add(`${data.channel}:${data.targetId}`)
      }
    } else if (data.type === 'unregister_sub') {
      const subs = portToSubs.get(port)
      if (subs) {
        subs.delete(`${data.channel}:${data.targetId}`)
      }
    } else if (data.type === 'disconnect') {
      handleTabDisconnect(port)
    }
  }

  port.start()
}
