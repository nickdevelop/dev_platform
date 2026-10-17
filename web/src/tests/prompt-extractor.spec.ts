import { describe, expect, it } from 'vitest'
import type { ChatEntry } from '../features/chat/state/sessionLog'
import { extractExecutionPrompt } from '../features/tasks/promptExtractor'

function msg(role: string, text: string): ChatEntry {
  return {
    id: `m_${Math.random()}`,
    kind: 'message',
    role,
    text,
    streaming: false,
  }
}

describe('extractExecutionPrompt', () => {
  it('extracts plan following ## 执行方案 header', () => {
    const entries: ChatEntry[] = [
      msg('user', '请帮我分析部署任务'),
      msg(
        'assistant',
        `经过深入分析，项目不存在歧义。

## 执行方案
1. 运行 npm install 安装依赖
2. 构建前端 npm run build
3. 启动后台服务 ./bin/platform

请确认以上方案后即可开始执行。`,
      ),
    ]

    const prompt = extractExecutionPrompt(entries)
    expect(prompt).toContain('1. 运行 npm install 安装依赖')
    expect(prompt).toContain('2. 构建前端 npm run build')
    expect(prompt).toContain('3. 启动后台服务 ./bin/platform')
    expect(prompt).not.toContain('经过深入分析，项目不存在歧义')
    expect(prompt).not.toContain('请确认以上方案')
  })

  it('extracts plan following 【可直接执行方案】 format', () => {
    const entries: ChatEntry[] = [
      msg(
        'assistant',
        `收到。
【可直接执行方案】：
- [ ] 备份数据库
- [ ] 迁移表结构
- [ ] 触发增量同步`,
      ),
    ]

    const prompt = extractExecutionPrompt(entries)
    expect(prompt).toContain('- [ ] 备份数据库')
    expect(prompt).toContain('- [ ] 迁移表结构')
    expect(prompt).not.toContain('收到。')
  })

  it('extracts numbered list directly when no explicit header', () => {
    const entries: ChatEntry[] = [
      msg(
        'assistant',
        `好的，以下是具体执行步骤：
1. 检查环境变量配置
2. 重启 docker 服务容器
3. 校验端口 8080 是否正常响应`,
      ),
    ]

    const prompt = extractExecutionPrompt(entries)
    expect(prompt).toContain('1. 检查环境变量配置')
    expect(prompt).toContain('3. 校验端口 8080 是否正常响应')
  })

  it('falls back to task title and description when no assistant message exists', () => {
    const entries: ChatEntry[] = [msg('user', '初始提问')]
    const prompt = extractExecutionPrompt(entries, {
      title: '定时数据清理任务',
      description: '每天凌晨清理超过30天的临时文件',
    })

    expect(prompt).toBe('【执行方案】定时数据清理任务\n需求与目标：每天凌晨清理超过30天的临时文件')
  })

  it('returns empty string when no messages and no fallback', () => {
    expect(extractExecutionPrompt([])).toBe('')
  })
})
