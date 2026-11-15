// Every place an agent backend is named shows its logo next to the name. The
// logo is the shared BackendIcon, which marks itself with data-test="agent-logo-<id>".
import { beforeEach, describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import BackendTag from '../shared/components/BackendTag.vue'
import AgentSwitch from '../features/chat/composer/AgentSwitch.vue'
import FlowRoleBadge from '../features/tasks/flow/FlowRoleBadge.vue'
import TaskManageDialog from '../features/tasks/TaskManageDialog.vue'
import { useAgentsStore } from '../stores/agents'
import { useProjectsStore } from '../stores/projects'
import type { AgentBackend, Capabilities } from '../api/agents'

function capabilities(): Capabilities {
  return {
    streaming: true,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: false,
    interrupt: false,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
    sessionHistory: false,
    rawEvents: false,
  } as Capabilities
}

function backend(id: string, label: string, order: number): AgentBackend {
  return {
    backend: id,
    capabilities: capabilities(),
    label,
    order,
    enabled: true,
    serviceSupported: false,
  } as AgentBackend
}

describe('agent backend logos', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const agents = useAgentsStore()
    agents.backends = [backend('pi', 'Pi Agent', 1), backend('opencode-v2', 'OpenCode', 2)]
  })

  it('shows the logo before the backend name in tags', () => {
    const wrapper = mount(BackendTag, { props: { backend: 'pi' } })

    expect(wrapper.find('[data-test="agent-logo-pi"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Pi Agent')
  })

  it('shows a logo on every option of the new-session agent switch', () => {
    const wrapper = mount(AgentSwitch, {
      props: {
        modelValue: 'pi',
        backends: useAgentsStore().backends,
      },
    })

    expect(wrapper.find('[data-test="agent-switch-pi"] [data-test="agent-logo-pi"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="agent-switch-opencode-v2"] [data-test="agent-logo-opencode-v2"]').exists()).toBe(true)
  })

  it('shows the logo of a flow role backend', () => {
    const wrapper = mount(FlowRoleBadge, { props: { name: '评审', backend: 'pi' } })

    expect(wrapper.find('[data-test="agent-logo-pi"]').exists()).toBe(true)
  })

  it('shows the logo in the task dialog agent choice', async () => {
    useProjectsStore().projects = []
    const wrapper = mount(TaskManageDialog, {
      props: { task: null },
      global: { stubs: { UiFormDialog: { template: '<div><slot /></div>' } } },
    })
    await flushPromises()

    expect(wrapper.find('#dlg-task-backend').exists()).toBe(true)
    expect(wrapper.find('[data-test="agent-logo-pi"]').exists()).toBe(true)
  })
})
