<script setup lang="ts">
// Dedicated Activity Panel view for session reasoning, tool executions, and subagent tasks.
// The pane is a detail-first stream: every block shows the full reasoning, tool
// input and tool output instead of a one-line summary. Blocks are separated by
// a horizontal rule and carry no surrounding card or border; long diffs and
// outputs keep their existing fold controls so the panel stays usable.

import { computed } from 'vue'
import RunningIndicator from '../../../shared/components/RunningIndicator.vue'
import type { Capabilities } from '../../../api/sessions'
import type { ChatEntry } from '../state/entry'
import EditDiffBody from './EditDiffBody.vue'
import ToolOutput from './ToolOutput.vue'
import * as log from './logModel'

const props = defineProps<{
  messages: ChatEntry[]
  capabilities?: Capabilities | null
  running?: boolean
}>()

const model = computed(() => log.buildLogModel(props.messages))

const resultForCall = (entry: ChatEntry): ChatEntry | undefined => model.value.resultForCall(entry)
// A call with no result reads "running" only while its turn is alive; after the
// turn ended nothing can answer it, so it reads as unfinished instead.
const pendingToolLabel = () => log.pendingToolLabel(props.running === true)

function toolEventsVisible(): boolean {
  return log.toolEventsVisible(props.capabilities)
}

function toolEventsVisibleFor(entry: ChatEntry): boolean {
  if (entry.kind !== 'tool_call' && entry.kind !== 'tool_result') return true
  return toolEventsVisible()
}

