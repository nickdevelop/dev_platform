// Unit tests for the new task panel: only running instances can host a task,
// model input and native-session discovery appear only when the selected
// instance's backend declares them, and creating a task reports the new id.
import { beforeEach, describe, expect, it, vi } from 'vitest'
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
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
    ...overrides,
  }
}

function instance(id: string, backend: AgentInstance['backend'], status: AgentInstance['status']): AgentInstance {
  return {
    id,
    projectId: 1,
    backend,
    directory: '/workspace/alpha',
    status,
    createdAt: '2026-10-09T00:00:00Z',
    updatedAt: '2026-10-09T00:00:00Z',
  }
}

function setup(options: {
  instances: AgentInstance[]
  pi?: Partial<Capabilities>
  opencode?: Partial<Capabilities>
}) {
  setActivePinia(createPinia())
  const agents = useAgentsStore()
  agents.instances = options.instances
  agents.backends = [
    { backend: 'opencode-v2', capabilities: capabilities(options.opencode) },
    { backend: 'pi', capabilities: capabilities(options.pi) },
  ]
  vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
  vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined as never)
  const sessions = useSessionsStore()
  const createSpy = vi.spyOn(sessions, 'createSession').mockResolvedValue({ id: 'sess-new' } as never)
  const wrapper = mount(NewTaskPanel, { props: { projectId: 1 } })
  return { wrapper, createSpy }
}

describe('NewTaskPanel', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists only running instances as task hosts', async () => {
    const { wrapper } = setup({
      instances: [instance('inst-1', 'pi', 'running'), instance('inst-2', 'opencode-v2', 'stopped')],
    })
    await flushPromises()

    const values = wrapper.findAll('#instance-select option').map((option) => (option.element as HTMLOptionElement).value)
    expect(values).toEqual(['inst-1'])
  })

  it('shows the model input only when the selected backend declares modelSelection', async () => {
    const { wrapper } = setup({
      instances: [instance('inst-1', 'pi', 'running')],
      pi: { modelSelection: false },
    })
    await flushPromises()
    expect(wrapper.find('#new-session-model').exists()).toBe(false)
  })

  it('offers the model input when the selected backend declares modelSelection', async () => {
    const { wrapper } = setup({
      instances: [instance('inst-1', 'opencode-v2', 'running')],
      opencode: { modelSelection: true },
    })
    await flushPromises()
    expect(wrapper.find('#new-session-model').exists()).toBe(true)
  })

  it('offers native-session discovery only when the backend declares sessionDiscovery', async () => {
    const without = setup({ instances: [instance('inst-1', 'pi', 'running')] })
    await flushPromises()
    expect(without.wrapper.find('.discover').exists()).toBe(false)
    without.wrapper.unmount()

    const withDiscovery = setup({
      instances: [instance('inst-1', 'opencode-v2', 'running')],
      opencode: { sessionDiscovery: true },
    })
    await flushPromises()
    expect(withDiscovery.wrapper.find('.discover').exists()).toBe(true)
  })

  it('disables creation and explains the requirement when no instance is running', async () => {
    const { wrapper } = setup({ instances: [instance('inst-2', 'pi', 'stopped')] })
    await flushPromises()

    expect(wrapper.text()).toContain('请先在项目设置中启动实例')
    expect(wrapper.find('button.create').attributes('disabled')).toBeDefined()
  })

  it('creates the task on the selected instance and reports the new session id', async () => {
    const { wrapper, createSpy } = setup({
      instances: [instance('inst-1', 'opencode-v2', 'running')],
      opencode: { modelSelection: true },
    })
    await flushPromises()

    await wrapper.find('#session-title').setValue('Chat 预留')
    await wrapper.find('#new-session-model').setValue('provider/model')
    await wrapper.find('form.create-session').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(1, {
      instanceId: 'inst-1',
      title: 'Chat 预留',
      model: 'provider/model',
    })
    expect(wrapper.emitted('created')).toEqual([['sess-new']])
  })
})
