<script setup lang="ts">
// Dedicated Activity Panel view for session reasoning, tool executions, and subagent tasks.
// Extracted from MessageView to enforce single-responsibility and decouple live tool monitoring
// from markdown message rendering.

import { computed, ref } from 'vue'
import RunningIndicator from '../../../shared/components/RunningIndicator.vue'
import type { Capabilities } from '../../../api/sessions'
import type { ChatEntry } from '../state/entry'
import * as log from './logModel'

const props = defineProps<{
  messages: ChatEntry[]
  capabilities?: Capabilities | null
  running?: boolean
}>()

const subagentModal = ref<ChatEntry | null>(null)

const model = computed(() => log.buildLogModel(props.messages))

function toolEventsVisible(): boolean {
  return log.toolEventsVisible(props.capabilities)
}

function toolEventsVisibleFor(entry: ChatEntry): boolean {
  if (entry.kind !== 'tool_call' && entry.kind !== 'tool_result') return true
  return toolEventsVisible()
}

const visibleEntries = computed<ChatEntry[]>(() =>
  props.messages.filter((entry) => {
    if (entry.kind === 'tool_call' || entry.kind === 'tool_result') {
      return toolEventsVisible()
    }
    return true
  }),
)

const flatActivityEntries = computed<ChatEntry[]>(() =>
  visibleEntries.value.filter(
    (entry) => log.isActivity(entry) && !log.isQuestionCall(entry) && toolEventsVisibleFor(entry),
  ),
)

const resultForCall = (entry: ChatEntry): ChatEntry | undefined => model.value.resultForCall(entry)

function toolSummaryText(entry: ChatEntry): string {
  return log.toolSummaryText(entry, resultForCall)
}

function toolIcon(entry: ChatEntry): string {
  return log.toolIcon(entry)
}

function isSubagentCall(entry: ChatEntry): boolean {
  return log.isSubagentCall(entry, resultForCall)
}

function subagentName(entry: ChatEntry): string {
  return log.subagentName(entry)
}

function subagentSessionId(entry: ChatEntry): string {
  return log.subagentSessionId(entry, resultForCall)
}

function toolName(entry: ChatEntry): string {
  return log.toolName(entry)
}

function toolArgument(entry: ChatEntry): string {
  return log.toolArgument(entry)
}

function prettyInput(entry: ChatEntry): string {
  return log.prettyInput(entry)
}

function openSubagent(entry: ChatEntry) {
  subagentModal.value = entry
}

function closeSubagent() {
  subagentModal.value = null
}
</script>

