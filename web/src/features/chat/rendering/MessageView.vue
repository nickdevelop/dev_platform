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

// The log can render split across the chat panel's two columns: display="messages"
// shows only user/agent message and error entries for the left column, while
// display="activity" shows only the thinking/tool entries for the right activity
// pane. The activity pane renders a flat list — no step grouping — where
// thinking, edit, shell and question cards fold (and start expanded) while every
// other tool call is a single summary line, and a subagent call is a named row
// with a button that opens its detail. A card body that carries a tool's output
// (a shell log, an edit diff) shows its first 15 rows with a centered ▼ that
// expands the rest, so a long output never dominates the pane. display="all"
// (the default) keeps the legacy interleaved log.
import { computed, reactive, ref } from 'vue'
import { copyToClipboard } from '../../../shared/clipboard'
import { formatMessageTime } from '../../../shared/format'
import { renderMarkdown } from '../../../shared/markdown'
import { splitDiagramBlocks } from '../../../shared/markdown-segments'
import PlantumlBlock from './PlantumlBlock.vue'
import EditToolBlock from './EditToolBlock.vue'
import ToolOutput from './ToolOutput.vue'
import ImagePreviewModal from './ImagePreviewModal.vue'
import RunningIndicator from '../../../shared/components/RunningIndicator.vue'
import type { Capabilities } from '../../../api/sessions'
import type { ImageAttachment } from '../../../api/messageTypes'
import type { ChatEntry } from '../state/entry'
import type { StreamStatus } from '../../../stores/sessions'
import * as log from './logModel'

const props = defineProps<{
  messages: ChatEntry[]
  capabilities?: Capabilities | null
  streamStatus?: StreamStatus
  display?: 'all' | 'messages' | 'activity' | 'chat'
  running?: boolean
  // Pure-chat message actions. Agent session panels leave these off.
  canEditUserMessages?: boolean
  canRegenerateEntryId?: string | null
  logLabel?: string
}>()

const emit = defineEmits<{
  edit: [entry: ChatEntry]
  regenerate: [entry: ChatEntry]
}>()

// The whole render model — call/result joins, step groups, subagent facts and
// question payloads are derived by ./logModel; this component only owns the
// display mode, the manually-opened state and the flat-activity fold state.
const model = computed(() => log.buildLogModel(props.messages))
const resultForCall = (entry: ChatEntry) => model.value.resultForCall(entry)

// Template-facing helpers: same names the template already uses, each backed
// by the pure model. The lookup-carrying ones (subagent facts, step summary,
// edit diffs) get the model's result lookup.
const toolName = (entry: ChatEntry) => log.toolName(entry)
const toolArgument = (entry: ChatEntry) => log.toolArgument(entry)
const prettyInput = (entry: ChatEntry) => log.prettyInput(entry)
function toolEventsVisible(): boolean {
  return log.toolEventsVisible(props.capabilities)
}
const modelLabel = (m: NonNullable<ChatEntry['model']>) => log.modelLabel(m)
const stepSummary = (entries: ChatEntry[]) => log.stepSummary(entries, resultForCall)
const isSubagentCall = (entry: ChatEntry) => log.isSubagentCall(entry, resultForCall)
const subagentSessionId = (entry: ChatEntry) => log.subagentSessionId(entry, resultForCall)
const subagentName = (entry: ChatEntry) => log.subagentName(entry)
const editPathFor = (entry: ChatEntry) => log.editPathFor(entry)
const editDiffFor = (entry: ChatEntry) => log.editDiffFor(entry, resultForCall)
const toolIcon = (entry: ChatEntry) => log.toolIcon(entry)
const isEditCall = log.isEditCall
const isShellCall = log.isShellCall
const isQuestionCall = log.isQuestionCall
const questionItems = log.questionItems

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
// backend that does not report tool events must not show tool activity. A
// message or error entry always renders.
function toolEventsVisibleFor(entry: ChatEntry): boolean {
  if (entry.kind === 'message' || entry.kind === 'error') return true
  return toolEventsVisible()
}

const flatExpandable = log.flatExpandable

function settled(entry: ChatEntry): boolean {
  return log.settled(entry, props.streamStatus)
}

const visibleEntries = computed(() => model.value.visibleEntries)
const renderBlocks = computed<log.RenderBlock[]>(() => log.groupEntryBlocks(visibleEntries.value))
const displayedBlocks = computed<log.RenderBlock[]>(() =>
  log.blocksForDisplay(renderBlocks.value, displayMode.value),
)

const previewImage = ref<ImageAttachment | null>(null)
const showPreviewModal = ref(false)

