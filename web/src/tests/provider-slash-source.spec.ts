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

  it('loads platform commands from API and caches results', async () => {
    const mockCommands: sessionsApi.PlatformCommand[] = [
      { name: 'compact', description: '压缩上下文', category: 'action', argumentHint: '[可选说明]' },
      { name: 'review', description: '注入代码审查提示词', category: 'prompt' },
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
      id: 'platform:compact',
      label: '/compact',
      detail: '压缩上下文 · [可选说明]',
      kind: 'command',
      badge: '功能',
      replacement: '/compact ',
    })
    expect(items[1].badge).toBe('技能')

    // Second load with filter should hit cache
    const filtered = await source.load(
      { trigger: '/', query: 'comp', start: 0, end: 5, atLineStart: true },
      new AbortController().signal,
    )
    expect(apiSpy).toHaveBeenCalledTimes(1)
    expect(filtered.length).toBe(1)
    expect(filtered[0].label).toBe('/compact')

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

  it('loads project commands when sessionId is empty (new session)', async () => {
    const newSessionSource = new ProviderSlashSource(
      () => 42,
      () => '',
      () => 'opencode-v2',
      () => 1,
    )

    const mockProjectCommands: sessionsApi.PlatformCommand[] = [
      { name: 'compact', description: '压缩上下文', category: 'action' },
      { name: 'plantuml', description: '绘图技能', category: 'prompt' },
    ]

    const projectApiSpy = vi.spyOn(sessionsApi, 'listProjectCommands').mockResolvedValue({
      commands: mockProjectCommands,
    })

    const items = await newSessionSource.load(
      { trigger: '/', query: '', start: 0, end: 1, atLineStart: true },
      new AbortController().signal,
    )

    expect(projectApiSpy).toHaveBeenCalledWith(42, 1)
    expect(items.length).toBe(2)
    expect(items[1].label).toBe('/plantuml')
    expect(items[1].badge).toBe('技能')

    // findCommand should also work in new session
    const found = newSessionSource.findCommand('plantuml')
    expect(found?.name).toBe('plantuml')
  })
})
