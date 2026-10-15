// The chat feature's execution layer: one module owns every SSE connection of
// the workbench and routes normalized event frames into the session store.
// Keeping the live EventSource handles outside reactive state and outside the
// store keeps browser resources out of Vue's proxies; the store stays pure
// data (features/chat/state) and this module owns the event consumption.

import {
  sessionEventsPath,
  type AgentSession,
  type MessageModel,
  type MessageUsage,
  type NormalizedEvent,
} from '../../../api/sessions'
import type { ChatEntry, StreamStatus } from '../state/sessionLog'
import { entryFromEvent } from '../state/sessionLog'

// The slice of the session store the execution layer drives. A structural
// interface instead of importing the store keeps the graph acyclic: the store
// imports this module for its actions, and this module never imports it at
// runtime.
export interface StreamStore {
  streamStatus: Record<string, StreamStatus>
  messages: Record<string, ChatEntry[]>
  historyTailGuard: Record<string, boolean>
  echoPending: Record<string, string>
  sessionById: (sessionId: string) => AgentSession | null
  pushEntry: (sessionId: string, entry: ChatEntry) => void
  setTurn: (sessionId: string, turn: AgentSession['turn']) => void
  settleStreaming: (sessionId: string) => void
  subscribe: (projectId: number, sessionId: string) => void
  appendMessage: (
    sessionId: string,
    role: string,
    text: string,
    final: boolean,
    sequence: number,
    model?: MessageModel,
    usage?: MessageUsage,
  ) => void
  appendThinking: (sessionId: string, text: string, final: boolean, sequence: number) => void
  applyLifecycle: (sessionId: string, event: NormalizedEvent) => void
  repairDanglingEntries: (projectId: number, sessionId: string) => Promise<void>
  rebuildFromHistory: (projectId: number, sessionId: string) => Promise<void>
}

// Live EventSource handles are deliberately kept outside reactive state: they
// are browser resources, not view data, and must not be proxied by Vue.
const sources = new Map<string, EventSource>()

// The id of the last SSE frame applied per session. The store re-dials with
// it (as a query parameter the server turns into a resume position) so a
// manually re-opened EventSource replays exactly what it missed; the
// browser's own reconnects carry the id as the Last-Event-ID header.
const lastEventIds = new Map<string, string>()

// A dropped EventSource is not always re-established by the browser: when the
// process behind a session dies, it fires one error and then sits in CONNECTING
// without dialling again — measured over 24s with nothing reaching the server.
// The store therefore re-dials once. `repairs` holds the pending attempt, and
// `repaired` marks a session that already used its one attempt, so a service an
// operator stopped on purpose is not reached for on a loop.
const stallMs = 3000
const repairs = new Map<string, ReturnType<typeof setTimeout>>()
const repaired = new Set<string>()

export function clearStreamRepair(sessionId: string) {
  const timer = repairs.get(sessionId)
  if (timer) clearTimeout(timer)
  repairs.delete(sessionId)
}

export function hasOpenStream(sessionId: string): boolean {
  return sources.has(sessionId)
}

// forgetResumePosition drops the resume marker when the log itself is rebuilt
// (a deleted or re-replayed session): the next dial starts from the top.
export function forgetResumePosition(sessionId: string) {
  lastEventIds.delete(sessionId)
}

export function clearResumePositions() {
  lastEventIds.clear()
}

export function openStreamIds(): string[] {
  return [...sources.keys()]
}

