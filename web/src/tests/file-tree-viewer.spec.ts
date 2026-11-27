import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useFileTreeStateStore } from '../stores/fileTreeState'
import { useProjectTerminalStore } from '../stores/projectTerminal'
import { getDirectory, isExternalUrl, resolveRelativePath } from '../features/files/pathUtils'
import ContextMenu from '../shared/components/ContextMenu.vue'
import type { ContextMenuAnchor, ContextMenuItem } from '../shared/components/contextMenu'
import FileTree from '../features/files/FileTree.vue'
import FileSidebar from '../features/files/FileSidebar.vue'
import CodeView from '../features/files/CodeView.vue'
import ArtifactToolbar from '../features/files/ArtifactToolbar.vue'
import MarkdownArtifactView from '../features/files/MarkdownArtifactView.vue'
import ArtifactImageView from '../features/files/ArtifactImageView.vue'
import HtmlArtifactView from '../features/files/HtmlArtifactView.vue'
import HtmlPreviewModal from '../features/files/HtmlPreviewModal.vue'
import FileViewer from '../features/files/FileViewer.vue'
import * as filesApi from '../api/files'
import type { FileTreeNode, GitStatusItem } from '../features/files/types'

// The context menu teleports to `body`, so every test reaches it through the
// document rather than through the wrapper.
function menuItems(menu: Element): HTMLButtonElement[] {
  return [...menu.querySelectorAll<HTMLButtonElement>('[data-test="context-menu-item"]')]
}

function menuLabels(menu: Element): string[] {
  return menuItems(menu).map((item) => item.textContent?.trim() ?? '')
}

describe('pathUtils', () => {
  it('extracts base directory correctly', () => {
    expect(getDirectory('src/components/Header.vue')).toBe('src/components')
    expect(getDirectory('README.md')).toBe('')
    expect(getDirectory('a/b/c/d.txt')).toBe('a/b/c')
  })

  it('resolves relative paths safely without escapes', () => {
    expect(resolveRelativePath('docs/guide', './intro.md')).toBe('docs/guide/intro.md')
    expect(resolveRelativePath('docs/guide', '../api.md')).toBe('docs/api.md')
    expect(resolveRelativePath('docs/guide', '../../root.md')).toBe('root.md')
    expect(resolveRelativePath('docs/guide', '/absolute/path.png')).toBe('absolute/path.png')
    expect(resolveRelativePath('', 'foo.txt')).toBe('foo.txt')
  })

  it('detects external URLs vs internal workspace paths', () => {
    expect(isExternalUrl('https://example.com/logo.png')).toBe(true)
    expect(isExternalUrl('http://github.com')).toBe(true)
    expect(isExternalUrl('mailto:test@test.com')).toBe(true)
    expect(isExternalUrl('#section-1')).toBe(true)
    expect(isExternalUrl('data:image/png;base64,abc')).toBe(true)
    expect(isExternalUrl('./assets/logo.png')).toBe(false)
    expect(isExternalUrl('../readme.md')).toBe(false)
    expect(isExternalUrl('src/main.ts')).toBe(false)
  })
})

