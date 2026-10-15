// Pinia store for the opencode raw event stream (透传 model). The backend
// proxies the shared sidecar's untouched OpenCodeEvent SSE bytes; this store
// holds one fetch+ReadableStream subscription per open session and projects
// only the approval kinds the normalized stream cannot carry:
// permission.asked/replied and form.created/replied/cancelled.
//
// Messages, thinking, tool calls/results and lifecycle stay on the legacy
// normalized stream (stores/sessions.ts) so existing rendering, scroll
// following and turn tracking are untouched. This store only drives the
// confirmation UI.

import { defineStore } from 'pinia'
import {
  listFormInfos,
  listPermissionRequests,
  replyFormInfo,
  replyPermissionRequest,
  rawEventsPath,
  type FormInfo,
  type PermissionRequest,
  type PermissionDecision,
} from '../api/interactions'
import { describeApiError, UnauthorizedError, ApiError } from '../api/client'
import { useAuthStore } from './auth'

// Live fetch readers are browser resources, not view data: kept outside
// reactive state like stores/sessions.ts does for EventSource handles.
const readers = new Map<string, { closed: boolean; controller: AbortController }>()

function readerKey(projectId: number, sessionId: string): string {
  return `${projectId}:${sessionId}`
}

// The minimal OpenCodeEvent shape this store consumes. Extra fields are
// ignored so the client stays forward-compatible with protocol additions.
interface RawEvent {
  type: string
  data?: {
    sessionID?: string
    requestID?: string
    id?: string
    form?: { id?: string }
    answer?: unknown
  }
}

function requestIdOf(event: RawEvent): string {
  return event.data?.requestID ?? event.data?.id ?? ''
}

function sessionIdOf(event: RawEvent): string {
  return event.data?.sessionID ?? ''
}

