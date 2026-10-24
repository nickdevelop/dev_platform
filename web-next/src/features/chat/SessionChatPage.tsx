/* oxlint-disable react/set-state-in-effect -- syncing selected model with backend snapshot */
import { Alert, Button, Empty, Select, Space, Spin, Tag, Typography } from 'antd'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useInteractionsStore } from '../../stores/interactions'
import { getPersistedApprovalMode, resolveAutopilotForm, setPersistedApprovalMode, type ApprovalMode } from './interactions/approvalMode'
import { useSessionsStore } from '../../stores/sessions'
import { useBottomFollow } from '../../shared/useBottomFollow'
import { listAgentBackendModels, type ModelInfo } from '../../api/agents'
import { getFileTree } from '../../api/files'
import type { FileTreeNode } from '../files/types'
import { ChatComposer } from './ChatComposer'
import { FormCards, PermissionCards } from './InteractionCards'
import { MessageList } from './MessageList'
import { FileSidebar } from '../files/FileSidebar'
import { FileViewer } from '../files/FileViewer'
import { ActivityPanel } from './ActivityPanel'
import styles from './SessionChatPage.module.css'

import type { ImageAttachment } from '../../api/messageTypes'
import type { ChatEntry } from './state/entry'
import type { FormInfo, PermissionRequest } from '../../api/interactions'

const EMPTY_ENTRIES: ChatEntry[] = []
const EMPTY_PERMISSIONS: PermissionRequest[] = []
const EMPTY_FORMS: FormInfo[] = []

const statusLabels: Record<string, string> = {
  idle: '空闲',
  connecting: '连接中',
  open: '已连接',
  reconnecting: '重连中',
  error: '连接错误',
  ended: '已结束',
}

