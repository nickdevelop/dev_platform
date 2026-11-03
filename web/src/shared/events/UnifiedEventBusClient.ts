// UnifiedEventBusClient manages the single multiplexed event stream for the application.
// In modern multi-tab browsers, it delegates the actual long-lived EventSource to a SharedWorker.
// If SharedWorker is unavailable (e.g. strict private mode or Vitest), it transparently falls back
// to maintaining a single EventSource in the main thread.

import {
  eventBusPath,
  subscribeEventBus,
  unsubscribeEventBus,
  type EventEnvelope,
  type SubscribeBusBody,
} from '../../api/events'

export type StreamStatus = 'idle' | 'connecting' | 'open' | 'error' | 'reconnecting'

export type EventHandler = (envelope: EventEnvelope) => void
export type StatusListener = (status: StreamStatus) => void
export type ReconnectListener = () => void

class UnifiedEventBusClient {
  private busId: string = ''
  private worker: SharedWorker | null = null
  private workerPort: MessagePort | null = null
  private localSource: EventSource | null = null

  private status: StreamStatus = 'idle'
  private statusListeners = new Set<StatusListener>()
  private reconnectListeners = new Set<ReconnectListener>()

  // Subscriptions registry in the current window: "channel:targetId" -> Set of callbacks
  private handlers = new Map<string, Set<EventHandler>>()

  // Track active channels/targets registered with the backend from this tab
  private activeSubs = new Set<string>()

  // Check whether we have already connected at least once to distinguish initial connect from reconnect
  private hasConnected = false

  constructor() {
    this.initTransport()
  }

