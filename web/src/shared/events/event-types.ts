// The complete eventType set an SSE frame of the unified bus can carry.
// Both the SharedWorker and the main-thread fallback listen per type with
// EventSource#addEventListener — a type missing from this list is never
// parsed and its frames are silently dropped by the browser, so every event
// type the backend can emit on the wire must appear here exactly once.
// The list is the union of the domain channels (sessions/chat/tasks/system):
// `lifecycle` and `unknown` come from the session event normalization
// contract (internal/core/agents), `chunk`/`delta` style names from the chat
// and task domains.
export const BUS_EVENT_TYPES = [
  'delta',
  'message',
  'thinking',
  'tool_call',
  'tool_result',
  'turn',
  'interactions',
  'interaction',
  'resync',
  'lifecycle',
  'status_change',
  'step_update',
  'chunk',
  'status',
  'conversation',
  'snapshot',
  'message_update',
  'closed',
  'error',
  'ready',
  'unknown',
] as const

export type BusEventType = (typeof BUS_EVENT_TYPES)[number]
