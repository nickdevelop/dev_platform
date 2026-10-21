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

const NEW_SESSION_STORAGE_KEY = 'dev-platform-new-session-draft'
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

const defaultNewSessionDraft = (): NewSessionDraft => ({
  projectId: null,
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

export const useDraftsStore = defineStore('drafts', {
  state: () => ({
    newSessionDraft: loadPersistedJson<NewSessionDraft>(
      NEW_SESSION_STORAGE_KEY,
      defaultNewSessionDraft(),
    ),
    sessionDrafts: loadPersistedJson<Record<string, SessionDraft>>(
      SESSION_DRAFTS_STORAGE_KEY,
      {},
    ),
    taskCreateDraft: loadPersistedJson<TaskCreateDraft>(
      TASK_CREATE_STORAGE_KEY,
      defaultTaskCreateDraft(),
    ),
  }),

  actions: {
    // --- New Session Draft Actions ---
    setNewSessionDraft(patch: Partial<NewSessionDraft>) {
      this.newSessionDraft = {
        ...this.newSessionDraft,
        ...patch,
      }
      persistJson(NEW_SESSION_STORAGE_KEY, this.newSessionDraft)
    },

    clearNewSessionDraft() {
      this.newSessionDraft = defaultNewSessionDraft()
      clearPersisted(NEW_SESSION_STORAGE_KEY)
    },

    // --- Existing Session Draft Actions ---
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
