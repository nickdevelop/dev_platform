import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import QuickFileSearchDialog from '../features/chat/pane/QuickFileSearchDialog.vue'
import {
  EXT_FILTER_OPTIONS,
  extSelectionLabel,
  normalizeCustomExt,
  resolveExtFilter,
} from '../features/chat/pane/quickSearchExt'
import { highlightMatch, clipAroundQuery } from '../features/chat/pane/quickSearchHighlight'
import * as projectsApi from '../api/projects'

describe('resolveExtFilter', () => {
  it('maps preset groups onto their extensions', () => {
    expect(resolveExtFilter('Go')).toEqual(['.go'])
    expect(resolveExtFilter('前端 JS/TS')).toEqual(['.js', '.jsx', '.ts', '.tsx', '.vue'])
  })

  it('normalizes manually typed tokens', () => {
    expect(resolveExtFilter('*.kt')).toEqual(['.kt'])
    expect(resolveExtFilter('kt,json')).toEqual(['.kt', '.json'])
    expect(resolveExtFilter('*.py; *.sql')).toEqual(['.py', '.sql'])
    expect(resolveExtFilter('PY')).toEqual(['.py'])
  })

  it('returns undefined for empty or junk selections', () => {
    expect(resolveExtFilter('')).toBeUndefined()
    expect(resolveExtFilter('  ')).toBeUndefined()
    expect(resolveExtFilter('*.')).toBeUndefined()
  })

  it('offers 全部后缀 plus the preset groups', () => {
    expect(EXT_FILTER_OPTIONS[0]).toEqual({ value: '', label: '全部后缀 (*.*)' })
    const labels = EXT_FILTER_OPTIONS.map((o) => o.value)
    for (const expected of ['Java', 'Go', 'Python', 'SQL', '前端 JS/TS', 'Shell 脚本']) {
      expect(labels).toContain(expected)
    }
  })
})

describe('normalizeCustomExt', () => {
  it('accepts the raw shapes the mockup allows', () => {
    expect(normalizeCustomExt('vue')).toBe('.vue')
    expect(normalizeCustomExt('.vue')).toBe('.vue')
    expect(normalizeCustomExt('*.Vue')).toBe('.vue')
    expect(normalizeCustomExt('kt,json')).toBe('.kt,.json')
  })

  it('treats wildcards as the unfiltered selection', () => {
    expect(normalizeCustomExt('*')).toBe('')
    expect(normalizeCustomExt('*.*')).toBe('')
    expect(normalizeCustomExt('all')).toBe('')
  })

  it('rejects junk', () => {
    expect(normalizeCustomExt('')).toBeUndefined()
    expect(normalizeCustomExt('*.')).toBeUndefined()
    expect(normalizeCustomExt('<script>')).toBeUndefined()
  })
})

describe('extSelectionLabel', () => {
  it('renders compact chip labels', () => {
    expect(extSelectionLabel('')).toBe('*.*')
    expect(extSelectionLabel('Java')).toBe('*.java')
    expect(extSelectionLabel('前端 JS/TS')).toBe('前端 JS/TS')
    expect(extSelectionLabel('.kt')).toBe('*.kt')
    expect(extSelectionLabel('.kt,.json')).toBe('*.kt +1')
  })
})

describe('highlightMatch', () => {
  it('escapes markup before inserting hit spans', () => {
    expect(highlightMatch('<b>x</b>', 'x')).toBe('&lt;b&gt;<span class="qs-hit">x</span>&lt;/b&gt;')
  })

  it('wraps case-insensitive matches and stays literal on regex chars', () => {
    expect(highlightMatch('Foo(bar', 'foo')).toBe('<span class="qs-hit">Foo</span>(bar')
    expect(highlightMatch('call(1)', '(')).toBe('call<span class="qs-hit">(</span>1)')
  })

  it('returns escaped text without a query', () => {
    expect(highlightMatch('a&b', '  ')).toBe('a&amp;b')
  })
})

describe('clipAroundQuery', () => {
  it('centers the window on the hit and adds ellipses on clipped ends', () => {
    const line = `logger.info("before context") ${'x'.repeat(80)} validateToken signature after${'y'.repeat(60)}`
    const clipped = clipAroundQuery(line, 'validateToken', 20)
    expect(clipped).toContain('validateToken')
    expect(clipped.startsWith('…')).toBe(true)
    expect(clipped.endsWith('…')).toBe(true)
    expect(clipped.length).toBeLessThan(line.length)
  })

  it('leaves short lines and misses head-aligned', () => {
    expect(clipAroundQuery('short line', 'short')).toBe('short line')
    expect(clipAroundQuery('short line', 'zzz')).toBe('short line')
    const long = 'a'.repeat(300)
    const clipped = clipAroundQuery(long, 'zzz')
    expect(clipped.endsWith('…')).toBe(true)
    expect(clipped.startsWith('…')).toBe(false)
  })
})

const mounted: ReturnType<typeof mount>[] = []

