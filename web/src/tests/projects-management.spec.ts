import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useProjectsStore } from '../stores/projects'
import { useOpenTabsStore, tabKey } from '../stores/openTabs'
import SessionTabs from '../features/workspace/SessionTabs.vue'
import ProjectsManagementView from '../features/projects/ProjectsManagementView.vue'
import TaskChatDialog from '../features/tasks/TaskChatDialog.vue'

describe('projects management, display ordering, and task dialog minimal mode', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
  })

  it('manages project display order and visibility in projects store', () => {
    const store = useProjectsStore()
    store.projects = [
      { id: 1, name: 'Project A', path: '/path/a', isGit: false },
      { id: 2, name: 'Project B', path: '/path/b', isGit: true },
      { id: 3, name: 'Project C', path: '/path/c', isGit: false },
    ]

    expect(store.orderedProjects.map((p) => p.id)).toEqual([1, 2, 3])
    expect(store.orderedVisibleProjects.map((p) => p.id)).toEqual([1, 2, 3])

    // Move Project B up
    store.moveProjectUp(2)
    expect(store.orderedProjects.map((p) => p.id)).toEqual([2, 1, 3])

    // Move Project B down
    store.moveProjectDown(2)
    expect(store.orderedProjects.map((p) => p.id)).toEqual([1, 2, 3])

    // Hide Project A
    store.toggleProjectVisibility(1)
    expect(store.isHidden(1)).toBe(true)
    expect(store.orderedVisibleProjects.map((p) => p.id)).toEqual([2, 3])

    // Unhide Project A
    store.setProjectVisibility(1, true)
    expect(store.isHidden(1)).toBe(false)
    expect(store.orderedVisibleProjects.map((p) => p.id)).toEqual([1, 2, 3])
  })

  it('opens and renders projects-management tab in openTabs and SessionTabs', async () => {
    const openTabs = useOpenTabsStore()
    openTabs.openProjectsManagement()

    expect(openTabs.tabs).toHaveLength(1)
    expect(openTabs.tabs[0].kind).toBe('projects-management')
    expect(tabKey(openTabs.tabs[0])).toBe('projects-management')

    const wrapper = mount(SessionTabs, {
      props: {
        tabs: openTabs.tabs,
        activeKey: 'projects-management',
      },
    })
    await flushPromises()

    const tab = wrapper.find('[data-test="open-tab"]')
    expect(tab.exists()).toBe(true)
    expect(tab.text()).toContain('项目管理')
  })

  it('renders ProjectsManagementView with order controls and visibility toggles', async () => {
    const projectsStore = useProjectsStore()
    projectsStore.projects = [
      { id: 10, name: 'Alpha', path: '/data/alpha', isGit: false },
      { id: 20, name: 'Beta', path: '/data/beta', isGit: true },
    ]

    const wrapper = mount(ProjectsManagementView)
    await flushPromises()

    const rows = wrapper.findAll('[data-test="project-manage-row"]')
    expect(rows).toHaveLength(2)

    // Verify "项目配置" action button is removed, only "解除" exists
    const actions = rows[0].find('.col-actions')
    expect(actions.text()).not.toContain('项目配置')
    expect(actions.text()).toContain('解除')

    // Toggle visibility for Alpha
    const toggle = rows[0].find('[data-test="project-visibility-toggle"]')
    await toggle.trigger('change')
    await flushPromises()
    expect(projectsStore.isHidden(10)).toBe(true)

    // Move Beta up
    const betaMoveUpBtn = rows[1].find('[data-test="btn-move-up"]')
    await betaMoveUpBtn.trigger('click')
    await flushPromises()
    expect(projectsStore.orderedProjects.map((p) => p.id)).toEqual([20, 10])
  })

  it('renders TaskChatDialog with minimal mode (hide-side-rail) and moderate size', async () => {
    const wrapper = mount(TaskChatDialog, {
      props: {
        title: '测试任务对话',
        projectId: 1,
        sessionId: 'ses-task-1',
      },
      global: {
        stubs: {
          Teleport: true,
          SessionChatPanel: {
            props: ['projectId', 'sessionId', 'hideSideRail'],
            template: '<div class="stubbed-session-panel" :data-hide-rail="String(hideSideRail)"></div>',
          },
        },
      },
    })
    await flushPromises()

    const panel = wrapper.find('.stubbed-session-panel')
    expect(panel.exists()).toBe(true)
    expect(panel.attributes('data-hide-rail')).toBe('true')
  })
})
