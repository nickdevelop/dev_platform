import { copyToClipboard } from './clipboard'

// Copies the code of a block whose copy button was pressed. Every surface that
// renders markdown through shared/markdown.ts delegates its click here. It
// reports whether the click hit a copy button, so callers can stop there.
export async function handleCodeCopyClick(event: MouseEvent): Promise<boolean> {
  const target = event.target as HTMLElement | null
  const btn = target?.closest<HTMLButtonElement>('.code-copy-btn')
  if (!btn) return false

  event.preventDefault()
  event.stopPropagation()

  const wrapper = btn.closest('.code-block-wrapper')
  const codeEl = wrapper?.querySelector('pre code') || wrapper?.querySelector('pre')
  if (!codeEl) return true

  const codeText = (codeEl.textContent || '').replace(/\n$/, '')
  const ok = await copyToClipboard(codeText)
  if (!ok) return true

  btn.classList.add('is-copied')
  btn.setAttribute('title', '已复制')
  btn.setAttribute('aria-label', '已复制')

  setTimeout(() => {
    btn.classList.remove('is-copied')
    btn.setAttribute('title', '复制代码')
    btn.setAttribute('aria-label', '复制代码')
  }, 1500)
  return true
}
