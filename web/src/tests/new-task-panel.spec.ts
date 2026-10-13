// Unit tests for the new task panel: it selects only an agent backend (never an
// instance or a model), marks a backend disabled for the project, submits the
// {backend, title} body, and reports a failed creation without navigating.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import NewTaskPanel from '../components/NewTaskPanel.vue'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore } from '../stores/sessions'
import type { AgentInstance } from '../api/agents'
import type { Capabilities } from '../api/sessions'

function capabilities(overrides: Partial<Capabilities> = {}): Capabilities {
  return {
    streaming: true,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: true,
    interrupt: false,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
    sessionHistory: false,
    rawEvents: false,
    ...overrides,
  }
}

function setup(options: {
  piEnabled?: boolean
  instances?: AgentInstance[]
  modelSelection?: boolean
} = {}) {
  setActivePinia(createPinia())
  const agents = useAgentsStore()
  agents.backends = [
    {
      backend: 'opencode-v2',
      capabilities: capabilities({ modelSelection: options.modelSelection ?? false }),
      label: 'OpenCode',
      order: 1,
      serviceSupported: true,
    },
    { backend: 'pi', capabilities: capabilities(), label: 'Pi Agent', order: 2, serviceSupported: false },
  ]
  if (options.piEnabled === false) {
    agents.backendConfigs = [{ projectId: 1, backend: 'pi', enabled: false } as never]
  }
  if (options.instances !== undefined) {
    agents.instances = options.instances
  }
  vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
  vi.spyOn(agents, 'loadBackendConfigs').mockResolvedValue(undefined as never)
  vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined as never)

  const sessions = useSessionsStore()
  const createSpy = vi.spyOn(sessions, 'createSession').mockResolvedValue({ id: 'sess-new' } as never)
  const wrapper = mount(NewTaskPanel, { props: { projectId: 1 } })
  return { wrapper, createSpy, sessions, agents }
}

describe('NewTaskPanel', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders only an agent select and an optional title', async () => {
    const { wrapper } = setup()
    await flushPromises()

    expect(wrapper.find('#backend-select').exists()).toBe(true)
    expect(wrapper.find('#session-title').exists()).toBe(true)
    expect(wrapper.find('#instance-select').exists()).toBe(false)
    expect(wrapper.find('#new-session-model').exists()).toBe(false)
    expect(wrapper.find('[data-test="discover-sessions"]').exists()).toBe(false)
  })

  it('submits { backend, title } and never an instanceId or model', async () => {
    const { wrapper, createSpy } = setup()
    await flushPromises()

    await wrapper.find('#backend-select').setValue('pi')
    await wrapper.find('#session-title').setValue('Chat 预留')
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(1, { backend: 'pi', title: 'Chat 预留' })
    const body = createSpy.mock.calls[0][1]
    expect(body).not.toHaveProperty('instanceId')
    expect(body).not.toHaveProperty('model')
    expect(body).not.toHaveProperty('message')
    expect(wrapper.emitted('created')).toEqual([['sess-new']])
  })

  it('labels the submit as 开始对话 when a first message is present', async () => {
    const { wrapper } = setup()
    await flushPromises()

    expect(wrapper.find('button.create').text()).toBe('新建任务')
    await wrapper.find('#first-message').setValue('帮我检查构建')
    expect(wrapper.find('button.create').text()).toBe('创建并开始对话')
  })

  it('sends the first message and model with the create request', async () => {
    // A running instance of the selected backend is what the model picker
    // loads from, mirroring the instance the create request will reuse.
    const { wrapper, createSpy } = setup({
      modelSelection: true,
      instances: [
        {
          id: 'inst-1',
          projectId: 1,
          backend: 'opencode-v2',
          directory: '/tmp/proj',
          status: 'running',
          createdAt: '',
          updatedAt: '',
        },
      ],
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        ({
          status: 200,
          json: async () => ({
            models: [{ id: 'p/m', provider: 'p', label: 'Model M' }],
          }),
        }) as Response,
      ),
    )
    await flushPromises()

    await wrapper.find('#backend-select').setValue('opencode-v2')
    await flushPromises()
    await wrapper.find('#first-message').setValue('帮我检查构建失败')
    await flushPromises()
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const body = createSpy.mock.calls[0][1]
    expect(body.backend).toBe('opencode-v2')
    expect(body.message).toBe('帮我检查构建失败')
    expect(body.model).toBe('p/m')
    expect(body).not.toHaveProperty('instanceId')
    expect(wrapper.emitted('created')).toEqual([['sess-new']])
  })

  it('blocks a first message when no running instance can serve the model list', async () => {
    const { wrapper, createSpy } = setup({ modelSelection: true })
    await flushPromises()

    await wrapper.find('#backend-select').setValue('opencode-v2')
    await flushPromises()
    expect(wrapper.find('[data-test="model-unavailable"]').exists()).toBe(true)

    await wrapper.find('#first-message').setValue('帮我检查构建失败')
    await flushPromises()
    expect(wrapper.find('button.create').attributes('disabled')).toBeDefined()

    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()
    expect(createSpy).not.toHaveBeenCalled()

    // Without a message the plain create stays available.
    await wrapper.find('#first-message').setValue('')
    await flushPromises()
    expect(wrapper.find('button.create').attributes('disabled')).toBeUndefined()
  })

  it('disables a backend disabled for the project and labels it', async () => {
    const { wrapper } = setup({ piEnabled: false })
    await flushPromises()

    const piOption = wrapper
      .findAll('#backend-select option')
      .find((option) => option.text().includes('pi'))!
    expect(piOption.attributes('disabled')).toBeDefined()
    expect(piOption.text()).toContain('已在项目中禁用')

    await wrapper.find('#backend-select').setValue('pi')
    await flushPromises()
    expect(wrapper.find('button.create').attributes('disabled')).toBeDefined()
  })

  it('shows an error and does not report success when creation fails', async () => {
    setActivePinia(createPinia())
    const agents = useAgentsStore()
    agents.backends = [{ backend: 'pi', capabilities: capabilities(), label: 'Pi Agent', order: 2, serviceSupported: false }]
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    vi.spyOn(agents, 'loadBackendConfigs').mockResolvedValue(undefined as never)
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          ({
            status: 502,
            json: async () => ({ error: 'instance_start_failed' }),
          }) as Response,
      ),
    )
    const sessions = useSessionsStore()
    const wrapper = mount(NewTaskPanel, { props: { projectId: 1 } })
    await flushPromises()

    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.emitted('created')).toBeUndefined()
    expect(sessions.sessions).toHaveLength(0)
  })
})
