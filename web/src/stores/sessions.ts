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
  deleteProjectSession,
  discoverProjectInstanceSessions,
  discoverProjectSessions as discoverProjectSessionsRequest,
  getSessionHistory,
  interruptProjectSession,
  listProjectSessions,
  sendProjectSessionMessage,
  sessionEventsPath,
  stopProjectSession,
  type AgentSession,
  type BackendId,
  type CreateSessionBody,
  type NormalizedEvent,
} from '../api/sessions'
import { ApiError, describeApiError, UnauthorizedError } from '../api/client'
import { useAuthStore } from './auth'

// One rendered line in a session's log. Agent replies are rendered as sanitized
// markdown in the view (see ../markdown); every other kind is plain text. A
// `separator` line marks where replayed history ends and the live stream begins;
// a `thinking` line is the agent's reasoning, which the pane shows collapsed.
export interface ChatEntry {
  id: string
  kind: 'message' | 'thinking' | 'tool_call' | 'tool_result' | 'error' | 'separator'
  role: string
  text: string
  streaming: boolean
  callId?: string
  name?: string
  // Tool calls carry the raw argument payload so the view can summarise it
  // (name + representative argument) without the store knowing tool schemas.
  input?: unknown
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

// findOpenStreaming returns the most recent still-streaming entry of a kind
// (and, for messages, of the same role), scanning past anything appended in
// between. Backends interleave their streams — pi emits a thinking_delta in
// the middle of a running text delta and closes the thinking block after text
// resumed — so the open block, not the last list position, is what a delta
// belongs to. Without this, interleaving fragments a block into several
// entries and its terminator opens an empty one.
function findOpenStreaming(
  list: ChatEntry[],
  kind: ChatEntry['kind'],
  role?: string,
): ChatEntry | undefined {
  for (let i = list.length - 1; i >= 0; i -= 1) {
    const entry = list[i]
    if (entry.kind === kind && entry.streaming && (role === undefined || entry.role === role)) {
      return entry
    }
  }
  return undefined
}

// entryFromEvent maps one non-message event to its log line, or null when the
// event renders nothing (lifecycle events are handled separately because they
// carry no log line of their own). Both the live path and the history replay
// go through it, so a replayed tool call looks exactly like a live one.
// keepSettledTurn merges an interrupt's response into the entry the live stream
// may have settled first. The backend answers from the session row as it stood
// when the interrupt was accepted; the stream reports the interruption as soon
// as the agent stops. When the stream already settled the turn, the entry keeps
// it settled — an interrupt can only end a turn, never restart one.
function keepSettledTurn(current: AgentSession, incoming: AgentSession): AgentSession {
  if (current.turn !== 'running' && incoming.turn === 'running') {
    return { ...incoming, turn: current.turn }
  }
  return incoming
}

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
        input: event.toolCall.input,
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
    // echoPending records, per session, the text of the user turn the
    // workbench just sent locally. The backend echoes that turn once on the
    // stream; matching the pending text — rather than whatever entry happens
    // to be last — is what survives diagnostics interleaved before the echo.
    echoPending: {} as Record<string, string>,
    activeSessionId: null as string | null,
    loading: false,
    error: '',
    // The code of the last API failure, so a view can act on a specific
    // refusal (the service is down) instead of recognising its own copy.
    errorCode: '',
    // Sequence number of the latest project-list request; a response that
    // arrives after a newer request was issued is discarded.
    listRequest: 0,
    // Counts the times this store changed the session index itself — a created
    // task, a deleted one. Components that keep their own copy of a project's
    // list (the task tree does, one list per expanded project) watch it to know
    // their copy went stale. A page load is not a change, so loading does not
    // bump it.
    revision: 0,
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
        this.echoPending = {}
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

    // discoverProjectSessions scans a backend at the project level: the server
    // starts or reuses the instance that serves the backend, so the caller
    // never names an instance. Results merge by platform id, so a rescan never
    // duplicates a session.
    async discoverProjectSessions(projectId: number, backend: BackendId) {
      try {
        const list = await discoverProjectSessionsRequest(projectId, backend)
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
        this.revision++
        this.error = ''
        this.errorCode = ''
        return created
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        this.errorCode = error instanceof ApiError ? error.code : ''
        return null
      }
    },

    // deleteSession removes a session from the platform's index. It closes the
    // session's stream and drops its local state so nothing stale can render;
    // the agent-native transcript is untouched and may be rediscovered later.
    async deleteSession(projectId: number, sessionId: string): Promise<boolean> {
      const auth = useAuthStore()
      try {
        await deleteProjectSession(projectId, sessionId, auth.csrfToken)
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        return false
      }
      this.unsubscribe(sessionId)
      this.sessions = this.sessions.filter((entry) => entry.id !== sessionId)
      this.revision++
      if (this.activeSessionId === sessionId) this.activeSessionId = null
      delete this.messages[sessionId]
      delete this.streamStatus[sessionId]
      delete this.historyLoaded[sessionId]
      delete this.historyError[sessionId]
      delete this.historyTruncated[sessionId]
      delete this.historyTailGuard[sessionId]
      this.error = ''
      return true
    },

