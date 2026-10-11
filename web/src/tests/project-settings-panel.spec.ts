// Unit tests for the project settings panel: project facts, per-project backend
// enablement, start gating for disabled backends, the danger-zone actions that
// hand off to the manage dialog, and the resource placeholder that must not call
// any resource endpoint.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ProjectSettingsPanel from '../components/ProjectSettingsPanel.vue'
import { useAgentsStore } from '../stores/agents'
import { useProjectsStore } from '../stores/projects'
import { useResourcesStore } from '../stores/resources'
import { useSessionsStore } from '../stores/sessions'
import type { Capabilities } from '../api/sessions'

function noCapabilities(overrides: Partial<Capabilities> = {}): Capabilities {
  return {
    streaming: false,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: false,
    interrupt: false,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
    sessionHistory: false,
    ...overrides,
  }
}

function mountPanel() {
  return mount(ProjectSettingsPanel, { props: { projectId: 1 } })
}

describe('ProjectSettingsPanel', () => {
  let fetchSpy: ReturnType<typeof vi.fn>

  beforeEach(() => {
    setActivePinia(createPinia())
    fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)

    const projects = useProjectsStore()
    projects.projects = [{ id: 1, name: 'alpha', path: '/workspace/alpha' } as never]

    const agents = useAgentsStore()
    agents.backends = [
      { backend: 'opencode-v2', capabilities: noCapabilities() },
      { backend: 'pi', capabilities: noCapabilities() },
    ]
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    vi.spyOn(agents, 'loadInstances').mockResolvedValue(undefined as never)
    vi.spyOn(agents, 'loadDiagnostics').mockResolvedValue(undefined as never)
    vi.spyOn(agents, 'loadBackendConfigs').mockResolvedValue(undefined as never)
    vi.spyOn(agents, 'setBackendEnabled').mockResolvedValue(true as never)

    const resources = useResourcesStore()
    vi.spyOn(resources, 'loadProject').mockResolvedValue(undefined as never)
    vi.spyOn(resources, 'loadAll').mockResolvedValue(undefined as never)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the project name and path', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.text()).toContain('alpha')
    expect(wrapper.text()).toContain('/workspace/alpha')
  })

  it('toggles a backend for this project through the agents store', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.find('[data-test="backend-toggle-pi"]').setValue(false)
    await flushPromises()

    expect(useAgentsStore().setBackendEnabled).toHaveBeenCalledWith(1, 'pi', false)
  })

  it('hands the danger-zone actions to the manage dialog with the project', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    const pick = (label: string) => wrapper.findAll('button').find((item) => item.text() === label)!
    await pick('重命名').trigger('click')
    await pick('移动').trigger('click')
    await pick('解除登记').trigger('click')
    await pick('删除目录').trigger('click')

    expect(wrapper.emitted('manage')?.map((entry) => entry[0])).toEqual([
      'rename',
      'move',
      'unregister',
      'delete',
    ])
    expect((wrapper.emitted('manage')![0][1] as { id: number }).id).toBe(1)
  })

  it('shows the resource management section and loads project resources', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    expect(useResourcesStore().loadProject).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('资源管理')
    expect(wrapper.text()).not.toContain('待资源管理功能接入')
  })

  it('asks for discovery from the project page without picking an instance', async () => {
    const agents = useAgentsStore()
    agents.backends = [{ backend: 'pi', capabilities: noCapabilities({ sessionDiscovery: true }) }]
    const sessions = useSessionsStore()
    const discover = vi.spyOn(sessions, 'discoverProjectSessions').mockResolvedValue([] as never)

    const wrapper = mountPanel()
    await flushPromises()

    // The instance list is gone from the project page: which instance serves
    // the backend is the platform's business, not the user's.
    expect(wrapper.find('.instance-list').exists()).toBe(false)

    const button = wrapper.find('[data-test="discover-sessions"]')
    expect(button.exists()).toBe(true)
    await button.trigger('click')
    await flushPromises()
    expect(discover).toHaveBeenCalledWith(1, 'pi')
  })

  it('offers no discovery action when the selected backend cannot discover sessions', async () => {
    const agents = useAgentsStore()
    agents.backends = [{ backend: 'pi', capabilities: noCapabilities({ sessionDiscovery: false }) }]

    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('[data-test="discover-sessions"]').exists()).toBe(false)
  })

  it('disables discovery when the selected backend is disabled for the project', async () => {
    const agents = useAgentsStore()
    agents.backends = [{ backend: 'pi', capabilities: noCapabilities({ sessionDiscovery: true }) }]
    agents.backendConfigs = [{ projectId: 1, backend: 'pi', enabled: false } as never]

    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.find('[data-test="discover-sessions"]').attributes('disabled')).toBeDefined()
  })
})
