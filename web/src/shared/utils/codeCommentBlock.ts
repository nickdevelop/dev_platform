// CodeComment XML blocks: convention for sending file selection annotations
// between the frontend composer and the agent backends, as well as rendering
// them as interactive cards in the chat conversation message view.
//
// Format:
// <CodeComment path="relative/path/to/file.ts" line="10-25">
// <Description>
// User comment or question regarding this snippet
// </Description>
// <Snippet>
// const snippet = code();
// </Snippet>
// </CodeComment>

import type { CodeCommentCard } from '../../features/files/types'

export interface ParsedCodeComment {
  path: string
  line: string
  startLine: number
  endLine: number
  description: string
  snippet: string
}

export interface ParsedCommentMessage {
  comments: ParsedCodeComment[]
  restText: string
}

function escapeXmlAttr(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

function decodeXmlAttr(value: string): string {
  return value
    .replace(/&quot;/g, '"')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&')
}

/**
 * Format a single CodeComment card to XML string.
 */
export function formatCodeCommentToXml(card: CodeCommentCard): string {
  const lineRange =
    card.startLine === card.endLine ? `${card.startLine}` : `${card.startLine}-${card.endLine}`
  const escapedPath = escapeXmlAttr(card.filePath)
  const escapedLine = escapeXmlAttr(lineRange)
  const desc = card.comment.trim()
  const snippet = card.codeSnippet.trim()

  return [
    `<CodeComment path="${escapedPath}" line="${escapedLine}">`,
    '<Description>',
    desc,
    '</Description>',
    '<Snippet>',
    snippet,
    '</Snippet>',
    '</CodeComment>',
  ].join('\n')
}

/**
 * Build the message text by prepending CodeComment XML blocks followed by the user text.
 */
export function buildCodeCommentsMessage(cards: CodeCommentCard[], userText: string): string {
  if (!cards || cards.length === 0) {
    return userText
  }
  const xmlBlocks = cards.map(formatCodeCommentToXml).join('\n\n')
  const trimmedText = userText.trim()
  if (!trimmedText) {
    return xmlBlocks
  }
  return `${xmlBlocks}\n\n${trimmedText}`
}

const CODE_COMMENT_GLOBAL_RE =
  /<CodeComment\s+path="([^"]*)"\s+line="([^"]*)"\s*>[\s\S]*?<Description>([\s\S]*?)<\/Description>[\s\S]*?<Snippet>([\s\S]*?)<\/Snippet>[\s\S]*?<\/CodeComment>/g

/**
 * Parse all CodeComment blocks from a user message.
 */
export function parseCodeCommentsMessage(text: string): ParsedCommentMessage {
  if (!text || !text.includes('<CodeComment')) {
    return { comments: [], restText: text || '' }
  }

  const comments: ParsedCodeComment[] = []
  const matches = [...text.matchAll(CODE_COMMENT_GLOBAL_RE)]

  if (matches.length === 0) {
    return { comments: [], restText: text }
  }

  for (const match of matches) {
    const rawPath = decodeXmlAttr(match[1] || '')
    const lineStr = decodeXmlAttr(match[2] || '')
    const description = (match[3] || '').trim()
    const snippet = (match[4] || '').trim()

    let startLine = 1
    let endLine = 1
    if (lineStr.includes('-')) {
      const parts = lineStr.split('-')
      startLine = parseInt(parts[0], 10) || 1
      endLine = parseInt(parts[1], 10) || startLine
    } else {
      startLine = parseInt(lineStr, 10) || 1
      endLine = startLine
    }

    comments.push({
      path: rawPath,
      line: lineStr,
      startLine,
      endLine,
      description,
      snippet,
    })
  }

  // Remove the matched CodeComment blocks from text to get restText
  const restText = text.replace(CODE_COMMENT_GLOBAL_RE, '').trim()

  return { comments, restText }
}

/**
 * Extract summary information for code snippets to keep UI compact.
 * Defaults to 2 lines for concise preview.
 */
export function getCodeSnippetSummary(
  snippet: string,
  maxLines = 2,
): { summary: string; totalLines: number; hasMore: boolean } {
  if (!snippet) {
    return { summary: '', totalLines: 0, hasMore: false }
  }
  const lines = snippet.split('\n')
  const totalLines = lines.length
  if (totalLines <= maxLines) {
    return { summary: snippet, totalLines, hasMore: false }
  }
  const summary = lines.slice(0, maxLines).join('\n')
  return { summary, totalLines, hasMore: true }
}
