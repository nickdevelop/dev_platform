// Pinia store for the pending interactions of open sessions: tool confirmations
// and question forms. The session's normalized stream is the only live source:
// a requested interaction adds its card, a resolved one removes it, and the
// snapshot a stream opens with replaces the whole pending set. Replies go
// through the reply endpoints, and a successful reply drops its card at once.

import { defineStore } from 'pinia'
import {
  replyFormInfo,
  replyPermissionRequest,
  type FormInfo,
  type PermissionDecision,
  type PermissionRequest,
} from '../api/interactions'
import type { InteractionPayload, InteractionSnapshot } from '../api/sessions'
import { describeApiError, UnauthorizedError, ApiError } from '../api/client'
import { useAuthStore } from './auth'

export const useInteractionsStore = defineStore('interactions', {
  state: () => ({
    permissions: {} as Record<string, PermissionRequest[]>,
    forms: {} as Record<string, FormInfo[]>,
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
    // applySnapshot replaces a session's pending set with the one its stream
    // reported at the join point.
    applySnapshot(sessionId: string, snapshot: InteractionSnapshot) {
      this.permissions[sessionId] = snapshot.permissions ?? []
      this.forms[sessionId] = snapshot.forms ?? []
    },

    // applyInteraction folds one interaction boundary from the stream into the
    // pending set. Both phases are idempotent, so a request re-sent by a resumed
    // stream never duplicates a card.
    applyInteraction(sessionId: string, payload: InteractionPayload) {
      if (payload.kind === 'permission') {
        const list = this.permissions[sessionId] ?? []
        if (payload.phase === 'resolved') {
          this.permissions[sessionId] = list.filter((entry) => entry.id !== payload.id)
          return
        }
        if (!payload.permission || list.some((entry) => entry.id === payload.id)) return
        this.permissions[sessionId] = [...list, payload.permission]
        return
      }
      const list = this.forms[sessionId] ?? []
      if (payload.phase === 'resolved') {
        this.forms[sessionId] = list.filter((entry) => entry.id !== payload.id)
        return
      }
      if (!payload.form || list.some((entry) => entry.id === payload.id)) return
      this.forms[sessionId] = [...list, payload.form]
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
          // The request is gone server-side (answered, cancelled or timed out):
          // drop the stale control instead of leaving an unanswerable 允许/拒绝.
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
  },
})