<template>
  <div class="session-activity-log">
    <ol
      class="entries flat-activity"
      role="log"
      aria-live="polite"
      aria-label="活动"
    >
      <li v-for="entry in flatActivityEntries" :key="entry.id" class="chat-entry" :class="`kind-${entry.kind}`">
        <!-- 思考内容直接输出，无需折叠 -->
        <div v-if="entry.kind === 'thinking'" class="flat-thinking flat-thinking-direct" data-test="thinking">
          <div class="thinking-direct-header">
            <span class="thinking-icon" aria-hidden="true">🧠</span>
            <span class="thinking-title">Thinking</span>
            <span v-if="entry.streaming" class="streaming-cursor" data-test="streaming" aria-hidden="true">…</span>
          </div>
          <p class="thinking-text">{{ entry.text }}</p>
        </div>

        <!-- A subagent/task call is a named row plus a button that opens its detail -->
        <div
          v-else-if="isSubagentCall(entry)"
          class="flat-tool flat-subagent"
          data-test="tool-call"
        >
          <div class="flat-head" data-test="flat-head">
            <span class="tool-icon" aria-hidden="true">🤖</span>
            <span class="tool-name">子任务</span>
            <span v-if="subagentName(entry)" class="tool-arg">{{ subagentName(entry) }}</span>
            <span v-if="entry.streaming || !resultForCall(entry)" class="tool-running" role="status">运行中…</span>
          </div>
          <button
            type="button"
            class="subagent-open"
            data-test="subagent-open"
            @click="openSubagent(entry)"
          >
            查看子任务详情
          </button>
        </div>

        <!-- 所有其它工具只展示摘要行 -->
        <div
          v-else-if="entry.kind === 'tool_call' && toolEventsVisible()"
          class="flat-tool flat-summary"
          data-test="tool-call"
        >
          <div class="flat-head" data-test="flat-head">
            <span class="tool-icon" aria-hidden="true">{{ toolIcon(entry) }}</span>
            <span class="tool-name">{{ toolSummaryText(entry) }}</span>
            <span v-if="entry.streaming || !resultForCall(entry)" class="tool-running" role="status">运行中…</span>
            <span v-else-if="resultForCall(entry)?.isError" class="tool-status-error" role="status">失败</span>
          </div>
        </div>

        <!-- A result whose call is missing shows as a summary line as well -->
        <div
          v-else-if="entry.kind === 'tool_result' && toolEventsVisible()"
          class="flat-tool flat-summary"
          data-test="tool-result"
        >
          <div class="flat-head" data-test="flat-head">
            <span class="tool-icon" aria-hidden="true">🔧</span>
            <span class="tool-name">{{ entry.text || '工具结果' }}</span>
          </div>
        </div>
      </li>

      <!-- Running indicator row when session turn is running -->
      <li v-if="props.running" class="chat-entry kind-running" data-test="running-entry">
        <div class="running-indicator-card">
          <RunningIndicator label="正在运行" />
          <span class="running-text">正在运行...</span>
        </div>
      </li>
    </ol>

    <p v-if="flatActivityEntries.length === 0" class="empty">暂无活动。</p>

    <!-- Subagent detail dialog modal -->
    <div
      v-if="subagentModal"
      class="subagent-backdrop"
      role="dialog"
      aria-modal="true"
      aria-label="子 Agent 详情"
      data-test="subagent-dialog"
      @click.self="closeSubagent"
    >
      <div class="subagent-panel">
        <div class="subagent-head">
          <p class="subagent-title">子 Agent · {{ toolName(subagentModal) }}</p>
          <button type="button" class="subagent-close" data-test="subagent-close" @click="closeSubagent">关闭</button>
        </div>
        <p v-if="subagentSessionId(subagentModal)" class="subagent-session">会话 {{ subagentSessionId(subagentModal) }}</p>
        <p v-if="toolArgument(subagentModal)" class="subagent-arg">{{ toolArgument(subagentModal) }}</p>
        <pre v-if="prettyInput(subagentModal)" class="subagent-input">{{ prettyInput(subagentModal) }}</pre>
        <pre
          v-if="resultForCall(subagentModal)"
          class="subagent-output"
          :class="{ 'is-error': resultForCall(subagentModal)?.isError }"
        >{{ resultForCall(subagentModal)?.text }}</pre>
        <p v-else class="tool-running">运行中…</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.session-activity-log {
  width: 100%;
}

.entries {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
}

.flat-activity {
  gap: 0.5rem;
}

.chat-entry {
  display: flex;
  flex-direction: column;
  width: 100%;
}

.flat-thinking {
  box-sizing: border-box;
  width: 100%;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  padding: 0.45rem 0.6rem;
}

.flat-tool {
  box-sizing: border-box;
  width: 100%;
  border: none;
  border-radius: 0;
  background: transparent;
  padding: 0.25rem 0.2rem;
}

.flat-head {
  margin: 0;
  display: flex;
  align-items: flex-start;
  gap: 0.45rem;
  font-size: 0.84rem;
  color: var(--color-text-muted);
  min-width: 0;
  width: 100%;
  line-height: 1.45;
}

.flat-head .tool-icon {
  flex-shrink: 0;
  line-height: 1.45;
  user-select: none;
}

