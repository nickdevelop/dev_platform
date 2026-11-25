import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ProxyEditDialog from '../features/proxies/ProxyEditDialog.vue'
import ProxyDirectoryPicker from '../features/proxies/ProxyDirectoryPicker.vue'
import ProxySettings from '../features/settings/ProxySettings.vue'
import * as proxiesApi from '../api/proxies'
import { useAuthStore } from '../stores/auth'
import { useProjectsStore } from '../stores/projects'
import { installTestCSRF } from './csrf-fixture'

// Element Plus 的下拉浮层与 ElMessageBox 都挂在 document 上，不在组件树里；
// jsdom 下 `<transition-group>` 又是 stub（校验文案节点不会渲染出来），所以断言
// 落在库的稳定结构类 `.is-error` 上——它表达的正是「这一格没过校验」。
const inBodyAll = (selector: string) => Array.from(document.querySelectorAll<HTMLElement>(selector))
const textOf = (el: Element | null | undefined) => (el?.textContent ?? '').trim()
// 校验状态用库的稳定结构类 `.is-error` 断言：文案节点在 jsdom 下被
// transition-group stub 掉了，而「这一格没过校验」本身就是被断言的事实。
// 弹窗内容渲染在组件树里（Teleport 只在 attachTo 时生效），所以从 wrapper 查。
const labelsWithError = (wrapper: VueWrapper) =>
  wrapper
    .findAll('.el-form-item.is-error .el-form-item__label')
    .map((label) => label.text())

const proxyRow = (id = 1): proxiesApi.ServiceProxy => ({
  id,
  name: 'Web Demo',
  alias: 'demo1',
  type: 'directory',
  target: '/data/projects/demo/dist',
  enabled: true,
  createdAt: '2026-10-28T10:00:00Z',
  updatedAt: '2026-10-28T10:00:00Z',
})

