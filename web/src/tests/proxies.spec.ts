import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import ProxiesView from '../features/proxies/ProxiesView.vue'
import ProxyEditDialog from '../features/proxies/ProxyEditDialog.vue'
import ProxyDirectoryPicker from '../features/proxies/ProxyDirectoryPicker.vue'
import ProxySettings from '../features/settings/ProxySettings.vue'
import * as proxiesApi from '../api/proxies'
import { useAuthStore } from '../stores/auth'
import { useProjectsStore } from '../stores/projects'
import { installTestCSRF } from './csrf-fixture'

describe('Service Proxies UI Components', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useAuthStore().$patch({ authenticated: true })
    installTestCSRF('csrf-token')
    vi.restoreAllMocks()
  })

  it('renders proxy list and supports toggling and deleting', async () => {
    const mockList: proxiesApi.ServiceProxy[] = [
      {
        id: 1,
        name: 'Web Demo',
        alias: 'demo1',
        type: 'directory',
        target: '/data/projects/demo/dist',
        enabled: true,
        createdAt: '2026-10-28T10:00:00Z',
        updatedAt: '2026-10-28T10:00:00Z',
      },
      {
        id: 2,
        name: 'Vite Service',
        alias: 'vite-app',
        type: 'port',
        target: '5173',
        enabled: false,
        online: false,
        createdAt: '2026-10-28T10:00:00Z',
        updatedAt: '2026-10-28T10:00:00Z',
      },
    ]

    vi.spyOn(proxiesApi, 'listProxies').mockResolvedValue(mockList)
    const toggleSpy = vi.spyOn(proxiesApi, 'toggleProxy').mockResolvedValue({
      ...mockList[0],
      enabled: false,
    })
    const deleteSpy = vi.spyOn(proxiesApi, 'deleteProxy').mockResolvedValue()
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(ProxiesView)
    await flushPromises()

    // Header & list
    expect(wrapper.text()).toContain('服务代理与 Demo 预览')
    expect(wrapper.text()).toContain('Web Demo')
    expect(wrapper.text()).toContain('Vite Service')
    expect(wrapper.text()).toContain('/proxy/demo1/')
    expect(wrapper.text()).toContain('/proxy/vite-app/')

    // Toggle
    const checkbox = wrapper.find('input[type="checkbox"]')
    await checkbox.trigger('change')
    expect(toggleSpy).toHaveBeenCalledWith(1)

    // Delete
    const deleteBtn = wrapper.find('button.text-red-500')
    await deleteBtn.trigger('click')
    expect(deleteSpy).toHaveBeenCalledWith(1)
  })

  it('ProxySettings renders correctly in Settings area', async () => {
    vi.spyOn(proxiesApi, 'listProxies').mockResolvedValue([])
    const wrapper = mount(ProxySettings)
    await flushPromises()
    expect(wrapper.text()).toContain('服务代理')
    expect(wrapper.text()).toContain('ContextPath Proxy')
    expect(wrapper.text()).toContain('暂无服务代理规则')
  })

  it('ProxyEditDialog supports project selection and directory appending', async () => {
    const projectsStore = useProjectsStore()
    projectsStore.projects = [
      {
        id: 1,
        name: 'MyWeb',
        path: '/data/projects/my-web',
      },
    ]

    const wrapper = mount(ProxyEditDialog)
    await flushPromises()

    // Select project from dropdown
    const select = wrapper.find('#dlg-proxy-project')
    expect(select.exists()).toBe(true)
    await select.setValue('1')
    await select.trigger('change')
    await flushPromises()

    // Auto-filled target, name, alias
    expect((wrapper.find('#dlg-proxy-target').element as HTMLInputElement).value).toBe('/data/projects/my-web')
    expect((wrapper.find('#dlg-proxy-name').element as HTMLInputElement).value).toBe('MyWeb 预览')

    // Click append subdir "+ /dist"
    const distBtn = wrapper.findAll('button').find((b) => b.text().includes('+ /dist'))
    expect(distBtn).toBeDefined()
    await distBtn?.trigger('click')
    expect((wrapper.find('#dlg-proxy-target').element as HTMLInputElement).value).toBe('/data/projects/my-web/dist')
  })

  it('ProxyDirectoryPicker lists directories and emits selection', async () => {
    vi.spyOn(proxiesApi, 'browseDirectories').mockResolvedValue({
      current: '/data/projects',
      parent: '/data',
      entries: [
        { name: 'appA', path: '/data/projects/appA' },
        { name: 'appB', path: '/data/projects/appB' },
      ],
    })

    const wrapper = mount(ProxyDirectoryPicker, {
      props: { initialPath: '/data/projects' },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('/data/projects')
    expect(wrapper.text()).toContain('appA')
    expect(wrapper.text()).toContain('appB')

    // Click entry to select
    const items = wrapper.findAll('li')
    await items[0].trigger('click')

    // Confirm selection
    const confirmBtn = wrapper.findAll('button').find((b) => b.text().includes('选择此目录'))
    await confirmBtn?.trigger('click')

    expect(wrapper.emitted('select')).toBeTruthy()
    expect(wrapper.emitted('select')?.[0]).toEqual(['/data/projects/appA'])
  })

  it('ProxyEditDialog validates inputs and creates proxy', async () => {
    const createSpy = vi.spyOn(proxiesApi, 'createProxy').mockResolvedValue({
      id: 3,
      name: 'New Proxy',
      alias: 'new-proxy',
      type: 'port',
      target: '3000',
      enabled: true,
      online: true,
      createdAt: '2026-10-28T10:00:00Z',
      updatedAt: '2026-10-28T10:00:00Z',
    })

    const wrapper = mount(ProxyEditDialog)
    await flushPromises()

    // 1. Submit empty -> error
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('请输入代理名称')
    expect(createSpy).not.toHaveBeenCalled()

    // 2. Fill invalid alias
    await wrapper.find('#dlg-proxy-name').setValue('New Proxy')
    await wrapper.find('#dlg-proxy-alias').setValue('INVALID_ALIAS!')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('ContextPath 别名仅支持小写字母')

    // 3. Fill valid alias, select port type, invalid port
    await wrapper.find('#dlg-proxy-alias').setValue('new-proxy')
    await wrapper.find('#dlg-proxy-type').setValue('port')
    await wrapper.find('#dlg-proxy-target').setValue('80') // invalid < 1024
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('端口号必须在 1024 ~ 65535 范围内')

    // 4. Fill valid port and submit
    await wrapper.find('#dlg-proxy-target').setValue('3000')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith({
      name: 'New Proxy',
      alias: 'new-proxy',
      type: 'port',
      target: '3000',
      enabled: true,
    })
    expect(wrapper.emitted('saved')).toBeTruthy()
  })
})
