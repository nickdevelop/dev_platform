/* oxlint-disable react/set-state-in-effect -- data loading effect intentionally updates state */
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { Alert, App, Button, Card, Form, Input, InputNumber, Modal, Space, Switch, Table, Tag, Typography } from 'antd'
import type { TableColumnsType } from 'antd'
import { useEffect, useState } from 'react'
import {
  addModel,
  createService,
  deleteService,
  listServices,
  testModel,
  updateModel,
  updateService,
  type ModelDto,
  type ServiceDto,
} from '../../api/models'
import styles from './ModelSettings.module.css'

interface ServiceForm {
  name: string
  baseUrl: string
  apiKey: string
  enabled: boolean
}

interface ModelForm {
  modelId: string
  name: string
  contextTokens: number
  outputTokens: number
  supportsText: boolean
  supportsImage: boolean
  supportsTools: boolean
}

export function ModelSettings() {
  const { modal, message } = App.useApp()
  const [services, setServices] = useState<ServiceDto[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [testing, setTesting] = useState<number | null>(null)
  const [serviceOpen, setServiceOpen] = useState(false)
  const [modelFor, setModelFor] = useState<ServiceDto | null>(null)
  const [editingService, setEditingService] = useState<ServiceDto | null>(null)
  const [editingModel, setEditingModel] = useState<ModelDto | null>(null)
  const [serviceForm] = Form.useForm<ServiceForm>()
  const [modelForm] = Form.useForm<ModelForm>()

  async function load() {
    setLoading(true)
    try {
      const result = await listServices()
      setServices(result.services)
      setError('')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '加载模型服务失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  async function submitService() {
    const values = await serviceForm.validateFields()
    try {
      if (editingService) await updateService(editingService.id, values)
      else await createService(values)
      message.success('模型服务已创建')
      setServiceOpen(false)
      setEditingService(null)
      serviceForm.resetFields()
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '创建失败')
    }
  }

  async function submitModel() {
    if (!modelFor) return
    const values = await modelForm.validateFields()
    try {
      if (editingModel) await updateModel(editingModel.id, { ...values, enabled: true })
      else await addModel(modelFor.id, { ...values, enabled: true })
      message.success('模型已添加')
      setModelFor(null)
      setEditingModel(null)
      modelForm.resetFields()
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '添加模型失败')
    }
  }

  function confirmDelete(service: ServiceDto) {
    modal.confirm({
      title: '删除模型服务',
      content: `删除服务「${service.name}」及其全部模型？`,
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        await deleteService(service.id)
        message.success('服务已删除')
        await load()
      },
    })
  }

  const columns: TableColumnsType<ModelDto> = [
    { title: '模型 ID', dataIndex: 'modelId', key: 'modelId' },
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: '能力',
      key: 'capabilities',
      render: (_, model) => (
        <Space size={2}>
          {model.supportsText && <Tag>文本</Tag>}
          {model.supportsImage && <Tag>图片</Tag>}
          {model.supportsTools && <Tag>工具</Tag>}
        </Space>
      ),
    },
    { title: '上下文', dataIndex: 'contextTokens', key: 'contextTokens' },
    { title: '输出', dataIndex: 'outputTokens', key: 'outputTokens' },
    {
      title: '状态',
      key: 'status',
      render: (_, model) =>
        model.usable ? <Tag color="success">可用</Tag> : <Tag color="error">不可用</Tag>,
    },
    {
      title: '操作',
      key: 'action',
      width: 140,
      render: (_, model) => (
        <>
        <Button
          size="small"
          type="link"
          onClick={() => {
            const service = services.find((entry) => entry.id === model.serviceId)
            if (!service) return
            setModelFor(service)
            setEditingModel(model)
            modelForm.setFieldsValue({
              modelId: model.modelId,
              name: model.name,
              contextTokens: model.contextTokens,
              outputTokens: model.outputTokens,
              supportsText: model.supportsText,
              supportsImage: model.supportsImage,
              supportsTools: model.supportsTools,
            })
          }}
        >
          编辑
        </Button>
        <Button
          size="small"
          type="link"
          loading={testing === model.id}
          onClick={async () => {
            const service = services.find((entry) => entry.id === model.serviceId)
            if (!service) return
            setTesting(model.id)
            try {
              const result = await testModel({
                baseUrl: service.baseUrl,
                secretRef: service.hasKey ? service.secretRef : undefined,
                modelId: model.modelId,
              })
              message.success(`调用正常（${result.latencyMs}ms）`)
            } catch (reason) {
              message.error(reason instanceof Error ? reason.message : '测试失败')
            } finally {
              setTesting(null)
            }
          }}
        >
          测试
        </Button>
        </>
      ),
    },
  ]

  return (
    <section className={styles.section} aria-label="模型设置">
      <div className={styles.head}>
        <Typography.Title level={2}>模型</Typography.Title>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => {
            setEditingService(null)
            serviceForm.resetFields()
            setServiceOpen(true)
          }}
        >
          新增服务
        </Button>
      </div>
      {error !== '' && <Alert type="error" showIcon title={error} />}
      {loading && services.length === 0 && <Typography.Text>加载中…</Typography.Text>}
      {services.map((service) => (
        <Card
          key={service.id}
          size="small"
          className={styles.service}
          title={
            <Space>
              <strong>{service.name}</strong>
              <Typography.Text type="secondary">{service.baseUrl}</Typography.Text>
              {!service.enabled && <Tag>已停用</Tag>}
              {service.hasKey && <Tag color="success">已配置 Key</Tag>}
            </Space>
          }
          extra={
            <Space>
              <Button
                size="small"
                onClick={() => {
                  setEditingService(service)
                  serviceForm.setFieldsValue({
                    name: service.name,
                    baseUrl: service.baseUrl,
                    apiKey: '',
                    enabled: service.enabled,
                  })
                  setServiceOpen(true)
                }}
              >
                编辑
              </Button>
              <Button
                size="small"
                icon={<PlusOutlined />}
                onClick={() => {
                  setEditingModel(null)
                  setModelFor(service)
                  modelForm.resetFields()
                }}
              >
                添加模型
              </Button>
              <Button size="small" danger icon={<DeleteOutlined />} onClick={() => confirmDelete(service)}>
                删除
              </Button>
            </Space>
          }
        >
          <Table<ModelDto>
            rowKey="id"
            size="small"
            columns={columns}
            dataSource={service.models}
            pagination={false}
            locale={{ emptyText: '该服务还没有模型' }}
          />
        </Card>
      ))}
      {!loading && services.length === 0 && <Typography.Text type="secondary">还没有模型服务。</Typography.Text>}

      <Modal
        title={editingService ? '编辑模型服务' : '新增模型服务'}
        open={serviceOpen}
        onCancel={() => setServiceOpen(false)}
        onOk={() => void submitService()}
        okText="保存"
        cancelText="取消"
      >
        <Form form={serviceForm} layout="vertical" initialValues={{ enabled: true }}>
          <Form.Item name="name" label="服务名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="baseUrl" label="API 地址" rules={[{ required: true }]}>
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>
          <Form.Item name="apiKey" label="API Key">
            <Input.Password />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={editingModel ? '编辑模型' : `向 ${modelFor?.name ?? ''} 添加模型`}
        open={modelFor !== null}
        onCancel={() => setModelFor(null)}
        onOk={() => void submitModel()}
        okText="添加"
        cancelText="取消"
      >
        <Form
          form={modelForm}
          layout="vertical"
          initialValues={{
            supportsText: true,
            supportsImage: false,
            supportsTools: false,
            contextTokens: 0,
            outputTokens: 0,
          }}
        >
          <Form.Item name="modelId" label="模型 ID" rules={[{ required: true }]}>
            <Input placeholder="gpt-4o" />
          </Form.Item>
          <Form.Item name="name" label="显示名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Space>
            <Form.Item name="supportsText" label="文本" valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="supportsImage" label="图片" valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="supportsTools" label="工具" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
          <Space>
            <Form.Item name="contextTokens" label="上下文">
              <InputNumber min={0} />
            </Form.Item>
            <Form.Item name="outputTokens" label="输出">
              <InputNumber min={0} />
            </Form.Item>
          </Space>
        </Form>
      </Modal>
    </section>
  )
}
