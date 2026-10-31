import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { getDirectory, isExternalUrl, resolveRelativePath } from '../features/files/pathUtils'
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
})

describe('FileSidebar.vue', () => {
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

    expect(getFileTreeSpy).toHaveBeenCalledWith(1, 'src')
    expect(wrapper.text()).toContain('index.ts')
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

    expect(statusSpy).toHaveBeenCalledWith(1, 'c1234567890abcdef')
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

    expect(discardSpy).toHaveBeenCalledWith(1, 'file-to-discard.txt')
    expect(wrapper.find('[data-test="discard-confirm-dialog"]').exists()).toBe(false)
  })
})

describe('CodeView.vue', () => {
  it('renders lines and line numbers', () => {
    const content = 'const a = 1\nconst b = 2\nconst c = 3'
    const wrapper = mount(CodeView, {
      props: { content },
    })

    expect(wrapper.findAll('.line-number').length).toBe(3)
    expect(wrapper.findAll('.code-line').length).toBe(3)
    expect(wrapper.text()).toContain('const a = 1')
  })

  it('supports in-file search', async () => {
    const content = 'hello world\nfoo bar\nhello universe'
    const wrapper = mount(CodeView, {
      props: { content },
    })

    // Open search bar
    const searchBtn = wrapper.findAll('.action-btn').find((b) => b.text().includes('查找'))
    await searchBtn?.trigger('click')

    expect(wrapper.find('.search-bar').exists()).toBe(true)

    const input = wrapper.find('.search-bar-input')
    await input.setValue('hello')

    expect(wrapper.text()).toContain('1/2')
  })
})

describe('MarkdownArtifactView.vue', () => {
  it('toggles between preview and source mode', async () => {
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

    // Default mode is preview
    expect(wrapper.find('.markdown-preview-scroll').exists()).toBe(true)
    expect(wrapper.html()).toContain('<h1>Header</h1>')
    expect(wrapper.html()).toContain('<strong>bold</strong>')

    // Switch to source mode
    const sourceBtn = wrapper.findAll('.mode-btn').find((b) => b.text().includes('源码'))
    await sourceBtn?.trigger('click')

    expect(wrapper.findComponent(CodeView).exists()).toBe(true)
    expect(wrapper.text()).toContain('# Header')
  })

  it('rewrites relative image sources to raw files API', () => {
    const md = '![diagram](./assets/arch.png)'
    const wrapper = mount(MarkdownArtifactView, {
      props: {
        projectId: 42,
        filePath: 'docs/guide/arch.md',
        content: md,
      },
    })

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

    expect(createTicketSpy).toHaveBeenCalledWith(42)
    const modal = wrapper.findComponent(HtmlPreviewModal)
    expect(modal.exists()).toBe(true)
    expect(modal.props('visible')).toBe(true)
    expect(modal.props('previewUrl')).toBe('/api/projects/42/files/preview/mock-random-ticket-123/docs/page.html')
    expect(modal.props('fileName')).toBe('page.html')
  })
})

