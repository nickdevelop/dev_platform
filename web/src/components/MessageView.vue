<script setup lang="ts">
// Renders one session's message log. Agent output is always interpolated as
// text — never v-html — so backend content cannot inject markup. Tool activity
// is shown only when the session's capabilities declare tool-call events.
import type { Capabilities } from '../api/sessions'
import type { ChatEntry, StreamStatus } from '../stores/sessions'

const props = defineProps<{
  messages: ChatEntry[]
  capabilities?: Capabilities | null
  streamStatus?: StreamStatus
}>()

function roleClass(role: string): string {
  return `role-${role}`
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
          <span class="message" :class="roleClass(entry.role)">
            <span class="message-role">{{ entry.role }}</span>
            <span class="message-text">{{ entry.text }}</span>
            <span v-if="entry.streaming" class="streaming-cursor" data-test="streaming" aria-hidden="true">…</span>
          </span>
        </template>

        <template v-else-if="entry.kind === 'tool_call'">
          <span v-if="props.capabilities?.toolCallEvents" class="tool-call" data-test="tool-call">
            工具调用：{{ entry.name }}
          </span>
        </template>

        <template v-else-if="entry.kind === 'tool_result'">
          <span v-if="props.capabilities?.toolCallEvents" class="tool-result" :class="{ 'is-error': entry.isError }" data-test="tool-result">
            工具结果：{{ entry.text }}
          </span>
        </template>

        <template v-else-if="entry.kind === 'error'">
          <span class="agent-error" data-test="agent-error">{{ entry.text }}</span>
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

.tool-call,
.tool-result {
  display: block;
  font-family: monospace;
  font-size: 0.85rem;
  color: #334155;
}

.tool-result.is-error,
.agent-error {
  color: #b91c1c;
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
