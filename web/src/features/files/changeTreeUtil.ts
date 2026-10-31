import type { ChangeTreeNode, GitFileChangeStatus, GitStatusItem } from './types'

export function buildChangeTree(items: GitStatusItem[]): {
  tree: ChangeTreeNode[]
  allDirPaths: string[]
} {
  interface IntermediateDir {
    name: string
    path: string
    isDir: true
    children: Map<string, IntermediateDir | ChangeTreeNode>
  }

  const root: IntermediateDir = {
    name: '',
    path: '',
    isDir: true,
    children: new Map(),
  }

  const allDirPathsSet = new Set<string>()

  for (const item of items) {
    const cleanPath = item.path.replace(/\\/g, '/').replace(/^\/+/, '')
    if (!cleanPath) continue

    const parts = cleanPath.split('/')
    let current = root

    for (let i = 0; i < parts.length; i++) {
      const part = parts[i]
      const isLast = i === parts.length - 1
      const currentPath = parts.slice(0, i + 1).join('/')

      if (isLast) {
        current.children.set(part, {
          name: part,
          path: currentPath,
          isDir: false,
          status: item.status,
        })
      } else {
        allDirPathsSet.add(currentPath)
        let dirNode = current.children.get(part) as IntermediateDir | undefined
        if (!dirNode || !dirNode.isDir) {
          dirNode = {
            name: part,
            path: currentPath,
            isDir: true,
            children: new Map(),
          }
          current.children.set(part, dirNode)
        }
        current = dirNode
      }
    }
  }

  function convert(node: IntermediateDir): ChangeTreeNode[] {
    const result: ChangeTreeNode[] = []
    for (const child of node.children.values()) {
      if (child.isDir) {
        const intermediate = child as IntermediateDir
        const convertedChildren = convert(intermediate)
        let totalCount = 0
        for (const c of convertedChildren) {
          totalCount += c.isDir ? (c.changeCount ?? 0) : 1
        }
        result.push({
          name: intermediate.name,
          path: intermediate.path,
          isDir: true,
          children: convertedChildren,
          changeCount: totalCount,
        })
      } else {
        result.push(child)
      }
    }

    result.sort((a, b) => {
      if (a.isDir !== b.isDir) {
        return a.isDir ? -1 : 1
      }
      return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
    })

    return result
  }

  return {
    tree: convert(root),
    allDirPaths: Array.from(allDirPathsSet),
  }
}

export function getChangeStatusLabel(status?: GitFileChangeStatus | string): { text: string; label: string; class: string } {
  switch (status) {
    case 'added':
      return { text: 'A', label: '新增', class: 'git-added' }
    case 'modified':
      return { text: 'M', label: '修改', class: 'git-modified' }
    case 'deleted':
      return { text: 'D', label: '删除', class: 'git-deleted' }
    case 'untracked':
      return { text: 'U', label: '未跟踪', class: 'git-untracked' }
    case 'renamed':
      return { text: 'R', label: '重命名', class: 'git-renamed' }
    default:
      return { text: 'M', label: '变更', class: 'git-modified' }
  }
}
