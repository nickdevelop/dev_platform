// One rendered line in a chat log, provider-neutral. Agent sessions and pure
// chats share the same vocabulary so the renderer, copy action, markdown and
// image handling are not duplicated. Pure chat only produces message,
// thinking and error entries; the tool fields exist for the agent workbench.

import type {
  ImageAttachment,
  MessageModel,
  MessageUsage,
} from '../../../api/messageTypes'

export type StreamStatus = 'idle' | 'connecting' | 'open' | 'reconnecting' | 'error' | 'ended'

export interface ChatEntry {
  id: string
  kind: 'message' | 'thinking' | 'tool_call' | 'tool_result' | 'error' | 'notice'
  role: string
  text: string
  images?: ImageAttachment[]
  streaming: boolean
  // Timestamp of the entry, formatted as ISO string.
  timestamp?: string
  // The model that produced an assistant message, when one is known.
  model?: MessageModel
  // Exact token usage of a final assistant message, when reported.
  usage?: MessageUsage
  callId?: string
  name?: string
  // Tool calls carry the raw argument payload so the view can summarise it
  // without the store knowing tool schemas.
  input?: unknown
  isError?: boolean
  // Normalized edit summary for file tools (edit).
  diff?: { file: string; patch?: string; additions: number; deletions: number }
}
