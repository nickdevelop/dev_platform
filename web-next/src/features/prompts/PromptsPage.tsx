import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons'
import { Alert, App, Button, Empty, Form, Input, Modal, Space, Spin, Tag, Typography } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import type { Prompt } from '../../api/prompts'
import { usePromptsStore } from '../../stores/prompts'
import { applyUserPromptTemplate, hasUserPromptPlaceholder, USER_PROMPT_PLACEHOLDER } from './promptTemplate'
import styles from './PromptsPage.module.css'

interface PromptFormValues {
  name: string
  description?: string
  content: string
}

export function PromptsPage() {
  const { promptId } = useParams()
  const id = promptId ? Number(promptId) : null
  const navigate = useNavigate()
  const { modal, message } = App.useApp()
  const prompts = usePromptsStore((state) => state.prompts)
  const loading = usePromptsStore((state) => state.loading)
  const error = usePromptsStore((state) => state.error)
  const loadPrompts = usePromptsStore((state) => state.loadPrompts)
  const create = usePromptsStore((state) => state.create)
  const update = usePromptsStore((state) => state.update)
  const remove = usePromptsStore((state) => state.remove)
  const [search, setSearch] = useState('')
  const [testInput, setTestInput] = useState('')
  const [editing, setEditing] = useState<Prompt | null | undefined>(undefined)
  const [form] = Form.useForm<PromptFormValues>()

  useEffect(() => {
    void loadPrompts()
  }, [loadPrompts])

  const active = useMemo(() => {
    if (id !== null) return prompts.find((prompt) => prompt.id === id) ?? null
    return prompts[0] ?? null
  }, [prompts, id])

  const visible = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (query === '') return prompts
    return prompts.filter(
      (prompt) =>
        prompt.name.toLowerCase().includes(query) ||
        prompt.description.toLowerCase().includes(query),
    )
  }, [prompts, search])

  const preview = active ? applyUserPromptTemplate(active.content, testInput) : ''

  function openCreate() {
    setEditing(null)
    form.setFieldsValue({ name: '', description: '', content: '' })
  }

  function openEdit(prompt: Prompt) {
    setEditing(prompt)
    form.setFieldsValue({
      name: prompt.name,
      description: prompt.description,
      content: prompt.content,
    })
  }

  async function submit() {
    const values = await form.validateFields()
    if (editing) {
      const saved = await update(editing.id, values)
      if (saved) {
        message.success('提示词已保存')
        setEditing(undefined)
        navigate(`/prompts/${saved.id}`)
      }
      return
    }
    const created = await create(values)
    if (created) {
      message.success('提示词已创建')
      setEditing(undefined)
      navigate(`/prompts/${created.id}`)
    }
  }

  function confirmDelete(prompt: Prompt) {
    modal.confirm({
      title: '删除提示词',
      content: `确定删除提示词「${prompt.name}」吗？`,
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        const ok = await remove(prompt.id)
        if (ok) {
          message.success('提示词已删除')
          navigate('/prompts')
        }
      },
    })
  }

  return (
    <div className={styles.page} data-test="prompts-page">
      <aside className={styles.sidebar} aria-label="提示词列表">
        <div className={styles.sidebarHead}>
          <Typography.Text strong>提示词</Typography.Text>
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新建
          </Button>
        </div>
        <Input.Search
          allowClear
          size="small"
          placeholder="搜索提示词"
          aria-label="搜索提示词"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        {loading && prompts.length === 0 ? (
          <div className={styles.center}>
            <Spin size="small" />
          </div>
        ) : visible.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有提示词" />
        ) : (
          <div className={styles.list}>
            {visible.map((prompt) => (
              <button
                key={prompt.id}
                type="button"
                className={`${styles.item} ${active?.id === prompt.id ? styles.active : ''}`}
                onClick={() => navigate(`/prompts/${prompt.id}`)}
              >
                <span className={styles.name}>{prompt.name}</span>
                {prompt.description && <span className={styles.desc}>{prompt.description}</span>}
              </button>
            ))}
          </div>
        )}
      </aside>

      <section className={styles.main} aria-label="提示词详情">
        {error !== '' && <Alert type="error" showIcon title={error} />}
        {active ? (
          <>
            <header className={styles.header}>
              <div>
                <div className={styles.titleRow}>
                  <Typography.Title level={2}>{active.name}</Typography.Title>
                  {hasUserPromptPlaceholder(active.content) && (
                    <Tag color="processing">包含 {USER_PROMPT_PLACEHOLDER} 占位符</Tag>
                  )}
                </div>
                {active.description && <Typography.Text type="secondary">{active.description}</Typography.Text>}
              </div>
              <Space>
                <Button icon={<EditOutlined />} data-test="edit-prompt-btn" onClick={() => openEdit(active)}>
                  编辑
                </Button>
                <Button danger icon={<DeleteOutlined />} data-test="delete-prompt-btn" onClick={() => confirmDelete(active)}>
                  删除
                </Button>
              </Space>
            </header>

            <div className={styles.body}>
              <section className={styles.card}>
                <Typography.Title level={4}>提示词模版</Typography.Title>
                <pre className={styles.template}>{active.content}</pre>
              </section>

              <section className={styles.card}>
                <Typography.Title level={4}>填充效果模拟测试</Typography.Title>
                <Typography.Paragraph type="secondary">
                  输入测试消息，实时预览占位符替换后的合成效果。
                </Typography.Paragraph>
                <div className={styles.previewGrid}>
                  <label htmlFor="test-user-input">
                    模拟用户输入
                    <Input.TextArea
                      id="test-user-input"
                      data-test="prompt-test-input"
                      rows={4}
                      value={testInput}
                      onChange={(event) => setTestInput(event.target.value)}
                    />
                  </label>
                  <div>
                    <span className={styles.previewLabel}>合成后发送给 Agent 的内容</span>
                    <pre className={styles.preview} data-test="prompt-preview-output">
                      {preview}
                    </pre>
                  </div>
                </div>
              </section>
            </div>
          </>
        ) : (
          <div className={styles.center}>
            <Empty description="未选择提示词">
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                新建提示词
              </Button>
            </Empty>
          </div>
        )}
      </section>

      <Modal
        title={editing ? '编辑提示词' : '新建提示词'}
        open={editing !== undefined}
        onCancel={() => setEditing(undefined)}
        onOk={() => void submit()}
        okText="保存"
        cancelText="取消"
        destroyOnHidden
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input />
          </Form.Item>
          <Form.Item name="content" label="提示词模版" rules={[{ required: true, message: '请输入内容' }]}>
            <Input.TextArea rows={10} placeholder={`支持 ${USER_PROMPT_PLACEHOLDER} 占位符`} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
