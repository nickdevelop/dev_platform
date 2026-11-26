import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import NotesSidebar from '../features/notes/NotesSidebar.vue'
import NoteItem from '../features/notes/NoteItem.vue'
import { useNotesStore } from '../stores/notes'
import { useProjectsStore } from '../stores/projects'
import * as notesApi from '../api/notes'
import * as projectsApi from '../api/projects'

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

  it('renders a note row with its snippet, project and time', () => {
    setActivePinia(createPinia())
    const projectsStore = useProjectsStore()
    projectsStore.projects = [
      { id: 10, name: '微服务重构', path: '/srv/micro', isGit: false },
    ]

    const wrapper = mount(NoteItem, {
      props: {
        note: {
          id: 2,
          projectId: 10,
          title: '拆分计划',
          content: '\n第一步：抽出网关\n第二步：下沉配置',
          createdAt: '',
          updatedAt: new Date().toISOString(),
        },
      },
    })

    expect(wrapper.find('.note-title').text()).toBe('拆分计划')
    // The snippet is the first non-empty line, not the whole body.
    expect(wrapper.find('.note-snippet').text()).toBe('第一步：抽出网关')
    expect(wrapper.find('.note-project').text()).toBe('微服务重构')
    // A bound project is told apart from a global note.
    expect(wrapper.find('.note-project--bound').exists()).toBe(true)
    expect(wrapper.find('.note-time').text()).toMatch(/\d{2}:\d{2}/)
  })

  it('keeps the delete click out of the menu item', async () => {
    setActivePinia(createPinia())
    const wrapper = mount(NoteItem, {
      props: {
        note: { id: 3, projectId: 0, title: '全局备忘', content: '', createdAt: '', updatedAt: '' },
      },
    })

    expect(wrapper.find('.note-project').text()).toBe('全局')
    await wrapper.find('[data-test="note-delete"]').trigger('click')
    expect(wrapper.emitted('delete')?.[0]?.[0]).toMatchObject({ id: 3 })
  })
})

describe('NotesSidebar', () => {
  const notes: notesApi.Note[] = [
    { id: 1, projectId: 0, title: '全局备忘', content: 'x', createdAt: '', updatedAt: '' },
    { id: 2, projectId: 7, title: '部署清单', content: 'y', createdAt: '', updatedAt: '' },
  ]

  async function mountSidebar(path = '/notes/2') {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/notes', component: { template: '<div />' } },
        { path: '/notes/:noteId', component: { template: '<div />' } },
      ],
    })
    await router.push(path)
    await router.isReady()

    const wrapper = mount(NotesSidebar, { global: { plugins: [router] } })
    await flushPromises()
    return wrapper
  }

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
    vi.spyOn(projectsApi, 'listProjects').mockResolvedValue({
      projects: [{ id: 7, name: '网关', path: '/srv/gateway', isGit: false }],
    })
    vi.spyOn(notesApi, 'listNotes').mockResolvedValue(notes)
  })

  it('lists notes as a router-driven el-menu with the open note active', async () => {
    const wrapper = await mountSidebar()

    const items = wrapper.findAll('.el-menu-item')
    expect(items).toHaveLength(2)
    expect(items[0].text()).toContain('全局备忘')

    const active = wrapper.findAll('.el-menu-item.is-active')
    expect(active).toHaveLength(1)
    expect(active[0].text()).toContain('部署清单')
  })

  it('separates a filtered-out list from an empty library', async () => {
    const wrapper = await mountSidebar()
    const store = useNotesStore()

    // Nothing at all yet: the empty state offers the primary action.
    store.notes = []
    await flushPromises()
    expect(wrapper.find('[data-test="list-state-empty"]').exists()).toBe(true)

    // Rows exist but the keyword hid them: the way back out is the point.
    store.notes = notes
    store.setSearchQuery('不存在的关键字')
    await flushPromises()
    const filtered = wrapper.find('[data-test="list-state-filtered"]')
    expect(filtered.exists()).toBe(true)
    expect(filtered.text()).toContain('重置筛选')
  })

  it('filters notes live from the sidebar', async () => {
    const wrapper = await mountSidebar()

    // `el-input` has inheritAttrs: false and forwards the attrs to its inner
    // <input>, so the data-test hook *is* the input element.
    await wrapper.find('[data-test="notes-search"]').setValue('部署')
    expect(wrapper.findAll('.el-menu-item')).toHaveLength(1)
  })
})
