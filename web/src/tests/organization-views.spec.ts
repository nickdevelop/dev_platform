import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import AssistantListView from '../features/organization/assistants/AssistantListView.vue'
import AssistantEditDialog from '../features/organization/assistants/AssistantEditDialog.vue'
import EmployeeListView from '../features/organization/employees/EmployeeListView.vue'
import EmployeeEditDialog from '../features/organization/employees/EmployeeEditDialog.vue'
import TeamListView from '../features/organization/teams/TeamListView.vue'
import TeamEditDialog from '../features/organization/teams/TeamEditDialog.vue'
import OrgPromptsView from '../features/organization/prompts/OrgPromptsView.vue'
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

  describe('AI Assistants (AssistantListView & AssistantEditDialog)', () => {
    const sampleAssistant = {
      id: 1,
      name: '默认开发助手',
      icon: '🤖', // taste-skill-ignore
      defaultModelId: 101,
      promptId: 1,
      systemPrompt: '',
      enablePlatformTools: true,
      maxToolSteps: 10,
      showInFloat: true,
      createdAt: '',
      updatedAt: '',
    }

    it('renders the assistant grid and opens the editor as a dialog', async () => {
      const chatsStore = useChatsStore()
      chatsStore.groups = [sampleAssistant]
      chatsStore.loadGroups = vi.fn().mockResolvedValue([])

      const wrapper = mount(AssistantListView)
      await flushPromises()

      expect(wrapper.text()).toContain('AI 助手')
      // Header keeps the title; the tally chip is gone and the toolbar carries
      // the create action plus the filter / search form.
      expect(wrapper.text()).not.toContain('1 个助手')
      expect(wrapper.find('[data-test="btn-create-assistant"]').exists()).toBe(true)
      expect(wrapper.find('[data-test="select-float-filter"]').exists()).toBe(true)
      expect(wrapper.find('[data-test="btn-search-assistant"]').exists()).toBe(true)
      expect(wrapper.find('[data-test="btn-reset-assistant"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('默认开发助手')
      expect(wrapper.findComponent(AssistantEditDialog).exists()).toBe(false)

      await wrapper.find('[data-test="btn-create-assistant"]').trigger('click')
      await flushPromises()

      expect(wrapper.findComponent(AssistantEditDialog).exists()).toBe(true)
      expect(wrapper.text()).toContain('新建 AI 助手')
      expect(wrapper.text()).toContain('01')
      expect(wrapper.text()).toContain('02')
      expect(wrapper.text()).toContain('运行模型与推理配置')
      expect(wrapper.text()).toContain('人设规范与系统提示词')
      expect(wrapper.text()).toContain('交互形态与自治能力')

      wrapper.findComponent(AssistantEditDialog).vm.$emit('cancel')
      await flushPromises()
      expect(wrapper.findComponent(AssistantEditDialog).exists()).toBe(false)
    })

    it('opens the edit dialog prefilled when clicking edit on an assistant row', async () => {
      const chatsStore = useChatsStore()
      chatsStore.groups = [{ ...sampleAssistant, id: 2, name: '架构顾问', icon: '💡' }] // taste-skill-ignore
      chatsStore.loadGroups = vi.fn().mockResolvedValue([])

      const wrapper = mount(AssistantListView)
      await flushPromises()

      const editBtn = wrapper.find('[data-test="btn-edit-assistant-2"]')
      expect(editBtn.exists()).toBe(true)
      await editBtn.trigger('click')
      await flushPromises()

      expect(wrapper.findComponent(AssistantEditDialog).exists()).toBe(true)
      expect(wrapper.text()).toContain('编辑 AI 助手 · 架构顾问')
    })
  })

  describe('AI Employees (EmployeeListView & EmployeeEditDialog)', () => {
    const sampleEmployee = {
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
    }

    it('renders the employee roster and opens the create dialog', async () => {
      const rolesStore = useRolesStore()
      rolesStore.roles = [sampleEmployee]
      rolesStore.loadRoles = vi.fn().mockResolvedValue(rolesStore.roles)

      const wrapper = mount(EmployeeListView)
      await flushPromises()

      expect(wrapper.text()).toContain('AI 员工')
      expect(wrapper.find('[data-test="select-backend-filter"]').exists()).toBe(true)
      expect(wrapper.find('[data-test="btn-create-employee"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('前端架构师')
      expect(wrapper.findComponent(EmployeeEditDialog).exists()).toBe(false)

      await wrapper.find('[data-test="btn-create-employee"]').trigger('click')
      await flushPromises()

      expect(wrapper.findComponent(EmployeeEditDialog).exists()).toBe(true)
      expect(wrapper.text()).toContain('新建 AI 员工')
      expect(wrapper.text()).toContain('员工档案与岗位')
      expect(wrapper.text()).toContain('执行引擎与运行模型')
      expect(wrapper.text()).toContain('人设与提示词')

      wrapper.findComponent(EmployeeEditDialog).vm.$emit('cancel')
      await flushPromises()
      expect(wrapper.findComponent(EmployeeEditDialog).exists()).toBe(false)
    })

    it('opens the edit dialog prefilled from the row action', async () => {
      const rolesStore = useRolesStore()
      rolesStore.roles = [{ ...sampleEmployee, id: 11, name: '代码审查专家', backend: 'opencode_v2' }]
      rolesStore.loadRoles = vi.fn().mockResolvedValue(rolesStore.roles)

      const wrapper = mount(EmployeeListView)
      await flushPromises()

      const editBtn = wrapper.find('[data-test="btn-edit-employee-11"]')
      expect(editBtn.exists()).toBe(true)
      await editBtn.trigger('click')
      await flushPromises()

      expect(wrapper.findComponent(EmployeeEditDialog).exists()).toBe(true)
      expect(wrapper.text()).toContain('编辑 AI 员工 · 代码审查专家')
    })

    it('takes the keyword on search only and resets both controls', async () => {
      const rolesStore = useRolesStore()
      rolesStore.roles = [
        sampleEmployee,
        { ...sampleEmployee, id: 12, name: '代码审查专家', backend: 'opencode_v2' },
      ]
      rolesStore.loadRoles = vi.fn().mockResolvedValue(rolesStore.roles)

      const wrapper = mount(EmployeeListView)
      await flushPromises()

      await wrapper.find('[data-test="input-list-keyword"]').setValue('代码')
      await flushPromises()
      // The typed keyword is a draft: the table has not followed it yet.
      expect(wrapper.text()).toContain('前端架构师')

      // The search button submits the form, exactly like Enter in the field.
      await wrapper.find('[data-test="btn-search-employee"]').trigger('submit')
      await flushPromises()
      expect(wrapper.text()).not.toContain('前端架构师')
      expect(wrapper.text()).toContain('代码审查专家')

      await wrapper.find('[data-test="btn-reset-employee"]').trigger('click')
      await flushPromises()
      expect(wrapper.text()).toContain('前端架构师')
      expect((wrapper.find('[data-test="input-list-keyword"]').element as HTMLInputElement).value).toBe('')
    })
  })

  describe('AI Teams (TeamListView & TeamEditDialog)', () => {
    const sampleTeam = {
      id: 20,
      name: '全栈攻坚特战队',
      description: '多 Agent 敏捷协同重构',
      orchestratorModel: 'gpt-4o',
      orchestratorPrompt: '中控调度指令',
      memberCount: 2,
      taskCount: 3,
      createdAt: '',
      updatedAt: '',
    }

    it('renders the team roster and opens the create dialog', async () => {
      const groupsStore = useGroupsStore()
      groupsStore.groups = [sampleTeam]
      groupsStore.loadGroups = vi.fn().mockResolvedValue([])

      const wrapper = mount(TeamListView)
      await flushPromises()

      expect(wrapper.text()).toContain('AI 团队')
      expect(wrapper.find('[data-test="btn-create-team"]').exists()).toBe(true)
      expect(wrapper.find('[data-test="btn-search-team"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('全栈攻坚特战队')
      expect(wrapper.findComponent(TeamEditDialog).exists()).toBe(false)

      await wrapper.find('[data-test="btn-create-team"]').trigger('click')
      await flushPromises()

      expect(wrapper.findComponent(TeamEditDialog).exists()).toBe(true)
      expect(wrapper.text()).toContain('新建 AI 团队')
      expect(wrapper.text()).toContain('基础信息与团队定位')
      expect(wrapper.text()).toContain('中控调度大脑')
      expect(wrapper.text()).toContain('编制专家员工')

      wrapper.findComponent(TeamEditDialog).vm.$emit('cancel')
      await flushPromises()
      expect(wrapper.findComponent(TeamEditDialog).exists()).toBe(false)
    })

    it('loads the team detail and opens the edit dialog from the row action', async () => {
      const groupsStore = useGroupsStore()
      groupsStore.groups = [sampleTeam]
      groupsStore.loadGroups = vi.fn().mockResolvedValue([])
      groupsStore.loadGroup = vi.fn().mockResolvedValue({ ...sampleTeam, members: [{ roleId: 1 }] })

      const wrapper = mount(TeamListView)
      await flushPromises()

      const editBtn = wrapper.find('[data-test="btn-edit-team-20"]')
      expect(editBtn.exists()).toBe(true)
      await editBtn.trigger('click')
      await flushPromises()

      expect(groupsStore.loadGroup).toHaveBeenCalledWith(20)
      expect(wrapper.findComponent(TeamEditDialog).exists()).toBe(true)
      expect(wrapper.text()).toContain('编辑 AI 团队 · 全栈攻坚特战队')
    })
  })

  describe('Interface Layout Consistency Check Across All Three Modules', () => {
    it('keeps the same numbered sections and save strip in every organization editor', async () => {
      const editors = [
        mount(AssistantEditDialog, { props: { assistant: null } }),
        mount(EmployeeEditDialog, { props: { employee: null } }),
        mount(TeamEditDialog, { props: { team: null } }),
      ]
      await flushPromises()

      // The assistant editor already lives in a dialog; the employee and team
      // editors are still workbench panels. What all three must keep in common is
      // the numbered section rhythm and the cancel / save strip with its shortcut
      // hint — that is what makes one editor feel like the next.
      for (const w of editors) {
        expect(w.text()).toContain('01')
        expect(w.text()).toContain('02')
        expect(w.text()).toContain('取消')
        expect(w.text()).toContain('Ctrl+S')
      }
    })
  })

  describe('OrgPromptsView in Organization Center', () => {
    it('renders the AI 提示词 view with its toolbar and rows', async () => {
      const promptsStore = usePromptsStore()
      promptsStore.prompts = [
        { id: 1, name: '角色提示词', type: 'role', description: 'desc', content: 'content', createdAt: '', updatedAt: '' },
        { id: 2, name: '流程提示词', type: 'flow', description: 'desc2', content: 'content2', createdAt: '', updatedAt: '' },
      ]
      const wrapper = mount(OrgPromptsView)
      await flushPromises()

      expect(wrapper.find('h1').text()).toBe('AI 提示词')
      // The count chip is gone from the header; the type filter carries the tally.
      expect(wrapper.text()).not.toContain('2 个提示词')
      expect(wrapper.find('[data-test="btn-create-prompt"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('角色提示词')
      expect(wrapper.text()).toContain('流程提示词')
    })
  })
})
