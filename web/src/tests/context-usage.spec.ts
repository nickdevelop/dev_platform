// Unit tests for the composer context line math: the exact backend anchor
// plus the trailing chars/4 estimate, with percentages only when the model
// reports a context window.
import { describe, expect, it } from 'vitest'
import { computeContextUsage, estimateTokens } from '../contextUsage'
import { ellipsisMiddle, formatTokens } from '../format'
import type { ChatEntry } from '../stores/sessions'

function message(id: string, role: string, text: string, usage?: ChatEntry['usage']): ChatEntry {
  return { id, kind: 'message', role, text, streaming: false, usage }
}

describe('estimateTokens', () => {
  it('approximates chars/4 rounding up', () => {
    expect(estimateTokens('')).toBe(0)
    expect(estimateTokens('abcd')).toBe(1)
    expect(estimateTokens('abcde')).toBe(2)
  })
})

describe('formatTokens', () => {
  it('formats human-readable counts like the pi footer', () => {
    expect(formatTokens(0)).toBe('0')
    expect(formatTokens(999)).toBe('999')
    expect(formatTokens(8100)).toBe('8.1k')
    expect(formatTokens(128000)).toBe('128k')
    expect(formatTokens(1000000)).toBe('1M')
    expect(formatTokens(1500000)).toBe('1.5M')
  })
})

describe('ellipsisMiddle', () => {
  it('keeps short paths whole and shortens long ones in the middle', () => {
    expect(ellipsisMiddle('/a/b')).toBe('/a/b')
    const long = '/data/projects/very-long-workspace-name/dev_platform/web/src'
    const shortened = ellipsisMiddle(long, 32)
    expect(shortened.length).toBeLessThan(long.length)
    expect(shortened).toContain('/.../')
    expect(shortened.startsWith('/data/')).toBe(true)
    expect(shortened.endsWith('web/src')).toBe(true)
  })
})

describe('computeContextUsage', () => {
  it('anchors on the newest assistant usage and estimates the tail', () => {
    const entries = [
      message('1', 'user', 'hello'),
      message('2', 'assistant', 'hi', { input: 8000, output: 2000 }),
      message('3', 'user', '0123456789'),
    ]
    const usage = computeContextUsage(entries, undefined, 128000)
    // Anchor 10000 + trailing ceil(10/4)=3 on the input side.
    expect(usage.used).toBe(10003)
    expect(usage.percent).toBe(8)
    expect(usage.estimated).toBe(true)
    expect(usage.parts.input).toBe(8003)
    expect(usage.parts.output).toBe(2000)
  })

  it('prefers an explicit turn-level report over message anchors', () => {
    const entries = [message('1', 'assistant', 'old', { input: 100, output: 10 })]
    const usage = computeContextUsage(entries, { input: 500, output: 60 }, 1000)
    // The turn report covers its whole turn including the logged message, so
    // nothing trails it: the figure is exact.
    expect(usage.used).toBe(560)
    expect(usage.percent).toBe(56)
    expect(usage.estimated).toBe(false)
    expect(usage.parts.input).toBe(500)
  })

  it('reports exact figures with no estimate when nothing trails the anchor', () => {
    const entries = [message('1', 'assistant', 'done', { input: 4000, output: 1000 })]
    const usage = computeContextUsage(entries, undefined, 100000)
    expect(usage.used).toBe(5000)
    expect(usage.percent).toBe(5)
    expect(usage.estimated).toBe(false)
  })

  it('omits the percentage without a model limit', () => {
    const entries = [message('1', 'assistant', 'done', { input: 4000, output: 1000 })]
    const usage = computeContextUsage(entries, undefined, undefined)
    expect(usage.used).toBe(5000)
    expect(usage.percent).toBeUndefined()
    expect(usage.limit).toBeUndefined()
    expect(usage.estimated).toBe(false)
  })

  it('estimates everything when the backend reports no usage', () => {
    const entries = [message('1', 'user', '01234567'), message('2', 'assistant', '0123')]
    const usage = computeContextUsage(entries, undefined, 1000)
    expect(usage.used).toBe(estimateTokens('01234567') + estimateTokens('0123'))
    expect(usage.estimated).toBe(true)
    expect(usage.percent).toBe(Math.round((usage.used / 1000) * 100))
  })

  it('counts reasoning blocks as reasoning, not input', () => {
    const entries: ChatEntry[] = [
      message('1', 'assistant', 'done', { input: 1000, output: 100 }),
      { id: '2', kind: 'thinking', role: 'assistant', text: '01234567', streaming: false },
    ]
    const usage = computeContextUsage(entries, undefined, undefined)
    expect(usage.parts.reasoning).toBe(estimateTokens('01234567'))
    expect(usage.parts.input).toBe(1000)
  })
})
