// Unit tests for the project tree: only the current project expands, tasks
// carry their backend tag and status, search never clears the active task,
// and an empty project still offers a new task.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import BackendTag from '../components/BackendTag.vue'
import ProjectTree from '../components/ProjectTree.vue'
import { useProjectsStore } from '../stores/projects'
import type { AgentSession, Capabilities } from '../api/sessions'
import type { Project } from '../api/projects'

function noCapabilities(): Capabilities {
  return {
    streaming: false,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: false,
    interrupt: false,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
    sessionHistory: false,
  }
}

function project(id: number, name: string): Project {
  return { id, name, path: `/workspace/${name}` } as Project
}

function session(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 'sess-1',
    projectId: 1,
    backend: 'pi',
    instanceId: 'inst-1',
    nativeSessionId: 'native-1',
    title: 'Task A',
    status: 'active',
    turn: 'idle',
    capabilities: noCapabilities(),
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
    ...overrides,
  }
}

// Serves /api/projects/{id}/sessions from a map and records every request so a
// test can prove which projects' tasks were fetched.
function stubSessions(byProject: Record<number, AgentSession[]>): string[] {
  const requested: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input), 'http://localhost')
      requested.push(url.pathname)
      const match = url.pathname.match(/^\/api\/projects\/(\d+)\/sessions$/)
      if (!match) throw new Error(`unexpected GET ${url.pathname}`)
      const list = byProject[Number(match[1])] ?? []
      return { status: 200, json: async () => ({ sessions: list }) } as Response
    }),
  )
  return requested
}

const mounted: ReturnType<typeof mount>[] = []

function mountTree(props: { activeProjectId: number | null; activeSessionId?: string | null; search?: string }) {
  const wrapper = mount(ProjectTree, {
    props: {
      activeProjectId: props.activeProjectId,
      activeSessionId: props.activeSessionId ?? null,
      search: props.search ?? '',
    },
  })
  mounted.push(wrapper)
  return wrapper
}