describe('FileTree.vue', () => {
  const sampleNodes: FileTreeNode[] = [
    {
      name: 'src',
      path: 'src',
      isDir: true,
      children: [
        { name: 'main.ts', path: 'src/main.ts', isDir: false },
        { name: 'styles.css', path: 'src/styles.css', isDir: false },
      ],
    },
    {
      name: 'README.md',
      path: 'README.md',
      isDir: false,
    },
  ]

  it('renders root nodes and handles folder expand/collapse', async () => {
    const wrapper = mount(FileTree, {
      props: {
        nodes: sampleNodes,
      },
    })

    expect(wrapper.text()).toContain('src')
    expect(wrapper.text()).toContain('README.md')
    // Initially folder is collapsed, children are not mounted
    expect(wrapper.text()).not.toContain('main.ts')

    // Click src directory row to expand
    const dirRow = wrapper.findAll('.file-tree-row').find((r) => r.text().includes('src'))
    await dirRow?.trigger('click')

    expect(wrapper.text()).toContain('main.ts')
    expect(wrapper.text()).toContain('styles.css')
  })

  it('emits select when clicking a file node', async () => {
    const wrapper = mount(FileTree, {
      props: {
        nodes: sampleNodes,
      },
    })

    const fileRow = wrapper.findAll('.file-tree-row').find((r) => r.text().includes('README.md'))
    await fileRow?.trigger('click')

    expect(wrapper.emitted('select')?.[0]).toEqual(['README.md'])
  })

  it('displays git badges correctly', () => {
    const gitMap = new Map([
      ['README.md', 'modified' as const],
      ['src/main.ts', 'added' as const],
    ])

    const wrapper = mount(FileTree, {
      props: {
        nodes: sampleNodes,
        gitChangesMap: gitMap,
      },
    })

    const badge = wrapper.find('.git-badge')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toBe('M')
    expect(badge.classes()).toContain('git-modified')
  })

  it('triggers loadChildren event when expanding an unloaded directory', async () => {
    const lazyNodes: FileTreeNode[] = [
      {
        name: 'pkg',
        path: 'pkg',
        isDir: true,
      },
    ]

    const wrapper = mount(FileTree, {
      props: {
        nodes: lazyNodes,
      },
    })

    const dirRow = wrapper.findAll('.file-tree-row').find((r) => r.text().includes('pkg'))
    await dirRow?.trigger('click')

    expect(wrapper.emitted('loadChildren')?.[0]).toEqual([lazyNodes[0]])
  })

  it('shows loading state when directory node is marked as loading', () => {
    const loadingNodes: FileTreeNode[] = [
      {
        name: 'pkg',
        path: 'pkg',
        isDir: true,
        loading: true,
      },
    ]

    const wrapper = mount(FileTree, {
      props: {
        nodes: loadingNodes,
      },
    })

    // Click to expand
    ;(wrapper.vm as any).toggleExpand('pkg')

    return wrapper.vm.$nextTick(() => {
      expect(wrapper.find('[data-test="tree-node-loading"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('加载中…')
    })
  })

  it('has no per-row buttons and reports a right click as a menu target', async () => {
    const wrapper = mount(FileTree, {
      props: {
        nodes: sampleNodes,
      },
    })

    // The upload / download / delete buttons are gone from the rows.
    expect(wrapper.find('.file-node-actions').exists()).toBe(false)
    expect(wrapper.findAll('.file-tree-row button')).toHaveLength(0)

    const fileRow = wrapper.findAll('.file-tree-row').find((r) => r.text().includes('README.md'))
    await fileRow?.trigger('contextmenu', { clientX: 42, clientY: 77 })
    expect(wrapper.emitted('menu')?.[0]).toEqual([{ path: 'README.md', isDir: false, x: 42, y: 77 }])

    const dirRow = wrapper.findAll('.file-tree-row').find((r) => r.text().includes('src'))
    await dirRow?.trigger('contextmenu', { clientX: 8, clientY: 9 })
    expect(wrapper.emitted('menu')?.[1]).toEqual([{ path: 'src', isDir: true, x: 8, y: 9 }])
  })
})

describe('ContextMenu.vue', () => {
  // One item list per mount, with the spies a test needs to read back.
  function makeItems() {
    const download = vi.fn()
    const copyAbsolute = vi.fn()
    const remove = vi.fn()
    const items: ContextMenuItem[] = [
      { label: '下载', icon: 'download', onSelect: download },
      { label: '复制绝对路径', icon: 'copy', disabled: true, onSelect: copyAbsolute },
      { label: '删除文件', icon: 'trash', danger: true, divider: true, onSelect: remove },
    ]
    return { items, download, copyAbsolute, remove }
  }

  function mountMenu(anchor: ContextMenuAnchor | null, fixture = makeItems()) {
    const wrapper = mount(ContextMenu, { props: { anchor, items: fixture.items }, attachTo: document.body })
    return { wrapper, fixture }
  }

  function menuEl(): HTMLElement | null {
    return document.querySelector('[data-test="context-menu"]')
  }

  it('renders no menu without an anchor', () => {
    const { wrapper } = mountMenu(null)
    expect(menuEl()).toBeNull()
    wrapper.unmount()
  })

  it('renders the items at the anchor and runs the chosen action', async () => {
    const { wrapper, fixture } = mountMenu({ x: 120, y: 200 })
    await flushPromises()
    const menu = menuEl()
    expect(menu).toBeTruthy()
    expect(menu!.style.left).toBe('120px')
    expect(menu!.style.top).toBe('200px')

    const buttons = menuItems(menu!)
    expect(buttons.map((b) => b.textContent?.trim())).toEqual(['下载', '复制绝对路径', '删除文件'])
    expect(buttons[1].disabled).toBe(true)
    expect(buttons[2].classList.contains('is-danger')).toBe(true)
    expect(buttons[2].classList.contains('has-divider')).toBe(true)

    buttons[0].click()
    await flushPromises()
    expect(fixture.download).toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeTruthy()
    wrapper.unmount()
  })

  it('never runs a disabled action', async () => {
    const { wrapper, fixture } = mountMenu({ x: 120, y: 200 })
    await flushPromises()
    const menu = menuEl()!
    menuItems(menu)[1].click()
    await flushPromises()
    expect(fixture.copyAbsolute).not.toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeFalsy()
    wrapper.unmount()
  })

  it('pushes the menu back inside the viewport', async () => {
    const { wrapper } = mountMenu({ x: window.innerWidth - 2, y: window.innerHeight - 2 })
    await flushPromises()
    const menu = menuEl()!
    expect(parseFloat(menu.style.left)).toBeLessThanOrEqual(window.innerWidth - 8)
    expect(parseFloat(menu.style.top)).toBeLessThanOrEqual(window.innerHeight - 8)
    wrapper.unmount()
  })

  it('closes on Escape and on a press outside the menu', () => {
    const first = mountMenu({ x: 40, y: 40 })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(first.wrapper.emitted('close')).toBeTruthy()
    first.wrapper.unmount()

    const second = mountMenu({ x: 40, y: 40 })
    document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    expect(second.wrapper.emitted('close')).toBeTruthy()
    second.wrapper.unmount()
  })

  it('ignores a press inside the menu', () => {
    const { wrapper } = mountMenu({ x: 40, y: 40 })
    menuEl()!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    expect(wrapper.emitted('close')).toBeFalsy()
    wrapper.unmount()
  })
})

describe('FileSidebar.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('keeps a session\'s expansions and tab across a remount, and isolates sessions', async () => {
    vi.spyOn(filesApi, 'getFileTree').mockImplementation(async (_pid, path) => {
      if (!path) return [{ name: 'src', path: 'src', isDir: true }]
      return [{ name: 'index.ts', path: 'src/index.ts', isDir: false }]
    })
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitCommits').mockResolvedValue([])

    const first = mount(FileSidebar, { props: { projectId: 1, sessionId: 'ses-keep' } })
    await vi.waitFor(() => expect(first.text()).toContain('src'))
    await first.findAll('.file-tree-row').find((r) => r.text().includes('src'))!.trigger('click')
    await flushPromises()
    expect(first.text()).toContain('index.ts')
    await first.findAll('.sidebar-tab').find((b) => b.text().includes('变更'))!.trigger('click')
    first.unmount()

    const remounted = mount(FileSidebar, { props: { projectId: 1, sessionId: 'ses-keep' } })
    await flushPromises()
    expect(remounted.findAll('.sidebar-tab').find((b) => b.text().includes('变更'))!.classes()).toContain('is-active')
    await remounted.findAll('.sidebar-tab').find((b) => b.text().includes('所有文件'))!.trigger('click')
    await vi.waitFor(() => expect(remounted.text()).toContain('index.ts'))

    const other = mount(FileSidebar, { props: { projectId: 1, sessionId: 'ses-other' } })
    await vi.waitFor(() => expect(other.text()).toContain('src'))
    expect(other.text()).not.toContain('index.ts')
    expect(other.findAll('.sidebar-tab').find((b) => b.text().includes('变更'))!.classes()).not.toContain('is-active')
  })

  it('releases the remembered tree of a session whose tab is closed', () => {
    const store = useFileTreeStateStore()
    store.save('ses-a', {
      activeTab: 'all',
      searchQuery: '',
      selectedCommit: '',
      expandedTreePaths: ['src'],
      expandedChangeDirs: [],
      isStagedGroupExpanded: true,
      isUnstagedGroupExpanded: true,
    })
    store.save('ses-b', { ...store.snapshotOf('ses-a')!, expandedTreePaths: [] })
    store.retainOnly(['ses-b'])
    expect(store.snapshotOf('ses-a')).toBeUndefined()
    expect(store.snapshotOf('ses-b')).toBeDefined()
  })

  it('loads children on demand when a directory is expanded in lazy loading mode', async () => {
    const getFileTreeSpy = vi.spyOn(filesApi, 'getFileTree').mockImplementation(async (_pid, path) => {
      if (!path) {
        return [
          { name: 'src', path: 'src', isDir: true },
          { name: 'README.md', path: 'README.md', isDir: false },
        ]
      }
      if (path === 'src') {
        return [
          { name: 'index.ts', path: 'src/index.ts', isDir: false },
        ]
      }
      return []
    })
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1 },
    })

    await vi.waitFor(() => expect(wrapper.text()).toContain('src'))
    expect(wrapper.text()).not.toContain('index.ts')

    // Find the FileTree component and emit loadChildren
    const tree = wrapper.findComponent(FileTree)
    expect(tree.exists()).toBe(true)

    // Trigger expand on src
    const srcRow = wrapper.findAll('.file-tree-row').find((r) => r.text().includes('src'))
    await srcRow?.trigger('click')
    await flushPromises()

    expect(getFileTreeSpy).toHaveBeenCalledWith(1, 'src', undefined)
    expect(wrapper.text()).toContain('index.ts')
  })

  it('loads multi-level children on demand when nested directory is expanded', async () => {
    const getFileTreeSpy = vi.spyOn(filesApi, 'getFileTree').mockImplementation(async (_pid, path) => {
      if (!path) {
        return [
          { name: 'src', path: 'src', isDir: true },
        ]
      }
      if (path === 'src') {
        return [
          { name: 'utils', path: 'src/utils', isDir: true },
        ]
      }
      if (path === 'src/utils') {
        return [
          { name: 'helper.ts', path: 'src/utils/helper.ts', isDir: false },
        ]
      }
      return []
    })
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1 },
    })

    await vi.waitFor(() => expect(wrapper.text()).toContain('src'))

    // Trigger expand on src (level 1)
    const srcRow = wrapper.findAll('.file-tree-row').find((r) => r.text().includes('src'))
    await srcRow?.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('utils')

    // Trigger expand on utils (level 2)
    const utilsRow = wrapper.findAll('.file-tree-row').find((r) => r.text().includes('utils'))
    await utilsRow?.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('helper.ts')
  })
  it('opens the file actions in a context menu, with no buttons in the rows', async () => {
    vi.spyOn(filesApi, 'getFileTree').mockResolvedValue([
      { name: 'src', path: 'src', isDir: true },
      { name: 'README.md', path: 'README.md', isDir: false },
    ])
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitCommits').mockResolvedValue([])

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1, directory: '/ws/demo' },
      attachTo: document.body,
    })
    await vi.waitFor(() => expect(wrapper.text()).toContain('README.md'))
    expect(wrapper.find('.file-node-actions').exists()).toBe(false)

    const row = wrapper.findAll('.file-tree-row').find((r) => r.text().includes('README.md'))!
    await row.trigger('contextmenu', { clientX: 30, clientY: 30 })
    const menu = document.querySelector('[data-test="context-menu"]')
    expect(menu).toBeTruthy()
    expect(menuLabels(menu!)).toEqual(['打开', '下载', '复制相对路径', '复制绝对路径', '删除文件'])

    // 打开 is what the left click always did.
    menuItems(menu!)[0].click()
    await flushPromises()
    expect(wrapper.emitted('select')?.[0]).toEqual(['README.md', { mode: 'view' }])
    expect(document.querySelector('[data-test="context-menu"]')).toBeNull()

    // Deleting still goes through the confirmation dialog, and the menu is gone.
    await row.trigger('contextmenu', { clientX: 30, clientY: 30 })
    const again = document.querySelector('[data-test="context-menu"]')!
    menuItems(again).at(-1)!.click()
    await flushPromises()
    expect(document.querySelector('[data-test="context-menu"]')).toBeNull()
    expect(wrapper.find('[data-test="delete-confirm-dialog"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('offers the directory actions and expands the folder from the menu', async () => {
    const getFileTreeSpy = vi.spyOn(filesApi, 'getFileTree').mockImplementation(async (_pid, path) => {
      if (!path) return [{ name: 'src', path: 'src', isDir: true }]
      return [{ name: 'index.ts', path: 'src/index.ts', isDir: false }]
    })
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitCommits').mockResolvedValue([])

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1, directory: '/ws/demo' },
      attachTo: document.body,
    })
    await vi.waitFor(() => expect(wrapper.text()).toContain('src'))

    const row = wrapper.findAll('.file-tree-row').find((r) => r.text().includes('src'))!
    await row.trigger('contextmenu', { clientX: 20, clientY: 20 })
    const menu = document.querySelector('[data-test="context-menu"]')!
    expect(menuLabels(menu)).toEqual(['展开', '上传到此目录', '复制相对路径', '复制绝对路径', '删除目录'])

    menuItems(menu)[0].click()
    await vi.waitFor(() => expect(wrapper.text()).toContain('index.ts'))
    expect(getFileTreeSpy).toHaveBeenCalledWith(1, 'src', undefined)

    // The same row, now open, offers the opposite gesture.
    await row.trigger('contextmenu', { clientX: 20, clientY: 20 })
    const second = document.querySelector('[data-test="context-menu"]')!
    expect(menuLabels(second)[0]).toBe('折叠')

    wrapper.unmount()
  })

  it('switches between changes and all files tabs', async () => {
    vi.spyOn(filesApi, 'getFileTree').mockResolvedValue([
      { name: 'app.vue', path: 'app.vue', isDir: false },
    ])
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([
      { path: 'app.vue', status: 'modified' },
    ])

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1 },
    })

    // Allow promise resolution
    await vi.waitFor(() => expect(wrapper.text()).toContain('app.vue'))

    const tabs = wrapper.findAll('.sidebar-tab')
    expect(tabs.length).toBe(2)

    // Switch to Changes tab
    await tabs[0].trigger('click')
    expect(tabs[0].classes()).toContain('is-active')
    expect(wrapper.find('.changes-list').exists()).toBe(true)

    // Switch to All tab
    await tabs[1].trigger('click')
    expect(tabs[1].classes()).toContain('is-active')
    expect(wrapper.find('.tree-container').exists()).toBe(true)
  })

  it('renders changes in tree layout and emits diff selection on click', async () => {
    vi.spyOn(filesApi, 'getFileTree').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitCommits').mockResolvedValue([
      {
        hash: 'c1234567890abcdef',
        shortHash: 'c123456',
        message: 'feat: add widgets',
        author: 'Dev',
        date: '2026-03-30T10:00:00Z',
        timestamp: 1774864800,
      },
    ])
    const statusSpy = vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([
      { path: 'src/components/Widget.vue', status: 'added' },
      { path: 'src/main.ts', status: 'modified' },
    ])

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1 },
    })

    // Switch to Changes tab
    const tabs = wrapper.findAll('.sidebar-tab')
    await tabs[0].trigger('click')

    await vi.waitFor(() => expect(wrapper.text()).toContain('Widget.vue'))

    // The changes should be displayed as a tree with folder 'components' and 'src'
    expect(wrapper.text()).toContain('components')
    expect(wrapper.text()).toContain('Widget.vue')

    // Click on a changed file row to emit select with diff mode
    const widgetRow = wrapper.findAll('.change-tree-row').find((r) => r.text().includes('Widget.vue'))
    await widgetRow?.trigger('click')

    expect(wrapper.emitted('select')).toBeTruthy()
    expect(wrapper.emitted('select')?.[0]).toEqual([
      'src/components/Widget.vue',
      { mode: 'diff', commitRef: undefined, commitMessage: undefined },
    ])

    // Change commit via select dropdown
    const select = wrapper.find('.commit-select')
    expect(select.exists()).toBe(true)
    await select.setValue('c1234567890abcdef')

    expect(statusSpy).toHaveBeenCalledWith(1, 'c1234567890abcdef', undefined)
  })

  it('does not render discard dialog initially when tree expands', async () => {
    vi.spyOn(filesApi, 'getFileTree').mockResolvedValue([
      { name: 'src', path: 'src', isDir: true },
    ])
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1 },
    })
    await vi.waitFor(() => expect(wrapper.text()).toContain('src'))

    // Dialog should NOT be in the DOM
    expect(wrapper.find('[data-test="discard-confirm-dialog"]').exists()).toBe(false)
  })

  it('renders workspace directory full path (without copy button) and read-only branch label', async () => {
    vi.spyOn(filesApi, 'getFileTree').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitCommits').mockResolvedValue([])

    const wrapper = mount(FileSidebar, {
      props: {
        projectId: 1,
        directory: '/data/projects/my-app',
        branch: 'feature-preview',
      },
    })
    await flushPromises()

    const workdirPath = wrapper.find('[data-test="workdir-path"]')
    expect(workdirPath.exists()).toBe(true)
    expect(workdirPath.text()).toBe('/data/projects/my-app')
    // No copy button
    expect(wrapper.find('[data-test="workdir-copy"]').exists()).toBe(false)

    // Open terminal button exists and opens project terminal with workspace dir
    const termBtn = wrapper.find('[data-test="open-terminal-btn"]')
    expect(termBtn.exists()).toBe(true)
    const termStore = useProjectTerminalStore()
    await termBtn.trigger('click')
    expect(termStore.isOpen).toBe(true)
    expect(termStore.cwd).toBe('/data/projects/my-app')

    // Branch label is read-only span
    const branchLabel = wrapper.find('[data-test="git-branch"]')
    expect(branchLabel.exists()).toBe(true)
    expect(branchLabel.element.tagName).toBe('SPAN')
    expect(branchLabel.text()).toContain('feature-preview')
  })

  it('keeps branch label visible when selecting a historical commit in changes tab', async () => {
    vi.spyOn(filesApi, 'getFileTree').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitCommits').mockResolvedValue([
      {
        hash: 'c1234567890abcdef',
        shortHash: 'c123456',
        message: 'feat: add widgets',
        author: 'Dev',
        date: '2026-03-30T10:00:00Z',
        timestamp: 1774864800,
      },
    ])
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])

    const wrapper = mount(FileSidebar, {
      props: {
        projectId: 1,
        branch: 'main',
      },
    })
    await flushPromises()

    // Switch to Changes tab
    const tabs = wrapper.findAll('.sidebar-tab')
    await tabs[0].trigger('click')
    await flushPromises()

    // Select historical commit
    const select = wrapper.find('.commit-select')
    expect(select.exists()).toBe(true)
    await select.setValue('c1234567890abcdef')
    await flushPromises()

    // Branch label must NOT disappear
    const branchLabel = wrapper.find('[data-test="git-branch"]')
    expect(branchLabel.exists()).toBe(true)
    expect(branchLabel.text()).toContain('main')
  })

  it('renders branch sync status (ahead / behind) after branch label', async () => {
    vi.spyOn(filesApi, 'getFileTree').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitCommits').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])
    const gitApi = await import('../api/git')
    vi.spyOn(gitApi, 'getProjectGitInfo').mockResolvedValue({
      isGit: true,
      branch: 'feature-sync',
      ahead: 2,
      behind: 1,
    })

    const wrapper = mount(FileSidebar, {
      props: {
        projectId: 1,
      },
    })
    await flushPromises()

    const branchLabel = wrapper.find('[data-test="git-branch"]')
    expect(branchLabel.exists()).toBe(true)
    expect(branchLabel.text()).toContain('feature-sync')

    const syncStatus = wrapper.find('[data-test="git-sync-status"]')
    expect(syncStatus.exists()).toBe(true)
    expect(syncStatus.text()).toContain('↑2')
    expect(syncStatus.text()).toContain('↓1')
    expect(branchLabel.attributes('title')).toBe('当前分支: feature-sync (2 个提交未推送, 1 个提交未拉取)')
  })

  it('manages discard dialog flow: open, cancel, and confirm', async () => {
    vi.spyOn(filesApi, 'getFileTree').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitCommits').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([
      { path: 'file-to-discard.txt', status: 'modified', staged: false },
    ])
    const discardSpy = vi.spyOn(filesApi, 'discardGitFile').mockResolvedValue(undefined as any)

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1 },
    })

    // Switch to Changes tab
    const tabs = wrapper.findAll('.sidebar-tab')
    await tabs[0].trigger('click')

    await vi.waitFor(() => expect(wrapper.text()).toContain('file-to-discard.txt'))
    expect(wrapper.find('[data-test="discard-confirm-dialog"]').exists()).toBe(false)

    // Trigger discard button on the unstaged file
    const discardBtn = wrapper.find('button[title="放弃更改"]')
    expect(discardBtn.exists()).toBe(true)
    await discardBtn.trigger('click')
    await flushPromises()

    // Dialog is now open
    const dialog = wrapper.find('[data-test="discard-confirm-dialog"]')
    expect(dialog.exists()).toBe(true)
    expect(dialog.text()).toContain('file-to-discard.txt')

    // Test cancel
    const cancelBtn = wrapper.find('[data-test="cancel-discard-btn"]')
    expect(cancelBtn.exists()).toBe(true)
    await cancelBtn.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="discard-confirm-dialog"]').exists()).toBe(false)
    expect(discardSpy).not.toHaveBeenCalled()

    // Test close via header close button
    await discardBtn.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="discard-confirm-dialog"]').exists()).toBe(true)
    const closeBtn = wrapper.find('button[aria-label="关闭对话框"]')
    expect(closeBtn.exists()).toBe(true)
    await closeBtn.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="discard-confirm-dialog"]').exists()).toBe(false)

    // Open again and confirm
    await discardBtn.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="discard-confirm-dialog"]').exists()).toBe(true)

    const confirmBtn = wrapper.find('[data-test="confirm-discard-btn"]')
    expect(confirmBtn.exists()).toBe(true)
    await confirmBtn.trigger('click')
    await flushPromises()

    expect(discardSpy).toHaveBeenCalledWith(1, 'file-to-discard.txt', undefined)
    expect(wrapper.find('[data-test="discard-confirm-dialog"]').exists()).toBe(false)
  })

  it('displays upload progress card and updates on progress events', async () => {
    vi.spyOn(filesApi, 'uploadFiles').mockImplementation(async (_projId, _files, _path, _wt, onProgress) => {
      if (onProgress) {
        onProgress({ loaded: 500000, total: 1000000, percent: 50 })
      }
      return { ok: true, uploaded: ['test.zip'] }
    })

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1 },
    })

    const file = new File(['x'.repeat(100)], 'test.zip')
    const fileInput = wrapper.find<HTMLInputElement>('input[type="file"]')
    Object.defineProperty(fileInput.element, 'files', {
      value: [file],
      writable: true,
    })

    await fileInput.trigger('change')
    await flushPromises()

    const progressCard = wrapper.find('[data-test="upload-progress-card"]')
    expect(progressCard.exists()).toBe(true)
    expect(progressCard.text()).toContain('test.zip')
  })

  it('shows upload error in the card without destroying the file tree', async () => {
    vi.spyOn(filesApi, 'getFileTree').mockResolvedValue([
      { name: 'existing-file.txt', path: 'existing-file.txt', isDir: false },
    ])
    vi.spyOn(filesApi, 'uploadFiles').mockRejectedValue(new Error('API 400: invalid_upload_payload'))

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1 },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('existing-file.txt')

    const file = new File(['x'], 'fail.bin')
    const fileInput = wrapper.find<HTMLInputElement>('input[type="file"]')
    Object.defineProperty(fileInput.element, 'files', {
      value: [file],
      writable: true,
    })

    await fileInput.trigger('change')
    await flushPromises()

    // File tree still intact (not replaced by state-error)
    expect(wrapper.text()).toContain('existing-file.txt')
    // Upload error card is visible
    const progressCard = wrapper.find('[data-test="upload-progress-card"]')
    expect(progressCard.exists()).toBe(true)
    expect(wrapper.find('[data-test="upload-error-msg"]').exists()).toBe(true)
  })

  it('renders empty content for non-git projects when viewMode is changes-only', async () => {
    const gitApi = await import('../api/git')
    vi.spyOn(gitApi, 'getProjectGitInfo').mockResolvedValue({ isGit: false })
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])
    vi.spyOn(filesApi, 'getGitCommits').mockResolvedValue([])

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1, viewMode: 'changes-only' },
    })
    await flushPromises()

    expect(wrapper.find('[data-test="non-git-empty"]').exists()).toBe(true)
    expect(wrapper.find('.commit-select').exists()).toBe(false)
    expect(wrapper.find('.changes-list').exists()).toBe(false)
    expect(wrapper.find('.tree-container').exists()).toBe(false)
  })

  it('renders only file tree without sub-tab switcher when viewMode is files-only', async () => {
    vi.spyOn(filesApi, 'getFileTree').mockResolvedValue([
      { name: 'README.md', path: 'README.md', isDir: false },
    ])
    vi.spyOn(filesApi, 'getGitStatus').mockResolvedValue([])
    const gitApi = await import('../api/git')
    vi.spyOn(gitApi, 'getProjectGitInfo').mockResolvedValue({ isGit: true, branch: 'main' })

    const wrapper = mount(FileSidebar, {
      props: { projectId: 1, directory: '/tmp/proj', viewMode: 'files-only' },
    })
    await flushPromises()

    expect(wrapper.findAll('.sidebar-tab')).toHaveLength(0)
    expect(wrapper.find('[data-test="open-terminal-btn"]').exists()).toBe(false)
    expect(wrapper.find('.tree-container').exists()).toBe(true)
    expect(wrapper.text()).toContain('README.md')
  })
})

