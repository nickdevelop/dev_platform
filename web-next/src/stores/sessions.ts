import { create } from 'zustand'
import {
  continueProjectSession,
  createProjectSession,
  deleteProjectSession,
  getSessionHistory,
  interruptProjectSession,
  listProjectSessions,
  sendProjectSessionMessage,
  stopProjectSession,
  type AgentSession,
  type CreateSessionBody,
  type ImageAttachment,
  type InteractionPayload,
  type InteractionSnapshot,
  type MessageModel,
  type MessageUsage,
  type NormalizedEvent,
  type SendMessageBody,
  type TurnPayload,
} from '../api/sessions'
import { ApiError, describeApiError, UnauthorizedError } from '../api/client'
import type { ChatEntry, StreamStatus } from '../features/chat/state/entry'
import {
  entryFromEvent,
  findOpenStreaming,
  keepSettledTurn,
  nextEntryID,
} from '../features/chat/state/sessionLog'
import {
  consumeEventFrame,
  closeAllSessionStreams,
  closeSessionStream,
  openStreamIds,
  resubscribeSession,
  subscribeSession,
} from '../features/chat/execution/SessionEventStream'
import type { StreamStore } from '../features/chat/execution/SessionEventStream'
import { useInteractionsStore } from './interactions'

interface SessionsState {
  sessions: AgentSession[]
  sessionsByProject: Record<number, AgentSession[]>
  sessionTurns: Record<string, 'idle' | 'running'>
  projectId: number | null
  messages: Record<string, ChatEntry[]>
  streamStatus: Record<string, StreamStatus>
  historyLoaded: Record<string, boolean>
  historyError: Record<string, string>
  historyTruncated: Record<string, boolean>
  historyTailGuard: Record<string, boolean>
  echoPending: Record<string, string>
  selectedModels: Record<string, string>
  autoModels: Record<string, string>
  activeSessionId: string | null
  lifecycleUsage: Record<string, MessageUsage>
  loading: boolean
  error: string
  errorCode: string
  listRequest: number
  revision: number
  loadSessions: (projectId: number) => Promise<void>
  refreshSessions: (projectId: number) => Promise<void>
  createSession: (projectId: number, body: CreateSessionBody) => Promise<AgentSession | null>
  deleteSession: (projectId: number, sessionId: string) => Promise<boolean>
  openSession: (projectId: number, sessionId: string) => Promise<void>
  loadHistory: (projectId: number, sessionId: string) => Promise<void>
  sendMessage: (
    projectId: number,
    sessionId: string,
    text: string,
    model?: string,
    images?: ImageAttachment[],
  ) => Promise<AgentSession | null>
  stopSession: (projectId: number, sessionId: string) => Promise<AgentSession | null>
  interrupt: (projectId: number, sessionId: string) => Promise<AgentSession | null>
  unsubscribe: (sessionId: string) => Promise<void>
  unsubscribeAll: () => void
  setStreamStatus: (sessionId: string, status: StreamStatus) => void
  clearEchoPending: (sessionId: string) => void
  consumeHistoryTailGuard: (sessionId: string) => boolean
  sessionById: (sessionId: string) => AgentSession | null
  sessionsFor: (projectId: number) => AgentSession[]
  modelFor: (sessionId: string) => string
  setModel: (sessionId: string, model: string) => void
  setAutoModel: (sessionId: string, model: string) => void
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
  applyTurn: (sessionId: string, turn: TurnPayload) => void
  applyInteraction: (sessionId: string, payload: InteractionPayload) => void
  applyInteractionSnapshot: (sessionId: string, snapshot: InteractionSnapshot) => void
  setTurn: (sessionId: string, turn: AgentSession['turn']) => void
  settleStreaming: (sessionId: string) => void
  pushEntry: (sessionId: string, entry: ChatEntry) => void
  repairDanglingEntries: (projectId: number, sessionId: string) => Promise<void>
  rebuildFromHistory: (projectId: number, sessionId: string) => Promise<void>
}

function replaceSession(list: AgentSession[], updated: AgentSession): AgentSession[] {
  return list.map((entry) => (entry.id === updated.id ? updated : entry))
}

function ensureMessages(state: SessionsState, sessionId: string): ChatEntry[] {
  return state.messages[sessionId] ?? []
}

function putMessages(
  set: (partial: Partial<SessionsState> | ((state: SessionsState) => Partial<SessionsState>)) => void,
  sessionId: string,
  entries: ChatEntry[],
) {
  set((state) => ({ messages: { ...state.messages, [sessionId]: entries } }))
}

