import { Alert, Typography } from 'antd'
import { useParams } from 'react-router'

export function NewSessionPage() {
  const { projectId } = useParams()
  return (
    <div style={{ padding: 'calc(var(--space-unit) * 3)' }} data-test="new-session-page">
      <Typography.Title level={2}>新建会话</Typography.Title>
      <Alert
        type="info"
        showIcon
        title="P2 Shell 占位页"
        description={`项目 ${projectId ?? '全局'} 的新建会话表单将在 P4 核心 Chat 阶段接入。`}
      />
    </div>
  )
}

export function SessionPage() {
  const { projectId, sessionId } = useParams()
  return (
    <div style={{ padding: 'calc(var(--space-unit) * 3)' }} data-test="conversation">
      <Typography.Title level={2}>会话</Typography.Title>
      <Alert
        type="info"
        showIcon
        title="P2 Shell 占位页"
        description={`项目 ${projectId} / 会话 ${sessionId} 的消息流将在 P4 核心 Chat 阶段接入。`}
      />
    </div>
  )
}
