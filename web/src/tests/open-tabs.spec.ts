// The open-tab model: sessions and project-settings pages share one tab strip, in
// the order the user opened them, with one tab per session and one per project
// settings page. Tab identity is a single key, so closing, activating and
// rendering never have to know which kind they are dealing with.
import { beforeEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SessionTabs from '../features/workspace/SessionTabs.vue'
import { tabKey, useOpenTabsStore, type OpenTab } from '../stores/openTabs'

function freshStore() {
  setActivePinia(createPinia())
  return useOpenTabsStore()
}

// sessionIdOf narrows the tab kinds the old assertions care about: only a
// session tab carries a session id.
function sessionIdOf(tab: OpenTab | null): string | null {
  return tab && tab.kind === 'session' ? tab.sessionId : null
}

describe('open tabs store', () => {
  beforeEach(() => localStorage.clear())

  it('gives each kind its own key', () => {
    const session: OpenTab = { kind: 'session', projectId: 3, sessionId: 'ses-1', title: 'x' }
    const settings: OpenTab = { kind: 'project-settings', projectId: 3 }
    expect(tabKey(session)).toBe('ses-1')
    expect(tabKey(settings)).toBe('settings:3')
  })

  it('opens one new-session tab however often its routes are visited', () => {
    const store = freshStore()
    store.openNewSession()
    store.openNewSession()
    expect(store.tabs).toHaveLength(1)
    expect(store.tabs[0]).toEqual({ kind: 'new-session' })
    expect(tabKey(store.tabs[0])).toBe('new-session')
  })

  it('opens one task tab per task and refreshes its title from the task list', () => {
    const store = freshStore()
    store.openTask(5, '')
    store.openTask(5, '构建部署')
    store.openTask(6, '另一个')
    expect(store.tabs.map(tabKey)).toEqual(['task:5', 'task:6'])
    expect(store.tabs[0]).toEqual({ kind: 'task', taskId: 5, title: '构建部署' })

    store.updateTaskTitle(6, '改名后')
    expect((store.tabs[1] as Extract<OpenTab, { kind: 'task' }>).title).toBe('改名后')
  })

  it('opens one platform-settings tab however often the categories are visited', () => {
    const store = freshStore()
    store.openPlatformSettings()
    store.openPlatformSettings()
    expect(store.tabs).toEqual([{ kind: 'platform-settings' }])
    expect(tabKey(store.tabs[0])).toBe('platform-settings')
  })

  it('replaces the new-session tab in place with the created session tab', () => {
    const store = freshStore()
    store.openTab(3, 'ses-1', 'first')
    store.openNewSession()

    store.promoteNewSession(3, 'ses-new')
    // The created session takes the strip position the new tab held, and the
    // focus moves to it; the title arrives later via updateTitle.
    expect(store.tabs.map(tabKey)).toEqual(['ses-1', 'ses-new'])
    expect(store.tabs[1]).toEqual({ kind: 'session', projectId: 3, sessionId: 'ses-new', title: '' })
    expect(store.lastActiveKey).toBe('ses-new')

    store.updateTitle('ses-new', '派生的任务名')
    expect((store.tabs[1] as Extract<OpenTab, { kind: 'session' }>).title).toBe('派生的任务名')
  })

  it('still opens a plain session tab when promoting without a new tab', () => {
    const store = freshStore()
    store.promoteNewSession(3, 'ses-x')
    expect(store.tabs).toEqual([{ kind: 'session', projectId: 3, sessionId: 'ses-x', title: '' }])
  })

  it('persists and restores the new kinds', () => {
    const store = freshStore()
    store.openNewSession()
    store.openTask(5, '构建部署')
    store.openPlatformSettings()
    store.setActiveKey('task:5')

    const reloaded = freshStore()
    expect(reloaded.tabs).toEqual([
      { kind: 'new-session' },
      { kind: 'task', taskId: 5, title: '构建部署' },
      { kind: 'platform-settings' },
    ])
    expect(reloaded.preferredTab ? tabKey(reloaded.preferredTab) : null).toBe('task:5')
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

  it('computes preferredTab as last active tab or first tab, and null when empty', () => {
    const store = freshStore()
    expect(store.preferredTab).toBeNull()

    // 1. When tabs exist but no explicit lastActiveKey set: defaults to first tab
    store.openTab(3, 'ses-1', 'first')
    store.openTab(3, 'ses-2', 'second')
    // openTab automatically marks the newly opened tab as active: ses-2
    expect(sessionIdOf(store.preferredTab)).toBe('ses-2')

    // 2. Setting active key explicitly
    store.setActiveKey('ses-1')
    expect(sessionIdOf(store.preferredTab)).toBe('ses-1')

    // 3. If active tab is closed, preferredTab falls back to remaining
    store.closeTab('ses-1')
    expect(sessionIdOf(store.preferredTab)).toBe('ses-2')

    // 4. Closing all tabs clears preferredTab
    store.closeAllTabs()
    expect(store.preferredTab).toBeNull()
  })

  it('persists lastActiveKey and restores it on next load', () => {
    const store = freshStore()
    store.openTab(3, 'ses-1', 'first')
    store.openTab(3, 'ses-2', 'second')
    store.setActiveKey('ses-1')

    const reloaded = freshStore()
    expect(reloaded.lastActiveKey).toBe('ses-1')
    expect(sessionIdOf(reloaded.preferredTab)).toBe('ses-1')
    expect(reloaded.preferredTab ? tabKey(reloaded.preferredTab) : null).toBe('ses-1')
  })
})

describe('SessionTabs component', () => {
  it('renders a plus button to add new session and emits newTab', async () => {
    const tabs: OpenTab[] = [
      { kind: 'session', projectId: 1, sessionId: 'ses-1', title: '会话 1' },
    ]
    const wrapper = mount(SessionTabs, {
      props: {
        tabs,
        activeKey: 'ses-1',
      },
    })

    const addBtn = wrapper.find('[data-test="tab-add-session"]')
    expect(addBtn.exists()).toBe(true)
    expect(addBtn.text()).toBe('+')

    await addBtn.trigger('click')
    expect(wrapper.emitted('newTab')).toHaveLength(1)
  })

  it('labels the new-session tab, task tabs and platform settings', () => {
    const tabs: OpenTab[] = [
      { kind: 'new-session' },
      { kind: 'task', taskId: 5, title: '构建部署' },
      { kind: 'platform-settings' },
    ]
    const wrapper = mount(SessionTabs, {
      props: {
        tabs,
        activeKey: 'new-session',
      },
    })

    const labels = wrapper.findAll('[data-test="open-tab"] .tab-label').map((node) => node.text())
    expect(labels).toEqual(['new', '构建部署', '设置'])
  })
})

