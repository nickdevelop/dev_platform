import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import FlowRoleBadge from '../features/tasks/flow/FlowRoleBadge.vue'
import FlowMessageItem from '../features/tasks/flow/FlowMessageItem.vue'
import FlowRoleManager from '../features/tasks/flow/FlowRoleManager.vue'
import FlowChatView from '../features/tasks/flow/FlowChatView.vue'
import type { Task, FlowRole, FlowMessage, FlowRun } from '../api/tasks'

describe('Collaborative Flow Tasks Unit Tests', () => {
  it('FlowRoleBadge renders role name, color dot, backend and model', () => {
    const role: FlowRole = {
      id: 1,
      taskId: 10,
      name: '架构师',
      description: '负责系统设计',
      backend: 'llm',
      model: 'gpt-4o',
      systemPrompt: '系统架构师',
      color: '#8b5cf6',
      createdAt: '',
      updatedAt: '',
    }

    const wrapper = mount(FlowRoleBadge, {
      props: { role },
    })

    expect(wrapper.text()).toContain('架构师')
    expect(wrapper.text()).toContain('llm')
    expect(wrapper.text()).toContain('gpt-4o')
    const dot = wrapper.find('.rounded-full')
    expect(dot.exists()).toBe(true)
    expect(dot.attributes('style')).toContain('background-color: rgb(139, 92, 246)')
  })

  it('FlowMessageItem renders user bubble on the right', () => {
    const userMsg: FlowMessage = {
      id: 1,
      runId: 100,
      senderType: 'user',
      senderName: '用户',
      content: '请设计并实现一个多 Agent 协同原型',
      status: 'complete',
      createdAt: '2026-11-03T10:00:00Z',
      updatedAt: '2026-11-03T10:00:00Z',
    }

    const wrapper = mount(FlowMessageItem, {
      props: { message: userMsg },
    })

    expect(wrapper.find('[data-test="msg-user"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('请设计并实现一个多 Agent 协同原型')
    expect(wrapper.find('[data-test="msg-user"]').classes()).toContain('flex-row-reverse')
  })

  it('FlowMessageItem renders orchestrator turn and highlights @role mentions', () => {
    const roles: FlowRole[] = [
      {
        id: 1,
        taskId: 10,
        name: '架构师',
        backend: 'llm',
        model: 'gpt-4o',
        color: '#8b5cf6',
        createdAt: '',
        updatedAt: '',
      },
      {
        id: 2,
        taskId: 10,
        name: '开发专家',
        backend: 'opencode',
        model: 'gpt-4o',
        color: '#3b82f6',
        createdAt: '',
        updatedAt: '',
      },
    ]

    const orchestratorMsg: FlowMessage = {
      id: 2,
      runId: 100,
      senderType: 'orchestrator',
      senderName: '中控调度员',
      content: '收到目标，现在指派任务：@架构师 请输出模块架构规约',
      status: 'complete',
      createdAt: '2026-11-03T10:01:00Z',
      updatedAt: '2026-11-03T10:01:00Z',
    }

    const wrapper = mount(FlowMessageItem, {
      props: { message: orchestratorMsg, roles },
    })

    expect(wrapper.find('[data-test="msg-orchestrator"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('平台中控')
    expect(wrapper.html()).toContain('mention-role-tag')
    expect(wrapper.html()).toContain('@架构师')
  })

  it('FlowMessageItem renders agent feedback with attached workspace session', async () => {
    const role: FlowRole = {
      id: 2,
      taskId: 10,
      name: '开发专家',
      backend: 'opencode',
      model: 'gpt-4o',
      color: '#3b82f6',
      createdAt: '',
      updatedAt: '',
    }

    const agentMsg: FlowMessage = {
      id: 3,
      runId: 100,
      senderType: 'agent',
      senderName: '开发专家',
      roleId: 2,
      content: '已根据方案完成代码修改，测试全部通过。',
      sessionId: 'sess_999',
      status: 'complete',
      createdAt: '2026-11-03T10:02:00Z',
      updatedAt: '2026-11-03T10:02:00Z',
    }

    const wrapper = mount(FlowMessageItem, {
      props: { message: agentMsg, role },
    })

    expect(wrapper.find('[data-test="msg-agent"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('开发专家')
    expect(wrapper.text()).toContain('sess_999')

    // Click open session
    const openBtn = wrapper.find('button')
    expect(openBtn.exists()).toBe(true)
    await openBtn.trigger('click')
    expect(wrapper.emitted('openSession')?.[0]).toEqual(['sess_999'])
  })

  it('FlowRoleManager displays roles and allows adding a new role', async () => {
    setActivePinia(createPinia())
    const roles: FlowRole[] = [
      {
        id: 1,
        taskId: 10,
        name: '架构师',
        description: '系统架构分析与方案拆解',
        backend: 'llm',
        model: 'gpt-4o',
        systemPrompt: '你是系统架构师',
        color: '#8b5cf6',
        createdAt: '',
        updatedAt: '',
      },
    ]

    const wrapper = mount(FlowRoleManager, {
      props: {
        taskId: 10,
        roles,
      },
    })

    expect(wrapper.text()).toContain('Agent 协同角色分工')
    expect(wrapper.text()).toContain('1 个角色')
    expect(wrapper.find('[data-test="role-card-1"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('架构师')

    // Open add dialog
    const addBtn = wrapper.find('[data-test="btn-add-role"]')
    expect(addBtn.exists()).toBe(true)
    await addBtn.trigger('click')
    await flushPromises()

    expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('新建 Agent 角色')
  })

  it('FlowChatView renders group chat workbench, team roles strip, and input area', async () => {
    setActivePinia(createPinia())
    const mockTask: Task = {
      id: 10,
      title: '多 Agent 协同测试任务',
      description: '全流程协作',
      taskType: 'flow',
      projectId: 1,
      backend: 'opencode',
      model: 'gpt-4o',
      orchestratorModel: 'gpt-4o',
      promptTemplate: '完成全套实现',
      scheduleType: 'manual',
      scheduleEnabled: false,
      status: 'active',
      createdAt: '2026-11-03T10:00:00Z',
      updatedAt: '2026-11-03T10:00:00Z',
    }

    const wrapper = mount(FlowChatView, {
      props: {
        task: mockTask,
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-test="flow-chat-view"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('多 Agent 协同群聊')
    expect(wrapper.text()).toContain('多 Agent 协同测试任务')
    expect(wrapper.find('[data-test="flow-messages-area"]').exists()).toBe(true)
    expect(wrapper.find('textarea').exists()).toBe(true)
  })
})
