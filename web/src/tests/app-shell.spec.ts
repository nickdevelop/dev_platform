// Routing and shell tests for the three-pane layout. The real router and App are
// mounted; the right-pane panels are stubbed in routing tests so each test checks
// only which panel the URL selects and which ids it passes. Covers the direct
// session URL (refresh), an unknown project id, the login boundary, and creating a
// session navigating to it.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import App from '../App.vue'
import ProjectTree from '../components/ProjectTree.vue'
import ProjectSettingsPanel from '../components/ProjectSettingsPanel.vue'
import SessionChatPanel from '../components/SessionChatPanel.vue'
import NewSessionPanel from '../components/NewSessionPanel.vue'
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
  stubs: Record<string, boolean> = {},
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
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('session')
    expect(router.currentRoute.value.params.sessionId).toBe('sess-new')
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

  it('navigates to the settings center from the category rail', async () => {
    const { wrapper, router } = await setup('/', { ProjectSettingsPanel: true })

    const settings = wrapper.findAll('a').find((link) => link.text() === '设置')
    expect(settings).toBeTruthy()
    await settings!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('settings-appearance')
  })

  it('hides the project sidebar in the settings center', async () => {
    const { wrapper } = await setup('/settings/appearance')

    expect(wrapper.find('aside.sidebar').exists()).toBe(false)
    expect(wrapper.text()).toContain('外观')
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
})