describe('CodeView.vue', () => {
  it('mounts monaco editor with content and line numbers enabled by default', () => {
    const content = 'const a = 1\nconst b = 2\nconst c = 3'
    const wrapper = mount(CodeView, {
      props: { content },
    })

    const host = wrapper.find('[data-test="monaco-editor-host"]')
    expect(host.exists()).toBe(true)
    expect(host.attributes('data-monaco-value')).toBe(content)
    expect(wrapper.text()).toContain('const a = 1')
    wrapper.unmount()
  })

  it('toggles line wrap option with Alt+Z', async () => {
    const content = 'hello world\nfoo bar\nhello universe'
    const wrapper = mount(CodeView, {
      props: { content },
    })

    const host = wrapper.find('[data-test="monaco-editor-host"]')
    expect(host.attributes('data-monaco-wrap')).toBe('on')

    await wrapper
      .find('[data-test="code-view"]')
      .trigger('keydown', { key: 'z', altKey: true })
    expect(host.attributes('data-monaco-wrap')).toBe('off')
    wrapper.unmount()
  })
})

describe('MarkdownArtifactView.vue', () => {
  it('defaults to source mode and toggles to preview mode', async () => {
    const md = '# Header\nThis is **bold** text.'
    const wrapper = mount(MarkdownArtifactView, {
      props: {
        projectId: 1,
        filePath: 'docs/test.md',
        content: md,
        fileSize: 100,
        lines: 2,
      },
    })

    // Default mode is source
    expect(wrapper.findComponent(CodeView).exists()).toBe(true)
    expect(wrapper.text()).toContain('# Header')
    expect(wrapper.find('.markdown-preview-scroll').exists()).toBe(false)

    // Switch to preview mode
    const previewBtn = wrapper.findAll('.mode-btn').find((b) => b.text().includes('预览'))
    await previewBtn?.trigger('click')

    expect(wrapper.find('.markdown-preview-scroll').exists()).toBe(true)
    expect(wrapper.html()).toContain('<h1>Header</h1>')
    expect(wrapper.html()).toContain('<strong>bold</strong>')
  })

  it('rewrites relative image sources to raw files API', async () => {
    const md = '![diagram](./assets/arch.png)'
    const wrapper = mount(MarkdownArtifactView, {
      props: {
        projectId: 42,
        filePath: 'docs/guide/arch.md',
        content: md,
      },
    })

    const previewBtn = wrapper.findAll('.mode-btn').find((b) => b.text().includes('预览'))
    await previewBtn?.trigger('click')

    const img = wrapper.find('img.markdown-image')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe(
      '/api/projects/42/files/raw?path=docs%2Fguide%2Fassets%2Farch.png',
    )
  })

  it('intercepts internal link clicks and emits open-file event', async () => {
    const md = '[API Specs](./api.md)'
    const wrapper = mount(MarkdownArtifactView, {
      props: {
        projectId: 1,
        filePath: 'docs/index.md',
        content: md,
      },
    })

    const previewBtn = wrapper.findAll('.mode-btn').find((b) => b.text().includes('预览'))
    await previewBtn?.trigger('click')

    const link = wrapper.find('a')
    expect(link.exists()).toBe(true)

    await link.trigger('click')

    expect(wrapper.emitted('open-file')?.[0]).toEqual(['docs/api.md'])
  })
})

