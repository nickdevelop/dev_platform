<script setup lang="ts">
// Renders one session's message log. A user turn is a right-aligned bubble,
// agent replies are full-width markdown text, and agent activity (reasoning +
// tool calls) is grouped into collapsible step cards like the opencode desktop
// client: consecutive thinking/tool entries between two messages form one step
// group whose summary names the tools involved. Each tool call and its result
// is still one collapsed block inside the group. Tool activity shows only when
// the session's capabilities declare tool-call events. User turns, reasoning
// and tool output stay literal text; agent replies render as sanitized markdown
// (see ../markdown), so backend content cannot inject live markup. An agent
// reply is cut around its ```plantuml fences first (see ../markdown-segments).
// Subagent tool calls offer a popup dialog with the full input/output.
//
// The log can render split across the chat panel's two columns: display="messages"
// shows only user/agent message and error entries for the left column, while
// display="activity" shows only the thinking/tool entries for the right activity
// pane. The activity pane renders a flat list — no step grouping — where each
// card folds on its own header: tool cards default collapsed, thinking cards
// default expanded. display="all" (the default) keeps the legacy interleaved log with
// collapsible step groups.
import { computed, reactive, ref } from 'vue'
import { renderMarkdown } from '../markdown'
import { splitDiagramBlocks } from '../markdown-segments'
import PlantumlBlock from './PlantumlBlock.vue'
import type { Capabilities, MessageModel } from '../api/sessions'
import type { ChatEntry, StreamStatus } from '../stores/sessions'

// modelLabel is the small tag under an agent reply: the model that produced
// it, with the named variant appended when one ran.
function modelLabel(model: MessageModel): string {
  return model.variant ? `${model.id}#${model.variant}` : model.id
}

const props = defineProps<{
  messages: ChatEntry[]
  capabilities?: Capabilities | null
  streamStatus?: StreamStatus
  // Which slice of the log to render. The chat panel renders the log split:
  // display="messages" shows only user/agent message and error entries for
  // the left column, display="activity" shows only the thinking/tool entries
  // for the right activity pane (a flat list; tools fold collapsed, thinking
  // folds expanded). display="all"
  // keeps the legacy interleaved log.
  display?: 'all' | 'messages' | 'activity'
}>()

const displayMode = computed(() => props.display ?? 'all')
// Manually opened blocks stay open while the turn streams. A native <details>
// loses its `open` state whenever Vue re-creates the element, and a step group
// is re-created whenever an activity entry is appended to it, so the view owns
// the state instead: a step group and each inner thinking/tool block remember
// whether the user opened them, keyed by the group id / entry id. Without this,
// expanding a running step collapses it again on the next tool call.
const openSteps = reactive(new Set<string>())
const openItems = reactive(new Set<string>())

function onStepToggle(id: string, event: Event) {
  if ((event.target as HTMLDetailsElement).open) openSteps.add(id)
  else openSteps.delete(id)
}

