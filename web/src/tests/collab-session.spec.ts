import { beforeEach, describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import CollabSessionChatView from '../features/groups/CollabSessionChatView.vue'
import { useGroupsStore } from '../stores/groups'
import { useProjectsStore } from '../stores/projects'
import type { AgentSession } from '../api/sessions'
import type { FlowMessage, FlowRun } from '../api/tasks'

describe('CollabSessionChatView Unit Tests', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders collaborative session view with team members, messages, and input box', async () => {
    const groupsStore = useGroupsStore()
    const projectsStore = useProjectsStore()

    const mockSession: AgentSession = {
      id: 'collab_sess_1',
      projectId: 1,
      backend: 'builtin',
      instanceId: 'inst_1',
      nativeSessionId: 'native_1',
      title: '团队协同设计会话',
      status: 'active',
      turn: 'idle',
      capabilities: {} as any,
      createdAt: '2026-11-22T10:00:00Z',
      updatedAt: '2026-11-22T10:00:00Z',
    }

    const mockMessages: FlowMessage[] = [
      {
        id: 1,
        runId: 101,
        senderType: 'user',
        senderName: '用户',
        content: '请协助设计前端架构',
        status: 'complete',
        createdAt: '2026-11-22T10:00:01Z',
        updatedAt: '2026-11-22T10:00:01Z',
      },
      {
        id: 2,
        runId: 101,
        senderType: 'orchestrator',
        senderName: '平台中控',
        content: '收到目标，正在调度架构师进行模块规划…',
        status: 'complete',
        createdAt: '2026-11-22T10:00:02Z',
        updatedAt: '2026-11-22T10:00:02Z',
      },
    ]

    const mockRun: FlowRun = {
      id: 101,
      taskId: 10,
      projectId: 1,
      status: 'running',
      currentStep: 1,
      maxSteps: 20,
      orchestratorModel: 'gpt-4o',
      triggerType: 'manual',
      createdAt: '2026-11-22T10:00:00Z',
      updatedAt: '2026-11-22T10:00:00Z',
    }

    const mockGroup = {
      id: 5,
      name: '全栈架构协同团队',
      description: '负责核心功能端到端落地',
      orchestratorModel: 'gpt-4o',
      orchestratorPrompt: '调度规范',
      memberCount: 1,
      taskCount: 1,
      createdAt: '2026-11-22T10:00:00Z',
      updatedAt: '2026-11-22T10:00:00Z',
      members: [
        {
          id: 1,
          groupId: 5,
          roleId: 201,
          sortOrder: 0,
          name: '前端架构师',
          description: '前端架构专家',
          backend: 'builtin',
          model: 'gpt-4o',
          promptId: 0,
          systemPrompt: '',
          createdAt: '2026-11-22T10:00:00Z',
        },
      ],
    }

    const mockTask = {
      id: 10,
      groupId: 5,
      title: '团队协同设计会话',
      description: '',
      projectId: 1,
      worktreeId: 0,
      prompt: '请协助设计前端架构',
      scheduleType: 'manual' as const,
      scheduleEnabled: false,
      status: 'active' as const,
      createdAt: '2026-11-22T10:00:00Z',
      updatedAt: '2026-11-22T10:00:00Z',
    }

    // Mock store context loading
    vi.spyOn(groupsStore, 'loadCollabSessionContext').mockImplementation(async () => {
      groupsStore.activeGroupDetail = mockGroup
      groupsStore.flowMessages = mockMessages
      groupsStore.activeRun = mockRun
      return {
        session: mockSession,
        group: mockGroup,
        task: mockTask,
        runs: [mockRun],
        activeRun: mockRun,
      }
    })

    vi.spyOn(groupsStore, 'loadRunMessages').mockImplementation(async () => {
      groupsStore.flowMessages = mockMessages
      return mockMessages
    })
    vi.spyOn(groupsStore, 'subscribeRunEvents').mockImplementation(() => {})
    const sendSpy = vi.spyOn(groupsStore, 'sendUserMessage').mockResolvedValue({
      id: 3,
      runId: 101,
      senderType: 'user',
      senderName: '用户',
      content: '追问：支持暗色模式吗？',
      status: 'complete',
      createdAt: '2026-11-22T10:00:05Z',
      updatedAt: '2026-11-22T10:00:05Z',
    })

    const wrapper = mount(CollabSessionChatView, {
      props: {
        projectId: 1,
        sessionId: 'collab_sess_1',
        session: mockSession,
      },
    })
    await flushPromises()

    // 1. Assert team header & member
    expect(wrapper.text()).toContain('全栈架构协同团队')
    expect(wrapper.text()).toContain('中控大脑: gpt-4o')
    expect(wrapper.text()).toContain('前端架构师')

    // 2. Assert message bubbles rendered
    expect(wrapper.text()).toContain('请协助设计前端架构')
    expect(wrapper.text()).toContain('收到目标，正在调度架构师进行模块规划…')

    // 3. Send a new message
    const textarea = wrapper.find('textarea')
    expect(textarea.exists()).toBe(true)
    await textarea.setValue('追问：支持暗色模式吗？')

    // Click send via form submit or enter
    const form = wrapper.find('form')
    expect(form.exists()).toBe(true)
    await form.trigger('submit')
    await flushPromises()

    expect(sendSpy).toHaveBeenCalledWith(101, '追问：支持暗色模式吗？')
    // Textarea cleared
    expect((textarea.element as HTMLTextAreaElement).value).toBe('')
  })
})
