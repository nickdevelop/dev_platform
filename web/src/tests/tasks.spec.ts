import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { useTasksStore } from '../stores/tasks'
import type { FlowDef, Task } from '../api/tasks'
import FlowStepper from '../features/tasks/FlowStepper.vue'
import ScheduleConfigDialog from '../features/tasks/ScheduleConfigDialog.vue'

describe('Tasks Module Tests', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
  })

  it('flow stepper renders transactional flow and critical confirmation notice', async () => {
    const flowDef: FlowDef = {
      type: 'transactional',
      name: '事务型',
      description: '事务型流程描述',
      steps: [
        {
          index: 1,
          key: 'confirm',
          name: '任务确认',
          description: '确认所有事项',
          prompt: '请向用户确认所有待办事项',
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
      stepsData: '',
      activeSessionId: '',
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

    // Stepper track has 2 steps
    expect(wrapper.findAll('.step-item')).toHaveLength(2)
    expect(wrapper.text()).toContain('任务确认')
    expect(wrapper.text()).toContain('任务执行')

    // At step 1, confirmation notice must be visible
    expect(wrapper.find('.confirmation-notice').exists()).toBe(true)
    expect(wrapper.text()).toContain('在【任务确认】阶段，应将任务执行过程中所可能遇到的需要确认的事项确认清楚')

    // Click advance button emits advance
    await wrapper.find('.btn-advance').trigger('click')
    expect(wrapper.emitted('advance')).toBeTruthy()
    expect(wrapper.emitted('advance')![0][0]).toBe(true)
  })

  it('flow stepper renders development flow with 4 steps', async () => {
    const flowDef: FlowDef = {
      type: 'development',
      name: '开发类',
      description: '开发类流程描述',
      steps: [
        { index: 1, key: 'analysis', name: '需求分析', description: '', prompt: '', isFinal: false },
        { index: 2, key: 'design', name: '需求设计', description: '', prompt: '', isFinal: false },
        { index: 3, key: 'plan', name: '制定计划', description: '', prompt: '', isFinal: false },
        { index: 4, key: 'execute', name: '执行', description: '', prompt: '', isFinal: true },
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
      currentStep: 3,
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

    const wrapper = mount(FlowStepper, {
      props: {
        task,
        flowDef,
      },
    })

    expect(wrapper.findAll('.step-item')).toHaveLength(4)
    expect(wrapper.find('.confirmation-notice').exists()).toBe(true)
    expect(wrapper.text()).toContain('在【制定计划】步骤中，需要将所有确认的事项确认清楚')
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
