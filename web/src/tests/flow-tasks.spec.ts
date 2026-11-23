import { beforeEach, describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import FlowRoleBadge from '../features/tasks/flow/FlowRoleBadge.vue'
import FlowMessageItem from '../features/tasks/flow/FlowMessageItem.vue'
import TaskCreatePanel from '../features/tasks/TaskCreatePanel.vue'
import { useModelsStore } from '../stores/models'
import { useProjectsStore } from '../stores/projects'
import { useAgentsStore } from '../stores/agents'
import { installTestCSRF } from './csrf-fixture'
import type { FlowRole, FlowMessage } from '../api/tasks'

describe('Collaborative Flow Tasks Unit Tests', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

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

  it('TaskCreatePanel only renders standard task form without flow task mode options', async () => {
    setActivePinia(createPinia())
    installTestCSRF('csrf-token')

    const projectsStore = useProjectsStore()
    projectsStore.projects = [
      { id: 1, name: '测试项目', path: '/test', isGit: false, activeBranch: 'main' } as any,
    ]

    const agentsStore = useAgentsStore()
    agentsStore.backends = [
      { id: 'opencode', name: 'OpenCode', enabled: true, pathSource: 'auto', path: '/bin/opencode', capabilities: {} } as any,
    ]
    agentsStore.backendsLoaded = true

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

    // Flow task switch cards should not exist
    expect(wrapper.findAll('.type-card').length).toBe(0)
    expect(wrapper.text()).not.toContain('流程任务')
    expect(wrapper.text()).not.toContain('多 Agent 协同')
    expect(wrapper.text()).not.toContain('中控调度大脑模型')

    // Standard task form fields exist
    expect(wrapper.find('#task-title').exists()).toBe(true)
    expect(wrapper.text()).toContain('创建任务')
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

    // Code inside \`\`\` must preserve raw @架构师 without being replaced into mention span
    const codeEl = wrapper.find('pre code')
    expect(codeEl.text()).toContain('const author = "@架构师"')

    // Regular text outside code should have mention span
    const mentions = wrapper.findAll('.mention-role-tag')
    expect(mentions.length).toBe(1)
    expect(mentions[0].text()).toBe('@架构师')
  })
})