  public getBusId(): string {
    if (!this.busId) {
      if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
        this.busId = crypto.randomUUID()
      } else {
        this.busId = `bus-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
      }
    }
    return this.busId
  }

  public setBusId(id: string) {
    this.busId = id
  }

  public resetBusId() {
    this.busId = ''
    this.hasConnected = false
    this.activeSubs.clear()
    this.handlers.clear()
    if (this.localSource) {
      this.localSource.close()
      this.localSource = null
    }
    this.status = 'idle'
  }

  public getStatus(): StreamStatus {
    return this.status
  }

  public onStatusChange(listener: StatusListener): () => void {
    this.statusListeners.add(listener)
    listener(this.status)
    return () => {
      this.statusListeners.delete(listener)
    }
  }

  public onReconnect(listener: ReconnectListener): () => void {
    this.reconnectListeners.add(listener)
    return () => {
      this.reconnectListeners.delete(listener)
    }
  }

  private setStatus(newStatus: StreamStatus) {
    if (this.status === newStatus) return
    const prevStatus = this.status
    this.status = newStatus

    for (const listener of this.statusListeners) {
      try {
        listener(newStatus)
      } catch {
        // Ignore listener error
      }
    }

    if (newStatus === 'open') {
      if (this.hasConnected && prevStatus === 'reconnecting') {
        // Reconnection occurred: trigger strategy A lightweight snapshot refresh
        for (const listener of this.reconnectListeners) {
          try {
            listener()
          } catch {
            // Ignore
          }
        }
      }
      this.hasConnected = true
    }
  }

  private initTransport() {
    // Only attempt SharedWorker in a standard browser window environment (skip under Vitest/Node)
    const isBrowser =
      typeof window !== 'undefined' &&
      typeof navigator !== 'undefined' &&
      !navigator.userAgent?.includes('Node.js') &&
      !navigator.userAgent?.includes('jsdom')

    const canUseSharedWorker = isBrowser && typeof SharedWorker !== 'undefined'

    if (canUseSharedWorker) {
      try {
        this.worker = new SharedWorker(
          new URL('./event-bus.worker.ts', import.meta.url),
          { type: 'module' },
        )
        const port = this.worker.port
        this.workerPort = port

        port.onmessage = (event: MessageEvent) => {
          const data = event.data as
            | { type: 'init'; busId: string; status: StreamStatus }
            | { type: 'status'; status: StreamStatus; busId: string }
            | { type: 'event'; envelope: EventEnvelope }

          if (!data || !data.type) return

          if (data.type === 'init') {
            if (data.busId) this.busId = data.busId
            this.setStatus(data.status)
          } else if (data.type === 'status') {
            if (data.busId) this.busId = data.busId
            this.setStatus(data.status)
          } else if (data.type === 'event') {
            this.dispatchEnvelope(data.envelope)
          }
        }

        port.start()

        // Clean up on tab close
        window.addEventListener('beforeunload', () => {
          port.postMessage({ type: 'disconnect' })
        })
        return
      } catch {
        // SharedWorker creation failed (e.g. sandbox restriction); fallback to local
        this.worker = null
        this.workerPort = null
      }
    }

    // Fallback: local EventSource inside main thread
    this.initLocalEventSource()
  }

  private initLocalEventSource() {
    if (typeof EventSource === 'undefined') return

    if (this.localSource && this.localSource.readyState !== EventSource.CLOSED) {
      return
    }

    const id = this.getBusId()
    this.setStatus('connecting')

    const es = new EventSource(eventBusPath(id))
    this.localSource = es

    es.onopen = () => {
      this.setStatus('open')
    }

    es.onerror = () => {
      if (es.readyState === EventSource.CLOSED) {
        this.setStatus('error')
        this.localSource = null
      } else {
        this.setStatus('reconnecting')
      }
    }

    es.addEventListener('ready', () => {
      this.setStatus('open')
    })

    const knownEvents = [
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

    for (const evt of knownEvents) {
      es.addEventListener(evt, (e) => {
        try {
          const parsed = JSON.parse(e.data) as EventEnvelope
          this.dispatchEnvelope(parsed)
        } catch {
          // Ignore
        }
      })
    }

    es.onmessage = (e) => {
      try {
        const parsed = JSON.parse(e.data) as EventEnvelope
        this.dispatchEnvelope(parsed)
      } catch {
        // Ignore
      }
    }
  }

  private dispatchEnvelope(env: EventEnvelope) {
    if (!env || !env.channel) return
    const key = `${env.channel}:${env.targetId}`
    const targetHandlers = this.handlers.get(key)
    if (targetHandlers) {
      for (const handler of targetHandlers) {
        try {
          handler(env)
        } catch {
          // Ignore handler exception
        }
      }
    }

    // Also dispatch to wildcard listeners if any
    const wildcardHandlers = this.handlers.get(`${env.channel}:*`)
    if (wildcardHandlers) {
      for (const handler of wildcardHandlers) {
        try {
          handler(env)
        } catch {
          // Ignore
        }
      }
    }
  }

  // subscribe attaches a channel & targetId, registers with backend and returns initial snapshot
  public async subscribe<T = unknown>(
    channel: string,
    targetId: string,
    onEvent: EventHandler,
    options?: { projectId?: number; lastEventId?: number; extra?: unknown },
  ): Promise<T | undefined> {
    const key = `${channel}:${targetId}`

    // 1. Register handler locally
    if (!this.handlers.has(key)) {
      this.handlers.set(key, new Set())
    }
    this.handlers.get(key)!.add(onEvent)

    // 2. Notify SharedWorker if present
    if (this.workerPort) {
      this.workerPort.postMessage({
        type: 'register_sub',
        channel,
        targetId,
      })
    } else {
      this.initLocalEventSource()
    }

    // 3. Register subscription with backend via standard transport (carrying CSRF & cookies)
    const id = this.getBusId()
    const body: SubscribeBusBody = {
      channel,
      targetId,
      projectId: options?.projectId,
      lastEventId: options?.lastEventId,
      extra: options?.extra,
    }

    this.activeSubs.add(key)
    const res = await subscribeEventBus<T>(id, body)
    return res.snapshot
  }

  // unsubscribe removes a handler and, if no listeners remain in this tab, detaches from backend
  public async unsubscribe(
    channel: string,
    targetId: string,
    onEvent?: EventHandler,
  ): Promise<void> {
    const key = `${channel}:${targetId}`
    if (onEvent) {
      const handlers = this.handlers.get(key)
      if (handlers) {
        handlers.delete(onEvent)
        if (handlers.size === 0) {
          this.handlers.delete(key)
        }
      }
    } else {
      this.handlers.delete(key)
    }

    // If no handlers left in this tab for this key, unregister from worker and backend
    if (!this.handlers.has(key)) {
      this.activeSubs.delete(key)
      if (this.workerPort) {
        this.workerPort.postMessage({
          type: 'unregister_sub',
          channel,
          targetId,
        })
      }
      const id = this.getBusId()
      try {
        await unsubscribeEventBus(id, channel, targetId)
      } catch {
        // Ignore unsubscription failure on teardown
      }
    }
  }
}

// Global singleton client for the application window
export const unifiedEventBus = new UnifiedEventBusClient()
