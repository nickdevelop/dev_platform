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

const stallMs = 3000

export class UnifiedEventBusClient {
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

  // Stall repair timer
  private repairTimer: ReturnType<typeof setTimeout> | null = null
  private hasRepaired = false

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
    this.close()
    this.activeSubs.clear()
    this.handlers.clear()
  }

  public getStatus(): StreamStatus {
    return this.status
  }

  public clearRepair() {
    if (this.repairTimer) {
      clearTimeout(this.repairTimer)
      this.repairTimer = null
    }
  }

  public ensureConnection(): EventSource | null {
    this.ensureTransport()
    return this.localSource
  }

  public close(): void {
    this.clearRepair()
    this.hasRepaired = false
    this.handlers.clear()
    this.activeSubs.clear()
    if (this.localSource) {
      this.localSource.close()
      this.localSource = null
    }
    if (this.workerPort) {
      try {
        this.workerPort.postMessage({ type: 'disconnect' })
      } catch {
        // Ignore
      }
    }
    this.hasConnected = false
    this.setStatus('idle')
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

  public setStatus(newStatus: StreamStatus) {
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
      this.clearRepair()
      this.hasRepaired = false
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

  private ensureTransport() {
    if (this.workerPort) return
    if (this.localSource && this.localSource.readyState !== EventSource.CLOSED) {
      if (this.status === 'reconnecting') {
        // Stale reconnecting connection: drop and re-dial fresh
        this.localSource.close()
        this.localSource = null
      } else {
        return
      }
    }

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
      if (this.status === 'reconnecting') {
        this.localSource.close()
        this.localSource = null
      } else {
        return
      }
    }

    const id = this.getBusId()
    this.setStatus('connecting')

    const es = new EventSource(eventBusPath(id))
    this.localSource = es

    es.onopen = () => {
      this.clearRepair()
      this.hasRepaired = false
      this.setStatus('open')
    }

    es.onerror = () => {
      if (es.readyState === EventSource.CLOSED) {
        this.clearRepair()
        this.setStatus('error')
        this.localSource = null
      } else {
        this.setStatus('reconnecting')
        if (this.hasRepaired || this.repairTimer) return
        this.repairTimer = setTimeout(() => {
          this.repairTimer = null
          if (this.status !== 'reconnecting') return
          this.hasRepaired = true
          if (this.localSource) {
            this.localSource.close()
            this.localSource = null
          }
          this.ensureTransport()
        }, stallMs)
      }
    }

    es.addEventListener('ready', () => {
      this.clearRepair()
      this.hasRepaired = false
      this.setStatus('open')
    })

    const knownEvents = [
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
      'unknown',
      'lifecycle',
    ]

    for (const evt of knownEvents) {
      es.addEventListener(evt, (e) => {
        try {
          const raw = JSON.parse(e.data)
          if (raw && typeof raw === 'object' && 'channel' in raw && 'targetId' in raw) {
            this.dispatchEnvelope(raw as EventEnvelope)
          } else {
            this.dispatchRawFrame(evt, raw, (e as any).lastEventId)
          }
        } catch {
          // Ignore
        }
      })
    }

    es.onmessage = (e) => {
      try {
        const raw = JSON.parse(e.data)
        if (raw && typeof raw === 'object' && 'channel' in raw && 'targetId' in raw) {
          this.dispatchEnvelope(raw as EventEnvelope)
        } else {
          this.dispatchRawFrame('message', raw, (e as any).lastEventId)
        }
      } catch {
        // Ignore
      }
    }
  }

  private dispatchRawFrame(evtType: string, raw: any, lastEventId?: string) {
    if (raw && typeof raw.chatId === 'number') {
      this.dispatchEnvelope({
        channel: 'chat',
        targetId: String(raw.chatId),
        eventType: raw.type || evtType,
        timestamp: Date.now(),
        payload: raw,
      })
      return
    }

    if (raw && typeof raw.sessionId === 'string') {
      this.dispatchEnvelope({
        id: lastEventId ? parseInt(lastEventId, 10) : raw.sequence,
        channel: 'sessions',
        targetId: raw.sessionId,
        eventType: raw.type || evtType,
        timestamp: Date.now(),
        payload: raw,
      })
      return
    }

    // Broadcast frames (e.g. resync or interactions with no target wrapper)
    for (const subKey of this.activeSubs) {
      const [channel, targetId] = subKey.split(':')
      this.dispatchEnvelope({
        channel,
        targetId,
        eventType: evtType,
        timestamp: Date.now(),
        payload: raw,
      })
    }
  }

  public dispatchEnvelope(env: EventEnvelope) {
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

  public async subscribe<T = unknown>(
    channel: string,
    targetId: string,
    onEvent: EventHandler,
    options?: { projectId?: number; lastEventId?: number; extra?: unknown },
  ): Promise<T | undefined> {
    const key = `${channel}:${targetId}`

    if (!this.handlers.has(key)) {
      this.handlers.set(key, new Set())
    }
    this.handlers.get(key)!.add(onEvent)

    this.ensureTransport()

    if (this.workerPort) {
      this.workerPort.postMessage({
        type: 'register_sub',
        channel,
        targetId,
      })
    }

    const id = this.getBusId()
    const body: SubscribeBusBody = {
      channel,
      targetId,
      projectId: options?.projectId,
      lastEventId: options?.lastEventId,
      extra: options?.extra,
    }

    this.activeSubs.add(key)
    const res = await subscribeEventBus<any>(id, body)
    const anyRes = res as any
    const snapshot = res?.snapshot ?? (anyRes?.conversation || anyRes?.turn ? anyRes : undefined)
    return snapshot as T
  }

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
