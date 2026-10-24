import { Empty, Typography } from 'antd'
import type { ChatEntry } from './state/entry'
import styles from './ActivityPanel.module.css'

interface ActivityPanelProps {
  entries: ChatEntry[]
}

const labels: Record<string, string> = {
  thinking: '思考',
  tool_call: '工具调用',
  tool_result: '工具结果',
  error: '错误',
  message: '消息',
}

export function ActivityPanel({ entries }: ActivityPanelProps) {
  const activity = entries.filter((entry) => entry.kind === 'thinking' || entry.kind === 'tool_call' || entry.kind === 'tool_result' || entry.kind === 'error')
  if (activity.length === 0) {
    return (
      <div className={styles.empty}>
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有活动" />
      </div>
    )
  }
  return (
    <div className={styles.panel} data-test="activity-panel">
      <Typography.Text strong>会话活动</Typography.Text>
      <div className={styles.list}>
        {activity.map((entry) => (
          <article key={entry.id} className={styles.item}>
            <header>
              <span className={styles.kind}>{labels[entry.kind] ?? entry.kind}</span>
              {entry.timestamp && (
                <time className={styles.time}>{new Date(entry.timestamp).toLocaleTimeString()}</time>
              )}
            </header>
            <p className={styles.text}>{entry.text || '（无文本）'}</p>
          </article>
        ))}
      </div>
    </div>
  )
}
