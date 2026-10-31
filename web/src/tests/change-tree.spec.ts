import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ChangeTree from '../features/files/ChangeTree.vue'
import { buildChangeTree, getChangeStatusLabel } from '../features/files/changeTreeUtil'
import type { GitStatusItem } from '../features/files/types'

describe('changeTreeUtil', () => {
  it('builds a hierarchical directory tree from flat git status items', () => {
    const items: GitStatusItem[] = [
      { path: 'src/components/Header.vue', status: 'modified' },
      { path: 'src/components/Footer.vue', status: 'added' },
      { path: 'src/main.ts', status: 'modified' },
      { path: 'README.md', status: 'modified' },
      { path: '.env.example', status: 'untracked' },
    ]

    const { tree, allDirPaths } = buildChangeTree(items)

    expect(allDirPaths).toContain('src')
    expect(allDirPaths).toContain('src/components')

    // Root should have: src (dir), .env.example, README.md
    expect(tree.length).toBe(3)
    expect(tree[0].name).toBe('src')
    expect(tree[0].isDir).toBe(true)
    expect(tree[0].changeCount).toBe(3) // 2 in components + 1 main.ts

    const srcChildren = tree[0].children || []
    expect(srcChildren.length).toBe(2)
    expect(srcChildren[0].name).toBe('components')
    expect(srcChildren[0].isDir).toBe(true)
    expect(srcChildren[0].changeCount).toBe(2)
    expect(srcChildren[1].name).toBe('main.ts')
    expect(srcChildren[1].isDir).toBe(false)
    expect(srcChildren[1].status).toBe('modified')
  })

  it('maps git status codes to readable labels and classes', () => {
    expect(getChangeStatusLabel('added').text).toBe('A')
    expect(getChangeStatusLabel('modified').text).toBe('M')
    expect(getChangeStatusLabel('deleted').text).toBe('D')
    expect(getChangeStatusLabel('untracked').text).toBe('U')
    expect(getChangeStatusLabel('renamed').text).toBe('R')
  })
})

describe('ChangeTree.vue', () => {
  const sampleItems: GitStatusItem[] = [
    { path: 'src/app.ts', status: 'modified' },
    { path: 'src/utils/math.ts', status: 'added' },
    { path: 'package.json', status: 'modified' },
  ]

  it('renders tree nodes with badges and handles node click', async () => {
    const { tree, allDirPaths } = buildChangeTree(sampleItems)
    const expanded = new Set(allDirPaths)

    const wrapper = mount(ChangeTree, {
      props: {
        nodes: tree,
        expandedPaths: expanded,
      },
    })

    // Expect directories and files rendered
    expect(wrapper.text()).toContain('src')
    expect(wrapper.text()).toContain('app.ts')
    expect(wrapper.text()).toContain('package.json')

    // Click on file triggers select event
    const fileRow = wrapper.findAll('.change-tree-row').find((r) => r.text().includes('package.json'))
    await fileRow?.trigger('click')

    expect(wrapper.emitted('select')).toBeTruthy()
    expect(wrapper.emitted('select')?.[0]).toEqual(['package.json'])
  })

  it('handles folder toggle click', async () => {
    const { tree, allDirPaths } = buildChangeTree(sampleItems)
    const expanded = new Set(allDirPaths)

    const wrapper = mount(ChangeTree, {
      props: {
        nodes: tree,
        expandedPaths: expanded,
      },
    })

    const dirRow = wrapper.findAll('.change-tree-row').find((r) => r.text().includes('src'))
    await dirRow?.trigger('click')

    expect(wrapper.emitted('toggle')).toBeTruthy()
    expect(wrapper.emitted('toggle')?.[0]).toEqual(['src'])
  })
})
