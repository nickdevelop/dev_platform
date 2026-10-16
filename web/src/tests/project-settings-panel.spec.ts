// Unit tests for the project settings panel: project facts, per-project backend
// enablement, start gating for disabled backends, the danger-zone actions that
// hand off to the manage dialog.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import ProjectSettingsPanel from '../features/projects/ProjectSettingsPanel.vue'
import { useAgentsStore } from '../stores/agents'
import { useProjectsStore } from '../stores/projects'
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
    rawEvents: false,
    ...overrides,
  }
}

// The panel links to the settings page that can start the service; the test
// harness has no router, so the link is rendered as a plain anchor.
const RouterLinkStub = defineComponent({
  name: 'RouterLinkStub',
  props: { to: { type: Object, required: true } },
  setup(props, { slots }) {
    const name = String((props.to as { name?: string }).name ?? '')
    return () => h('a', { href: `/${name}` }, slots.default?.())
  },
})

function mountPanel() {
  return mount(ProjectSettingsPanel, {
    props: { projectId: 1 },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
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
      { backend: 'opencode-v2', capabilities: noCapabilities(), label: 'OpenCode', order: 1, serviceSupported: true },
      { backend: 'pi', capabilities: noCapabilities(), label: 'Pi Agent', order: 2, serviceSupported: false },
    ]
    vi.spyOn(agents, 'loadBackends').mockResolvedValue(undefined as never)
    // The panel no longer asks about instances; a service look happens once.
    vi.spyOn(agents, 'detectService').mockResolvedValue(null as never)
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

  it('hands the danger-zone actions to the manage dialog with the project', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    const pick = (label: string) => wrapper.findAll('button').find((item) => item.text() === label)!
    await pick('重命名').trigger('click')
    await pick('移动').trigger('click')
    await pick('解除登记').trigger('click')
    expect(wrapper.findAll('button').some((item) => item.text() === '删除目录')).toBe(false)

    expect(wrapper.emitted('manage')?.map((entry) => entry[0])).toEqual([
      'rename',
      'move',
      'unregister',
    ])
    expect((wrapper.emitted('manage')![0][1] as { id: number }).id).toBe(1)
  })

  // The whole "instance" concept is background plumbing: a user reading this
  // panel must never meet the word, nor a control that pretends to operate one.
  it('shows nothing about instances, and looks at the service exactly once', async () => {
    const agents = useAgentsStore()
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.text()).not.toContain('实例')
    expect(wrapper.find('[data-test="instance-list"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="stop-instance"]').exists()).toBe(false)
    expect(agents.detectService).toHaveBeenCalledTimes(1)
    expect(agents.detectService).toHaveBeenCalledWith('opencode-v2')
    // A backend with no machine-level service is never even asked.
    expect(agents.detectService).not.toHaveBeenCalledWith('pi')
  })

  it('points at the one page that can start a stopped service', async () => {
    const agents = useAgentsStore()
    agents.services = {
      'opencode-v2': {
        backend: 'opencode-v2',
        supported: true,
        state: 'stopped',
        checkedAt: '2026-10-13T00:00:00Z',
      },
    }
    const wrapper = mountPanel()
    await flushPromises()

    const notice = wrapper.find('[data-test="project-service-down"]')
    expect(notice.exists()).toBe(true)
    expect(notice.text()).toContain('平台设置 → Agent')
    expect(notice.find('a').attributes('href')).toBe('/settings-agents')
  })

  it('stays quiet about the service while it is running', async () => {
    const agents = useAgentsStore()
    agents.services = {
      'opencode-v2': {
        backend: 'opencode-v2',
        supported: true,
        state: 'running',
        checkedAt: '2026-10-13T00:00:00Z',
      },
    }
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.find('[data-test="project-service-down"]').exists()).toBe(false)
  })

  it('asks for discovery from the project page without picking an instance', async () => {
    const agents = useAgentsStore()
    agents.backends = [{ backend: 'pi', capabilities: noCapabilities({ sessionDiscovery: true }), label: 'Pi Agent', order: 2, serviceSupported: false }]
    const sessions = useSessionsStore()
    const discover = vi.spyOn(sessions, 'discoverProjectSessions').mockResolvedValue([] as never)

    const wrapper = mountPanel()
    await flushPromises()

    const button = wrapper.find('[data-test="discover-sessions"]')
    expect(button.exists()).toBe(true)
    await button.trigger('click')
    await flushPromises()
    expect(discover).toHaveBeenCalledWith(1, 'pi')
  })

  it('offers no discovery action when the selected backend cannot discover sessions', async () => {
    const agents = useAgentsStore()
    agents.backends = [{ backend: 'pi', capabilities: noCapabilities({ sessionDiscovery: false }), label: 'Pi Agent', order: 2, serviceSupported: false }]

    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('[data-test="discover-sessions"]').exists()).toBe(false)
  })

  it('disables discovery when the selected backend is disabled for the project', async () => {
    const agents = useAgentsStore()
    agents.backends = [{ backend: 'pi', capabilities: noCapabilities({ sessionDiscovery: true }), label: 'Pi Agent', order: 2, serviceSupported: false }]
    agents.backendConfigs = [{ projectId: 1, backend: 'pi', enabled: false } as never]

    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.find('[data-test="discover-sessions"]').attributes('disabled')).toBeDefined()
  })
})
