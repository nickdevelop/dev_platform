// Tests for the agent management section: it renders detection state, keeps an
// auto-detected path read-only, allows a manual path only when auto detection
// failed, runs detection, and toggles platform enablement.
//
// The view is Element Plus now: keep the functional API mocked at the module
// boundary (result feedback is ElMessage, destructive confirmation is
// ElMessageBox) while every `<el-*>` component renders for real through the
// resolver. The spread of `importOriginal` is what keeps the components alive
// under the mock.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Mock } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { ElMessage, ElMessageBox } from 'element-plus'
import AgentSettings from '../features/engines/AgentSettingsView.vue'
import { useAuthStore } from '../stores/auth'
import { installTestCSRF } from './csrf-fixture'
import { useAgentsStore } from '../stores/agents'
import type { AgentServiceStatus } from '../api/agents'

vi.mock('element-plus', async (importOriginal) => {
  const actual = await importOriginal<typeof import('element-plus')>()
  return {
    ...actual,
    ElMessage: {
      success: vi.fn(),
      error: vi.fn(),
      warning: vi.fn(),
      info: vi.fn(),
    },
    ElMessageBox: {
      confirm: vi.fn(() => Promise.resolve(true)),
    },
  }
})

type Call = { method: string; path: string; body: any }

const confirmMock = ElMessageBox.confirm as unknown as Mock
const messageSuccess = ElMessage.success as unknown as Mock
const messageError = ElMessage.error as unknown as Mock

function backendList() {
  const capabilities = (sessionHistory: boolean) => ({
    streaming: true, sessionDiscovery: false, sessionContinue: false, stop: true, interrupt: false,
    modelSelection: true, toolCallEvents: false, toolConfirmation: false, sessionHistory, rawEvents: false,
  })
  return {
    backends: [
      { backend: 'opencode-v2', capabilities: capabilities(true), enabled: true, label: 'OpenCode', order: 1, serviceSupported: true },
      { backend: 'pi', capabilities: capabilities(false), enabled: true, label: 'Pi Agent', order: 2, serviceSupported: false },
      { backend: 'deepseek', capabilities: capabilities(false), enabled: true, label: 'DeepSeek', order: 3, serviceSupported: false },
      { backend: 'builtin', capabilities: capabilities(true), enabled: true, label: '内置Agent', order: 4, serviceSupported: false },
    ],
  }
}

