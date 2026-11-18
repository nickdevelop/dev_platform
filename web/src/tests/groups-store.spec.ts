import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useGroupsStore } from '../stores/groups'
import * as groupsApi from '../api/groups'
import { installTestCSRF } from './csrf-fixture'

describe('useGroupsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    installTestCSRF()
    vi.restoreAllMocks()
  })

  it('loads groups list and handles active group detail', async () => {
    const mockGroups: groupsApi.Group[] = [
      {
        id: 1,
        name: '系统重构组',
        description: '负责核心系统重构',
        orchestratorModel: 'gpt-4o',
        orchestratorPrompt: '你是一个中控大脑',
        memberCount: 2,
        taskCount: 3,
        createdAt: '2026-11-18T10:00:00Z',
        updatedAt: '2026-11-18T10:00:00Z',
      },
    ]

    vi.spyOn(groupsApi, 'listGroups').mockResolvedValue(mockGroups)

    const store = useGroupsStore()
    await store.loadGroups()

    expect(store.groups.length).toBe(1)
    expect(store.groups[0].name).toBe('系统重构组')
    expect(store.error).toBe('')
  })

  it('creates a new group and updates store state', async () => {
    const createdDetail: groupsApi.GroupDetail = {
      id: 2,
      name: '前端攻坚组',
      description: '负责 UI 与交互',
      orchestratorModel: 'claude-3-5-sonnet',
      orchestratorPrompt: '调度前端专家',
      memberCount: 1,
      taskCount: 0,
      createdAt: '2026-11-18T11:00:00Z',
      updatedAt: '2026-11-18T11:00:00Z',
      members: [
        {
          id: 10,
          groupId: 2,
          roleId: 5,
          sortOrder: 0,
          name: 'UI 设计师',
          description: '设计高审美界面',
          backend: 'opencode',
          model: 'claude-3-5-sonnet',
          promptId: 1,
          systemPrompt: '负责 UI',
          createdAt: '2026-11-18T11:00:00Z',
        },
      ],
    }

    vi.spyOn(groupsApi, 'createGroup').mockResolvedValue(createdDetail)
    vi.spyOn(groupsApi, 'getGroup').mockResolvedValue(createdDetail)

    const store = useGroupsStore()
    const result = await store.createGroup({
      name: '前端攻坚组',
      orchestratorModel: 'claude-3-5-sonnet',
      roleIds: [5],
    })

    expect(result.id).toBe(2)
    expect(store.groups[0].id).toBe(2)
    expect(store.activeGroupId).toBe(2)
    expect(store.activeGroupDetail?.members.length).toBe(1)
  })

  it('manages group tasks with worktree scoping', async () => {
    const mockTasks: groupsApi.GroupTask[] = [
      {
        id: 101,
        groupId: 1,
        title: '用户鉴权工作树开发',
        description: '在 feat/auth 工作树下完成',
        projectId: 7,
        worktreeId: 12,
        prompt: '实现基于 session 的跨域单点登录',
        scheduleType: 'manual',
        scheduleEnabled: false,
        status: 'active',
        createdAt: '2026-11-18T12:00:00Z',
        updatedAt: '2026-11-18T12:00:00Z',
      },
    ]

    vi.spyOn(groupsApi, 'listGroupTasks').mockResolvedValue(mockTasks)

    const store = useGroupsStore()
    await store.loadGroupTasks(1)

    expect(store.groupTasks.length).toBe(1)
    expect(store.groupTasks[0].worktreeId).toBe(12)
    expect(store.groupTasks[0].projectId).toBe(7)
  })

  it('triggers task and creates collaborative flow run', async () => {
    const mockRun = {
      id: 88,
      taskId: 101,
      projectId: 7,
      status: 'running' as const,
      currentStep: 0,
      maxSteps: 20,
      orchestratorModel: 'gpt-4o',
      triggerType: 'manual',
      prompt: '执行鉴权开发',
      createdAt: '2026-11-18T12:00:00Z',
      updatedAt: '2026-11-18T12:00:00Z',
    }

    vi.spyOn(groupsApi, 'triggerGroupTask').mockResolvedValue(mockRun)

    const store = useGroupsStore()
    vi.spyOn(store, 'subscribeRunEvents').mockImplementation(() => {})
    store.groupTasks = [
      {
        id: 101,
        groupId: 1,
        title: '任务A',
        description: '',
        projectId: 7,
        worktreeId: 0,
        prompt: '目标',
        scheduleType: 'manual',
        scheduleEnabled: false,
        status: 'active',
        createdAt: '2026-11-18T12:00:00Z',
        updatedAt: '2026-11-18T12:00:00Z',
      },
    ]

    const run = await store.triggerTask(1, 101)
    expect(run.id).toBe(88)
    expect(store.activeRun?.id).toBe(88)
    expect(store.groupTasks[0].lastRunStatus).toBe('running')
  })
})
