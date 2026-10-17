import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { mount } from '@vue/test-utils'
import type { FlowDef, Task } from '../api/tasks'
import FlowStepper from '../features/tasks/FlowStepper.vue'
import ScheduleConfigDialog from '../features/tasks/ScheduleConfigDialog.vue'

describe('Tasks Module Tests', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
  })

  it('flow stepper renders transactional flow with analysis step and execution prompt card', async () => {
    const flowDef: FlowDef = {
      type: 'transactional',
      name: '事务型',
      description: '事务型流程描述',
      steps: [
        {
          index: 1,
          key: 'analysis',
          name: '任务分析',
          description: 'Agent 分析任务、识别歧义与不确定点并输出执行计划',
          prompt: '请分析任务需求，识别信息是否有歧义、不确定点与风险',
          isFinal: false,
        },
        {
          index: 2,
          key: 'execute',
          name: '任务执行',
          description: '免打断执行任务',
          prompt: '执行任务',
          isFinal: true,
        },
      ],
    }

    const task: Task = {
      id: 1,
      title: '事务测试任务',
      description: '测试描述',
      projectId: 1,
      backend: 'opencode_v2',
      model: 'gpt-4o',
      flowType: 'transactional',
      currentStep: 1,
      stepStatus: 'in_progress',
      status: 'in_progress',
      isConfirmed: false,
      executionPrompt: '初始执行提示词',
      stepsData: '',
      activeSessionId: 'sess_1',
      scheduleType: 'none',
      scheduleEnabled: false,
      createdAt: '2026-10-17T00:00:00Z',
      updatedAt: '2026-10-17T00:00:00Z',
    }

    const wrapper = mount(FlowStepper, {
      props: {
        task,
        flowDef,
      },
    })

    // Stepper track has 2 steps with "任务分析" as step 1
    expect(wrapper.findAll('.step-item')).toHaveLength(2)
    expect(wrapper.text()).toContain('任务分析')
    expect(wrapper.text()).toContain('任务执行')

    // At step 1, confirmation notice must be visible and emphasize ambiguity identification
    expect(wrapper.find('.confirmation-notice').exists()).toBe(true)
    expect(wrapper.text()).toContain('在【任务分析】阶段，Agent 需分析任务需求，识别信息是否有歧义')

    // Execution prompt box must be visible
    expect(wrapper.find('.execution-prompt-box').exists()).toBe(true)
    expect(wrapper.text()).toContain('提炼第二步执行提示词')

    // Click save prompt emits savePrompt with text
    await wrapper.find('.btn-save-prompt').trigger('click')
    expect(wrapper.emitted('savePrompt')).toBeTruthy()
    expect(wrapper.emitted('savePrompt')![0][0]).toBe('初始执行提示词')
  })

  it('flow stepper renders development flow and stage transition modal with agent invocation', async () => {
    const flowDef: FlowDef = {
      type: 'development',
      name: '开发类',
      description: '开发类流程描述',
      steps: [
        { index: 1, key: 'analysis', name: '需求分析', description: '', prompt: '分析需求', isFinal: false },
        { index: 2, key: 'design', name: '需求设计', description: '', prompt: '设计方案', isFinal: false },
        { index: 3, key: 'plan', name: '制定计划', description: '', prompt: '制定计划', isFinal: false },
        { index: 4, key: 'execute', name: '执行', description: '', prompt: '执行代码', isFinal: true },
      ],
    }

    const task: Task = {
      id: 2,
      title: '开发测试任务',
      description: '开发类测试描述',
      projectId: 1,
      backend: 'pi',
      model: 'claude-3-5-sonnet',
      flowType: 'development',
      currentStep: 1,
      stepStatus: 'in_progress',
      status: 'in_progress',
      isConfirmed: false,
      stepsData: '',
      activeSessionId: 'sess_dev_1',
      scheduleType: 'none',
      scheduleEnabled: false,
      createdAt: '2026-10-17T00:00:00Z',
      updatedAt: '2026-10-17T00:00:00Z',
    }

    const wrapper = mount(FlowStepper, {
      props: {
        task,
        flowDef,
      },
    })

    expect(wrapper.findAll('.step-item')).toHaveLength(4)

    // Click advance button opens stage transition modal
    await wrapper.find('.btn-advance').trigger('click')
    expect(wrapper.find('.stage-modal').exists()).toBe(true)
    expect(wrapper.text()).toContain('确认推进至第 2 步：需求设计')

    // Fill user supplement and click send
    await wrapper.find('.supplement-textarea').setValue('请注意表结构规范')
    await wrapper.find('.modal-actions .btn-primary').trigger('click')

    expect(wrapper.emitted('advance')).toBeTruthy()
    const payload = wrapper.emitted('advance')![0]
    expect(payload[0]).toBe(true)
    expect(payload[2]).toContain('请注意表结构规范')
    expect(payload[2]).toContain('推进至第 2 步：需求设计')
  })

  it('schedule dialog gates scheduling when task is not at final step or unconfirmed', async () => {
    const unreadyTask: Task = {
      id: 3,
      title: '未就绪任务',
      description: '',
      projectId: 1,
      backend: 'opencode_v2',
      model: 'gpt-4o',
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
    }

    const wrapper = mount(ScheduleConfigDialog, {
      props: {
        task: unreadyTask,
      },
    })

    // Guard notice must be displayed
    expect(wrapper.find('.guard-notice').exists()).toBe(true)
    expect(wrapper.text()).toContain('未满足自动调度条件')

    // Switch input must be disabled
    const switchInput = wrapper.find('.switch-input')
    expect((switchInput.element as HTMLInputElement).disabled).toBe(true)
  })

  it('schedule dialog allows scheduling when task is at final step and confirmed', async () => {
    const readyTask: Task = {
      id: 4,
      title: '已就绪事务任务',
      description: '',
      projectId: 1,
      backend: 'opencode_v2',
      model: 'gpt-4o',
      flowType: 'transactional',
      currentStep: 2,
      stepStatus: 'confirmed',
      status: 'ready_for_execution',
      isConfirmed: true,
      executionPrompt: '可直接执行提示词',
      stepsData: '',
      activeSessionId: '',
      scheduleType: 'cron',
      cronExpr: '0 2 * * *',
      scheduleEnabled: true,
      createdAt: '2026-10-17T00:00:00Z',
      updatedAt: '2026-10-17T00:00:00Z',
    }

    const wrapper = mount(ScheduleConfigDialog, {
      props: {
        task: readyTask,
      },
    })

    // Guard notice should NOT be displayed
    expect(wrapper.find('.guard-notice').exists()).toBe(false)

    // Switch input should be enabled and checked
    const switchInput = wrapper.find('.switch-input')
    expect((switchInput.element as HTMLInputElement).disabled).toBe(false)
    expect((switchInput.element as HTMLInputElement).checked).toBe(true)

    // Cron options visible for transactional
    expect(wrapper.text()).toContain('周期执行（Cron）')
  })
})
