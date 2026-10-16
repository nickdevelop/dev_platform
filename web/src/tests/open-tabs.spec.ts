// The open-tab model: sessions and project-settings pages share one tab strip, in
// the order the user opened them, with one tab per session and one per project
// settings page. Tab identity is a single key, so closing, activating and
// rendering never have to know which kind they are dealing with.
import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { tabKey, useOpenTabsStore, type OpenTab } from '../stores/openTabs'

function freshStore() {
  setActivePinia(createPinia())
  return useOpenTabsStore()
}

describe('open tabs store', () => {
  beforeEach(() => localStorage.clear())

  it('gives each kind its own key', () => {
    const session: OpenTab = { kind: 'session', projectId: 3, sessionId: 'ses-1', title: 'x' }
    const settings: OpenTab = { kind: 'project-settings', projectId: 3 }
    expect(tabKey(session)).toBe('ses-1')
    expect(tabKey(settings)).toBe('settings:3')
  })

  it('opens one project-settings tab per project, whatever the user clicks', () => {
    const store = freshStore()
    store.openProjectSettings(3)
    store.openProjectSettings(3)
    expect(store.tabs).toHaveLength(1)
    expect(store.tabs[0]).toEqual({ kind: 'project-settings', projectId: 3 })
  })

  it('keeps sessions and settings side by side in opening order', () => {
    const store = freshStore()
    store.openTab(3, 'ses-1', 'first')
    store.openProjectSettings(3)
    store.openTab(3, 'ses-2', 'second')

    expect(store.tabs.map(tabKey)).toEqual(['ses-1', 'settings:3', 'ses-2'])
  })

  it('closes by key for either kind, and by session id for sessions', () => {
    const store = freshStore()
    store.openTab(3, 'ses-1', 'first')
    store.openProjectSettings(3)

    store.closeTabByKey('settings:3')
    expect(store.tabs.map(tabKey)).toEqual(['ses-1'])

    store.openProjectSettings(4)
    store.closeTab('ses-1')
    expect(store.tabs.map(tabKey)).toEqual(['settings:4'])
  })

  it('moves a settings tab to the front when it is opened again', () => {
    const store = freshStore()
    store.openProjectSettings(3)
    store.openTab(3, 'ses-1', 'first')
    store.openProjectSettings(3)
    expect(store.tabs.map(tabKey)).toEqual(['ses-1', 'settings:3'])
  })

  it('migrates the tabs a previous version stored', () => {
    localStorage.setItem(
      'dev-platform-open-tabs',
      JSON.stringify([{ projectId: 7, sessionId: 'ses-old', title: 'old session' }]),
    )
    const store = freshStore()

    expect(store.tabs).toEqual([{ kind: 'session', projectId: 7, sessionId: 'ses-old', title: 'old session' }])
    // The migrated set is written under the new key, so a later load never
    // sees the old shape again.
    expect(JSON.parse(localStorage.getItem('dev-platform-open-tabs-v2') ?? '[]')).toEqual([
      { kind: 'session', projectId: 7, sessionId: 'ses-old', title: 'old session' },
    ])
  })

  it('removes the legacy key after migrating, so it never comes back', () => {
    localStorage.setItem(
      'dev-platform-open-tabs',
      JSON.stringify([{ projectId: 7, sessionId: 'ses-old', title: 'old session' }]),
    )
    const store = freshStore()

    expect(store.tabs).toHaveLength(1)
    expect(localStorage.getItem('dev-platform-open-tabs')).toBeNull()
  })

  it('keeps an emptied strip empty instead of resurrecting legacy tabs', () => {
    // A user who migrated earlier still has the legacy key around; emptying
    // the strip must stay empty on the next load, not bring the old tabs back.
    localStorage.setItem(
      'dev-platform-open-tabs',
      JSON.stringify([{ projectId: 7, sessionId: 'ses-old', title: 'old session' }]),
    )
    localStorage.setItem('dev-platform-open-tabs-v2', '[]')
    expect(freshStore().tabs).toEqual([])

    localStorage.setItem(
      'dev-platform-open-tabs',
      JSON.stringify([{ projectId: 7, sessionId: 'ses-old', title: 'old session' }]),
    )
    const store = freshStore()
    store.openTab(3, 'ses-1', 'first')
    store.closeAllTabs()
    expect(store.tabs).toEqual([])

    const reloaded = freshStore()
    expect(reloaded.tabs).toEqual([])
  })

  it('restores what it persisted, and ignores a corrupt store', () => {
    const store = freshStore()
    store.openTab(3, 'ses-1', 'first')
    store.openProjectSettings(3)

    const reloaded = freshStore()
    expect(reloaded.tabs.map(tabKey)).toEqual(['ses-1', 'settings:3'])

    localStorage.setItem('dev-platform-open-tabs-v2', 'not json at all')
    expect(freshStore().tabs).toEqual([])
  })

  it('renames a session tab without touching the settings tab', () => {
    const store = freshStore()
    store.openTab(3, 'ses-1', '')
    store.openProjectSettings(3)
    store.updateTitle('ses-1', 'now titled')

    expect(store.tabs[0]).toEqual({ kind: 'session', projectId: 3, sessionId: 'ses-1', title: 'now titled' })
    expect(store.tabs[1]).toEqual({ kind: 'project-settings', projectId: 3 })
  })

  it('pins a tab to the front and spares it from bulk closes', () => {
    const store = freshStore()
    store.openTab(3, 'ses-1', 'first')
    store.openTab(3, 'ses-2', 'second')
    store.openProjectSettings(3)

    store.pinTab('ses-2')
    expect(store.tabs.map(tabKey)).toEqual(['ses-2', 'ses-1', 'settings:3'])
    expect(store.tabs[0].pinned).toBe(true)

    // Closing the others keeps the named tab and the pinned one.
    const closed = store.closeOtherTabsByKey('ses-1')
    expect(closed.map(tabKey)).toEqual(['settings:3'])
    expect(store.tabs.map(tabKey)).toEqual(['ses-2', 'ses-1'])

    // Closing all keeps only the pinned tab.
    const closedAll = store.closeAllTabs()
    expect(closedAll.map(tabKey)).toEqual(['ses-1'])
    expect(store.tabs.map(tabKey)).toEqual(['ses-2'])

    store.unpinTab('ses-2')
    expect(store.tabs[0].pinned).toBeUndefined()
  })

  it('restores a pinned tab with its flag', () => {
    const store = freshStore()
    store.openTab(3, 'ses-1', 'first')
    store.pinTab('ses-1')

    const reloaded = freshStore()
    expect(reloaded.tabs).toEqual([
      { kind: 'session', projectId: 3, sessionId: 'ses-1', title: 'first', pinned: true },
    ])
  })
})
