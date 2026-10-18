// Routing and shell tests for the three-pane layout. The real router and App are
// mounted; the right-pane panels are stubbed in routing tests so each test checks
// only which panel the URL selects and which ids it passes. Covers the direct
// session URL (refresh), an unknown project id, the login boundary, and creating a
// session navigating to it.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { Component } from 'vue'
import App from '../App.vue'
import ProjectTree from '../features/projects/ProjectTree.vue'
import ProjectSettingsPanel from '../features/projects/ProjectSettingsPanel.vue'
import SessionChatPanel from '../features/chat/SessionChatPanel.vue'
import NewSessionPanel from '../features/chat/composer/NewSessionPanel.vue'
import { createAppRouter } from '../router'
import { useAgentsStore } from '../stores/agents'
import { useAuthStore } from '../stores/auth'
import { useProjectsStore } from '../stores/projects'
import { useSessionsStore } from '../stores/sessions'
import type { Capabilities } from '../api/sessions'

function capabilities(): Capabilities {
  return {
    streaming: true,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: true,
    interrupt: false,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
    sessionHistory: false,
    rawEvents: false,
  }
}

async function setup(
  path: string,
  stubs: Record<string, boolean | Component> = {},
  prepare: () => void = () => {},
) {
  // Tabs persist in this browser's storage; a test must see only the tabs its
  // own route opened.
  localStorage.clear()
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.$patch({ authenticated: true, sessionChecked: true })
  const projects = useProjectsStore()
  projects.projects = [{ id: 1, name: 'alpha', path: '/workspace/alpha' } as never]
  vi.spyOn(projects, 'load').mockResolvedValue(undefined)
  prepare()

  const router = createAppRouter()
  await router.push(path)
  await router.isReady()

  const wrapper = mount(App, {
    global: { plugins: [pinia, router], stubs: { ProjectTree: true, ...stubs } },
  })
  await flushPromises()
  return { wrapper, router, pinia }
}

