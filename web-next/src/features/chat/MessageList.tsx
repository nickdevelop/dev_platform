import { Alert, Tag, Typography } from 'antd'
import type { ChatEntry } from '../../features/chat/state/entry'
import { renderMarkdown } from '../../shared/markdown'
import styles from './MessageList.module.css'

interface MessageListProps {
  entries: ChatEntry[]
  running: boolean
}

function imageSrc(entry: ChatEntry): string[] {
  return (entry.images ?? []).map((image) =>
    image.data.startsWith('data:') ? image.data : `data:${image.mediaType};base64,${image.data}`,
  )
}

function toolText(entry: ChatEntry): string {
  if (entry.input === undefined) return ''
  try {
    return JSON.stringify(entry.input, null, 2)
  } catch {
    return String(entry.input)
  }
}

export function MessageList({ entries, running }: MessageListProps) {
  if (entries.length === 0) {
    return (
      <div className={styles.empty} data-test="message-empty">
        <Typography.Text type="secondary">还没有消息，发送第一条消息开始会话。</Typography.Text>
      </div>
    )
  }

  return (
    <ol className={styles.list} role="log" aria-live="polite" aria-label="会话消息">
      {entries.map((entry) => {
        if (entry.kind === 'message') {
          const assistant = entry.role !== 'user'
          return (
            <li key={entry.id} className={styles.item} data-test="message-entry">
              <div className={`${styles.bubble} ${assistant ? styles.agent : styles.user}`} data-test="bubble">
                {imageSrc(entry).length > 0 && (
                  <div className={styles.images}>
                    {imageSrc(entry).map((src, index) => (
                      <img key={index} src={src} alt="附件" />
                    ))}
                  </div>
                )}
                {entry.text !== '' &&
                  (assistant ? (
                    <div
                      className={`${styles.markdown} markdown-body`}
                      data-test="message-text"
                      dangerouslySetInnerHTML={{ __html: renderMarkdown(entry.text) }}
                    />
                  ) : (
                    <span className={styles.text} data-test="message-text">
                      {entry.text}
                    </span>
                  ))}
                {entry.streaming && <span className={styles.cursor}>…</span>}
              </div>
              {assistant && (
                <div className={styles.meta}>
                  {entry.model && <Tag>{entry.model.id}</Tag>}
                  {entry.timestamp && (
                    <span>{new Date(entry.timestamp).toLocaleTimeString()}</span>
                  )}
                </div>
              )}
            </li>
          )
        }

        if (entry.kind === 'thinking') {
          return (
            <li key={entry.id} className={styles.item}>
              <details className={styles.thinking} data-test="thinking">
                <summary>
                  思考 {entry.streaming ? '（生成中…）' : ''}
                </summary>
                <pre>{entry.text}</pre>
              </details>
            </li>
          )
        }

        if (entry.kind === 'tool_call' || entry.kind === 'tool_result') {
          return (
            <li key={entry.id} className={styles.item}>
              <details className={styles.tool} data-test="tool-call">
                <summary>
                  <Tag color={entry.isError ? 'error' : 'default'}>
                    {entry.kind === 'tool_call' ? entry.name || '工具调用' : '工具结果'}
                  </Tag>
                </summary>
                {toolText(entry) && <pre>{toolText(entry)}</pre>}
                {entry.text && <pre>{entry.text}</pre>}
              </details>
            </li>
          )
        }

        return (
          <li key={entry.id} className={styles.item}>
            <Alert
              type={entry.kind === 'error' ? 'error' : 'info'}
              showIcon
              title={entry.text}
            />
          </li>
        )
      })}
      {running && (
        <li className={styles.item} data-test="running-entry">
          <div className={styles.running}>正在运行…</div>
        </li>
      )}
    </ol>
  )
}