describe('ProjectTree', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const projects = useProjectsStore()
    projects.projects = [project(1, 'alpha'), project(2, 'beta')]
  })

  afterEach(() => {
    for (const wrapper of mounted) wrapper.unmount()
    mounted.length = 0
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('expands only the current project and never fetches the others', async () => {
    const requested = stubSessions({
      1: [session({ id: 's1', title: 'Task A' })],
      2: [session({ id: 's2', projectId: 2, title: 'Task B' })],
    })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    expect(wrapper.text()).toContain('alpha')
    expect(wrapper.text()).toContain('beta')
    expect(wrapper.text()).toContain('Task A')
    expect(wrapper.text()).not.toContain('Task B')
    expect(requested).toEqual(['/api/projects/1/sessions'])
  })

  it('shows a running turn as 运行中 and hides idle status', async () => {
    stubSessions({
      1: [
        session({ id: 's1', title: 'Task A', backend: 'opencode-v2', turn: 'running' }),
        session({ id: 's2', title: 'Task B', status: 'active', turn: 'idle' }),
      ],
    })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    const running = wrapper.findAll('.task').find((item) => item.text().includes('Task A'))
    expect(running).toBeDefined()
    const tag = running!.findComponent(BackendTag)
    expect(tag.exists()).toBe(true)
    expect(tag.props('backend')).toBe('opencode-v2')
    expect(tag.text()).toBe('opencode')
    expect(running!.find('.status').text()).toBe('运行中')

    const idle = wrapper.findAll('.task').find((item) => item.text().includes('Task B'))
    expect(idle).toBeDefined()
    expect(idle!.find('.status').exists()).toBe(false)
  })

  it('shows no status label for a disconnected session', async () => {
    // Only a running turn carries a marker: a disconnected session needs no
    // badge once opening a task repairs it.
    stubSessions({ 1: [session({ id: 's1', title: 'Task A', status: 'disconnected' })] })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    expect(wrapper.find('.status').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('断开')
  })

  it('polls while the expanded project has a running task and stops when none is', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    const byProject: Record<number, AgentSession[]> = { 1: [session({ id: 's1', turn: 'running' })] }
    const requested = stubSessions(byProject)
    mountTree({ activeProjectId: 1 })
    await flushPromises()
    expect(requested).toEqual(['/api/projects/1/sessions'])

    vi.advanceTimersByTime(2000)
    await flushPromises()
    expect(requested.length).toBe(2)

    // The turn finished: the next refresh carries no running task and the poll
    // stops, so further time passes issue no more requests.
    byProject[1] = [session({ id: 's1', turn: 'idle' })]
    vi.advanceTimersByTime(2000)
    await flushPromises()
    expect(requested.length).toBe(3)

    vi.advanceTimersByTime(4000)
    await flushPromises()
    expect(requested.length).toBe(3)
  })

  it('keeps the active task selected and emits nothing when search has no match', async () => {
    stubSessions({
      1: [session({ id: 's1', title: 'Task A' })],
    })
    const wrapper = mountTree({ activeProjectId: 1, activeSessionId: 's1', search: 'zzz' })
    await flushPromises()

    expect(wrapper.text()).toContain('没有匹配的任务')
    expect(wrapper.text()).toContain('alpha')
    expect(wrapper.emitted('select-session')).toBeUndefined()
    expect(wrapper.emitted('select-project')).toBeUndefined()
  })

  it('filters other projects by name and always keeps the active project', async () => {
    stubSessions({ 1: [] })
    const wrapper = mountTree({ activeProjectId: 1, search: 'zzz' })
    await flushPromises()
    expect(wrapper.text()).toContain('alpha')
    expect(wrapper.text()).not.toContain('beta')

    await wrapper.setProps({ search: 'bet' })
    expect(wrapper.text()).toContain('beta')
  })

  it('reports when no project matches the search', async () => {
    stubSessions({})
    const wrapper = mountTree({ activeProjectId: null, search: 'zzz' })
    await flushPromises()
    expect(wrapper.text()).toContain('没有匹配的项目')
  })

  it('shows a new-task entry for a project with no tasks', async () => {
    stubSessions({ 1: [] })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    expect(wrapper.text()).toContain('该项目还没有任务')
    const newTask = wrapper.findAll('.new-task').find((item) => item.text().includes('新建任务'))
    expect(newTask).toBeDefined()
    await newTask!.trigger('click')
    expect(wrapper.emitted('new-task')).toEqual([[1]])
  })

  it('switches expansion and loads tasks when the active project changes', async () => {
    const requested = stubSessions({
      1: [session({ id: 's1', title: 'Task A' })],
      2: [session({ id: 's2', projectId: 2, title: 'Task B' })],
    })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()
    await wrapper.setProps({ activeProjectId: 2 })
    await flushPromises()

    expect(wrapper.text()).toContain('Task B')
    expect(wrapper.text()).not.toContain('Task A')
    expect(requested).toContain('/api/projects/2/sessions')
  })

  it('toggles a project on row click without opening its settings', async () => {
    stubSessions({ 1: [session({ id: 's1', title: 'Task A' })] })
    const wrapper = mountTree({ activeProjectId: null })
    await flushPromises()

    const toggle = wrapper.find('.project-toggle')
    expect(toggle.exists()).toBe(true)

    await toggle.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Task A')
    expect(wrapper.emitted('select-project')).toBeUndefined()

    await toggle.trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('Task A')
  })

  it('opens project settings from the settings control', async () => {
    stubSessions({ 1: [] })
    const wrapper = mountTree({ activeProjectId: null })
    await flushPromises()

    await wrapper.find('.project-settings').trigger('click')
    expect(wrapper.emitted('select-project')).toEqual([[1]])
  })

  it('shows the untitled placeholder instead of the internal id, in list and in search', async () => {
    stubSessions({ 1: [session({ id: 's-x', title: '' })] })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    expect(wrapper.text()).toContain('未命名任务')
    expect(wrapper.text()).not.toContain('s-x')

    // The search filter matches the placeholder, not the hidden id.
    await wrapper.setProps({ search: '未命名' })
    expect(wrapper.text()).toContain('未命名任务')
    await wrapper.setProps({ search: 's-x' })
    expect(wrapper.text()).toContain('没有匹配的任务')
  })
})
