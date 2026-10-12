<script setup lang="ts">
// Renders one session's message log, one line per kind: a user turn is a
// right-aligned bubble, agent replies are full-width text, reasoning is a
// collapsed Thinking block, and each tool call and its result form one
// collapsed block. Tool activity is shown only when the session's capabilities
// declare tool-call events. User turns, reasoning and tool output are
// interpolated as text; agent replies are rendered as sanitized markdown (see
// ../markdown), so backend content still cannot inject live markup. An agent
// reply is cut around its ```plantuml fences first (see ../markdown-segments):
// those become PlantumlBlock diagrams instead of code blocks.
import { computed } from 'vue'
import { renderMarkdown } from '../markdown'
import { splitDiagramBlocks } from '../markdown-segments'
import PlantumlBlock from './PlantumlBlock.vue'
import type { Capabilities } from '../api/sessions'
import type { ChatEntry, StreamStatus } from '../stores/sessions'

const props = defineProps<{
  messages: ChatEntry[]
  capabilities?: Capabilities | null
  streamStatus?: StreamStatus
}>()

// Tool entries are gated on the capability, exactly like the live controls: a
// backend that does not report tool events must not show tool activity.
function toolEventsVisible(): boolean {
  return props.capabilities?.toolCallEvents === true
}

// Whether a reply is finished with. An event stream that died without a
// turn-ending event leaves its entry flagged as streaming forever, and a
// diagram fence in that reply would wait for a closing token that can never
// arrive — once the stream is gone, what is on screen is all there is.
function settled(entry: ChatEntry): boolean {
  return entry.streaming !== true || props.streamStatus === 'error'
}

// A tool call and its result are one block, keyed by the call id. An agent can
// report a result more than once; only the latest is kept so a call never
// renders several results.
const resultsByCall = computed(() => {
  const results = new Map<string, ChatEntry>()
  for (const entry of props.messages) {
    if (entry.kind === 'tool_result' && entry.callId) results.set(entry.callId, entry)
  }
  return results
})

// Every call id that has a call block. A result whose call is missing (a
// truncated history, say) is rendered on its own instead of being dropped.
const callIds = computed(() => {
  const ids = new Set<string>()
  for (const entry of props.messages) {
    if (entry.kind === 'tool_call' && entry.callId) ids.add(entry.callId)
  }
  return ids
})

function resultForCall(entry: ChatEntry): ChatEntry | undefined {
  return entry.callId ? resultsByCall.value.get(entry.callId) : undefined
}

// A result whose call is present renders inside that call's block, so it is
// dropped from the top-level list. A result with no matching call (a truncated
// history, say) stays on its own so its output is never lost.
const visibleEntries = computed(() =>
  props.messages.filter(
    (entry) =>
      !(entry.kind === 'tool_result' && entry.callId !== undefined && callIds.value.has(entry.callId)),
  ),
)

function toolName(entry: ChatEntry): string {
  return entry.name ?? entry.text
}

// The argument keys that name what a tool acted on, in preference order, so the
// collapsed line reads like `read /path` or `bash ls`. Unknown tools fall back
// to their first string argument.
const ARGUMENT_KEYS = [
  'path',
  'filePath',
  'file_path',
  'file',
  'command',
  'pattern',
  'query',
  'url',
  'prompt',
  'description',
  'name',
]

function toolArgument(entry: ChatEntry): string {
  const input = entry.input
  if (input === null || input === undefined) return ''
  if (typeof input === 'string') return input
  if (typeof input === 'object') {
    const record = input as Record<string, unknown>
    for (const key of ARGUMENT_KEYS) {
      const value = record[key]
      if (typeof value === 'string' && value.trim() !== '') return value
    }
    for (const value of Object.values(record)) {
      if (typeof value === 'string' && value.trim() !== '') return value
    }
  }
  return ''
}
</script>

