// Tests for the agent management section: it renders detection state, keeps an
// auto-detected path read-only, allows a manual path only when auto detection
// failed, runs detection, and toggles platform enablement.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AgentSettings from '../features/settings/AgentSettings.vue'
import { useAuthStore } from '../stores/auth'
import { installTestCSRF } from './csrf-fixture'
import { useAgentsStore } from '../stores/agents'
import type { AgentServiceStatus } from '../api/agents'

type Call = { method: string; path: string; body: any }

function backendList() {
  const capabilities = (sessionHistory: boolean) => ({
    streaming: true, sessionDiscovery: false, sessionContinue: false, stop: true, interrupt: false,
    modelSelection: true, toolCallEvents: false, toolConfirmation: false, sessionHistory, rawEvents: false,
  })
  return {
    backends: [
      { backend: 'opencode-v2', capabilities: capabilities(true), enabled: true, label: 'OpenCode', order: 1, serviceSupported: true },
      { backend: 'pi', capabilities: capabilities(false), enabled: true, label: 'Pi Agent', order: 2, serviceSupported: false },
    ],
  }
}

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
      if (key in extra) {
        const payload = extra[key] as Record<string, unknown>
        // A stubbed payload may name its own status so a refusal is exercised
        // the way the server sends it (error envelope + 4xx).
        const status = typeof payload === 'object' && payload !== null && '__status' in payload
          ? (payload.__status as number)
          : 200
        const body = typeof payload === 'object' && payload !== null ? { ...payload, __status: undefined } : payload
        return { status, json: async () => body } as Response
      }
      if (key === 'GET /agents') return { status: 200, json: async () => agents } as Response
      if (key === 'GET /agent-backends') return { status: 200, json: async () => backendList() } as Response
      if (key === 'GET /agent-backends/diagnostics') {
        return { status: 200, json: async () => ({ diagnostics: [] }) } as Response
      }
      if (key.startsWith('GET /agent-backends/') && key.endsWith('/models')) {
        return { status: 200, json: async () => ({ models: [], updatedAt: null }) } as Response
      }
      return { status: 200, json: async () => ({}) } as Response
    }),
  )
  return calls
}

async function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().$patch({ authenticated: true })
  installTestCSRF('csrf-1')
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

