import { DeleteOutlined, FolderOpenOutlined, PlusOutlined, SettingOutlined } from '@ant-design/icons'
import { App, Button, Empty, Spin, Tag, Tooltip } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import type { Project } from '../../api/projects'
import type { AgentSession } from '../../api/sessions'
import { useAgentsStore } from '../../stores/agents'
import { useSessionsStore } from '../../stores/sessions'
import { BackendTag } from '../../shared/BackendTag'
import styles from './ProjectTree.module.css'

interface ProjectTreeProps {
  projects: Project[]
  activeProjectId?: number | null
  activeSessionId?: string | null
  search: string
  loading?: boolean
  onNewSession: (projectId: number) => void
  onOpenSettings: (projectId: number) => void
  onSelectSession: (projectId: number, sessionId: string) => void
}

function sessionTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString()
}

export function ProjectTree({
  projects,
  activeProjectId,
  activeSessionId,
  search,
  loading = false,
  onNewSession,
  onOpenSettings,
  onSelectSession,
}: ProjectTreeProps) {
  const { modal, message } = App.useApp()
  const sessions = useSessionsStore((state) => state.sessionsByProject)
  const sessionsLoading = useSessionsStore((state) => state.loading)
  const sessionsError = useSessionsStore((state) => state.error)
  const loadSessions = useSessionsStore((state) => state.loadSessions)
  const deleteSession = useSessionsStore((state) => state.deleteSession)
  const backendLabel = useAgentsStore((state) => state.backendLabel)
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  useEffect(() => {
    // The tree searches across project + session titles, so load every project's
    // index once. The store de-duplicates and skips already-loaded projects.
    for (const project of projects) {
      void loadSessions(project.id)
    }
  }, [projects, loadSessions])

  const query = search.trim().toLowerCase()

  const visibleProjects = useMemo(() => {
    if (query === '') return projects
    return projects.filter((project) => {
      if (project.name.toLowerCase().includes(query) || project.path.toLowerCase().includes(query)) {
        return true
      }
      return (sessions[project.id] ?? []).some((session) =>
        (session.title ?? '').toLowerCase().includes(query),
      )
    })
  }, [projects, query, sessions])

  const visibleSessions = useMemo(() => {
    const map: Record<number, AgentSession[]> = {}
    for (const project of projects) {
      const list = sessions[project.id] ?? []
      map[project.id] =
        query === ''
          ? list
          : list.filter((session) => (session.title ?? '').toLowerCase().includes(query))
    }
    return map
  }, [projects, query, sessions])

  function toggle(projectId: number) {
    setExpanded((current) => {
      const next = new Set(current)
      if (next.has(projectId)) next.delete(projectId)
      else next.add(projectId)
      return next
    })
  }

  function confirmDelete(project: Project, session: AgentSession) {
    modal.confirm({
      title: '删除会话',
      content: `确定删除会话「${session.title || session.id}」吗？仅从平台列表移除，Agent 原生会话仍保留。`,
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        const success = await deleteSession(project.id, session.id)
        if (!success) {
          message.error('删除失败，请重试')
          return
        }
        message.success('会话已删除')
      },
    })
  }

  if (loading && projects.length === 0) {
    return (
      <div className={styles.state}>
        <Spin size="small" />
      </div>
    )
  }

  if (projects.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有项目" />
  }

  return (
    <ul className={styles.tree} aria-label="项目与会话" data-test="project-tree">
      {visibleProjects.map((project) => {
        const open = expanded.has(project.id) || query !== ''
        const list = visibleSessions[project.id] ?? []
        return (
          <li key={project.id} className={styles.project}>
            <div
              className={styles.projectRow}
              data-test="project-row"
              data-project-id={project.id}
              data-selected={project.id === activeProjectId ? 'true' : undefined}
            >
              <button
                type="button"
                className={styles.projectToggle}
                aria-expanded={open}
                onClick={() => toggle(project.id)}
              >
                <span className={styles.caret}>{open ? '▾' : '▸'}</span>
                <FolderOpenOutlined />
                <span className={styles.projectName}>{project.name}</span>
                <span className={styles.count}>{(sessions[project.id] ?? []).length}</span>
              </button>
              <Tooltip title="新建会话">
                <Button
                  type="text"
                  size="small"
                  aria-label={`新建会话：${project.name}`}
                  icon={<PlusOutlined />}
                  onClick={() => onNewSession(project.id)}
                />
              </Tooltip>
              <Tooltip title="项目设置">
                <Button
                  type="text"
                  size="small"
                  aria-label={`项目设置：${project.name}`}
                  icon={<SettingOutlined />}
                  onClick={() => onOpenSettings(project.id)}
                />
              </Tooltip>
            </div>

            {open && (
              <ul className={styles.sessions}>
                {sessionsError !== '' && (
                  <li className={styles.error} role="alert">
                    {sessionsError}
                  </li>
                )}
                {sessionsLoading && list.length === 0 && (
                  <li className={styles.state}>
                    <Spin size="small" />
                  </li>
                )}
                {list.length === 0 && !sessionsLoading ? (
                  <li className={styles.empty}>该项目还没有会话。</li>
                ) : (
                  list.map((session) => (
                    <li
                      key={session.id}
                      className={styles.session}
                      data-test="session-row"
                      data-session-id={session.id}
                      data-selected={session.id === activeSessionId ? 'true' : undefined}
                    >
                      <button
                        type="button"
                        className={styles.sessionOpen}
                        aria-label={session.title || '未命名会话'}
                        onClick={() => onSelectSession(project.id, session.id)}
                      >
                        <span className={styles.sessionTop}>
                          <span className={styles.sessionTitle}>{session.title || '未命名会话'}</span>
                        </span>
                        <span className={styles.sessionMeta}>
                          <BackendTag backend={session.backend} label={backendLabel(session.backend)} />
                          {session.model && <span className={styles.model}>{session.model}</span>}
                          <span className={styles.time}>{sessionTime(session.updatedAt)}</span>
                        </span>
                      </button>
                      <Tooltip title="删除会话">
                        <button
                          type="button"
                          className={styles.delete}
                          aria-label={`删除会话：${session.title || session.id}`}
                          onClick={(event) => {
                            event.stopPropagation()
                            confirmDelete(project, session)
                          }}
                        >
                          <DeleteOutlined />
                        </button>
                      </Tooltip>
                    </li>
                  ))
                )}
              </ul>
            )}
          </li>
        )
      })}
      {visibleProjects.length === 0 && <li className={styles.empty}>没有匹配的项目。</li>}
      <li className={styles.hint}>
        <Tag bordered={false}>{projects.length} 个项目</Tag>
      </li>
    </ul>
  )
}
