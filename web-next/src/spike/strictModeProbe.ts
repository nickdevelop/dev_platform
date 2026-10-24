interface ProbeConnection {
  id: number
  listeners: Set<() => void>
}

let connection: ProbeConnection | null = null
let totalConnections = 0

export interface ProbeSnapshot {
  totalConnections: number
  liveListeners: number
  connectionId: number | null
}

// A deliberately window-level singleton, shaped like EventBusClient: acquiring
// a listener must reuse the one connection rather than open a new one. React
// StrictMode's setup -> cleanup -> setup sequence then leaves one live listener
// and, critically, one total connection.
export function acquireProbe(listener: () => void): () => void {
  if (!connection) {
    connection = { id: ++totalConnections, listeners: new Set() }
  }
  connection.listeners.add(listener)
  listener()
  return () => {
    connection?.listeners.delete(listener)
  }
}

export function probeSnapshot(): ProbeSnapshot {
  return {
    totalConnections,
    liveListeners: connection?.listeners.size ?? 0,
    connectionId: connection?.id ?? null,
  }
}

export function resetProbe(): void {
  connection = null
  totalConnections = 0
}