function onItemToggle(id: string, event: Event) {
  if ((event.target as HTMLDetailsElement).open) openItems.add(id)
  else openItems.delete(id)
}

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
// collapsed line reads like `read /path` or `bash ls`. Description wins over a
// full prompt (subagent/question cards read like the TUI), and unknown tools
// fall back to their first string argument.
const ARGUMENT_KEYS = [
  'path',
  'filePath',
  'file_path',
  'file',
  'command',
  'pattern',
  'query',
  'url',
  'description',
  'prompt',
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

// --- Step grouping (opencode desktop style) ---

type RenderBlock =
  | { kind: 'entry'; entry: ChatEntry }
  | { kind: 'step'; id: string; entries: ChatEntry[] }

function isActivity(entry: ChatEntry): boolean {
  return entry.kind === 'thinking' || entry.kind === 'tool_call' || entry.kind === 'tool_result'
}

// Consecutive thinking/tool entries between messages form one step group, so
// a long agent turn reads as one collapsible card instead of a flat list of
// blocks. Single entries stay ungrouped only when isolated by messages.
//
// The group id is its first entry's id, not a join of every member: entries are
// only ever appended to a running group, so the first member is stable while
// the group grows. A key that changed on every append would make Vue re-create
// the <details> and drop the state the user opened it in.
const renderBlocks = computed<RenderBlock[]>(() => {
  const blocks: RenderBlock[] = []
  let current: ChatEntry[] | null = null
  const flush = () => {
    if (current && current.length > 0) {
      blocks.push({ kind: 'step', id: `step:${current[0].id}`, entries: current })
      current = null
    }
  }
  for (const entry of visibleEntries.value) {
    if (isActivity(entry) && toolEventsVisibleFor(entry)) {
      if (!current) current = []
      current.push(entry)
    } else {
      flush()
      blocks.push({ kind: 'entry', entry })
    }
  }
  flush()
  return blocks
})

// The split log reuses the same grouping: display="messages" keeps only
// message/error entries for the left column, display="activity" keeps only
// the thinking/tool step groups for the right pane. display="all" keeps the
// legacy interleaved log.
const displayedBlocks = computed<RenderBlock[]>(() => {
  if (displayMode.value === 'messages') {
    return renderBlocks.value.filter(
      (block) =>
        block.kind === 'entry' && (block.entry.kind === 'message' || block.entry.kind === 'error'),
    )
  }
  if (displayMode.value === 'activity') {
    return renderBlocks.value.filter((block) => block.kind === 'step')
  }
  return renderBlocks.value
})

// The activity pane's flat list: the same visible thinking/tool entries in
// transcript order, with no step grouping and no collapsing. A tool result
// whose call is present is already merged into that call's card (see
// visibleEntries), so it is not listed twice.
const flatActivityEntries = computed<ChatEntry[]>(() =>
  visibleEntries.value.filter((entry) => isActivity(entry) && toolEventsVisibleFor(entry)),
)

// Per-card folding for the flat activity list: tool cards default collapsed,
// thinking cards default expanded. The set records only the user's toggles
// away from those defaults, keyed by entry id, so newly streamed entries
// always open in their default state.
const flatToggles = reactive(new Set<string>())
function flatExpanded(entry: ChatEntry): boolean {
  const toggled = flatToggles.has(entry.id)
  return entry.kind === 'thinking' ? !toggled : toggled
}
function toggleFlat(entry: ChatEntry) {
  if (flatToggles.has(entry.id)) flatToggles.delete(entry.id)
  else flatToggles.add(entry.id)
}

function toolEventsVisibleFor(entry: ChatEntry): boolean {
  if (entry.kind === 'message' || entry.kind === 'error') return true
  return toolEventsVisible()
}

function stepSummary(entries: ChatEntry[]): string {
  const tools = entries.filter((entry) => entry.kind === 'tool_call')
  const thinkings = entries.filter((entry) => entry.kind === 'thinking')
  const parts: string[] = []
  if (tools.length > 0) {
    const names = [...new Set(tools.map((entry) => toolName(entry)))].slice(0, 3)
    parts.push(`${tools.length} 个工具 · ${names.join('、')}`)
  }
  if (thinkings.length > 0) parts.push('思考')
  const running = entries.some((entry) => entry.kind === 'tool_call' && !resultForCall(entry))
  return parts.length > 0 ? parts.join(' · ') + (running ? ' · 运行中' : '') : 'Agent 活动'
}

// --- Subagent popup ---

const SUBAGENT_NAMES = new Set(['task', 'subagent', 'Task', 'Subagent', 'delegate', 'spawn'])

function isSubagentCall(entry: ChatEntry): boolean {
  if (entry.kind !== 'tool_call') return false
  const name = (entry.name ?? entry.text ?? '').toLowerCase()
  if (SUBAGENT_NAMES.has(entry.name ?? '') || SUBAGENT_NAMES.has(name)) return true
  const result = resultForCall(entry)
  const haystack = `${typeof entry.input === 'string' ? entry.input : JSON.stringify(entry.input ?? '')} ${result?.text ?? ''}`
  return haystack.includes('<subagent') || haystack.includes('sessionID=')
}

function subagentSessionId(entry: ChatEntry): string {
  const result = resultForCall(entry)
  const haystack = `${typeof entry.input === 'string' ? entry.input : JSON.stringify(entry.input ?? '')} ${result?.text ?? ''}`
  const match = haystack.match(/sessionID=["']([^"']+)["']/)
  return match?.[1] ?? ''
}

const subagentModal = ref<ChatEntry | null>(null)

function openSubagent(entry: ChatEntry) {
  subagentModal.value = entry
}

function closeSubagent() {
  subagentModal.value = null
}

function prettyInput(entry: ChatEntry): string {
  if (entry.input === null || entry.input === undefined) return ''
  if (typeof entry.input === 'string') return entry.input
  try {
    return JSON.stringify(entry.input, null, 2)
  } catch {
    return String(entry.input)
  }
}
</script>

<template>
  <div class="message-view">
    <p
      v-if="displayMode !== 'activity' && (props.streamStatus === 'reconnecting' || props.streamStatus === 'error')"
      class="stream-status"
      role="status"
      aria-live="polite"
    >
      <template v-if="props.streamStatus === 'reconnecting'">事件流已断开，如未自行恢复可重新连接。</template>
      <template v-else>事件流已断开，无法接收新消息。</template>
    </p>

    <ol
      v-if="displayMode === 'activity'"
      class="entries flat-activity"
      role="log"
      aria-live="polite"
      aria-label="思考与工具活动"
    >
      <li v-for="entry in flatActivityEntries" :key="entry.id" class="chat-entry" :class="`kind-${entry.kind}`">
        <div v-if="entry.kind === 'thinking'" class="flat-thinking" data-test="thinking">
          <button
            type="button"
            class="flat-toggle"
            data-test="flat-toggle"
            :aria-expanded="flatExpanded(entry).toString()"
            aria-label="折叠或展开思考内容"
            @click="toggleFlat(entry)"
          >
            <span class="flat-chevron" aria-hidden="true">{{ flatExpanded(entry) ? '▾' : '▸' }}</span>
            <span class="thinking-icon" aria-hidden="true">🧠</span>
            <span class="thinking-title">Thinking</span>
            <span v-if="entry.streaming" class="streaming-cursor" data-test="streaming" aria-hidden="true">…</span>
          </button>
          <p v-if="flatExpanded(entry)" class="thinking-text">{{ entry.text }}</p>
        </div>

        <div
          v-else-if="entry.kind === 'tool_call' && toolEventsVisible()"
          class="flat-tool"
          data-test="tool-call"
        >
          <button
            type="button"
            class="flat-toggle"
            data-test="flat-toggle"
            :aria-expanded="flatExpanded(entry).toString()"
            :aria-label="`折叠或展开工具 ${toolName(entry)}`"
            @click="toggleFlat(entry)"
          >
            <span class="flat-chevron" aria-hidden="true">{{ flatExpanded(entry) ? '▾' : '▸' }}</span>
            <span class="tool-icon" aria-hidden="true">🔧</span>
            <span class="tool-name">{{ toolName(entry) }}</span>
            <span v-if="toolArgument(entry)" class="tool-arg">{{ toolArgument(entry) }}</span>
            <span v-if="isSubagentCall(entry)" class="subagent-badge">子任务</span>
            <span v-if="entry.streaming || !resultForCall(entry)" class="tool-running" role="status">运行中…</span>
          </button>
          <div v-if="flatExpanded(entry)" class="tool-body">
            <pre v-if="toolArgument(entry)" class="tool-args">{{ toolArgument(entry) }}</pre>
            <pre
              v-if="resultForCall(entry)"
              class="tool-output"
              :class="{ 'is-error': resultForCall(entry)?.isError }"
            >{{ resultForCall(entry)?.text }}</pre>
            <button
              v-if="isSubagentCall(entry)"
              type="button"
              class="subagent-open"
              data-test="subagent-open"
              @click="openSubagent(entry)"
            >
              查看子 Agent
            </button>
          </div>
        </div>

        <div
          v-else-if="entry.kind === 'tool_result' && toolEventsVisible()"
          class="flat-tool"
          data-test="tool-result"
        >
          <button
            type="button"
            class="flat-toggle"
            data-test="flat-toggle"
            :aria-expanded="flatExpanded(entry).toString()"
            aria-label="折叠或展开工具结果"
            @click="toggleFlat(entry)"
          >
            <span class="flat-chevron" aria-hidden="true">{{ flatExpanded(entry) ? '▾' : '▸' }}</span>
            <span class="tool-icon" aria-hidden="true">🔧</span>
            <span class="tool-name">工具结果</span>
          </button>
          <div v-if="flatExpanded(entry)" class="tool-body">
            <pre class="tool-output" :class="{ 'is-error': entry.isError }">{{ entry.text }}</pre>
          </div>
        </div>
      </li>
    </ol>

    <ol
      v-else
      class="entries"
      role="log"
      aria-live="polite"
      aria-label="会话消息"
    >
      <li v-for="block in displayedBlocks" :key="block.kind === 'entry' ? block.entry.id : block.id" class="chat-entry" :class="block.kind === 'step' ? 'kind-step' : `kind-${block.entry.kind}`">
        <template v-if="block.kind === 'entry'">
          <template v-if="block.entry.kind === 'message'">
            <div class="bubble" :class="block.entry.role === 'user' ? 'bubble-user' : 'bubble-agent'" data-test="bubble">
              <span v-if="block.entry.role === 'user'" class="message-text">{{ block.entry.text }}</span>
              <div v-else class="message-text markdown-body">
                <template v-for="(segment, index) in splitDiagramBlocks(block.entry.text)" :key="`${block.entry.id}:${index}`">
                  <div
                    v-if="segment.kind === 'markdown'"
                    class="markdown-seg"
                    v-html="renderMarkdown(segment.text)"
                  ></div>
                  <PlantumlBlock
                    v-else
                    :source="segment.text"
                    :ready="segment.closed && settled(block.entry)"
                    :finished="settled(block.entry)"
                  />
                </template>
              </div>
              <span v-if="block.entry.streaming" class="streaming-cursor" data-test="streaming" aria-hidden="true">…</span>
              <span
                v-if="block.entry.role !== 'user' && block.entry.model"
                class="message-model"
                data-test="message-model"
              >{{ modelLabel(block.entry.model) }}</span>
            </div>
          </template>

          <template v-else-if="block.entry.kind === 'error'">
            <p class="agent-error" data-test="agent-error">{{ block.entry.text }}</p>
          </template>

          <template v-else>
            <!-- Activity entries always render inside step groups; this fallback
                 only covers backends without tool events. -->
          </template>
        </template>

        <template v-else>
          <details
            class="step-group"
            data-test="step-group"
            :open="openSteps.has(block.id)"
            @toggle="onStepToggle(block.id, $event)"
          >
            <summary class="step-summary">
              <span class="step-icon" aria-hidden="true">⚙️</span>
              <span class="step-title">{{ stepSummary(block.entries) }}</span>
              <span class="step-count">{{ block.entries.length }} 步</span>
            </summary>
            <div class="step-body">
              <template v-for="entry in block.entries" :key="entry.id">
                <details
                  v-if="entry.kind === 'thinking'"
                  class="thinking"
                  data-test="thinking"
                  :open="openItems.has(entry.id)"
                  @toggle="onItemToggle(entry.id, $event)"
                >
                  <summary>
                    <span class="thinking-icon" aria-hidden="true">🧠</span>
                    <span class="thinking-title">Thinking</span>
                  </summary>
                  <p class="thinking-text">{{ entry.text }}</p>
                </details>

                <details
                  v-else-if="entry.kind === 'tool_call' && toolEventsVisible()"
                  class="tool"
                  data-test="tool-call"
                  :open="openItems.has(entry.id)"
                  @toggle="onItemToggle(entry.id, $event)"
                >
                  <summary>
                    <span class="tool-icon" aria-hidden="true">🔧</span>
                    <span class="tool-name">{{ toolName(entry) }}</span>
                    <span v-if="toolArgument(entry)" class="tool-arg">{{ toolArgument(entry) }}</span>
                    <span v-if="isSubagentCall(entry)" class="subagent-badge">子任务</span>
                  </summary>
                  <div class="tool-body">
                    <pre v-if="toolArgument(entry)" class="tool-args">{{ toolArgument(entry) }}</pre>
                    <pre
                      v-if="resultForCall(entry)"
                      class="tool-output"
                      :class="{ 'is-error': resultForCall(entry)?.isError }"
                    >{{ resultForCall(entry)?.text }}</pre>
                    <p v-else class="tool-running" role="status">运行中…</p>
                    <button
                      v-if="isSubagentCall(entry)"
                      type="button"
                      class="subagent-open"
                      data-test="subagent-open"
                      @click="openSubagent(entry)"
                    >
                      查看子 Agent
                    </button>
                  </div>
                </details>

                <details
                  v-else-if="entry.kind === 'tool_result' && toolEventsVisible()"
                  class="tool"
                  data-test="tool-result"
                  :open="openItems.has(entry.id)"
                  @toggle="onItemToggle(entry.id, $event)"
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
            </div>
          </details>
        </template>
      </li>
    </ol>

    <p v-if="displayMode === 'activity' ? flatActivityEntries.length === 0 : props.messages.length === 0" class="empty">{{ displayMode === 'activity' ? '暂无思考与工具活动。' : '尚无消息。' }}</p>

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

/* The per-message model tag: small, muted, and set apart from the reply text
   it belongs to. */
.message-model {
  display: inline-block;
  margin-top: 0.25rem;
  font-size: 0.75rem;
  color: var(--color-text-muted);
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

.step-group {
  width: 100%;
  /* The box carries padding and a border on top of its width; without
     border-box that padding pushes the group past the log's content box and
     the conversation grows a horizontal scrollbar. */
  box-sizing: border-box;
  border: 1px solid var(--color-border);
  border-radius: 10px;
  background: var(--color-surface);
  padding: 0.4rem 0.6rem;
}

.step-summary {
  cursor: pointer;
  display: flex;
  align-items: baseline;
  gap: 0.45rem;
  font-size: 0.85rem;
  color: var(--color-text-muted);
  list-style: none;
}

.step-summary::-webkit-details-marker {
  display: none;
}

.step-icon {
  font-size: 0.9rem;
}

.step-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text);
  font-weight: 600;
}

