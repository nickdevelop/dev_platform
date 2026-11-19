import { describe, expect, it } from 'vitest'
import {
  formatCodeCommentToXml,
  buildCodeCommentsMessage,
  parseCodeCommentsMessage,
  getCodeSnippetSummary,
} from '../shared/utils/codeCommentBlock'
import type { CodeCommentCard } from '../features/files/types'

describe('codeCommentBlock', () => {
  const sampleCard: CodeCommentCard = {
    id: 'test-1',
    filePath: 'src/components/Test.vue',
    fileName: 'Test.vue',
    startLine: 12,
    endLine: 20,
    codeSnippet: 'const a = 1\nconst b = 2\nconsole.log(a + b)',
    comment: '检查此处的变量定义与打印',
    createdAt: 1000,
  }

  it('formats single CodeComment to XML', () => {
    const xml = formatCodeCommentToXml(sampleCard)
    expect(xml).toContain('<CodeComment path="src/components/Test.vue" line="12-20">')
    expect(xml).toContain('<Description>\n检查此处的变量定义与打印\n</Description>')
    expect(xml).toContain('<Snippet>\nconst a = 1\nconst b = 2\nconsole.log(a + b)\n</Snippet>')
    expect(xml).toContain('</CodeComment>')
  })

  it('formats single line CodeComment line attribute as single number', () => {
    const singleLineCard: CodeCommentCard = {
      ...sampleCard,
      startLine: 15,
      endLine: 15,
    }
    const xml = formatCodeCommentToXml(singleLineCard)
    expect(xml).toContain('<CodeComment path="src/components/Test.vue" line="15">')
  })

  it('builds combined message with XML blocks and user text', () => {
    const message = buildCodeCommentsMessage([sampleCard], '请根据上述代码进行优化')
    expect(message).toContain('<CodeComment path="src/components/Test.vue" line="12-20">')
    expect(message.endsWith('请根据上述代码进行优化')).toBe(true)
  })

  it('parses CodeComment blocks from message text', () => {
    const built = buildCodeCommentsMessage([sampleCard], '后续自然提问')
    const parsed = parseCodeCommentsMessage(built)

    expect(parsed.comments).toHaveLength(1)
    expect(parsed.comments[0].path).toBe('src/components/Test.vue')
    expect(parsed.comments[0].startLine).toBe(12)
    expect(parsed.comments[0].endLine).toBe(20)
    expect(parsed.comments[0].description).toBe('检查此处的变量定义与打印')
    expect(parsed.comments[0].snippet).toBe('const a = 1\nconst b = 2\nconsole.log(a + b)')
    expect(parsed.restText).toBe('后续自然提问')
  })

  it('handles message with multiple CodeComment blocks', () => {
    const card2: CodeCommentCard = {
      id: 'test-2',
      filePath: 'src/main.ts',
      fileName: 'main.ts',
      startLine: 5,
      endLine: 5,
      codeSnippet: 'createApp(App).mount("#app")',
      comment: '挂载点',
      createdAt: 2000,
    }

    const built = buildCodeCommentsMessage([sampleCard, card2], '提问正文')
    const parsed = parseCodeCommentsMessage(built)

    expect(parsed.comments).toHaveLength(2)
    expect(parsed.comments[0].path).toBe('src/components/Test.vue')
    expect(parsed.comments[1].path).toBe('src/main.ts')
    expect(parsed.comments[1].line).toBe('5')
    expect(parsed.comments[1].startLine).toBe(5)
    expect(parsed.comments[1].endLine).toBe(5)
    expect(parsed.restText).toBe('提问正文')
  })

  it('generates compact summary for code snippets (defaults to 2 lines)', () => {
    const longCode = 'line 1\nline 2\nline 3\nline 4\nline 5'
    const res = getCodeSnippetSummary(longCode)
    expect(res.totalLines).toBe(5)
    expect(res.hasMore).toBe(true)
    expect(res.summary).toBe('line 1\nline 2')

    const shortCode = 'line 1\nline 2'
    const res2 = getCodeSnippetSummary(shortCode)
    expect(res2.totalLines).toBe(2)
    expect(res2.hasMore).toBe(false)
    expect(res2.summary).toBe(shortCode)
  })
})
