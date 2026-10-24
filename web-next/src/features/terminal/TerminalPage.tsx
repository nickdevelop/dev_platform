/* oxlint-disable react/set-state-in-effect -- data loading effect intentionally updates state */
import { DeleteOutlined, PlusOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { Alert, App, Button, Empty, Form, Input, InputNumber, Modal, Select, Typography } from 'antd'
import { useEffect, useState } from 'react'
import {
  createTerminalServer,
  deleteTerminalServer,
  getTerminalStatus,
  listTerminalServers,
  type TerminalServer,
} from '../../api/terminal'
import { XtermPane } from './XtermPane'
import styles from './TerminalPage.module.css'

interface ServerValues {
  name: string
  host: string
  port: number
  username: string
  auth_type: 'password' | 'key'
  secret: string
}

export function TerminalPage() {
  const { modal, message } = App.useApp()
  const [servers, setServers] = useState<TerminalServer[]>([])
  const [supported, setSupported] = useState(true)
  const [statusMessage, setStatusMessage] = useState('')
  const [tabs, setTabs] = useState<number[]>([])
  const [activeId, setActiveId] = useState<number | null>(null)
  const [open, setOpen] = useState(false)
  const [error, setError] = useState('')
  const [form] = Form.useForm<ServerValues>()

  async function load() {
    try {
      const [status, list] = await Promise.all([getTerminalStatus(), listTerminalServers()])
      setSupported(status.supported)
      setStatusMessage(status.message ?? '')
      setServers(list)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '加载终端失败')
    }
  }

  useEffect(() => {
    void load()
  }, [])

  function openTab(id: number) {
    setTabs((current) => (current.includes(id) ? current : [...current, id]))
    setActiveId(id)
  }

  function closeTab(id: number) {
    setTabs((current) => {
      const next = current.filter((entry) => entry !== id)
      if (activeId === id) setActiveId(next[next.length - 1] ?? null)
      return next
    })
  }

  async function createServer() {
    const values = await form.validateFields()
    try {
      const created = await createTerminalServer(values)
      message.success('服务器已添加')
      setOpen(false)
      form.resetFields()
      await load()
      openTab(created.id)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '添加服务器失败')
    }
  }

  function remove(server: TerminalServer) {
    modal.confirm({
      title: '删除服务器',
      content: `确定删除「${server.name}」吗？`,
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        await deleteTerminalServer(server.id)
        message.success('服务器已删除')
        if (activeId === server.id) setActiveId(null)
        await load()
      },
    })
  }

  return (
    <div className={styles.page} data-test="terminal-page">
      <aside className={styles.sidebar} aria-label="服务器列表">
        <div className={styles.head}>
          <Typography.Text strong>终端</Typography.Text>
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>
            添加
          </Button>
        </div>
        <Alert
          type={supported ? 'success' : 'warning'}
          showIcon
          title={supported ? '本机终端可用' : '本机终端不可用'}
          description={statusMessage || undefined}
        />
        <Button
          type={activeId === 0 ? 'primary' : 'default'}
          icon={<ThunderboltOutlined />}
          disabled={!supported}
          onClick={() => openTab(0)}
        >
          本机终端
        </Button>
        {servers.map((server) => (
          <div
            key={server.id}
            className={`${styles.server} ${activeId === server.id ? styles.active : ''}`}
          >
            <button type="button" className={styles.serverMain} onClick={() => openTab(server.id)}>
              <span>{server.name}</span>
              <span className={styles.serverMeta}>
                {server.username}@{server.host}:{server.port}
              </span>
            </button>
            <Button size="small" type="text" danger icon={<DeleteOutlined />} onClick={() => remove(server)} />
          </div>
        ))}
        {servers.length === 0 && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有 SSH 服务器" />}
      </aside>

      <main className={styles.main}>
        {error !== '' && <Alert type="error" showIcon title={error} />}
        <div className={styles.tabBar}>
          {tabs.map((id) => {
            const server = servers.find((entry) => entry.id === id)
            return (
              <div key={id} className={`${styles.tab} ${activeId === id ? styles.activeTab : ''}`}>
                <button type="button" className={styles.tabButton} onClick={() => setActiveId(id)}>
                  {id === 0 ? '本机终端' : server?.name ?? `服务器 ${id}`}
                </button>
                <Button
                  size="small"
                  type="text"
                  aria-label={`关闭终端 ${id === 0 ? '本机' : server?.name ?? id}`}
                  onClick={() => closeTab(id)}
                >
                  ×
                </Button>
              </div>
            )
          })}
        </div>
        {tabs.length === 0 ? (
          <div className={styles.center}>
            <Empty description="选择本机终端或服务器开始连接" />
          </div>
        ) : (
          tabs.map((id) => (
            <div key={id} className={styles.terminalPane} style={{ display: activeId === id ? 'flex' : 'none' }}>
              <XtermPane serverId={id} />
            </div>
          ))
        )}
      </main>

      <Modal
        title="添加 SSH 服务器"
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => void createServer()}
        okText="保存"
        cancelText="取消"
      >
        <Form
          form={form}
          layout="vertical"
          initialValues={{ port: 22, auth_type: 'password' }}
        >
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="host" label="主机" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="port" label="端口" rules={[{ required: true }]}>
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="auth_type" label="认证方式">
            <Select
              options={[
                { value: 'password', label: '密码' },
                { value: 'key', label: '私钥' },
              ]}
            />
          </Form.Item>
          <Form.Item name="secret" label="密码 / 私钥" rules={[{ required: true }]}>
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
