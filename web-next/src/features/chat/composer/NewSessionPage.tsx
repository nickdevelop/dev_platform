/* oxlint-disable react/set-state-in-effect -- model list loading effect intentionally updates state */
import { Alert, Button, Card, Empty, Form, Input, Select, Space, Typography } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { listAgentBackendModels, type ModelInfo } from '../../../api/agents'
import { createProjectSession } from '../../../api/sessions'
import { useAgentsStore } from '../../../stores/agents'
import { useProjectsStore } from '../../../stores/projects'
import { usePromptsStore } from '../../../stores/prompts'
import { applyUserPromptTemplate, hasUserPromptPlaceholder, USER_PROMPT_PLACEHOLDER } from '../../prompts/promptTemplate'
import styles from './NewSessionPage.module.css'

interface NewSessionValues {
  projectId: number
  backend: string
  model?: string
  promptId?: number
  message?: string
}

export function NewSessionPage() {
  const { projectId: routeProjectId } = useParams()
  const navigate = useNavigate()
  const [form] = Form.useForm<NewSessionValues>()
  const [models, setModels] = useState<ModelInfo[]>([])
  const [modelsError, setModelsError] = useState('')
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)

  const projects = useProjectsStore((state) => state.projects)
  const backends = useAgentsStore((state) => state.backends)
  const prompts = usePromptsStore((state) => state.prompts)
  const loadPrompts = usePromptsStore((state) => state.loadPrompts)
  const backend = Form.useWatch('backend', form)
  const promptId = Form.useWatch('promptId', form)
  const messageText = Form.useWatch('message', form) ?? ''
  const selectedPrompt = prompts.find((prompt) => prompt.id === promptId)

  const availableBackends = useMemo(
    () => backends.filter((entry) => entry.enabled !== false),
    [backends],
  )

  useEffect(() => {
    void loadPrompts()
    const initialProject = routeProjectId ? Number(routeProjectId) : projects[0]?.id
    form.setFieldsValue({
      projectId: initialProject,
      backend: availableBackends[0]?.backend,
    })
  }, [form, loadPrompts, projects, routeProjectId, availableBackends])

  useEffect(() => {
    if (!backend) {
      setModels([])
      return
    }
    let cancelled = false
    setModelsError('')
    void listAgentBackendModels(backend)
      .then((result) => {
        if (cancelled) return
        setModels(result.models)
        const current = form.getFieldValue('model')
        if (!current && result.models[0]) form.setFieldValue('model', result.models[0].id)
      })
      .catch((reason: unknown) => {
        if (cancelled) return
        setModels([])
        setModelsError(reason instanceof Error ? reason.message : '模型列表加载失败')
      })
    return () => {
      cancelled = true
    }
  }, [backend, form])

  async function submit(values: NewSessionValues) {
    setCreating(true)
    setError('')
    const prompt = prompts.find((entry) => entry.id === values.promptId)
    const content = prompt
      ? applyUserPromptTemplate(prompt.content, values.message ?? '')
      : (values.message ?? '').trim()
    try {
      const result = await createProjectSession(values.projectId, {
        backend: values.backend,
        model: values.model,
        message: content || undefined,
      })
      if (result.messageError) {
        setError(`会话已创建，但首条消息未发送：${result.messageError}`)
      }
      navigate(`/projects/${values.projectId}/sessions/${result.session.id}`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '创建会话失败')
    } finally {
      setCreating(false)
    }
  }

  if (projects.length === 0) {
    return (
      <div className={styles.page}>
        <Empty description="还没有项目，请先注册或创建项目" />
      </div>
    )
  }

  return (
    <div className={styles.page} data-test="new-session-page">
      <Card className={styles.card} title="新建会话">
        <Form
          form={form}
          layout="vertical"
          initialValues={{ backend: availableBackends[0]?.backend }}
          onFinish={(values) => void submit(values)}
        >
          <Form.Item name="projectId" label="项目" rules={[{ required: true, message: '请选择项目' }]}>
            <Select
              options={projects.map((project) => ({ value: project.id, label: project.name }))}
              onChange={() => form.setFieldValue('model', undefined)}
            />
          </Form.Item>
          <Form.Item name="backend" label="Agent 后端" rules={[{ required: true, message: '请选择后端' }]}>
            <Select
              options={availableBackends.map((entry) => ({
                value: entry.backend,
                label: entry.label || entry.backend,
              }))}
              onChange={() => form.setFieldValue('model', undefined)}
            />
          </Form.Item>
          <Form.Item name="model" label="模型">
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              placeholder={modelsError || '使用后端默认模型'}
              options={models.map((model) => ({
                value: model.id,
                label: `${model.name || model.id}${model.provider ? ` · ${model.provider}` : ''}`,
              }))}
            />
          </Form.Item>
          <Form.Item name="promptId" label="提示词模板（可选）">
            <Select
              allowClear
              placeholder="选择一个提示词模板"
              options={prompts.map((prompt) => ({ value: prompt.id, label: prompt.name }))}
            />
          </Form.Item>
          <Form.Item
            name="message"
            label={selectedPrompt ? '补充内容（可选）' : '首条消息（可选）'}
            extra={
              selectedPrompt
                ? hasUserPromptPlaceholder(selectedPrompt.content)
                  ? `输入内容将替换 ${USER_PROMPT_PLACEHOLDER}`
                  : '输入内容将追加在提示词模板之后'
                : undefined
            }
          >
            <Input.TextArea rows={5} placeholder="描述会话，例如：帮我检查构建失败的原因…" />
          </Form.Item>
          {selectedPrompt && (
            <Alert
              type="info"
              showIcon
              className={styles.preview}
              title="将发送的内容预览"
              description={
                <pre className={styles.previewText}>
                  {applyUserPromptTemplate(selectedPrompt.content, messageText)}
                </pre>
              }
            />
          )}
          {modelsError !== '' && <Alert type="warning" showIcon title={modelsError} />}
          {error !== '' && <Alert type="error" showIcon title={error} />}
          <Space>
            <Button type="primary" htmlType="submit" loading={creating}>
              创建会话
            </Button>
            <Button onClick={() => navigate('/')}>取消</Button>
          </Space>
        </Form>
      </Card>
      <Typography.Paragraph type="secondary" className={styles.hint}>
        会话创建后会自动打开；标题由服务端根据首条消息生成。
      </Typography.Paragraph>
    </div>
  )
}
