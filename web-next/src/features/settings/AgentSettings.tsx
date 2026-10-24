/* oxlint-disable react/set-state-in-effect -- data loading effect intentionally updates state */
import { Alert, Button, Card, Input, Modal, Space, Switch, Tag, Typography } from 'antd'
import { useEffect, useState } from 'react'
import {
  detectAgent,
  listAgents,
  setAgentEnabled,
  setAgentPath,
  type AgentDto,
} from '../../api/settings'
import { fetchAgentDiagnostics } from '../../api/agents'
import styles from './AgentSettings.module.css'

export function AgentSettings() {
  const [agents, setAgents] = useState<AgentDto[]>([])
  const [paths, setPaths] = useState<Record<string, string>>({})
  const [detections, setDetections] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [diagnostics, setDiagnostics] = useState('')
  const [diagnosticsOpen, setDiagnosticsOpen] = useState(false)

  async function load() {
    try {
      const result = await listAgents()
      setAgents(result.agents)
      setError('')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '加载 Agent 失败')
    }
  }

  useEffect(() => {
    void load()
  }, [])

  async function toggle(agent: AgentDto, enabled: boolean) {
    try {
      const updated = await setAgentEnabled(agent.backend, enabled)
      setAgents((current) => current.map((entry) => (entry.backend === agent.backend ? updated : entry)))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存失败')
    }
  }

  async function savePath(agent: AgentDto) {
    const path = paths[agent.backend] ?? ''
    try {
      const updated = await setAgentPath(agent.backend, path)
      setAgents((current) => current.map((entry) => (entry.backend === agent.backend ? updated : entry)))
      setPaths((current) => ({ ...current, [agent.backend]: '' }))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存路径失败')
    }
  }

  async function detect(agent: AgentDto) {
    try {
      const result = await detectAgent(agent.backend)
      setDetections((current) => ({
        ...current,
        [agent.backend]: result.status === 'ok' ? `检测成功${result.version ? ` · ${result.version}` : ''}` : result.error || '检测失败',
      }))
      await load()
    } catch (reason) {
      setDetections((current) => ({
        ...current,
        [agent.backend]: reason instanceof Error ? reason.message : '检测失败',
      }))
    }
  }

  return (
    <section className={styles.section} aria-label="Agent 设置">
      <div className={styles.head}>
        <Typography.Title level={2}>Agent</Typography.Title>
        <Button
          onClick={async () => {
            try {
              const report = await fetchAgentDiagnostics()
              setDiagnostics(JSON.stringify(report, null, 2))
              setDiagnosticsOpen(true)
            } catch (reason) {
              setError(reason instanceof Error ? reason.message : '诊断失败')
            }
          }}
        >
          诊断
        </Button>
      </div>
      {error !== '' && <Alert type="error" showIcon title={error} />}
      {agents.map((agent) => (
        <Card
          key={agent.backend}
          size="small"
          title={
            <Space>
              <strong>{agent.backend}</strong>
              <Tag color={agent.enabled ? 'success' : 'default'}>{agent.enabled ? '启用' : '停用'}</Tag>
              {agent.version && <Tag>{agent.version}</Tag>}
            </Space>
          }
          extra={
            <Switch
              checked={agent.enabled}
              aria-label={`启用 ${agent.backend}`}
              onChange={(checked) => void toggle(agent, checked)}
            />
          }
        >
          <div className={styles.row}>
            <Typography.Text type="secondary">
              路径来源：{agent.pathSource} {agent.path ? `· ${agent.path}` : ''}
            </Typography.Text>
            <Space.Compact style={{ width: '100%' }}>
              <Input
                placeholder="手动填写可执行文件路径"
                value={paths[agent.backend] ?? ''}
                onChange={(event) =>
                  setPaths((current) => ({ ...current, [agent.backend]: event.target.value }))
                }
              />
              <Button onClick={() => void savePath(agent)}>保存</Button>
              <Button onClick={() => void detect(agent)}>检测</Button>
            </Space.Compact>
            {detections[agent.backend] && (
              <Alert
                type={detections[agent.backend].startsWith('检测成功') ? 'success' : 'warning'}
                showIcon
                title={detections[agent.backend]}
              />
            )}
            {agent.modes && agent.modes.length > 0 && (
              <Typography.Text type="secondary">模式：{agent.modes.join(' / ')}</Typography.Text>
            )}
          </div>
        </Card>
      ))}
      {agents.length === 0 && <Typography.Text type="secondary">没有可用的 Agent 后端。</Typography.Text>}
      <Modal
        open={diagnosticsOpen}
        onCancel={() => setDiagnosticsOpen(false)}
        footer={null}
        width={720}
        title="Agent 诊断"
      >
        <pre className={styles.diagnostics}>{diagnostics}</pre>
      </Modal>
    </section>
  )
}