describe('FileViewer.vue', () => {
  it('manages tabs and switches viewer components based on file type', async () => {
    vi.spyOn(filesApi, 'getFileContent').mockImplementation(async (_pid, path) => {
      if (path.endsWith('.md')) {
        return {
          path,
          size: 50,
          lines: 2,
          mimeType: 'text/markdown',
          isBinary: false,
          content: '# Title\nHello',
        }
      } else if (path.endsWith('.png')) {
        return {
          path,
          size: 1024,
          lines: 0,
          mimeType: 'image/png',
          isBinary: true,
        }
      }
      return {
        path,
        size: 30,
        lines: 1,
        mimeType: 'text/plain',
        isBinary: false,
        content: 'console.log("hi")',
      }
    })

    const wrapper = mount(FileViewer, {
      props: {
        projectId: 1,
        activePath: 'docs/readme.md',
      },
    })

    await vi.waitFor(() => expect(wrapper.findComponent(MarkdownArtifactView).exists()).toBe(true))
    expect(wrapper.findAll('.viewer-tab').length).toBe(1)
    expect(wrapper.text()).toContain('readme.md')

    // Switch to an image
    await wrapper.setProps({ activePath: 'logo.png' })
    await vi.waitFor(() => expect(wrapper.findComponent(ArtifactImageView).exists()).toBe(true))
    expect(wrapper.findAll('.viewer-tab').length).toBe(2)

    // Switch to code
    await wrapper.setProps({ activePath: 'index.ts' })
    await vi.waitFor(() => expect(wrapper.findComponent(CodeView).exists()).toBe(true))
    expect(wrapper.findAll('.viewer-tab').length).toBe(3)

    // Close one tab
    const closeBtns = wrapper.findAll('.tab-close-btn')
    await closeBtns[0].trigger('click')
    expect(wrapper.findAll('.viewer-tab').length).toBe(2)
  })

  it('renders file content in hideTabs mode without tab strip and handles empty state properly', async () => {
    vi.spyOn(filesApi, 'getFileContent').mockImplementation(async (_pid, path) => ({
      path,
      size: 100,
      lines: 3,
      mimeType: 'text/markdown',
      isBinary: false,
      content: '# Documentation\nContent here',
    }))

    const wrapper = mount(FileViewer, {
      props: {
        projectId: 1,
        activePath: 'docs/guide.md',
        hideTabs: true,
      },
    })

    await vi.waitFor(() => expect(wrapper.findComponent(MarkdownArtifactView).exists()).toBe(true))
    // In hideTabs mode, top tab strip should not exist
    expect(wrapper.find('[data-test="viewer-tab-strip"]').exists()).toBe(false)
    // Empty state must NOT be rendered when activePath is provided and content is loaded
    expect(wrapper.find('.empty-state').exists()).toBe(false)
    expect(wrapper.text()).toContain('Documentation')

    // When activePath is cleared, it should show empty state
    await wrapper.setProps({ activePath: '' })
    await flushPromises()
    expect(wrapper.find('.empty-state').exists()).toBe(true)
  })

  it('renders HtmlArtifactView when an html file is opened', async () => {
    vi.spyOn(filesApi, 'getFileContent').mockResolvedValue({
      path: 'index.html',
      size: 150,
      lines: 8,
      mimeType: 'text/html',
      isBinary: false,
      content: '<!DOCTYPE html><html><body><h1>Hello World</h1></body></html>',
    })

    const wrapper = mount(FileViewer, {
      props: {
        projectId: 1,
        activePath: 'index.html',
      },
    })

    await vi.waitFor(() => expect(wrapper.findComponent(HtmlArtifactView).exists()).toBe(true))
    expect(wrapper.text()).toContain('预览')
    expect(wrapper.findComponent(CodeView).exists()).toBe(true)
    expect(wrapper.text()).toContain('Hello World')

    // Test add-comment event forwarding from HtmlArtifactView to FileViewer
    const htmlView = wrapper.findComponent(HtmlArtifactView)
    const card = {
      id: 'html-card-1',
      filePath: 'index.html',
      fileName: 'index.html',
      startLine: 1,
      endLine: 1,
      codeSnippet: '<html><body>Hello World</body></html>',
      comment: '更新主页标题',
      createdAt: Date.now(),
    }
    await htmlView.vm.$emit('add-comment', card)
    expect(wrapper.emitted('add-comment')).toBeTruthy()
    expect(wrapper.emitted('add-comment')![0][0]).toEqual(card)
  })
})