describe('Service Proxies UI Components', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useAuthStore().$patch({ authenticated: true })
    installTestCSRF('csrf-token')
    vi.restoreAllMocks()
  })


  it('ProxySettings renders correctly in Settings area', async () => {
    vi.spyOn(proxiesApi, 'listProxies').mockResolvedValue([])
    const wrapper = mount(ProxySettings)
    await flushPromises()
    expect(wrapper.text()).toContain('服务代理')
    expect(wrapper.text()).toContain('暂无服务代理规则')
  })

  it('ProxySettings toggles a proxy through the row switch', async () => {
    vi.spyOn(proxiesApi, 'listProxies').mockResolvedValue([proxyRow(1)])
    const toggleSpy = vi.spyOn(proxiesApi, 'toggleProxy').mockResolvedValue({
      ...proxyRow(1),
      enabled: false,
    })

    const wrapper = mount(ProxySettings)
    await flushPromises()

    const toggle = wrapper.find('[data-test="proxy-enabled-1"] input')
    expect(toggle.exists()).toBe(true)
    await toggle.setValue(false)
    await flushPromises()

    expect(toggleSpy).toHaveBeenCalledWith(1)
  })

  it('ProxySettings deletes through the shared confirmation box', async () => {
    vi.spyOn(proxiesApi, 'listProxies').mockResolvedValue([proxyRow(7)])
    const deleteSpy = vi.spyOn(proxiesApi, 'deleteProxy').mockResolvedValue()

    const wrapper = mount(ProxySettings)
    await flushPromises()

    await wrapper.find('[data-test="btn-delete-proxy-7"]').trigger('click')
    await flushPromises()

    // ElMessageBox 挂在 document 上，不在组件树里。
    expect(document.body.textContent).toContain('确定要删除代理「Web Demo」吗？')
    const confirm = inBodyAll('.el-message-box__btns button').find((b) => textOf(b) === '删除')
    expect(confirm).toBeDefined()
    confirm?.click()
    await flushPromises()

    expect(deleteSpy).toHaveBeenCalledWith(7)
  })

  it('ProxyEditDialog supports project selection and directory appending', async () => {
    useProjectsStore().projects = [
      {
        id: 1,
        name: 'MyWeb',
        path: '/data/projects/my-web',
        isGit: false,
      } as never,
    ]

    const wrapper = mount(ProxyEditDialog)
    await flushPromises()

    // 选中项目：el-select 监听的是点开浮层再点选项，不是原生 select 的 change。
    await wrapper.find('.el-select__wrapper').trigger('click')
    await flushPromises()
    const option = inBodyAll('.el-select-dropdown__item')[0]
    expect(option).toBeDefined()
    option?.click()
    await flushPromises()

    // 自动带出目标目录、名称与别名
    expect((wrapper.find('[data-test="input-proxy-target"]').element as HTMLInputElement).value).toBe(
      '/data/projects/my-web',
    )
    expect((wrapper.find('[data-test="input-proxy-name"]').element as HTMLInputElement).value).toBe(
      'MyWeb 预览',
    )

    // 追加常用构建子目录 /dist
    await wrapper.find('[data-test="append-subdir-dist"]').trigger('click')
    expect((wrapper.find('[data-test="input-proxy-target"]').element as HTMLInputElement).value).toBe(
      '/data/projects/my-web/dist',
    )
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
      attachTo: document.body,
    })
    await flushPromises()

    expect(document.body.textContent).toContain('/data/projects')
    expect(document.body.textContent).toContain('appA')
    expect(document.body.textContent).toContain('appB')

    const rows = wrapper.findAll('.dir-row')
    expect(rows).toHaveLength(2)
    await rows[0].trigger('click')

    await wrapper.find('[data-test="dir-picker-confirm"]').trigger('click')
    await flushPromises()

    expect(wrapper.emitted('select')).toBeTruthy()
    expect(wrapper.emitted('select')?.[0]).toEqual(['/data/projects/appA'])
  })

  it('ProxyEditDialog validates inputs and creates proxy', async () => {
    const createSpy = vi.spyOn(proxiesApi, 'createProxy').mockResolvedValue({
      ...proxyRow(3),
      name: 'New Proxy',
      alias: 'new-proxy',
      type: 'port',
      target: '3000',
      enabled: true,
      online: true,
    })

    const wrapper = mount(ProxyEditDialog)
    await flushPromises()

    // 1. 空表单直接确认：字段级校验拦下，不发请求。
    await wrapper.find('[data-test="btn-save-proxy"]').trigger('click')
    await flushPromises()
    expect(labelsWithError(wrapper)).toContain('代理名称')
    expect(createSpy).not.toHaveBeenCalled()

    // 2. 非法别名
    await wrapper.find('[data-test="input-proxy-name"]').setValue('New Proxy')
    await wrapper.find('[data-test="input-proxy-alias"]').setValue('INVALID_ALIAS!')
    await wrapper.find('[data-test="btn-save-proxy"]').trigger('click')
    await flushPromises()
    expect(labelsWithError(wrapper)).toContain('ContextPath 别名')
    expect(createSpy).not.toHaveBeenCalled()

    // 3. 合法别名 + 端口类型 + 非法端口
    await wrapper.find('[data-test="input-proxy-alias"]').setValue('new-proxy')
    const portRadio = wrapper.findAll('.el-radio').find((r) => r.text().includes('端口反向代理'))
    await portRadio?.find('input').setValue(true)
    await flushPromises()
    await wrapper.find('[data-test="input-proxy-target"]').setValue('80')
    await wrapper.find('[data-test="btn-save-proxy"]').trigger('click')
    await flushPromises()
    expect(labelsWithError(wrapper)).toContain('本地监听端口号')
    expect(createSpy).not.toHaveBeenCalled()

    // 4. 合法端口，提交
    await wrapper.find('[data-test="input-proxy-target"]').setValue('3000')
    await wrapper.find('[data-test="btn-save-proxy"]').trigger('click')
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
