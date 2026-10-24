/* oxlint-disable react/set-state-in-effect -- data loading effects intentionally update state after async work */
import { FileTextOutlined, FolderOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { Alert, Button, Input, Segmented, Spin, Tag, Tree } from 'antd'
import type { DataNode, TreeProps } from 'antd/es/tree'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { getFileTree, getGitStatus } from '../../api/files'
import { searchProjectFiles } from '../../api/projects'
import type { FileTreeNode, GitFileChangeStatus, GitStatusItem } from './types'
import styles from './FileSidebar.module.css'

interface FileSidebarProps {
  projectId: number
  activePath?: string
  onSelect: (path: string) => void
}

interface TreeNode extends DataNode {
  key: string
  title: string
  isLeaf?: boolean
  children?: TreeNode[]
  loaded?: boolean
  isDir: boolean
  path: string
}

function toTree(nodes: FileTreeNode[]): TreeNode[] {
  return nodes.map((node) => ({
    key: node.path,
    title: node.name,
    path: node.path,
    isDir: node.isDir,
    isLeaf: !node.isDir,
    children: node.children ? toTree(node.children) : undefined,
    loaded: node.loaded,
  }))
}

function updateChildren(nodes: TreeNode[], path: string, children: TreeNode[]): TreeNode[] {
  return nodes.map((node) => {
    if (node.path === path) return { ...node, children, loaded: true }
    if (node.children) return { ...node, children: updateChildren(node.children, path, children) }
    return node
  })
}

const changeLabels: Record<GitFileChangeStatus, string> = {
  added: 'A',
  modified: 'M',
  deleted: 'D',
  untracked: 'U',
  renamed: 'R',
}

export function FileSidebar({ projectId, activePath, onSelect }: FileSidebarProps) {
  const [tab, setTab] = useState<'all' | 'changes'>('all')
  const [tree, setTree] = useState<TreeNode[]>([])
  const [changes, setChanges] = useState<GitStatusItem[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [search, setSearch] = useState('')
  const [results, setResults] = useState<Array<{ path: string; isDir: boolean }>>([])
  const [searching, setSearching] = useState(false)
  const timer = useRef<number | null>(null)
  const changeMap = useMemo(() => new Map(changes.map((item) => [item.path, item.status])), [changes])

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const [nextTree, nextChanges] = await Promise.all([
        getFileTree(projectId),
        getGitStatus(projectId).catch(() => []),
      ])
      setTree(toTree(nextTree))
      setChanges(nextChanges)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '加载文件失败')
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEffect(() => {
    if (timer.current !== null) window.clearTimeout(timer.current)
    const query = search.trim()
    if (query === '') {
      setResults([])
      return
    }
    setSearching(true)
    timer.current = window.setTimeout(() => {
      void searchProjectFiles(projectId, query, 50)
        .then((response) => setResults(response.files ?? []))
        .catch(() => setResults([]))
        .finally(() => setSearching(false))
    }, 150)
    return () => {
      if (timer.current !== null) window.clearTimeout(timer.current)
    }
  }, [projectId, search])

  const loadData: TreeProps['loadData'] = async (node) => {
    const target = node as unknown as TreeNode
    if (!target.isDir || target.loaded) return
    const children = await getFileTree(projectId, target.path)
    setTree((current) => updateChildren(current, target.path, toTree(children)))
  }

  const titleRender = (node: DataNode) => {
    const target = node as unknown as TreeNode
    const change = !target.isDir ? changeMap.get(target.path) : undefined
    return (
      <span className={styles.nodeTitle} data-test="file-tree-row" data-path={target.path}>
        <span className={styles.icon}>
          {target.isDir ? <FolderOutlined /> : <FileTextOutlined />}
        </span>
        <span className={styles.name}>{target.title as string}</span>
        {change && (
          <Tag className={styles.change} color={change === 'deleted' ? 'error' : 'warning'}>
            {changeLabels[change]}
          </Tag>
        )}
      </span>
    )
  }

  return (
    <aside className={styles.sidebar} aria-label="文件导航" data-test="file-sidebar">
      <div className={styles.head}>
        <Segmented
          size="small"
          value={tab}
          onChange={(value) => setTab(value as 'all' | 'changes')}
          options={[
            { label: `所有文件`, value: 'all' },
            { label: `变更 ${changes.length > 0 ? `(${changes.length})` : ''}`, value: 'changes' },
          ]}
        />
        <Button
          type="text"
          size="small"
          icon={<ReloadOutlined />}
          aria-label="刷新文件树"
          onClick={() => void refresh()}
        />
      </div>
      <Input
        allowClear
        size="small"
        prefix={<SearchOutlined />}
        placeholder="搜索文件"
        aria-label="搜索文件"
        value={search}
        onChange={(event) => setSearch(event.target.value)}
      />
      {error !== '' && <Alert type="error" showIcon title={error} />}
      {loading && tree.length === 0 ? (
        <div className={styles.state}>
          <Spin size="small" />
        </div>
      ) : search.trim() !== '' ? (
        <div className={styles.results}>
          {searching && <Spin size="small" />}
          {!searching && results.length === 0 && <p className={styles.empty}>没有匹配的文件。</p>}
          {results.map((item) => (
            <button
              key={item.path}
              type="button"
              className={styles.result}
              title={item.path}
              onClick={() => !item.isDir && onSelect(item.path)}
            >
              {item.isDir ? <FolderOutlined /> : <FileTextOutlined />}
              <span>{item.path}</span>
            </button>
          ))}
        </div>
      ) : tab === 'changes' ? (
        <div className={styles.results}>
          {changes.length === 0 && <p className={styles.empty}>没有未提交的变更。</p>}
          {changes.map((item) => (
            <button
              key={item.path}
              type="button"
              className={styles.result}
              title={item.path}
              onClick={() => onSelect(item.path)}
            >
              <Tag color={item.status === 'deleted' ? 'error' : 'warning'}>
                {changeLabels[item.status]}
              </Tag>
              <span>{item.path}</span>
            </button>
          ))}
        </div>
      ) : (
        <Tree
          className={styles.tree}
          treeData={tree}
          loadData={loadData}
          titleRender={titleRender}
          selectedKeys={activePath ? [activePath] : []}
          onSelect={(keys, info) => {
            const node = info.node as unknown as TreeNode
            if (!node.isDir && typeof keys[0] === 'string') onSelect(keys[0])
          }}
          data-test="file-tree"
        />
      )}
    </aside>
  )
}
