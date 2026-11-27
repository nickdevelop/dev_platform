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

  it('renders GitCommitBar for working tree and switches to GitCommitDetailBar for history commit', async () => {
    vi.mocked(getGitCommits).mockResolvedValue([
      {
        hash: 'c123456789012345678901234567890123456789',
        shortHash: 'c123456',
        message: 'fix: resolve race condition in worker pool',
        author: 'Alice',
        date: '2026-11-19 12:00:00',
        timestamp: 1795060800,
      },
    ])

    const treeState = useFileTreeStateStore()
    treeState.save('s-commit-test', {
      activeTab: 'changes',
      searchQuery: '',
      selectedCommit: '',
      expandedTreePaths: [],
      expandedChangeDirs: [],
      isStagedGroupExpanded: true,
      isUnstagedGroupExpanded: true,
    })

    const wrapper = mountSidebar('s-commit-test')
    await flushPromises()

    // In working tree mode (uncommitted changes), GitCommitBar is displayed
    expect(wrapper.findComponent({ name: 'GitCommitBar' }).exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'GitCommitDetailBar' }).exists()).toBe(false)

    // Select history commit via the commit selector
    const commitSelect = wrapper.find('select.commit-select')
    expect(commitSelect.exists()).toBe(true)
    await commitSelect.setValue('c123456789012345678901234567890123456789')
    await flushPromises()

    // Now GitCommitBar is replaced with GitCommitDetailBar showing history commit message
    expect(wrapper.findComponent({ name: 'GitCommitBar' }).exists()).toBe(false)
    const detailBar = wrapper.findComponent({ name: 'GitCommitDetailBar' })
    expect(detailBar.exists()).toBe(true)
    expect(detailBar.text()).toContain('fix: resolve race condition in worker pool')
    expect(detailBar.text()).toContain('Alice')
    expect(detailBar.text()).toContain('c123456')

    // Click reset button in detail bar
    const resetBtn = detailBar.find('[data-test="btn-reset-to-working"]')
    expect(resetBtn.exists()).toBe(true)
    await resetBtn.trigger('click')
    await flushPromises()

    // Back to working tree mode with GitCommitBar
    expect(wrapper.findComponent({ name: 'GitCommitBar' }).exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'GitCommitDetailBar' }).exists()).toBe(false)
  })

  it('isolates cache keys between changes-only and files-only instances of the same session', async () => {
    const firstFiles = mount(FileSidebar, { props: { projectId: 1, sessionId: 's-split', viewMode: 'files-only' } })
    await flushPromises()
    const srcRow = firstFiles.findAll('.file-tree-row').find((r) => r.text().includes('src'))
    await srcRow?.trigger('click')
    await flushPromises()
    firstFiles.unmount()

    const firstChanges = mount(FileSidebar, { props: { projectId: 1, sessionId: 's-split', viewMode: 'changes-only' } })
    await flushPromises()
    firstChanges.unmount()

    const store = useFileTreeStateStore()
    expect(store.snapshotOf('s-split:files')?.expandedTreePaths).toContain('src')
    expect(store.snapshotOf('s-split:changes')?.activeTab).toBe('changes')

    store.retainOnly([])
    expect(store.snapshotOf('s-split:files')).toBeUndefined()
    expect(store.snapshotOf('s-split:changes')).toBeUndefined()
  })
})
