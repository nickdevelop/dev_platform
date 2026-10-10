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
import type { Capabilities } from '../api/sessions'

function noCapabilities(): Capabilities {
  return {
    streaming: false,
    sessionDiscovery: false,
    sessionContinue: false,
    stop: false,
    modelSelection: false,
    toolCallEvents: false,
    toolConfirmation: false,
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

  it('disables starting an instance when the selected backend is disabled', async () => {
    const agents = useAgentsStore()
    agents.backendConfigs = [{ projectId: 1, backend: 'opencode-v2', enabled: false } as never]
    const wrapper = mountPanel()
    await flushPromises()

    const start = wrapper.find('button.start')
    expect(start.attributes('disabled')).toBeDefined()
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

  it('shows the resource placeholder without calling any resource endpoint', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.text()).toContain('待资源管理功能接入')
    expect(fetchSpy).not.toHaveBeenCalled()
  })
})
