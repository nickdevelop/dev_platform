import { describe, expect, it } from 'vitest'
import {
  buildPromptMessage,
  parsePromptMessage,
  promptBlockLabel,
  promptKindLabel,
} from '../features/prompts/promptBlock'

describe('promptBlock utility', () => {
  it('builds a role block followed by the user message', () => {
    const message = buildPromptMessage(
      { name: '代码审查', type: 'role', content: '请审查以下代码：' },
      '修复登录问题',
    )
    expect(message).toBe('<Role name="代码审查">\n请审查以下代码：\n</Role>\n\n修复登录问题')
  })

  it('builds a flow block and leaves the block alone without user text', () => {
    const message = buildPromptMessage(
      { name: '并行评审', type: 'flow', content: '先拆解再执行' },
      '   ',
    )
    expect(message).toBe('<Flow name="并行评审">\n先拆解再执行\n</Flow>')
  })

  it('escapes and decodes the prompt name', () => {
    const message = buildPromptMessage(
      { name: 'A & B "quote" <tag>', type: 'role', content: 'body' },
      'go',
    )
    expect(message).toContain('<Role name="A &amp; B &quot;quote&quot; &lt;tag&gt;">')
    const block = parsePromptMessage(message)
    expect(block?.name).toBe('A & B "quote" <tag>')
  })

  it('parses the block and leaves the user text as rest', () => {
    const block = parsePromptMessage(
      '<Role name="代码审查">\n第一行\n第二行\n</Role>\n\n帮我检查构建',
    )
    expect(block).toEqual({
      kind: 'role',
      name: '代码审查',
      body: '第一行\n第二行',
      rest: '帮我检查构建',
    })
  })

  it('parses a flow block and an empty rest', () => {
    const block = parsePromptMessage('<Flow name="并行评审">\n先拆解\n</Flow>')
    expect(block?.kind).toBe('flow')
    expect(block?.name).toBe('并行评审')
    expect(block?.rest).toBe('')
  })

  it('returns null for ordinary or malformed messages', () => {
    expect(parsePromptMessage('修复登录问题')).toBeNull()
    expect(parsePromptMessage('前言\n<Role name="x">body</Role>')).toBeNull()
    expect(parsePromptMessage('<Role name="x">body</Flow>')).toBeNull()
  })

  it('labels parsed blocks and prompt kinds', () => {
    expect(promptBlockLabel({ kind: 'role', name: '代码审查', body: '', rest: '' })).toBe(
      '启用角色：代码审查',
    )
    expect(promptBlockLabel({ kind: 'flow', name: '并行评审', body: '', rest: '' })).toBe(
      '启用流程：并行评审',
    )
    expect(promptKindLabel('role')).toBe('角色')
    expect(promptKindLabel('flow')).toBe('流程')
  })
})
