// Unit tests for TasksView.vue
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { Task, TaskRun } from '../api/tasks'
import TasksView from '../features/tasks/TasksView.vue'

const routeState = {
  name: 'tasks' as string,
  params: {} as Record<string, string>,
}
const routerPush = vi.fn()
const routerReplace = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: routerPush, replace: routerReplace }),
}))

function mockTask(overrides: Partial<Task> = {}): Task {
  return {
    id: 1,
    title: '普通会话模板任务',
    description: '日常检查',
    taskType: 'standard',
    projectId: 1,
    backend: 'opencode_v2',
    model: 'gpt-4o',
    promptTemplate: '检查并输出分析',
    scheduleType: 'manual',
    scheduleEnabled: false,
    lastRunStatus: 'idle',
    status: 'active',
    createdAt: '2026-10-18T00:00:00Z',
    updatedAt: '2026-10-18T00:00:00Z',
    ...overrides,
  }
}

async function mountView(
  tasks: Task[] = [],
  routeOverrides: { name?: string; params?: Record<string, string> } = {},
  runs: TaskRun[] = [],
) {
  routeState.name = routeOverrides.name ?? 'tasks'
  routeState.params = routeOverrides.params ?? {}
  routerPush.mockClear()
  routerReplace.mockClear()

  const pinia = createPinia()
  setActivePinia(pinia)

  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input), 'http://localhost')
      const path = url.pathname

      if (path === '/api/tasks') {
        return { status: 200, json: async () => ({ tasks }) } as Response
      }
      if (path.startsWith('/api/tasks/') && path.endsWith('/runs')) {
        return { status: 200, json: async () => ({ runs }) } as Response
      }
      if (path.startsWith('/api/tasks/')) {
        const id = Number(path.split('/')[3])
        const found = tasks.find((t) => t.id === id) || tasks[0]
        return { status: 200, json: async () => found } as Response
      }
      if (path === '/api/projects') {
        return { status: 200, json: async () => ({ projects: [{ id: 1, name: 'demo', path: '/demo' }] }) } as Response
      }
      if (path === '/api/agent-backends') {
        return { status: 200, json: async () => ({ backends: [] }) } as Response
      }
      if (path === '/api/prompts') {
        return { status: 200, json: async () => ({ prompts: [] }) } as Response
      }
      return { status: 200, json: async () => ({}) } as Response
    }),
  )

  const wrapper = mount(TasksView, { global: { plugins: [pinia] } })
  await flushPromises()
  return wrapper
}

describe('TasksView Workspace View', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders empty workbench when no tasks exist', async () => {
    const wrapper = await mountView([], { name: 'tasks' })
    expect(wrapper.find('[data-test="empty-workbench"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="btn-create-first-task"]').exists()).toBe(true)
  })

  it('renders TaskCreatePanel when route is task-new', async () => {
    const wrapper = await mountView([], { name: 'task-new' })
    expect(wrapper.find('[data-test="task-create-panel"]').exists()).toBe(true)
  })

  it('renders TaskDetailView when a task is selected', async () => {
    const t = mockTask({ id: 9, title: '特定任务' })
    const wrapper = await mountView([t], { name: 'task-detail', params: { taskId: '9' } })

    expect(wrapper.find('[data-test="task-detail-view"]').exists()).toBe(true)
    expect(wrapper.find('.task-title').text()).toBe('特定任务')
  })
})
