import { describe, expect, it, vi } from 'vitest'
import { WorkspaceFileSource } from '../features/chat/composer/dropdown/sources/WorkspaceFileSource'
import * as projectsApi from '../api/projects'

describe('WorkspaceFileSource', () => {
  const source = new WorkspaceFileSource(() => 101)

  it('matches @ at text start or after whitespace', () => {
    expect(source.match('@src', 4)).toEqual({
      trigger: '@',
      query: 'src',
      start: 0,
      end: 4,
      atLineStart: true,
    })

    expect(source.match('please read @mai', 16)).toEqual({
      trigger: '@',
      query: 'mai',
      start: 12,
      end: 16,
      atLineStart: false,
    })

    // Email address should not match
    expect(source.match('test@example.com', 12)).toBeNull()

    // Query with spaces should not match
    expect(source.match('@src file', 9)).toBeNull()

    // Completed markdown tokens should not trigger open match
    expect(source.match('@[My Title](session:123)', 24)).toBeNull()
  })

  it('loads files and folders from searchProjectFiles API', async () => {
    const mockFiles: projectsApi.FileItem[] = [
      { path: 'src/main.go', isDir: false },
      { path: 'src/utils', isDir: true },
    ]

    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValueOnce({
      files: mockFiles,
    })

    const items = await source.load(
      { trigger: '@', query: 'src', start: 0, end: 4, atLineStart: true },
      new AbortController().signal,
    )

    expect(items.length).toBe(2)
    expect(items[0]).toEqual({
      id: 'file:src/main.go',
      label: '@src/main.go',
      detail: '工作区文件',
      kind: 'file',
      badge: '文件',
      replacement: '@src/main.go ',
    })
    expect(items[1]).toEqual({
      id: 'folder:src/utils',
      label: '@src/utils/',
      detail: '工作区文件夹',
      kind: 'folder',
      badge: '目录',
      replacement: '@src/utils/ ',
    })
  })

  it('handles aborted signal gracefully', async () => {
    const ac = new AbortController()
    ac.abort()

    vi.spyOn(projectsApi, 'searchProjectFiles').mockResolvedValueOnce({
      files: [{ path: 'test.txt', isDir: false }],
    })

    const items = await source.load(
      { trigger: '@', query: 'test', start: 0, end: 5, atLineStart: true },
      ac.signal,
    )

    expect(items).toEqual([])
  })
})
