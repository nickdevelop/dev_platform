import {
  Alert,
  App,
  Button,
  Card,
  Dropdown,
  Form,
  Input,
  Modal,
  Segmented,
  Select,
  Slider,
  Space,
  Splitter,
  Switch,
  Table,
  Tabs,
  Tag,
  Tree,
  Typography,
  theme as antdTheme,
} from 'antd'
import type { InputRef, MenuProps, TableColumnsType, TreeDataNode } from 'antd'
import { useEffect, useRef, useState } from 'react'
import { useAppearanceStore } from '../stores/appearance'
import { densities, themes } from '../theme/presets'
import { StrictModeProbe } from './StrictModeProbe'
import styles from './SpikePage.module.css'
import './SpikePage.global.css'

interface ModelRow {
  key: string
  id: string
  name: string
  capability: string
  status: 'ok' | 'off'
}

const modelRows: ModelRow[] = [
  { key: '1', id: 'pi/gpt-5', name: 'GPT-5', capability: '文本 / 工具', status: 'ok' },
  { key: '2', id: 'opencode/claude', name: 'Claude', capability: '文本 / 图片', status: 'ok' },
  { key: '3', id: 'deepseek/chat', name: 'DeepSeek Chat', capability: '文本', status: 'off' },
]

const fileTree: TreeDataNode[] = [
  {
    key: 'src',
    title: 'src',
    children: [
      { key: 'src/app', title: 'app' },
      { key: 'src/features', title: 'features' },
      { key: 'src/shared', title: 'shared' },
    ],
  },
  { key: 'docs', title: 'docs' },
  { key: 'README.md', title: 'README.md' },
]

const menuItems: MenuProps['items'] = [
  { key: 'pin', label: '固定标签页' },
  { key: 'close', label: '关闭标签页' },
  { key: 'others', label: '关闭其他标签页' },
]

