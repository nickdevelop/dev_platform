// Routing and shell tests for the three-pane layout. The real router and App are
// mounted; the right-pane panels are stubbed in routing tests so each test checks
// only which panel the URL selects and which ids it passes. Covers the direct
// task URL (refresh), an unknown project id, the login boundary, and creating a
// task navigating to it.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import App from '../App.vue'
import ProjectTree from '../components/ProjectTree.vue'
import ProjectSettingsPanel from '../components/ProjectSettingsPanel.vue'
import TaskChatPanel from '../components/TaskChatPanel.vue'
import NewTaskPanel from '../components/NewTaskPanel.vue'
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
  }
}

async function setup(
  path: string,
  stubs: Record<string, boolean> = {},
  prepare: () => void = () => {},
) {
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
    expect(wrapper.text()).toContain('从左侧选择一个项目或任务')
    expect(wrapper.findComponent(ProjectSettingsPanel).exists()).toBe(false)
    const chat = wrapper.findAll('button').find((button) => button.text() === 'Chat')
    expect(chat?.attributes('disabled')).toBeDefined()
  })

  it('selects project settings for /projects/:projectId', async () => {
    const { wrapper } = await setup('/projects/1', { ProjectSettingsPanel: true })

    expect(wrapper.findComponent(ProjectSettingsPanel).props('projectId')).toBe(1)
    expect(wrapper.findComponent(ProjectTree).props('activeProjectId')).toBe(1)
    expect(wrapper.findComponent(ProjectTree).props('activeSessionId')).toBeNull()
  })

  it('restores the same task from a directly opened task URL', async () => {
    const { wrapper } = await setup('/projects/1/tasks/sess-1', { TaskChatPanel: true })

    expect(wrapper.findComponent(TaskChatPanel).props()).toMatchObject({
      projectId: 1,
      sessionId: 'sess-1',
    })
    expect(wrapper.findComponent(ProjectTree).props('activeProjectId')).toBe(1)
    expect(wrapper.findComponent(ProjectTree).props('activeSessionId')).toBe('sess-1')
  })

  it('reports a missing project instead of rendering another project', async () => {
    const { wrapper } = await setup('/projects/999', { ProjectSettingsPanel: true })

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

  it('opens the created task after a new task is created', async () => {
    const { wrapper, router } = await setup('/projects/1/tasks/new', {}, () => {
      const agents = useAgentsStore()
      agents.backends = [{ backend: 'pi', capabilities: capabilities() }]
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
    expect(wrapper.findComponent(NewTaskPanel).exists()).toBe(true)
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('task')
    expect(router.currentRoute.value.params.sessionId).toBe('sess-new')
  })
})
