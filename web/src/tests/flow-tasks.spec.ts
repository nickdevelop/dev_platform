import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import FlowRoleBadge from '../features/tasks/flow/FlowRoleBadge.vue'
import FlowMessageItem from '../features/tasks/flow/FlowMessageItem.vue'
import FlowRoleManager from '../features/tasks/flow/FlowRoleManager.vue'
import FlowChatView from '../features/tasks/flow/FlowChatView.vue'
import TaskCreatePanel from '../features/tasks/TaskCreatePanel.vue'
import { useModelsStore } from '../stores/models'
import { useRolesStore } from '../stores/roles'
import { useTasksStore } from '../stores/tasks'
import { installTestCSRF } from './csrf-fixture'
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

  it('TaskCreatePanel flow mode selects orchestrator model from chat models and roles from role library', async () => {
    setActivePinia(createPinia())
    installTestCSRF('csrf-token')

    const modelsStore = useModelsStore()
    modelsStore.services = [
      {
        id: 1,
        name: 'OpenAI 官方服务',
        enabled: true,
        baseUrl: 'https://api.openai.com/v1',
        hasKey: true,
        secretRef: 'sec-1',
        models: [
          {
            id: 101,
            serviceId: 1,
            modelId: 'gpt-4o',
            name: 'GPT-4o 旗舰模型',
            usable: true,
            supportsText: true,
            supportsImage: true,
            supportsTools: true,
            contextTokens: 128000,
            outputTokens: 4096,
            enabled: true,
          },
        ],
      },
      {
        id: 2,
        name: 'DeepSeek',
        enabled: true,
        baseUrl: 'https://api.deepseek.com',
        hasKey: true,
        secretRef: 'sec-2',
        models: [
          {
            id: 201,
            serviceId: 2,
            modelId: 'deepseek-chat',
            name: 'DeepSeek-V3',
            usable: true,
            supportsText: true,
            supportsImage: false,
            supportsTools: true,
            contextTokens: 64000,
            outputTokens: 4096,
            enabled: true,
          },
        ],
      },
    ]

    const rolesStore = useRolesStore()
    rolesStore.roles = [
      {
        id: 1,
        name: '前端架构师',
        description: '负责前端系统架构规约',
        backend: 'llm',
        model: 'gpt-4o',
        promptId: 0,
        systemPrompt: '前端专家人设',
        color: '#3b82f6',
        createdAt: '',
        updatedAt: '',
      },
      {
        id: 2,
        name: '代码开发工程师',
        description: '负责工作区代码落地与自测',
        backend: 'opencode',
        model: 'gpt-4o',
        promptId: 0,
        systemPrompt: '资深全栈工程师',
        color: '#10b981',
        createdAt: '',
        updatedAt: '',
      },
    ]

    const wrapper = mount(TaskCreatePanel, {
      props: { initialProjectId: 1 },
      global: {
        stubs: {
          Teleport: true,
        },
      },
    })
    await flushPromises()

    // Switch to Collaborative Flow Task type
    const flowCard = wrapper.findAll('.type-card')[1]
    await flowCard.trigger('click')
    await flushPromises()

    // 1. Verify orchestrator model selector is present with options
    expect(wrapper.text()).toContain('中控调度大脑模型')
    expect(wrapper.find('#orchestrator-model').exists()).toBe(false) // No longer a raw text input

    // 2. Verify roles are loaded from role library and displayed
    expect(wrapper.text()).toContain('团队协同分工角色')
    expect(wrapper.text()).toContain('前端架构师')
    expect(wrapper.text()).toContain('代码开发工程师')

    // 3. Test removing a role
    const removeButtons = wrapper.findAll('button').filter((b) => b.text() === '移除')
    expect(removeButtons.length).toBe(2)
    await removeButtons[0].trigger('click')
    await flushPromises()

    // Now 1 role remains
    expect(wrapper.text()).toContain('已选用 1 个角色')
    const remainingCards = wrapper.findAll('.roles-config-section .grid > div')
    expect(remainingCards.length).toBe(1)
    expect(remainingCards[0].text()).toContain('代码开发工程师')
  })

  it('FlowMessageItem renders rich markdown with lists, headings, and code block copy button', async () => {
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
    ]

    const markdownMsg: FlowMessage = {
      id: 3,
      runId: 100,
      senderType: 'agent',
      senderName: '架构师',
      roleId: 1,
      content: `## 阶段计划
通知 @架构师 注意如下规范：
1. 第一步：定义接口
2. 第二步：编写测试

\`\`\`ts
// Code should preserve @架构师 verbatim
const author = "@架构师"
\`\`\`
`,
      status: 'complete',
      createdAt: '2026-11-03T10:02:00Z',
      updatedAt: '2026-11-03T10:02:00Z',
    }

    const wrapper = mount(FlowMessageItem, {
      props: { message: markdownMsg, roles },
    })

    // Check heading and list rendering
    expect(wrapper.find('h2').exists()).toBe(true)
    expect(wrapper.find('h2').text()).toBe('阶段计划')
    expect(wrapper.find('ol').exists()).toBe(true)
    expect(wrapper.findAll('li').length).toBe(2)

    // Check code block and copy button
    const codeWrapper = wrapper.find('.code-block-wrapper')
    expect(codeWrapper.exists()).toBe(true)
    const copyBtn = codeWrapper.find('.code-copy-btn')
    expect(copyBtn.exists()).toBe(true)

    // Code inside ``` must preserve raw @架构师 without being replaced into mention span
    const codeEl = wrapper.find('pre code')
    expect(codeEl.text()).toContain('const author = "@架构师"')

    // Regular text outside code should have mention span
    const mentions = wrapper.findAll('.mention-role-tag')
    expect(mentions.length).toBe(1)
    expect(mentions[0].text()).toBe('@架构师')
  })

  it('FlowChatView resets run state and does not bleed messages/status from another task when switched to an unexecuted task', async () => {
    setActivePinia(createPinia())
    const tasksStore = useTasksStore()

    const taskFailed: Task = {
      id: 2,
      title: '梳理AI知识库需求',
      description: '',
      taskType: 'flow',
      projectId: 1,
      backend: 'opencode',
      model: 'ankki-code-plus',
      orchestratorModel: 'ankki-code-plus',
      promptTemplate: '',
      scheduleType: 'manual',
      scheduleEnabled: false,
      status: 'active',
      createdAt: '2026-11-03T10:00:00Z',
      updatedAt: '2026-11-03T10:00:00Z',
    }

    const taskUnexecuted: Task = {
      id: 1,
      title: '团队会议室在线预约功能',
      description: '',
      taskType: 'flow',
      projectId: 1,
      backend: 'opencode',
      model: 'ankki-code-plus',
      orchestratorModel: 'ankki-code-plus',
      promptTemplate: '',
      scheduleType: 'manual',
      scheduleEnabled: false,
      status: 'active',
      createdAt: '2026-11-03T10:00:00Z',
      updatedAt: '2026-11-03T10:00:00Z',
    }

    const failedRun: FlowRun = {
      id: 201,
      taskId: 2,
      projectId: 1,
      status: 'failed',
      currentStep: 0,
      maxSteps: 20,
      orchestratorModel: 'ankki-code-plus',
      triggerType: 'manual',
      createdAt: '2026-11-03T10:00:00Z',
      updatedAt: '2026-11-03T10:00:00Z',
    }

    const failedMessages: FlowMessage[] = [
      {
        id: 1,
        runId: 201,
        senderType: 'user',
        senderName: '用户',
        content: '梳理AI知识库需求',
        status: 'complete',
        createdAt: '2026-11-03T10:00:00Z',
        updatedAt: '2026-11-03T10:00:00Z',
      },
    ]

    // Setup store mock state for task 2
    tasksStore.flowRuns[2] = [failedRun]
    tasksStore.flowMessages[201] = failedMessages
    tasksStore.activeFlowRun = failedRun

    // Mock store methods
    vi.spyOn(tasksStore, 'loadRoles').mockImplementation(async () => [])
    vi.spyOn(tasksStore, 'loadFlowRuns').mockImplementation(async (taskId: number) => {
      return tasksStore.flowRuns[taskId] || []
    })
    vi.spyOn(tasksStore, 'loadFlowRun').mockImplementation(async (runId: number) => {
      if (runId === 201) return failedRun
      return null
    })
    vi.spyOn(tasksStore, 'loadFlowMessages').mockImplementation(async (runId: number) => {
      return tasksStore.flowMessages[runId] || []
    })
    vi.spyOn(tasksStore, 'connectFlowEvents').mockImplementation(() => {})
    vi.spyOn(tasksStore, 'disconnectFlowEvents').mockImplementation(() => {})

    // 1. Mount FlowChatView with task 2 (the failed one)
    const wrapper = mount(FlowChatView, {
      props: { task: taskFailed },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('梳理AI知识库需求')
    expect(wrapper.text()).toContain('执行失败')
    expect(wrapper.find('[data-test="btn-rerun-flow"]').text()).toBe('重新启动')

    // 2. Switch to task 1 (the unexecuted one with no runs)
    await wrapper.setProps({ task: taskUnexecuted })
    await flushPromises()

    // 3. Assert: Task 1 should have clean unexecuted state, no bleed of task 2
    expect(wrapper.text()).toContain('团队会议室在线预约功能')
    expect(wrapper.text()).not.toContain('梳理AI知识库需求')
    expect(wrapper.text()).not.toContain('执行失败')
    expect(wrapper.find('[data-test="flow-status-idle"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="flow-status-idle"]').text()).toContain('未执行')
    expect(wrapper.find('[data-test="btn-rerun-flow"]').text()).toBe('启动协同流程')
    expect(wrapper.text()).toContain('流程尚未启动')
    expect(wrapper.text()).toContain('步骤: 0 / 20')
  })
})