<template>
  <div class="message-view">
    <p
      v-if="props.streamStatus === 'reconnecting' || props.streamStatus === 'error'"
      class="stream-status"
      role="status"
      aria-live="polite"
    >
      <template v-if="props.streamStatus === 'reconnecting'">事件流已断开，正在重连…</template>
      <template v-else>事件流已断开，无法接收新消息。</template>
    </p>

    <ol class="entries" role="log" aria-live="polite" aria-label="会话消息">
      <li v-for="entry in visibleEntries" :key="entry.id" class="chat-entry" :class="`kind-${entry.kind}`">
        <template v-if="entry.kind === 'message'">
          <div class="bubble" :class="entry.role === 'user' ? 'bubble-user' : 'bubble-agent'" data-test="bubble">
            <span v-if="entry.role === 'user'" class="message-text">{{ entry.text }}</span>
            <div v-else class="message-text markdown-body">
              <template v-for="(segment, index) in splitDiagramBlocks(entry.text)" :key="`${entry.id}:${index}`">
                <div
                  v-if="segment.kind === 'markdown'"
                  class="markdown-seg"
                  v-html="renderMarkdown(segment.text)"
                ></div>
                <PlantumlBlock
                  v-else
                  :source="segment.text"
                  :ready="segment.closed && settled(entry)"
                  :finished="settled(entry)"
                />
              </template>
            </div>
            <span v-if="entry.streaming" class="streaming-cursor" data-test="streaming" aria-hidden="true">…</span>
          </div>
        </template>

        <template v-else-if="entry.kind === 'thinking'">
          <details class="thinking" data-test="thinking">
            <summary>
              <span class="thinking-icon" aria-hidden="true">🧠</span>
              <span class="thinking-title">Thinking</span>
            </summary>
            <p class="thinking-text">{{ entry.text }}</p>
          </details>
        </template>

        <template v-else-if="entry.kind === 'tool_call'">
          <details v-if="toolEventsVisible()" class="tool" data-test="tool-call">
            <summary>
              <span class="tool-icon" aria-hidden="true">🔧</span>
              <span class="tool-name">{{ toolName(entry) }}</span>
              <span v-if="toolArgument(entry)" class="tool-arg">{{ toolArgument(entry) }}</span>
            </summary>
            <div class="tool-body">
              <pre v-if="toolArgument(entry)" class="tool-args">{{ toolArgument(entry) }}</pre>
              <pre
                v-if="resultForCall(entry)"
                class="tool-output"
                :class="{ 'is-error': resultForCall(entry)?.isError }"
              >{{ resultForCall(entry)?.text }}</pre>
              <p v-else class="tool-running" role="status">运行中…</p>
            </div>
          </details>
        </template>

        <template v-else-if="entry.kind === 'tool_result'">
          <details
            v-if="toolEventsVisible()"
            class="tool"
            data-test="tool-result"
          >
            <summary>
              <span class="tool-icon" aria-hidden="true">🔧</span>
              <span class="tool-name">工具结果</span>
            </summary>
            <div class="tool-body">
              <pre class="tool-output" :class="{ 'is-error': entry.isError }">{{ entry.text }}</pre>
            </div>
          </details>
        </template>

        <template v-else-if="entry.kind === 'error'">
          <p class="agent-error" data-test="agent-error">{{ entry.text }}</p>
        </template>

        <template v-else-if="entry.kind === 'separator'">
          <hr class="separator-rule" />
          <span class="separator-text" data-test="history-separator">{{ entry.text }}</span>
        </template>
      </li>
    </ol>

    <p v-if="props.messages.length === 0" class="empty">尚无消息。</p>
  </div>
</template>