describe('HtmlPreviewModal.vue', () => {
  it('renders dialog with 80% viewport defaults and sandboxed iframe', () => {
    const wrapper = mount(HtmlPreviewModal, {
      props: {
        visible: true,
        fileName: 'demo.html',
        previewUrl: '/api/projects/1/files/preview/test-ticket/demo.html',
        filePath: 'public/demo.html',
      },
      attachTo: document.body,
    })

    const dialog = document.querySelector('[data-test="html-preview-dialog"]')
    expect(dialog).toBeTruthy()
    expect(dialog?.classList.contains('is-fullscreen')).toBe(false)

    const iframe = document.querySelector('[data-test="html-preview-frame"]') as HTMLIFrameElement
    expect(iframe).toBeTruthy()
    expect(iframe.getAttribute('sandbox')).toBe('allow-scripts allow-forms allow-modals')
    expect(iframe.getAttribute('src')).toBe('/api/projects/1/files/preview/test-ticket/demo.html')

    wrapper.unmount()
  })

  it('toggles full screen mode on fullscreen button click and restores on Esc', async () => {
    const wrapper = mount(HtmlPreviewModal, {
      props: {
        visible: true,
        fileName: 'demo.html',
        previewUrl: '/api/projects/1/files/preview/test-ticket/demo.html',
      },
      attachTo: document.body,
    })

    const fullscreenBtn = document.querySelector('[data-test="modal-fullscreen-btn"]') as HTMLButtonElement
    expect(fullscreenBtn).toBeTruthy()

    // Click full screen
    fullscreenBtn.click()
    await wrapper.vm.$nextTick()
    let dialog = document.querySelector('[data-test="html-preview-dialog"]')
    expect(dialog?.classList.contains('is-fullscreen')).toBe(true)
    expect(fullscreenBtn.textContent).toContain('还原')

    // Press Esc when in fullscreen - should exit fullscreen first without closing modal
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    dialog = document.querySelector('[data-test="html-preview-dialog"]')
    expect(dialog?.classList.contains('is-fullscreen')).toBe(false)
    expect(wrapper.emitted('close')).toBeFalsy()

    // Press Esc again - should emit close
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeTruthy()

    wrapper.unmount()
  })

  it('triggers refresh and increments iframe key', async () => {
    const wrapper = mount(HtmlPreviewModal, {
      props: {
        visible: true,
        fileName: 'demo.html',
        previewUrl: '/api/projects/1/files/preview/test-ticket/demo.html',
      },
      attachTo: document.body,
    })

    const refreshBtn = document.querySelector('[data-test="modal-refresh-btn"]') as HTMLButtonElement
    expect(refreshBtn).toBeTruthy()
    refreshBtn.click()
    await wrapper.vm.$nextTick()

    // Component internal key should increment
    expect((wrapper.vm as any).iframeKey).toBe(1)
    wrapper.unmount()
  })

  it('emits close when clicking close button or backdrop', async () => {
    const wrapper = mount(HtmlPreviewModal, {
      props: {
        visible: true,
        fileName: 'demo.html',
        previewUrl: '/api/projects/1/files/preview/test-ticket/demo.html',
      },
      attachTo: document.body,
    })

    const closeBtn = document.querySelector('[data-test="modal-close-btn"]') as HTMLButtonElement
    closeBtn.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeTruthy()

    const backdrop = document.querySelector('[data-test="html-preview-backdrop"]') as HTMLElement
    backdrop.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')?.length).toBe(2)

    wrapper.unmount()
  })
})

