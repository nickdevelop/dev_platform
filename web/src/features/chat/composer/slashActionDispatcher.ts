// Platform Slash Command Action Dispatcher.
// Decouples slash command execution routing and error feedback from UI panels.

import { compactSession } from '../../../api/sessions'

export interface SlashActionContext {
  projectId: number
  sessionId: string
  findCommand: (name: string) => { name: string; category?: string } | undefined
  compactSession?: typeof compactSession
}

export interface SlashActionResult {
  handled: boolean
  notice?: string
  clearInput?: boolean
}

export type SlashActionHandler = (
  args: string,
  ctx: SlashActionContext,
) => Promise<{ notice: string; clearInput?: boolean }>

// Action registry for platform-level executable slash commands.
const actionHandlers: Record<string, SlashActionHandler> = {
  compact: async (args, ctx) => {
    const compactFn = ctx.compactSession ?? compactSession
    try {
      await compactFn(ctx.projectId, ctx.sessionId, args)
      return {
        notice: '已向后端发送会话历史上下文压缩指令。',
        clearInput: true,
      }
    } catch (err: unknown) {
      return {
        notice: `压缩执行失败: ${err instanceof Error ? err.message : String(err)}`,
        clearInput: false,
      }
    }
  },
}

/**
 * Parses and executes platform slash commands when matched.
 * Returns handled=true when the input was intercepted and processed.
 */
export async function dispatchSlashCommand(
  input: string,
  ctx: SlashActionContext,
): Promise<SlashActionResult> {
  const trimmed = input.trim()
  if (!trimmed.startsWith('/')) {
    return { handled: false }
  }

  const spaceIndex = trimmed.indexOf(' ')
  const name = spaceIndex === -1 ? trimmed.slice(1) : trimmed.slice(1, spaceIndex)
  const args = spaceIndex === -1 ? '' : trimmed.slice(spaceIndex + 1).trim()
  const lowerName = name.toLowerCase()

  const cmd = ctx.findCommand(lowerName)

  // 1. Registered platform action commands (e.g. /compact)
  const handler = actionHandlers[lowerName]
  if (handler) {
    const res = await handler(args, ctx)
    return {
      handled: true,
      notice: res.notice,
      clearInput: res.clearInput !== false,
    }
  }

  // 2. Prompt injection commands (leave to composer to expand / send)
  if (cmd?.category === 'prompt') {
    return { handled: false }
  }

  // 3. Unknown or unsupported commands fallback notice
  return {
    handled: true,
    notice: `未知命令 /${name}。当前平台支持的命令：/compact`,
    clearInput: true,
  }
}