function openImagePreview(img: ImageAttachment) {
  previewImage.value = img
  showPreviewModal.value = true
}

function closeImagePreview() {
  showPreviewModal.value = false
  previewImage.value = null
}
// The activity pane's flat list: the same visible thinking/tool entries in
// transcript order, with no step grouping and no collapsing. A tool result
// whose call is present is already merged into that call's card (see
// buildLogModel), so it is not listed twice.
const flatActivityEntries = computed<ChatEntry[]>(() =>
  visibleEntries.value.filter((entry) => log.isActivity(entry) && toolEventsVisibleFor(entry)),
)

// Per-card folding for the flat activity list. Only three kinds fold — thinking,
// edit and shell — and they all start expanded, because the user wants to read
// the reasoning, the diff and the command as they happen. The set records only a
// toggle away from that default, keyed by entry id, so a newly streamed entry
// always opens expanded. Every other tool call is a one-line summary with no
// fold (see log.flatExpandable).
const flatToggles = reactive(new Set<string>())
function flatExpanded(entry: ChatEntry): boolean {
  const toggled = flatToggles.has(entry.id)
  return log.flatExpandable(entry) ? !toggled : false
}
function toggleFlat(entry: ChatEntry) {
  if (flatToggles.has(entry.id)) flatToggles.delete(entry.id)
  else flatToggles.add(entry.id)
}

// The subagent popup keeps the whole entry so the dialog can show the call and
// its result side by side.
const subagentModal = ref<ChatEntry | null>(null)

function openSubagent(entry: ChatEntry) {
  subagentModal.value = entry
}

function closeSubagent() {
  subagentModal.value = null
}

const copiedEntryId = ref<string | null>(null)
let copyTimer: ReturnType<typeof setTimeout> | null = null

async function editMessage(entry: ChatEntry) {
  emit('edit', entry)
}

function regenerateMessage(entry: ChatEntry) {
  emit('regenerate', entry)
}

async function copyMessage(entry: ChatEntry) {
  if (!entry.text) return
  const ok = await copyToClipboard(entry.text)
  if (!ok) return
  copiedEntryId.value = entry.id
  if (copyTimer) clearTimeout(copyTimer)
  copyTimer = setTimeout(() => {
    if (copiedEntryId.value === entry.id) {
      copiedEntryId.value = null
    }
  }, 1500)
}
</script>

