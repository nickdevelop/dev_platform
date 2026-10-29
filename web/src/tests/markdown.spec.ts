// Unit tests for the markdown renderer used by agent messages. The renderer is
// the trust boundary: agent text must become common markdown, but backend
// content must never be able to inject live markup or event handlers.
import { describe, expect, it } from 'vitest'
import { renderMarkdown } from '../shared/markdown'

describe('renderMarkdown', () => {
  it('renders common block and inline markdown', () => {
    const html = renderMarkdown('# 标题\n\n**粗体** 和 `代码`\n\n- 一\n- 二')

    expect(html).toContain('<h1')
    expect(html).toContain('<strong>粗体</strong>')
    expect(html).toContain('<code>代码</code>')
    expect(html).toContain('<li>一</li>')
  })

  it('turns a newline inside a paragraph into a line break', () => {
    expect(renderMarkdown('第一行\n第二行')).toContain('<br>')
  })

  it('drops script tags from agent content', () => {
    const html = renderMarkdown('before\n\n<script>alert(1)</script>\n\nafter')

    expect(html).not.toContain('<script')
    expect(html).toContain('before')
    expect(html).toContain('after')
  })

  it('strips event handlers from injected markup', () => {
    const html = renderMarkdown('<img src=x onerror="alert(1)">')

    expect(html).not.toContain('onerror')
  })

  it('keeps ordinary links', () => {
    const html = renderMarkdown('[文档](https://example.com/docs)')

    expect(html).toContain('href="https://example.com/docs"')
    expect(html).toContain('文档')
  })

  it('wraps code blocks with a wrapper and copy button', () => {
    const html = renderMarkdown('```typescript\nconst a = 1 < 2;\nconsole.log(a);\n```')

    expect(html).toContain('class="code-block-wrapper"')
    expect(html).toContain('class="code-copy-btn"')
    expect(html).toContain('title="复制代码"')
    expect(html).toContain('aria-label="复制代码"')
    expect(html).toContain('class="code-copy-text">复制</span>')
    expect(html).toContain('<pre><code class="language-typescript">')
    expect(html).toContain('const a = 1 &lt; 2;')
  })
})