describe('HtmlArtifactView.vue', () => {
  it('renders source CodeView and toolbar with metadata', () => {
    const wrapper = mount(HtmlArtifactView, {
      props: {
        projectId: 1,
        filePath: 'public/index.html',
        content: '<!DOCTYPE html><html><body><h1>Test</h1></body></html>',
        fileSize: 1024,
        lines: 5,
      },
    })

    expect(wrapper.findComponent(CodeView).exists()).toBe(true)
    expect(wrapper.text()).toContain('预览')
    expect(wrapper.text()).toContain('5 行')
    expect(wrapper.text()).toContain('1.0 KB')
    expect(wrapper.text()).toContain('public/index.html')
  })

  it('requests preview ticket and opens HtmlPreviewModal when clicking 预览', async () => {
    const createTicketSpy = vi.spyOn(filesApi, 'createPreviewTicket').mockResolvedValue('mock-random-ticket-123')

    const wrapper = mount(HtmlArtifactView, {
      props: {
        projectId: 42,
        filePath: 'docs/page.html',
        content: '<h1>Content</h1>',
      },
    })

    const browseBtn = wrapper.find('[data-test="browse-in-browser-btn"]')
    expect(browseBtn.exists()).toBe(true)

    await browseBtn.trigger('click')
    await flushPromises()

    expect(createTicketSpy).toHaveBeenCalledWith(42, undefined)
    const modal = wrapper.findComponent(HtmlPreviewModal)
    expect(modal.exists()).toBe(true)
    expect(modal.props('visible')).toBe(true)
    expect(modal.props('previewUrl')).toBe('/api/projects/42/files/preview/mock-random-ticket-123/docs/page.html')
    expect(modal.props('fileName')).toBe('page.html')
  })
})

