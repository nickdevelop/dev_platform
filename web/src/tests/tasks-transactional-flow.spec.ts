import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { Task } from '../api/tasks'
import TasksView from '../features/tasks/TasksView.vue'
import { useSessionsStore } from '../stores/sessions'

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

function transactionalTask(overrides: Partial<Task> = {}): Task {
  return {
    id: 10,
    title: '自动化构建部署任务',
    description: '拉取最新代码并构建镜像发布至测试环境',
    projectId: 1,
    backend: 'opencode_v2',
    model: 'gpt-4o',
    flowType: 'transactional',
    currentStep: 1,
    stepStatus: 'in_progress',
    status: 'in_progress',
    isConfirmed: false,
    stepsData: '',
    activeSessionId: 'sess_analysis_1',
    scheduleType: 'none',
    scheduleEnabled: false,
    createdAt: '2026-10-17T00:00:00Z',
    updatedAt: '2026-10-17T00:00:00Z',
    ...overrides,
  }
}

describe('Tasks Transactional Flow: Stage Transitions & Prompt Extraction', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  async function mountViewWithTask(task: Task) {
    const pinia = createPinia()
    setActivePinia(pinia)

    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(String(input), 'http://localhost')
        const path = url.pathname
        const method = init?.method || 'GET'

        if (path === '/api/tasks/flows') {
          return { status: 200, json: async () => ({ flows: [] }) } as Response
        }
        if (path === '/api/tasks' && method === 'GET') {
          return { status: 200, json: async () => ({ tasks: [task] }) } as Response
        }
        if (path === '/api/projects') {
          return { status: 200, json: async () => ({ projects: [{ id: 1, name: 'CoreApp' }] }) } as Response
        }
        if (path === '/api/agent-backends') {
          return { status: 200, json: async () => ({ backends: [] }) } as Response
        }
        if (path.includes('/runs')) {
          return { status: 200, json: async () => ({ runs: [] }) } as Response
        }
        if (path.includes('/execution-prompt')) {
          const body = JSON.parse(String(init?.body || '{}'))
          return {
            status: 200,
            json: async () => ({
              ...task,
              currentStep: 2,
              isConfirmed: true,
              status: 'ready_for_execution',
              executionPrompt: body.prompt,
            }),
          } as Response
        }
        if (path.includes('/schedule')) {
          return { status: 200, json: async () => task } as Response
        }
        if (path.includes('/history')) {
          return {
            status: 200,
            json: async () => ({
              events: [
                {
                  sequence: 1,
                  type: 'message',
                  message: {
                    role: 'assistant',
                    text: `已识别项目无歧义，技术选型明确。

## 执行方案
1. git checkout main && git pull
2. docker build -t app:latest .
3. docker run -d -p 8080:8080 app:latest

请确认后即可开始执行。`,
                    final: true,
                  },
                },
              ],
              truncated: false,
            }),
          } as Response
        }
        return { status: 200, json: async () => ({}) } as Response
      }),
    )

    const wrapper = mount(TasksView, {
      global: {
        plugins: [pinia],
        stubs: {
          SessionChatPanel: {
            template: '<div class="stubbed-session-chat-panel"></div>',
            props: ['projectId', 'sessionId'],
          },
          RouterLink: true,
        },
      },
    })
    await flushPromises()
    return wrapper
  }

  it('initializes in Stage 1 with analysis banner, embedded chat, and extraction advance button', async () => {
    const task = transactionalTask()
    const wrapper = await mountViewWithTask(task)

    // Stage Stepper has 3 stages for transactional tasks
    const stageItems = wrapper.findAll('.stage-step-item')
    expect(stageItems).toHaveLength(3)
    expect(stageItems[0].classes()).toContain('active')
    expect(stageItems[0].text()).toContain('任务分析与澄清')

    // Stage 1 view elements
    expect(wrapper.find('.stage-analysis-view').exists()).toBe(true)
    expect(wrapper.find('.banner-badge').text()).toContain('步骤 1 · 任务分析与澄清')
    expect(wrapper.find('.btn-extract-next').exists()).toBe(true)
    expect(wrapper.find('.btn-extract-next').text()).toContain('提取方案并进入下一步')
  })

  it('automatically extracts execution prompt from session and transitions to Stage 2 view', async () => {
    const task = transactionalTask()
    const wrapper = await mountViewWithTask(task)

    // Click "提取方案并进入下一步"
    const extractBtn = wrapper.find('.btn-extract-next')
    await extractBtn.trigger('click')
    await flushPromises()

    // Stepper switches to Stage 2 active
    const stageItems = wrapper.findAll('.stage-step-item')
    expect(stageItems[1].classes()).toContain('active')
    expect(stageItems[1].text()).toContain('方案确认与就绪')

    // Screen switches to Stage 2 dedicated view
    expect(wrapper.find('.stage-analysis-view').exists()).toBe(false)
    expect(wrapper.find('.stage-confirm-view').exists()).toBe(true)

    // Extracted prompt is automatically populated in textarea
    const textarea = wrapper.find<HTMLTextAreaElement>('.execution-prompt-textarea')
    expect(textarea.element.value).toContain('1. git checkout main && git pull')
    expect(textarea.element.value).toContain('2. docker build -t app:latest .')
    expect(textarea.element.value).not.toContain('已识别项目无歧义')

    // Re-extract button and Confirmation notice are present
    expect(wrapper.find('.btn-re-extract').exists()).toBe(true)
    expect(wrapper.find('.confirmation-notice').exists()).toBe(true)
    expect(wrapper.find('.schedule-config-card').exists()).toBe(true)
  })

  it('confirms Stage 2 and transitions to Stage 3 execution and monitoring view', async () => {
    const task = transactionalTask()
    const wrapper = await mountViewWithTask(task)

    // Go to Stage 2
    await wrapper.find('.btn-extract-next').trigger('click')
    await flushPromises()

    // In Stage 2, confirm execution prompt
    const confirmBtn = wrapper.find('.btn-confirm-ready')
    expect(confirmBtn.text()).toContain('确认执行方案并就绪')
    await confirmBtn.trigger('click')
    await flushPromises()

    // Stepper switches to Stage 3 active
    const stageItems = wrapper.findAll('.stage-step-item')
    expect(stageItems[2].classes()).toContain('active')
    expect(stageItems[2].text()).toContain('任务执行与监控')

    // Screen switches to Stage 3 dedicated view
    expect(wrapper.find('.stage-confirm-view').exists()).toBe(false)
    expect(wrapper.find('.stage-execution-view').exists()).toBe(true)
    expect(wrapper.find('.ready-status-badge').text()).toContain('任务已就绪')
    expect(wrapper.find('.btn-trigger').exists()).toBe(true)
    expect(wrapper.find('.empty-runs-card').exists()).toBe(true)
  })

  it('allows clicking stage stepper items to navigate freely between views', async () => {
    const task = transactionalTask({
      isConfirmed: true,
      currentStep: 2,
      status: 'ready_for_execution',
      executionPrompt: 'echo "Ready to deploy"',
    })
    const wrapper = await mountViewWithTask(task)

    // Ready task starts in Stage 3
    expect(wrapper.find('.stage-execution-view').exists()).toBe(true)

    // Click Stage 1 to review analysis session
    const stageItems = wrapper.findAll('.stage-step-item')
    await stageItems[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('.stage-analysis-view').exists()).toBe(true)

    // Click Stage 2 to review or edit prompt
    await stageItems[1].trigger('click')
    await flushPromises()
    expect(wrapper.find('.stage-confirm-view').exists()).toBe(true)

    // Click Stage 3 to return to execution monitor
    await stageItems[2].trigger('click')
    await flushPromises()
    expect(wrapper.find('.stage-execution-view').exists()).toBe(true)
  })

  it('renders root workspace container and supports one-time schedule confirmation without errors', async () => {
    const task = transactionalTask()
    const wrapper = await mountViewWithTask(task)

    // Verify root layout container
    expect(wrapper.find('.transactional-flow-workspace').exists()).toBe(true)

    // Advance to Stage 2
    await wrapper.find('.btn-extract-next').trigger('click')
    await flushPromises()

    // Select "定时单次执行"
    const radioOnce = wrapper.findAll<HTMLInputElement>('input[type="radio"]')
    const onceOpt = radioOnce.find((r) => r.element.value === 'once')
    expect(onceOpt).toBeDefined()
    await onceOpt!.setValue(true)
    await flushPromises()

    // Datetime input should be present
    const datetimeInput = wrapper.find<HTMLInputElement>('.datetime-input')
    expect(datetimeInput.exists()).toBe(true)
    await datetimeInput.setValue('2026-10-25T14:30')
    await flushPromises()

    // Confirm Stage 2 without runtime ReferenceError (schedAtIso bug fix)
    const confirmBtn = wrapper.find('.btn-confirm-ready')
    await confirmBtn.trigger('click')
    await flushPromises()

    // Should successfully reach Stage 3 without error banner
    expect(wrapper.find('.action-error-banner').exists()).toBe(false)
    expect(wrapper.find('.stage-execution-view').exists()).toBe(true)
  })
})
