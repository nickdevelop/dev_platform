// Platform built-in slash commands handled on the client side.

export type BuiltInAction = 'new' | 'stop' | 'model' | 'help'

export interface BuiltInCommand {
  name: string
  aliases?: string[]
  description: string
  action: BuiltInAction
  argumentHint?: string
}

export interface BuiltInCommandResult {
  command: BuiltInCommand
  args: string
}

export const BUILT_IN_COMMANDS: BuiltInCommand[] = [
  {
    name: 'new',
    aliases: ['clear'],
    description: '新建并切换到全新会话',
    action: 'new',
  },
  {
    name: 'stop',
    description: '停止当前正在运行的 Agent 任务',
    action: 'stop',
  },
  {
    name: 'model',
    description: '切换当前会话所使用的模型',
    action: 'model',
    argumentHint: 'model-name',
  },
  {
    name: 'help',
    description: '查看可用命令与快捷操作提示',
    action: 'help',
  },
]

const commandMap = new Map<string, BuiltInCommand>()

for (const cmd of BUILT_IN_COMMANDS) {
  commandMap.set(cmd.name.toLowerCase(), cmd)
  if (cmd.aliases) {
    for (const alias of cmd.aliases) {
      commandMap.set(alias.toLowerCase(), cmd)
    }
  }
}

/**
 * Detects whether the user input line is a platform built-in slash command.
 */
export function detectBuiltInCommand(input: string): BuiltInCommandResult | null {
  const trimmed = input.trim()
  if (!trimmed.startsWith('/')) return null

  const spaceIndex = trimmed.indexOf(' ')
  const cmdPart = spaceIndex === -1 ? trimmed.slice(1) : trimmed.slice(1, spaceIndex)
  const args = spaceIndex === -1 ? '' : trimmed.slice(spaceIndex + 1).trim()

  const cmd = commandMap.get(cmdPart.toLowerCase())
  if (!cmd) return null

  return { command: cmd, args }
}
