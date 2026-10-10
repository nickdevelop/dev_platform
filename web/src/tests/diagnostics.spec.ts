// Unit tests for the backend diagnostics panel and the agents store's
// diagnostics loading: statuses, versions, redacted check messages and the
// capability matrix must render exactly as the backend reported them, and a
// failed diagnostics load must not mask the rest of the page.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia, type Pinia } from 'pinia'
import BackendDiagnostics from '../components/BackendDiagnostics.vue'
import { useAgentsStore } from '../stores/agents'
import type { AgentDiagnostic, BackendDiagnostics as BackendDiagnosticsReport } from '../api/agents'

const mounted: ReturnType<typeof mount>[] = []

afterEach(() => {
  for (const wrapper of mounted) wrapper.unmount()
  mounted.length = 0
  vi.unstubAllGlobals()
})

function report(overrides: Partial<BackendDiagnosticsReport> = {}): BackendDiagnosticsReport {
  return {
    backend: 'opencode-v2',
    status: 'ok',
    version: '2.0.24',
    testedVersion: '2.0.24',
    checks: [
      { kind: 'executable', status: 'ok', message: 'opencode executable "opencode" found' },
      { kind: 'version', status: 'ok', message: 'opencode CLI 2.0.24 is within the tested range [2.0.0, 3.0.0)' },
      { kind: 'api', status: 'ok', message: 'no running opencode instances to probe' },
    ],
    capabilities: {
      streaming: true,
      sessionDiscovery: true,
      sessionContinue: true,
      stop: true,
      interrupt: true,
      modelSelection: true,
      toolCallEvents: true,
      toolConfirmation: false,
      sessionHistory: false,
    },
    ...overrides,
  }
}

describe('BackendDiagnostics panel', () => {
  it('renders status, versions, checks and the capability matrix', () => {
    const wrapper = mount(BackendDiagnostics, {
      props: {
        diagnostics: [
          report(),
          report({
            backend: 'pi',
            status: 'warning',
            version: '2.4.0',
            testedVersion: '1.1.0',
            checks: [
              { kind: 'executable', status: 'ok', message: 'pi executable "pi" found' },
              {
                kind: 'version',
                status: 'warning',
                message: 'pi CLI 2.4.0 is outside the tested range [1.0.0, 2.0.0); the tested version is 1.1.0',
              },
            ],
          }),
        ],
      },
    })
    mounted.push(wrapper)

    const items = wrapper.findAll('[data-backend-diagnostics]')
    expect(items).toHaveLength(2)
    expect(items[0].attributes('data-backend-diagnostics')).toBe('opencode-v2')
    expect(items[0].find('[data-diagnostics-status]').text()).toBe('正常')
    expect(items[0].text()).toContain('检测到 2.0.24（已验证 2.0.24）')
    expect(items[0].findAll('[data-diagnostic-kind]')).toHaveLength(3)
    expect(items[0].text()).toContain('opencode executable "opencode" found')

    // Unsupported capabilities are visibly off, never presented as available.
    expect(items[0].text()).toContain('工具确认 ✕')
    expect(items[0].text()).not.toContain('流式输出 ✕')

    expect(items[1].attributes('data-backend-diagnostics')).toBe('pi')
    expect(items[1].find('[data-diagnostics-status]').text()).toBe('警告')
    expect(items[1].text()).toContain('outside the tested range')
  })

  it('renders an unknown backend without checks honestly', () => {
    const wrapper = mount(BackendDiagnostics, {
      props: { diagnostics: [report({ status: 'unknown', version: undefined, testedVersion: undefined, checks: [], capabilities: undefined })] },
    })
    mounted.push(wrapper)

    expect(wrapper.find('[data-diagnostics-status]').text()).toBe('未知')
    expect(wrapper.text()).toContain('该后端未提供诊断信息。')
    expect(wrapper.text()).not.toContain('已验证')
  })

  it('shows the empty state when no diagnostics were loaded', () => {
    const wrapper = mount(BackendDiagnostics, { props: { diagnostics: [] } })
    mounted.push(wrapper)
    expect(wrapper.text()).toContain('暂无后端诊断信息。')
  })

  it('renders defensively when a report has no checks array', () => {
    const wrapper = mount(BackendDiagnostics, {
      props: {
        diagnostics: [report({ checks: undefined as unknown as AgentDiagnostic[] })],
      },
    })
    mounted.push(wrapper)

    expect(wrapper.find('[data-diagnostics-status]').text()).toBe('正常')
    expect(wrapper.text()).toContain('该后端未提供诊断信息。')
  })
})

describe('agents store diagnostics', () => {
  let pinia: Pinia
  function setup() {
    pinia = createPinia()
    setActivePinia(pinia)
    return useAgentsStore()
  }

  it('loads the diagnostics report', async () => {
    const store = setup()
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        status: 200,
        json: async () => ({ diagnostics: [report()] }),
      })),
    )

    await store.loadDiagnostics()
    await flushPromises()

    expect(store.diagnostics).toHaveLength(1)
    expect(store.diagnostics[0].backend).toBe('opencode-v2')
    expect(store.diagnosticsError).toBe('')
  })

  it('records a failed load on its own flag without touching other state', async () => {
    const store = setup()
    store.error = 'instances error'
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ status: 500, json: async () => ({ error: 'server_error' }) })),
    )

    await store.loadDiagnostics()
    await flushPromises()

    expect(store.diagnostics).toEqual([])
    expect(store.diagnosticsError).not.toBe('')
    expect(store.error).toBe('instances error')
  })

  it('clears a stale error after a successful reload', async () => {
    const store = setup()
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ status: 500, json: async () => ({ error: 'server_error' }) })),
    )
    await store.loadDiagnostics()
    await flushPromises()
    expect(store.diagnosticsError).not.toBe('')

    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ status: 200, json: async () => ({ diagnostics: [report()] }) })),
    )
    await store.loadDiagnostics()
    await flushPromises()

    expect(store.diagnosticsError).toBe('')
    expect(store.diagnostics).toHaveLength(1)
  })
})