<template>
  <div class="message-view">
    <p
      v-if="displayMode !== 'activity' && displayMode !== 'chat' && (props.streamStatus === 'reconnecting' || props.streamStatus === 'error')"
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
      aria-label="活动"
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

        <!-- A subagent/task call is a named row plus a button that opens its
             full detail; the subagent's body is never rendered inline. -->
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

        <!-- Edit and shell calls fold, and start expanded: their diff/output is
             the point of watching the turn. -->
        <div
          v-else-if="entry.kind === 'tool_call' && flatExpandable(entry) && toolEventsVisible()"
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
            <span class="tool-icon" aria-hidden="true">{{ toolIcon(entry) }}</span>
            <span class="tool-name">{{ toolName(entry) }}</span>
            <span v-if="toolArgument(entry)" class="tool-arg">{{ toolArgument(entry) }}</span>
            <span v-if="entry.streaming || !resultForCall(entry)" class="tool-running" role="status">运行中…</span>
          </button>
          <div v-if="flatExpanded(entry)" class="tool-body">
            <template v-if="isQuestionCall(entry)">
              <div
                v-for="(item, index) in questionItems(entry)"
                :key="index"
                class="question-item"
                data-test="question-item"
              >
                <p v-if="item.header" class="question-header">{{ item.header }}</p>
                <p v-if="item.question" class="question-text" data-test="question-text">{{ item.question }}</p>
                <ul v-if="item.options.length > 0" class="question-options">
                  <li
                    v-for="(option, optionIndex) in item.options"
                    :key="optionIndex"
                    class="question-option"
                  >
                    <span class="question-option-label">{{ option.label }}</span>
                    <span v-if="option.description" class="question-option-description">{{ option.description }}</span>
                  </li>
                </ul>
                <span v-if="item.multiple" class="question-multiple">可多选</span>
              </div>
              <pre
                v-if="questionItems(entry).length === 0 && toolArgument(entry)"
                class="tool-args"
              >{{ toolArgument(entry) }}</pre>
              <ToolOutput
                v-if="resultForCall(entry)"
                :text="resultForCall(entry)?.text ?? ''"
                :is-error="resultForCall(entry)?.isError"
              />
            </template>
            <EditToolBlock
              v-else-if="isEditCall(entry) && editDiffFor(entry)"
              :file-path="editPathFor(entry)"
              :additions="editDiffFor(entry)?.additions"
              :deletions="editDiffFor(entry)?.deletions"
              :patch="editDiffFor(entry)?.patch"
              :output="resultForCall(entry)?.text"
              :is-error="resultForCall(entry)?.isError"
            />
            <template v-else>
              <pre v-if="toolArgument(entry)" class="tool-args">{{ toolArgument(entry) }}</pre>
              <ToolOutput
                v-if="resultForCall(entry)"
                :text="resultForCall(entry)?.text ?? ''"
                :is-error="resultForCall(entry)?.isError"
              />
            </template>
          </div>
        </div>

        <!-- Every other tool call is a single summary line: icon, name and the
             argument that names what it acted on. No fold, no body. -->
        <div
          v-else-if="entry.kind === 'tool_call' && toolEventsVisible()"
          class="flat-tool flat-summary"
          data-test="tool-call"
        >
          <div class="flat-head" data-test="flat-head">
            <span class="tool-icon" aria-hidden="true">{{ toolIcon(entry) }}</span>
            <span class="tool-name">{{ toolName(entry) }}</span>
            <span v-if="toolArgument(entry)" class="tool-arg">{{ toolArgument(entry) }}</span>
            <span v-if="entry.streaming || !resultForCall(entry)" class="tool-running" role="status">运行中…</span>
          </div>
        </div>

        <!-- A result whose call is missing (a truncated history) shows as a
             summary line as well, never as an expandable body. -->
        <div
          v-else-if="entry.kind === 'tool_result' && toolEventsVisible()"
          class="flat-tool flat-summary"
          data-test="tool-result"
        >
          <div class="flat-head" data-test="flat-head">
            <span class="tool-icon" aria-hidden="true">🔧</span>
            <span class="tool-name">工具结果</span>
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

    <ol
      v-else
      class="entries"
      role="log"
      aria-live="polite"
      :aria-label="props.logLabel ?? '会话消息'"
    >
      <li v-for="block in displayedBlocks" :key="block.kind === 'entry' ? block.entry.id : block.id" class="chat-entry" :class="block.kind === 'step' ? 'kind-step' : `kind-${block.entry.kind}`">
        <template v-if="block.kind === 'entry'">
          <template v-if="block.entry.kind === 'message'">
            <div class="bubble" :class="block.entry.role === 'user' ? 'bubble-user' : 'bubble-agent'" data-test="bubble">
              <template v-if="block.entry.role === 'user'">
                <div v-if="block.entry.images && block.entry.images.length > 0" class="message-images-grid" data-test="message-images">
                  <div
                    v-for="img in block.entry.images"
                    :key="img.id"
                    class="message-image-item"
                    :title="img.name"
                    @click="openImagePreview(img)"
                  >
                    <img :src="img.data.startsWith('data:') ? img.data : `data:${img.mediaType};base64,${img.data}`" :alt="img.name" />
                  </div>
                </div>
                <span v-if="block.entry.text" class="message-text">{{ block.entry.text }}</span>
              </template>
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
              <div
                v-if="block.entry.role !== 'user'"
                class="message-meta"
                data-test="message-meta"
              >
                <span
                  v-if="block.entry.model"
                  class="message-model"
                  data-test="message-model"
                >{{ modelLabel(block.entry.model) }}</span>
                <span
                  v-if="block.entry.timestamp"
                  class="message-time"
                  data-test="message-time"
                >{{ formatMessageTime(block.entry.timestamp) }}</span>
                <button
                  v-if="props.canRegenerateEntryId && block.entry.id === props.canRegenerateEntryId && !block.entry.streaming"
                  type="button"
                  class="message-action-btn"
                  data-test="message-regenerate-btn"
                  title="重新生成这条回复"
                  @click="regenerateMessage(block.entry)"
                >
                  重新生成
                </button>
                <button
                  type="button"
                  class="message-copy-btn"
                  :class="{ 'is-copied': copiedEntryId === block.entry.id }"
                  data-test="message-copy-btn"
                  :title="copiedEntryId === block.entry.id ? '已复制' : '复制本条消息'"
                  :aria-label="copiedEntryId === block.entry.id ? '已复制' : '复制本条消息'"
                  @click="copyMessage(block.entry)"
                >
                  <svg
                    v-if="copiedEntryId === block.entry.id"
                    class="copy-icon"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    aria-hidden="true"
                  >
                    <polyline points="20 6 9 17 4 12"></polyline>
                  </svg>
                  <svg
                    v-else
                    class="copy-icon"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    aria-hidden="true"
                  >
                    <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                  </svg>
                </button>
              </div>
            </div>
            <div
              v-if="block.entry.role === 'user' && (block.entry.timestamp || block.entry.text)"
              class="user-message-meta"
              data-test="user-message-meta"
            >
              <span
                v-if="block.entry.timestamp"
                class="user-message-time"
                data-test="user-message-time"
              >{{ formatMessageTime(block.entry.timestamp) }}</span>
              <button
                v-if="props.canEditUserMessages && !block.entry.streaming"
                type="button"
                class="message-action-btn user-edit-btn"
                data-test="message-edit-btn"
                title="编辑这条消息并重新发送"
                @click="editMessage(block.entry)"
              >
                编辑
              </button>
              <button
                v-if="block.entry.text"
                type="button"
                class="message-copy-btn user-copy-btn"
                :class="{ 'is-copied': copiedEntryId === block.entry.id }"
                data-test="message-copy-btn"
                :title="copiedEntryId === block.entry.id ? '已复制' : '复制本条消息'"
                :aria-label="copiedEntryId === block.entry.id ? '已复制' : '复制本条消息'"
                @click="copyMessage(block.entry)"
              >
                <svg
                  v-if="copiedEntryId === block.entry.id"
                  class="copy-icon"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  aria-hidden="true"
                >
                  <polyline points="20 6 9 17 4 12"></polyline>
                </svg>
                <svg
                  v-else
                  class="copy-icon"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  aria-hidden="true"
                >
                  <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                  <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                </svg>
              </button>
            </div>
          </template>

          <template v-else-if="block.entry.kind === 'error'">
            <p class="agent-error" data-test="agent-error">{{ block.entry.text }}</p>
          </template>

          <template v-else-if="block.entry.kind === 'notice'">
            <div class="system-notice-row" data-test="system-notice">
              <span class="notice-line"></span>
              <span class="notice-pill">
                <span class="notice-icon" aria-hidden="true">🔀</span>
                <span class="notice-text">{{ block.entry.text }}</span>
              </span>
              <span class="notice-line"></span>
            </div>
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
              <span class="step-icon" aria-hidden="true">{{ displayMode === 'chat' ? '🧠' : '⚙️' }}</span>
              <span class="step-title">{{ displayMode === 'chat' ? '思考过程' : stepSummary(block.entries) }}</span>
              <span v-if="displayMode !== 'chat'" class="step-count">{{ block.entries.length }} 步</span>
            </summary>
            <div class="step-body">
              <template v-if="displayMode === 'chat'">
                <div
                  v-for="entry in block.entries.filter((item) => item.kind === 'thinking')"
                  :key="entry.id"
                  class="chat-thinking"
                  data-test="thinking"
                >
                  <p class="thinking-text">{{ entry.text }}</p>
                  <span v-if="entry.streaming" class="streaming-cursor" data-test="streaming" aria-hidden="true">…</span>
                </div>
              </template>
              <template v-else v-for="entry in block.entries" :key="entry.id">
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
                    <EditToolBlock
                      v-if="isEditCall(entry) && editDiffFor(entry)"
                      :file-path="editPathFor(entry)"
                      :additions="editDiffFor(entry)?.additions"
                      :deletions="editDiffFor(entry)?.deletions"
                      :patch="editDiffFor(entry)?.patch"
                      :output="resultForCall(entry)?.text"
                      :is-error="resultForCall(entry)?.isError"
                    />
                    <template v-else>
                      <pre v-if="toolArgument(entry)" class="tool-args">{{ toolArgument(entry) }}</pre>
                      <ToolOutput
                        v-if="resultForCall(entry)"
                        :text="resultForCall(entry)?.text ?? ''"
                        :is-error="resultForCall(entry)?.isError"
                      />
                      <p v-else class="tool-running" role="status">运行中…</p>
                    </template>
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
                    <ToolOutput :text="entry.text" :is-error="entry.isError" />
                  </div>
                </details>
              </template>
            </div>
          </details>
        </template>
      </li>

      <!-- Running indicator row when session turn is running -->
      <li v-if="props.running" class="chat-entry kind-running" data-test="running-entry">
        <div class="running-indicator-card">
          <RunningIndicator label="正在运行" />
          <span class="running-text">正在运行...</span>
        </div>
      </li>
    </ol>

    <p v-if="displayMode === 'activity' ? flatActivityEntries.length === 0 : props.messages.length === 0" class="empty">{{ displayMode === 'activity' ? '暂无活动。' : '尚无消息。' }}</p>

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

    <ImagePreviewModal
      :image="previewImage"
      :visible="showPreviewModal"
      @close="closeImagePreview"
    />
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

