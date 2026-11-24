import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import AssistantListView from '../features/organization/assistants/AssistantListView.vue'
import AssistantEditPanel from '../features/organization/assistants/AssistantEditPanel.vue'
import EmployeeListView from '../features/organization/employees/EmployeeListView.vue'
import EmployeeEditPanel from '../features/organization/employees/EmployeeEditPanel.vue'
import TeamListView from '../features/organization/teams/TeamListView.vue'
import TeamEditPanel from '../features/organization/teams/TeamEditPanel.vue'
import { useChatsStore } from '../stores/chats'
import { useRolesStore } from '../stores/roles'
import { useGroupsStore } from '../stores/groups'
import { useModelsStore } from '../stores/models'
import { usePromptsStore } from '../stores/prompts'
import { useAgentsStore } from '../stores/agents'
import { installTestCSRF } from './csrf-fixture'

const pushMock = vi.fn()
const replaceMock = vi.fn()
let currentRouteQuery: Record<string, any> = {}

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: pushMock,
    replace: replaceMock,
  }),
  useRoute: () => ({
    query: currentRouteQuery,
    params: {},
    path: '/org',
  }),
}))

describe('Organization Center Re-architecture (No Popups & Consistent Layout)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    installTestCSRF('csrf-token')
    pushMock.mockClear()
    replaceMock.mockClear()
    currentRouteQuery = {}
    vi.restoreAllMocks()

    // Mock Models Store
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
            capabilities: ['chat'],
          },
        ],
      },
    ] as any
    modelsStore.load = vi.fn().mockResolvedValue(modelsStore.services)

    // Mock Prompts Store
    const promptsStore = usePromptsStore()
    promptsStore.prompts = [
      {
        id: 1,
        name: '全栈架构专家人设',
        type: 'role',
        description: '系统架构与技术选型人设',
        content: '你是一个经验丰富的高级架构专家，负责系统架构把关与设计规范制定。',
        createdAt: '',
        updatedAt: '',
      },
    ]
    promptsStore.loadPrompts = vi.fn().mockResolvedValue(promptsStore.prompts)

    // Mock Agents Store
    const agentsStore = useAgentsStore()
    agentsStore.backends = [
      {
        backend: 'opencode_v2',
        label: 'OpenCode V2',
        enabled: true,
      },
      {
        backend: 'pi',
        label: 'Pi Agent',
        enabled: true,
      },
    ] as any
    agentsStore.loadBackends = vi.fn().mockResolvedValue(agentsStore.backends)
  })

  describe('AI Assistants (AssistantListView & AssistantEditPanel)', () => {
    it('renders assistant list without modal dialogs and clicking create switches to AssistantEditPanel in-place', async () => {
      const chatsStore = useChatsStore()
      chatsStore.groups = [
        {
          id: 1,
          name: '默认开发助手',
          icon: '🤖',
          defaultModelId: 101,
          promptId: 1,
          systemPrompt: '',
          enablePlatformTools: true,
          maxToolSteps: 10,
          showInFloat: true,
          createdAt: '',
          updatedAt: '',
        },
      ]
      chatsStore.loadGroups = vi.fn().mockResolvedValue([])

      const wrapper = mount(AssistantListView)
      await flushPromises()

      // 1. Verify list rendered and zero modal backdrops
      expect(wrapper.text()).toContain('AI 助手')
      expect(wrapper.text()).toContain('1 个助手')
      expect(wrapper.text()).toContain('默认开发助手')
      expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
      expect(wrapper.findComponent(AssistantEditPanel).exists()).toBe(false)

      // 2. Click "+ 新建 AI 助手"
      const createBtn = wrapper.find('[data-test="btn-create-assistant"]')
      expect(createBtn.exists()).toBe(true)
      await createBtn.trigger('click')
      await flushPromises()

      // 3. Verify in-place AssistantEditPanel is rendered, NO dialogs
      expect(wrapper.findComponent(AssistantEditPanel).exists()).toBe(true)
      expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
      expect(wrapper.text()).toContain('新建 AI 助手')
      expect(wrapper.text()).toContain('01 基础档案与视觉标识')
      expect(wrapper.text()).toContain('02 运行模型与推理配置')
      expect(wrapper.text()).toContain('03 人设规范与系统提示词')
      expect(wrapper.text()).toContain('04 交互形态与自治能力')

      // 4. Click cancel button and verify returning to list
      const cancelBtn = wrapper.find('button[title="返回列表 (Esc)"]')
      expect(cancelBtn.exists()).toBe(true)
      await cancelBtn.trigger('click')
      await flushPromises()

      expect(wrapper.findComponent(AssistantEditPanel).exists()).toBe(false)
      expect(wrapper.text()).toContain('AI 助手')
    })

    it('opens edit mode when clicking edit on assistant row without popups', async () => {
      const chatsStore = useChatsStore()
      const sampleAssistant = {
        id: 2,
        name: '架构顾问',
        icon: '💡',
        defaultModelId: 101,
        promptId: 1,
        systemPrompt: '',
        enablePlatformTools: false,
        maxToolSteps: 10,
        showInFloat: false,
        createdAt: '',
        updatedAt: '',
      }
      chatsStore.groups = [sampleAssistant]
      chatsStore.loadGroups = vi.fn().mockResolvedValue([])

      const wrapper = mount(AssistantListView)
      await flushPromises()

      // Click "编辑" button
      const editBtn = wrapper.find('button[title="编辑配置"]')
      expect(editBtn.exists()).toBe(true)
      await editBtn.trigger('click')
      await flushPromises()

      // Verify in-place panel opens with assistant data, no dialog
      expect(wrapper.findComponent(AssistantEditPanel).exists()).toBe(true)
      expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
      expect(wrapper.text()).toContain('编辑 AI 助手 · 架构顾问')
    })
  })

  describe('AI Employees (EmployeeListView & EmployeeEditPanel)', () => {
    it('renders employee list without modal dialogs and clicking create switches to EmployeeEditPanel in-place', async () => {
      const rolesStore = useRolesStore()
      rolesStore.roles = [
        {
          id: 10,
          name: '前端架构师',
          description: '主导 Vue3 与工程化设计',
          backend: 'llm',
          model: 'gpt-4o',
          color: '#3b82f6',
          promptId: 1,
          systemPrompt: '',
          createdAt: '',
          updatedAt: '',
        },
      ]
      rolesStore.loadRoles = vi.fn().mockResolvedValue(rolesStore.roles)

      const wrapper = mount(EmployeeListView)
      await flushPromises()

      // 1. Verify list rendered and zero modal backdrops
      expect(wrapper.text()).toContain('AI 员工')
      expect(wrapper.text()).toContain('1 名员工')
      expect(wrapper.text()).toContain('前端架构师')
      expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
      expect(wrapper.findComponent(EmployeeEditPanel).exists()).toBe(false)

      // 2. Click "+ 新建 AI 员工"
      const createBtn = wrapper.find('[data-test="btn-create-employee"]')
      expect(createBtn.exists()).toBe(true)
      await createBtn.trigger('click')
      await flushPromises()

      // 3. Verify in-place EmployeeEditPanel is rendered, NO dialogs
      expect(wrapper.findComponent(EmployeeEditPanel).exists()).toBe(true)
      expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
      expect(wrapper.text()).toContain('新建 AI 员工')
      expect(wrapper.text()).toContain('01 基础档案与视觉标识')
      expect(wrapper.text()).toContain('02 执行引擎与运行模型')
      expect(wrapper.text()).toContain('03 人设规范与系统提示词')

      // 4. Click cancel button and verify returning to list
      const cancelBtn = wrapper.find('button[title="返回列表 (Esc)"]')
      expect(cancelBtn.exists()).toBe(true)
      await cancelBtn.trigger('click')
      await flushPromises()

      expect(wrapper.findComponent(EmployeeEditPanel).exists()).toBe(false)
      expect(wrapper.text()).toContain('AI 员工')
    })

    it('opens edit mode when clicking edit on employee row without popups', async () => {
      const rolesStore = useRolesStore()
      const sampleEmployee = {
        id: 11,
        name: '代码审查专家',
        description: '严格的代码质量审计',
        backend: 'opencode_v2',
        model: 'default',
        color: '#10b981',
        promptId: 1,
        systemPrompt: '',
        createdAt: '',
        updatedAt: '',
      }
      rolesStore.roles = [sampleEmployee]
      rolesStore.loadRoles = vi.fn().mockResolvedValue(rolesStore.roles)

      const wrapper = mount(EmployeeListView)
      await flushPromises()

      // Click "编辑" button
      const editBtn = wrapper.find('button[title="编辑档案"]')
      expect(editBtn.exists()).toBe(true)
      await editBtn.trigger('click')
      await flushPromises()

      // Verify in-place panel opens with employee data, no dialog
      expect(wrapper.findComponent(EmployeeEditPanel).exists()).toBe(true)
      expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
      expect(wrapper.text()).toContain('编辑 AI 员工 · 代码审查专家')
    })
  })

  describe('AI Teams (TeamListView & TeamEditPanel)', () => {
    it('renders team list without modal dialogs and clicking create switches to TeamEditPanel in-place', async () => {
      const groupsStore = useGroupsStore()
      groupsStore.groups = [
        {
          id: 20,
          name: '全栈攻坚特战队',
          description: '多 Agent 敏捷协同重构',
          orchestratorModel: 'gpt-4o',
          orchestratorPrompt: '中控调度指令',
          memberCount: 2,
          taskCount: 3,
          createdAt: '',
          updatedAt: '',
        },
      ]
      groupsStore.loadGroups = vi.fn().mockResolvedValue([])

      const wrapper = mount(TeamListView)
      await flushPromises()

      // 1. Verify list rendered and zero modal backdrops
      expect(wrapper.text()).toContain('AI 团队')
      expect(wrapper.text()).toContain('1 个团队')
      expect(wrapper.text()).toContain('全栈攻坚特战队')
      expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
      expect(wrapper.findComponent(TeamEditPanel).exists()).toBe(false)

      // 2. Click "+ 新建 AI 团队"
      const createBtn = wrapper.find('[data-test="btn-create-team"]')
      expect(createBtn.exists()).toBe(true)
      await createBtn.trigger('click')
      await flushPromises()

      // 3. Verify in-place TeamEditPanel is rendered, NO dialogs
      expect(wrapper.findComponent(TeamEditPanel).exists()).toBe(true)
      expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
      expect(wrapper.text()).toContain('新建 AI 团队')
      expect(wrapper.text()).toContain('01 基础信息与团队定位')
      expect(wrapper.text()).toContain('02 中控调度大脑 (Orchestrator)')
      expect(wrapper.text()).toContain('03 编制专家员工 (Staffing)')

      // 4. Click cancel button and verify returning to list
      const cancelBtn = wrapper.find('button[title="返回列表 (Esc)"]')
      expect(cancelBtn.exists()).toBe(true)
      await cancelBtn.trigger('click')
      await flushPromises()

      expect(wrapper.findComponent(TeamEditPanel).exists()).toBe(false)
      expect(wrapper.text()).toContain('AI 团队')
    })

    it('loads detail and opens edit mode without modal dialogs when editing team', async () => {
      const groupsStore = useGroupsStore()
      const sampleTeam = {
        id: 21,
        name: '微服务重构团队',
        description: '后端架构治理',
        orchestratorModel: 'gpt-4o',
        orchestratorPrompt: '中控规范',
        memberCount: 1,
        taskCount: 0,
        createdAt: '',
        updatedAt: '',
      }
      groupsStore.groups = [sampleTeam]
      groupsStore.loadGroups = vi.fn().mockResolvedValue([])
      groupsStore.loadGroup = vi.fn().mockResolvedValue({
        ...sampleTeam,
        members: [{ roleId: 10, roleName: '前端架构师', backend: 'llm', model: 'gpt-4o', color: '#3b82f6' }],
      })

      const wrapper = mount(TeamListView)
      await flushPromises()

      // Click "编辑" button
      const editBtn = wrapper.find('button[title="编辑设置与编制员工"]')
      expect(editBtn.exists()).toBe(true)
      await editBtn.trigger('click')
      await flushPromises()

      // Verify in-place panel opens with team data, no dialog
      expect(wrapper.findComponent(TeamEditPanel).exists()).toBe(true)
      expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
      expect(wrapper.text()).toContain('编辑 AI 团队 · 微服务重构团队')
    })
  })

  describe('Interface Layout Consistency Check Across All Three Modules', () => {
    it('guarantees identical header structure, action button placement, and keyboard shortcuts in edit panels', async () => {
      // 1. Mount AssistantEditPanel
      const assistantWrapper = mount(AssistantEditPanel, {
        props: { assistant: null },
      })
      await flushPromises()

      // 2. Mount EmployeeEditPanel
      const employeeWrapper = mount(EmployeeEditPanel, {
        props: { employee: null },
      })
      await flushPromises()

      // 3. Mount TeamEditPanel
      const teamWrapper = mount(TeamEditPanel, {
        props: { team: null },
      })
      await flushPromises()

      // Verify all three panels share exact same header structure
      for (const w of [assistantWrapper, employeeWrapper, teamWrapper]) {
        expect(w.find('button[title="返回列表 (Esc)"]').exists()).toBe(true)
        expect(w.find('button[title="返回列表 (Esc)"]').text()).toContain('返回列表')
        expect(w.text()).toContain('Ctrl+S')
        expect(w.text()).toContain('01')
        expect(w.text()).toContain('02')
        expect(w.text()).toContain('取消')
      }
    })
  })
})
