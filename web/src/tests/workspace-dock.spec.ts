import { describe, expect, it, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useWorkspaceDockStore, buildFileTabKey, FILE_TREE_TAB_KEY } from '../stores/workspaceDock'

describe('WorkspaceDockStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  it('initializes with default state', () => {
    const store = useWorkspaceDockStore()
    expect(store.openTabs).toEqual([])
    expect(store.activeTabKey).toBe(FILE_TREE_TAB_KEY)
    expect(store.isLocked).toBe(false)
    expect(store.effectiveScope).toBeNull()
  })

  it('opens a file tab and handles duplicate open', () => {
    const store = useWorkspaceDockStore()
    store.setSessionScope(1, 0)

    const tab1 = store.openFile({ projectId: 1, worktreeId: 0, path: 'src/main.ts' })
    expect(store.openTabs.length).toBe(1)
    expect(store.activeTabKey).toBe(tab1.key)
    expect(tab1.name).toBe('main.ts')
    expect(store.isTabForeign(tab1)).toBe(false)

    // Opening same file updates active tab
    const tab2 = store.openFile({ projectId: 1, worktreeId: 0, path: 'src/main.ts', initialLine: 10 })
    expect(store.openTabs.length).toBe(1)
    expect(tab2.initialLine).toBe(10)
  })

  it('marks foreign tabs when scope changes', () => {
    const store = useWorkspaceDockStore()
    store.setSessionScope(1, 0)

    const tabA = store.openFile({ projectId: 1, worktreeId: 0, path: 'src/a.ts' })
    expect(store.isTabForeign(tabA)).toBe(false)

    // Switch to another project or worktree
    store.setSessionScope(2, 0)
    expect(store.isTabForeign(tabA)).toBe(true)

    const tabB = store.openFile({ projectId: 2, worktreeId: 0, path: 'src/b.ts' })
    expect(store.isTabForeign(tabB)).toBe(false)
    expect(store.openTabs.length).toBe(2)
  })

  it('supports locking the workspace scope', () => {
    const store = useWorkspaceDockStore()
    store.setSessionScope(1, 0)
    expect(store.effectiveScope).toEqual({ projectId: 1, worktreeId: 0 })

    store.toggleLock()
    expect(store.isLocked).toBe(true)
    expect(store.lockedScope).toEqual({ projectId: 1, worktreeId: 0 })

    // When session switches to project 2, effective scope remains locked to project 1
    store.setSessionScope(2, 0)
    expect(store.activeScope).toEqual({ projectId: 2, worktreeId: 0 })
    expect(store.effectiveScope).toEqual({ projectId: 1, worktreeId: 0 })

    // Unlock
    store.toggleLock()
    expect(store.isLocked).toBe(false)
    expect(store.effectiveScope).toEqual({ projectId: 2, worktreeId: 0 })
  })

  it('manages drafts and dirty status per tab', () => {
    const store = useWorkspaceDockStore()
    const tab = store.openFile({ projectId: 1, path: 'src/test.ts' })

    expect(tab.isDirty).toBe(false)
    store.setDraft(tab.key, 'const x = 1;', true)
    expect(tab.draftContent).toBe('const x = 1;')
    expect(tab.isDirty).toBe(true)
    expect(store.hasDirtyTabs).toBe(true)

    store.markSaved(tab.key)
    expect(tab.isDirty).toBe(false)
    expect(store.hasDirtyTabs).toBe(false)
  })

  it('closes tab and selects next tab or fallback to filetree', () => {
    const store = useWorkspaceDockStore()
    const t1 = store.openFile({ projectId: 1, path: '1.txt' })
    const t2 = store.openFile({ projectId: 1, path: '2.txt' })

    expect(store.activeTabKey).toBe(t2.key)
    store.closeTab(t2.key)
    expect(store.openTabs.length).toBe(1)
    expect(store.activeTabKey).toBe(t1.key)

    store.closeTab(t1.key)
    expect(store.openTabs.length).toBe(0)
    expect(store.activeTabKey).toBe(FILE_TREE_TAB_KEY)
  })
})
