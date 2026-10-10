<script setup lang="ts">
// Renders one session's message log, one line per kind: a user turn is a
// right-aligned bubble, agent text is a left-aligned one, reasoning and tool
// output are collapsed blocks the reader can open, and tool activity is shown
// only when the session's capabilities declare tool-call events. Agent output
// is always interpolated as text — never v-html — so backend content cannot
// inject markup.
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

// toolSummary names a tool call in one line. The argument JSON, when the
// backend sent any, is a short hint rather than the whole payload.
function toolSummary(entry: ChatEntry): string {
  return entry.name ?? entry.text
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
      <li v-for="entry in props.messages" :key="entry.id" class="chat-entry" :class="`kind-${entry.kind}`">
        <template v-if="entry.kind === 'message'">
          <div class="bubble" :class="entry.role === 'user' ? 'bubble-user' : 'bubble-agent'" data-test="bubble">
            <span class="message-text">{{ entry.text }}</span>
            <span v-if="entry.streaming" class="streaming-cursor" data-test="streaming" aria-hidden="true">…</span>
          </div>
        </template>

        <template v-else-if="entry.kind === 'thinking'">
          <details class="thinking" data-test="thinking">
            <summary>
              <span class="thinking-title">{{ entry.streaming ? '思考中…' : '已思考' }}</span>
              <span class="toggle-hint">点击展开</span>
            </summary>
            <p class="thinking-text">{{ entry.text }}</p>
          </details>
        </template>

        <template v-else-if="entry.kind === 'tool_call'">
          <div v-if="toolEventsVisible()" class="tool-call" data-test="tool-call">
            <span class="tool-label">工具调用</span>
            <span class="tool-name">{{ toolSummary(entry) }}</span>
          </div>
        </template>

        <template v-else-if="entry.kind === 'tool_result'">
          <details
            v-if="toolEventsVisible()"
            class="tool-result"
            :class="{ 'is-error': entry.isError }"
            data-test="tool-result"
          >
            <summary>
              <span class="tool-label">工具结果</span>
              <span class="tool-length">{{ entry.text.length }} 字</span>
              <span class="toggle-hint">点击展开</span>
            </summary>
            <pre class="tool-output">{{ entry.text }}</pre>
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
  color: #64748b;
}

.message-text {
  white-space: pre-wrap;
  word-break: break-word;
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
  background: #dbeafe;
  color: #1e3a8a;
}

.bubble-agent {
  margin-right: auto;
  background: #f1f5f9;
  color: #1f2328;
}

.tool-call {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  font-size: 0.85rem;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.35rem 0.6rem;
  background: #fbfcfe;
}

.tool-label {
  color: #475569;
  font-weight: 600;
}

.tool-name,
.tool-output,
.tool-length,
.thinking-text {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.tool-name {
  color: #0f172a;
}

.tool-result summary,
.thinking summary {
  cursor: pointer;
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  font-size: 0.85rem;
  color: #475569;
}

.tool-result {
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.35rem 0.6rem;
  background: #fbfcfe;
}

.tool-result.is-error {
  border-color: #fecaca;
  background: #fef2f2;
}

.tool-output {
  margin: 0.4rem 0 0;
  max-height: 22rem;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 0.8rem;
  color: #334155;
}

.tool-result.is-error .tool-output {
  color: #b91c1c;
}

.toggle-hint,
.tool-length {
  color: #94a3b8;
  font-size: 0.75rem;
}

.thinking {
  border-left: 2px solid #e2e8f0;
  padding-left: 0.6rem;
}

.thinking-text {
  margin: 0.4rem 0 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 0.8rem;
  color: #64748b;
}

.tool-call,
.tool-result {
  display: block;
}

.tool-call {
  display: flex;
  align-items: baseline;
}

.stream-status {
  color: #b45309;
  margin: 0 0 0.5rem;
}

.empty {
  color: #64748b;
}

.kind-separator {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: #94a3b8;
  font-size: 0.8rem;
}

.separator-rule {
  flex: 1;
  border: none;
  border-top: 1px solid #e2e8f0;
  margin: 0;
}

.separator-text {
  flex-shrink: 0;
}
</style>
