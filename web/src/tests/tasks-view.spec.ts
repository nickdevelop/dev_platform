// Unit tests for the task center view. The list column is normalized to the
// project sidebar's chrome (same head/search/filter controls), and every filter
// it offers actually narrows the list — including the status filter, which was
// declared in the view but had no control before.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { Task } from '../api/tasks'
import TasksView from '../features/tasks/TasksView.vue'

// The view navigates with the router; the routing itself is covered elsewhere,
// so the composables are stubbed to keep this test on the rendered list.
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: {} }),
  useRouter: () => ({ push: vi.fn() }),
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

async function mountView(tasks: Task[]) {
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

describe('tasks view', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders the list column with the project sidebar chrome', async () => {
    const wrapper = await mountView([task({ id: 1 }), task({ id: 2, title: '另一个' })])

    // Same head / search structure the project sidebar uses, so the two columns
    // are one continuous surface.
    expect(wrapper.find('.tasks-sidebar .sidebar-head h2').text()).toBe('任务')
    expect(wrapper.find('.tasks-sidebar .head-actions button').text()).toBe('新建')
    expect(wrapper.find('.tasks-sidebar input.search').exists()).toBe(true)
    expect(wrapper.findAll('.tasks-sidebar .filter-select')).toHaveLength(3)
    expect(wrapper.findAll('.task-card')).toHaveLength(2)
  })

  it('narrows the list with the status filter', async () => {
    const wrapper = await mountView([
      task({ id: 1, title: '已完成任务', status: 'completed' }),
      task({ id: 2, title: '进行中任务', status: 'in_progress' }),
    ])

    await wrapper.find('.filter-select.status-filter').setValue('completed')

    const cards = wrapper.findAll('.task-card')
    expect(cards).toHaveLength(1)
    expect(cards[0].text()).toContain('已完成任务')
  })

  it('marks the active task and shows its flow workspace', async () => {
    const wrapper = await mountView([task({ id: 7, title: '被选中的任务' })])

    const cards = wrapper.findAll('.task-card')
    expect(cards[0].classes()).toContain('active')
    expect(wrapper.find('.task-detail-pane .task-main-title').text()).toBe('被选中的任务')
    // No session yet: the pane offers to create one instead of rendering chat.
    expect(wrapper.find('.no-session-card').exists()).toBe(true)
  })

  it('renders development workflow layout with flow-workspace-body', async () => {
    const wrapper = await mountView([
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