.message-images-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin-bottom: 0.35rem;
}

.message-image-item {
  width: 56px;
  height: 56px;
  border-radius: 6px;
  overflow: hidden;
  cursor: pointer;
  border: 1px solid rgba(255, 255, 255, 0.2);
  background: rgba(0, 0, 0, 0.2);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: transform 0.15s ease, border-color 0.15s ease;
}

.message-image-item:hover {
  transform: scale(1.04);
  border-color: rgba(255, 255, 255, 0.6);
}

.message-image-item img {
  width: 100%;
  height: 100%;
  object-fit: cover;
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
.message-meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.25rem;
}

.message-model {
  display: inline-block;
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.message-time {
  display: inline-block;
  font-size: 0.72rem;
  color: var(--color-text-muted);
}

.message-copy-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0.15rem;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  line-height: 1;
  transition: color 0.15s ease, background-color 0.15s ease, border-color 0.15s ease;
}

.message-copy-btn:hover {
  color: var(--color-text);
  background: var(--color-surface-muted);
  border-color: var(--color-border);
}

.message-copy-btn.is-copied {
  color: var(--color-success);
}

.message-action-btn {
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  padding: 0.1rem 0.3rem;
  cursor: pointer;
  line-height: 1.2;
}

.message-action-btn:hover {
  color: var(--color-text);
  background: var(--color-surface-muted);
  border-color: var(--color-border);
}

