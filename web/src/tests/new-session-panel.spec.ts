// Unit tests for the new session panel: it chooses an agent backend through a
// segmented switch (never a select, instance, or model field), submits the
// {backend, message?, model?} body with no title, and reports a failed
// creation without navigating.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import NewSessionPanel from '../components/NewSessionPanel.vue'
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
  const wrapper = mount(NewSessionPanel, { props: { projectId: 1 } })
  return { wrapper, createSpy, sessions, agents }
}

function backendButton(wrapper: ReturnType<typeof mount>, backend: string) {
  return wrapper.find(`.backend-switch [data-backend="${backend}"]`)
}

describe('NewSessionPanel', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders a backend switch and a message composer, without a select or title', async () => {
    const { wrapper } = setup()
    await flushPromises()

    expect(wrapper.find('.backend-switch[role="radiogroup"]').exists()).toBe(true)
    expect(wrapper.find('.backend-switch [data-backend="opencode-v2"]').exists()).toBe(true)
    expect(wrapper.find('.backend-switch [data-backend="pi"]').exists()).toBe(true)
    expect(wrapper.find('#first-message').exists()).toBe(true)
    expect(wrapper.find('select').exists()).toBe(false)
    expect(wrapper.find('#session-title').exists()).toBe(false)
    expect(wrapper.find('#backend-select').exists()).toBe(false)
    expect(wrapper.find('#instance-select').exists()).toBe(false)
    expect(wrapper.find('#new-session-model').exists()).toBe(false)
    expect(wrapper.find('[data-test="discover-sessions"]').exists()).toBe(false)
  })

  it('submits { backend, message } and never a title, instanceId', async () => {
    const { wrapper, createSpy } = setup()
    await flushPromises()

    await backendButton(wrapper, 'pi').trigger('click')
    await wrapper.find('#first-message').setValue('帮我检查构建')
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(1, { backend: 'pi', message: '帮我检查构建' })
    const body = createSpy.mock.calls[0][1]
    expect(body).not.toHaveProperty('title')
    expect(body).not.toHaveProperty('instanceId')
    expect(body).not.toHaveProperty('model')
    expect(wrapper.emitted('created')).toEqual([['sess-new']])
  })

  it('creates an empty session without a message field', async () => {
    const { wrapper, createSpy } = setup()
    await flushPromises()

    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const body = createSpy.mock.calls[0][1]
    expect(body).not.toHaveProperty('message')
    expect(body).not.toHaveProperty('title')
  })

  it('labels the submit as 开始对话 when a first message is present', async () => {
    const { wrapper } = setup()
    await flushPromises()

    expect(wrapper.find('button.create').text()).toBe('新建会话')
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

    await backendButton(wrapper, 'opencode-v2').trigger('click')
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
    expect(body).not.toHaveProperty('title')
    expect(body).not.toHaveProperty('instanceId')
    expect(wrapper.emitted('created')).toEqual([['sess-new']])
  })

  it('sends the first message on the default model when no instance serves the list', async () => {
    const { wrapper, createSpy } = setup({ modelSelection: true })
    await flushPromises()

    await backendButton(wrapper, 'opencode-v2').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="model-unavailable"]').exists()).toBe(true)

    await wrapper.find('#first-message').setValue('帮我检查构建失败')
    await flushPromises()
    expect(wrapper.find('button.create').attributes('disabled')).toBeUndefined()

    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()
    expect(createSpy).toHaveBeenCalledTimes(1)
    const body = createSpy.mock.calls[0][1]
    expect(body.message).toBe('帮我检查构建失败')
    expect(body).not.toHaveProperty('model')
  })

  it('disables a backend disabled for the project and labels it', async () => {
    const { wrapper } = setup({ piEnabled: false })
    await flushPromises()

    const piButton = backendButton(wrapper, 'pi')
    expect(piButton.attributes('disabled')).toBeDefined()
    expect(piButton.text()).toContain('已在项目中禁用')

    await backendButton(wrapper, 'opencode-v2').trigger('click')
    await flushPromises()
    expect(wrapper.find('button.create').attributes('disabled')).toBeUndefined()
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
    const wrapper = mount(NewSessionPanel, { props: { projectId: 1 } })
    await flushPromises()

    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.emitted('created')).toBeUndefined()
    expect(sessions.sessions).toHaveLength(0)
  })
})