describe('AgentSettings diagnostics and models', () => {
  it('shows backend diagnostics on the agent page and not elsewhere', async () => {
    const report = {
      backend: 'opencode-v2',
      status: 'warning',
      version: '2.0.24',
      testedVersion: '2.0.24',
      checks: [{ kind: 'version', status: 'warning', message: 'version outside the tested range' }],
      capabilities: {
        streaming: true, sessionDiscovery: false, sessionContinue: true, stop: true, interrupt: true,
        modelSelection: true, toolCallEvents: true, toolConfirmation: false, sessionHistory: false, rawEvents: false,
      },
    }
    const calls = stubFetch({ 'GET /agent-backends/diagnostics': { diagnostics: [report] } })
    const wrapper = await mountView()

    expect(calls.some((c) => c.method === 'GET' && c.path === '/agent-backends/diagnostics')).toBe(true)
    const details = wrapper.find('[data-test="agent-diagnostics-opencode-v2"]')
    expect(details.exists()).toBe(true)
    expect(details.text()).toContain('警告')
    expect(details.attributes('open')).toBeDefined()
    expect(details.text()).toContain('version outside the tested range')
  })

  it('reads the persisted model snapshot and only contacts the agent on refresh', async () => {
    const calls = stubFetch({
      'GET /agent-backends/opencode-v2/models': {
        models: [{ id: 'acme/gpt', name: 'GPT', provider: 'acme' }],
        updatedAt: '2026-10-23T12:00:00Z',
      },
      'POST /agent-backends/opencode-v2/models/refresh': {
        models: [
          { id: 'acme/gpt', name: 'GPT', provider: 'acme' },
          { id: 'acme/small', name: 'Small', provider: 'acme' },
        ],
        updatedAt: '2026-10-23T12:30:00Z',
      },
    })
    const wrapper = await mountView()

    const block = wrapper.find('[data-test="agent-models-opencode-v2"]')
    expect(block.exists()).toBe(true)
    expect(block.text()).toContain('GPT')
    // Mount reads the persisted snapshot only.
    expect(calls.filter((c) => c.method === 'POST' && c.path.includes('/models/refresh'))).toHaveLength(0)

    await wrapper.find('[data-test="agent-models-refresh-opencode-v2"]').trigger('click')
    await flushPromises()

    expect(
      calls.some((c) => c.method === 'POST' && c.path === '/agent-backends/opencode-v2/models/refresh'),
    ).toBe(true)
    expect(block.text()).toContain('Small')
  })

  it('explains that a backend has never been synced instead of asking it', async () => {
    const calls = stubFetch()
    const wrapper = await mountView()

    expect(wrapper.find('[data-test="agent-models-pi"]').text()).toContain('尚未同步模型')
    expect(calls.some((c) => c.method === 'POST' && c.path.includes('/models/refresh'))).toBe(false)
  })

  it('toggles model enablement and updates model configuration', async () => {
    const calls = stubFetch({
      'GET /agent-backends/opencode-v2/models': {
        models: [{ id: 'acme/gpt', name: 'GPT', provider: 'acme', enabled: true }],
        updatedAt: '2026-10-23T12:00:00Z',
      },
      'PATCH /agent-backends/opencode-v2/models': {
        id: 'acme/gpt',
        name: 'Custom GPT',
        provider: 'acme',
        enabled: false,
      },
    })
    const wrapper = await mountView()

    const toggle = wrapper.find('[data-agent-model-toggle="acme/gpt"]')
    expect(toggle.exists()).toBe(true)
    expect((toggle.element as HTMLInputElement).checked).toBe(true)

    // Toggle to disable
    await toggle.setValue(false)
    await flushPromises()

    const patchCall = calls.find((c) => c.method === 'PATCH' && c.path === '/agent-backends/opencode-v2/models')
    expect(patchCall).toBeDefined()
    expect(patchCall?.body).toMatchObject({ modelId: 'acme/gpt', enabled: false })

    // Open edit form
    await wrapper.find('[data-agent-model-edit="acme/gpt"]').trigger('click')
    await flushPromises()

    const form = wrapper.find('[data-agent-model-form="acme/gpt"]')
    expect(form.exists()).toBe(true)

    // Submit edit form
    await form.trigger('submit')
    await flushPromises()

    expect(calls.filter((c) => c.method === 'PATCH')).toHaveLength(2)
  })
})

// The machine-level service block: the settings page is the only place the
// platform starts or stops it, so what it shows must be exactly what the last
// detection observed, and stopping must never happen without a confirmation.
const runningStatus: AgentServiceStatus = {
  backend: 'opencode-v2',
  supported: true,
  state: 'running',
  endpoint: 'http://127.0.0.1:4096',
  pid: 4242,
  version: '2.0.24',
  executable: '/usr/local/bin/opencode',
  detail: '',
  checkedAt: '2026-10-13T08:00:00Z',
}

function serviceStatus(overrides: Record<string, unknown> = {}) {
  return { status: { ...runningStatus, ...overrides } }
}

