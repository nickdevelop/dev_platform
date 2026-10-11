// Tests for the agent management section: it renders detection state, keeps an
// auto-detected path read-only, allows a manual path only when auto detection
// failed, runs detection, and toggles platform enablement.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AgentSettings from '../components/settings/AgentSettings.vue'
import { useAuthStore } from '../stores/auth'

type Call = { method: string; path: string; body: any }

function stubFetch(extra: Record<string, unknown> = {}) {
  const calls: Call[] = []
  const agents = {
    agents: [
      { backend: 'pi', enabled: true, path: '/usr/local/bin/pi', pathSource: 'auto', available: true, version: '1.1.0' },
      { backend: 'opencode-v2', enabled: false, path: '', pathSource: 'none', available: false },
    ],
  }
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input).replace('/api', '')
      const method = init?.method ?? 'GET'
      const body = init?.body ? JSON.parse(String(init.body)) : undefined
      calls.push({ method, path, body })
      const key = `${method} ${path}`
      if (key in extra) return { status: 200, json: async () => extra[key] } as Response
      if (key === 'GET /agents') return { status: 200, json: async () => agents } as Response
      return { status: 200, json: async () => ({}) } as Response
    }),
  )
  return calls
}

async function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().$patch({ authenticated: true, csrfToken: 'csrf-1' })
  const wrapper = mount(AgentSettings, { global: { plugins: [pinia] } })
  await flushPromises()
  return wrapper
}

afterEach(() => vi.unstubAllGlobals())

describe('AgentSettings', () => {
  it('shows an auto-detected path read-only and offers manual input when missing', async () => {
    stubFetch()
    const wrapper = await mountView()

    const piPath = wrapper.find('[data-agent-path-display="pi"]')
    expect(piPath.text()).toContain('/usr/local/bin/pi')
    expect(wrapper.find('[data-agent-path-input="pi"]').exists()).toBe(false)

    expect(wrapper.find('[data-agent-path-input="opencode-v2"]').exists()).toBe(true)
  })

  it('runs a detection and shows the version', async () => {
    const calls = stubFetch({ 'POST /agents/pi/detect': { backend: 'pi', status: 'ok', version: '1.2.3' } })
    const wrapper = await mountView()

    await wrapper.find('[data-detect="pi"]').trigger('click')
    await flushPromises()

    expect(calls.some((c) => c.method === 'POST' && c.path === '/agents/pi/detect')).toBe(true)
    expect(wrapper.text()).toContain('1.2.3')
  })

  it('toggles platform enablement', async () => {
    const calls = stubFetch()
    const wrapper = await mountView()

    await wrapper.find('[data-agent-enabled="opencode-v2"]').setValue(true)
    await flushPromises()

    const put = calls.find((c) => c.method === 'PUT' && c.path === '/agents/opencode-v2')
    expect(put?.body).toMatchObject({ enabled: true })
  })
})