function stubFetch(extra: Record<string, unknown> = {}) {
  const calls: Call[] = []
  const agents = {
    agents: [
      { backend: 'pi', enabled: true, path: '/usr/local/bin/pi', pathSource: 'auto', available: true, version: '1.1.0' },
      { backend: 'opencode-v2', enabled: false, path: '', pathSource: 'none', available: false },
      { backend: 'deepseek', enabled: true, path: '/usr/local/bin/dsh', pathSource: 'auto', available: true, version: '0.2.1', idleTimeoutMinutes: 30 },
      { backend: 'builtin', enabled: true, path: '', pathSource: 'embedded', available: true, maxTurnSteps: 30, defaultMaxTurnSteps: 30 },
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

beforeEach(() => {
  confirmMock.mockReset()
  confirmMock.mockResolvedValue(true)
  messageSuccess.mockClear()
  messageError.mockClear()
})

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

  it('marks each tab dot by platform enablement, not by detection', async () => {
    // pi is enabled but its binary is missing; opencode-v2 is detected but off.
    // The dot must follow the enablement, so the two states must disagree.
    stubFetch({
      'GET /agents': {
        agents: [
          { backend: 'pi', enabled: true, path: '', pathSource: 'none', available: false },
          { backend: 'opencode-v2', enabled: false, path: '/usr/bin/opencode', pathSource: 'auto', available: true, version: '2.0.27' },
        ],
      },
    })
    const wrapper = await mountView()

    const enabledDot = wrapper.find('[data-test="agent-tab-pi"] span[title="已启用"]')
    expect(enabledDot.exists()).toBe(true)
    expect(enabledDot.classes()).toContain('bg-emerald-500')

    const disabledDot = wrapper.find('[data-test="agent-tab-opencode-v2"] span[title="未启用"]')
    expect(disabledDot.exists()).toBe(true)
    expect(disabledDot.classes()).toContain('bg-slate-300')

    // Detection stays in the card body, never on the tab.
    expect(wrapper.find('[data-test="agent-tab-pi"]').text()).not.toContain('检测到')
  })

  it('runs a detection and reports the version once through ElMessage', async () => {
    const calls = stubFetch({ 'POST /agents/pi/detect': { backend: 'pi', status: 'ok', version: '1.2.3' } })
    const wrapper = await mountView()

    await wrapper.find('[data-detect="pi"]').trigger('click')
    await flushPromises()

    expect(calls.some((c) => c.method === 'POST' && c.path === '/agents/pi/detect')).toBe(true)
    expect(messageSuccess).toHaveBeenCalledWith('检测成功：版本 1.2.3')
  })

  it('toggles platform enablement', async () => {
    const calls = stubFetch()
    const wrapper = await mountView()

    await wrapper.find('[data-agent-enabled="opencode-v2"] input').setValue(true)
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
    const collapse = wrapper.find('[data-test="agent-diagnostics-opencode-v2"]')
    expect(collapse.exists()).toBe(true)
    expect(collapse.text()).toContain('警告')
    // A warning opens the accordion by default.
    expect(collapse.find('.el-collapse-item').classes()).toContain('is-active')
    expect(collapse.text()).toContain('version outside the tested range')
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

  it('orders enabled models first and filters by name', async () => {
    stubFetch({
      'GET /agent-backends/opencode-v2/models': {
        models: [
          { id: 'acme/plain', name: 'Plain Model', provider: 'acme', enabled: false },
          { id: 'acme/small', name: 'Small Model', provider: 'acme', enabled: true },
          { id: 'acme/vision', name: 'Vision Model', provider: 'acme', enabled: true },
        ],
        updatedAt: '2026-10-23T12:00:00Z',
      },
    })
    const wrapper = await mountView()

    const block = wrapper.find('[data-test="agent-models-opencode-v2"]')
    // The row order is visible through the per-row switch hooks.
    let ids = block.findAll('[data-agent-model-toggle]').map((el) => el.attributes('data-agent-model-toggle'))
    // Enabled models keep their relative order and move above the disabled one.
    expect(ids).toEqual(['acme/small', 'acme/vision', 'acme/plain'])

    await block.find('[data-test="agent-models-search-opencode-v2"]').setValue('vision')
    await flushPromises()
    ids = block.findAll('[data-agent-model-toggle]').map((el) => el.attributes('data-agent-model-toggle'))
    expect(ids).toEqual(['acme/vision'])
    expect(block.text()).not.toContain('Plain Model')
  })

  it('selects all and clears all models through the bulk endpoint', async () => {
    const calls = stubFetch({
      'GET /agent-backends/opencode-v2/models': {
        models: [
          { id: 'acme/one', name: 'One', provider: 'acme', enabled: true },
          { id: 'acme/two', name: 'Two', provider: 'acme', enabled: true },
        ],
        updatedAt: '2026-10-23T12:00:00Z',
      },
      'PUT /agent-backends/opencode-v2/models/enabled': {
        models: [
          { id: 'acme/one', name: 'One', provider: 'acme', enabled: false },
          { id: 'acme/two', name: 'Two', provider: 'acme', enabled: false },
        ],
        updatedAt: '2026-10-23T12:00:00Z',
      },
    })
    const wrapper = await mountView()

    await wrapper.find('[data-test="agent-models-clear-all-opencode-v2"]').trigger('click')
    await flushPromises()

    const clear = calls.find(
      (c) => c.method === 'PUT' && c.path === '/agent-backends/opencode-v2/models/enabled',
    )
    expect(clear?.body).toEqual({ enabled: false })
    for (const toggle of wrapper.findAll('[data-agent-model-toggle]')) {
      expect((toggle.find('input').element as HTMLInputElement).checked).toBe(false)
    }
  })

  it('selects all models through the bulk endpoint', async () => {
    const calls = stubFetch({
      'GET /agent-backends/opencode-v2/models': {
        models: [
          { id: 'acme/one', name: 'One', provider: 'acme', enabled: false },
          { id: 'acme/two', name: 'Two', provider: 'acme', enabled: false },
        ],
        updatedAt: '2026-10-23T12:00:00Z',
      },
      'PUT /agent-backends/opencode-v2/models/enabled': {
        models: [
          { id: 'acme/one', name: 'One', provider: 'acme', enabled: true },
          { id: 'acme/two', name: 'Two', provider: 'acme', enabled: true },
        ],
        updatedAt: '2026-10-23T12:00:00Z',
      },
    })
    const wrapper = await mountView()

    await wrapper.find('[data-test="agent-models-select-all-opencode-v2"]').trigger('click')
    await flushPromises()

    const clear = calls.find(
      (c) => c.method === 'PUT' && c.path === '/agent-backends/opencode-v2/models/enabled',
    )
    expect(clear?.body).toEqual({ enabled: true })
    for (const toggle of wrapper.findAll('[data-agent-model-toggle]')) {
      expect((toggle.find('input').element as HTMLInputElement).checked).toBe(true)
    }
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
    expect((toggle.find('input').element as HTMLInputElement).checked).toBe(true)

    // Toggle to disable
    await toggle.find('input').setValue(false)
    await flushPromises()

    const patchCall = calls.find((c) => c.method === 'PATCH' && c.path === '/agent-backends/opencode-v2/models')
    expect(patchCall).toBeDefined()
    expect(patchCall?.body).toMatchObject({ modelId: 'acme/gpt', enabled: false })

    // Open the edit dialog
    await wrapper.find('[data-agent-model-edit="acme/gpt"]').trigger('click')
    await flushPromises()

    const form = wrapper.find('[data-test="agent-model-form"]')
    expect(form.exists()).toBe(true)

    // Submit through the shared footer's save button
    await wrapper.find('[data-test="agent-model-form-submit"]').trigger('click')
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

    expect(confirmMock).toHaveBeenCalledTimes(1)
    const question = String(confirmMock.mock.calls[0][0])
    expect(question).toContain('桌面端 opencode')
    expect(question).toContain('正在运行的会话会被中断')

    expect(calls.some((c) => c.method === 'POST' && c.path === '/agent-services/opencode-v2/stop')).toBe(true)
  })

  it('leaves the service alone when the operator cancels', async () => {
    const calls = stubFetch({ 'GET /agent-services/opencode-v2': serviceStatus() })
    confirmMock.mockRejectedValueOnce(new Error('cancel'))
    const wrapper = await mountView()

    await wrapper.find('[data-test="service-stop"]').trigger('click')
    await flushPromises()

    expect(confirmMock).toHaveBeenCalledTimes(1)
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

    expect(
      messageError.mock.calls.some((args) => String(args[0]).includes('已在运行')),
    ).toBe(true)
  })

  it('switches between agent tabs and configures dsh idle timeout', async () => {
    const calls = stubFetch({
      'PUT /agents/deepseek/idle-timeout': {
        backend: 'deepseek',
        enabled: true,
        path: '/usr/local/bin/dsh',
        pathSource: 'auto',
        available: true,
        idleTimeoutMinutes: 15,
      },
    })
    const wrapper = await mountView()

    // Tab labels exist for each agent
    expect(wrapper.find('[data-test="agent-tab-opencode-v2"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="agent-tab-pi"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="agent-tab-deepseek"]').exists()).toBe(true)

    // Switch to deepseek tab
    await wrapper.find('[data-test="agent-tab-deepseek"]').trigger('click')
    await flushPromises()

    // DeepSeek card is visible and provides idle timeout controls
    const idleBlock = wrapper.find('[data-test="agent-idle-timeout-deepseek"]')
    expect(idleBlock.exists()).toBe(true)
    expect(idleBlock.text()).toContain('DSH 进程空闲断开时间')

    // Use the 15m preset (a click on the fourth preset chip) and save
    const presetButtons = idleBlock.findAll('.preset-buttons button')
    await presetButtons[0].trigger('click')
    await wrapper.find('[data-test="save-idle-timeout-deepseek"]').trigger('click')
    await flushPromises()

    const putCall = calls.find((c) => c.method === 'PUT' && c.path === '/agents/deepseek/idle-timeout')
    expect(putCall).toBeDefined()
    expect(putCall?.body).toMatchObject({ minutes: 15 })
    expect(messageSuccess).toHaveBeenCalledWith('空闲断开时间已成功保存')
  })

  it('renders general settings tab by default and saves system prompt', async () => {
    const calls = stubFetch({
      'GET /settings/agent-general': {
        systemPrompt: '# Initial Instructions\nAlways be helpful.',
        path: '/tmp/devplatform/AGENTS.md',
        updatedAt: '2026-11-02T10:00:00Z',
      },
      'PUT /settings/agent-general': {
        systemPrompt: '# Updated Instructions\nBe concise.',
        path: '/tmp/devplatform/AGENTS.md',
        updatedAt: '2026-11-02T10:05:00Z',
      },
    })
    const wrapper = await mountView()

    // 1. Check general tab exists and is selected by default
    const generalTab = wrapper.find('[data-test="agent-tab-general"]')
    expect(generalTab.exists()).toBe(true)
    const activeItems = wrapper.findAll('.el-tabs__item.is-active')
    expect(activeItems).toHaveLength(1)
    expect(activeItems[0].text()).toContain('全局设置')

    // 2. Check general settings card is rendered with initial prompt
    const generalCard = wrapper.find('[data-test="agent-card-general"]')
    expect(generalCard.exists()).toBe(true)
    const promptInput = wrapper.find('[data-test="agent-general-prompt-input"]')
    expect(promptInput.exists()).toBe(true)
    expect((promptInput.element as HTMLTextAreaElement).value).toBe('# Initial Instructions\nAlways be helpful.')

    // 3. Edit system prompt and save
    await promptInput.setValue('# Updated Instructions\nBe concise.')
    await wrapper.find('[data-test="save-agent-general"]').trigger('click')
    await flushPromises()

    const putCall = calls.find((c) => c.method === 'PUT' && c.path === '/settings/agent-general')
    expect(putCall).toBeDefined()
    expect(putCall?.body).toMatchObject({ systemPrompt: '# Updated Instructions\nBe concise.' })
    expect(messageSuccess).toHaveBeenCalledWith('全局系统提示词已成功保存并同步')

    // 4. Switch to an agent tab and then switch back to general
    await wrapper.find('[data-test="agent-tab-pi"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.el-tabs__item.is-active')[0].text()).toContain('Pi Agent')

    await generalTab.trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.el-tabs__item.is-active')[0].text()).toContain('全局设置')
  })

  it('switches to builtin tab and delegates max turn steps to coder engine', async () => {
    stubFetch()
    const wrapper = await mountView()

    // Tab label exists for builtin
    expect(wrapper.find('[data-test="agent-tab-builtin"]').exists()).toBe(true)
    await wrapper.find('[data-test="agent-tab-builtin"]').trigger('click')
    await flushPromises()

    // Builtin max turn steps block is removed from AgentSettingsView and unified in CoderEngineView
    const stepsBlock = wrapper.find('[data-test="agent-max-turn-steps-builtin"]')
    expect(stepsBlock.exists()).toBe(false)
  })

  it('renders usage panel and refreshes usage when supportsUsage is true', async () => {
    const usageReport = {
      backend: 'pi',
      description: 'Quota is consumed proportionally.',
      groups: [
        {
          name: 'Gemini Models',
          description: 'Gemini 3.8 Flash',
          buckets: [
            {
              id: 'gemini-weekly',
              name: 'Weekly Limit Remaining',
              remainingFraction: 0.81,
              resetTime: '2026-11-21T01:13:16Z',
            },
          ],
        },
      ],
      rawOutput: 'Gemini Models 81%',
      updatedAt: '2026-11-17T02:35:12Z',
    }

    const calls = stubFetch({
      'GET /agent-backends/pi/usage': usageReport,
    })

    // Pi agent has supportsUsage: true
    const agentsResp = {
      agents: [
        { backend: 'pi', enabled: true, path: '/usr/local/bin/pi', pathSource: 'auto', available: true, version: '1.1.0', supportsUsage: true },
      ],
    }
    // Override GET /agents
    const origFetch = global.fetch
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input).replace('/api', '')
      const method = init?.method ?? 'GET'
      if (`${method} ${path}` === 'GET /agents') {
        return { status: 200, json: async () => agentsResp } as Response
      }
      return origFetch(input, init)
    }))

    const wrapper = await mountView()
    await wrapper.find('[data-test="agent-tab-pi"]').trigger('click')
    await flushPromises()

    const panel = wrapper.find('[data-test="agent-usage-panel-pi"]')
    expect(panel.exists()).toBe(true)
    expect(panel.text()).toContain('使用量与配额查询')
    expect(panel.text()).toContain('Gemini Models')
    expect(panel.text()).toContain('剩余 81%')

    // Click refresh button
    const refreshBtn = panel.find('[data-test="refresh-usage-btn"]')
    expect(refreshBtn.exists()).toBe(true)
    await refreshBtn.trigger('click')
    await flushPromises()

    const usageCalls = calls.filter((c) => c.method === 'GET' && c.path === '/agent-backends/pi/usage')
    expect(usageCalls.length).toBeGreaterThanOrEqual(1)
  })
})
