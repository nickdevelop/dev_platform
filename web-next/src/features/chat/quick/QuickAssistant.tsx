/* oxlint-disable react/set-state-in-effect -- EventSource lifecycle sets connecting state */
import { CloseOutlined, MessageOutlined } from '@ant-design/icons'
import { Alert, Button, Tag } from 'antd'
import { useEffect, useRef, useState } from 'react'
import {
  chatEventsPath,
  createChat,
  listChatGroups,
  listChats,
  sendChatMessage,
  stopChat,
  type Chat,
  type ChatEvent,
  type ChatGroup,
} from '../../../api/chats'
import { listServices } from '../../../api/models'
import type { ChatEntry, StreamStatus } from '../state/entry'
import { applyDelta, entriesFromMessages, settleStreaming, upsertMessage } from '../pure/chatEntries'
import { ChatComposer } from '../ChatComposer'
import { MessageList } from '../MessageList'
import styles from './QuickAssistant.module.css'

export function QuickAssistant() {
  const [open, setOpen] = useState(false)
  const [chat, setChat] = useState<Chat | null>(null)
  const [groupId, setGroupId] = useState<number | null>(null)
  const [entries, setEntries] = useState<ChatEntry[]>([])
  const [running, setRunning] = useState(false)
  const [status, setStatus] = useState<StreamStatus>('idle')
  const [modelId, setModelId] = useState<number | null>(null)
  const [error, setError] = useState('')
  const sourceRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!open) return
    void (async () => {
      try {
        const groups = await listChatGroups()
        const quick = groups.groups.find((entry: ChatGroup) => entry.systemKey === 'quick-assistant')
        const nextGroup = quick?.id ?? null
        setGroupId(nextGroup)
        if (nextGroup !== null) {
          const list = await listChats(20, 0, nextGroup)
          if (list.chats.length > 0) setChat(list.chats[0])
        }
        const services = await listServices()
        const model = services.services
          .filter((service) => service.enabled)
          .flatMap((service) => service.models)
          .find((entry) => entry.enabled && entry.usable && entry.supportsText)
        setModelId(model?.id ?? null)
      } catch (reason) {
        setError(reason instanceof Error ? reason.message : '快捷助手加载失败')
      }
    })()
  }, [open])

  useEffect(() => {
    if (!open || !chat) return
    const source = new EventSource(chatEventsPath(chat.id))
    sourceRef.current = source
    setStatus('connecting')
    source.onopen = () => setStatus('open')
    source.onerror = () => setStatus(source.readyState === EventSource.CLOSED ? 'error' : 'reconnecting')
    const listen = <T extends ChatEvent['type']>(
      type: T,
      apply: (event: Extract<ChatEvent, { type: T }>) => void,
    ) => {
      source.addEventListener(type, (event) => {
        try {
          const parsed = JSON.parse((event as MessageEvent).data) as ChatEvent
          if (parsed.type === type) apply(parsed as Extract<ChatEvent, { type: T }>)
        } catch {
          // Ignore a malformed frame.
        }
      })
    }
    listen('snapshot', (event) => {
      setChat(event.snapshot.conversation)
      setEntries(entriesFromMessages(event.snapshot.messages))
      setRunning(event.snapshot.running)
      setStatus('open')
    })
    listen('delta', (event) => setEntries((current) => applyDelta(current, event.delta)))
    listen('message_update', (event) => {
      setEntries((current) => upsertMessage(current, event.message))
      if (event.message.status !== 'pending') setRunning(false)
    })
    listen('conversation', (event) => {
      setChat(event.conversation)
      setRunning(event.conversation.running)
    })
    listen('turn', (event) => {
      setRunning(event.turn.phase === 'started')
      if (event.turn.phase === 'ended') setEntries((current) => settleStreaming(current))
    })
    return () => {
      source.close()
      sourceRef.current = null
    }
  }, [open, chat])

  async function send(text: string) {
    setError('')
    if (modelId === null) {
      setError('没有可用模型，请先在设置中配置。')
      return
    }
    try {
      if (!chat) {
        const created = await createChat({ modelId, text, groupId: groupId ?? undefined })
        setChat(created)
        setRunning(true)
        return
      }
      const updated = await sendChatMessage(chat.id, { modelId, text })
      setChat(updated)
      setRunning(true)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '发送失败')
    }
  }

  async function stop() {
    if (!chat) return
    try {
      await stopChat(chat.id)
    } catch {
      // The stream's ended frame settles the UI.
    }
  }

  return (
    <>
      <Button
        className={styles.launcher}
        type="primary"
        shape="circle"
        size="large"
        icon={<MessageOutlined />}
        aria-label="快捷助手"
        data-test="quick-assistant-launcher"
        onClick={() => setOpen((value) => !value)}
      />
      {open && (
        <dialog open className={styles.panel} aria-label="快捷助手" data-test="quick-assistant-panel">
          <header className={styles.header}>
            <div>
              <strong>快捷助手</strong>
              {status !== 'idle' && <Tag>{status}</Tag>}
            </div>
            <Button size="small" type="text" icon={<CloseOutlined />} aria-label="关闭快捷助手" onClick={() => setOpen(false)} />
          </header>
          <div className={styles.body}>
            {error !== '' && <Alert type="error" showIcon title={error} />}
            {!chat && entries.length === 0 && <p className={styles.hint}>发送第一条消息开始快捷对话。</p>}
            <MessageList entries={entries} running={running} />
          </div>
          <ChatComposer running={running} disabled={modelId === null} onSend={send} onStop={stop} />
        </dialog>
      )}
    </>
  )
}
