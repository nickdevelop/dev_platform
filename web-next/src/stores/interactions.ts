import { create } from 'zustand'
import { ApiError, describeApiError, UnauthorizedError } from '../api/client'
import {
  replyFormInfo,
  replyPermissionRequest,
  type FormInfo,
  type PermissionDecision,
  type PermissionRequest,
} from '../api/interactions'
import type { InteractionPayload, InteractionSnapshot } from '../api/sessions'

interface InteractionsState {
  permissions: Record<string, PermissionRequest[]>
  forms: Record<string, FormInfo[]>
  error: string
  applySnapshot: (sessionId: string, snapshot: InteractionSnapshot) => void
  applyInteraction: (sessionId: string, payload: InteractionPayload) => void
  replyPermission: (
    projectId: number,
    sessionId: string,
    requestId: string,
    decision: PermissionDecision,
  ) => Promise<boolean>
  replyForm: (
    projectId: number,
    sessionId: string,
    formId: string,
    answer: Record<string, unknown>,
  ) => Promise<boolean>
}

export const useInteractionsStore = create<InteractionsState>((set) => ({
  permissions: {},
  forms: {},
  error: '',

  applySnapshot: (sessionId, snapshot) => {
    set((state) => ({
      permissions: { ...state.permissions, [sessionId]: snapshot.permissions ?? [] },
      forms: { ...state.forms, [sessionId]: snapshot.forms ?? [] },
    }))
  },

  applyInteraction: (sessionId, payload) => {
    if (payload.kind === 'permission') {
      set((state) => {
        const list = state.permissions[sessionId] ?? []
        if (payload.phase === 'resolved') {
          return {
            permissions: {
              ...state.permissions,
              [sessionId]: list.filter((entry) => entry.id !== payload.id),
            },
          }
        }
        if (!payload.permission || list.some((entry) => entry.id === payload.id)) return state
        return {
          permissions: { ...state.permissions, [sessionId]: [...list, payload.permission] },
        }
      })
      return
    }
    set((state) => {
      const list = state.forms[sessionId] ?? []
      if (payload.phase === 'resolved') {
        return { forms: { ...state.forms, [sessionId]: list.filter((entry) => entry.id !== payload.id) } }
      }
      if (!payload.form || list.some((entry) => entry.id === payload.id)) return state
      return { forms: { ...state.forms, [sessionId]: [...list, payload.form] } }
    })
  },

  replyPermission: async (projectId, sessionId, requestId, decision) => {
    try {
      await replyPermissionRequest(projectId, sessionId, requestId, { decision })
      set((state) => ({
        permissions: {
          ...state.permissions,
          [sessionId]: (state.permissions[sessionId] ?? []).filter((entry) => entry.id !== requestId),
        },
        error: '',
      }))
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      if (error instanceof ApiError && error.code === 'permission_expired') {
        set((state) => ({
          permissions: {
            ...state.permissions,
            [sessionId]: (state.permissions[sessionId] ?? []).filter((entry) => entry.id !== requestId),
          },
        }))
      }
      set({ error: describeApiError(error) })
      return false
    }
  },

  replyForm: async (projectId, sessionId, formId, answer) => {
    try {
      await replyFormInfo(projectId, sessionId, formId, { answer })
      set((state) => ({
        forms: {
          ...state.forms,
          [sessionId]: (state.forms[sessionId] ?? []).filter((entry) => entry.id !== formId),
        },
        error: '',
      }))
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      if (error instanceof ApiError && error.code === 'form_expired') {
        set((state) => ({
          forms: {
            ...state.forms,
            [sessionId]: (state.forms[sessionId] ?? []).filter((entry) => entry.id !== formId),
          },
        }))
      }
      set({ error: describeApiError(error) })
      return false
    }
  },
}))

export function permissionsFor(sessionId: string): PermissionRequest[] {
  return useInteractionsStore.getState().permissions[sessionId] ?? []
}

export function formsFor(sessionId: string): FormInfo[] {
  return useInteractionsStore.getState().forms[sessionId] ?? []
}
