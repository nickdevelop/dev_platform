// Unit tests for the task detail pane (see TasksView.vue). The list column
// lives in the shell's middle column (TasksSidebar.vue, covered separately);
// these tests exercise what the third column shows for the task the route — or
// the first task in the list when none was picked yet — points at.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { Task } from '../api/tasks'
import TasksView from '../features/tasks/TasksView.vue'

// The view navigates with the router; the routing itself is covered elsewhere,
// so the composables are stubbed to keep this test on the rendered pane.
const routeParams: Record<string, string> = {}
const routerPush = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: routeParams }),
  useRouter: () => ({ push: routerPush }),
}))

function task(overrides: Partial<Task> = {}): Task {
  return {
    id: 1,
    title: '任务',
    description: '',
    projectId: 1,
    backend: 'pi',
    model: 'm',
    flowType: 'transactional',
    currentStep: 1,
    stepStatus: 'in_progress',
    status: 'in_progress',
    isConfirmed: false,
    stepsData: '',
    activeSessionId: '',
    scheduleType: 'none',
    scheduleEnabled: false,
    createdAt: '2026-10-17T00:00:00Z',
    updatedAt: '2026-10-17T00:00:00Z',
    ...overrides,
  }
}

async function mountPane(tasks: Task[], params: Record<string, string> = {}) {
  Object.keys(routeParams).forEach((key) => delete routeParams[key])
  Object.assign(routeParams, params)
  routerPush.mockClear()
  const pinia = createPinia()
  setActivePinia(pinia)
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const path = new URL(String(input), 'http://localhost').pathname
      const body =
        path === '/api/tasks/flows'
          ? { flows: [] }
          : path === '/api/tasks'
            ? { tasks }
            : path === '/api/projects'
              ? { projects: [{ id: 1, name: 'alpha', path: '/workspace/alpha' }] }
              : path === '/api/agent-backends'
                ? { backends: [] }
                : {}
      return { status: 200, json: async () => body } as Response
    }),
  )
  const wrapper = mount(TasksView, { global: { plugins: [pinia] } })
  await flushPromises()
  return wrapper
}

describe('tasks detail pane', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the first task when no task detail route is active', async () => {
    const wrapper = await mountPane([task({ id: 7, title: '被选中的任务' })])

    expect(wrapper.find('.task-detail-pane .task-main-title').text()).toBe('被选中的任务')
    // No session yet: the pane offers to create one instead of rendering chat.
    expect(wrapper.find('.no-session-card').exists()).toBe(true)
  })

  it('shows the task the route points at', async () => {
    const wrapper = await mountPane(
      [task({ id: 7, title: '第一个' }), task({ id: 8, title: '路由选中' })],
      { taskId: '8' },
    )

    expect(wrapper.find('.task-detail-pane .task-main-title').text()).toBe('路由选中')
  })

  it('renders the empty state when no task exists', async () => {
    const wrapper = await mountPane([])

    expect(wrapper.find('.empty-workbench').exists()).toBe(true)
    expect(wrapper.find('.task-detail-pane').exists()).toBe(false)
  })

  it('renders development workflow layout with flow-workspace-body', async () => {
    const wrapper = await mountPane([
      task({
        id: 8,
        title: '开发类流程任务',
        flowType: 'development',
        currentStep: 1,
        status: 'in_progress',
      }),
    ])

    expect(wrapper.find('.stepper-section').exists()).toBe(true)
    expect(wrapper.find('.flow-workspace-body').exists()).toBe(true)
  })
})