export function SessionChatPage() {
  const { projectId: projectParam, sessionId = '' } = useParams()
  const projectId = Number(projectParam)
  const navigate = useNavigate()
  const [sendError, setSendError] = useState('')
  const [filesOpen, setFilesOpen] = useState(false)
  const [activityOpen, setActivityOpen] = useState(false)
  const [commandNotice, setCommandNotice] = useState('')
  const [activeFile, setActiveFile] = useState('')
  const [models, setModels] = useState<ModelInfo[]>([])
  const [mentionOptions, setMentionOptions] = useState<string[]>([])
  const [selectedModel, setSelectedModel] = useState('')
  const [approvalMode, setApprovalMode] = useState<ApprovalMode>(() => getPersistedApprovalMode())
  const handledPermissions = useRef(new Set<string>())
  const handledForms = useRef(new Set<string>())
  const opened = useRef('')

  const sessions = useSessionsStore((state) => state.sessions)
  const loadSessions = useSessionsStore((state) => state.loadSessions)
  const openSession = useSessionsStore((state) => state.openSession)
  const sendMessage = useSessionsStore((state) => state.sendMessage)
  const stopSession = useSessionsStore((state) => state.stopSession)
  const loadHistory = useSessionsStore((state) => state.loadHistory)
  const session = useSessionsStore((state) => state.sessionById(sessionId))
  const entries = useSessionsStore((state) => state.messages[sessionId]) ?? EMPTY_ENTRIES
  const status = useSessionsStore((state) => state.streamStatus[sessionId]) ?? 'idle'
  const turns = useSessionsStore((state) => state.sessionTurns)
  const historyError = useSessionsStore((state) => state.historyError[sessionId]) ?? ''
  const historyTruncated = useSessionsStore((state) => state.historyTruncated[sessionId]) ?? false
  const error = useSessionsStore((state) => state.error)
  const permissions = useInteractionsStore((state) => state.permissions[sessionId]) ?? EMPTY_PERMISSIONS
  const forms = useInteractionsStore((state) => state.forms[sessionId]) ?? EMPTY_FORMS
  const replyPermission = useInteractionsStore((state) => state.replyPermission)
  const replyForm = useInteractionsStore((state) => state.replyForm)

  useEffect(() => {
    const key = `${projectId}:${sessionId}`
    if (opened.current === key) return
    opened.current = key
    void (async () => {
      await loadSessions(projectId)
      await openSession(projectId, sessionId)
    })()
  }, [projectId, sessionId, loadSessions, openSession])

  const running = turns[sessionId] === 'running' || session?.turn === 'running'
  const followKey = `${entries.length}:${entries[entries.length - 1]?.text.length ?? 0}`
  const { scrollerRef, showJump, onScroll, scrollToBottom } = useBottomFollow(followKey)

  const title = useMemo(() => session?.title || sessionId, [session?.title, sessionId])

  useEffect(() => {
    let cancelled = false
    void getFileTree(projectId)
      .then((tree) => {
        if (cancelled) return
        const paths: string[] = []
        const walk = (nodes: FileTreeNode[]) => {
          for (const node of nodes) {
            if (node.isDir) walk(node.children ?? [])
            else if (paths.length < 300) paths.push(node.path)
          }
        }
        walk(tree)
        setMentionOptions(paths)
      })
      .catch(() => {
        if (!cancelled) setMentionOptions([])
      })
    return () => {
      cancelled = true
    }
  }, [projectId])

  useEffect(() => {
    if (!session?.backend) return
    let cancelled = false
    void listAgentBackendModels(session.backend)
      .then((result) => {
        if (!cancelled) setModels(result.models)
      })
      .catch(() => {
        if (!cancelled) setModels([])
      })
    return () => {
      cancelled = true
    }
  }, [session?.backend])

  useEffect(() => {
    setSelectedModel(session?.model ?? '')
  }, [session?.model])

  useEffect(() => {
    if (approvalMode === 'default') return
    for (const permission of permissions) {
      if (handledPermissions.current.has(permission.id)) continue
      handledPermissions.current.add(permission.id)
      void replyPermission(projectId, sessionId, permission.id, 'once')
    }
  }, [approvalMode, permissions, projectId, sessionId, replyPermission])

  useEffect(() => {
    if (approvalMode !== 'autopilot') return
    for (const form of forms) {
      if (handledForms.current.has(form.id)) continue
      const resolution = resolveAutopilotForm(form)
      if (!resolution.canAutoSubmit) continue
      handledForms.current.add(form.id)
      void replyForm(projectId, sessionId, form.id, resolution.answers)
    }
  }, [approvalMode, forms, projectId, sessionId, replyForm])

  function handleCommand(text: string): boolean {
    const [raw, ...rest] = text.slice(1).split(/\s+/)
    const command = raw.toLowerCase()
    if (command === 'help') {
      setCommandNotice('可用命令：/files 打开文件、/activity 打开活动、/model <id> 切换模型、/stop 停止生成。')
      return true
    }
    if (command === 'files') {
      setActivityOpen(false)
      setFilesOpen((value) => !value)
      return true
    }
    if (command === 'activity') {
      setFilesOpen(false)
      setActivityOpen((value) => !value)
      return true
    }
    if (command === 'model') {
      const model = rest.join(' ').trim()
      if (model === '') setCommandNotice('用法：/model <模型 ID>')
      else {
        setSelectedModel(model)
        setCommandNotice(`已切换到模型 ${model}`)
      }
      return true
    }
    if (command === 'stop') {
      void onStop()
      return true
    }
    return false
  }

  async function onSend(text: string, images: ImageAttachment[]) {
    setSendError('')
    setCommandNotice('')
    if (text.startsWith('/') && handleCommand(text)) return
    const updated = await sendMessage(projectId, sessionId, text, selectedModel || undefined, images)
    if (!updated) setSendError(useSessionsStore.getState().error || '发送失败，请重试。')
  }

  async function onStop() {
    await stopSession(projectId, sessionId)
  }

  if (!session && sessions.length === 0 && error === '') {
    return (
      <div className={styles.state}>
        <Spin />
      </div>
    )
  }

  if (!session) {
    return (
      <div className={styles.state}>
        <Empty description="会话不存在或已被移除">
          <Button onClick={() => navigate('/')}>返回工作台</Button>
        </Empty>
      </div>
    )
  }

  return (
    <section className={styles.page} aria-label="会话对话" data-test="conversation">
      <header className={styles.header}>
        <div className={styles.titleRow}>
          <Typography.Title level={3}>{title}</Typography.Title>
          <Tag>{session.backend}</Tag>
          <Tag color={status === 'open' ? 'success' : status === 'error' ? 'error' : 'default'}>
            {statusLabels[status] ?? status}
          </Tag>
          {running && <span className={styles.running}>运行中</span>}
        </div>
        <Space>
          <Button size="small" onClick={() => setFilesOpen((value) => !value)}>
            {filesOpen ? '收起文件' : '文件'}
          </Button>
          <Button size="small" onClick={() => { setFilesOpen(false); setActivityOpen((value) => !value) }}>
            {activityOpen ? '收起活动' : '活动'}
          </Button>
          <Button
            size="small"
            danger
            onClick={() => {
              void (async () => {
                await stopSession(projectId, sessionId)
                navigate('/')
              })()
            }}
            disabled={!running}
          >
            停止并返回
          </Button>
        </Space>
      </header>

      {commandNotice !== '' && <Alert className={styles.truncated} type="info" showIcon closable title={commandNotice} onClose={() => setCommandNotice('')} />}

      <div className={`${styles.workspace} ${filesOpen || activityOpen ? styles.withFiles : ''}`}>
        <div className={styles.conversation}>
        {(historyError !== '' || error !== '' || sendError !== '') && (
          <div className={styles.alerts}>
            {historyError !== '' && (
              <Alert
                type="error"
                showIcon
                title={historyError}
                action={
                  <Button size="small" onClick={() => void loadHistory(projectId, sessionId)}>
                    重试
                  </Button>
                }
              />
            )}
            {error !== '' && <Alert type="error" showIcon title={error} />}
            {sendError !== '' && <Alert type="error" showIcon title={sendError} />}
          </div>
        )}

        {historyTruncated && (
          <Alert className={styles.truncated} type="info" showIcon title="仅显示最近的历史记录。" />
        )}

        <div className={styles.scroll} ref={scrollerRef} onScroll={onScroll} data-test="conversation-scroll">
          <MessageList entries={entries} running={running} />
        </div>

        {showJump && (
          <Button className={styles.jump} size="small" onClick={() => scrollToBottom(true)}>
            ↓ 滚动到最新消息
          </Button>
        )}

        <PermissionCards
          permissions={permissions}
          onReply={async (id, decision) => {
            await replyPermission(projectId, sessionId, id, decision)
          }}
        />
        <FormCards
          forms={forms}
          onReply={async (id, answer) => {
            await replyForm(projectId, sessionId, id, answer)
          }}
        />

        <ChatComposer
          running={running}
          allowImages
          mentionOptions={mentionOptions}
          toolbar={
            <>
            <Select
              size="small"
              allowClear
              showSearch
              optionFilterProp="label"
              style={{ minWidth: '14rem' }}
              value={selectedModel || undefined}
              placeholder="后端默认模型"
              options={models.map((model) => ({
                value: model.id,
                label: `${model.name || model.id}${model.provider ? ` · ${model.provider}` : ''}`,
              }))}
              onChange={(value) => setSelectedModel(value ?? '')}
              data-test="session-model"
            />
            <Select
              size="small"
              value={approvalMode}
              style={{ minWidth: '8rem' }}
              options={[
                { value: 'default', label: '默认审批' },
                { value: 'auto-approve', label: '自动审批' },
                { value: 'autopilot', label: '托管' },
              ]}
              onChange={(value: ApprovalMode) => {
                setApprovalMode(value)
                setPersistedApprovalMode(value)
              }}
              data-test="approval-mode"
            />
            </>
          }
          onSend={onSend}
          onStop={onStop}
        />
        </div>
        {activityOpen && !filesOpen && (
          <div className={styles.activity}>
            <ActivityPanel entries={entries} />
          </div>
        )}
        {filesOpen && (
          <div className={styles.files}>
            <FileSidebar projectId={projectId} activePath={activeFile} onSelect={setActiveFile} />
            <FileViewer projectId={projectId} path={activeFile} />
          </div>
        )}
      </div>

    </section>
  )
}
