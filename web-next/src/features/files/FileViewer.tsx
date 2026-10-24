/* oxlint-disable react/set-state-in-effect -- file loading effect intentionally drives loading state */
import { FileTextOutlined, ReloadOutlined } from '@ant-design/icons'
import { Alert, Button, Empty, Spin, Typography } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { getFileContent, getFileRawUrl } from '../../api/files'
import { renderMarkdown } from '../../shared/markdown'
import type { FileContentResponse } from './types'
import styles from './FileViewer.module.css'

interface FileViewerProps {
  projectId: number
  path: string
}

function extensionOf(path: string): string {
  return path.split('.').pop()?.toLowerCase() ?? ''
}

function isImage(path: string): boolean {
  return ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp'].includes(extensionOf(path))
}

function isMarkdown(path: string): boolean {
  return ['md', 'markdown'].includes(extensionOf(path))
}

export function FileViewer({ projectId, path }: FileViewerProps) {
  const [data, setData] = useState<FileContentResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!path) return
    let cancelled = false
    setLoading(true)
    setError('')
    void getFileContent(projectId, path)
      .then((content) => {
        if (!cancelled) setData(content)
      })
      .catch((reason: unknown) => {
        if (cancelled) return
        setError(reason instanceof Error ? reason.message : '读取文件失败')
        setData(null)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [projectId, path])

  const lines = useMemo(() => (data?.content ?? '').split('\n'), [data?.content])
  const image = path !== '' && isImage(path)

  if (!path) {
    return (
      <div className={styles.viewer} data-test="file-viewer">
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="在左侧文件树中点击文件进行查看" />
      </div>
    )
  }

  return (
    <section className={styles.viewer} data-test="file-viewer" aria-label={`文件预览 ${path}`}>
      <header className={styles.header}>
        <div className={styles.title}>
          <FileTextOutlined />
          <span title={path}>{path}</span>
        </div>
        {data && (
          <Typography.Text type="secondary">
            {data.size} B · {data.lines} 行
          </Typography.Text>
        )}
      </header>

      {loading && (
        <div className={styles.state}>
          <Spin />
        </div>
      )}

      {!loading && error !== '' && (
        <div className={styles.state}>
          <Alert
            type="error"
            showIcon
            title={error}
            action={
              <Button size="small" icon={<ReloadOutlined />} onClick={() => setError('')}>
                返回
              </Button>
            }
          />
        </div>
      )}

      {!loading && error === '' && data && image && (
        <div className={styles.imageWrap}>
          <img src={getFileRawUrl(projectId, path)} alt={path} />
        </div>
      )}

      {!loading && error === '' && data && !image && data.isBinary && (
        <div className={styles.state}>
          <Alert type="info" showIcon title="该文件是二进制内容，无法以文本预览。" />
        </div>
      )}

      {!loading && error === '' && data && !image && !data.isBinary && isMarkdown(path) && (
        <div className={styles.markdownScroll}>
          <article
            className={`${styles.markdown} markdown-body`}
            // renderMarkdown runs marked + DOMPurify; only sanitized HTML reaches the DOM.
            dangerouslySetInnerHTML={{ __html: renderMarkdown(data.content ?? '') }}
          />
        </div>
      )}

      {!loading &&
        error === '' &&
        data &&
        !image &&
        !data.isBinary &&
        !isMarkdown(path) && (
          <div className={styles.codeScroll}>
            <pre className={styles.code}>
              {lines.map((line, index) => (
                <span key={index} className={styles.line}>
                  <span className={styles.lineNumber}>{index + 1}</span>
                  <span className={styles.lineText}>{line || ' '}</span>
                </span>
              ))}
            </pre>
          </div>
        )}

      {data?.truncated && (
        <Alert
          className={styles.truncated}
          type="warning"
          showIcon
          title="文件较大，仅显示部分内容。"
        />
      )}
    </section>
  )
}
