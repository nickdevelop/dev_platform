import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import SettingsRolesView from '../features/settings/SettingsRolesView.vue'
import { useRolesStore } from '../stores/roles'
import { usePromptsStore } from '../stores/prompts'
import { useModelsStore } from '../stores/models'
import { useAgentsStore } from '../stores/agents'
import type { AgentRole } from '../api/roles'
import { installTestCSRF } from './csrf-fixture'

const routerPush = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush }),
}))

describe('SettingsRolesView Unit Tests', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    installTestCSRF('csrf-token')
    routerPush.mockClear()
    vi.restoreAllMocks()
  })

  it('separates label and sublabel for long prompt descriptions without overflow', async () => {
    const promptsStore = usePromptsStore()
    const longDesc = '这是一个非常非常长的提示词说明文字，包含了大量的专业设定和多角度的职责约束描述，如果直接拼接到单行标题中会导致下拉框无限被拉长并产生横向滚动条'
    promptsStore.prompts = [
      {
        id: 1,
        name: '全栈架构专家',
        type: 'role',
        description: longDesc,
        content: '你是一个架构专家',
        createdAt: '',
        updatedAt: '',
      },
    ]

    const modelsStore = useModelsStore()
    modelsStore.services = [
      {
        id: 1,
        name: 'OpenAI 服务',
        enabled: true,
        baseUrl: 'https://api.openai.com/v1',
        hasKey: true,
        secretRef: 'sec-1',
        models: [
          {
            id: 11,
            serviceId: 1,
            modelId: 'gpt-4o',
            name: 'GPT-4o',
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

    const wrapper = mount(SettingsRolesView, {
      global: {
        stubs: {
          Teleport: true,
        },
      },
    })
    await flushPromises()

    // Open create dialog
    const createBtn = wrapper.find('[data-test="btn-create-role"]')
    expect(createBtn.exists()).toBe(true)
    await createBtn.trigger('click')
    await flushPromises()

    // 1. Verify prompt selector container has min-w-0 flex-1
    const promptMenuContainer = wrapper.find('.flex-1.min-w-0')
    expect(promptMenuContainer.exists()).toBe(true)

    // 2. Verify backend capsules and model dropdown integration
    expect(wrapper.text()).toContain('Agent 后端')
    expect(wrapper.text()).toContain('运行模型')
    expect(wrapper.find('.backend-switch').exists()).toBe(true)
    // By default formBackend is 'llm', model dropdown is populated with gpt-4o
    expect(wrapper.text()).toContain('GPT-4o (gpt-4o)')
  })

  it('automatically manages model dropdown candidates upon backend selection', async () => {
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
            id: 11,
            serviceId: 1,
            modelId: 'gpt-4o',
            name: 'GPT-4o',
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

    // Stub agent backend models api and models services api
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const path = new URL(String(input), 'http://localhost').pathname
        if (path === '/api/agent-backends') {
          return {
            status: 200,
            json: async () => ({
              backends: [
                { backend: 'opencode_v2', label: 'OpenCode V2', enabled: true },
                { backend: 'pi', label: 'Pi Agent', enabled: true },
              ],
            }),
          } as Response
        }
        if (path === '/api/model-services') {
          return {
            status: 200,
            json: async () => ({
              services: [
                {
                  id: 1,
                  name: 'OpenAI 官方服务',
                  enabled: true,
                  baseUrl: 'https://api.openai.com/v1',
                  hasKey: true,
                  secretRef: 'sec-1',
                  models: [
                    {
                      id: 11,
                      serviceId: 1,
                      modelId: 'gpt-4o',
                      name: 'GPT-4o',
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
              ],
            }),
          } as Response
        }
        if (path === '/api/agent-backends/opencode_v2/models' || path === '/api/agent-backends/opencode/models') {
          return {
            status: 200,
            json: async () => ({
              models: [
                { id: 'claude-3-5-sonnet', name: 'Claude 3.5 Sonnet', enabled: true },
                { id: 'gpt-4o-mini', name: 'GPT-4o Mini', enabled: true },
              ],
            }),
          } as Response
        }
        return { status: 200, json: async () => ({}) } as Response
      }),
    )

    const wrapper = mount(SettingsRolesView, {
      global: {
        stubs: {
          Teleport: true,
        },
      },
    })
    await flushPromises()

    // Open create dialog
    await wrapper.find('[data-test="btn-create-role"]').trigger('click')
    await flushPromises()

    // Initially backend is 'llm'
    expect(wrapper.text()).toContain('GPT-4o (gpt-4o)')

    // Verify capsule radio buttons exist
    const backendButtons = wrapper.findAll('.backend-option')
    expect(backendButtons.length).toBeGreaterThanOrEqual(2)

    // Switch backend to OpenCode V2 capsule
    const opencodeBtn = backendButtons.find((btn) => btn.text().includes('OpenCode'))
    expect(opencodeBtn?.exists()).toBe(true)
    await opencodeBtn!.trigger('click')
    await flushPromises()

    // Now opencode ModelSelect is active and loads its models
    expect(wrapper.find('.model-select-wrapper').exists()).toBe(true)
  })

  it('deletes a role only after confirming in the shared dialog', async () => {
    const rolesStore = useRolesStore()
    const role: AgentRole = {
      id: 1,
      name: '架构师',
      description: '负责系统架构',
      backend: 'llm',
      model: 'gpt-4o',
      promptId: 0,
      systemPrompt: '',
      color: '#3b82f6',
      createdAt: '',
      updatedAt: '',
    }

    const wrapper = mount(SettingsRolesView, {
      global: { stubs: { Teleport: true } },
    })
    await flushPromises()
    rolesStore.roles = [role]
    await flushPromises()

    const deleteSpy = vi.spyOn(rolesStore, 'deleteRole').mockResolvedValue(undefined as never)
    await wrapper.find('button[title="删除角色"]').trigger('click')
    expect(wrapper.text()).toContain('确定要删除角色「架构师」吗？')
    expect(deleteSpy).not.toHaveBeenCalled()

    await wrapper.find('[data-test="confirm-accept-btn"]').trigger('click')
    await flushPromises()
    expect(deleteSpy).toHaveBeenCalledWith(1)
  })
})
