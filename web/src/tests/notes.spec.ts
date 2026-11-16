import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useNotesStore } from '../stores/notes'
import * as notesApi from '../api/notes'

describe('useNotesStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('filters notes by search query and project id', () => {
    const store = useNotesStore()
    store.notes = [
      { id: 1, projectId: 0, title: 'Global Quick Idea', content: 'draft details', createdAt: '', updatedAt: '' },
      { id: 2, projectId: 10, title: 'Project Alpha Arch', content: 'microservices', createdAt: '', updatedAt: '' },
      { id: 3, projectId: 20, title: 'Project Beta Memo', content: 'release checklist', createdAt: '', updatedAt: '' },
    ]

    // Initial state: all
    expect(store.filteredNotes).toHaveLength(3)

    // Filter by project
    store.setSelectedProjectId(10)
    expect(store.filteredNotes).toHaveLength(1)
    expect(store.filteredNotes[0].id).toBe(2)

    // Filter by global
    store.setSelectedProjectId(0)
    expect(store.filteredNotes).toHaveLength(1)
    expect(store.filteredNotes[0].id).toBe(1)

    // Clear project filter, set search query
    store.setSelectedProjectId(null)
    store.setSearchQuery('checklist')
    expect(store.filteredNotes).toHaveLength(1)
    expect(store.filteredNotes[0].id).toBe(3)
  })

  it('creates new note and prepends to list', async () => {
    const store = useNotesStore()
    const mockCreated: notesApi.Note = {
      id: 99,
      projectId: 0,
      title: '无标题笔记',
      content: '',
      createdAt: '2026-11-15T00:00:00Z',
      updatedAt: '2026-11-15T00:00:00Z',
    }
    vi.spyOn(notesApi, 'createNote').mockResolvedValue(mockCreated)

    const result = await store.createNewNote()
    expect(result).toEqual(mockCreated)
    expect(store.notes[0]).toEqual(mockCreated)
    expect(store.activeNoteId).toBe(99)
    expect(store.activeNote).toEqual(mockCreated)
  })

  it('updates note and reflects in list', async () => {
    const store = useNotesStore()
    store.notes = [
      { id: 5, projectId: 0, title: 'Original', content: 'old content', createdAt: '', updatedAt: '' },
    ]
    store.activeNoteId = 5

    const updatedNote: notesApi.Note = {
      id: 5,
      projectId: 1,
      title: 'New Title',
      content: 'new content',
      createdAt: '',
      updatedAt: '2026-11-15T01:00:00Z',
    }
    vi.spyOn(notesApi, 'updateNote').mockResolvedValue(updatedNote)

    const res = await store.saveNote(5, { title: 'New Title', content: 'new content', projectId: 1 })
    expect(res).toEqual(updatedNote)
    expect(store.notes[0].title).toBe('New Title')
    expect(store.notes[0].projectId).toBe(1)
    expect(store.activeNote?.title).toBe('New Title')
  })

  it('removes note and recovers active note selection', async () => {
    const store = useNotesStore()
    store.notes = [
      { id: 1, projectId: 0, title: 'First', content: '', createdAt: '', updatedAt: '' },
      { id: 2, projectId: 0, title: 'Second', content: '', createdAt: '', updatedAt: '' },
    ]
    store.activeNoteId = 1
    vi.spyOn(notesApi, 'deleteNote').mockResolvedValue()

    const ok = await store.removeNote(1)
    expect(ok).toBe(true)
    expect(store.notes).toHaveLength(1)
    expect(store.notes[0].id).toBe(2)
    expect(store.activeNoteId).toBe(2)
  })

  it('persists and restores view mode', () => {
    const store = useNotesStore()
    expect(store.viewMode).toBe('split')

    store.setViewMode('edit')
    expect(store.viewMode).toBe('edit')
    expect(localStorage.getItem('dev-platform-note-view-mode-v1')).toBe('edit')
  })
})

describe('notes list rendering', () => {
  it('gets markdown typography from the shared global stylesheet', () => {
    const main = readFileSync(resolve(__dirname, '../main.ts'), 'utf8')
    expect(main).toContain("import './shared/markdown-body.css'")

    const css = readFileSync(resolve(__dirname, '../shared/markdown-body.css'), 'utf8')
    expect(css).toMatch(/\.markdown-body h1 \{/)
    expect(css).toMatch(/\.markdown-body ul \{[^}]*list-style-type:\s*disc/)
    expect(css).toMatch(/\.markdown-body ol \{[^}]*list-style-type:\s*decimal/)
  })

  it('left-aligns the note card snippet and meta row', () => {
    const sfc = readFileSync(resolve(__dirname, '../features/notes/NoteItem.vue'), 'utf8')
    const bottom = sfc.match(/\.note-bottom \{([^}]*)\}/)?.[1] ?? ''
    // The shared .sidebar-card-row-bottom centers its children, which reads as
    // centered text inside this vertical stack, so the card must undo it.
    expect(bottom).toMatch(/align-items:\s*stretch/)
    expect(bottom).toMatch(/text-align:\s*left/)

    const meta = sfc.match(/\.note-meta \{([^}]*)\}/)?.[1] ?? ''
    expect(meta).toMatch(/width:\s*100%/)
  })
})
