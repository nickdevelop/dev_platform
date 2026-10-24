import { Card, Typography } from 'antd'
import { useAuthStore } from '../stores/auth'
import { useProjectsStore } from '../stores/projects'
import { useSettingsStore } from '../stores/settings'

export function HomePage() {
  const authenticated = useAuthStore((state) => state.authenticated)
  const projects = useProjectsStore((state) => state.projects)
  const appearance = useSettingsStore((state) => state.appearance)

  return (
    <div style={{ padding: 'calc(var(--space-unit) * 3)' }}>
      <Typography.Title level={2}>工作台</Typography.Title>
      <Typography.Paragraph type="secondary">
        P2 已完成项目树、Tabs 与项目设置；核心会话界面将在 P4 接入。
      </Typography.Paragraph>
      <Card size="small" title="当前状态">
        <Typography.Paragraph>
          登录态：{authenticated ? '已认证' : '未认证'}
        </Typography.Paragraph>
        <Typography.Paragraph>项目数：{projects.length}</Typography.Paragraph>
        <Typography.Paragraph>
          外观：{appearance.theme} / {appearance.density} / {appearance.fontSize}px
        </Typography.Paragraph>
      </Card>
    </div>
  )
}