.flat-head .thinking-title {
  flex-shrink: 0;
}

.flat-head .tool-name {
  flex: 1;
  min-width: 0;
  white-space: pre-wrap;
  word-break: break-word;
  overflow-wrap: anywhere;
  line-height: 1.45;
}

.flat-subagent .flat-head .tool-name {
  flex-shrink: 0;
}

.flat-head .tool-arg {
  flex: 1;
  min-width: 0;
  font-family: var(--font-mono, monospace);
  font-size: 0.8rem;
  color: var(--color-text);
  white-space: pre-wrap;
  word-break: break-word;
  overflow-wrap: anywhere;
  line-height: 1.45;
}

.flat-head .tool-running {
  flex-shrink: 0;
  margin-left: auto;
  font-size: 0.75rem;
  line-height: 1.45;
  white-space: nowrap;
  align-self: flex-start;
  color: var(--color-accent, #2563eb);
}

.flat-head .tool-status-error {
  flex-shrink: 0;
  margin-left: auto;
  font-size: 0.75rem;
  line-height: 1.45;
  white-space: nowrap;
  align-self: flex-start;
  color: var(--color-danger, #dc2626);
}

.flat-thinking-direct {
  background: var(--color-surface);
  border-color: var(--color-border);
  border-left: 2px solid var(--color-border-strong, #94a3b8);
  padding: 0.5rem 0.75rem;
}

.thinking-direct-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.25rem;
  font-size: 0.8rem;
  font-weight: 500;
  color: var(--color-text-muted);
}

.thinking-direct-header .thinking-title {
  color: var(--color-text-muted);
}

.thinking-text {
  margin: 0;
  font-size: 0.82rem;
  line-height: 1.45;
  color: var(--color-text);
  white-space: pre-wrap;
  word-break: break-word;
}

.flat-subagent {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.subagent-open {
  align-self: flex-start;
  font-size: 0.75rem;
  padding: 0.2rem 0.5rem;
  border-radius: 4px;
  border: 1px solid var(--color-border);
  background: var(--color-surface-muted, rgba(0, 0, 0, 0.04));
  color: var(--color-text);
  cursor: pointer;
}

.subagent-open:hover {
  background: var(--color-border-strong, rgba(0, 0, 0, 0.08));
}

.running-indicator-card {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  font-size: 0.85rem;
  color: var(--color-text-muted);
}

.streaming-cursor {
  display: inline-block;
  margin-left: 0.15rem;
  animation: pulse 1s infinite;
  color: var(--color-accent, #2563eb);
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.2; }
}

.empty {
  margin: 1.5rem 0;
  text-align: center;
  font-size: 0.85rem;
  color: var(--color-text-muted);
}

/* Subagent popup modal */
.subagent-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  padding: 1rem;
}

.subagent-panel {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.15);
  max-width: 600px;
  width: 100%;
  max-height: 80vh;
  display: flex;
  flex-direction: column;
  padding: 1rem;
  gap: 0.75rem;
  overflow: hidden;
}

.subagent-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.subagent-title {
  margin: 0;
  font-weight: 600;
  font-size: 0.95rem;
}

.subagent-close {
  background: none;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  padding: 0.2rem 0.5rem;
  font-size: 0.8rem;
  cursor: pointer;
}

.subagent-session,
.subagent-arg {
  margin: 0;
  font-size: 0.8rem;
  color: var(--color-text-muted);
  font-family: var(--font-mono, monospace);
}

.subagent-input,
.subagent-output {
  margin: 0;
  font-size: 0.8rem;
  background: var(--color-bg, #f8fafc);
  padding: 0.5rem;
  border-radius: 4px;
  overflow: auto;
  max-height: 200px;
  white-space: pre-wrap;
  word-break: break-all;
}

.subagent-output.is-error {
  color: var(--color-danger, #dc2626);
}
</style>