export const useSessionsStore = create<SessionsState>((set, getState) => ({
  sessions: [],
  sessionsByProject: {},
  sessionTurns: {},
  projectId: null,
  messages: {},
  streamStatus: {},
  historyLoaded: {},
  historyError: {},
  historyTruncated: {},
  historyTailGuard: {},
  echoPending: {},
  selectedModels: {},
  autoModels: {},
  activeSessionId: null,
  lifecycleUsage: {},
  loading: false,
  error: '',
  errorCode: '',
  listRequest: 0,
  revision: 0,

  loadSessions: async (projectId) => {
    const request = ++getState().listRequest
    set({ projectId, loading: true })
    if (getState().projectId !== projectId) {
      set({ sessions: getState().sessionsByProject[projectId] ?? [] })
    }
    try {
      const list = await listProjectSessions(projectId)
      if (request !== getState().listRequest) return
      set((state) => {
        const turns = { ...state.sessionTurns }
        for (const session of list.sessions) if (session.turn) turns[session.id] = session.turn
        return {
          sessions: list.sessions,
          sessionsByProject: { ...state.sessionsByProject, [projectId]: list.sessions },
          sessionTurns: turns,
          loading: false,
          error: '',
        }
      })
    } catch (error) {
      if (error instanceof UnauthorizedError) {
        set({ loading: false })
        return
      }
      if (request !== getState().listRequest) return
      set({ loading: false, error: describeApiError(error) })
    }
  },

  refreshSessions: async (projectId) => {
    const request = ++getState().listRequest
    try {
      const list = await listProjectSessions(projectId)
      if (request !== getState().listRequest) return
      set({ sessions: list.sessions, sessionsByProject: { ...getState().sessionsByProject, [projectId]: list.sessions } })
    } catch {
      // A silent refresh keeps the last good list.
    }
  },

  createSession: async (projectId, body) => {
    try {
      const result = await createProjectSession(projectId, body)
      const created = result.session
      set((state) => ({
        sessions: [...state.sessions, created],
        sessionsByProject: {
          ...state.sessionsByProject,
          [projectId]: [...(state.sessionsByProject[projectId] ?? []), created],
        },
        revision: state.revision + 1,
        error: '',
      }))
      return created
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ error: describeApiError(error), errorCode: error instanceof ApiError ? error.code : '' })
      return null
    }
  },

  deleteSession: async (projectId, sessionId) => {
    try {
      await deleteProjectSession(projectId, sessionId)
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      set({ error: describeApiError(error) })
      return false
    }
    await getState().unsubscribe(sessionId)
    set((state) => {
      const messages = { ...state.messages }
      const streamStatus = { ...state.streamStatus }
      const historyLoaded = { ...state.historyLoaded }
      const historyError = { ...state.historyError }
      const historyTruncated = { ...state.historyTruncated }
      const historyTailGuard = { ...state.historyTailGuard }
      const selectedModels = { ...state.selectedModels }
      const autoModels = { ...state.autoModels }
      const sessionTurns = { ...state.sessionTurns }
      delete messages[sessionId]
      delete streamStatus[sessionId]
      delete historyLoaded[sessionId]
      delete historyError[sessionId]
      delete historyTruncated[sessionId]
      delete historyTailGuard[sessionId]
      delete selectedModels[sessionId]
      delete autoModels[sessionId]
      delete sessionTurns[sessionId]
      const sessionsByProject: Record<number, AgentSession[]> = {}
      for (const [key, list] of Object.entries(state.sessionsByProject)) {
        sessionsByProject[Number(key)] = list.filter((entry) => entry.id !== sessionId)
      }
      return {
        sessions: state.sessions.filter((entry) => entry.id !== sessionId),
        sessionsByProject,
        activeSessionId: state.activeSessionId === sessionId ? null : state.activeSessionId,
        messages,
        streamStatus,
        historyLoaded,
        historyError,
        historyTruncated,
        historyTailGuard,
        selectedModels,
        autoModels,
        sessionTurns,
        revision: state.revision + 1,
        error: '',
      }
    })
    return true
  },

  openSession: async (projectId, sessionId) => {
    try {
      const updated = await continueProjectSession(projectId, sessionId)
      set((state) => ({
        sessions: replaceSession(state.sessions, updated),
        sessionsByProject: {
          ...state.sessionsByProject,
          [projectId]: replaceSession(state.sessionsByProject[projectId] ?? [], updated),
        },
        sessionTurns: { ...state.sessionTurns, [sessionId]: updated.turn },
      }))
    } catch (error) {
      if (error instanceof UnauthorizedError) return
      set({ error: describeApiError(error), errorCode: error instanceof ApiError ? error.code : '' })
      return
    }
    await getState().loadHistory(projectId, sessionId)
    set({ activeSessionId: sessionId })
    await subscribeSession(sessionStreamStore, projectId, sessionId)
  },

  loadHistory: async (projectId, sessionId) => {
    if (getState().historyLoaded[sessionId]) return
    try {
      const history = await getSessionHistory(projectId, sessionId)
      if (getState().historyLoaded[sessionId]) return
      const entries: ChatEntry[] = []
      for (const event of history.events) {
        if (event.type === 'message' && event.message) {
          appendMessageTo(entries, sessionId, event.message.role, event.message.text, event.message.final, event.sequence, event.message.model, event.message.usage, event.message.images, event.timestamp)
          continue
        }
        if (event.type === 'thinking' && event.thinking) {
          appendThinkingTo(entries, sessionId, event.thinking.text, event.thinking.final, event.sequence)
          continue
        }
        const entry = entryFromEvent(sessionId, event)
        if (entry) entries.push(entry)
      }
      set((state) => ({
        messages: { ...state.messages, [sessionId]: entries },
        historyLoaded: { ...state.historyLoaded, [sessionId]: true },
        historyTruncated: { ...state.historyTruncated, [sessionId]: history.truncated },
        historyTailGuard: { ...state.historyTailGuard, [sessionId]: history.events.length > 0 },
        historyError: { ...state.historyError, [sessionId]: '' },
      }))
    } catch (error) {
      if (error instanceof UnauthorizedError) return
      set((state) => ({ historyError: { ...state.historyError, [sessionId]: describeApiError(error) } }))
    }
  },

  sendMessage: async (projectId, sessionId, text, model, images) => {
    const trimmed = text.trim()
    if (trimmed === '' && (!images || images.length === 0)) return null
    getState().appendMessage(sessionId, 'user', trimmed, true, 0, undefined, undefined, images, new Date().toISOString())
    set((state) => ({ echoPending: { ...state.echoPending, [sessionId]: trimmed } }))
    try {
      const body: SendMessageBody = { text: trimmed }
      if (model) body.model = model
      if (images && images.length > 0) body.images = images
      const updated = await sendProjectSessionMessage(projectId, sessionId, body)
      set((state) => ({
        sessions: replaceSession(state.sessions, updated),
        sessionsByProject: { ...state.sessionsByProject, [projectId]: replaceSession(state.sessionsByProject[projectId] ?? [], updated) },
        error: '',
      }))
      return updated
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ error: describeApiError(error) })
      return null
    }
  },

  stopSession: async (projectId, sessionId) => {
    try {
      const stopped = await stopProjectSession(projectId, sessionId)
      set((state) => ({ sessions: replaceSession(state.sessions, stopped) }))
      return stopped
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ error: describeApiError(error) })
      return null
    }
  },

  interrupt: async (projectId, sessionId) => {
    try {
      const interrupted = await interruptProjectSession(projectId, sessionId)
      set((state) => ({
        sessions: state.sessions.map((entry) => (entry.id === sessionId ? keepSettledTurn(entry, interrupted) : entry)),
      }))
      return interrupted
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ error: describeApiError(error) })
      return null
    }
  },

  unsubscribe: async (sessionId) => {
    await closeSessionStream(sessionStreamStore, sessionId)
  },

  unsubscribeAll: () => closeAllSessionStreams(sessionStreamStore),

  setStreamStatus: (sessionId, status) => {
    set((state) => ({ streamStatus: { ...state.streamStatus, [sessionId]: status } }))
  },

  clearEchoPending: (sessionId) => {
    set((state) => {
      const next = { ...state.echoPending }
      delete next[sessionId]
      return { echoPending: next }
    })
  },

  consumeHistoryTailGuard: (sessionId) => {
    if (!getState().historyTailGuard[sessionId]) return false
    set((state) => ({ historyTailGuard: { ...state.historyTailGuard, [sessionId]: false } }))
    return true
  },

  sessionById: (sessionId) => getState().sessions.find((entry) => entry.id === sessionId) ?? null,
  sessionsFor: (projectId) => getState().sessionsByProject[projectId] ?? [],
  modelFor: (sessionId) => getState().selectedModels[sessionId] ?? getState().sessionById(sessionId)?.model ?? '',

  setModel: (sessionId, model) => {
    set((state) => ({ selectedModels: { ...state.selectedModels, [sessionId]: model } }))
  },

  setAutoModel: (sessionId, model) => {
    set((state) => ({ autoModels: { ...state.autoModels, [sessionId]: model } }))
  },

  appendMessage: (sessionId, role, text, final, sequence, model, usage, images, timestamp) => {
    const list = [...ensureMessages(getState(), sessionId)]
    const open = findOpenStreaming(list, 'message', role)
    if (open) {
      const index = list.indexOf(open)
      list[index] = {
        ...open,
        text: final ? (text !== '' ? text : open.text) : open.text + text,
        streaming: final ? false : open.streaming,
        images: images && images.length > 0 ? images : open.images,
        model: model ?? open.model,
        usage: final ? usage ?? open.usage : open.usage,
        timestamp: timestamp ?? open.timestamp,
      }
      putMessages(set, sessionId, list)
      return
    }
    if (text === '' && (!images || images.length === 0)) return
    list.push({
      id: nextEntryID(sessionId, sequence, 'message'),
      kind: 'message',
      role,
      text,
      images,
      streaming: !final,
      timestamp: timestamp || new Date().toISOString(),
      model,
      usage: final ? usage : undefined,
    })
    putMessages(set, sessionId, list)
  },

  appendThinking: (sessionId, text, final, sequence) => {
    const list = [...ensureMessages(getState(), sessionId)]
    const open = findOpenStreaming(list, 'thinking')
    if (open) {
      const index = list.indexOf(open)
      list[index] = { ...open, text: final ? (text !== '' ? text : open.text) : open.text + text, streaming: final ? false : open.streaming }
      putMessages(set, sessionId, list)
      return
    }
    if (text === '') return
    list.push({ id: nextEntryID(sessionId, sequence, 'thinking'), kind: 'thinking', role: 'assistant', text, streaming: !final })
    putMessages(set, sessionId, list)
  },

  applyLifecycle: (sessionId, event) => {
    if (event.lifecycle?.usage) set((state) => ({ lifecycleUsage: { ...state.lifecycleUsage, [sessionId]: event.lifecycle!.usage! } }))
    const lifecycleState = event.lifecycle?.state
    if (lifecycleState === 'failed' || lifecycleState === 'error' || lifecycleState === 'disconnected') {
      set((state) => ({
        sessions: state.sessions.map((entry) => (entry.id === sessionId ? { ...entry, status: 'disconnected' } : entry)),
      }))
    }
  },

  applyTurn: (sessionId, turn) => {
    if (turn.phase === 'started') {
      set((state) => ({ sessionTurns: { ...state.sessionTurns, [sessionId]: 'running' } }))
      return
    }
    set((state) => ({ sessionTurns: { ...state.sessionTurns, [sessionId]: 'idle' } }))
    getState().settleStreaming(sessionId)
  },

  applyInteraction: (sessionId, payload) => useInteractionsStore.getState().applyInteraction(sessionId, payload),
  applyInteractionSnapshot: (sessionId, snapshot) => useInteractionsStore.getState().applySnapshot(sessionId, snapshot),

  setTurn: (sessionId, turn) => {
    set((state) => ({
      sessionTurns: turn ? { ...state.sessionTurns, [sessionId]: turn } : state.sessionTurns,
      sessions: state.sessions.map((entry) => (entry.id === sessionId ? { ...entry, turn } : entry)),
      sessionsByProject: Object.fromEntries(
        Object.entries(state.sessionsByProject).map(([key, list]) => [key, list.map((entry) => (entry.id === sessionId ? { ...entry, turn } : entry))]),
      ),
    }))
  },

  settleStreaming: (sessionId) => {
    set((state) => {
      const list = (state.messages[sessionId] ?? []).map((entry) => (entry.streaming ? { ...entry, streaming: false } : entry))
      const echoPending = { ...state.echoPending }
      delete echoPending[sessionId]
      return { messages: { ...state.messages, [sessionId]: list }, echoPending }
    })
  },

  pushEntry: (sessionId, entry) => {
    putMessages(set, sessionId, [...ensureMessages(getState(), sessionId), entry])
  },

  repairDanglingEntries: async (projectId, sessionId) => {
    if (!getState().historyLoaded[sessionId]) await getState().loadHistory(projectId, sessionId)
  },

  rebuildFromHistory: async (projectId, sessionId) => {
    await getState().unsubscribe(sessionId)
    set((state) => ({
      messages: { ...state.messages, [sessionId]: [] },
      historyLoaded: { ...state.historyLoaded, [sessionId]: false },
      historyTailGuard: { ...state.historyTailGuard, [sessionId]: false },
    }))
    await getState().openSession(projectId, sessionId)
  },
}))

