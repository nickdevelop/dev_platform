import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GitCommitBar from '../features/files/GitCommitBar.vue'
import BranchSelector from '../features/files/BranchSelector.vue'
import ChangeTree from '../features/files/ChangeTree.vue'
import * as filesApi from '../api/files'

describe('GitCommitBar.vue', () => {
  it('disables commit button when message is empty or no changes exist', () => {
    const wrapper = mount(GitCommitBar, {
      props: {
        projectId: 1,
        stagedCount: 0,
        unstagedCount: 0,
      },
    })

    const button = wrapper.find('button')
    expect(button.attributes('disabled')).toBeDefined()
  })

  it('enables commit button when message is typed and changes exist', async () => {
    const wrapper = mount(GitCommitBar, {
      props: {
        projectId: 1,
        stagedCount: 1,
        unstagedCount: 0,
      },
    })

    const textarea = wrapper.find('textarea')
    await textarea.setValue('feat: initial commit')

    const button = wrapper.find('button')
    expect(button.attributes('disabled')).toBeUndefined()
  })

  it('calls commitGitChanges and emits committed event on button click', async () => {
    const commitSpy = vi.spyOn(filesApi, 'commitGitChanges').mockResolvedValue({
      hash: 'abc1234567890',
      shortHash: 'abc1234',
      message: 'feat: add login',
      author: 'Author',
      date: '2026-10-31T12:00:00Z',
      timestamp: 1793448000,
    })

    const wrapper = mount(GitCommitBar, {
      props: {
        projectId: 1,
        stagedCount: 2,
        unstagedCount: 0,
      },
    })

    await wrapper.find('textarea').setValue('feat: add login')
    await wrapper.find('button').trigger('click')

    expect(commitSpy).toHaveBeenCalledWith(1, 'feat: add login')
    expect(wrapper.emitted('committed')).toBeTruthy()
  })

  it('emits stageAllAndCommit when stagedCount is 0 but unstagedCount > 0', async () => {
    const wrapper = mount(GitCommitBar, {
      props: {
        projectId: 1,
        stagedCount: 0,
        unstagedCount: 3,
      },
    })

    await wrapper.find('textarea').setValue('feat: auto stage all')
    await wrapper.find('button').trigger('click')

    expect(wrapper.emitted('stageAllAndCommit')).toBeTruthy()
    expect(wrapper.emitted('stageAllAndCommit')?.[0]).toEqual(['feat: auto stage all'])
  })
})

describe('BranchSelector.vue', () => {
  it('renders current branch and opens dropdown on click', async () => {
    vi.spyOn(filesApi, 'getGitBranches').mockResolvedValue({
      branches: ['main', 'feat/login', 'bugfix/nav'],
      current: 'main',
    })

    const wrapper = mount(BranchSelector, {
      props: {
        projectId: 1,
        currentBranch: 'main',
      },
    })

    // Click trigger button to open
    const trigger = wrapper.find('button')
    expect(trigger.text()).toContain('main')

    await trigger.trigger('click')
    expect(wrapper.find('input').exists()).toBe(true)
  })

  it('triggers checkoutGitBranch on branch selection', async () => {
    vi.spyOn(filesApi, 'getGitBranches').mockResolvedValue({
      branches: ['main', 'feat/login'],
      current: 'main',
    })
    const checkoutSpy = vi.spyOn(filesApi, 'checkoutGitBranch').mockResolvedValue({ ok: true })

    const wrapper = mount(BranchSelector, {
      props: {
        projectId: 1,
        currentBranch: 'main',
      },
    })

    await wrapper.find('button').trigger('click')
    const branchItems = wrapper.findAll('.max-h-48 button')
    expect(branchItems.length).toBe(2)

    await branchItems[1].trigger('click')
    expect(checkoutSpy).toHaveBeenCalledWith(1, 'feat/login', false)
    expect(wrapper.emitted('branchChanged')).toBeTruthy()
  })
})

describe('ChangeTree.vue with Actions', () => {
  it('renders stage, unstage and discard action buttons when showActions is true', async () => {
    const wrapper = mount(ChangeTree, {
      props: {
        nodes: [
          { name: 'app.vue', path: 'app.vue', isDir: false, status: 'modified' },
        ],
        expandedPaths: new Set(),
        showActions: true,
        isStaged: false,
      },
    })

    const actionButtons = wrapper.findAll('.file-actions button')
    expect(actionButtons.length).toBe(2) // Stage and Discard

    await actionButtons[0].trigger('click')
    expect(wrapper.emitted('stage')?.[0]).toEqual(['app.vue'])

    await actionButtons[1].trigger('click')
    expect(wrapper.emitted('discard')?.[0]).toEqual(['app.vue'])
  })

  it('renders unstage action button when isStaged is true', async () => {
    const wrapper = mount(ChangeTree, {
      props: {
        nodes: [
          { name: 'app.vue', path: 'app.vue', isDir: false, status: 'modified' },
        ],
        expandedPaths: new Set(),
        showActions: true,
        isStaged: true,
      },
    })

    const actionButtons = wrapper.findAll('.file-actions button')
    expect(actionButtons.length).toBe(1) // Unstage

    await actionButtons[0].trigger('click')
    expect(wrapper.emitted('unstage')?.[0]).toEqual(['app.vue'])
  })
})