<style scoped>
.entries {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.message {
  display: flex;
  gap: 0.5rem;
}

.message-role {
  flex-shrink: 0;
  font-weight: 600;
  text-transform: uppercase;
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.message-text {
  word-break: break-word;
}

/* A user turn keeps its own line breaks; agent replies are rendered markdown,
   whose block elements carry their own spacing. */
.bubble-user .message-text {
  white-space: pre-wrap;
}

.markdown-body :deep(p) {
  margin: 0 0 0.5rem;
}

.markdown-body :deep(p:last-child) {
  margin-bottom: 0;
}

.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3),
.markdown-body :deep(h4) {
  margin: 0.5rem 0 0.35rem;
  line-height: 1.3;
}

.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  margin: 0 0 0.5rem;
  padding-left: 1.25rem;
}

.markdown-body :deep(code) {
  background: var(--color-border);
  border-radius: 4px;
  padding: 0.05rem 0.3rem;
  font-family: var(--font-family);
  font-size: 0.85em;
}

.markdown-body :deep(pre) {
  margin: 0 0 0.5rem;
  padding: 0.6rem 0.75rem;
  border-radius: 8px;
  background: var(--color-text);
  color: var(--color-border);
  overflow-x: auto;
}

.markdown-body :deep(pre code) {
  background: transparent;
  padding: 0;
  color: inherit;
  font-size: 0.8rem;
}

.markdown-body :deep(blockquote) {
  margin: 0 0 0.5rem;
  padding-left: 0.75rem;
  border-left: 3px solid var(--color-border-strong);
  color: var(--color-text-muted);
}

.markdown-body :deep(a) {
  color: var(--color-primary);
}

.markdown-body :deep(table) {
  border-collapse: collapse;
  margin: 0 0 0.5rem;
}

.markdown-body :deep(th),
.markdown-body :deep(td) {
  border: 1px solid var(--color-border-strong);
  padding: 0.2rem 0.5rem;
}

/* A conversation reads as two columns: the user's turns sit on the right, the
   agent's on the left, so a long exchange stays scannable. */
.entries > .kind-message {
  display: flex;
}

.kind-message {
  display: flex;
}

.bubble {
  max-width: min(46rem, 82%);
  padding: 0.5rem 0.75rem;
  border-radius: 10px;
  line-height: 1.5;
}

.bubble-user {
  margin-left: auto;
  background: var(--color-info-soft);
  color: var(--color-info);
}

/* Agent replies are full-width plain text: no bubble and no background, unlike
   the user's own turns. */
.bubble-agent {
  width: 100%;
  max-width: 100%;
  margin-right: 0;
  padding: 0;
  background: transparent;
  border-radius: 0;
  color: var(--color-text);
}

.thinking,
.tool {
  width: 100%;
}

.thinking summary,
.tool summary {
  cursor: pointer;
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  font-size: 0.85rem;
  color: var(--color-text-muted);
}

.thinking-title {
  font-style: italic;
}

.thinking-icon,
.tool-icon {
  font-size: 0.9rem;
}

/* Expanded reasoning and tool bodies carry the background; their summary lines
   stay flat, matching the agent's plain-text replies. */
.thinking-text,
.tool-body {
  margin-top: 0.4rem;
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  background: var(--color-surface-muted);
}

.thinking-text {
  margin-bottom: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.tool-name {
  color: var(--color-text);
  font-weight: 600;
}

.tool-arg {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text);
  font-family: var(--font-family);
}

.tool-args,
.tool-output,
.tool-running {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-family);
  font-size: 0.8rem;
  color: var(--color-text);
}

.tool-args + .tool-output {
  margin-top: 0.4rem;
}

.tool-output.is-error {
  color: var(--color-danger);
}

.tool-running {
  color: var(--color-text-muted);
}

.stream-status {
  color: var(--color-warning);
  margin: 0 0 0.5rem;
}

.empty {
  color: var(--color-text-muted);
}

.kind-separator {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--color-text-muted);
  font-size: 0.8rem;
}

.separator-rule {
  flex: 1;
  border: none;
  border-top: 1px solid var(--color-border);
  margin: 0;
}

.separator-text {
  flex-shrink: 0;
}
</style>