export const useInteractionsStore = defineStore('interactions', {
  state: () => ({
    permissions: {} as Record<string, PermissionRequest[]>,
    forms: {} as Record<string, FormInfo[]>,
    status: {} as Record<string, 'idle' | 'connecting' | 'open' | 'error'>,
    error: '',
  }),

  getters: {
    permissionsFor: (state) => (sessionId: string) => state.permissions[sessionId] ?? [],
    formsFor: (state) => (sessionId: string) => state.forms[sessionId] ?? [],
    pendingApprovalFor:
      (state) => (sessionId: string): { requestId: string; action: string } | null => {
        const first = state.permissions[sessionId]?.[0]
        if (!first) return null
        return { requestId: first.id, action: first.action }
      },
  },

  actions: {
    async loadApprovals(projectId: number, sessionId: string) {
      try {
        const [permissions, forms] = await Promise.all([
          listPermissionRequests(projectId, sessionId),
          listFormInfos(projectId, sessionId),
        ])
        this.permissions[sessionId] = permissions.permissions ?? []
        this.forms[sessionId] = forms.forms ?? []
        this.error = ''
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      }
    },

    async replyPermission(
      projectId: number,
      sessionId: string,
      requestId: string,
      decision: PermissionDecision,
    ): Promise<boolean> {
      const auth = useAuthStore()
      try {
        await replyPermissionRequest(projectId, sessionId, requestId, { decision }, auth.csrfToken)
        this.permissions[sessionId] = (this.permissions[sessionId] ?? []).filter(
          (entry) => entry.id !== requestId,
        )
        this.error = ''
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        if (error instanceof ApiError && error.code === 'permission_expired') {
          // The request is gone server-side (answered, cancelled or timed
          // out): drop the stale control instead of leaving an unanswerable
          // 允许/拒绝 behind.
          this.permissions[sessionId] = (this.permissions[sessionId] ?? []).filter(
            (entry) => entry.id !== requestId,
          )
        }
        this.error = describeApiError(error)
        return false
      }
    },

    async replyForm(
      projectId: number,
      sessionId: string,
      formId: string,
      answer: Record<string, unknown>,
    ): Promise<boolean> {
      const auth = useAuthStore()
      try {
        await replyFormInfo(projectId, sessionId, formId, { answer }, auth.csrfToken)
        this.forms[sessionId] = (this.forms[sessionId] ?? []).filter((entry) => entry.id !== formId)
        this.error = ''
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        if (error instanceof ApiError && error.code === 'form_expired') {
          // The form is gone server-side: drop the stale card instead of
          // leaving an unanswerable question behind.
          this.forms[sessionId] = (this.forms[sessionId] ?? []).filter((entry) => entry.id !== formId)
        }
        this.error = describeApiError(error)
        return false
      }
    },

    // Subscribes to the raw stream with fetch+ReadableStream (EventSource
    // cannot be used: the platform relies on cookies and needs explicit
    // reconnect control). Reconnects with backoff on drops; a second call
    // for the same session is a no-op.
    subscribeRaw(projectId: number, sessionId: string, nativeSessionId: string) {
      const key = readerKey(projectId, sessionId)
      if (readers.has(key)) return
      const controller = new AbortController()
      readers.set(key, { closed: false, controller })
      this.status[sessionId] = 'connecting'
      void this.pumpRaw(projectId, sessionId, nativeSessionId, controller.signal, key)
    },

    unsubscribeRaw(projectId: number, sessionId: string) {
      const key = readerKey(projectId, sessionId)
      const reader = readers.get(key)
      if (reader) {
        reader.closed = true
        reader.controller.abort()
        readers.delete(key)
      }
      this.status[sessionId] = 'idle'
    },

    unsubscribeAllRaw() {
      for (const [key, reader] of [...readers.entries()]) {
        reader.closed = true
        reader.controller.abort()
        readers.delete(key)
        void key
      }
    },

    handleRawEvent(sessionId: string, nativeSessionId: string, event: RawEvent) {
      // Raw events carry the native session id; platform ids never appear on
      // the wire. Drop anything for another session so instances stay
      // isolated in the UI.
      const owner = sessionIdOf(event)
      if (owner !== '' && owner !== nativeSessionId) return
      switch (event.type) {
        case 'permission.asked': {
          const requestId = requestIdOf(event)
          if (requestId === '') return
          const list = this.permissions[sessionId] ?? []
          if (list.some((entry) => entry.id === requestId)) return
          const data = (event.data ?? {}) as Record<string, unknown>
          list.push({
            id: requestId,
            sessionId,
            action: typeof data['action'] === 'string' ? (data['action'] as string) : 'tool',
            resources: Array.isArray(data['resources']) ? (data['resources'] as string[]) : [],
            message: typeof data['message'] === 'string' ? (data['message'] as string) : undefined,
          })
          this.permissions[sessionId] = list
          return
        }
        case 'permission.replied': {
          const requestId = requestIdOf(event)
          if (requestId === '') return
          this.permissions[sessionId] = (this.permissions[sessionId] ?? []).filter(
            (entry) => entry.id !== requestId,
          )
          return
        }
        case 'form.created': {
          const raw = event.data as unknown as {
            form?: { id?: string; title?: string; fields?: unknown }
          }
          const formId = raw?.form?.id ?? requestIdOf(event)
          if (formId === '') return
          const list = this.forms[sessionId] ?? []
          if (list.some((entry) => entry.id === formId)) return
          list.push({
            id: formId,
            sessionId,
            title: raw?.form?.title ?? '表单',
            fields: raw?.form?.fields,
          })
          this.forms[sessionId] = list
          return
        }
        case 'form.replied':
        case 'form.cancelled': {
          const formId = requestIdOf(event) || (event.data as unknown as { id?: string })?.id || ''
          if (formId === '') return
          this.forms[sessionId] = (this.forms[sessionId] ?? []).filter((entry) => entry.id !== formId)
          return
        }
        default:
          return
      }
    },

    async pumpRaw(
      projectId: number,
      sessionId: string,
      nativeSessionId: string,
      signal: AbortSignal,
      key: string,
    ) {
      let backoff = 200
      for (;;) {
        const entry = readers.get(key)
        if (!entry || entry.closed || signal.aborted) return
        try {
          const response = await fetch(rawEventsPath(projectId, sessionId), { signal })
          if (!response.ok || !response.body) throw new Error(`HTTP ${response.status}`)
          this.status[sessionId] = 'open'
          backoff = 200
          await this.readRawBody(response.body, (event) =>
            this.handleRawEvent(sessionId, nativeSessionId, event),
          )
          if (signal.aborted) return
        } catch (error) {
          if (signal.aborted) return
          void error
          this.status[sessionId] = 'error'
        }
        // Reconnect with capped backoff; the loop exits when unsubscribed.
        await new Promise((resolve) => setTimeout(resolve, backoff))
        backoff = Math.min(backoff * 2, 5000)
        if (!readers.has(key)) return
        this.status[sessionId] = 'connecting'
      }
    },

    async readRawBody(
      body: ReadableStream<Uint8Array>,
      onEvent: (event: RawEvent) => void,
    ): Promise<void> {
      const reader = body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      try {
        for (;;) {
          const { done, value } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })
          let boundary = buffer.indexOf('\n\n')
          while (boundary >= 0) {
            const block = buffer.slice(0, boundary)
            buffer = buffer.slice(boundary + 2)
            for (const line of block.split('\n')) {
              const trimmed = line.trimEnd()
              if (!trimmed.startsWith('data:')) continue
              const payload = trimmed.slice('data:'.length).trimStart()
              if (payload === '') continue
              try {
                onEvent(JSON.parse(payload) as RawEvent)
              } catch {
                // One malformed event never breaks the stream.
              }
            }
            boundary = buffer.indexOf('\n\n')
          }
        }
      } finally {
        reader.releaseLock()
      }
    },
  },
})