function appendMessageTo(
  list: ChatEntry[],
  sessionId: string,
  role: string,
  text: string,
  final: boolean,
  sequence: number,
  model?: MessageModel,
  usage?: MessageUsage,
  images?: ImageAttachment[],
  timestamp?: string,
) {
  const open = findOpenStreaming(list, 'message', role)
  if (open) {
    open.text = final ? (text !== '' ? text : open.text) : open.text + text
    open.streaming = !final
    if (images?.length) open.images = images
    if (model) open.model = model
    if (final && usage) open.usage = usage
    return
  }
  if (text === '' && (!images || images.length === 0)) return
  list.push({
    id: nextEntryID(sessionId, sequence, 'message'),
    kind: 'message',
    role,
    text,
    images,
    streaming: !final,
    timestamp: timestamp || new Date().toISOString(),
    model,
    usage: final ? usage : undefined,
  })
}

function appendThinkingTo(list: ChatEntry[], sessionId: string, text: string, final: boolean, sequence: number) {
  const open = findOpenStreaming(list, 'thinking')
  if (open) {
    open.text = final ? (text !== '' ? text : open.text) : open.text + text
    open.streaming = !final
    return
  }
  if (text !== '') list.push({ id: nextEntryID(sessionId, sequence, 'thinking'), kind: 'thinking', role: 'assistant', text, streaming: !final })
}