// subscribeSession opens (or re-opens) a session's normalized event stream. A
// second call for the same session is a no-op, so a view re-render never opens
// a duplicate stream. A stream that has reported an error is the exception:
// after a drop the browser may sit in CONNECTING forever without dialling
// again — observed when the process behind a session dies — so the session
// would stay at 正在重连 for the rest of the session.
export function subscribeSession(store: StreamStore, projectId: number, sessionId: string) {
  const existing = sources.get(sessionId)
  if (existing) {
    if (store.streamStatus[sessionId] !== 'reconnecting') return
    existing.close()
    sources.delete(sessionId)
  }
  store.streamStatus[sessionId] = 'connecting'
  const source = new EventSource(sessionEventsPath(projectId, sessionId, lastEventIds.get(sessionId)))
  sources.set(sessionId, source)

  source.onopen = () => {
    clearStreamRepair(sessionId)
    repaired.delete(sessionId)
    store.streamStatus[sessionId] = 'open'
    void store.repairDanglingEntries(projectId, sessionId)
  }
  source.onerror = () => {
    if (source.readyState === EventSource.CLOSED) {
      store.streamStatus[sessionId] = 'error'
      sources.delete(sessionId)
    } else {
      store.streamStatus[sessionId] = 'reconnecting'
      // One repair, no more. The browser may simply stop dialling, and a
      // conversation that went quiet with the user watching it has no other
      // way back; but an operator who stopped a service on purpose must not
      // find the platform reaching for it on a loop, so a second stall is
      // the user's decision, made with the reconnect button.
      if (repaired.has(sessionId) || repairs.has(sessionId)) return
      repairs.set(
        sessionId,
        setTimeout(() => {
          repairs.delete(sessionId)
          if (store.streamStatus[sessionId] !== 'reconnecting') return
          repaired.add(sessionId)
          store.subscribe(projectId, sessionId)
        }, stallMs),
      )
    }
  }
  for (const type of ['lifecycle', 'message', 'thinking', 'tool_call', 'tool_result', 'error', 'unknown']) {
    source.addEventListener(type, (event) => {
      const frame = event as MessageEvent
      // Remember the frame's position so a later manual re-dial resumes
      // from it; the browser's own reconnects carry it automatically.
      if (frame.lastEventId) lastEventIds.set(sessionId, frame.lastEventId)
      consumeEventFrame(store, sessionId, frame.data)
    })
  }
  // The server sends this when a resumed stream could not cover the
  // client's gap (its replay buffer evicted the events or the stream was
  // recreated since): the log on screen may be missing its middle, so it
  // is rebuilt from history while the live stream keeps flowing.
  source.addEventListener('resync', () => {
    void store.rebuildFromHistory(projectId, sessionId)
  })
}

export function closeSessionStream(store: StreamStore, sessionId: string) {
  const source = sources.get(sessionId)
  clearStreamRepair(sessionId)
  repaired.delete(sessionId)
  if (source) {
    source.close()
    sources.delete(sessionId)
  }
  store.streamStatus[sessionId] = 'idle'
}

export function closeAllSessionStreams(store: StreamStore) {
  for (const sessionId of openStreamIds()) closeSessionStream(store, sessionId)
}

// consumeEventFrame applies one normalized event to its owning session.
// Events addressed to any other instance or session are dropped, which is
// what keeps instances isolated in the UI.
export function consumeEventFrame(store: StreamStore, sessionId: string, data: string) {
  let event: NormalizedEvent
  try {
    event = JSON.parse(data) as NormalizedEvent
  } catch {
    return
  }
  const session = store.sessionById(sessionId)
  if (!session) return
  if (event.instanceId && event.instanceId !== session.instanceId) return
  if (event.sessionId && event.sessionId !== sessionId) return

  if (event.type === 'message') {
    if (!event.message) return
    if (consumeHistoryGuard(store, sessionId, event.message.role, event.message.text)) return
    if (event.message.role === 'user' && store.echoPending[sessionId] === event.message.text) {
      // The echo of the turn the workbench sent itself: already shown.
      delete store.echoPending[sessionId]
      return
    }
    if (event.message.role !== 'user') {
      // The agent answered; no echo can still be pending for this text.
      delete store.echoPending[sessionId]
    }
    store.appendMessage(
      sessionId,
      event.message.role,
      event.message.text,
      event.message.final,
      event.sequence,
      event.message.model,
      event.message.usage,
    )
    return
  }
  if (event.type === 'thinking') {
    if (!event.thinking) return
    store.appendThinking(sessionId, event.thinking.text, event.thinking.final, event.sequence)
    return
  }
  if (event.type === 'lifecycle') {
    store.applyLifecycle(sessionId, event)
    return
  }
  const entry = entryFromEvent(sessionId, event)
  if (!entry) return
  store.pushEntry(sessionId, entry)
  // These are the backend's turn-ending error codes; any other error is
  // reported without concluding the turn.
  if (event.type === 'error' && event.error) {
    if (event.error.code === 'execution_failed' || event.error.code === 'process_exit') {
      store.setTurn(sessionId, 'idle')
      store.settleStreaming(sessionId)
    }
  }
}

// consumeHistoryGuard reports whether a live message is a re-delivery of
// the line history ended with, which the backend may send once right after
// a replay. The guard is spent on the first live message either way, so a
// genuinely repeated sentence later in the conversation is never dropped.
function consumeHistoryGuard(store: StreamStore, sessionId: string, role: string, text: string): boolean {
  if (!store.historyTailGuard[sessionId]) return false
  store.historyTailGuard[sessionId] = false
  const last = [...(store.messages[sessionId] ?? [])]
    .reverse()
    .find((entry) => entry.kind === 'message')
  return last !== undefined && last.role === role && last.text === text
}