.step-count {
  flex-shrink: 0;
  font-size: 0.75rem;
}

/* The activity pane's flat list: every thinking/tool entry renders as its own
   expanded card — no grouping, no <details> — so the right column reads
   top-to-bottom in transcript order. */
.flat-activity {
  gap: 0.5rem;
}

.flat-thinking,
.flat-tool {
  box-sizing: border-box;
  width: 100%;
  border: 1px solid var(--color-border);
  border-radius: 10px;
  background: var(--color-surface);
  padding: 0.45rem 0.6rem;
}

.flat-head {
  margin: 0;
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  font-size: 0.85rem;
  color: var(--color-text-muted);
  min-width: 0;
}

.flat-head .tool-name,
.flat-head .thinking-title {
  flex-shrink: 0;
}

.flat-head .tool-arg {
  flex: 1;
}

/* The flat card's toggle: the whole header is one button, with a chevron
   showing the fold state. Tools start collapsed, thinking starts expanded. */
.flat-toggle {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  width: 100%;
  margin: 0;
  padding: 0;
  border: none;
  background: transparent;
  cursor: pointer;
  font: inherit;
  font-size: 0.85rem;
  color: var(--color-text-muted);
  text-align: left;
  min-width: 0;
}

.flat-chevron {
  flex-shrink: 0;
  font-size: 0.75rem;
  width: 1em;
}

