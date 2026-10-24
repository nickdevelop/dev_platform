// Provider-neutral message primitives shared by the agent session API and the
// pure-chat API. Keeping them here prevents pure chat from depending on the
// agent session DTO module just to describe an image or a model tag.

export type ImageMediaType = 'image/jpeg' | 'image/png' | 'image/webp' | 'image/gif'

export interface ImageAttachment {
  id: string
  name: string
  mediaType: ImageMediaType
  data: string
  size: number
}

// Mirrors agents.MessageModel: the model that produced one assistant message.
// id is the provider-scoped "provider/model" form; variant is set when the
// model ran a named variant.
export interface MessageModel {
  id: string
  provider: string
  variant?: string
}

// Exact backend-reported token usage. Never estimated.
export interface MessageUsage {
  input: number
  output: number
  reasoning?: number
  cacheRead?: number
  cacheWrite?: number
}
