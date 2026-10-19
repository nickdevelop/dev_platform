import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import type { Task, TaskRun } from '../api/tasks'
import TaskCreatePanel from '../features/tasks/TaskCreatePanel.vue'
import TaskDetailView from '../features/tasks/TaskDetailView.vue'

describe('Tasks Module Unit Tests', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
    localStorage.clear()
  })

  it('TaskCreatePanel renders template inputs and execution mode choices', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const path = new URL(String(input), 'http://localhost').pathname
        const body =
          path === '/api/projects'
            ? { projects: [{ id: 1, name: 'demo-proj', path: '/demo' }] }
            : path === '/api/agent-backends'
              ? {
                  backends: [
                    { backend: 'opencode_v2', label: 'OpenCode V2', enabled: true },
                    { backend: 'pi', label: 'Pi Agent', enabled: true },
                  ],
                }
              : path === '/api/prompts'
                ? { prompts: [] }
                : {}
        return { status: 200, json: async () => body } as Response
      }),
    )

    const wrapper = mount(TaskCreatePanel, {
      props: {
        initialProjectId: 1,
      },
    })
    await flushPromises()

    // 1. Task type selector: standard active, flow disabled
    expect(wrapper.find('.task-type-selector').exists()).toBe(true)
    expect(wrapper.text()).toContain('普通任务')
    expect(wrapper.text()).toContain('流程任务')
    expect(wrapper.find('.type-card.active .type-title').text()).toBe('普通任务')

    // 2. Execution mode choices
    const modeCards = wrapper.findAll('.mode-card')
    expect(modeCards.length).toBe(3)
    expect(wrapper.text()).toContain('手动执行')
    expect(wrapper.text()).toContain('定时执行')
    expect(wrapper.text()).toContain('周期执行')

    // 3. Switch to cron shows cron config
    await modeCards[2].trigger('click')
    await flushPromises()
    expect(wrapper.find('#cron-expr').exists()).toBe(true)
  })

  it('TaskDetailView renders session template, manual trigger, and task runs table', async () => {
    const mockTask: Task = {
      id: 10,
      title: '生产环境巡检',
      description: '日常定时巡检',
      taskType: 'standard',
      projectId: 1,
      backend: 'opencode_v2',
      model: 'gpt-4o',
      promptTemplate: '请检查生产环境错误日志并汇总',
      scheduleType: 'cron',
      cronExpr: '0 9 * * *',
      scheduleEnabled: true,
      lastRunStatus: 'success',
      status: 'active',
      createdAt: '2026-10-18T00:00:00Z',
      updatedAt: '2026-10-18T00:00:00Z',
    }

    const mockRuns: TaskRun[] = [
      {
        id: 101,
        taskId: 10,
        sessionId: 'sess_task_101',
        triggerType: 'manual',
        status: 'success',
        prompt: '请检查生产环境错误日志并汇总',
        triggeredAt: '2026-10-19T09:00:00Z',
        finishedAt: '2026-10-19T09:01:30Z',
        createdAt: '2026-10-19T09:00:00Z',
      },
    ]

    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const path = new URL(String(input), 'http://localhost').pathname
        const body =
          path === '/api/projects'
            ? { projects: [{ id: 1, name: 'demo-proj', path: '/demo' }] }
            : path === '/api/tasks/10'
              ? mockTask
              : path === '/api/tasks/10/runs'
                ? { runs: mockRuns }
                : {}
        return { status: 200, json: async () => body } as Response
      }),
    )

    const wrapper = mount(TaskDetailView, {
      props: {
        taskId: 10,
      },
      global: {
        stubs: {
          TaskChatDialog: {
            props: ['title', 'projectId', 'sessionId', 'backend', 'model'],
            template: '<div data-test="task-chat-dialog" :data-session="sessionId">{{ title }}</div>',
          },
        },
      },
    })
    await flushPromises()

    // Header info
    expect(wrapper.find('.task-title').text()).toBe('生产环境巡检')
    expect(wrapper.find('.btn-trigger').exists()).toBe(true)

    // Template overview
    expect(wrapper.find('.template-content').text()).toBe('请检查生产环境错误日志并汇总')

    // Runs Table
    const table = wrapper.find('[data-test="task-runs-table"]')
    expect(table.exists()).toBe(true)
    expect(wrapper.findAll('[data-test="task-run-row"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('#101')
    expect(wrapper.text()).toContain('手动触发')
    expect(wrapper.text()).toContain('sess_task_101')

    // Click "查看会话" button opens session modal with session id
    await wrapper.find('[data-test="btn-view-session"]').trigger('click')
    await flushPromises()

    const dialog = wrapper.find('[data-test="task-chat-dialog"]')
    expect(dialog.exists()).toBe(true)
    expect(dialog.attributes('data-session')).toBe('sess_task_101')
  })
})