.flat-toggle .tool-name,
.flat-toggle .thinking-title {
  flex-shrink: 0;
}

.flat-toggle .tool-arg {
  flex: 1;
}

.flat-thinking .thinking-text,
.flat-tool .tool-body {
  margin-top: 0.4rem;
}

.step-body {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin-top: 0.4rem;
  padding-top: 0.4rem;
  border-top: 1px dashed var(--color-border);
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
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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

.subagent-badge {
  flex-shrink: 0;
  font-size: 0.72rem;
  color: var(--color-primary-hover);
  background: var(--color-info-soft);
  border-radius: 999px;
  padding: 0.05rem 0.45rem;
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

.subagent-open {
  margin-top: 0.4rem;
  padding: 0.25rem 0.6rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
  font-size: 0.8rem;
}

.subagent-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.35);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 30;
  padding: 1rem;
}

.subagent-panel {
  width: min(40rem, 92vw);
  max-height: 84vh;
  overflow: auto;
  background: var(--color-surface);
  border-radius: 12px;
  padding: 1rem 1.1rem;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.25);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.subagent-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.subagent-title {
  margin: 0;
  font-weight: 700;
}

.subagent-close {
  padding: 0.25rem 0.6rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
}

.subagent-session,
.subagent-arg {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.85rem;
  word-break: break-word;
}

.subagent-input,
.subagent-output {
  margin: 0;
  padding: 0.6rem 0.7rem;
  border-radius: 8px;
  background: var(--color-surface-muted);
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-family);
  font-size: 0.8rem;
  max-height: 40vh;
  overflow: auto;
}

.subagent-output.is-error {
  color: var(--color-danger);
}

.stream-status {
  color: var(--color-warning);
  margin: 0 0 0.5rem;
}

.empty {
  color: var(--color-text-muted);
}
</style>
