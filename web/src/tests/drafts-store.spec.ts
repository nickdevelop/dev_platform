import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useDraftsStore } from '../stores/drafts'

describe('useDraftsStore', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
  })

  describe('newSessionDraft', () => {
    it('initializes with default empty draft', () => {
      const store = useDraftsStore()
      expect(store.newSessionDraft).toEqual({
        projectId: null,
        backend: '',
        model: '',
        promptId: '',
        messageText: '',
        attachedImages: [],
      })
    })

    it('persists changes to localStorage and loads them back', () => {
      const store = useDraftsStore()
      store.setNewSessionDraft({
        projectId: 2,
        backend: 'opencode-v2',
        messageText: 'Hello from draft',
      })

      expect(store.newSessionDraft.projectId).toBe(2)
      expect(store.newSessionDraft.backend).toBe('opencode-v2')
      expect(store.newSessionDraft.messageText).toBe('Hello from draft')

      // Verify persistence by creating a fresh store instance from localStorage
      setActivePinia(createPinia())
      const freshStore = useDraftsStore()
      expect(freshStore.newSessionDraft.projectId).toBe(2)
      expect(freshStore.newSessionDraft.backend).toBe('opencode-v2')
      expect(freshStore.newSessionDraft.messageText).toBe('Hello from draft')
    })

    it('clears draft back to defaults', () => {
      const store = useDraftsStore()
      store.setNewSessionDraft({
        projectId: 5,
        messageText: 'Draft text',
      })
      store.clearNewSessionDraft()

      expect(store.newSessionDraft.projectId).toBeNull()
      expect(store.newSessionDraft.messageText).toBe('')

      // LocalStorage should also be cleared
      setActivePinia(createPinia())
      const freshStore = useDraftsStore()
      expect(freshStore.newSessionDraft.projectId).toBeNull()
      expect(freshStore.newSessionDraft.messageText).toBe('')
    })
  })

  describe('sessionDrafts (per-session composer draft)', () => {
    it('returns empty draft when none exists for a session', () => {
      const store = useDraftsStore()
      const draft = store.getSessionDraft('ses-123')
      expect(draft).toEqual({
        messageText: '',
        attachedImages: [],
      })
    })

    it('saves draft per session and isolates different sessions', () => {
      const store = useDraftsStore()
      store.setSessionDraft('ses-1', { messageText: 'Session 1 text' })
      store.setSessionDraft('ses-2', { messageText: 'Session 2 text' })

      expect(store.getSessionDraft('ses-1').messageText).toBe('Session 1 text')
      expect(store.getSessionDraft('ses-2').messageText).toBe('Session 2 text')

      // Clear session 1 only
      store.clearSessionDraft('ses-1')
      expect(store.getSessionDraft('ses-1').messageText).toBe('')
      expect(store.getSessionDraft('ses-2').messageText).toBe('Session 2 text')
    })

    it('persists session drafts across store instances', () => {
      const store = useDraftsStore()
      store.setSessionDraft('ses-persist', { messageText: 'Will survive reload' })

      setActivePinia(createPinia())
      const freshStore = useDraftsStore()
      expect(freshStore.getSessionDraft('ses-persist').messageText).toBe('Will survive reload')
    })
  })

  describe('taskCreateDraft', () => {
    it('saves and clears task create draft', () => {
      const store = useDraftsStore()
      store.setTaskCreateDraft({
        title: 'Nightly build task',
        projectId: 3,
        customCronExpr: '0 2 * * *',
      })

      expect(store.taskCreateDraft.title).toBe('Nightly build task')
      expect(store.taskCreateDraft.projectId).toBe(3)

      store.clearTaskCreateDraft()
      expect(store.taskCreateDraft.title).toBe('')
      expect(store.taskCreateDraft.projectId).toBeNull()
    })
  })

  describe('newSessionDrafts per project isolation', () => {
    it('isolates new-session draft across different projects', () => {
      const store = useDraftsStore()
      store.setNewSessionDraft({ messageText: 'Draft for Project 1' }, 1)
      store.setNewSessionDraft({ messageText: 'Draft for Project 2' }, 2)

      expect(store.getNewSessionDraft(1).messageText).toBe('Draft for Project 1')
      expect(store.getNewSessionDraft(2).messageText).toBe('Draft for Project 2')
      expect(store.getNewSessionDraft(3).messageText).toBe('')

      store.clearNewSessionDraft(1)
      expect(store.getNewSessionDraft(1).messageText).toBe('')
      expect(store.getNewSessionDraft(2).messageText).toBe('Draft for Project 2')
    })
  })
})
