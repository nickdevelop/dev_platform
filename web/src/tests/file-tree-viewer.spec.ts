import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { getDirectory, isExternalUrl, resolveRelativePath } from '../features/files/pathUtils'
import FileTree from '../features/files/FileTree.vue'
import FileSidebar from '../features/files/FileSidebar.vue'
import CodeView from '../features/files/CodeView.vue'
import ArtifactToolbar from '../features/files/ArtifactToolbar.vue'
import MarkdownArtifactView from '../features/files/MarkdownArtifactView.vue'
import ArtifactImageView from '../features/files/ArtifactImageView.vue'
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
})

describe('FileSidebar.vue', () => {
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
})
