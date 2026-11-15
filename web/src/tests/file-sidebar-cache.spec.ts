// Regression tests for the workspace file sidebar. Collapsing the tree unmounts
// the sidebar; reopening it must not fetch the tree again while the loaded data
// is still valid, and an expanded directory must show its children instead of
// staying in the loading state.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import FileSidebar from '../features/files/FileSidebar.vue'
import { useFileTreeStateStore } from '../stores/fileTreeState'
import { getFileTree, getGitCommits, getGitStatus } from '../api/files'

vi.mock('../api/files', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/files')>()),
  getFileTree: vi.fn(),
  getGitStatus: vi.fn(),
  getGitCommits: vi.fn(),
}))

vi.mock('../api/projects', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/projects')>()),
  searchProjectFiles: vi.fn(),
}))

function mountSidebar(sessionId = 's1') {
  return mount(FileSidebar, { props: { projectId: 1, sessionId } })
}

describe('file sidebar data memory', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(getFileTree).mockReset()
    vi.mocked(getGitStatus).mockReset()
    vi.mocked(getGitCommits).mockReset()
    vi.mocked(getFileTree).mockImplementation(async (_projectId, path) =>
      path === 'src'
        ? [{ name: 'b.ts', path: 'src/b.ts', isDir: false }]
        : [
            { name: 'src', path: 'src', isDir: true, loaded: false },
            { name: 'a.txt', path: 'a.txt', isDir: false },
          ],
    )
    vi.mocked(getGitStatus).mockResolvedValue([])
    vi.mocked(getGitCommits).mockResolvedValue([])
  })

  it('shows the children of a directory expanded from memory instead of loading forever', async () => {
    const treeState = useFileTreeStateStore()
    treeState.save('s1', {
      activeTab: 'all',
      searchQuery: '',
      selectedCommit: '',
      expandedTreePaths: ['src'],
      expandedChangeDirs: [],
      isStagedGroupExpanded: true,
      isUnstagedGroupExpanded: true,
    })

    const wrapper = mountSidebar()
    await flushPromises()

    expect(getFileTree).toHaveBeenCalledWith(1, 'src', undefined)
    expect(wrapper.text()).toContain('b.ts')
    expect(wrapper.find('[data-test="tree-node-loading"]').exists()).toBe(false)
  })

  it('reopens without any request while the loaded data is still valid', async () => {
    const first = mountSidebar()
    await flushPromises()
    first.unmount()

    vi.mocked(getFileTree).mockClear()
    vi.mocked(getGitStatus).mockClear()
    vi.mocked(getGitCommits).mockClear()

    const reopened = mountSidebar()
    await flushPromises()

    expect(getFileTree).not.toHaveBeenCalled()
    expect(getGitStatus).not.toHaveBeenCalled()
    expect(getGitCommits).not.toHaveBeenCalled()
    expect(reopened.text()).toContain('a.txt')
  })

  it('loads the tree again once the loaded data has been invalidated', async () => {
    const first = mountSidebar()
    await flushPromises()
    first.unmount()

    useFileTreeStateStore().invalidateData('s1')
    vi.mocked(getFileTree).mockClear()

    mountSidebar()
    await flushPromises()

    expect(getFileTree).toHaveBeenCalledWith(1, undefined, undefined)
  })
})
