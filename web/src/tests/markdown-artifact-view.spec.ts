import { beforeEach, describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import MarkdownArtifactView from '../features/files/MarkdownArtifactView.vue'
import { clearMermaidCache, configureMermaidEngine } from '../mermaid/renderer'

describe('MarkdownArtifactView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    clearMermaidCache()
    configureMermaidEngine(null)
  })

  it('renders markdown text and highlighted code block', async () => {
    const md = '# 标题\n\n```typescript\nconst message = "hello";\n```'
    const wrapper = mount(MarkdownArtifactView, {
      props: {
        projectId: 1,
        filePath: 'doc.md',
        content: md,
      },
    })
    await flushPromises()

    const preview = wrapper.find('[data-test="markdown-preview"]')
    expect(preview.exists()).toBe(true)
    expect(preview.html()).toContain('<h1')
    expect(preview.html()).toContain('class="language-typescript hljs"')
    expect(preview.html()).toContain('class="hljs-keyword">const</span>')
  })

  it('renders embedded mermaid diagram block in preview mode', async () => {
    configureMermaidEngine(async () => '<svg><text>rendered-mermaid</text></svg>')
    const md = '文档前言\n\n```mermaid\ngraph TD\nA --> B\n```\n\n文档结语'
    const wrapper = mount(MarkdownArtifactView, {
      props: {
        projectId: 1,
        filePath: 'arch.md',
        content: md,
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-test="mermaid-block"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="mermaid-canvas"]').html()).toContain('rendered-mermaid')
  })

  it('switches to source mode and renders CodeView', async () => {
    const md = '# 纯文本'
    const wrapper = mount(MarkdownArtifactView, {
      props: {
        projectId: 1,
        filePath: 'readme.md',
        content: md,
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-test="code-view"]').exists()).toBe(false)
    const sourceBtn = wrapper.findAll('.mode-btn').find((btn) => btn.text().includes('源码'))
    await sourceBtn?.trigger('click')

    expect(wrapper.find('[data-test="code-view"]').exists()).toBe(true)
  })
})
