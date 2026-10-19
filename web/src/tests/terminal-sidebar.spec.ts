// Unit tests for TerminalSidebar.vue
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import TerminalSidebar from '../features/terminal/TerminalSidebar.vue'
import { useTerminalStore } from '../stores/terminal'
import { useAuthStore } from '../stores/auth'

describe('TerminalSidebar Component', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  async function mountSidebar(supported = true) {
    const pinia = createPinia()
    setActivePinia(pinia)

    useAuthStore().$patch({ authenticated: true, csrfToken: 'test-csrf' })

    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.includes('/terminal/status')) {
          return {
            status: 200,
            json: async () => ({ supported, os: supported ? 'linux' : 'windows' }),
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
                name: '测试服务器',
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

    const wrapper = mount(TerminalSidebar, { global: { plugins: [pinia] } })
    await flushPromises()
    return { wrapper, store: useTerminalStore() }
  }

  it('renders server list with localhost and remote server', async () => {
    const { wrapper } = await mountSidebar(true)

    expect(wrapper.find('h2').text()).toBe('服务器')
    expect(wrapper.text()).toContain('本机 (Localhost)')
    expect(wrapper.text()).toContain('测试服务器')
    expect(wrapper.find('.local-badge').text()).toBe('本机')
  })

  it('shows platform warning when not on Linux', async () => {
    const { wrapper } = await mountSidebar(false)
    expect(wrapper.find('.platform-warning').exists()).toBe(true)
    expect(wrapper.text()).toContain('该功能仅在 Linux 服务器环境中支持')
  })

  it('filters servers by search keyword', async () => {
    const { wrapper } = await mountSidebar(true)

    const searchInput = wrapper.find('input.search')
    await searchInput.setValue('测试')

    expect(wrapper.text()).toContain('测试服务器')
    expect(wrapper.text()).not.toContain('本机 (Localhost)')
  })

  it('clicking connect opens a terminal tab', async () => {
    const { wrapper, store } = await mountSidebar(true)

    const connectButtons = wrapper.findAll('.btn-connect')
    expect(connectButtons.length).toBeGreaterThan(0)

    await connectButtons[0].trigger('click')
    expect(store.tabs.length).toBe(1)
    expect(store.activeTabId).toBe(store.tabs[0].id)
  })

  it('opens add server dialog on button click', async () => {
    const { wrapper } = await mountSidebar(true)

    expect(wrapper.find('.server-form').exists()).toBe(false)
    await wrapper.find('.create-server-btn').trigger('click')
    expect(wrapper.find('.server-form').exists()).toBe(true)
  })
})
