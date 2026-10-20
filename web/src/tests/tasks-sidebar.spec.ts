// Unit tests for TasksSidebar.vue
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { Task } from '../api/tasks'
import TasksSidebar from '../features/tasks/TasksSidebar.vue'

const routeParams: Record<string, string> = {}
let routeName = 'tasks'
const routerPush = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: routeName, params: routeParams }),
  useRouter: () => ({ push: routerPush }),
}))

function mockTask(overrides: Partial<Task> = {}): Task {
  return {
    id: 1,
    title: '任务标题',
    description: '',
    taskType: 'standard',
    projectId: 1,
    backend: 'opencode_v2',
    model: 'gpt-4o',
    promptTemplate: '提示词模板',
    scheduleType: 'manual',
    scheduleEnabled: false,
    lastRunStatus: 'idle',
    status: 'active',
    createdAt: '2026-10-18T00:00:00Z',
    updatedAt: '2026-10-18T00:00:00Z',
    ...overrides,
  }
}

async function mountSidebar(tasks: Task[], params: Record<string, string> = {}, rName = 'tasks') {
  Object.keys(routeParams).forEach((key) => delete routeParams[key])
  Object.assign(routeParams, params)
  routeName = rName
  routerPush.mockClear()

  const pinia = createPinia()
  setActivePinia(pinia)

  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const path = new URL(String(input), 'http://localhost').pathname
      const body =
        path === '/api/tasks'
          ? { tasks }
          : path === '/api/projects'
            ? { projects: [{ id: 1, name: 'alpha', path: '/workspace/alpha' }] }
            : {}
      return { status: 200, json: async () => body } as Response
    }),
  )

  const wrapper = mount(TasksSidebar, { global: { plugins: [pinia] } })
  await flushPromises()
  return wrapper
}

describe('TasksSidebar Component', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders the sidebar with search, filters, and task cards', async () => {
    const wrapper = await mountSidebar([mockTask({ id: 1 }), mockTask({ id: 2, title: '另一个任务' })])

    expect(wrapper.find('.sidebar-head h2').text()).toBe('任务')
    expect(wrapper.find('.new-task-btn').text()).toContain('新建')
    expect(wrapper.find('.search-input').exists()).toBe(true)
    expect(wrapper.findAll('.filter-select')).toHaveLength(2)
    expect(wrapper.findAll('.task-card')).toHaveLength(2)
  })

  it('filters tasks by type filter', async () => {
    const wrapper = await mountSidebar([
      mockTask({ id: 1, title: '标准任务', taskType: 'standard' }),
      mockTask({ id: 2, title: '流程任务', taskType: 'flow' }),
    ])

    await wrapper.find('.filter-select.type-filter').setValue('flow')
    await flushPromises()

    const cards = wrapper.findAll('.task-card')
    expect(cards).toHaveLength(1)
    expect(cards[0].text()).toContain('流程任务')
  })

  it('marks the active task matching route params', async () => {
    const wrapper = await mountSidebar(
      [mockTask({ id: 7, title: '选中的' }), mockTask({ id: 8, title: '未选中的' })],
      { taskId: '7' },
      'task-detail',
    )

    const cards = wrapper.findAll('.task-card')
    expect(cards[0].classes()).toContain('active')
    expect(cards[1].classes()).not.toContain('active')
  })

  it('clicking "新建" button navigates to task-new', async () => {
    const wrapper = await mountSidebar([mockTask({ id: 1 })])
    await wrapper.find('.new-task-btn').trigger('click')

    expect(routerPush).toHaveBeenCalledWith({ name: 'task-new' })
  })

  it('clicking a task card navigates to task-detail', async () => {
    const wrapper = await mountSidebar([mockTask({ id: 12, title: '点击卡片' })])
    await wrapper.find('.task-card').trigger('click')

    expect(routerPush).toHaveBeenCalledWith({ name: 'task-detail', params: { taskId: 12 } })
  })

  it('renders title and new task button in separated rows to avoid overlap with dock controls', async () => {
    const wrapper = await mountSidebar([mockTask({ id: 1 })])

    const head = wrapper.find('.sidebar-head')
    expect(head.find('h2').text()).toBe('任务')
    expect(head.findAll('button')).toHaveLength(0)

    const actions = wrapper.find('.sidebar-actions')
    expect(actions.exists()).toBe(true)
    expect(actions.find('.new-task-btn').text()).toContain('新建')
  })
})
