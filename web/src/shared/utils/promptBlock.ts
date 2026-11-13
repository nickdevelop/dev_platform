// Role/Flow prompt blocks: the platform convention for the first user message
// of an agent session. A prompt block is built here when a session starts and
// parsed here when the conversation renders it. Both sides (Go title
// derivation and this module) must agree on the exact shape:
//
//	<Role name="name">
//	prompt body
//	</Role>
//
// followed by a blank line and the user's own message.

export type PromptKind = 'role' | 'flow'

export interface PromptBlock {
  kind: PromptKind
  // Prompt display name, with attribute escaping undone.
  name: string
  // Prompt content between the tags.
  body: string
  // The user's own message after the block.
  rest: string
}

const TAG_BY_KIND: Record<PromptKind, 'Role' | 'Flow'> = {
  role: 'Role',
  flow: 'Flow',
}

const KIND_BY_TAG: Record<string, PromptKind> = {
  Role: 'role',
  Flow: 'flow',
}

// The trailing \s* consumes the separator between the block and the user's
// message; only a leading block is recognised.
const PROMPT_BLOCK_RE = /^\s*<(Role|Flow)\s+name="([^"]*)"\s*>([\s\S]*?)<\/\1>\s*/
const PROMPT_NAME_BREAKS_RE = /[\r\n\t]/g

function escapePromptName(name: string): string {
  return name
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(PROMPT_NAME_BREAKS_RE, ' ')
}

function decodePromptName(name: string): string {
  // Decode &amp; last so an escaped entity sequence survives round-tripping.
  return name
    .replace(/&quot;/g, '"')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&')
}

/**
 * Builds the first user message for a session that selected a role or flow
 * prompt. The prompt block comes first; the user's own text follows after one
 * blank line. An empty user text leaves the block alone.
 */
export function buildPromptMessage(
  prompt: { name: string; type: PromptKind; content: string },
  userText: string,
): string {
  const tag = TAG_BY_KIND[prompt.type] ?? 'Role'
  const body = prompt.content.trim()
  const block = `<${tag} name="${escapePromptName(prompt.name)}">\n${body}\n</${tag}>`
  const rest = userText.trim()
  return rest === '' ? block : `${block}\n\n${rest}`
}

/**
 * Parses a leading Role/Flow prompt block. Returns null for ordinary messages
 * or a malformed block so the caller can render the text unchanged.
 */
export function parsePromptMessage(text: string): PromptBlock | null {
  const match = PROMPT_BLOCK_RE.exec(text)
  if (!match) return null
  const kind = KIND_BY_TAG[match[1]]
  if (!kind) return null
  return {
    kind,
    name: decodePromptName(match[2]),
    body: match[3].trim(),
    rest: text.slice(match[0].length).trim(),
  }
}

/** The session-view label for a parsed prompt block. */
export function promptBlockLabel(block: PromptBlock): string {
  return `${block.kind === 'role' ? '启用角色' : '启用流程'}：${block.name}`
}

/** The short type label used by prompt pickers. */
export function promptKindLabel(kind: PromptKind): string {
  return kind === 'role' ? '角色' : '流程'
}