const sessionStreamStore: StreamStore = {
  get streamStatus() { return useSessionsStore.getState().streamStatus },
  get messages() { return useSessionsStore.getState().messages },
  get historyTailGuard() { return useSessionsStore.getState().historyTailGuard },
  get echoPending() { return useSessionsStore.getState().echoPending },
  sessionById: (sessionId) => useSessionsStore.getState().sessionById(sessionId),
  pushEntry: (sessionId, entry) => useSessionsStore.getState().pushEntry(sessionId, entry),
  setTurn: (sessionId, turn) => useSessionsStore.getState().setTurn(sessionId, turn),
  settleStreaming: (sessionId) => useSessionsStore.getState().settleStreaming(sessionId),
  applyTurn: (sessionId, turn) => useSessionsStore.getState().applyTurn(sessionId, turn),
  applyInteraction: (sessionId, payload) => useSessionsStore.getState().applyInteraction(sessionId, payload),
  applyInteractionSnapshot: (sessionId, snapshot) => useSessionsStore.getState().applyInteractionSnapshot(sessionId, snapshot),
  subscribe: (projectId, sessionId) => { void subscribeSession(sessionStreamStore, projectId, sessionId) },
  appendMessage: (sessionId, role, text, final, sequence, model, usage, images, timestamp) => useSessionsStore.getState().appendMessage(sessionId, role, text, final, sequence, model, usage, images, timestamp),
  appendThinking: (sessionId, text, final, sequence) => useSessionsStore.getState().appendThinking(sessionId, text, final, sequence),
  applyLifecycle: (sessionId, event) => useSessionsStore.getState().applyLifecycle(sessionId, event),
  repairDanglingEntries: (projectId, sessionId) => useSessionsStore.getState().repairDanglingEntries(projectId, sessionId),
  rebuildFromHistory: (projectId, sessionId) => useSessionsStore.getState().rebuildFromHistory(projectId, sessionId),
  setStreamStatus: (sessionId, status) => useSessionsStore.getState().setStreamStatus(sessionId, status),
  clearEchoPending: (sessionId) => useSessionsStore.getState().clearEchoPending(sessionId),
  consumeHistoryTailGuard: (sessionId) => useSessionsStore.getState().consumeHistoryTailGuard(sessionId),
}

// Kept for tests/tools that want to drive a frame without a live EventSource.
export function consumeFrame(sessionId: string, data: string) {
  consumeEventFrame(sessionStreamStore, sessionId, data)
}

export function activeStreamIds(): string[] {
  return openStreamIds()
}

export { resubscribeSession }
