import { Alert, Button, Form, Input, Modal, Space } from 'antd'
import { useState } from 'react'
import { describeApiError, UnauthorizedError } from '../../api/client'
import {
  createProject,
  moveProject,
  registerProject,
  renameProject,
  updateProjectName,
  unregisterProject,
  type Project,
} from '../../api/projects'

export type ProjectDialogKind =
  | 'register'
  | 'create'
  | 'move'
  | 'rename'
  | 'rename-name'
  | 'unregister'

interface ProjectManageModalProps {
  kind: ProjectDialogKind
  project: Project | null
  onClose: () => void
  onSubmitted: () => void
}

interface FormValues {
  path?: string
  name?: string
  confirmName?: string
}

const titles: Record<ProjectDialogKind, string> = {
  register: '注册已有项目目录',
  create: '新建项目目录',
  move: '移动项目',
  rename: '重命名项目目录',
  'rename-name': '修改项目名称',
  unregister: '解除项目登记（保留目录）',
}

const confirmLabels: Record<ProjectDialogKind, string> = {
  register: '注册',
  create: '创建',
  move: '移动',
  rename: '重命名目录',
  'rename-name': '保存',
  unregister: '解除登记',
}

export function ProjectManageModal({
  kind,
  project,
  onClose,
  onSubmitted,
}: ProjectManageModalProps) {
  const [form] = Form.useForm<FormValues>()
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const typedConfirmation = kind === 'move' || kind === 'rename'

  async function submit() {
    let values: FormValues
    try {
      values = await form.validateFields()
    } catch {
      return
    }
    setSubmitting(true)
    setError('')
    try {
      switch (kind) {
        case 'register':
          await registerProject((values.path ?? '').trim())
          break
        case 'create':
          await createProject((values.path ?? '').trim(), (values.name ?? '').trim())
          break
        case 'move':
          await moveProject(project!.id, (values.path ?? '').trim(), values.confirmName ?? '')
          break
        case 'rename':
          await renameProject(project!.id, (values.name ?? '').trim(), values.confirmName ?? '')
          break
        case 'rename-name':
          await updateProjectName(project!.id, (values.name ?? '').trim())
          break
        case 'unregister':
          await unregisterProject(project!.id)
          break
      }
      onSubmitted()
      onClose()
    } catch (reason) {
      if (reason instanceof UnauthorizedError) return
      setError(describeApiError(reason))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      open
      title={`${titles[kind]}${project && kind !== 'register' && kind !== 'create' ? ` ${project.name}` : ''}`}
      onCancel={onClose}
      destroyOnHidden
      footer={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button
            type="primary"
            danger={kind === 'unregister'}
            loading={submitting}
            onClick={() => void submit()}
          >
            {confirmLabels[kind]}
          </Button>
        </Space>
      }
    >
      <Form
        form={form}
        layout="vertical"
        preserve={false}
        initialValues={{ name: kind === 'rename-name' ? project?.name ?? '' : '' }}
      >
        {(kind === 'register' || kind === 'create' || kind === 'move') && (
          <Form.Item
            name="path"
            label={
              kind === 'register'
                ? '目录的绝对路径'
                : kind === 'create'
                  ? '父目录的绝对路径'
                  : '目标完整路径'
            }
            rules={[{ required: true, message: '请输入绝对路径' }]}
          >
            <Input id="dialog-path" autoComplete="off" spellCheck={false} />
          </Form.Item>
        )}
        {(kind === 'create' || kind === 'rename' || kind === 'rename-name') && (
          <Form.Item
            name="name"
            label={kind === 'create' ? '项目目录名' : kind === 'rename' ? '新目录名' : '项目显示名称'}
            rules={[{ required: true, message: '请输入名称' }]}
          >
            <Input
              id={kind === 'create' ? 'dialog-name' : kind === 'rename' ? 'dialog-new-name' : 'dialog-display-name'}
              autoComplete="off"
              spellCheck={false}
            />
          </Form.Item>
        )}
        {typedConfirmation && project && (
          <Form.Item
            name="confirmName"
            label={`输入当前项目名“${project.name}”以确认`}
            rules={[
              { required: true, message: '请输入当前项目名' },
              {
                validator: async (_, value) => {
                  if (value !== project.name) throw new Error('输入的项目名不一致')
                },
              },
            ]}
          >
            <Input id="dialog-confirm-name" autoComplete="off" />
          </Form.Item>
        )}
        {kind === 'unregister' && (
          <Alert
            type="warning"
            showIcon
            title="目录会被完整保留"
            description="解除登记只移除平台记录并停止实例，不会删除磁盘目录。"
          />
        )}
        {error !== '' && <Alert type="error" showIcon title={error} role="alert" />}
      </Form>
    </Modal>
  )
}
