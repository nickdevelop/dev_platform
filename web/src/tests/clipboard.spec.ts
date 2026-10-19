import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { copyToClipboard } from '../shared/clipboard'

describe('copyToClipboard', () => {
  const originalClipboard = navigator.clipboard

  afterEach(() => {
    Object.defineProperty(navigator, 'clipboard', {
      value: originalClipboard,
      writable: true,
      configurable: true,
    })
    vi.restoreAllMocks()
  })

  it('returns false when text is empty', async () => {
    const success = await copyToClipboard('')
    expect(success).toBe(false)
  })

  it('uses navigator.clipboard.writeText when available', async () => {
    const writeTextMock = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: writeTextMock },
      writable: true,
      configurable: true,
    })

    const success = await copyToClipboard('hello world')
    expect(success).toBe(true)
    expect(writeTextMock).toHaveBeenCalledWith('hello world')
  })

  it('falls back to execCommand when clipboard.writeText fails', async () => {
    const writeTextMock = vi.fn().mockRejectedValue(new Error('Permission denied'))
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: writeTextMock },
      writable: true,
      configurable: true,
    })

    const execCommandMock = vi.fn().mockReturnValue(true)
    document.execCommand = execCommandMock

    const success = await copyToClipboard('fallback text')
    expect(success).toBe(true)
    expect(execCommandMock).toHaveBeenCalledWith('copy')
  })
})
