// Unit tests for the new task panel: it selects only an agent backend (never an
// instance or a model), marks a backend disabled for the project, submits the
// {backend, title} body, and reports a failed creation without navigating.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import NewTaskPanel from '../components/NewTaskPanel.vue'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore } from '../stores/sessions'
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

function setup(options: { piEnabled?: boolean } = {}) {
  setActivePinia(createPinia())
  const agents = useAgentsStore()
  agents.backends = [
    { backend: 'opencode-v2', capabilities: capabilities(), label: 'OpenCode', order: 1, serviceSupported: true },
    { backend: 'pi', capabilities: capabilities(), label: 'Pi Agent', order: 2, serviceSupported: false },
  ]
  if (options.piEnabled === false) {
    agents.backendConfigs = [{ projectId: 1, backend: 'pi', enabled: false } as never]
  }
  vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
  vi.spyOn(agents, 'loadBackendConfigs').mockResolvedValue(undefined as never)

  const sessions = useSessionsStore()
  const createSpy = vi.spyOn(sessions, 'createSession').mockResolvedValue({ id: 'sess-new' } as never)
  const wrapper = mount(NewTaskPanel, { props: { projectId: 1 } })
  return { wrapper, createSpy, sessions }
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
    expect(wrapper.emitted('created')).toEqual([['sess-new']])
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