.message-copy-btn .copy-icon {
  width: 0.85rem;
  height: 0.85rem;
  flex-shrink: 0;
}

.user-message-meta {
  align-self: flex-end;
  display: flex;
  align-items: center;
  gap: 0.35rem;
  margin-top: 0.2rem;
  margin-right: 0.25rem;
  line-height: 1;
}

.user-message-time {
  font-size: 0.72rem;
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
  flex-direction: column;
}

.kind-message {
  display: flex;
  flex-direction: column;
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
   showing the fold state. Thinking, edit, shell and question start expanded. */
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

/* A question tool's body: one block per question, the options listed under it. */
.question-item {
  margin: 0 0 0.5rem;
}

.question-item:last-child {
  margin-bottom: 0;
}

.question-header {
  margin: 0;
  font-size: 0.72rem;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.question-text {
  margin: 0.15rem 0 0.3rem;
  font-weight: 600;
  word-break: break-word;
}

.question-options {
  margin: 0;
  padding-left: 1.1rem;
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.question-option-label {
  font-weight: 600;
}

.question-option-description {
  margin-left: 0.35rem;
  color: var(--color-text-muted);
  font-size: 0.85rem;
  overflow-wrap: anywhere;
}

.question-multiple {
  font-size: 0.72rem;
  color: var(--color-text-muted);
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

.chat-thinking {
  color: var(--color-text-muted);
  font-size: 0.85rem;
  line-height: 1.5;
  margin-bottom: 0.35rem;
}

.chat-thinking .thinking-text {
  margin: 0;
  white-space: pre-wrap;
}

.thinking-text {
  margin-bottom: 0;
  white-space: pre-wrap;
  word-break: break-word;
  overflow-wrap: anywhere;
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

/* Tool args and running note are literal text: they wrap anywhere so a long
   unbroken token (a path, a hash) cannot widen the card and grow a horizontal
   scrollbar in the narrow activity pane. */
.tool-args,
.tool-running {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  overflow-wrap: anywhere;
  font-family: var(--font-family);
  font-size: 0.8rem;
  color: var(--color-text);
}

.tool-args + .tool-output-wrap {
  margin-top: 0.4rem;
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

/* System notice row (model switch, etc.) */
.system-notice-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  width: 100%;
  margin: 0.75rem 0;
  user-select: none;
}

.notice-line {
  flex: 1;
  height: 1px;
  background: var(--color-border);
  opacity: 0.6;
}

.notice-pill {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.2rem 0.65rem;
  background: var(--color-surface-muted, #f1f5f9);
  border: 1px solid var(--color-border);
  border-radius: 999px;
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.notice-icon {
  font-size: 0.8rem;
}

.notice-text {
  font-weight: 500;
}

/* Running indicator at the end of the log */
.kind-running {
  display: flex;
  align-items: center;
  margin: 0.5rem 0;
}

.running-indicator-card {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.3rem 0.7rem;
  background: var(--color-surface-muted, #f8fafc);
  border: 1px solid var(--color-border);
  border-radius: 999px;
  font-size: 0.8rem;
  color: var(--color-text-muted);
  user-select: none;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
}

.running-text {
  font-size: 0.8rem;
  color: var(--color-text-muted);
  font-weight: 500;
}
</style>
