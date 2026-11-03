import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import SettingsRolesView from '../features/settings/SettingsRolesView.vue'
import { useRolesStore } from '../stores/roles'
import { usePromptsStore } from '../stores/prompts'
import { useModelsStore } from '../stores/models'
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

    // 2. Verify backend and model dropdown integration
    expect(wrapper.text()).toContain('执行后端')
    expect(wrapper.text()).toContain('模型名称')
    // By default formBackend is 'llm', model dropdown is populated with gpt-4o
    expect(wrapper.text()).toContain('GPT-4o (gpt-4o)')
  })

  it('automatically manages model dropdown candidates upon backend selection', async () => {
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
        if (path === '/api/model-services') {
          return {
            status: 200,
            json: async () => ({
              services: [
                {
                  id: 1,
                  name: 'OpenAI 官方服务',
                  enabled: true,
                  endpoint: '',
                  models: [
                    {
                      id: 11,
                      modelId: 'gpt-4o',
                      name: 'GPT-4o',
                      usable: true,
                      supportsText: true,
                      supportsImage: true,
                      enabled: true,
                    },
                  ],
                },
              ],
            }),
          } as Response
        }
        if (path === '/api/agent-backends/opencode/models') {
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

    // Switch backend to opencode
    const backendSelect = wrapper.find('select')
    await backendSelect.setValue('opencode')
    await flushPromises()

    // Now opencode models are fetched and auto-managed in dropdown
    expect(wrapper.text()).toContain('Claude 3.5 Sonnet')
  })
})
