import { DeleteOutlined, PlusOutlined, PlayCircleOutlined } from '@ant-design/icons'
import {
  Alert,
  App,
  Button,
  Descriptions,
  Empty,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd'
import type { TableColumnsType } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router'
import type { CreateTaskPayload, Task, TaskRun } from '../../api/tasks'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import { usePromptsStore } from '../../stores/prompts'
import { useTasksStore } from '../../stores/tasks'
import styles from './TasksPage.module.css'

interface TaskFormValues {
  title: string
  description?: string
  projectId: number
  backend: string
  model: string
  promptTemplate: string
  scheduleType: 'manual' | 'once' | 'cron'
  scheduledAt?: string
  cronExpr?: string
  scheduleEnabled: boolean
}

const statusColor: Record<string, string> = {
  running: 'processing',
  success: 'success',
  failed: 'error',
  idle: 'default',
}

function scheduleLabel(task: Task): string {
  if (task.scheduleType === 'manual') return '手动'
  if (task.scheduleType === 'once') return '单次'
  return task.cronExpr || 'Cron'
}

export function TasksPage() {
  const { taskId } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const locationNew = location.pathname.endsWith('/new')
  const tasks = useTasksStore((state) => state.tasks)
  const loading = useTasksStore((state) => state.loading)
  const error = useTasksStore((state) => state.error)
  const loadTasks = useTasksStore((state) => state.loadTasks)
  const loadProjects = useProjectsStore((state) => state.load)
  const loadBackends = useAgentsStore((state) => state.loadBackends)
  const loadPrompts = usePromptsStore((state) => state.loadPrompts)
  const [search, setSearch] = useState('')

  useEffect(() => {
    void loadTasks()
    void loadProjects()
    void loadBackends()
    void loadPrompts()
  }, [loadTasks, loadProjects, loadBackends, loadPrompts])

  const visible = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (query === '') return tasks
    return tasks.filter(
      (task) =>
        task.title.toLowerCase().includes(query) || task.description.toLowerCase().includes(query),
    )
  }, [tasks, search])

  const selectedId = taskId ? Number(taskId) : null

  return (
    <div className={styles.page} data-test="tasks-page">
      <aside className={styles.sidebar} aria-label="任务列表">
        <div className={styles.sidebarHead}>
          <Typography.Text strong>任务</Typography.Text>
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => navigate('/tasks/new')}>
            新建
          </Button>
        </div>
        <Input.Search
          allowClear
          size="small"
          placeholder="搜索任务"
          aria-label="搜索任务"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        {loading && tasks.length === 0 ? (
          <div className={styles.center}>
            <Spin size="small" />
          </div>
        ) : visible.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有任务" />
        ) : (
          <div className={styles.list}>
            {visible.map((task) => (
              <button
                key={task.id}
                type="button"
                className={`${styles.item} ${selectedId === task.id ? styles.active : ''}`}
                onClick={() => navigate(`/tasks/${task.id}`)}
              >
                <span className={styles.itemTitle}>{task.title}</span>
                <span className={styles.itemMeta}>
                  <Tag color={statusColor[task.lastRunStatus ?? 'idle']}>
                    {task.lastRunStatus ?? '未运行'}
                  </Tag>
                  <span>{scheduleLabel(task)}</span>
                </span>
              </button>
            ))}
          </div>
        )}
      </aside>

      <main className={styles.main}>
        {error !== '' && <Alert type="error" showIcon title={error} />}
        {locationNew ? (
          <TaskForm onSaved={(task) => navigate(`/tasks/${task.id}`)} />
        ) : selectedId !== null ? (
          <TaskDetail taskId={selectedId} />
        ) : (
          <div className={styles.center}>
            <Empty description="选择一个任务，或新建任务">
              <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/tasks/new')}>
                新建任务
              </Button>
            </Empty>
          </div>
        )}
      </main>
    </div>
  )
}

