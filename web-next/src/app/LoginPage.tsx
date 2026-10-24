import { Alert, Button, Form, Input, Typography } from 'antd'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { describeApiError } from '../api/client'
import { useAuthStore } from '../stores/auth'

interface LoginValues {
  password: string
}

export function LoginPage() {
  const navigate = useNavigate()
  const login = useAuthStore((state) => state.login)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  async function onFinish(values: LoginValues) {
    setSubmitting(true)
    setError('')
    try {
      await login(values.password)
      navigate('/', { replace: true })
    } catch (reason) {
      setError(describeApiError(reason))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="p0-login">
      <Typography.Title level={2}>AI 开发项目管理平台</Typography.Title>
      <Form<LoginValues> layout="vertical" onFinish={(values) => void onFinish(values)}>
        <Form.Item
          name="password"
          label="平台密码"
          rules={[{ required: true, message: '请输入平台密码' }]}
        >
          <Input.Password
            id="login-password"
            aria-label="平台密码"
            autoComplete="current-password"
            disabled={submitting}
          />
        </Form.Item>
        <Form.Item>
          <Button type="primary" htmlType="submit" loading={submitting} block>
            登录
          </Button>
        </Form.Item>
        {error !== '' && <Alert type="error" showIcon title={error} role="alert" />}
      </Form>
    </main>
  )
}