describe('three-pane app shell', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
    vi.stubGlobal('fetch', vi.fn())
  })

  it('shows the tree, the empty workspace and a disabled Chat category at /', async () => {
    const { wrapper } = await setup('/', { ProjectSettingsPanel: true })

    expect(wrapper.findComponent(ProjectTree).exists()).toBe(true)
    expect(wrapper.text()).toContain('从左侧选择一个项目或会话')
    expect(wrapper.findComponent(ProjectSettingsPanel).exists()).toBe(false)
    const chat = wrapper.findAll('button').find((button) => button.text() === 'Chat')
    expect(chat?.attributes('disabled')).toBeDefined()
  })

  it('opens project settings as a tab at /projects/:projectId/settings', async () => {
    const { wrapper, router } = await setup('/projects/1/settings', { ProjectSettingsPanel: true })

    expect(wrapper.findComponent(ProjectSettingsPanel).props('projectId')).toBe(1)
    expect(wrapper.findComponent(ProjectTree).props('activeProjectId')).toBe(1)
    expect(wrapper.findComponent(ProjectTree).props('activeSessionId')).toBeNull()

    // The settings page is a tab like any other, and it is the active one.
    const tabs = wrapper.findAll('[data-test="open-tab"]')
    expect(tabs).toHaveLength(1)
    expect(tabs[0].attributes('data-tab-key')).toBe('settings:1')
    expect(tabs[0].attributes('aria-selected')).toBe('true')
    expect(tabs[0].text()).toContain('alpha · 设置')
    expect(router.currentRoute.value.name).toBe('project-settings')
  })

  it('opens one settings tab per project however often it is visited', async () => {
    const { wrapper, router } = await setup('/projects/1/sessions/sess-1', {
      SessionChatPanel: true,
      ProjectSettingsPanel: true,
    })
    expect(wrapper.findAll('[data-test="open-tab"]')).toHaveLength(1)

    await router.push({ name: 'project-settings', params: { projectId: '1' } })
    await flushPromises()
    await router.push({ name: 'home' })
    await flushPromises()
    await router.push({ name: 'project-settings', params: { projectId: '1' } })
    await flushPromises()

    const keys = wrapper.findAll('[data-test="open-tab"]').map((tab) => tab.attributes('data-tab-key'))
    expect(keys).toEqual(['sess-1', 'settings:1'])
  })

  it('redirects the old project URL to the workbench', async () => {
    const { router } = await setup('/projects/1', { ProjectSettingsPanel: true })

    // A project has no page of its own any more: its settings are a tab, so the
    // stale URL must not render a panel no tab points at.
    expect(router.currentRoute.value.name).toBe('home')
  })

  it('restores the same session from a directly opened session URL', async () => {
    const { wrapper } = await setup('/projects/1/sessions/sess-1', { SessionChatPanel: true })

    expect(wrapper.findComponent(SessionChatPanel).props()).toMatchObject({
      projectId: 1,
      sessionId: 'sess-1',
    })
    expect(wrapper.findComponent(ProjectTree).props('activeProjectId')).toBe(1)
    expect(wrapper.findComponent(ProjectTree).props('activeSessionId')).toBe('sess-1')
  })

  it('reports a missing project instead of rendering another project', async () => {
    const { wrapper } = await setup('/projects/999/settings', { ProjectSettingsPanel: true })

    expect(wrapper.text()).toContain('项目不存在或已移除')
    expect(wrapper.findComponent(ProjectSettingsPanel).exists()).toBe(false)
  })

  it('keeps the login page outside the shell', async () => {
    const { wrapper } = await setup('/login')

    expect(wrapper.findComponent(ProjectTree).exists()).toBe(false)
  })

  it('sends an unauthenticated user from the shell to login', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuthStore()
    auth.$patch({ authenticated: false, sessionChecked: true })
    const router = createAppRouter()
    await router.push('/')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('login')
  })

  it('redirects unknown paths to the home view', async () => {
    const { router } = await setup('/no-such-page')

    expect(router.currentRoute.value.name).toBe('home')
  })

  it('opens the created session after a new session is created', async () => {
    const { wrapper, router } = await setup('/projects/1/sessions/new', {}, () => {
      const agents = useAgentsStore()
      agents.backends = [{ backend: 'pi', capabilities: capabilities(), label: 'Pi Agent', order: 2, serviceSupported: false }]
      agents.instances = [
        {
          id: 'inst-1',
          projectId: 1,
          backend: 'pi',
          directory: '/workspace/alpha',
          status: 'running',
          createdAt: '2026-10-09T00:00:00Z',
          updatedAt: '2026-10-09T00:00:00Z',
        },
      ]
      vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined)
      vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined)
      vi.spyOn(useSessionsStore(), 'createSession').mockResolvedValue({ id: 'sess-new' } as never)
    })
    expect(wrapper.findComponent(NewSessionPanel).exists()).toBe(true)

    // The form itself is a tab, labelled "new" until the session exists.
    const before = wrapper.findAll('[data-test="open-tab"]')
    expect(before).toHaveLength(1)
    expect(before[0].attributes('data-tab-key')).toBe('new-session')
    expect(before[0].text()).toContain('new')

    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('session')
    expect(router.currentRoute.value.params.sessionId).toBe('sess-new')
    // The "new" tab is replaced in place by the created session's tab; the
    // server-derived title fills the label once the session list reports it.
    const openTabs = wrapper.findAll('[data-test="open-tab"]')
    expect(openTabs).toHaveLength(1)
    expect(openTabs[0].attributes('data-tab-key')).toBe('sess-new')
    expect(openTabs[0].text()).not.toBe('new')
  })

  it('gives the session conversation a non-scrolling outer pane', async () => {
    const { wrapper } = await setup('/projects/1/sessions/sess-1', { SessionChatPanel: true })

    // The conversation owns the only scroller (the message log); the shell's
    // content pane must not scroll as well, or the page shows two scrollbars.
    expect(wrapper.find('.pane').classes()).toContain('pane-full')
  })

  it('lets the project settings pane scroll in the content region', async () => {
    const { wrapper } = await setup('/projects/1', { ProjectSettingsPanel: true })

    expect(wrapper.find('.pane').classes()).not.toContain('pane-full')
  })

  it('pins a tab and keeps it when the other tabs are closed', async () => {
    const { wrapper, router } = await setup('/projects/1/sessions/sess-1', {
      SessionChatPanel: true,
      ProjectSettingsPanel: true,
    })
    await router.push({ name: 'project-settings', params: { projectId: '1' } })
    await flushPromises()
    expect(wrapper.findAll('[data-test="open-tab"]')).toHaveLength(2)

    // Pin the session tab from its context menu; it moves to the front.
    const first = wrapper.findAll('[data-test="open-tab"]')[0]
    await first.trigger('contextmenu', { clientX: 5, clientY: 5 })
    await wrapper.find('[data-test="menu-toggle-pin"]').trigger('click')
    await flushPromises()

    const tabs = wrapper.findAll('[data-test="open-tab"]')
    expect(tabs[0].attributes('data-tab-key')).toBe('sess-1')
    expect(tabs[0].attributes('data-pinned')).toBe('true')

    // Close all: the pinned tab survives, so the strip is not empty and the
    // view falls back to it.
    await tabs[0].trigger('contextmenu', { clientX: 5, clientY: 5 })
    await wrapper.find('[data-test="menu-close-all"]').trigger('click')
    await flushPromises()

    const remaining = wrapper.findAll('[data-test="open-tab"]')
    expect(remaining).toHaveLength(1)
    expect(remaining[0].attributes('data-tab-key')).toBe('sess-1')
    expect(router.currentRoute.value.name).toBe('session')
  })

  it('shows an inline icon on every first-level rail entry', async () => {
    const { wrapper } = await setup('/', { ProjectSettingsPanel: true })

    // 项目 / 任务 / 提示词 / Chat / 设置 / 退出 / 收起 each carry one inline SVG icon.
    const rail = wrapper.find('nav.category-rail')
    expect(rail.findAll('svg.rail-icon')).toHaveLength(7)
    // The icons are decorative: the labels stay as the entries' readable text.
    expect(rail.findAll('a, button').map((entry) => entry.text())).toEqual([
      '项目',
      '任务',
      '提示词',
      'Chat',
      '设置',
      '退出',
      '收起',
    ])
  })

  it('collapses the project column from the rail and persists the choice', async () => {
    const { wrapper } = await setup('/', { ProjectSettingsPanel: true })
    expect(wrapper.find('aside.sidebar').exists()).toBe(true)

    await wrapper.find('[data-test="sidebar-toggle"]').trigger('click')
    expect(wrapper.find('aside.sidebar').exists()).toBe(false)
    expect(wrapper.find('.app-shell').classes()).toContain('sidebar-collapsed')
    expect(localStorage.getItem('dev-platform-sidebar-v1')).toBe('collapsed')

    // The same toggle brings the column back and reverses the stored choice.
    await wrapper.find('[data-test="sidebar-toggle"]').trigger('click')
    expect(wrapper.find('aside.sidebar').exists()).toBe(true)
    expect(localStorage.getItem('dev-platform-sidebar-v1')).toBe('expanded')
  })

  it('brings the project column back when the project category is clicked', async () => {
    const { wrapper } = await setup('/', { ProjectSettingsPanel: true })
    await wrapper.find('[data-test="sidebar-toggle"]').trigger('click')
    expect(wrapper.find('aside.sidebar').exists()).toBe(false)

    // Clicking the already-active project entry changes no route, so the click
    // itself has to restore the column it depends on.
    await wrapper.findAll('a').find((link) => link.text() === '项目')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('aside.sidebar').exists()).toBe(true)
    expect(localStorage.getItem('dev-platform-sidebar-v1')).toBe('expanded')
  })

  it('starts with the project column collapsed when the stored choice says so', async () => {
    const { wrapper } = await setup('/', { ProjectSettingsPanel: true }, () => {
      localStorage.setItem('dev-platform-sidebar-v1', 'collapsed')
    })

    expect(wrapper.find('aside.sidebar').exists()).toBe(false)
  })

  it('navigates to the settings center from the category rail', async () => {
    const { wrapper, router } = await setup('/', { ProjectSettingsPanel: true })

    const settings = wrapper.findAll('a').find((link) => link.text() === '设置')
    expect(settings).toBeTruthy()
    await settings!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('settings-appearance')
  })

  it('renders the settings menu in the middle column of the settings center and isolates workspace tabs', async () => {
    const { wrapper } = await setup('/settings/appearance')

    // The middle column carries the settings categories instead of the project
    // tree; the settings center is isolated from workspace session tabs.
    expect(wrapper.find('[data-test="settings-nav"]').exists()).toBe(true)
    expect(wrapper.findComponent(ProjectTree).exists()).toBe(false)
    expect(wrapper.text()).toContain('外观')
    // Workspace tabs are not rendered in the settings center
    expect(wrapper.findAll('[data-test="open-tab"]')).toHaveLength(0)
  })

  it('returns to the project area when the project category is clicked in settings', async () => {
    const { wrapper, router } = await setup('/settings/appearance')

    const project = wrapper.findAll('a').find((link) => link.text() === '项目')
    expect(project).toBeTruthy()
    await project!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('home')
    expect(wrapper.find('aside.sidebar').exists()).toBe(true)
  })

  it('isolates the tasks center so workspace session tabs do not display over tasks', async () => {
    const { wrapper } = await setup('/tasks/5', {
      TasksView: true,
      TasksSidebar: { template: '<div class="tasks-nav-stub"></div>' },
    })

    // The middle column carries the task list in the tasks center.
    expect(wrapper.find('.tasks-nav-stub').exists()).toBe(true)
    // The workspace tab strip is isolated and not rendered in tasks mode.
    expect(wrapper.findAll('[data-test="open-tab"]')).toHaveLength(0)
  })

  it('keeps project tabs open without mixing them into tasks or settings', async () => {
    const { wrapper, router } = await setup('/projects/1/sessions/ses-1', {
      TasksView: true,
      TasksSidebar: { template: '<div class="tasks-nav-stub"></div>' },
    })
    await flushPromises()

    // Under project area: tab is present
    expect(wrapper.findAll('[data-test="open-tab"]')).toHaveLength(1)
    expect(wrapper.find('[data-test="open-tab"]').text()).toContain('ses-1')

    // Navigate to tasks: tabs are not shown in tasks center
    await router.push('/tasks')
    await flushPromises()
    expect(wrapper.findAll('[data-test="open-tab"]')).toHaveLength(0)

    // Navigate to settings: tabs are not shown in settings center
    await router.push('/settings/appearance')
    await flushPromises()
    expect(wrapper.findAll('[data-test="open-tab"]')).toHaveLength(0)

    // Navigate back to project via rail link: tab is restored
    const projectLink = wrapper.findAll('a').find((link) => link.text() === '项目')
    expect(projectLink).toBeTruthy()
    await projectLink!.trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-test="open-tab"]')).toHaveLength(1)
    expect(wrapper.find('[data-test="open-tab"]').text()).toContain('ses-1')
  })

  it('marks the project category as current only outside the settings center', async () => {
    const outside = (await setup('/projects/1', { ProjectSettingsPanel: true })).wrapper
    expect(
      outside.findAll('a').find((link) => link.text() === '项目')!.attributes('aria-current'),
    ).toBe('page')

    const inside = (await setup('/settings/appearance')).wrapper
    const project = inside.findAll('a').find((link) => link.text() === '项目')!
    expect(project.attributes('aria-current')).toBeUndefined()
    expect(
      inside.findAll('a').find((link) => link.text() === '设置')!.attributes('aria-current'),
    ).toBe('page')
  })

  it('isolates the prompts center so workspace session tabs do not display over prompts', async () => {
    const { wrapper } = await setup('/prompts', {
      PromptsView: true,
      PromptsSidebar: { template: '<div class="prompts-nav-stub"></div>' },
    })

    // The middle column carries the prompts list in the prompts center.
    expect(wrapper.find('.prompts-nav-stub').exists()).toBe(true)
    // The workspace tab strip is isolated and not rendered in prompts mode.
    expect(wrapper.findAll('[data-test="open-tab"]')).toHaveLength(0)
  })
})
