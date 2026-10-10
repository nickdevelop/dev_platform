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
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
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

function mountTree(props: { activeProjectId: number | null; activeSessionId?: string | null; search?: string }) {
  return mount(ProjectTree, {
    props: {
      activeProjectId: props.activeProjectId,
      activeSessionId: props.activeSessionId ?? null,
      search: props.search ?? '',
    },
  })
}

describe('ProjectTree', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const projects = useProjectsStore()
    projects.projects = [project(1, 'alpha'), project(2, 'beta')]
  })

  afterEach(() => {
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

  it('shows each task with its backend tag and status', async () => {
    stubSessions({
      1: [session({ id: 's1', title: 'Task A', backend: 'opencode-v2', status: 'active' })],
    })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    const task = wrapper.findAll('.task').find((item) => item.text().includes('Task A'))
    expect(task).toBeDefined()
    const tag = task!.findComponent(BackendTag)
    expect(tag.exists()).toBe(true)
    expect(tag.props('backend')).toBe('opencode-v2')
    expect(tag.text()).toBe('opencode')
    expect(task!.text()).toContain('运行中')
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
})
