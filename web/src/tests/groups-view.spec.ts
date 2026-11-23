import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import GroupsView from '../features/groups/GroupsView.vue'
import GroupsSidebar from '../features/groups/GroupsSidebar.vue'
import GroupEditPanel from '../features/groups/GroupEditPanel.vue'
import GroupDetailView from '../features/groups/GroupDetailView.vue'
import { useGroupsStore } from '../stores/groups'
import { useRolesStore } from '../stores/roles'
import { useProjectsStore } from '../stores/projects'
import { useModelsStore } from '../stores/models'

// Mock vue-router
const pushMock = vi.fn()
const replaceMock = vi.fn()
let currentRouteName = 'groups'
let currentRouteParams: Record<string, any> = {}

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: pushMock,
    replace: replaceMock,
  }),
  useRoute: () => ({
    name: currentRouteName,
    params: currentRouteParams,
    path: '/groups',
    meta: { groups: true },
  }),
  RouterLink: {
    template: '<a><slot /></a>',
  },
}))

describe('Groups View and Sidebar Component Suite', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    pushMock.mockClear()
    replaceMock.mockClear()
    currentRouteName = 'groups'
    currentRouteParams = {}

    // Mock API methods on stores
    const groupsStore = useGroupsStore()
    groupsStore.loadGroups = vi.fn().mockResolvedValue([])
    groupsStore.loadGroup = vi.fn().mockImplementation(async (id: number) => ({
      id,
      name: '系统重构攻坚组',
      description: '核心重构团队',
      orchestratorModel: 'gpt-4o',
      orchestratorPrompt: '中控规范',
      members: [],
    }))
    groupsStore.loadGroupTasks = vi.fn().mockResolvedValue([])

    const rolesStore = useRolesStore()
    rolesStore.loadRoles = vi.fn().mockResolvedValue([])

    const projectsStore = useProjectsStore()
    projectsStore.load = vi.fn().mockResolvedValue([])

    const sampleServices = [
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
            capabilities: ['chat'],
          },
        ],
      },
    ] as any
    const modelsStore = useModelsStore()
    modelsStore.services = sampleServices
    modelsStore.load = vi.fn().mockImplementation(async () => {
      modelsStore.services = sampleServices
      return sampleServices
    })
  })

  it('renders empty guidance when no groups exist and clicking create navigates to group-new route', async () => {
    const wrapper = mount(GroupsView)
    await wrapper.vm.$nextTick()

    // No modal dialog overlay should exist
    expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
    expect(wrapper.text()).toContain('团队中心')

    // Find and click create button
    const createBtn = wrapper.find('button')
    expect(createBtn.exists()).toBe(true)
    await createBtn.trigger('click')

    expect(pushMock).toHaveBeenCalledWith({ name: 'group-new' })
  })

  it('renders GroupEditPanel when route is group-new without any modal overlay', async () => {
    currentRouteName = 'group-new'
    const wrapper = mount(GroupsView)
    await wrapper.vm.$nextTick()

    // Edit panel should be mounted directly in the workspace pane
    expect(wrapper.findComponent(GroupEditPanel).exists()).toBe(true)
    expect(wrapper.find('[data-test="group-edit-panel"]').exists()).toBe(true)
    // No modal backdrop
    expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
  })

  it('renders GroupDetailView when activeGroupId is provided and does not pop up any dialog by default', async () => {
    const groupsStore = useGroupsStore()
    groupsStore.groups = [
      {
        id: 1,
        name: '研发一部',
        description: '核心开发',
        orchestratorModel: 'gpt-4o',
        orchestratorPrompt: '中控提示',
        memberCount: 2,
        taskCount: 1,
        createdAt: '2026-01-01',
        updatedAt: '2026-01-01',
      },
    ]
    currentRouteName = 'group-detail'
    currentRouteParams = { groupId: '1' }

    const wrapper = mount(GroupsView)
    await wrapper.vm.$nextTick()

    expect(wrapper.findComponent(GroupDetailView).exists()).toBe(true)
    // No modal dialog should pop up on page load
    expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
  })

  it('GroupsSidebar renders standard Zone A/B/C elements and clicking + 新建 navigates to group-new', async () => {
    const wrapper = mount(GroupsSidebar)
    await wrapper.vm.$nextTick()

    expect(wrapper.find('.sidebar-actions .action-btn').exists()).toBe(true)
    expect(wrapper.find('.sidebar-search-input').exists()).toBe(true)
    expect(wrapper.find('.sidebar-list-container').exists()).toBe(true)

    // Click new group button
    await wrapper.find('.sidebar-actions .action-btn').trigger('click')
    expect(pushMock).toHaveBeenCalledWith({ name: 'group-new' })
  })

  it('GroupEditPanel supports model selection and add/remove pattern for team members', async () => {
    const rolesStore = useRolesStore()
    const sampleRoles = [
      { id: 101, name: '架构专家', description: '负责系统架构', backend: 'pi', model: 'claude-3-5', color: '#3b82f6' } as any,
      { id: 102, name: '前端工程师', description: '负责UI开发', backend: 'builtin', model: 'gpt-4o', color: '#10b981' } as any,
    ]
    rolesStore.roles = sampleRoles
    rolesStore.loadRoles = vi.fn().mockImplementation(async () => {
      rolesStore.roles = sampleRoles
      return sampleRoles
    })

    const wrapper = mount(GroupEditPanel)
    await flushPromises()

    // Team members should use add/remove pattern
    expect(wrapper.text()).toContain('团队专家成员')
    // Member cards or add buttons should be rendered
    const removeButtons = wrapper.findAll('button').filter((b) => b.text().includes('移除'))
    // Initially first 2 roles are selected if available
    expect(removeButtons.length).toBe(2)

    // Clicking remove should remove that role
    await removeButtons[0].trigger('click')
    await flushPromises()
    const updatedRemoveButtons = wrapper.findAll('button').filter((b) => b.text().includes('移除'))
    expect(updatedRemoveButtons.length).toBe(1)
  })

  it('submits createGroup with correct roleIds payload on form save', async () => {
    const rolesStore = useRolesStore()
    rolesStore.roles = [
      { id: 101, name: '架构专家', description: '负责系统架构', backend: 'pi', model: 'claude-3-5' } as any,
    ]
    rolesStore.loadRoles = vi.fn().mockResolvedValue(rolesStore.roles)

    const groupsStore = useGroupsStore()
    groupsStore.createGroup = vi.fn().mockResolvedValue({ id: 88, name: '新建测试团队' })

    const wrapper = mount(GroupEditPanel)
    await flushPromises()

    // Fill in inputs
    const inputs = wrapper.findAll('input')
    // Name input
    await inputs[0].setValue('新团队名称')
    // Submit form
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(groupsStore.createGroup).toHaveBeenCalledWith(
      expect.objectContaining({
        name: '新团队名称',
        orchestratorModel: expect.any(String),
        roleIds: expect.arrayContaining([101]),
      }),
    )
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('submits updateGroup and updateMembers when editing an existing group', async () => {
    const existingGroup = {
      id: 7,
      name: '原团队',
      description: '原描述',
      orchestratorModel: 'gpt-4o',
      orchestratorPrompt: '原有提示词',
      memberCount: 1,
      taskCount: 0,
      createdAt: '2026-01-01',
      updatedAt: '2026-01-01',
      members: [
        { id: 1, groupId: 7, roleId: 101, sortOrder: 0, name: '架构专家', description: '', backend: 'pi', model: 'claude', promptId: 0, systemPrompt: '', createdAt: '' },
      ],
    }

    const groupsStore = useGroupsStore()
    groupsStore.updateGroup = vi.fn().mockResolvedValue(existingGroup)
    groupsStore.updateMembers = vi.fn().mockResolvedValue([])

    const wrapper = mount(GroupEditPanel, {
      props: {
        group: existingGroup as any,
      },
    })
    await flushPromises()

    // Change name
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('修改后团队名称')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(groupsStore.updateGroup).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        name: '修改后团队名称',
        orchestratorModel: 'gpt-4o',
      }),
    )
    expect(groupsStore.updateMembers).toHaveBeenCalledWith(7, [101])
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('GroupDetailView displays team info, orchestrator brain, and member expert cards', async () => {
    const groupsStore = useGroupsStore()
    groupsStore.activeGroupDetail = {
      id: 1,
      name: '架构组',
      description: '核心架构团队',
      orchestratorModel: 'gpt-4o',
      orchestratorPrompt: '中控提示',
      memberCount: 1,
      taskCount: 0,
      createdAt: '2026-01-01',
      updatedAt: '2026-01-01',
      members: [
        {
          id: 1,
          groupId: 1,
          roleId: 201,
          sortOrder: 0,
          name: '架构师',
          description: '系统方案设计',
          backend: 'builtin',
          model: 'gpt-4o',
          promptId: 0,
          systemPrompt: '',
          createdAt: '2026-01-01',
        },
      ],
    }

    const wrapper = mount(GroupDetailView, {
      props: { groupId: 1 },
    })
    await flushPromises()

    // 1. Should display team name and brain info
    expect(wrapper.text()).toContain('架构组')
    expect(wrapper.text()).toContain('中控大脑: gpt-4o')
    expect(wrapper.text()).toContain('团队专家成员')
    expect(wrapper.text()).toContain('架构师')
    expect(wrapper.text()).toContain('系统方案设计')

    // 2. Emits edit on clicking 编辑团队设置
    const editBtn = wrapper.findAll('button').find((b) => b.text().includes('编辑团队设置'))
    expect(editBtn?.exists()).toBe(true)
    await editBtn?.trigger('click')
    expect(wrapper.emitted('edit')).toBeTruthy()
  })
})