export function SpikePage() {
  const { message } = App.useApp()
  const { token } = antdTheme.useToken()
  const appearance = useAppearanceStore((state) => state.appearance)
  const setAppearance = useAppearanceStore((state) => state.setAppearance)
  const reset = useAppearanceStore((state) => state.reset)

  const [modalOpen, setModalOpen] = useState(false)
  const modalInputRef = useRef<InputRef>(null)
  const [selectedFiles, setSelectedFiles] = useState<string[]>([])
  const [splitSizes, setSplitSizes] = useState<number[]>([45, 55])
  const [form] = Form.useForm<{ name: string; backend: string; note?: string; enabled: boolean }>()

  // Focus the first real control after the dialog has mounted. The short delay
  // lets antd's own focus trap settle first; the unit test can observe it under
  // jsdom, where CSS transition events never fire.
  useEffect(() => {
    if (!modalOpen) return
    const timer = window.setTimeout(() => modalInputRef.current?.focus(), 50)
    return () => window.clearTimeout(timer)
  }, [modalOpen])


  const columns: TableColumnsType<ModelRow> = [
    { title: '模型 ID', dataIndex: 'id', key: 'id' },
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '能力', dataIndex: 'capability', key: 'capability' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (value: ModelRow['status']) =>
        value === 'ok' ? <Tag color="success">可用</Tag> : <Tag>未启用</Tag>,
    },
  ]

  return (
    <div className={styles.page} data-test="p0-spike">
      <div className={styles.intro}>
        <div>
          <Typography.Title level={2}>React + antd P0 技术验证</Typography.Title>
          <Typography.Paragraph type="secondary">
            验证主题双轨、表单/表格/弹窗/树/分栏、路由守卫、StrictMode 幂等与生产构建。
          </Typography.Paragraph>
        </div>
        <Space wrap>
          <Button onClick={() => setModalOpen(true)} data-test="open-modal">
            打开弹窗
          </Button>
          <span data-test="modal-state">{modalOpen ? 'open' : 'closed'}</span>
          <Dropdown menu={{ items: menuItems }} trigger={['click']}>
            <Button>标签页菜单</Button>
          </Dropdown>
          <Button onClick={reset}>重置外观</Button>
        </Space>
      </div>

      <Card size="small" title="外观（4 主题 × 3 密度 × 字号）" className={styles.card}>
        <div className={styles.controls}>
          <div className={styles.controlGroup}>
            <span>主题</span>
            <Segmented
              value={appearance.theme}
              onChange={(value) => setAppearance({ theme: String(value) })}
              options={themes.map((entry) => ({ label: entry.label, value: entry.key }))}
              aria-label="主题"
              data-test="theme-control"
            />
          </div>
          <div className={styles.controlGroup}>
            <span>密度</span>
            <Segmented
              value={appearance.density}
              onChange={(value) => setAppearance({ density: String(value) })}
              options={densities.map((entry) => ({ label: entry.label, value: entry.key }))}
              aria-label="密度"
              data-test="density-control"
            />
          </div>
          <label className={styles.slider}>
            字号 {appearance.fontSize}px
            <Slider
              min={12}
              max={20}
              step={1}
              value={appearance.fontSize}
              onChange={(value) => setAppearance({ fontSize: value })}
            />
          </label>
        </div>
        <Alert
          type="info"
          showIcon
          title={`antd 派生 token: colorPrimary=${token.colorPrimary} / controlHeight=${token.controlHeight} / radius=${token.borderRadius}；页面主色由 --color-primary 固定`}
        />
      </Card>

      <div className={styles.grid}>
        <Card size="small" title="表单 + 校验 + 反馈" className={styles.card}>
          <Form
            form={form}
            layout="vertical"
            initialValues={{ enabled: true }}
            onFinish={(values) => {
              void message.success(`已提交：${values.name} / ${values.backend}`)
            }}
          >
            <Form.Item
              name="name"
              label="会话名称"
              rules={[{ required: true, message: '请输入会话名称' }]}
            >
              <Input placeholder="例如：登录页重构" data-test="form-name" />
            </Form.Item>
            <Form.Item
              name="backend"
              label="Agent 后端"
              rules={[{ required: true, message: '请选择 Agent 后端' }]}
            >
              <Select
                placeholder="选择后端"
                showSearch
                optionFilterProp="label"
                options={[
                  { value: 'pi', label: 'Pi' },
                  { value: 'opencode-v2', label: 'OpenCode V2' },
                  { value: 'deepseek', label: 'DeepSeek' },
                ]}
              />
            </Form.Item>
            <Form.Item name="note" label="备注">
              <Input.TextArea rows={2} placeholder="可选" />
            </Form.Item>
            <Form.Item name="enabled" label="启用" valuePropName="checked">
              <Switch />
            </Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" data-test="submit-form">
                提交
              </Button>
              <Button
                onClick={() => {
                  form.resetFields()
                }}
              >
                重置
              </Button>
            </Space>
          </Form>
        </Card>

        <Card size="small" title="表格（antd Table）" className={styles.card}>
          <Table<ModelRow>
            rowKey="key"
            size="small"
            columns={columns}
            dataSource={modelRows}
            pagination={false}
            data-test="model-table"
          />
        </Card>

        <Card size="small" title="文件树（antd Tree）" className={styles.card}>
          <Tree
            treeData={fileTree}
            defaultExpandAll
            selectedKeys={selectedFiles}
            onSelect={(keys) => setSelectedFiles(keys.map(String))}
            data-test="file-tree"
          />
          <Typography.Text type="secondary">
            已选：{selectedFiles.length > 0 ? selectedFiles.join(' / ') : '无'}
          </Typography.Text>
        </Card>

        <Card size="small" title="Tabs + Splitter" className={styles.card}>
          <Tabs
            size="small"
            items={[
              { key: 'activity', label: '活动', children: '活动面板内容' },
              { key: 'files', label: '文件', children: '文件面板内容' },
            ]}
          />
          <div className={styles.splitterBox}>
            <Splitter onResize={setSplitSizes} data-test="splitter">
              <Splitter.Panel defaultSize="45%" min="20%">
                <div className={styles.panel}>左栏</div>
              </Splitter.Panel>
              <Splitter.Panel>
                <div className={styles.panel}>右栏</div>
              </Splitter.Panel>
            </Splitter>
          </div>
          <Typography.Text type="secondary">尺寸：{splitSizes.join(' / ')}</Typography.Text>
        </Card>

        <Card size="small" title="StrictMode 探针" className={styles.card}>
          <StrictModeProbe />
        </Card>
      </div>

      <Modal
        title="弹窗行为验证"
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => {
          setModalOpen(false)
          void message.success('弹窗已确认')
        }}
        destroyOnHidden
        data-test="spike-modal"
      >
        <Typography.Paragraph>
          检查焦点是否进入弹窗、Esc 是否关闭、关闭后焦点是否回到触发按钮。
        </Typography.Paragraph>
        <Input ref={modalInputRef} placeholder="弹窗内输入框" data-test="modal-input" />
      </Modal>
    </div>
  )
}
