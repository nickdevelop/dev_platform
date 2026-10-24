// The chat feature's execution layer: one module owns every SSE connection of
// the workbench and routes normalized event frames into the session store.
// Keeping the live EventSource handles outside reactive state and outside the
// store keeps browser resources out of Vue's proxies; the store stays pure
// data (features/chat/state) and this module owns the event consumption.

import {
  clearAllResumePositions,
  closeBus,
  forgetSessionResumePosition,
  getSubscribedSessionIds,
  hasSubscribedSession,
  resubscribeSessionOnBus,
  subscribeSessionOnBus,
  unsubscribeSessionFromBus,
} from './EventBusClient'
import type {
  AgentSession,
  ImageAttachment,
  InteractionPayload,
  InteractionSnapshot,
  MessageModel,
  MessageUsage,
  NormalizedEvent,
  TurnPayload,
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
  setStreamStatus: (sessionId: string, status: StreamStatus) => void
  clearEchoPending: (sessionId: string) => void
  consumeHistoryTailGuard: (sessionId: string) => boolean
  sessionById: (sessionId: string) => AgentSession | null
  pushEntry: (sessionId: string, entry: ChatEntry) => void
  setTurn: (sessionId: string, turn: AgentSession['turn']) => void
  settleStreaming: (sessionId: string) => void
  applyTurn: (sessionId: string, turn: TurnPayload) => void
  applyInteraction: (sessionId: string, payload: InteractionPayload) => void
  applyInteractionSnapshot: (sessionId: string, snapshot: InteractionSnapshot) => void
  subscribe: (projectId: number, sessionId: string) => void
  appendMessage: (
    sessionId: string,
    role: string,
    text: string,
    final: boolean,
    sequence: number,
    model?: MessageModel,
    usage?: MessageUsage,
    images?: ImageAttachment[],
    timestamp?: string,
  ) => void
  appendThinking: (sessionId: string, text: string, final: boolean, sequence: number) => void
  applyLifecycle: (sessionId: string, event: NormalizedEvent) => void
  repairDanglingEntries: (projectId: number, sessionId: string) => Promise<void>
  rebuildFromHistory: (projectId: number, sessionId: string) => Promise<void>
}

export function clearStreamRepair(_sessionId: string) {
  // Bus manages connection health centrally at the window level
}

export function hasOpenStream(sessionId: string): boolean {
  return hasSubscribedSession(sessionId)
}

// forgetResumePosition drops the resume marker when the log itself is rebuilt
// (a deleted or re-replayed session): the next dial starts from the top.
export function forgetResumePosition(sessionId: string) {
  forgetSessionResumePosition(sessionId)
}

export function clearResumePositions() {
  clearAllResumePositions()
}

export function openStreamIds(): string[] {
  return getSubscribedSessionIds()
}

// subscribeSession delegates to the multiplexed event bus.
export function subscribeSession(store: StreamStore, projectId: number, sessionId: string) {
  return subscribeSessionOnBus(store, projectId, sessionId)
}

export function resubscribeSession(store: StreamStore, projectId: number, sessionId: string) {
  return resubscribeSessionOnBus(store, projectId, sessionId)
}

export function closeSessionStream(store: StreamStore, sessionId: string) {
  return unsubscribeSessionFromBus(store, sessionId)
}

export function closeAllSessionStreams(store: StreamStore) {
  closeBus(store)
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
      store.clearEchoPending(sessionId)
      return
    }
    if (event.message.role !== 'user') {
      // The agent answered; no echo can still be pending for this text.
      store.clearEchoPending(sessionId)
    }
    store.appendMessage(
      sessionId,
      event.message.role,
      event.message.text,
      event.message.final,
      event.sequence,
      event.message.model,
      event.message.usage,
      event.message.images,
      event.timestamp,
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
  if (event.type === 'turn') {
    if (event.turn) store.applyTurn(sessionId, event.turn)
    return
  }
  if (event.type === 'interaction') {
    if (event.interaction) store.applyInteraction(sessionId, event.interaction)
    return
  }
  const entry = entryFromEvent(sessionId, event)
  if (!entry) return
  store.pushEntry(sessionId, entry)
}

// consumeHistoryGuard reports whether a live message is a re-delivery of
// the line history ended with, which the backend may send once right after
// a replay. The guard is spent on the first live message either way, so a
// genuinely repeated sentence later in the conversation is never dropped.
function consumeHistoryGuard(store: StreamStore, sessionId: string, role: string, text: string): boolean {
  if (!store.consumeHistoryTailGuard(sessionId)) return false
  const last = [...(store.messages[sessionId] ?? [])]
    .reverse()
    .find((entry) => entry.kind === 'message')
  return last !== undefined && last.role === role && last.text === text
}