describe('AgentSettings service block', () => {
  it('detects the service-backed backend once on load and never asks a backend without a service', async () => {
    const calls = stubFetch({ 'GET /agent-services/opencode-v2': serviceStatus() })
    await mountView()

    expect(calls.filter((c) => c.path === '/agent-services/opencode-v2')).toHaveLength(1)
    // pi has no machine-level service: asking would be noise and would render a
    // control the user must not see.
    expect(calls.some((c) => c.path.startsWith('/agent-services/pi'))).toBe(false)
  })

  it('offers start only when the service is not running, and stop only when it is', async () => {
    stubFetch({ 'GET /agent-services/opencode-v2': serviceStatus() })
    const wrapper = await mountView()

    expect(wrapper.find('[data-test="service-state-opencode-v2"]').text()).toContain('运行中')
    expect(wrapper.find('[data-test="service-start"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-test="service-stop"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.find('[data-test="service-endpoint"]').text()).toContain('127.0.0.1')
    expect(wrapper.find('[data-test="service-pid"]').text()).toContain('4242')
    expect(wrapper.find('[data-test="service-checked-at"]').text()).toContain('最后检测于')
  })

  it('refuses to offer stop when nothing is running', async () => {
    stubFetch({
      'GET /agent-services/opencode-v2': serviceStatus({ state: 'stopped', endpoint: '', pid: 0 }),
    })
    const wrapper = await mountView()

    expect(wrapper.find('[data-test="service-state-opencode-v2"]').text()).toContain('未启动')
    expect(wrapper.find('[data-test="service-stop"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-test="service-start"]').attributes('disabled')).toBeUndefined()
  })

  it('shows why an existing service is unusable', async () => {
    stubFetch({
      'GET /agent-services/opencode-v2': serviceStatus({
        state: 'unreachable',
        detail: 'connection refused',
      }),
    })
    const wrapper = await mountView()

    expect(wrapper.find('[data-test="service-state-opencode-v2"]').text()).toContain('无法连接')
    expect(wrapper.find('[data-test="service-detail"]').text()).toContain('connection refused')
  })

  it('asks for confirmation before stopping, because other clients lose their server', async () => {
    const calls = stubFetch({ 'GET /agent-services/opencode-v2': serviceStatus() })
    const wrapper = await mountView()

    await wrapper.find('[data-test="service-stop"]').trigger('click')
    await flushPromises()

    const dialog = wrapper.find('[data-test="service-stop-dialog"]')
    expect(dialog.exists()).toBe(true)
    expect(dialog.text()).toContain('桌面端 opencode')
    expect(dialog.text()).toContain('正在运行的会话会被中断')
    expect(calls.some((c) => c.path === '/agent-services/opencode-v2/stop')).toBe(false)

    await wrapper.find('[data-test="service-stop-confirm"]').trigger('click')
    await flushPromises()

    expect(calls.some((c) => c.method === 'POST' && c.path === '/agent-services/opencode-v2/stop')).toBe(true)
  })

  it('leaves the service alone when the operator cancels', async () => {
    const calls = stubFetch({ 'GET /agent-services/opencode-v2': serviceStatus() })
    const wrapper = await mountView()

    await wrapper.find('[data-test="service-stop"]').trigger('click')
    await wrapper.find('[data-test="service-stop-cancel"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="service-stop-dialog"]').exists()).toBe(false)
    expect(calls.some((c) => c.path === '/agent-services/opencode-v2/stop')).toBe(false)
  })

  it('detects again only when the operator asks', async () => {
    const calls = stubFetch({ 'GET /agent-services/opencode-v2': serviceStatus() })
    const wrapper = await mountView()

    await wrapper.find('[data-test="service-detect"]').trigger('click')
    await flushPromises()

    expect(calls.filter((c) => c.path === '/agent-services/opencode-v2')).toHaveLength(2)
  })

  it('shows the reason a start or stop was refused', async () => {
    stubFetch({
      'GET /agent-services/opencode-v2': serviceStatus(),
      'POST /agent-services/opencode-v2/start': { __status: 409, error: 'service_already_running' },
    })
    const wrapper = await mountView()

    // A running service leaves start disabled, so drive the refused case from
    // a stopped service whose start races another client's.
    const store = useAgentsStore()
    store.services = { 'opencode-v2': { ...runningStatus, state: 'stopped' } }
    await flushPromises()
    await wrapper.find('[data-test="service-start"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="service-error"]').text()).toContain('已在运行')
  })
})