    // Subscribes to a session's normalized event stream. A second call for the
    // same session is a no-op, so a view re-render never opens a duplicate
    // stream. A stream that has reported an error is the exception: after a
    // drop the browser may sit in CONNECTING forever without dialling again —
    // observed when the process behind a session dies — so the task would stay
    // at 正在重连 for the rest of the session. Opening the task again is the
    // user asking for live events, and the dead stream is replaced.
    subscribe(projectId: number, sessionId: string) {
      const existing = sources.get(sessionId)
      if (existing) {
        if (this.streamStatus[sessionId] !== 'reconnecting') return
        existing.close()
        sources.delete(sessionId)
      }
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
      for (const type of ['lifecycle', 'message', 'thinking', 'tool_call', 'tool_result', 'error', 'unknown']) {
        source.addEventListener(type, (event) => {
          this.handleEvent(sessionId, (event as MessageEvent).data)
        })
      }
    },

    // Marks a session active and opens its stream. Multiple tasks can stay
    // open at once (tab bar), so activating one session never closes another
    // session's stream; each subscription lives until its task is closed or
    // deleted. Re-activating the same session is a no-op.
    // activateSession is the low-level subscription switch; callers opening a
    // task use openSession so the replay always lands before the stream.
    activateSession(projectId: number, sessionId: string) {
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
        if (
          event.message.role === 'user' &&
          this.echoPending[sessionId] === event.message.text
        ) {
          // The echo of the turn the workbench sent itself: already shown.
          delete this.echoPending[sessionId]
          return
        }
        if (event.message.role !== 'user') {
          // The agent answered; no echo can still be pending for this text.
          delete this.echoPending[sessionId]
        }
        this.appendMessage(sessionId, event.message.role, event.message.text, event.message.final, event.sequence)
        return
      }
      if (event.type === 'thinking') {
        if (!event.thinking) return
        this.appendThinking(sessionId, event.thinking.text, event.thinking.final, event.sequence)
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
          this.settleStreaming(sessionId)
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
            // (Echo dedupe lives in the live path's echoPending; replay never
            // consults it.)
            this.appendMessage(
              sessionId,
              event.message.role,
              event.message.text,
              event.message.final,
              event.sequence,
            )
            continue
          }
          if (event.type === 'thinking' && event.thinking) {
            this.appendThinking(sessionId, event.thinking.text, event.thinking.final, event.sequence)
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
    ) {
      const list = this.ensureMessages(sessionId)
      const open = findOpenStreaming(list, 'message', role)
      if (open) {
        if (text !== '') open.text += text
        if (final) open.streaming = false
        return
      }
      // An empty event with no open block: a terminator whose block already
      // settled (or never streamed) renders nothing but a blank bubble, so it
      // is dropped rather than pushed.
      if (text === '') return
      list.push({
        id: nextEntryID(sessionId, sequence, 'message'),
        kind: 'message',
        role,
        text,
        streaming: !final,
      })
    },

    // appendThinking adds reasoning to the log, merging into the thinking
    // block that is still open (see findOpenStreaming) so the pane shows one
    // growing block rather than a line per chunk. An event that belongs to no
    // open block — an empty terminator arriving after the block settled, or
    // an empty delta — renders an empty block and is dropped.
    appendThinking(sessionId: string, text: string, final: boolean, sequence: number) {
      const list = this.ensureMessages(sessionId)
      const open = findOpenStreaming(list, 'thinking')
      if (open) {
        if (text !== '') open.text += text
        if (final) open.streaming = false
        return
      }
      if (text !== '') {
        list.push({
          id: nextEntryID(sessionId, sequence, 'thinking'),
          kind: 'thinking',
          role: 'assistant',
          text,
          streaming: !final,
        })
      }
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
        this.settleStreaming(sessionId)
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

    // settleStreaming closes every still-open entry of a session. A turn that
    // ended (settled, interrupted or failed) can no longer extend them, and a
    // block left streaming shows a cursor that never stops blinking.
    settleStreaming(sessionId: string) {
      for (const entry of this.messages[sessionId] ?? []) {
        entry.streaming = false
      }
      delete this.echoPending[sessionId]
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
      // Show the user's turn immediately; the stream echo is matched against
      // this pending text and dropped.
      this.appendMessage(sessionId, 'user', trimmed, true, 0)
      this.echoPending[sessionId] = trimmed
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
        this.errorCode = ''
        return updated
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        // A refused reconnect is the case the panel renders as "reconnect once
        // the service is up", so the code has to survive the copy.
        this.errorCode = error instanceof ApiError ? error.code : ''
        return null
      }
    },

    // Interrupts the session's current turn. The server's returned DTO
    // replaces the list entry, so the turn state is never optimistic; an idle
    // session is an idempotent success and produces no error.
    //
    // One thing the DTO may not be trusted about: the turn. The backend builds
    // the response from the row as it stood when the interrupt was accepted,
    // while the event stream reports the interruption as soon as the agent
    // stops — so a response can arrive *after* the stream already settled the
    // turn and say it is still running. An interrupt can only end a turn, so a
    // turn the live stream has settled is never rewound.
    async interrupt(projectId: number, sessionId: string) {
      const auth = useAuthStore()
      try {
        const interrupted = await interruptProjectSession(projectId, sessionId, auth.csrfToken)
        this.sessions = this.sessions.map((entry) =>
          entry.id === sessionId ? keepSettledTurn(entry, interrupted) : entry,
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
