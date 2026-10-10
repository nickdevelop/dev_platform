// Pinia store for the unified session workbench. It owns the project's session
// index, the per-session message log and the per-session SSE subscription.
//
// Isolation is enforced here: a subscription is keyed by platform session id
// and an event is applied to a session only when its instanceId matches that
// session's instance (and, for session-scoped events, its sessionId matches).
// Events from one instance can therefore never render into another instance's
// session view.

import { defineStore } from 'pinia'
import {
  continueProjectSession,
  createProjectSession,
  discoverProjectInstanceSessions,
  getSessionHistory,
  interruptProjectSession,
  listProjectSessions,
  sendProjectSessionMessage,
  sessionEventsPath,
  stopProjectSession,
  type AgentSession,
  type CreateSessionBody,
  type NormalizedEvent,
} from '../api/sessions'
import { describeApiError, UnauthorizedError } from '../api/client'
import { useAuthStore } from './auth'

// One rendered line in a session's log. Agent output is only ever carried as
// text and interpolated, never injected as HTML. A `separator` line marks where
// replayed history ends and the live stream begins.
export interface ChatEntry {
  id: string
  kind: 'message' | 'tool_call' | 'tool_result' | 'error' | 'separator'
  role: string
  text: string
  streaming: boolean
  callId?: string
  name?: string
  isError?: boolean
}

export type StreamStatus = 'idle' | 'connecting' | 'open' | 'reconnecting' | 'error' | 'ended'

// Live EventSource handles are deliberately kept outside reactive state: they
// are browser resources, not view data, and must not be proxied by Vue.
const sources = new Map<string, EventSource>()

let entryCounter = 0

function nextEntryID(sessionId: string, sequence: number, kind: string): string {
  entryCounter += 1
  return `${sessionId}:${sequence}:${kind}:${entryCounter}`
}

// entryFromEvent maps one non-message event to its log line, or null when the
// event renders nothing (lifecycle events are handled separately because they
// carry no log line of their own). Both the live path and the history replay
// go through it, so a replayed tool call looks exactly like a live one.
function entryFromEvent(sessionId: string, event: NormalizedEvent): ChatEntry | null {
  switch (event.type) {
    case 'tool_call':
      if (!event.toolCall) return null
      return {
        id: nextEntryID(sessionId, event.sequence, 'tool_call'),
        kind: 'tool_call',
        role: 'assistant',
        text: event.toolCall.name,
        streaming: false,
        callId: event.toolCall.callId,
        name: event.toolCall.name,
      }
    case 'tool_result':
      if (!event.toolResult) return null
      return {
        id: nextEntryID(sessionId, event.sequence, 'tool_result'),
        kind: 'tool_result',
        role: 'tool',
        text: event.toolResult.output ?? '',
        streaming: false,
        callId: event.toolResult.callId,
        isError: event.toolResult.isError,
      }
    case 'error':
      if (!event.error) return null
      return {
        id: nextEntryID(sessionId, event.sequence, 'error'),
        kind: 'error',
        role: 'system',
        text: event.error.message,
        streaming: false,
      }
    default:
      return null
  }
}