function TaskForm({ task, onSaved }: { task?: Task; onSaved: (task: Task) => void }) {
  const [form] = Form.useForm<TaskFormValues>()
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const projects = useProjectsStore((state) => state.projects)
  const backends = useAgentsStore((state) => state.backends)
  const prompts = usePromptsStore((state) => state.prompts)
  const create = useTasksStore((state) => state.create)
  const update = useTasksStore((state) => state.update)
  const scheduleType = Form.useWatch('scheduleType', form) ?? 'manual'

  useEffect(() => {
    if (!task) return
    form.setFieldsValue({
      title: task.title,
      description: task.description,
      projectId: task.projectId,
      backend: task.backend,
      model: task.model,
      promptTemplate: task.promptTemplate,
      scheduleType: task.scheduleType,
      scheduledAt: task.scheduledAt ? task.scheduledAt.slice(0, 16) : undefined,
      cronExpr: task.cronExpr,
      scheduleEnabled: task.scheduleEnabled,
    })
  }, [form, task])

  async function submit(values: TaskFormValues) {
    setSubmitting(true)
    setError('')
    const payload: CreateTaskPayload = {
      title: values.title.trim(),
      description: values.description?.trim(),
      taskType: 'standard',
      projectId: values.projectId,
      backend: values.backend,
      model: values.model.trim(),
      promptTemplate: values.promptTemplate,
      scheduleType: values.scheduleType,
      scheduleEnabled: values.scheduleEnabled,
    }
    if (values.scheduleType === 'once' && values.scheduledAt) {
      payload.scheduledAt = new Date(values.scheduledAt).toISOString()
    }
    if (values.scheduleType === 'cron') payload.cronExpr = values.cronExpr?.trim()
    try {
      const saved = task ? await update(task.id, payload) : await create(payload)
      if (!saved) {
        setError(useTasksStore.getState().error || '保存任务失败')
        return
      }
      onSaved(saved)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className={styles.formPage}>
      <Typography.Title level={2}>{task ? '编辑任务' : '创建任务'}</Typography.Title>
      <Typography.Paragraph type="secondary">
        配置可手动、定时或周期执行的会话模板。
      </Typography.Paragraph>
      <Form
        form={form}
        layout="vertical"
        initialValues={
          task
            ? {
                title: task.title,
                description: task.description,
                projectId: task.projectId,
                backend: task.backend,
                model: task.model,
                promptTemplate: task.promptTemplate,
                scheduleType: task.scheduleType,
                scheduledAt: task.scheduledAt?.slice(0, 16),
                cronExpr: task.cronExpr,
                scheduleEnabled: task.scheduleEnabled,
              }
            : { scheduleType: 'manual', scheduleEnabled: true }
        }
        onFinish={(values) => void submit(values)}
      >
        <Form.Item name="title" label="任务名称" rules={[{ required: true, message: '请输入任务名称' }]}>
          <Input />
        </Form.Item>
        <Form.Item name="description" label="任务描述">
          <Input />
        </Form.Item>
        <Form.Item name="projectId" label="关联项目" rules={[{ required: true, message: '请选择项目' }]}>
          <Select options={projects.map((project) => ({ value: project.id, label: project.name }))} />
        </Form.Item>
        <Form.Item name="backend" label="Agent 后端" rules={[{ required: true, message: '请选择后端' }]}>
          <Select
            options={backends.map((backend) => ({
              value: backend.backend,
              label: backend.label || backend.backend,
            }))}
          />
        </Form.Item>
        <Form.Item name="model" label="运行模型" rules={[{ required: true, message: '请输入模型 ID' }]}>
          <Input placeholder="例如 pi/gpt-5" />
        </Form.Item>
        <Form.Item name="promptTemplate" label="执行提示词模板" rules={[{ required: true, message: '请输入提示词' }]}>
          <Input.TextArea rows={5} />
        </Form.Item>
        <Form.Item label="从提示词模板填充">
          <Select
            allowClear
            placeholder="选择一个提示词模板"
            options={prompts.map((prompt) => ({ value: prompt.id, label: prompt.name }))}
            onChange={(value) => {
              const prompt = prompts.find((entry) => entry.id === value)
              if (prompt) form.setFieldValue('promptTemplate', prompt.content)
            }}
          />
        </Form.Item>
        <Form.Item name="scheduleType" label="执行方式">
          <Select
            options={[
              { value: 'manual', label: '手动执行' },
              { value: 'once', label: '定时执行（单次）' },
              { value: 'cron', label: '周期执行（Cron）' },
            ]}
          />
        </Form.Item>
        {scheduleType === 'once' && (
          <Form.Item name="scheduledAt" label="定时触发时间" rules={[{ required: true, message: '请选择时间' }]}>
            <Input type="datetime-local" />
          </Form.Item>
        )}
        {scheduleType === 'cron' && (
          <>
          <Form.Item name="cronExpr" label="Cron 表达式" rules={[{ required: true, message: '请输入 Cron' }]}>
            <Input placeholder="0 9 * * 1-5" />
          </Form.Item>
          <Form.Item label="常用计划">
            <Space wrap>
              <Button size="small" onClick={() => form.setFieldValue('cronExpr', '0 9 * * 1-5')}>工作日 9:00</Button>
              <Button size="small" onClick={() => form.setFieldValue('cronExpr', '0 9 * * *')}>每天 9:00</Button>
              <Button size="small" onClick={() => form.setFieldValue('cronExpr', '0 9 * * 1')}>每周一 9:00</Button>
            </Space>
          </Form.Item>
          </>
        )}
        <Form.Item name="scheduleEnabled" label="启用调度" valuePropName="checked">
          <Switch />
        </Form.Item>
        {error !== '' && <Alert type="error" showIcon title={error} />}
        <Space>
          <Button type="primary" htmlType="submit" loading={submitting}>
            {task ? '保存修改' : '创建任务'}
          </Button>
          <Button onClick={() => form.resetFields()}>重置</Button>
        </Space>
      </Form>
    </div>
  )
}

function TaskDetail({ taskId }: { taskId: number }) {
  const navigate = useNavigate()
  const { modal, message } = App.useApp()
  const task = useTasksStore((state) => state.tasks.find((entry) => entry.id === taskId) ?? null)
  const runs = useTasksStore((state) => state.runs[taskId]) ?? []
  const loadTask = useTasksStore((state) => state.loadTask)
  const loadRuns = useTasksStore((state) => state.loadRuns)
  const trigger = useTasksStore((state) => state.trigger)
  const remove = useTasksStore((state) => state.remove)
  const [editOpen, setEditOpen] = useState(false)

  useEffect(() => {
    void loadTask(taskId)
    void loadRuns(taskId)
  }, [taskId, loadTask, loadRuns])

  if (!task) {
    return (
      <div className={styles.center}>
        <Spin />
      </div>
    )
  }

  const columns: TableColumnsType<TaskRun> = [
    { title: '编号', dataIndex: 'id', key: 'id', width: 80, render: (id) => `#${id}` },
    { title: '触发方式', dataIndex: 'triggerType', key: 'triggerType', width: 100 },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => <Tag color={statusColor[status]}>{status}</Tag>,
    },
    {
      title: '触发时间',
      dataIndex: 'triggeredAt',
      key: 'triggeredAt',
      render: (value: string) => new Date(value).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_, run) => (
        <Button
          size="small"
          type="link"
          onClick={() => navigate(`/projects/${task.projectId}/sessions/${run.sessionId}`)}
        >
          查看会话
        </Button>
      ),
    },
  ]

  return (
    <div className={styles.detail}>
      <header className={styles.header}>
        <div>
          <Typography.Title level={2}>{task.title}</Typography.Title>
          <Space>
            <Tag color={statusColor[task.lastRunStatus ?? 'idle']}>{task.lastRunStatus ?? '未运行'}</Tag>
            <Tag>{scheduleLabel(task)}</Tag>
            {task.scheduleEnabled ? <Tag color="success">调度启用</Tag> : <Tag>调度停用</Tag>}
          </Space>
        </div>
        <Space>
          <Button onClick={() => setEditOpen(true)}>编辑</Button>
          <Button
            type="primary"
            icon={<PlayCircleOutlined />}
            onClick={async () => {
              const run = await trigger(task.id)
              if (run) message.success('任务已触发')
            }}
          >
            立即执行
          </Button>
          <Button
            danger
            icon={<DeleteOutlined />}
            onClick={() =>
              modal.confirm({
                title: '删除任务',
                content: `确定删除任务「${task.title}」吗？`,
                okText: '删除',
                okButtonProps: { danger: true },
                cancelText: '取消',
                onOk: async () => {
                  if (await remove(task.id)) {
                    message.success('任务已删除')
                    navigate('/tasks')
                  }
                },
              })
            }
          >
            删除
          </Button>
        </Space>
      </header>

      <Descriptions bordered size="small" column={2}>
        <Descriptions.Item label="项目">{task.projectId}</Descriptions.Item>
        <Descriptions.Item label="Agent">{task.backend}</Descriptions.Item>
        <Descriptions.Item label="模型">{task.model}</Descriptions.Item>
        <Descriptions.Item label="下次执行">
          {task.nextRunAt ? new Date(task.nextRunAt).toLocaleString() : '—'}
        </Descriptions.Item>
        <Descriptions.Item label="任务描述" span={2}>
          {task.description || '—'}
        </Descriptions.Item>
      </Descriptions>

      <section className={styles.card}>
        <Typography.Title level={4}>执行提示词</Typography.Title>
        <pre className={styles.prompt}>{task.promptTemplate}</pre>
      </section>

      <Modal open={editOpen} onCancel={() => setEditOpen(false)} footer={null} title="编辑任务" width={720}>
        <TaskForm
          task={task}
          onSaved={() => {
            setEditOpen(false)
            message.success('任务已保存')
            void loadRuns(task.id)
          }}
        />
      </Modal>

      <section className={styles.card}>
        <Typography.Title level={4}>执行历史</Typography.Title>
        <Table<TaskRun>
          rowKey="id"
          size="small"
          columns={columns}
          dataSource={runs}
          pagination={false}
          locale={{ emptyText: '还没有执行记录' }}
          data-test="task-runs-table"
        />
      </section>
    </div>
  )
}
