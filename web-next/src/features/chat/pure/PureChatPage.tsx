import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons'
import { Alert, App, Button, Empty, Input, Modal, Select, Space, Tag, Typography } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { listServices, type ModelDto } from '../../../api/models'
import type { ChatEntry } from '../state/entry'
import { useChatsStore } from '../../../stores/chats'
import { ChatComposer } from '../ChatComposer'
import { MessageList } from '../MessageList'
import styles from './PureChatPage.module.css'

const EMPTY_ENTRIES: ChatEntry[] = []

function flattenModels(services: Awaited<ReturnType<typeof listServices>>['services']): ModelDto[] {
  return services
    .filter((service) => service.enabled)
    .flatMap((service) => service.models)
    .filter((model) => model.enabled && model.usable && model.supportsText)
}

export function PureChatPage() {
  const { chatId } = useParams()
  const id = chatId ? Number(chatId) : null
  const navigate = useNavigate()
  const { modal, message } = App.useApp()
  const [models, setModels] = useState<ModelDto[]>([])
  const [modelsError, setModelsError] = useState('')
  const [groupOpen, setGroupOpen] = useState(false)
  const [groupName, setGroupName] = useState('')
  const [sendError, setSendError] = useState('')
  const [renameOpen, setRenameOpen] = useState(false)
  const [renameValue, setRenameValue] = useState('')

  const groups = useChatsStore((state) => state.groups)
  const chats = useChatsStore((state) => state.chats)
  const activeId = useChatsStore((state) => state.activeId)
  const active = useChatsStore((state) => state.active)
  const messages = useChatsStore((state) =>
    activeId === null ? EMPTY_ENTRIES : (state.messages[activeId] ?? EMPTY_ENTRIES),
  )
  const running = useChatsStore((state) => (activeId === null ? false : state.running[activeId] === true))
  const loadChats = useChatsStore((state) => state.loadChats)
  const loadGroups = useChatsStore((state) => state.loadGroups)
  const openChat = useChatsStore((state) => state.openChat)
  const startNewChat = useChatsStore((state) => state.startNewChat)
  const createAndSend = useChatsStore((state) => state.createAndSend)
  const sendMessage = useChatsStore((state) => state.sendMessage)
  const stop = useChatsStore((state) => state.stop)
  const regenerate = useChatsStore((state) => state.regenerate)
  const removeActive = useChatsStore((state) => state.removeActive)
  const setModel = useChatsStore((state) => state.setModel)
  const removeGroup = useChatsStore((state) => state.deleteGroup)
  const moveToGroup = useChatsStore((state) => state.moveToGroup)
  const rename = useChatsStore((state) => state.rename)
  const createGroup = useChatsStore((state) => state.createGroup)
  const draftModelId = useChatsStore((state) => state.draftModelId)

  useEffect(() => {
    void loadGroups()
    void loadChats()
    void listServices()
      .then((result) => setModels(flattenModels(result.services)))
      .catch(() => setModelsError('模型列表加载失败，请到设置中配置模型服务。'))
  }, [loadGroups, loadChats])

  useEffect(() => {
    if (id !== null && id !== activeId) void openChat(id)
  }, [id, activeId, openChat])

  const selectedModelId = active?.modelId ?? draftModelId ?? models[0]?.id ?? null
  const modelOptions = useMemo(
    () => models.map((model) => ({ value: model.id, label: model.name || model.modelId })),
    [models],
  )

  const grouped = useMemo(() => {
    const byGroup = new Map<number, typeof chats>()
    const ungrouped: typeof chats = []
    for (const chat of chats) {
      if (chat.groupId && chat.groupId > 0) {
        byGroup.set(chat.groupId, [...(byGroup.get(chat.groupId) ?? []), chat])
      } else {
        ungrouped.push(chat)
      }
    }
    return { byGroup, ungrouped }
  }, [chats])

  function newChat() {
    startNewChat()
    navigate('/chat')
  }

  async function onSend(text: string, images: Parameters<typeof sendMessage>[2] = []) {
    setSendError('')
    if (selectedModelId === null) {
      setSendError('请先在设置中配置可用模型。')
      return
    }
    if (activeId !== null) {
      const ok = await sendMessage(text, selectedModelId, images)
      if (!ok) setSendError(useChatsStore.getState().sendError || '发送失败，请重试。')
      return
    }
    const created = await createAndSend(text, selectedModelId, undefined, images)
    if (!created) {
      setSendError(useChatsStore.getState().sendError || '创建对话失败。')
      return
    }
    navigate(`/chat/${created.id}`)
  }

  function confirmDelete() {
    if (!active) return
    modal.confirm({
      title: '删除对话',
      content: `确定删除「${active.title || '未命名对话'}」吗？`,
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        const ok = await removeActive()
        if (ok) {
          message.success('对话已删除')
          navigate('/chat')
        }
      },
    })
  }

  return (
    <div className={styles.page} data-test="pure-chat-page">
      <aside className={styles.sidebar} aria-label="对话列表">
        <div className={styles.sidebarHead}>
          <Typography.Text strong>Chat</Typography.Text>
          <Space size={4}>
            <Button size="small" type="primary" icon={<PlusOutlined />} onClick={newChat}>
              新建
            </Button>
            <Button size="small" onClick={() => setGroupOpen(true)}>
              分组
            </Button>
          </Space>
        </div>
        <div className={styles.chatList}>
          {groups.map((group) => {
            const list = grouped.byGroup.get(group.id) ?? []
            if (list.length === 0) return null
            return (
              <section key={group.id} className={styles.group}>
                <div className={styles.groupHead}>
                  <span>{group.name}</span>
                  {group.systemKey ? <Tag>固定</Tag> : null}
                  {!group.systemKey && (
                    <Button
                      size="small"
                      type="text"
                      danger
                      icon={<DeleteOutlined />}
                      aria-label={`删除分组 ${group.name}`}
                      onClick={() =>
                        modal.confirm({
                          title: '删除分组',
                          content: `删除分组「${group.name}」？组内对话也会删除。`,
                          okText: '删除',
                          okButtonProps: { danger: true },
                          cancelText: '取消',
                          onOk: async () => {
                            const ok = await removeGroup(group.id)
                            if (ok) message.success('分组已删除')
                          },
                        })
                      }
                    />
                  )}
                </div>
                {list.map((chat) => (
                  <button
                    key={chat.id}
                    type="button"
                    className={`${styles.chatRow} ${chat.id === activeId ? styles.activeChat : ''}`}
                    onClick={() => navigate(`/chat/${chat.id}`)}
                  >
                    <span className={styles.chatTitle}>{chat.title || '未命名对话'}</span>
                    <span className={styles.chatModel}>{chat.modelName}</span>
                  </button>
                ))}
              </section>
            )
          })}
          {grouped.ungrouped.map((chat) => (
            <button
              key={chat.id}
              type="button"
              className={`${styles.chatRow} ${chat.id === activeId ? styles.activeChat : ''}`}
              onClick={() => navigate(`/chat/${chat.id}`)}
            >
              <span className={styles.chatTitle}>{chat.title || '未命名对话'}</span>
              <span className={styles.chatModel}>{chat.modelName}</span>
            </button>
          ))}
          {chats.length === 0 && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有对话" />}
        </div>
      </aside>

      <section className={styles.main} data-test="conversation">
        <header className={styles.header}>
          <div className={styles.titleRow}>
            <Typography.Title level={3}>{active?.title || 'Chat'}</Typography.Title>
            {running && <Tag color="processing">生成中</Tag>}
          </div>
          <Space>
            <Select
              size="small"
              style={{ minWidth: '12rem' }}
              placeholder="选择模型"
              value={selectedModelId ?? undefined}
              options={modelOptions}
              onChange={(value) => {
                void setModel(value)
              }}
              data-test="chat-model"
            />
            {active && (
              <>
                <Button size="small" onClick={() => void regenerate()} disabled={running}>
                  重新生成
                </Button>
                <Button
                  size="small"
                  icon={<EditOutlined />}
                  onClick={() => {
                    setRenameValue(active.title || '')
                    setRenameOpen(true)
                  }}
                >
                  重命名
                </Button>
                <Select
                  size="small"
                  style={{ minWidth: '9rem' }}
                  placeholder="移动到分组"
                  value={active.groupId ?? undefined}
                  options={groups.map((group) => ({ value: group.id, label: group.name }))}
                  onChange={async (value) => {
                    const ok = await moveToGroup(active.id, value)
                    if (ok) message.success('已移动')
                  }}
                  data-test="chat-group-select"
                />
                <Button size="small" danger icon={<DeleteOutlined />} onClick={confirmDelete}>
                  删除
                </Button>
              </>
            )}
          </Space>
        </header>

        {(modelsError || sendError) && (
          <Alert
            className={styles.alert}
            type="error"
            showIcon
            title={modelsError || sendError}
          />
        )}

        <div className={styles.messages}>
          <MessageList entries={messages} running={running} />
        </div>

        <ChatComposer
          running={running}
          allowImages
          disabled={selectedModelId === null}
          onSend={onSend}
          onStop={async () => {
            await stop()
          }}
        />
      </section>

      <Modal
        title="重命名对话"
        open={renameOpen}
        onCancel={() => setRenameOpen(false)}
        onOk={async () => {
          if (renameValue.trim() === '') return
          const ok = await rename(renameValue.trim())
          if (ok) {
            message.success('已重命名')
            setRenameOpen(false)
          }
        }}
        okText="保存"
        cancelText="取消"
      >
        <Input
          value={renameValue}
          onChange={(event) => setRenameValue(event.target.value)}
          aria-label="对话标题"
        />
      </Modal>

      <Modal
        title="新建分组"
        open={groupOpen}
        onCancel={() => setGroupOpen(false)}
        onOk={async () => {
          if (groupName.trim() === '') return
          await createGroup(groupName.trim())
          setGroupName('')
          setGroupOpen(false)
        }}
        okText="创建"
        cancelText="取消"
      >
        <Input
          value={groupName}
          onChange={(event) => setGroupName(event.target.value)}
          placeholder="分组名称"
          aria-label="分组名称"
        />
      </Modal>
    </div>
  )
}
