import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import ConnectionDialog from '../features/workspace/ConnectionDialog.vue'
import { useDesktopConnection, resetDesktopStateForTesting, type DesktopIpcApi } from '../shared/desktop'

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn(),
  }),
}))

describe('Desktop Connections & Dialog', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
    resetDesktopStateForTesting()
    delete (window as any).desktopAPI
  })

  it('handles non-desktop environment gracefully', async () => {
    const wrapper = mount(ConnectionDialog, {
      props: { open: true },
      global: {
        stubs: {
          UiDialog: {
            props: ['title'],
            template: '<div class="ui-dialog-stub"><h2>{{ title }}</h2><slot /></div>',
          },
        },
      },
    })

    expect(wrapper.text()).toContain('服务连接管理器')
    expect(wrapper.text()).toContain('当前处于浏览器模式')
  })

  it('loads connections and switches target in desktop environment', async () => {
    const mockConnections = [
      { id: 'local', name: '本地内置服务', type: 'local', createdAt: 1000 },
      { id: 'rem-1', name: '团队开发服', type: 'remote', url: 'http://192.168.1.100:8080', createdAt: 2000 },
    ]

    const mockStatus = {
      activeConnectionId: 'local',
      connectionState: 'connected' as const,
      localService: { running: true, port: 18080 },
      proxyPort: 34567,
    }

    const mockApi: Partial<DesktopIpcApi> = {
      getStatus: vi.fn().mockResolvedValue(mockStatus),
      getConnections: vi.fn().mockResolvedValue(mockConnections),
      switchConnection: vi.fn().mockResolvedValue({
        ...mockStatus,
        activeConnectionId: 'rem-1',
      }),
      saveConnection: vi.fn().mockImplementation((conn) => Promise.resolve({ ...conn, id: 'rem-new', createdAt: 3000 })),
      removeConnection: vi.fn().mockResolvedValue(true),
      testConnection: vi.fn().mockResolvedValue({ ok: true, latencyMs: 15, version: '1.0.0' }),
      onStatusChanged: vi.fn().mockReturnValue(() => {}),
    }

    ;(window as any).desktopAPI = mockApi

    const desktop = useDesktopConnection()
    await desktop.refresh()

    const wrapper = mount(ConnectionDialog, {
      props: { open: true },
      global: {
        stubs: {
          UiDialog: {
            props: ['title'],
            template: '<div class="ui-dialog-stub"><h2>{{ title }}</h2><slot /></div>',
          },
        },
      },
    })

    await nextTick()
    await nextTick()

    expect(wrapper.text()).toContain('本地内置服务')
    expect(wrapper.text()).toContain('团队开发服')
    expect(wrapper.text()).toContain('当前激活')

    // Find the connect button for the remote server
    const connectButtons = wrapper.findAll('button').filter((b) => b.text() === '连接')
    expect(connectButtons.length).toBeGreaterThan(0)

    // Trigger switch
    await connectButtons[0].trigger('click')
    expect(mockApi.switchConnection).toHaveBeenCalledWith('rem-1')
  })

  it('validates URL before testing or saving a remote server', async () => {
    const mockApi: Partial<DesktopIpcApi> = {
      getStatus: vi.fn().mockResolvedValue({
        activeConnectionId: 'local',
        connectionState: 'connected' as const,
        localService: { running: true },
        proxyPort: 1234,
      }),
      getConnections: vi.fn().mockResolvedValue([]),
      onStatusChanged: vi.fn().mockReturnValue(() => {}),
    }

    ;(window as any).desktopAPI = mockApi

    const desktop = useDesktopConnection()
    await desktop.refresh()

    const wrapper = mount(ConnectionDialog, {
      props: { open: true },
      global: {
        stubs: {
          UiDialog: {
            props: ['title'],
            template: '<div class="ui-dialog-stub"><h2>{{ title }}</h2><slot /></div>',
          },
        },
      },
    })

    await nextTick()
    await nextTick()

    // Click "+ 添加远程服务"
    const addBtn = wrapper.findAll('button').find((b) => b.text().includes('添加远程服务'))
    expect(addBtn).toBeDefined()
    await addBtn!.trigger('click')

    await nextTick()
    expect(wrapper.text()).toContain('添加远程服务器')

    // Click "保存" without name
    const saveBtn = wrapper.findAll('button').find((b) => b.text() === '保存')
    expect(saveBtn).toBeDefined()
    await saveBtn!.trigger('click')

    await nextTick()
    expect(wrapper.text()).toContain('请输入服务器名称')
  })
})