// Derive from the render model, not the raw props: buildLogModel already
// removes a result whose call is on screen, because that call's card prints it.
const visibleEntries = computed<ChatEntry[]>(() =>
  model.value.visibleEntries.filter((entry) => {
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

const runningAction = computed(() => log.deriveRunningAction(props.messages))

function toolKind(entry: ChatEntry): log.ToolKind {
  return log.toolKind(entry)
}

function toolName(entry: ChatEntry): string {
  return log.toolName(entry)
}

function prettyInput(entry: ChatEntry): string {
  return log.prettyInput(entry)
}

const isSubagentCall = (entry: ChatEntry) => log.isSubagentCall(entry, resultForCall)
const subagentName = (entry: ChatEntry) => log.subagentName(entry)
const subagentSessionId = (entry: ChatEntry) => log.subagentSessionId(entry, resultForCall)
const isEditCall = log.isEditCall
const editPathFor = (entry: ChatEntry) => log.editPathFor(entry)
const editDiffFor = (entry: ChatEntry) => log.editDiffFor(entry, resultForCall)
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
            <span class="thinking-icon" aria-hidden="true">
              <svg class="glyph-svg" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M8 2a4 4 0 0 0-4 4c0 1.5.8 2.8 2 3.5V11h4V9.5c1.2-.7 2-2 2-3.5a4 4 0 0 0-4-4z" />
                <line x1="6" y1="13" x2="10" y2="13" />
                <line x1="7" y1="15" x2="9" y2="15" />
              </svg>
            </span>
            <span class="thinking-title">Thinking</span>
            <span v-if="entry.streaming" class="streaming-cursor" data-test="streaming" aria-hidden="true">…</span>
          </div>
          <p class="thinking-text">{{ entry.text }}</p>
        </div>

        <!-- A subagent/task call shows its full prompt and result inline. -->
        <div
          v-else-if="isSubagentCall(entry)"
          class="flat-tool flat-subagent"
          data-test="tool-call"
        >
          <div class="flat-head" data-test="flat-head">
            <span class="tool-icon" aria-hidden="true">
              <svg class="glyph-svg" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6">
                <rect x="3" y="4" width="10" height="8" rx="2" />
                <line x1="8" y1="2" x2="8" y2="4" />
                <circle cx="6" cy="8" r="0.75" fill="currentColor" />
                <circle cx="10" cy="8" r="0.75" fill="currentColor" />
                <line x1="6" y1="10" x2="10" y2="10" />
              </svg>
            </span>
            <span class="tool-name">子任务</span>
            <span v-if="subagentName(entry)" class="tool-arg">{{ subagentName(entry) }}</span>
            <span v-if="entry.streaming || !resultForCall(entry)" class="tool-running" role="status">{{ pendingToolLabel() }}</span>
            <span v-else-if="resultForCall(entry)?.isError" class="tool-status-error" role="status">失败</span>
          </div>
          <p v-if="subagentSessionId(entry)" class="subagent-session">会话 {{ subagentSessionId(entry) }}</p>
          <pre v-if="prettyInput(entry)" class="tool-args" data-test="tool-input">{{ prettyInput(entry) }}</pre>
          <ToolOutput
            v-if="resultForCall(entry)"
            :text="resultForCall(entry)?.text ?? ''"
            :is-error="resultForCall(entry)?.isError"
          />
        </div>

        <!-- An edit call with a diff shows the patch directly. -->
        <div
          v-else-if="entry.kind === 'tool_call' && isEditCall(entry) && editDiffFor(entry)?.patch"
          class="flat-tool flat-edit"
          data-test="tool-call"
        >
          <div class="flat-head" data-test="flat-head">
            <span class="tool-icon" aria-hidden="true">
              <svg class="glyph-svg" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M11 2.5l2.5 2.5L5 13.5H2.5V11L11 2.5z" />
              </svg>
            </span>
            <span class="tool-name">edit</span>
            <span v-if="editPathFor(entry)" class="tool-arg">{{ editPathFor(entry) }}</span>
            <span v-if="entry.streaming || !resultForCall(entry)" class="tool-running" role="status">{{ pendingToolLabel() }}</span>
            <span v-else-if="resultForCall(entry)?.isError" class="tool-status-error" role="status">失败</span>
          </div>
          <EditDiffBody :patch="editDiffFor(entry)?.patch" />
        </div>

        <!-- Every other tool call shows its name, full input and output. -->
        <div
          v-else-if="entry.kind === 'tool_call' && toolEventsVisible()"
          class="flat-tool flat-detail"
          data-test="tool-call"
        >
          <div class="flat-head" data-test="flat-head">
            <span class="tool-icon" aria-hidden="true">
              <template v-if="toolKind(entry) === 'question'">
                <svg class="glyph-svg" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                  <circle cx="8" cy="8" r="6.5" />
                  <path d="M6 6a2 2 0 1 1 3 1.73c-.5.3-.8.7-.8 1.27v.5" />
                  <circle cx="8" cy="11.5" r="0.5" fill="currentColor" />
                </svg>
              </template>
              <template v-else-if="toolKind(entry) === 'read'">
                <svg class="glyph-svg" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
                  <path d="M3 2h7l3 3v9H3V2z" />
                  <polyline points="10 2 10 5 13 5" />
                </svg>
              </template>
              <template v-else-if="toolKind(entry) === 'edit'">
                <svg class="glyph-svg" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M11 2.5l2.5 2.5L5 13.5H2.5V11L11 2.5z" />
                </svg>
              </template>
              <template v-else-if="toolKind(entry) === 'bash'">
                <svg class="glyph-svg" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                  <polyline points="4 5 8 8 4 11" />
                  <line x1="9" y1="11" x2="13" y2="11" />
                </svg>
              </template>
              <template v-else-if="toolKind(entry) === 'search'">
                <svg class="glyph-svg" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                  <circle cx="7" cy="7" r="4.5" />
                  <line x1="10.5" y1="10.5" x2="14" y2="14" />
                </svg>
              </template>
              <template v-else-if="toolKind(entry) === 'fetch'">
                <svg class="glyph-svg" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5">
                  <circle cx="8" cy="8" r="6.5" />
                  <ellipse cx="8" cy="8" rx="3" ry="6.5" />
                  <line x1="1.5" y1="8" x2="14.5" y2="8" />
                </svg>
              </template>
              <template v-else>
                <svg class="glyph-svg" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6">
                  <path d="M14 10l-4-4M10 2a3.5 3.5 0 0 0-4.8 4.8L2 10v4h4l3.2-3.2A3.5 3.5 0 0 0 14 6V2h-4z" />
                </svg>
              </template>
            </span>
            <span class="tool-name">{{ toolName(entry) }}</span>
            <span v-if="entry.streaming || !resultForCall(entry)" class="tool-running" role="status">{{ pendingToolLabel() }}</span>
            <span v-else-if="resultForCall(entry)?.isError" class="tool-status-error" role="status">失败</span>
          </div>
          <pre v-if="prettyInput(entry)" class="tool-args" data-test="tool-input">{{ prettyInput(entry) }}</pre>
          <ToolOutput
            v-if="resultForCall(entry)"
            :text="resultForCall(entry)?.text ?? ''"
            :is-error="resultForCall(entry)?.isError"
          />
        </div>

        <!-- A result whose call is missing still shows its full output. -->
        <div
          v-else-if="entry.kind === 'tool_result' && toolEventsVisible()"
          class="flat-tool flat-detail"
          data-test="tool-result"
        >
          <div class="flat-head" data-test="flat-head">
            <span class="tool-icon" aria-hidden="true">
              <svg class="glyph-svg" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6">
                <path d="M14 10l-4-4M10 2a3.5 3.5 0 0 0-4.8 4.8L2 10v4h4l3.2-3.2A3.5 3.5 0 0 0 14 6V2h-4z" />
              </svg>
            </span>
            <span class="tool-name">工具结果</span>
          </div>
          <ToolOutput :text="entry.text" :is-error="entry.isError" />
        </div>
      </li>

      <!-- Running indicator row when session turn is running -->
      <li v-if="props.running" class="chat-entry kind-running" data-test="running-entry">
        <div class="running-indicator-card" :class="`kind-${runningAction.kind}`">
          <RunningIndicator :label="runningAction.label" />
          <span class="running-text" data-test="running-label">{{ runningAction.label }}</span>
          <code
            v-if="runningAction.code"
            class="running-code"
            data-test="running-code"
            :title="runningAction.fullCode || runningAction.code"
          >{{ runningAction.code }}</code>
        </div>
      </li>
    </ol>

    <p v-if="flatActivityEntries.length === 0" class="empty">暂无活动。</p>
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
  gap: 0;
}

/* Each activity block is separated by a rule instead of being wrapped in a
   card, so the pane reads as one continuous transcript. */
.flat-activity > .chat-entry {
  box-sizing: border-box;
  width: 100%;
  padding: 0.65rem 0;
}

.flat-activity > .chat-entry:first-child {
  padding-top: 0;
}

.flat-activity > .chat-entry + .chat-entry {
  border-top: 1px solid var(--color-border);
}

.flat-thinking,
.flat-tool {
  box-sizing: border-box;
  width: 100%;
  border: none;
  border-radius: 0;
  background: transparent;
  padding: 0;
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

.flat-thinking-direct .thinking-text {
  margin-top: 0.35rem;
}

.thinking-direct-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
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
  line-height: 1.55;
  color: var(--color-text);
  white-space: pre-wrap;
  word-break: break-word;
  overflow-wrap: anywhere;
}

/* Tool input is literal data: wrap anywhere so a long token cannot widen the
   pane, and keep it visually subordinate to the tool name. */
.tool-args {
  margin: 0.35rem 0 0;
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace);
  font-size: 0.78rem;
  line-height: 1.5;
  color: var(--color-text);
  white-space: pre-wrap;
  word-break: break-word;
  overflow-wrap: anywhere;
}

.tool-args + :deep(.tool-output-wrap),
.flat-detail :deep(.tool-output-wrap),
.flat-subagent :deep(.tool-output-wrap),
.flat-edit :deep(.edit-diff-wrap) {
  margin-top: 0.4rem;
}

.subagent-session {
  margin: 0.35rem 0 0;
  font-size: 0.75rem;
  font-family: var(--font-mono, monospace);
  color: var(--color-text-muted);
}

.tool-running {
  margin: 0.35rem 0 0;
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.running-indicator-card {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: 0;
  font-size: 0.85rem;
  color: var(--color-text-muted);
}

.running-code {
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace);
  font-size: 0.75rem;
  background: transparent;
  border: none;
  padding: 0;
  max-width: 360px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text);
  line-height: 1.4;
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
</style>
