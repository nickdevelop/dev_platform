import { describe, it, expect, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import ProjectSidebar from '../features/projects/ProjectSidebar.vue'
import ChatSidebar from '../features/chat/pure/ChatSidebar.vue'
import TasksSidebar from '../features/tasks/TasksSidebar.vue'
import GroupsSidebar from '../features/groups/GroupsSidebar.vue'
import NotesSidebar from '../features/notes/NotesSidebar.vue'
import SettingsNav from '../features/settings/SettingsNav.vue'
import EnginesSidebar from '../features/engines/EnginesSidebar.vue'
import OrgSidebar from '../features/organization/OrgSidebar.vue'

describe('Column 2 (Sidebar) Design Consistency across menus', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('ProjectSidebar conforms to Data Terminal 3-zone standard', () => {
    const wrapper = mount(ProjectSidebar, {
      props: {
        activeProjectId: null,
        activeSessionId: null,
      },
    })

    // Zone A: Actions
    expect(wrapper.find('.sidebar-actions').exists()).toBe(true)
    expect(wrapper.find('.action-btn.register').exists()).toBe(false)
    expect(wrapper.find('.action-btn.create').exists()).toBe(true)
    expect(wrapper.find('.action-btn.project-settings-btn').exists()).toBe(true)

    // Zone B: Search with inset icon and filter
    expect(wrapper.find('.sidebar-search-bar').exists()).toBe(true)
    expect(wrapper.find('.sidebar-search-icon').exists()).toBe(true)
    expect(wrapper.find('.sidebar-filter-bar').exists()).toBe(true)
    expect(wrapper.find('[data-test="show-task-sessions"]').exists()).toBe(true)

    // Zone C: Tree container
    expect(wrapper.find('.tree').exists()).toBe(true)
  })

  it('ChatSidebar conforms to Data Terminal 3-zone standard', () => {
    const wrapper = mount(ChatSidebar)

    // Zone A: Action
    expect(wrapper.find('.sidebar-actions').exists()).toBe(true)
    expect(wrapper.find('[data-test="chat-new-group-btn"]').exists()).toBe(true)

    // Zone B: Search with inset icon
    expect(wrapper.find('.sidebar-search-bar').exists()).toBe(true)
    expect(wrapper.find('.sidebar-search-icon').exists()).toBe(true)

    // Zone C: Assistant list
    expect(wrapper.find('[data-test="assistant-list"]').exists()).toBe(true)
  })

  it('TasksSidebar conforms to Data Terminal 3-zone standard', () => {
    const wrapper = mount(TasksSidebar)

    // Zone A: Action
    expect(wrapper.find('.sidebar-actions').exists()).toBe(true)
    expect(wrapper.find('[data-test="btn-new-task"]').exists()).toBe(true)

    // Zone B: Search with inset icon and dropdown filters
    expect(wrapper.find('.sidebar-search-bar').exists()).toBe(true)
    expect(wrapper.find('.sidebar-search-icon').exists()).toBe(true)
    expect(wrapper.find('.sidebar-filters').exists()).toBe(true)

    // Zone C: Task list
    expect(wrapper.find('.task-list').exists()).toBe(true)
  })

  it('GroupsSidebar conforms to Data Terminal 3-zone standard', () => {
    const wrapper = mount(GroupsSidebar)

    // Zone A: Action
    expect(wrapper.find('.sidebar-actions').exists()).toBe(true)
    expect(wrapper.find('[data-test="btn-new-group"]').exists()).toBe(true)

    // Zone B: Search with inset icon
    expect(wrapper.find('.sidebar-search-bar').exists()).toBe(true)
    expect(wrapper.find('.sidebar-search-icon').exists()).toBe(true)
  })

  it('NotesSidebar conforms to the 3-zone standard with an el-menu list', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/notes', component: { template: '<div />' } },
        { path: '/notes/:noteId', component: { template: '<div />' } },
      ],
    })
    await router.push('/notes')
    await router.isReady()

    const wrapper = mount(NotesSidebar, { global: { plugins: [router] } })
    await flushPromises()

    // Zone A: the one action this column offers, an el-button now.
    expect(wrapper.find('.sidebar-actions').exists()).toBe(true)
    expect(wrapper.find('[data-test="new-note-btn"]').exists()).toBe(true)
    // Zone B: live filters, both library controls.
    expect(wrapper.find('[data-test="notes-search"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="notes-project-filter"]').exists()).toBe(true)
    // Zone C: no notes loaded in this test, so the menu is absent and the
    // three-state empty block stands in for it.
    expect(wrapper.find('[data-test="notes-menu"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="list-state-empty"]').exists()).toBe(true)
  })

  it('SettingsNav renders the settings navigation as a router-driven el-menu', async () => {
    // Same contract as the org and engines sidebars: a real router (memory
    // history), because the menu navigates by index in router mode and marks the
    // current path — both need it.
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/settings/appearance', component: { template: '<div />' } },
        { path: '/settings/models', component: { template: '<div />' } },
        { path: '/settings/proxies', component: { template: '<div />' } },
        { path: '/settings/connections', component: { template: '<div />' } },
        { path: '/settings/terminal', component: { template: '<div />' } },
        { path: '/settings/debug', component: { template: '<div />' } },
      ],
    })
    await router.push('/settings/terminal')
    await router.isReady()

    const wrapper = mount(SettingsNav, { global: { plugins: [router] } })

    // Double header must stay eliminated: the shell owns the title row.
    expect(wrapper.find('.settings-nav-header').exists()).toBe(false)
    expect(wrapper.find('[data-test="settings-nav"]').exists()).toBe(true)

    const items = wrapper.findAll('.el-menu-item')
    expect(items).toHaveLength(6)
    // Menu order is part of the product contract: appearance, models,
    // proxies, connections, terminal, debug (Role and Prompt moved to Organization).
    expect(wrapper.findAll('.link-label').map((label) => label.text())).toEqual([
      '外观',
      '模型',
      '服务代理',
      '服务连接',
      '终端服务器',
      '调试面板',
    ])
    // Every entry still carries its second line: the menu replaced the list
    // markup, not the information it showed.
    expect(wrapper.findAll('.link-desc')).toHaveLength(6)

    const active = wrapper.findAll('.el-menu-item.is-active')
    expect(active).toHaveLength(1)
    expect(active[0].text()).toContain('终端服务器')
  })

  it('EnginesSidebar renders the engines navigation as a router-driven el-menu', async () => {
    // Same contract as the org sidebar: a real router (memory history),
    // because the menu navigates by index in router mode and marks the
    // current path — both need it.
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/engines/assistant', component: { template: '<div />' } },
        { path: '/engines/coder', component: { template: '<div />' } },
        { path: '/engines/orchestrator', component: { template: '<div />' } },
        { path: '/engines/agents', component: { template: '<div />' } },
      ],
    })
    await router.push('/engines/agents')
    await router.isReady()

    const wrapper = mount(EnginesSidebar, { global: { plugins: [router] } })

    expect(wrapper.find('[data-test="engines-nav"]').exists()).toBe(true)
    const items = wrapper.findAll('.el-menu-item')
    expect(items).toHaveLength(4)
    // Menu order is part of the product contract.
    expect(wrapper.findAll('.link-label').map((label) => label.text())).toEqual([
      '助手引擎',
      '会话引擎',
      '协同引擎',
      'Agent配置',
    ])
    // Every entry still carries its second line: the menu replaced the list
    // markup, not the information it showed.
    expect(wrapper.findAll('.link-desc')).toHaveLength(4)

    const active = wrapper.findAll('.el-menu-item.is-active')
    expect(active).toHaveLength(1)
    expect(active[0].text()).toContain('Agent配置')
  })

  it('OrgSidebar renders the org navigation as a router-driven el-menu', async () => {
    // A real router (memory history) rather than a stub: the menu navigates by
    // index in router mode and marks the current path, and both need it.
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/org/prompts', component: { template: '<div />' } },
        { path: '/org/assistants', component: { template: '<div />' } },
        { path: '/org/employees', component: { template: '<div />' } },
        { path: '/org/teams', component: { template: '<div />' } },
      ],
    })
    await router.push('/org/assistants')
    await router.isReady()

    const wrapper = mount(OrgSidebar, { global: { plugins: [router] } })

    expect(wrapper.find('[data-test="org-sidebar"]').exists()).toBe(true)
    const items = wrapper.findAll('.el-menu-item')
    expect(items).toHaveLength(4)
    expect(wrapper.findAll('.link-label').map((label) => label.text())).toEqual([
      'AI 提示词',
      'AI 助手',
      'AI 员工',
      'AI 团队',
    ])
    // Every entry still carries its second line: the menu replaced the list
    // markup, not the information it showed.
    expect(wrapper.findAll('.link-desc')).toHaveLength(4)

    const active = wrapper.findAll('.el-menu-item.is-active')
    expect(active).toHaveLength(1)
    expect(active[0].text()).toContain('AI 助手')
  })
})
