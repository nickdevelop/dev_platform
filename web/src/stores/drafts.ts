import { defineStore } from 'pinia'
import type { BackendId, ImageAttachment } from '../api/sessions'
import type { ScheduleType, TaskType } from '../api/tasks'

export interface NewSessionDraft {
  projectId: number | null
  backend: BackendId | ''
  model: string
  promptId: number | ''
  messageText: string
  attachedImages: ImageAttachment[]
}

export interface SessionDraft {
  messageText: string
  attachedImages: ImageAttachment[]
}

export interface TaskCreateDraft {
  taskType?: TaskType
  title: string
  description: string
  projectId: number | null
  backend: BackendId | ''
  model: string
  promptText: string
  promptId: number | ''
  scheduleType: ScheduleType
  scheduledAtDate: string
  cronShortcut: string
  customCronExpr: string
}

interface PersistedNewSessionDrafts {
  drafts: Record<string, NewSessionDraft>
  lastProjectId: number | null
}

const NEW_SESSION_STORAGE_KEY = 'dev-platform-new-session-drafts-v2'
const LEGACY_NEW_SESSION_STORAGE_KEY = 'dev-platform-new-session-draft'
const SESSION_DRAFTS_STORAGE_KEY = 'dev-platform-session-drafts'
const TASK_CREATE_STORAGE_KEY = 'dev-platform-task-create-draft'

function loadPersistedJson<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return fallback
    const parsed = JSON.parse(raw) as T
    return parsed ?? fallback
  } catch {
    return fallback
  }
}

function persistJson(key: string, data: unknown) {
  try {
    localStorage.setItem(key, JSON.stringify(data))
  } catch {
    // Best-effort persistence: e.g. localStorage quota exceeded
  }
}

function clearPersisted(key: string) {
  try {
    localStorage.removeItem(key)
  } catch {
    // Best-effort
  }
}

const defaultNewSessionDraft = (projectId: number | null = null): NewSessionDraft => ({
  projectId,
  backend: '',
  model: '',
  promptId: '',
  messageText: '',
  attachedImages: [],
})

const defaultTaskCreateDraft = (): TaskCreateDraft => ({
  taskType: 'standard',
  title: '',
  description: '',
  projectId: null,
  backend: '',
  model: '',
  promptText: '',
  promptId: '',
  scheduleType: 'manual',
  scheduledAtDate: '',
  cronShortcut: 'custom',
  customCronExpr: '0 9 * * *',
})

function draftKeyForProject(projectId?: number | null): string {
  return projectId !== null && projectId !== undefined ? `p-${projectId}` : 'global'
}

function loadInitialNewSessionDrafts(): { drafts: Record<string, NewSessionDraft>; lastProjectId: number | null } {
  const stored = loadPersistedJson<PersistedNewSessionDrafts | Record<string, NewSessionDraft> | null>(
    NEW_SESSION_STORAGE_KEY,
    null,
  )
  if (stored && typeof stored === 'object') {
    if ('drafts' in stored && typeof (stored as PersistedNewSessionDrafts).drafts === 'object') {
      const persisted = stored as PersistedNewSessionDrafts
      return {
        drafts: persisted.drafts ?? {},
        lastProjectId: persisted.lastProjectId ?? null,
      }
    }
    // If stored directly as record
    return { drafts: stored as Record<string, NewSessionDraft>, lastProjectId: null }
  }
  // Fallback to legacy single draft if present
  const legacy = loadPersistedJson<NewSessionDraft | null>(LEGACY_NEW_SESSION_STORAGE_KEY, null)
  if (legacy && typeof legacy === 'object') {
    const k = draftKeyForProject(legacy.projectId)
    return {
      drafts: { [k]: legacy },
      lastProjectId: legacy.projectId ?? null,
    }
  }
  return { drafts: {}, lastProjectId: null }
}

