import { describe, expect, it, vi } from 'vitest'
import {
  formatSessionMention,
  parseSessionMentions,
  resolveSessionMentions,
} from '../features/chat/composer/sessionMentionResolver'
import { SessionMentionSource } from '../features/chat/composer/dropdown/sources/SessionMentionSource'
import type { AgentSession, SessionHistory } from '../api/sessions'

describe('sessionMentionResolver', () => {
  it('formats and parses session mentions accurately', () => {
    const formatted = formatSessionMention('重构架构讨论 [v2]', 'ses-123')
    expect(formatted).toBe('@[重构架构讨论 \\[v2\\]](session:ses-123) ')

    const text = `参考此方案：${formatted}，请核对。`
    const parsed = parseSessionMentions(text)
    expect(parsed.length).toBe(1)
    expect(parsed[0].sessionId).toBe('ses-123')
    expect(parsed[0].title).toBe('重构架构讨论 [v2]')
  })

  it('resolves session mentions by injecting history dialogue', async () => {
    const text = '请对比 @[方案 A](session:ses-1) 与最新进展'
    const mockHistory: any = {
      entries: [
        {
          id: '1',
          kind: 'user',
          text: '我们采用方案 A 可以吗？',
          timestamp: '2026-10-16T10:00:00Z',
        },
        {
          id: '2',
          kind: 'agent',
          text: '方案 A 很稳健，建议采用。',
          timestamp: '2026-10-16T10:00:05Z',
        },
      ],
      truncated: false,
    }

    const fetchHistory = vi.fn().mockResolvedValue(mockHistory)
    const result = await resolveSessionMentions(1, text, fetchHistory)

    expect(fetchHistory).toHaveBeenCalledWith('ses-1')
    expect(result).toContain('@"方案 A"')
    expect(result).toContain('<referenced_session id="ses-1" title="方案 A">')
    expect(result).toContain('[用户]: 我们采用方案 A 可以吗？')
    expect(result).toContain('[Agent]: 方案 A 很稳健，建议采用。')
  })

  it('handles fetch failure gracefully without throwing', async () => {
    const text = '检查 @[已删除会话](session:ses-missing)'
    const fetchHistory = vi.fn().mockRejectedValue(new Error('not found'))

    const result = await resolveSessionMentions(1, text, fetchHistory)
    expect(result).toContain('@"已删除会话"')
    expect(result).toContain('<referenced_session id="ses-missing" title="已删除会话" unavailable="true" />')
  })
})

describe('SessionMentionSource', () => {
  const dummySession = (id: string, title: string): AgentSession => ({
    id,
    projectId: 1,
    backend: 'opencode-v2',
    instanceId: 'inst-1',
    nativeSessionId: `nat-${id}`,
    title,
    status: 'active',
    turn: 'idle',
    capabilities: {
      streaming: true,
      sessionDiscovery: true,
      sessionContinue: true,
      stop: true,
      modelSelection: true,
      toolCallEvents: true,
      toolConfirmation: true,
      interrupt: true,
      sessionHistory: true,
      rawEvents: true,
    },
    createdAt: '2026-10-16T10:00:00Z',
    updatedAt: '2026-10-16T10:00:00Z',
  })

  it('filters out current session and matches by query', () => {
    const all = [
      dummySession('ses-current', '当前会话'),
      dummySession('ses-1', '登录流程重构'),
      dummySession('ses-2', '工作区样式调优'),
    ]

    const source = new SessionMentionSource(
      () => all,
      () => 'ses-current',
    )

    const match = { trigger: '@' as const, query: '重构', start: 0, end: 3, atLineStart: true }
    const items = source.load(match, new AbortController().signal)

    expect(items.length).toBe(1)
    expect(items[0].id).toBe('session:ses-1')
    expect(items[0].label).toBe('@登录流程重构')
    expect(items[0].kind).toBe('session')
    expect(items[0].badge).toBe('会话')
    expect(items[0].replacement).toContain('@[登录流程重构](session:ses-1)')
  })
})
