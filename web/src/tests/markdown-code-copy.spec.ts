import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { handleCodeCopyClick } from '../shared/markdown-code-copy'
import { renderMarkdown } from '../shared/markdown'

function mountHtml(html: string): HTMLElement {
  const root = document.createElement('div')
  root.className = 'markdown-body'
  root.innerHTML = html
  document.body.appendChild(root)
  return root
}

function clickEvent(): MouseEvent {
  return new MouseEvent('click', { bubbles: true, cancelable: true })
}

describe('handleCodeCopyClick', () => {
  let writeText: ReturnType<typeof vi.fn>

  beforeEach(() => {
    vi.useFakeTimers()
    writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      writable: true,
      configurable: true,
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('copies the code of a block rendered by renderMarkdown', async () => {
    const root = mountHtml(renderMarkdown('```go\npackage main\n```'))
    const btn = root.querySelector<HTMLButtonElement>('.code-copy-btn')!
    const event = clickEvent()
    btn.dispatchEvent(event)

    const handled = await handleCodeCopyClick(event)
    expect(handled).toBe(true)
    expect(writeText).toHaveBeenCalledWith('package main')
    expect(btn.classList.contains('is-copied')).toBe(true)
    expect(btn.getAttribute('title')).toBe('已复制')

    vi.advanceTimersByTime(1500)
    expect(btn.classList.contains('is-copied')).toBe(false)
    expect(btn.getAttribute('title')).toBe('复制代码')
  })

  it('ignores clicks outside a copy button', async () => {
    const root = mountHtml(renderMarkdown('```go\npackage main\n```\n\n正文'))
    const paragraph = root.querySelector('p')!
    const event = clickEvent()
    paragraph.dispatchEvent(event)

    expect(await handleCodeCopyClick(event)).toBe(false)
    expect(writeText).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
  })
})

describe('shared code block styles', () => {
  it('are loaded globally, so every markdown surface gets the copy button layout', () => {
    const main = readFileSync(resolve(__dirname, '../main.ts'), 'utf8')
    expect(main).toContain("import './shared/markdown-code.css'")

    const css = readFileSync(resolve(__dirname, '../shared/markdown-code.css'), 'utf8')
    expect(css).toMatch(/\.markdown-body \.code-copy-btn \{[^}]*position:\s*absolute/)
    expect(css).toMatch(/\.markdown-body \.code-copy-btn \.code-copy-icon \{[^}]*width:/)
  })
})