function mountDialog(props: { open: boolean; projectId: number; worktreeId?: number }) {
  const wrapper = mount(QuickFileSearchDialog, { props, attachTo: document.body })
  mounted.push(wrapper as unknown as ReturnType<typeof mount>)
  return wrapper
}

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

const tick = () => new Promise((r) => setTimeout(r, 150))

describe('QuickFileSearchDialog', () => {
  it('does not render when open is false', () => {
    const wrapper = mountDialog({ open: false, projectId: 1 })
    expect(wrapper.find('[data-test="quick-file-search-dialog"]').exists()).toBe(false)
  })

  it('opens in file mode with the search bar and lists files', async () => {
    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValueOnce({
      files: [
        { path: 'src/main.go', isDir: false },
        { path: 'src/components', isDir: true },
      ],
    })
    const wrapper = mountDialog({ open: true, projectId: 1 })
    await flushPromises()

    expect(wrapper.find('[data-test="quick-file-search-dialog"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="quick-search-bar"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="quick-search-type-select"]').text()).toContain('文件名')

    const items = wrapper.findAll('[data-test="quick-search-item"]')
    expect(items.length).toBe(1)
    // 后缀 chip + 高亮文件名 + 路径行
    expect(items[0].text()).toContain('.go')
    expect(items[0].text()).toContain('main.go')
    expect(items[0].find('.qs-hit').exists()).toBe(false)

    // 文件模式没有后缀过滤
    expect(wrapper.find('[data-test="quick-search-ext-btn"]').exists()).toBe(false)
    // 状态条给出模式提示
    expect(wrapper.find('[data-test="quick-search-stats"]').text()).toBe('文件名搜索')
  })

  it('switches modes through the prefix dropdown', async () => {
    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValue({ files: [] })
    const wrapper = mountDialog({ open: true, projectId: 1 })
    await flushPromises()

    await wrapper.find('[data-test="quick-search-type-select"]').trigger('click')
    const menu = wrapper.find('[data-test="quick-search-mode-menu"]')
    expect(menu.exists()).toBe(true)

    await menu.find('[data-test="quick-search-mode-content"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="quick-search-type-select"]').text()).toContain('内容')
    // 内容模式出现后缀 chip，默认全部
    expect(wrapper.find('[data-test="quick-search-ext-btn"]').text()).toContain('*.*')
    expect(wrapper.find('[data-test="quick-search-stats"]').text()).toContain('范围 全部文件')
  })

  it('searches content, highlights matches and sends the active selection', async () => {
    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValue({ files: [] })
    const contentSpy = vi.spyOn(projectsApi, 'searchProjectContent').mockResolvedValue({
      matches: [{ path: 'src/main.ts', line: 42, content: 'const app = createApp(App)' }],
    })
    const wrapper = mountDialog({ open: true, projectId: 1 })
    await flushPromises()

    // Tab 切到内容模式
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }))
    await wrapper.vm.$nextTick()

    const input = wrapper.find<HTMLInputElement>('[data-test="quick-file-search-input"]')
    await input.setValue('createApp')
    await tick()
    await flushPromises()

    expect(contentSpy).toHaveBeenCalledWith(1, 'createApp', 50, undefined, undefined)

    const item = wrapper.find('[data-test="quick-search-content-item"]')
    expect(item.exists()).toBe(true)
    // 与文件行一致：文件名后跟 :行号，第二行是命中摘要
    expect(item.text()).toContain('main.ts:42')
    // 命中词高亮
    expect(item.find('.qs-hit').text()).toBe('createApp')

    expect(wrapper.find('[data-test="quick-search-stats"]').text()).toBe('共 1 条匹配结果')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('select')?.[0]).toEqual(['src/main.ts', 42])
  })

  it('filters by preset group, appends custom extensions and retries with all', async () => {
    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValue({ files: [] })
    const contentSpy = vi.spyOn(projectsApi, 'searchProjectContent').mockResolvedValue({ matches: [] })
    const wrapper = mountDialog({ open: true, projectId: 1 })
    await flushPromises()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }))
    await wrapper.vm.$nextTick()
    const input = wrapper.find<HTMLInputElement>('[data-test="quick-file-search-input"]')
    await input.setValue('handler')
    await tick()
    await flushPromises()

    // 预设组：Go
    await wrapper.find('[data-test="quick-search-ext-btn"]').trigger('click')
    await wrapper.find('[data-test="quick-search-ext-option-Go"]').trigger('click')
    await tick()
    await flushPromises()
    expect(contentSpy).toHaveBeenLastCalledWith(1, 'handler', 50, undefined, ['.go'])
    expect(wrapper.find('[data-test="quick-search-ext-btn"]').text()).toContain('*.go')

    // 自定义后缀：下拉面板顶部的输入框 + 确定
    await wrapper.find('[data-test="quick-search-ext-btn"]').trigger('click')
    const extInput = wrapper.find<HTMLInputElement>('[data-test="quick-search-ext-input"]')
    await extInput.setValue('kt')
    await wrapper.find('[data-test="quick-search-ext-apply"]').trigger('click')
    await tick()
    await flushPromises()
    expect(contentSpy).toHaveBeenLastCalledWith(1, 'handler', 50, undefined, ['.kt'])
    expect(wrapper.find('[data-test="quick-search-ext-btn"]').text()).toContain('*.kt')
    // 自定义后缀追加进了下拉列表
    await wrapper.find('[data-test="quick-search-ext-btn"]').trigger('click')
    expect(
      wrapper.find('[data-test="quick-search-ext-menu"]').text(),
    ).toContain('自定义 (*.kt)')

    // 无结果 + 有过滤 → 「切换到全部后缀重试」
    expect(wrapper.find('[data-test="quick-search-ext-retry-all"]').exists()).toBe(true)
    await wrapper.find('[data-test="quick-search-ext-retry-all"]').trigger('click')
    await tick()
    await flushPromises()
    expect(contentSpy).toHaveBeenLastCalledWith(1, 'handler', 50, undefined, undefined)
    expect(wrapper.find('[data-test="quick-search-ext-btn"]').text()).toContain('*.*')
  })

  it('uses the typed custom option when no preset matches the draft', async () => {
    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValue({ files: [] })
    const contentSpy = vi.spyOn(projectsApi, 'searchProjectContent').mockResolvedValue({ matches: [] })
    const wrapper = mountDialog({ open: true, projectId: 1 })
    await flushPromises()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }))
    await wrapper.vm.$nextTick()
    const input = wrapper.find<HTMLInputElement>('[data-test="quick-file-search-input"]')
    await input.setValue('value')
    await tick()
    await flushPromises()

    await wrapper.find('[data-test="quick-search-ext-btn"]').trigger('click')
    const extInput = wrapper.find<HTMLInputElement>('[data-test="quick-search-ext-input"]')
    await extInput.setValue('wasm')
    await wrapper.vm.$nextTick()
    const useBtn = wrapper.find('[data-test="quick-search-ext-use-custom"]')
    expect(useBtn.exists()).toBe(true)
    expect(useBtn.text()).toContain('*.wasm')
    // Enter 在自定义输入框里直接应用，不触发结果打开
    await extInput.trigger('keydown.enter')
    await tick()
    await flushPromises()
    expect(contentSpy).toHaveBeenLastCalledWith(1, 'value', 50, undefined, ['.wasm'])
    expect(wrapper.emitted('select')).toBeUndefined()
  })

  it('navigates with ArrowDown/ArrowUp and selects with Enter', async () => {
    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValueOnce({
      files: [
        { path: 'src/first.ts', isDir: false },
        { path: 'src/second.ts', isDir: false },
      ],
    })
    const wrapper = mountDialog({ open: true, projectId: 1 })
    await flushPromises()

    let items = wrapper.findAll('[data-test="quick-search-item"]')
    expect(items[0].classes()).toContain('is-active')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }))
    await wrapper.vm.$nextTick()
    items = wrapper.findAll('[data-test="quick-search-item"]')
    expect(items[1].classes()).toContain('is-active')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('select')?.[0]).toEqual(['src/second.ts'])
  })

  it('Enter with an open dropdown only closes it; Escape closes dropdown then dialog', async () => {
    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValue({
      files: [{ path: 'src/a.ts', isDir: false }],
    })
    const wrapper = mountDialog({ open: true, projectId: 1 })
    await flushPromises()

    await wrapper.find('[data-test="quick-search-type-select"]').trigger('click')
    expect(wrapper.find('[data-test="quick-search-mode-menu"]').exists()).toBe(true)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-test="quick-search-mode-menu"]').exists()).toBe(false)
    expect(wrapper.emitted('select')).toBeUndefined()

    // 重新打开下拉：Esc 先收下拉，再按一次才关弹窗
    await wrapper.find('[data-test="quick-search-type-select"]').trigger('click')
    expect(wrapper.find('[data-test="quick-search-mode-menu"]').exists()).toBe(true)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-test="quick-search-mode-menu"]').exists()).toBe(false)
    expect(wrapper.emitted('close')).toBeUndefined()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')?.length).toBe(1)

    await wrapper.find('.quick-search-backdrop').trigger('click')
    expect(wrapper.emitted('close')?.length).toBe(2)
  })

  it('clear button empties the query and is hidden when there is nothing to clear', async () => {
    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValue({ files: [] })
    const wrapper = mountDialog({ open: true, projectId: 1 })
    await flushPromises()
    expect(wrapper.find('[data-test="quick-search-clear-btn"]').exists()).toBe(false)

    const input = wrapper.find<HTMLInputElement>('[data-test="quick-file-search-input"]')
    await input.setValue('abc')
    expect(wrapper.find('[data-test="quick-search-clear-btn"]').exists()).toBe(true)

    await wrapper.find('[data-test="quick-search-clear-btn"]').trigger('click')
    expect(input.element.value).toBe('')
    expect(wrapper.find('[data-test="quick-search-clear-btn"]').exists()).toBe(false)
  })
})
