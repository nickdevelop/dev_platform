import { Empty, Typography } from 'antd'
import { useState } from 'react'
import { useParams } from 'react-router'
import { useProjectsStore } from '../../stores/projects'
import { FileSidebar } from './FileSidebar'
import { FileViewer } from './FileViewer'
import styles from './FilesPage.module.css'

export function FilesPage() {
  const { projectId } = useParams()
  const id = Number(projectId)
  const project = useProjectsStore((state) => state.byId(id))
  const [activePath, setActivePath] = useState('')

  if (!project) {
    return (
      <div className={styles.page}>
        <Empty description="项目不存在或已移除" />
      </div>
    )
  }

  return (
    <div className={styles.page} data-test="files-page">
      <header className={styles.header}>
        <div>
          <Typography.Title level={3}>{project.name} · 文件</Typography.Title>
          <Typography.Text type="secondary">{project.path}</Typography.Text>
        </div>
      </header>
      <div className={styles.body}>
        <FileSidebar projectId={id} activePath={activePath} onSelect={setActivePath} />
        <FileViewer projectId={id} path={activePath} />
      </div>
    </div>
  )
}