export const useSessionsStore = defineStore('sessions', {
  state: () => ({
    sessions: [] as AgentSession[],
    projectId: null as number | null,
    messages: {} as Record<string, ChatEntry[]>,
    streamStatus: {} as Record<string, StreamStatus>,
    // History is loaded once per session: historyLoaded guards a second fetch,
    // historyError carries a retryable failure, historyTruncated records that
    // the backend only returned the most recent part of the transcript.
    historyLoaded: {} as Record<string, boolean>,
    historyError: {} as Record<string, string>,
    historyTruncated: {} as Record<string, boolean>,
    // historyTailGuard is true right after a replay, when a backend may
    // re-deliver the message the history ended with.
    historyTailGuard: {} as Record<string, boolean>,
    activeSessionId: null as string | null,
    loading: false,
    error: '',
    // Sequence number of the latest project-list request; a response that
    // arrives after a newer request was issued is discarded.
    listRequest: 0,
  }),

  getters: {
    messagesFor: (state) => (sessionId: string) => state.messages[sessionId] ?? [],
    statusFor: (state) => (sessionId: string) => state.streamStatus[sessionId] ?? 'idle',
    sessionById: (state) => (sessionId: string) =>
      state.sessions.find((entry) => entry.id === sessionId) ?? null,
    activeSession(state): AgentSession | null {
      return state.sessions.find((entry) => entry.id === state.activeSessionId) ?? null
    },
  },

  actions: {
    async loadSessions(projectId: number) {
      const request = ++this.listRequest
      if (this.projectId !== projectId) {
        this.sessions = []
        // History is per session id, but a different project's log must never
        // render under the new project, so the replay state is reset with the
        // list it belongs to.
        this.messages = {}
        this.historyLoaded = {}
        this.historyError = {}
        this.historyTruncated = {}
        this.historyTailGuard = {}
      }
      this.loading = true
      this.projectId = projectId
      try {
        const list = await listProjectSessions(projectId)
        if (request !== this.listRequest) return
        this.sessions = list.sessions
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        if (request !== this.listRequest) return
        this.error = describeApiError(error)
      } finally {
        if (request === this.listRequest) this.loading = false
      }
    },

    // Silently refreshes the session list for the running-turn poll: it never
    // clears the list or toggles loading, and a response that arrives after a
    // newer request is discarded. A failure keeps the last good list and stays
    // quiet, because a poll must not surface a transient error.
    async refreshSessions(projectId: number) {
      const request = ++this.listRequest
      try {
        const list = await listProjectSessions(projectId)
        if (request !== this.listRequest) return
        if (this.projectId !== projectId) return
        this.sessions = list.sessions
      } catch (error) {
        // Deliberately silent: keep the last good list.
        void error
      }
    },

    // Discovers an instance's agent-native sessions and merges them into the
    // session list by platform id, so a rescan never duplicates a session.
    async discoverNativeSessions(projectId: number, instanceId: string) {
      try {
        const list = await discoverProjectInstanceSessions(projectId, instanceId)
        const byId = new Map(this.sessions.map((entry) => [entry.id, entry]))
        for (const discovered of list.sessions) byId.set(discovered.id, discovered)
        this.sessions = [...byId.values()]
        this.error = ''
        return list.sessions
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        return null
      }
    },

    async createSession(projectId: number, body: CreateSessionBody) {
      const auth = useAuthStore()
      try {
        const created = await createProjectSession(projectId, body, auth.csrfToken)
        this.sessions = [...this.sessions, created]
        this.error = ''
        return created
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        return null
      }
    },

    // Subscribes to a session's normalized event stream. A second call for the
    // same session is a no-op, so a view re-render never opens a duplicate
    // stream. EventSource reconnects transparently after a transient drop; a
    // terminal close is surfaced as `error` without touching other sessions.
    subscribe(projectId: number, sessionId: string) {
      if (sources.has(sessionId)) return
      this.streamStatus[sessionId] = 'connecting'
      const source = new EventSource(sessionEventsPath(projectId, sessionId))
      sources.set(sessionId, source)

      source.onopen = () => {
        this.streamStatus[sessionId] = 'open'
      }
      source.onerror = () => {
        if (source.readyState === EventSource.CLOSED) {
          this.streamStatus[sessionId] = 'error'
          sources.delete(sessionId)
        } else {
          this.streamStatus[sessionId] = 'reconnecting'
        }
      }
      for (const type of ['lifecycle', 'message', 'tool_call', 'tool_result', 'error', 'unknown']) {
        source.addEventListener(type, (event) => {
          this.handleEvent(sessionId, (event as MessageEvent).data)
        })
      }
    },

    // Marks a session active and opens its stream. Switching sessions closes
    // the previously active session's stream so only one subscription is live
    // at a time; re-activating the same session is a no-op.
    // activateSession is the low-level subscription switch; callers opening a
    // task use openSession so the replay always lands before the stream.
    activateSession(projectId: number, sessionId: string) {
      const previous = this.activeSessionId
      if (previous && previous !== sessionId) this.unsubscribe(previous)
      this.activeSessionId = sessionId
      this.subscribe(projectId, sessionId)
    },

    // Re-opens a stream after a terminal error, keeping the message log.
    resubscribe(projectId: number, sessionId: string) {
      this.unsubscribe(sessionId)
      this.subscribe(projectId, sessionId)
    },

    unsubscribe(sessionId: string) {
      const source = sources.get(sessionId)
      if (source) {
        source.close()
        sources.delete(sessionId)
      }
      this.streamStatus[sessionId] = 'idle'
    },

    unsubscribeAll() {
      for (const sessionId of [...sources.keys()]) this.unsubscribe(sessionId)
    },

    // Applies one normalized event to its owning session. Events addressed to
    // any other instance or session are dropped, which is what keeps instances
    // isolated in the UI.
    handleEvent(sessionId: string, data: string) {
      let event: NormalizedEvent
      try {
        event = JSON.parse(data) as NormalizedEvent
      } catch {
        return
      }
      const session = this.sessionById(sessionId)
      if (!session) return
      if (event.instanceId && event.instanceId !== session.instanceId) return
      if (event.sessionId && event.sessionId !== sessionId) return

      if (event.type === 'message') {
        if (!event.message) return
        if (this.consumeHistoryGuard(sessionId, event.message.role, event.message.text)) return
        this.appendMessage(sessionId, event.message.role, event.message.text, event.message.final, event.sequence)
        return
      }
      if (event.type === 'lifecycle') {
        this.applyLifecycle(sessionId, event)
        return
      }
      const entry = entryFromEvent(sessionId, event)
      if (!entry) return
      this.pushEntry(sessionId, entry)
      // These are the backend's turn-ending error codes; any other error is
      // reported without concluding the turn.
      if (event.type === 'error' && event.error) {
        if (event.error.code === 'execution_failed' || event.error.code === 'process_exit') {
          this.setTurn(sessionId, 'idle')
        }
      }
    },

    // consumeHistoryGuard reports whether a live message is a re-delivery of
    // the line history ended with, which the backend may send once right after
    // a replay. The guard is spent on the first live message either way, so a
    // genuinely repeated sentence later in the conversation is never dropped.
    consumeHistoryGuard(sessionId: string, role: string, text: string): boolean {
      if (!this.historyTailGuard[sessionId]) return false
      this.historyTailGuard[sessionId] = false
      const last = [...(this.messages[sessionId] ?? [])]
        .reverse()
        .find((entry) => entry.kind === 'message')
      return last !== undefined && last.role === role && last.text === text
    },

    // loadHistory replays the session's agent-native prior turns into the log
    // and marks where they end. It is called by openSession before the stream
    // opens; a failure is recorded for retry and never blocks the live stream.
    async loadHistory(projectId: number, sessionId: string) {
      if (this.historyLoaded[sessionId]) return
      this.historyError[sessionId] = ''
      try {
        const history = await getSessionHistory(projectId, sessionId)
        if (this.historyLoaded[sessionId]) return
        const entries = this.ensureMessages(sessionId)
        for (const event of history.events) {
          if (event.type === 'message' && event.message) {
            // Replayed messages are complete and are never echo-deduplicated:
            // two identical user turns in a transcript are two real turns.
            this.appendMessage(
              sessionId,
              event.message.role,
              event.message.text,
              event.message.final,
              event.sequence,
              false,
            )
            continue
          }
          const entry = entryFromEvent(sessionId, event)
          if (entry) entries.push(entry)
        }
        if (history.events.length > 0) {
          entries.push({
            id: nextEntryID(sessionId, 0, 'separator'),
            kind: 'separator',
            role: 'system',
            text: '以上为历史记录',
            streaming: false,
          })
          this.historyTailGuard[sessionId] = true
        }
        this.historyTruncated[sessionId] = history.truncated
        this.historyLoaded[sessionId] = true
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.historyError[sessionId] = describeApiError(error)
      }
    },

    // openSession makes a task usable in one ordered step: re-attach the
    // session (starting a backend instance when none is running), replay its
    // history, then subscribe. The order is load-bearing, not an optimization:
    // subscribing first would render live events above the history that
    // arrives after them.
    async openSession(projectId: number, sessionId: string) {
      await this.continueSession(projectId, sessionId)
      await this.loadHistory(projectId, sessionId)
      this.activateSession(projectId, sessionId)
    },

    appendMessage(
      sessionId: string,
      role: string,
      text: string,
      final: boolean,
      sequence: number,
      dedupeEcho = true,
    ) {
      const list = this.ensureMessages(sessionId)
      const last = list[list.length - 1]
      if (last && last.kind === 'message' && last.streaming && last.role === role) {
        last.text += text
        if (final) last.streaming = false
        return
      }
      // A user turn echoed by the backend is deduplicated against the copy the
      // workbench already added when the message was sent. A replay opts out:
      // a transcript's repeated lines are distinct turns, not echoes.
      if (
        dedupeEcho &&
        role === 'user' &&
        last &&
        last.kind === 'message' &&
        last.role === 'user' &&
        last.text === text
      ) {
        return
      }
      list.push({
        id: nextEntryID(sessionId, sequence, 'message'),
        kind: 'message',
        role,
        text,
        streaming: !final,
      })
    },

    applyLifecycle(sessionId: string, event: NormalizedEvent) {
      const state = event.lifecycle?.state
      // The turn is derived from the same lifecycle states the backend uses, so
      // the open panel's send/stop action tracks a live turn. An interrupted
      // turn settles as agent_settled (detail aborted) with no error entry.
      if (state === 'execution_started' || state === 'agent_start') {
        this.setTurn(sessionId, 'running')
        return
      }
      if (
        state === 'execution_succeeded' ||
        state === 'execution_interrupted' ||
        state === 'agent_settled' ||
        state === 'agent_end'
      ) {
        this.setTurn(sessionId, 'idle')
        return
      }
      if (state === 'stopped') {
        this.setStatus(sessionId, 'stopped')
        return
      }
      // A failed/errored instance means the session is no longer usable; it is
      // reported disconnected (resumable once a capable instance runs again).
      if (state === 'failed' || state === 'error' || state === 'disconnected') {
        this.setStatus(sessionId, 'disconnected')
      }
    },

    setTurn(sessionId: string, turn: AgentSession['turn']) {
      this.sessions = this.sessions.map((entry) =>
        entry.id === sessionId ? { ...entry, turn } : entry,
      )
    },

    setStatus(sessionId: string, status: AgentSession['status']) {
      this.sessions = this.sessions.map((entry) =>
        entry.id === sessionId ? { ...entry, status } : entry,
      )
    },

    ensureMessages(sessionId: string): ChatEntry[] {
      if (!this.messages[sessionId]) this.messages[sessionId] = []
      return this.messages[sessionId]
    },

    pushEntry(sessionId: string, entry: ChatEntry) {
      this.ensureMessages(sessionId).push(entry)
    },

    async sendMessage(projectId: number, sessionId: string, text: string, model?: string) {
      const auth = useAuthStore()
      const trimmed = text.trim()
      if (trimmed === '') return null
      // Show the user's turn immediately; a streamed echo is deduplicated.
      this.appendMessage(sessionId, 'user', trimmed, true, 0)
      try {
        const updated = await sendProjectSessionMessage(
          projectId,
          sessionId,
          model ? { text: trimmed, model } : { text: trimmed },
          auth.csrfToken,
        )
        this.error = ''
        return updated
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        return null
      }
    },

    async continueSession(projectId: number, sessionId: string) {
      const auth = useAuthStore()
      try {
        const updated = await continueProjectSession(projectId, sessionId, auth.csrfToken)
        this.sessions = this.sessions.map((entry) => (entry.id === sessionId ? updated : entry))
        this.error = ''
        return updated
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        return null
      }
    },

    // Interrupts the session's current turn. The server's returned DTO
    // replaces the list entry, so the turn state is never optimistic; an idle
    // session is an idempotent success and produces no error.
    async interrupt(projectId: number, sessionId: string) {
      const auth = useAuthStore()
      try {
        const interrupted = await interruptProjectSession(projectId, sessionId, auth.csrfToken)
        this.sessions = this.sessions.map((entry) =>
          entry.id === sessionId ? interrupted : entry,
        )
        this.error = ''
        return interrupted
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        return null
      }
    },

    // Stops the session's run. The adapter stops at instance level, so the
    // target is marked stopped and its same-instance siblings become
    // disconnected; sessions on other instances are left untouched.
    async stopSession(projectId: number, sessionId: string) {
      const auth = useAuthStore()
      const target = this.sessionById(sessionId)
      try {
        const stopped = await stopProjectSession(projectId, sessionId, auth.csrfToken)
        this.sessions = this.sessions.map((entry) => {
          if (entry.id === sessionId) return stopped
          if (target && entry.instanceId === target.instanceId) {
            return { ...entry, status: 'disconnected' }
          }
          return entry
        })
        this.error = ''
        return stopped
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        return null
      }
    },
  },
})
