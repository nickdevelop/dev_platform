// Unit tests for TerminalSettingsView.vue
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import TerminalSettingsView from '../features/settings/TerminalSettingsView.vue'
import { useTerminalStore } from '../stores/terminal'
import { useProjectTerminalStore } from '../stores/projectTerminal'
import { useAuthStore } from '../stores/auth'
import { installTestCSRF } from './csrf-fixture'

describe('TerminalSettingsView Component', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  async function mountView(supported = true) {
    const pinia = createPinia()
    setActivePinia(pinia)

    useAuthStore().$patch({ authenticated: true })
    installTestCSRF('test-csrf')

    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        const method = init?.method || 'GET'

        if (url.includes('/terminal/status')) {
          return {
            status: 200,
            json: async () => ({ supported, os: supported ? 'linux' : 'windows' }),
          } as Response
        }
        if (url.includes('/terminal/servers/test')) {
          return {
            status: 200,
            json: async () => ({ ok: true, message: '连接成功' }),
          } as Response
        }
        if (url.includes('/terminal/servers') && method === 'DELETE') {
          return {
            status: 200,
            json: async () => ({ ok: true }),
          } as Response
        }
        if (url.includes('/terminal/servers')) {
          return {
            status: 200,
            json: async () => [
              {
                id: 0,
                name: '本机 (Localhost)',
                host: 'localhost',
                port: 0,
                username: 'local',
                auth_type: 'local',
                is_local: true,
                has_secret: false,
              },
              {
                id: 1,
                name: '生产服务器',
                host: '10.0.0.12',
                port: 22,
                username: 'root',
                auth_type: 'password',
                is_local: false,
                has_secret: true,
              },
            ],
          } as Response
        }
        return { status: 200, json: async () => ({}) } as Response
      }),
    )

    const wrapper = mount(TerminalSettingsView, { global: { plugins: [pinia] } })
    await flushPromises()
    return {
      wrapper,
      terminalStore: useTerminalStore(),
      projectTerminal: useProjectTerminalStore(),
    }
  }

  it('renders local environment and remote server list', async () => {
    const { wrapper } = await mountView(true)

    expect(wrapper.text()).toContain('终端服务器设置')
    expect(wrapper.text()).toContain('本地宿主环境')
    expect(wrapper.text()).toContain('生产服务器')
    expect(wrapper.find('[data-test="create-server-btn"]').exists()).toBe(true)
  })

  it('shows warning alert when local PTY is not supported', async () => {
    const { wrapper } = await mountView(false)
    expect(wrapper.text()).toContain('不支持本地 PTY')
    const openLocalBtn = wrapper.find('[data-test="open-local-term-btn"]')
    expect(openLocalBtn.attributes('disabled')).toBeDefined()
  })

  it('opens terminal tab when clicking open terminal buttons', async () => {
    const { wrapper, projectTerminal } = await mountView(true)

    // Open local terminal
    const openLocalBtn = wrapper.find('[data-test="open-local-term-btn"]')
    await openLocalBtn.trigger('click')
    expect(projectTerminal.tabs.length).toBe(1)
    expect(projectTerminal.tabs[0].serverId).toBe(0)

    // Open remote server terminal
    const openServerBtn = wrapper.find('[data-test="open-server-term-btn"]')
    await openServerBtn.trigger('click')
    expect(projectTerminal.tabs.length).toBe(2)
    expect(projectTerminal.tabs[1].serverId).toBe(1)
  })

  it('tests server connection and displays result pill', async () => {
    const { wrapper } = await mountView(true)

    const testBtn = wrapper.find('[data-test="test-server-btn"]')
    expect(testBtn.exists()).toBe(true)
    await testBtn.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('连接测试成功')
  })

  it('opens create server dialog when clicking create button', async () => {
    const { wrapper } = await mountView(true)

    const createBtn = wrapper.find('[data-test="create-server-btn"]')
    await createBtn.trigger('click')
    await flushPromises()

    expect(wrapper.findComponent({ name: 'ServerDialog' }).exists()).toBe(true)
  })

  it('opens delete confirmation and deletes server', async () => {
    const { wrapper, terminalStore } = await mountView(true)

    const deleteBtn = wrapper.find('[data-test="delete-server-btn"]')
    await deleteBtn.trigger('click')
    await flushPromises()

    expect(wrapper.findComponent({ name: 'UiConfirmDialog' }).exists()).toBe(true)

    // Confirm delete
    const confirmDialog = wrapper.findComponent({ name: 'UiConfirmDialog' })
    confirmDialog.vm.$emit('confirm')
    await flushPromises()

    expect(terminalStore.servers.some((s) => s.id === 1)).toBe(false)
  })
})
