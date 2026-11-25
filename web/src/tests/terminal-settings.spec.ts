// Unit tests for TerminalSettingsView.vue
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import TerminalSettingsView from '../features/settings/TerminalSettingsView.vue'
import { useTerminalStore } from '../stores/terminal'
import { useProjectTerminalStore } from '../stores/projectTerminal'
import { useAuthStore } from '../stores/auth'
import { installTestCSRF } from './csrf-fixture'

// el-select / ElMessageBox 的内容是 Teleport 到 body 的，VTU 不会自动卸载组件，
// 上一个用例留下的浮层会被下一个用例的 document 查询捞到（点了没反应）。
enableAutoUnmount(afterEach)

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
        if (url.includes('/terminal/local/shells/scan')) {
          return {
            status: 200,
            json: async () => [
              {
                id: 'git-bash',
                name: 'Git Bash',
                path: 'C:\\Program Files\\Git\\bin\\bash.exe',
                args: ['--login', '-i'],
                isRecommended: true,
              },
              {
                id: 'powershell',
                name: 'Windows PowerShell',
                path: 'C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe',
                args: ['-NoLogo'],
                isRecommended: false,
              },
            ],
          } as Response
        }
        if (url.includes('/terminal/local/shell/validate')) {
          return {
            status: 200,
            json: async () => ({ ok: true, message: 'Shell 路径有效且可执行' }),
          } as Response
        }
        if (url.includes('/terminal/local/shell') && method === 'PUT') {
          const body = typeof init?.body === 'string' ? JSON.parse(init.body) : {}
          return {
            status: 200,
            json: async () => ({
              path: body.path || 'C:\\Program Files\\Git\\bin\\bash.exe',
              args: body.args || ['--login', '-i'],
              name: 'Git Bash',
              is_custom: true,
              is_recommended: true,
            }),
          } as Response
        }
        if (url.includes('/terminal/local/shell')) {
          return {
            status: 200,
            json: async () => ({
              path: '/bin/bash',
              args: [],
              name: 'bash',
              is_custom: false,
              is_recommended: true,
            }),
          } as Response
        }
        if (url.includes('/terminal/servers') && method === 'DELETE') {
          return {
            status: 200,
            json: async () => ({ ok: true }),
          } as Response
        }
        if (url.includes('/terminal/config/reset') && method === 'POST') {
          return {
            status: 200,
            json: async () => ({ font_family: '' }),
          } as Response
        }
        if (url.includes('/terminal/config') && method === 'PUT') {
          const body = typeof init?.body === 'string' ? JSON.parse(init.body) : {}
          return {
            status: 200,
            json: async () => ({ font_family: body.font_family || '' }),
          } as Response
        }
        if (url.includes('/terminal/config')) {
          return {
            status: 200,
            json: async () => ({ font_family: '' }),
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

    expect(wrapper.text()).toContain('终端服务器')
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

    // ElMessageBox 挂在 document 上，不在组件树里。
    expect(document.body.textContent).toContain('确定要删除服务器「生产服务器」吗？')
    const confirm = Array.from(document.querySelectorAll<HTMLElement>('.el-message-box__btns button')).find(
      (btn) => btn.textContent?.trim() === '删除',
    )
    expect(confirm).toBeDefined()
    confirm?.click()
    await flushPromises()

    expect(terminalStore.servers.some((s) => s.id === 1)).toBe(false)
  })

  it('scans local shells and allows selecting and saving Git Bash', async () => {
    const { wrapper, terminalStore } = await mountView(true)

    // Check initial shell summary
    expect(wrapper.text()).toContain('当前生效 Shell')
    expect(wrapper.text()).toContain('bash')

    // Click scan shells button
    const scanBtn = wrapper.find('[data-test="scan-shells-btn"]')
    expect(scanBtn.exists()).toBe(true)
    await scanBtn.trigger('click')
    await flushPromises()

    // Discovered shells rendered
    expect(wrapper.text()).toContain('Git Bash')
    expect(wrapper.text()).toContain('Windows PowerShell')
    expect(terminalStore.discoveredShells.length).toBe(2)

    // Validate path
    const validateBtn = wrapper.find('[data-test="validate-shell-btn"]')
    const pathInput = wrapper.find<HTMLInputElement>('[data-test="shell-path-input"]')
    await pathInput.setValue('C:\\Program Files\\Git\\bin\\bash.exe')
    await validateBtn.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Shell 路径有效且可执行')

    // Save shell config
    const saveBtn = wrapper.find('[data-test="save-shell-btn"]')
    await saveBtn.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('本地终端 Shell 设置已保存并即时生效')
    expect(terminalStore.localShell?.is_custom).toBe(true)
  })

  it('renders terminal font settings and allows choosing preset, saving custom font, and resetting', async () => {
    const { wrapper, terminalStore } = await mountView(true)

    expect(wrapper.text()).toContain('终端字体')
    expect(wrapper.text()).toContain('平台统一字体')

    const fontInput = wrapper.find<HTMLInputElement>('[data-test="terminal-font-input"]')
    expect(fontInput.exists()).toBe(true)
    expect(fontInput.element.value).toBe('')

    // 预设现在是一个 el-select（枚举值就该用下拉，八个手绘 chip 会横向溢出）：
    // 点开浮层再选项，浮层挂在 document 上。
    await wrapper.find('.el-select__wrapper').trigger('click')
    await flushPromises()
    const option = Array.from(document.querySelectorAll<HTMLElement>('.el-select-dropdown__item')).find((el) =>
      el.textContent?.includes('JetBrains Mono'),
    )
    expect(option).toBeDefined()
    option?.click()
    await flushPromises()
    expect(fontInput.element.value).toBe('JetBrains Mono')

    // Save font
    const saveFontBtn = wrapper.find('[data-test="save-font-btn"]')
    await saveFontBtn.trigger('click')
    await flushPromises()

    // 轻提示挂在 body 上（失败与成功都只报一次，不再在表单下面挂返绿字横幅）。
    expect(document.body.textContent).toContain('已保存自定义终端字体：JetBrains Mono')
    expect(terminalStore.fontFamily).toBe('JetBrains Mono')
    expect(terminalStore.effectiveFontFamily).toBe('"JetBrains Mono", var(--font-mono)')

    // Reset font
    const resetFontBtn = wrapper.find('[data-test="reset-font-btn"]')
    await resetFontBtn.trigger('click')
    await flushPromises()

    expect(document.body.textContent).toContain('已恢复平台统一内置字体')
    expect(terminalStore.fontFamily).toBe('')
    expect(terminalStore.effectiveFontFamily).toBe('var(--font-mono)')
  })
})
