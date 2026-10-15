// Unit tests for the project tree: only the current project expands, sessions
// carry their backend tag and status, search never clears the active session,
// and an empty project still offers a new session.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { backendFixtures } from './backend-fixtures'
import { useAgentsStore } from '../stores/agents'
import BackendTag from '../shared/components/BackendTag.vue'
import ProjectTree from '../features/projects/ProjectTree.vue'
import { useProjectsStore } from '../stores/projects'
import { useSessionsStore } from '../stores/sessions'
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
    rawEvents: false,
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
    title: 'Session A',
    status: 'active',
    turn: 'idle',
    capabilities: noCapabilities(),
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
    ...overrides,
  }
}

// Serves /api/projects/{id}/sessions from a map and records every request so a
// test can prove which projects' sessions were fetched.
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
    useAgentsStore().backends = backendFixtures()
  })

  afterEach(() => {
    for (const wrapper of mounted) wrapper.unmount()
    mounted.length = 0
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('expands only the current project and never fetches the others', async () => {
    const requested = stubSessions({
      1: [session({ id: 's1', title: 'Session A' })],
      2: [session({ id: 's2', projectId: 2, title: 'Session B' })],
    })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    expect(wrapper.text()).toContain('alpha')
    expect(wrapper.text()).toContain('beta')
    expect(wrapper.text()).toContain('Session A')
    expect(wrapper.text()).not.toContain('Session B')
    expect(requested).toEqual(['/api/projects/1/sessions'])
  })

  it('marks a running turn with an animated icon and hides idle status', async () => {
    stubSessions({
      1: [
        session({ id: 's1', title: 'Session A', backend: 'opencode-v2', turn: 'running' }),
        session({ id: 's2', title: 'Session B', status: 'active', turn: 'idle' }),
      ],
    })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    const running = wrapper.findAll('.session').find((item) => item.text().includes('Session A'))
    expect(running).toBeDefined()
    const tag = running!.findComponent(BackendTag)
    expect(tag.exists()).toBe(true)
    expect(tag.props('backend')).toBe('opencode-v2')
    expect(tag.text()).toBe('OpenCode')
    expect(running!.find('[data-test="running-indicator"]').exists()).toBe(true)
    // The marker replaces the plain 运行中 text; the row announces it instead.
    expect(running!.text()).not.toContain('运行中')

    const idle = wrapper.findAll('.session').find((item) => item.text().includes('Session B'))
    expect(idle).toBeDefined()
    expect(idle!.find('[data-test="running-indicator"]').exists()).toBe(false)
  })

  it('shows no status marker for a disconnected session', async () => {
    // Only a running turn carries a marker: a disconnected session needs no
    // badge once opening a session repairs it.
    stubSessions({ 1: [session({ id: 's1', title: 'Session A', status: 'disconnected' })] })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    expect(wrapper.find('[data-test="running-indicator"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('断开')
  })

  it('polls while the expanded project has a running session and stops when none is', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    const byProject: Record<number, AgentSession[]> = { 1: [session({ id: 's1', turn: 'running' })] }
    const requested = stubSessions(byProject)
    mountTree({ activeProjectId: 1 })
    await flushPromises()
    expect(requested).toEqual(['/api/projects/1/sessions'])

    vi.advanceTimersByTime(2000)
    await flushPromises()
    expect(requested.length).toBe(2)

    // The turn finished: the next refresh carries no running session and the poll
    // stops, so further time passes issue no more requests.
    byProject[1] = [session({ id: 's1', turn: 'idle' })]
    vi.advanceTimersByTime(2000)
    await flushPromises()
    expect(requested.length).toBe(3)

    vi.advanceTimersByTime(4000)
    await flushPromises()
    expect(requested.length).toBe(3)
  })

  it('keeps the active session selected and emits nothing when search has no match', async () => {
    stubSessions({
      1: [session({ id: 's1', title: 'Session A' })],
    })
    const wrapper = mountTree({ activeProjectId: 1, activeSessionId: 's1', search: 'zzz' })
    await flushPromises()

    expect(wrapper.text()).toContain('没有匹配的会话')
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

  it('offers a new session from the project row icon, ahead of the settings icon', async () => {
    stubSessions({ 1: [] })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    expect(wrapper.text()).toContain('该项目还没有会话')
    const classes = wrapper
      .findAll('.project-row button')
      .map((button) => button.classes().join(' '))
    const newSessionIndex = classes.findIndex((value) => value.includes('project-new-session'))
    const settingsIndex = classes.findIndex((value) => value.includes('project-settings'))
    expect(newSessionIndex).toBeGreaterThanOrEqual(0)
    expect(newSessionIndex).toBeLessThan(settingsIndex)
    const newSession = wrapper.find('.project-new-session')
    expect(newSession.attributes('title')).toBe('新建会话')
    expect(newSession.attributes('aria-label')).toBe('新建会话：alpha')
    await newSession.trigger('click')
    expect(wrapper.emitted('new-session')).toEqual([[1]])
  })

  it('keeps both projects expanded when the active project changes', async () => {
    const requested = stubSessions({
      1: [session({ id: 's1', title: 'Session A' })],
      2: [session({ id: 's2', projectId: 2, title: 'Session B' })],
    })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()
    await wrapper.setProps({ activeProjectId: 2 })
    await flushPromises()

    // Multiple projects stay expanded at once: the previous project's sessions
    // remain visible instead of collapsing.
    expect(wrapper.text()).toContain('Session B')
    expect(wrapper.text()).toContain('Session A')
    expect(requested).toContain('/api/projects/2/sessions')
  })

  it('orders sessions by most recent activity first', async () => {
    stubSessions({
      1: [
        session({ id: 's-old', title: 'Oldest', updatedAt: '2026-10-01T00:00:00Z' }),
        session({ id: 's-mid', title: 'Middle', updatedAt: '2026-10-05T00:00:00Z' }),
        session({ id: 's-new', title: 'Newest', updatedAt: '2026-10-09T00:00:00Z' }),
      ],
    })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    const titles = wrapper.findAll('.session').map((item) => item.text())
    expect(titles[0]).toContain('Newest')
    expect(titles[1]).toContain('Middle')
    expect(titles[2]).toContain('Oldest')
  })

  it('shows only the newest 15 sessions and reveals the next page on demand', async () => {
    stubSessions({
      1: Array.from({ length: 40 }, (_, index) =>
        session({
          id: `s-${index}`,
          title: `Session ${index}`,
          updatedAt: `2026-10-01T00:00:${String(index).padStart(2, '0')}Z`,
        }),
      ),
    })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    expect(wrapper.findAll('.session').length).toBe(15)
    // Newest first: index 39 is the most recent interaction.
    expect(wrapper.findAll('.session')[0].text()).toContain('Session 39')
    expect(wrapper.text()).not.toContain('Session 24')

    const more = wrapper.find('.load-more')
    expect(more.exists()).toBe(true)
    await more.trigger('click')
    expect(wrapper.findAll('.session').length).toBe(30)
    expect(wrapper.text()).not.toContain('Session 9')

    await wrapper.find('.load-more').trigger('click')
    expect(wrapper.findAll('.session').length).toBe(40)
    expect(wrapper.find('.load-more').exists()).toBe(false)
  })

  it('paginates each expanded project independently', async () => {
    const many = (projectId: number) =>
      Array.from({ length: 20 }, (_, index) =>
        session({
          id: `p${projectId}-s-${index}`,
          projectId,
          title: `Session ${projectId}-${index}`,
          updatedAt: `2026-10-01T00:0${projectId}:${String(index).padStart(2, '0')}Z`,
        }),
      )
    stubSessions({ 1: many(1), 2: many(2) })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()
    await wrapper.find('.load-more').trigger('click')
    expect(wrapper.findAll('.session').length).toBe(20)

    // Opening another project expands it alongside the first: both lists stay
    // visible, each with its own page (project 1 expanded to 20, project 2 at
    // the first 15).
    await wrapper.setProps({ activeProjectId: 2 })
    await flushPromises()
    expect(wrapper.findAll('.session').length).toBe(35)
    expect(wrapper.text()).toContain('Session 2-19')
    expect(wrapper.text()).toContain('Session 1-19')
  })

  it('toggles a project on row click without opening its settings', async () => {
    stubSessions({ 1: [session({ id: 's1', title: 'Session A' })] })
    const wrapper = mountTree({ activeProjectId: null })
    await flushPromises()

    const toggle = wrapper.find('.project-toggle')
    expect(toggle.exists()).toBe(true)

    await toggle.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Session A')
    expect(wrapper.emitted('select-project')).toBeUndefined()

    await toggle.trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('Session A')
  })

  it('opens project settings from the settings control', async () => {
    stubSessions({ 1: [] })
    const wrapper = mountTree({ activeProjectId: null })
    await flushPromises()

    await wrapper.find('.project-settings').trigger('click')
    // Settings open as a workbench tab; the tree never navigates to a project
    // page, because there is none.
    expect(wrapper.emitted('open-settings')).toEqual([[1]])
    expect(wrapper.emitted('select-project')).toBeUndefined()
  })

  it('expands a project from its name without asking to navigate anywhere', async () => {
    stubSessions({ 1: [] })
    const wrapper = mountTree({ activeProjectId: null })
    await flushPromises()

    const toggle = wrapper.find('.project-toggle')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')
    await flushPromises()

    expect(wrapper.find('.project-toggle').attributes('aria-expanded')).toBe('true')
    expect(wrapper.emitted('open-settings')).toBeUndefined()
    expect(wrapper.emitted('select-project')).toBeUndefined()
  })

  it('shows the untitled placeholder instead of the internal id, in list and in search', async () => {
    stubSessions({ 1: [session({ id: 's-x', title: '' })] })
    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()

    expect(wrapper.text()).toContain('未命名会话')
    expect(wrapper.text()).not.toContain('s-x')

    // The search filter matches the placeholder, not the hidden id.
    await wrapper.setProps({ search: '未命名' })
    expect(wrapper.text()).toContain('未命名会话')
    await wrapper.setProps({ search: 's-x' })
    expect(wrapper.text()).toContain('没有匹配的会话')
  })

  // The tree keeps one list per expanded project and polls only while a turn
  // runs, so a session created or deleted in the main pane would otherwise stay
  // invisible — or linger after deletion — until the project was toggled.
  it('shows a session the moment the store creates one, and drops it when deleted', async () => {
    const sessions = useSessionsStore()
    sessions.sessions = [session()]
    // A new session, then a delete, both served from this list.
    const live = [session()]
    let created: AgentSession | null = null
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(String(input), 'http://localhost')
        const method = (init?.method ?? 'GET').toUpperCase()
        if (url.pathname === '/api/projects/1/sessions' && method === 'GET') {
          return { status: 200, json: async () => ({ sessions: live }) } as Response
        }
        if (url.pathname === '/api/projects/1/sessions' && method === 'POST') {
          created = session({ id: 'sess-2', title: 'Session B' })
          live.push(created)
          return { status: 201, json: async () => ({ session: created }) } as Response
        }
        if (url.pathname === '/api/projects/1/sessions/sess-2' && method === 'DELETE') {
          live.splice(1, 1)
          return { status: 204, json: async () => ({}) } as Response
        }
        if (url.pathname === '/api/auth/session') {
          return { status: 200, json: async () => ({ authenticated: true, csrfToken: 'csrf' }) } as Response
        }
        throw new Error(`unexpected ${method} ${url.pathname}`)
      }),
    )

    const wrapper = mountTree({ activeProjectId: 1 })
    await flushPromises()
    expect(wrapper.text()).toContain('Session A')
    expect(wrapper.text()).not.toContain('Session B')

    await sessions.createSession(1, { backend: 'pi', title: 'Session B' })
    await flushPromises()
    expect(wrapper.text()).toContain('Session B')

    await sessions.deleteSession(1, 'sess-2')
    await flushPromises()
    expect(wrapper.text()).not.toContain('Session B')
    expect(wrapper.text()).toContain('Session A')
  })
})
