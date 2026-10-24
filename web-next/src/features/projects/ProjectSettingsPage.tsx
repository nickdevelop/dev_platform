import { Button, Empty, Space, Typography } from 'antd'
import { useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import type { Project } from '../../api/projects'
import { useProjectsStore } from '../../stores/projects'
import { ProjectManageModal, type ProjectDialogKind } from './ProjectManageModal'
import styles from './ProjectSettingsPage.module.css'

export function ProjectSettingsPage() {
  const { projectId } = useParams()
  const navigate = useNavigate()
  const id = Number(projectId)
  const project = useProjectsStore((state) => state.byId(id))
  const load = useProjectsStore((state) => state.load)
  const [dialog, setDialog] = useState<ProjectDialogKind | null>(null)

  if (!project) {
    return (
      <div className={styles.page}>
        <Empty description="项目不存在或已移除" />
      </div>
    )
  }

  const manage = (kind: ProjectDialogKind) => setDialog(kind)

  return (
    <section className={styles.page} aria-label="项目设置">
      <header className={styles.header}>
        <div>
          <Typography.Title level={2}>{project.name}</Typography.Title>
          <Typography.Text type="secondary">{project.path}</Typography.Text>
        </div>
        <Space wrap>
          <Button onClick={() => navigate(`/projects/${id}/files`)}>浏览文件</Button>
          <Button onClick={() => manage('rename-name')}>修改名称</Button>
          <Button onClick={() => manage('rename')}>重命名</Button>
          <Button onClick={() => manage('move')}>移动</Button>
          <Button danger onClick={() => manage('unregister')}>
            解除登记
          </Button>
        </Space>
      </header>
      <div className={styles.body}>
        <Typography.Paragraph type="secondary">
          项目设置目前只包含目录与登记信息。文件树、Git 状态与会话工作台将在后续阶段接入。
        </Typography.Paragraph>
      </div>

      {dialog && (
        <ProjectManageModal
          key={dialog}
          kind={dialog}
          project={project as Project}
          onClose={() => setDialog(null)}
          onSubmitted={() => void load()}
        />
      )}
    </section>
  )
}
