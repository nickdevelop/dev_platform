// Store for Note Management.
import { defineStore } from 'pinia'
import { describeApiError, UnauthorizedError } from '../api/client'
import {
  listNotes,
  getNote,
  createNote,
  updateNote,
  deleteNote,
  type Note,
  type CreateNotePayload,
  type UpdateNotePayload,
} from '../api/notes'

export type NoteViewMode = 'split' | 'edit' | 'preview'

const NOTE_VIEW_MODE_KEY = 'dev-platform-note-view-mode-v1'

export const useNotesStore = defineStore('notes', {
  state: () => {
    let initialViewMode: NoteViewMode = 'split'
    try {
      const saved = localStorage.getItem(NOTE_VIEW_MODE_KEY)
      if (saved === 'split' || saved === 'edit' || saved === 'preview') {
        initialViewMode = saved
      }
    } catch {
      initialViewMode = 'split'
    }

    return {
      notes: [] as Note[],
      activeNoteId: null as number | null,
      currentNote: null as Note | null,
      loading: false,
      saving: false,
      error: '',
      searchQuery: '',
      selectedProjectId: null as number | null, // null: 全部, 0: 全局笔记, >0: 对应项目
      viewMode: initialViewMode,
    }
  },

  getters: {
    filteredNotes: (state): Note[] => {
      let list = state.notes
      if (state.selectedProjectId !== null) {
        list = list.filter((n) => n.projectId === state.selectedProjectId)
      }
      if (state.searchQuery.trim()) {
        const q = state.searchQuery.trim().toLowerCase()
        list = list.filter(
          (n) =>
            n.title.toLowerCase().includes(q) ||
            n.content.toLowerCase().includes(q),
        )
      }
      return list
    },
    activeNote: (state): Note | null => {
      if (!state.activeNoteId) return null
      if (state.currentNote && state.currentNote.id === state.activeNoteId) {
        return state.currentNote
      }
      return state.notes.find((n) => n.id === state.activeNoteId) ?? null
    },
  },

  actions: {
    setViewMode(mode: NoteViewMode) {
      this.viewMode = mode
      try {
        localStorage.setItem(NOTE_VIEW_MODE_KEY, mode)
      } catch {
        // Persistence is best-effort.
      }
    },

    setSearchQuery(q: string) {
      this.searchQuery = q
    },

    setSelectedProjectId(projId: number | null) {
      this.selectedProjectId = projId
    },

    async loadNotes() {
      this.loading = true
      try {
        const projectIdParam =
          this.selectedProjectId !== null ? this.selectedProjectId : undefined
        this.notes = await listNotes(this.searchQuery, projectIdParam)
        this.error = ''

        // If an active note is chosen, make sure it is updated
        if (this.activeNoteId) {
          const found = this.notes.find((n) => n.id === this.activeNoteId)
          if (found) {
            this.currentNote = found
          } else if (this.notes.length > 0) {
            this.activeNoteId = this.notes[0].id
            this.currentNote = this.notes[0]
          } else {
            this.activeNoteId = null
            this.currentNote = null
          }
        } else if (this.notes.length > 0) {
          this.activeNoteId = this.notes[0].id
          this.currentNote = this.notes[0]
        }
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      } finally {
        this.loading = false
      }
    },

    async selectNote(id: number | null) {
      this.activeNoteId = id
      if (!id) {
        this.currentNote = null
        return
      }
      const existing = this.notes.find((n) => n.id === id)
      if (existing) {
        this.currentNote = existing
      }
      try {
        const note = await getNote(id)
        this.currentNote = note
        const idx = this.notes.findIndex((n) => n.id === id)
        if (idx !== -1) {
          this.notes[idx] = note
        } else {
          this.notes.unshift(note)
        }
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      }
    },

    async createNewNote(payload?: CreateNotePayload): Promise<Note | null> {
      this.loading = true
      try {
        const defaultProjId =
          payload?.projectId !== undefined
            ? payload.projectId
            : this.selectedProjectId !== null && this.selectedProjectId > 0
              ? this.selectedProjectId
              : 0

        const note = await createNote({
          title: payload?.title || '无标题笔记',
          content: payload?.content || '',
          projectId: defaultProjId,
        })
        this.notes.unshift(note)
        this.activeNoteId = note.id
        this.currentNote = note
        this.error = ''
        return note
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        throw error
      } finally {
        this.loading = false
      }
    },

    async saveNote(id: number, payload: UpdateNotePayload): Promise<Note | null> {
      this.saving = true
      try {
        const updated = await updateNote(id, payload)
        this.currentNote = updated
        const idx = this.notes.findIndex((n) => n.id === id)
        if (idx !== -1) {
          this.notes[idx] = updated
        }
        return updated
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        throw error
      } finally {
        this.saving = false
      }
    },

    async removeNote(id: number): Promise<boolean> {
      try {
        await deleteNote(id)
        this.notes = this.notes.filter((n) => n.id !== id)
        if (this.activeNoteId === id) {
          if (this.notes.length > 0) {
            this.activeNoteId = this.notes[0].id
            this.currentNote = this.notes[0]
          } else {
            this.activeNoteId = null
            this.currentNote = null
          }
        }
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.error = describeApiError(error)
        throw error
      }
    },
  },
})
