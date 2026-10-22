// Tests for the catalog-driven backend facts: labels, presentation order and
// service support come from the backend list, and an unknown backend never
// borrows another backend's name or tone.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import BackendTag from '../shared/components/BackendTag.vue'
import { useAgentsStore } from '../stores/agents'
import { backendFixtures } from './backend-fixtures'

let pinia: ReturnType<typeof createPinia>

beforeEach(() => {
  pinia = createPinia()
  setActivePinia(pinia)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('agents store backend facts', () => {
  it('labels a known backend with its catalog name and an unknown one with its raw id', () => {
    const agents = useAgentsStore()
    agents.backends = backendFixtures()
    expect(agents.backendLabel('opencode-v2')).toBe('OpenCode')
    expect(agents.backendLabel('claude-code')).toBe('claude-code')
  })

  it('orders backends by the catalog order, not by the list order received', () => {
    const agents = useAgentsStore()
    agents.backends = [...backendFixtures()].reverse()
    expect(agents.orderedBackends.map((entry) => entry.backend)).toEqual(['opencode-v2', 'pi'])
  })

  it('reads a valid catalog brand colour and ignores an invalid one', () => {
    const agents = useAgentsStore()
    agents.backends = backendFixtures()
    expect(agents.backendColor('opencode-v2')).toBe('#10B981')
    expect(agents.backendColor('pi')).toBe('#F97316')
    agents.backends = [{ ...backendFixtures()[0], color: 'red' }]
    expect(agents.backendColor('opencode-v2')).toBe('')
  })

  it('lists only the backends whose service the operator can start and stop', () => {
    const agents = useAgentsStore()
    agents.backends = backendFixtures()
    expect(agents.serviceBackends.map((entry) => entry.backend)).toEqual(['opencode-v2'])
  })

  it('treats a malformed backend list as empty instead of failing', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ status: 200, json: async () => ({}) }) as Response),
    )
    const agents = useAgentsStore()
    await agents.loadBackends()
    expect(agents.backends).toEqual([])
    expect(agents.orderedBackends).toEqual([])
  })
})

describe('BackendTag', () => {
  it('shows the catalog label for a known backend', async () => {
    const agents = useAgentsStore()
    agents.backends = backendFixtures()
    const wrapper = mount(BackendTag, { props: { backend: 'pi' }, global: { plugins: [pinia] } })
    await flushPromises()
    expect(wrapper.text()).toBe('Pi Agent')
  })

  it('renders the declared brand colour as a soft-toned tag', async () => {
    const agents = useAgentsStore()
    agents.backends = backendFixtures()
    const wrapper = mount(BackendTag, { props: { backend: 'opencode-v2' }, global: { plugins: [pinia] } })
    await flushPromises()
    expect(wrapper.attributes('data-backend-color')).toBe('#10B981')
    expect(wrapper.classes()).toContain('has-color')
    expect(wrapper.attributes('style')).toContain('--tag-base: #10B981')
    // The chip keeps the original tag typography; only its hue is declared.
    expect(wrapper.attributes('style')).not.toContain('background-color')
  })

  it('uses declared tone when the backend specifies one', async () => {
    const agents = useAgentsStore()
    agents.backends = [
      ...backendFixtures(),
      {
        backend: 'deepseek',
        capabilities: backendFixtures()[0].capabilities,
        enabled: true,
        label: 'DeepSeek',
        order: 3,
        serviceSupported: false,
        tone: 'info',
      },
    ]
    const wrapper = mount(BackendTag, { props: { backend: 'deepseek' }, global: { plugins: [pinia] } })
    await flushPromises()
    expect(wrapper.text()).toBe('DeepSeek')
    expect(wrapper.classes()).toContain('tone-info')
  })

  it('never names an unknown backend after another one', async () => {
    const agents = useAgentsStore()
    agents.backends = backendFixtures()
    const wrapper = mount(BackendTag, { props: { backend: 'claude-code' }, global: { plugins: [pinia] } })
    await flushPromises()
    expect(wrapper.text()).toBe('claude-code')
    expect(wrapper.text()).not.toContain('opencode')
    expect(wrapper.classes()).toContain('tone-neutral')
  })
})