export const useDraftsStore = defineStore('drafts', {
  state: () => {
    const initialNewSession = loadInitialNewSessionDrafts()
    return {
      newSessionDrafts: initialNewSession.drafts,
      lastNewSessionProjectId: initialNewSession.lastProjectId,
      sessionDrafts: loadPersistedJson<Record<string, SessionDraft>>(
        SESSION_DRAFTS_STORAGE_KEY,
        {},
      ),
      taskCreateDraft: loadPersistedJson<TaskCreateDraft>(
        TASK_CREATE_STORAGE_KEY,
        defaultTaskCreateDraft(),
      ),
    }
  },

  getters: {
    // Backward compatibility getter: returns the active/last edited new-session draft
    newSessionDraft: (state): NewSessionDraft => {
      if (state.lastNewSessionProjectId !== null && state.lastNewSessionProjectId !== undefined) {
        const activeKey = draftKeyForProject(state.lastNewSessionProjectId)
        if (state.newSessionDrafts[activeKey]) {
          return state.newSessionDrafts[activeKey]
        }
      }
      if (state.newSessionDrafts['global']) {
        return state.newSessionDrafts['global']
      }
      // If any project draft exists, return the first one
      const keys = Object.keys(state.newSessionDrafts)
      if (keys.length > 0 && state.newSessionDrafts[keys[0]]) {
        return state.newSessionDrafts[keys[0]]
      }
      return defaultNewSessionDraft(null)
    },
  },

  actions: {
    // --- New Session Draft Actions (Per-Project & Global) ---
    getNewSessionDraft(projectId?: number | null): NewSessionDraft {
      const key = draftKeyForProject(projectId)
      return (
        this.newSessionDrafts[key] ??
        defaultNewSessionDraft(projectId ?? null)
      )
    },

    setNewSessionDraft(patch: Partial<NewSessionDraft>, explicitProjectId?: number | null) {
      const targetProjectId =
        explicitProjectId !== undefined
          ? explicitProjectId
          : patch.projectId !== undefined
            ? patch.projectId
            : this.lastNewSessionProjectId

      const key = draftKeyForProject(targetProjectId)
      const current = this.getNewSessionDraft(targetProjectId)
      const updated: NewSessionDraft = {
        ...current,
        ...patch,
        projectId: targetProjectId ?? null,
      }

      this.newSessionDrafts = {
        ...this.newSessionDrafts,
        [key]: updated,
      }
      if (targetProjectId !== null && targetProjectId !== undefined) {
        this.lastNewSessionProjectId = targetProjectId
      }

      persistJson(NEW_SESSION_STORAGE_KEY, {
        drafts: this.newSessionDrafts,
        lastProjectId: this.lastNewSessionProjectId,
      })
    },

    clearNewSessionDraft(projectId?: number | null) {
      const targetProjectId =
        projectId !== undefined ? projectId : this.lastNewSessionProjectId
      const key = draftKeyForProject(targetProjectId)

      if (this.newSessionDrafts[key]) {
        const next = { ...this.newSessionDrafts }
        delete next[key]
        this.newSessionDrafts = next
      }
      if (this.lastNewSessionProjectId === targetProjectId || projectId === undefined) {
        this.lastNewSessionProjectId = null
      }

      persistJson(NEW_SESSION_STORAGE_KEY, {
        drafts: this.newSessionDrafts,
        lastProjectId: this.lastNewSessionProjectId,
      })
    },

    // --- Existing Session Draft Actions (Per-Session) ---
    getSessionDraft(sessionId: string): SessionDraft {
      return (
        this.sessionDrafts[sessionId] ?? {
          messageText: '',
          attachedImages: [],
        }
      )
    },

    setSessionDraft(sessionId: string, patch: Partial<SessionDraft>) {
      const current = this.getSessionDraft(sessionId)
      const updated: SessionDraft = {
        ...current,
        ...patch,
      }

      // If draft is completely empty, clean it up to save space
      if (!updated.messageText && (!updated.attachedImages || updated.attachedImages.length === 0)) {
        const next = { ...this.sessionDrafts }
        delete next[sessionId]
        this.sessionDrafts = next
      } else {
        this.sessionDrafts = {
          ...this.sessionDrafts,
          [sessionId]: updated,
        }
      }
      persistJson(SESSION_DRAFTS_STORAGE_KEY, this.sessionDrafts)
    },

    clearSessionDraft(sessionId: string) {
      if (this.sessionDrafts[sessionId]) {
        const next = { ...this.sessionDrafts }
        delete next[sessionId]
        this.sessionDrafts = next
        persistJson(SESSION_DRAFTS_STORAGE_KEY, this.sessionDrafts)
      }
    },

    // --- Task Create Draft Actions ---
    setTaskCreateDraft(patch: Partial<TaskCreateDraft>) {
      this.taskCreateDraft = {
        ...this.taskCreateDraft,
        ...patch,
      }
      persistJson(TASK_CREATE_STORAGE_KEY, this.taskCreateDraft)
    },

    clearTaskCreateDraft() {
      this.taskCreateDraft = defaultTaskCreateDraft()
      clearPersisted(TASK_CREATE_STORAGE_KEY)
    },
  },
})
