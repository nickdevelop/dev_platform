import { describe, expect, it, vi } from 'vitest'
import { ProviderSlashSource } from '../features/chat/composer/dropdown/sources/ProviderSlashSource'
import * as sessionsApi from '../api/sessions'

describe('ProviderSlashSource', () => {
  const source = new ProviderSlashSource(
    () => 10,
    () => 'ses-abc',
  )

  it('matches slash only at line start', () => {
    expect(source.match('/re', 3)).toEqual({
      trigger: '/',
      query: 're',
      start: 0,
      end: 3,
      atLineStart: true,
    })

    expect(source.match('first line\n/co', 14)).toEqual({
      trigger: '/',
      query: 'co',
      start: 11,
      end: 14,
      atLineStart: true,
    })

    expect(source.match('check a/b path', 10)).toBeNull()
    expect(source.match('/review args', 12)).toBeNull()
  })

  it('loads commands from API and caches results', async () => {
    const mockCommands: sessionsApi.AgentCommand[] = [
      { name: 'review', description: '代码审查', kind: 'command', argumentHint: '[path]' },
      { name: 'opencode', description: '加载技能', kind: 'skill' },
    ]

    const apiSpy = vi.spyOn(sessionsApi, 'listSessionCommands').mockResolvedValue({
      commands: mockCommands,
    })

    // First load fetches via API
    const items = await source.load(
      { trigger: '/', query: '', start: 0, end: 1, atLineStart: true },
      new AbortController().signal,
    )

    expect(apiSpy).toHaveBeenCalledTimes(1)
    expect(items.length).toBe(2)
    expect(items[0]).toEqual({
      id: 'provider:review',
      label: '/review',
      detail: '代码审查 · [path]',
      kind: 'command',
      badge: 'Agent',
      replacement: '/review ',
    })
    expect(items[1].badge).toBe('技能')

    // Second load with filter should hit cache
    const filtered = await source.load(
      { trigger: '/', query: 'open', start: 0, end: 5, atLineStart: true },
      new AbortController().signal,
    )
    expect(apiSpy).toHaveBeenCalledTimes(1)
    expect(filtered.length).toBe(1)
    expect(filtered[0].label).toBe('/opencode')

    // Clear cache allows fresh API fetch
    source.clearCache('ses-abc')
    await source.load(
      { trigger: '/', query: '', start: 0, end: 1, atLineStart: true },
      new AbortController().signal,
    )
    expect(apiSpy).toHaveBeenCalledTimes(2)
  })

  it('returns empty list when aborted', async () => {
    const ac = new AbortController()
    ac.abort()

    const items = await source.load(
      { trigger: '/', query: '', start: 0, end: 1, atLineStart: true },
      ac.signal,
    )
    expect(items).toEqual([])
  })
})
