import { Alert, Button, Card, Checkbox, Input, InputNumber, Select, Space, Tag } from 'antd'
import { useState } from 'react'
import type { FormInfo, PermissionDecision, PermissionRequest } from '../../api/interactions'
import styles from './InteractionCards.module.css'

interface FormField {
  key: string
  title?: string
  type?: string
  required?: boolean
  options?: Array<{ value: string; label: string }>
  description?: string
}

function fieldsOf(form: FormInfo): FormField[] {
  if (!Array.isArray(form.fields)) return []
  return (form.fields as FormField[]).filter((field) => field && typeof field.key === 'string')
}

export function PermissionCards({
  permissions,
  onReply,
}: {
  permissions: PermissionRequest[]
  onReply: (id: string, decision: PermissionDecision) => Promise<void>
}) {
  if (permissions.length === 0) return null
  return (
    <div className={styles.stack}>
      {permissions.map((permission) => (
        <Alert
          key={permission.id}
          type="warning"
          showIcon
          data-test="permission-card"
          title={`需要确认：${permission.action}`}
          description={
            <div className={styles.permissionBody}>
              {permission.message && <p>{permission.message}</p>}
              {permission.resources.length > 0 && (
                <p className={styles.resources}>{permission.resources.join(' · ')}</p>
              )}
              <Space wrap>
                <Button size="small" type="primary" onClick={() => void onReply(permission.id, 'once')}>
                  允许一次
                </Button>
                <Button size="small" onClick={() => void onReply(permission.id, 'always')}>
                  始终允许
                </Button>
                <Button size="small" danger onClick={() => void onReply(permission.id, 'reject')}>
                  拒绝
                </Button>
              </Space>
            </div>
          }
        />
      ))}
    </div>
  )
}

export function FormCards({
  forms,
  onReply,
}: {
  forms: FormInfo[]
  onReply: (id: string, answer: Record<string, unknown>) => Promise<void>
}) {
  const [values, setValues] = useState<Record<string, Record<string, unknown>>>({})
  if (forms.length === 0) return null

  return (
    <div className={styles.stack}>
      {forms.map((form) => {
        const fields = fieldsOf(form)
        const current = values[form.id] ?? {}
        return (
          <Card key={form.id} size="small" title={form.title} data-test="form-card">
            {fields.map((field) => (
              <div key={field.key} className={styles.field}>
                <label>
                  {field.title ?? field.key}
                  {field.required && <span className={styles.required}> *</span>}
                </label>
                {field.description && <p className={styles.description}>{field.description}</p>}
                {field.type === 'boolean' ? (
                  <Checkbox
                    checked={current[field.key] === true}
                    onChange={(event) =>
                      setValues((all) => ({
                        ...all,
                        [form.id]: { ...current, [field.key]: event.target.checked },
                      }))
                    }
                  />
                ) : field.type === 'multiselect' ? (
                  <Select
                    mode="multiple"
                    style={{ width: '100%' }}
                    options={field.options ?? []}
                    value={(current[field.key] as string[]) ?? []}
                    onChange={(value) =>
                      setValues((all) => ({ ...all, [form.id]: { ...current, [field.key]: value } }))
                    }
                  />
                ) : field.type === 'number' || field.type === 'integer' ? (
                  <InputNumber
                    style={{ width: '100%' }}
                    value={current[field.key] as number | undefined}
                    onChange={(value) =>
                      setValues((all) => ({ ...all, [form.id]: { ...current, [field.key]: value } }))
                    }
                  />
                ) : field.options && field.options.length > 0 ? (
                  <Select
                    style={{ width: '100%' }}
                    options={field.options}
                    value={current[field.key] as string | undefined}
                    onChange={(value) =>
                      setValues((all) => ({ ...all, [form.id]: { ...current, [field.key]: value } }))
                    }
                  />
                ) : (
                  <Input
                    value={(current[field.key] as string) ?? ''}
                    onChange={(event) =>
                      setValues((all) => ({
                        ...all,
                        [form.id]: { ...current, [field.key]: event.target.value },
                      }))
                    }
                  />
                )}
              </div>
            ))}
            {fields.length === 0 && <Tag>该提问没有可填写字段</Tag>}
            <Button
              type="primary"
              size="small"
              disabled={fields.some((field) => field.required && current[field.key] === undefined)}
              onClick={() => void onReply(form.id, current)}
            >
              提交回答
            </Button>
          </Card>
        )
      })}
    </div>
  )
}
