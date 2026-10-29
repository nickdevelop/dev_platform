import { describe, expect, it, vi } from 'vitest'
import { dispatchSlashCommand } from '../features/chat/composer/slashActionDispatcher'

describe('dispatchSlashCommand', () => {
  it('returns handled=false for non-slash inputs', async () => {
    const res = await dispatchSlashCommand('hello world', {
      projectId: 1,
      sessionId: 's-1',
      findCommand: () => undefined,
    })
    expect(res.handled).toBe(false)
  })

  it('executes /compact command successfully', async () => {
    const mockCompact = vi.fn().mockResolvedValue({ ok: true })
    const res = await dispatchSlashCommand('/compact summarize recent changes', {
      projectId: 1,
      sessionId: 's-1',
      findCommand: (name) => ({ name, category: 'action' }),
      compactSession: mockCompact,
    })

    expect(res.handled).toBe(true)
    expect(res.clearInput).toBe(true)
    expect(res.notice).toContain('已向后端发送会话历史上下文压缩指令')
    expect(mockCompact).toHaveBeenCalledWith(1, 's-1', 'summarize recent changes')
  })

  it('handles /compact failure gracefully', async () => {
    const mockCompact = vi.fn().mockRejectedValue(new Error('backend timeout'))
    const res = await dispatchSlashCommand('/compact', {
      projectId: 1,
      sessionId: 's-1',
      findCommand: (name) => ({ name, category: 'action' }),
      compactSession: mockCompact,
    })

    expect(res.handled).toBe(true)
    expect(res.clearInput).toBe(false)
    expect(res.notice).toContain('压缩执行失败: backend timeout')
  })

  it('returns handled=false for prompt category commands', async () => {
    const res = await dispatchSlashCommand('/review', {
      projectId: 1,
      sessionId: 's-1',
      findCommand: (name) => ({ name, category: 'prompt' }),
    })

    expect(res.handled).toBe(false)
  })

  it('returns fallback notice for unknown commands', async () => {
    const res = await dispatchSlashCommand('/unknown foo bar', {
      projectId: 1,
      sessionId: 's-1',
      findCommand: () => undefined,
    })

    expect(res.handled).toBe(true)
    expect(res.clearInput).toBe(true)
    expect(res.notice).toContain('未知命令 /unknown')
  })
})
