// Pure render model for a session's message log, provider-neutral. It knows
// nothing about Vue components: it takes plain ChatEntry lists (and small
// lookups) and returns what the renderer should draw. The step-group logic
// mirrors the opencode desktop client's collapsible turn cards, and the
// question/edit/shell primitives mirror its tool cards.

import type { Capabilities } from '../../../api/sessions'
import type { MessageModel } from '../../../api/messageTypes'
import type { ChatEntry } from '../state/entry'

export type ResultLookup = (entry: ChatEntry) => ChatEntry | undefined

// modelLabel is the small tag under an agent reply: the model that produced
// it, with the named variant appended when one ran.
export function modelLabel(model: MessageModel): string {
  return model.variant ? `${model.id}#${model.variant}` : model.id
}

// Whether a reply is finished with. An event stream that died without a
// turn-ending event leaves its entry flagged as streaming forever, and a
// diagram fence in that reply would wait for a closing token that can never
// arrive — once the stream is gone, what is on screen is all there is.
export function settled(entry: ChatEntry, streamStatus: string | undefined): boolean {
  return entry.streaming !== true || streamStatus === 'error'
}

export function toolName(entry: ChatEntry): string {
  return entry.name ?? entry.text ?? ''
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

export function toolArgument(entry: ChatEntry): string {
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

// Tool entries are gated on the capability, exactly like the live controls: a
// backend that does not report tool events must not show tool activity.
export function toolEventsVisible(capabilities: Capabilities | null | undefined): boolean {
  return capabilities?.toolCallEvents === true
}

// A tool call and its result are one block, keyed by the call id. An agent can
// report a result more than once; only the latest is kept so a call never
// renders several results.
export interface LogModel {
  resultsByCall: Map<string, ChatEntry>
  callIds: Set<string>
  resultForCall: ResultLookup
  visibleEntries: ChatEntry[]
}

export function buildLogModel(messages: ChatEntry[]): LogModel {
  const resultsByCall = new Map<string, ChatEntry>()
  for (const entry of messages) {
    if (entry.kind === 'tool_result' && entry.callId) resultsByCall.set(entry.callId, entry)
  }
  const callIds = new Set<string>()
  for (const entry of messages) {
    if (entry.kind === 'tool_call' && entry.callId) callIds.add(entry.callId)
  }
  const resultForCall: ResultLookup = (entry) =>
    entry.callId ? resultsByCall.get(entry.callId) : undefined
  // A result whose call is present renders inside that call's block, so it is
  // dropped from the top-level list. A result with no matching call (a
  // truncated history, say) stays on its own so its output is never lost.
  const visibleEntries = messages.filter(
    (entry) => !(entry.kind === 'tool_result' && entry.callId !== undefined && callIds.has(entry.callId)),
  )
  return { resultsByCall, callIds, resultForCall, visibleEntries }
}

// --- Step grouping (opencode desktop style) ---

export type RenderBlock =
  | { kind: 'entry'; entry: ChatEntry }
  | { kind: 'step'; id: string; entries: ChatEntry[] }

export function isActivity(entry: ChatEntry): boolean {
  return entry.kind === 'thinking' || entry.kind === 'tool_call' || entry.kind === 'tool_result'
}

// Consecutive thinking/tool entries between messages form one step group, so
// a long agent turn reads as one collapsible card instead of a flat list of
// blocks. Activity entries are accepted (they passed the capability gate
// already, see toolEventsVisible); everything else ends the group.
//
// The group id is built from its first entry's id: entries are only ever
// appended to a running group, so the first member is stable while the group
// grows. A key that changed on every append would make Vue re-create the
// <details> and drop the open state the user chose.
export function groupEntryBlocks(visibleEntries: ChatEntry[]): RenderBlock[] {
  const blocks: RenderBlock[] = []
  let current: ChatEntry[] | null = null
  const flush = () => {
    if (current && current.length > 0) {
      blocks.push({ kind: 'step', id: `step:${current[0].id}`, entries: current })
      current = null
    }
  }
  for (const entry of visibleEntries) {
    if (isQuestionCall(entry)) {
      flush()
      blocks.push({ kind: 'entry', entry })
      continue
    }
    if (isActivity(entry)) {
      if (!current) current = []
      current.push(entry)
    } else {
      flush()
      blocks.push({ kind: 'entry', entry })
    }
  }
  flush()
  return blocks
}

// The split log reuses the same grouping: display="messages" keeps only
// message/error/notice entries and user-facing question tools for the left column,
// display="activity" keeps only the thinking/tool step groups for the right pane.
// display="all" keeps the legacy interleaved log.
export function blocksForDisplay(blocks: RenderBlock[], display: string): RenderBlock[] {
  if (display === 'messages') {
    return blocks.filter(
      (block) =>
        block.kind === 'entry' &&
        (block.entry.kind === 'message' ||
          block.entry.kind === 'error' ||
          block.entry.kind === 'notice' ||
          isQuestionCall(block.entry)),
    )
  }
  if (display === 'activity') {
    return blocks.filter((block) => block.kind === 'step')
  }
  if (display === 'chat') {
    // Pure chat has no tools: keep messages, errors, notices and thinking
    // groups, never a tool-only step card.
    return blocks.filter((block) => {
      if (block.kind === 'entry') {
        return (
          block.entry.kind === 'message' ||
          block.entry.kind === 'error' ||
          block.entry.kind === 'notice'
        )
      }
      return block.entries.some((entry) => entry.kind === 'thinking')
    })
  }
  return blocks
}

// The activity pane's flat list: the same visible thinking/tool entries in
// transcript order, with no step grouping and no collapsing. A tool result
// whose call is present is already merged into that call's card (see
// buildLogModel), so it is not listed twice.

export function stepSummary(entries: ChatEntry[], resultForCall: ResultLookup): string {
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

export function isSubagentCall(entry: ChatEntry, resultForCall: ResultLookup): boolean {
  if (entry.kind !== 'tool_call') return false
  const name = (entry.name ?? entry.text ?? '').toLowerCase()
  if (SUBAGENT_NAMES.has(entry.name ?? '') || SUBAGENT_NAMES.has(name)) return true
  const result = resultForCall(entry)
  const haystack = `${typeof entry.input === 'string' ? entry.input : JSON.stringify(entry.input ?? '')} ${result?.text ?? ''}`
  return haystack.includes('<subagent') || haystack.includes('sessionID=')
}

export function subagentSessionId(entry: ChatEntry, resultForCall: ResultLookup): string {
  const result = resultForCall(entry)
  const haystack = `${typeof entry.input === 'string' ? entry.input : JSON.stringify(entry.input ?? '')} ${result?.text ?? ''}`
  const match = haystack.match(/sessionID=["']([^"']+)["']/)
  return match?.[1] ?? ''
}

export function prettyInput(entry: ChatEntry): string {
  if (entry.input === null || entry.input === undefined) return ''
  if (typeof entry.input === 'string') return entry.input
  try {
    return JSON.stringify(entry.input, null, 2)
  } catch {
    return String(entry.input)
  }
}

// --- Desktop-style edit rendering (opencode + pi) ---

export function isEditCall(entry: ChatEntry): boolean {
  return entry.kind === 'tool_call' && (entry.name ?? entry.text ?? '').toLowerCase() === 'edit'
}

// The tool calls whose detail is worth the space: an edit's diff and a shell
// command's output. flatExpandable drives both the activity pane's fold and the
// default-expanded set; every other tool call is a summary line only.
const SHELL_TOOL_NAMES = new Set([
  'shell',
  'bash',
  'sh',
  'zsh',
  'pwsh',
  'powershell',
  'cmd',
  'command',
])

export function isShellCall(entry: ChatEntry): boolean {
  return (
    entry.kind === 'tool_call' && SHELL_TOOL_NAMES.has((entry.name ?? entry.text ?? '').toLowerCase())
  )
}

// The question tool asks the user to choose among options. Its row folds like
// the other detail-worthy tools, and its body renders the questions readably
// instead of a raw payload.
const QUESTION_TOOL_NAMES = new Set([
  'question',
  'ask',
  'askuserquestion',
  'ask_user',
  'ask_user_question',
  'ask_question',
  'ask_followup_question',
  'inquire',
])

export function isQuestionCall(entry: ChatEntry): boolean {
  if (entry.kind !== 'tool_call') return false
  const name = (entry.name ?? entry.text ?? '').toLowerCase()
  return (
    QUESTION_TOOL_NAMES.has(name) ||
    name.includes('ask_user') ||
    name.includes('question')
  )
}

export interface QuestionOption {
  label: string
  description: string
}

export interface QuestionItem {
  id?: string
  header: string
  question: string
  options: QuestionOption[]
  multiple: boolean
}

// A question option is tolerated in either the opencode {label,description} shape
// or a bare value/label string, so a schema drift degrades to a readable list.
function questionOption(raw: unknown): QuestionOption | null {
  if (typeof raw === 'string') return { label: raw, description: '' }
  if (!raw || typeof raw !== 'object') return null
  const option = raw as Record<string, unknown>
  const label =
    typeof option.label === 'string'
      ? option.label
      : typeof option.value === 'string'
        ? option.value
        : ''
  if (label === '') return null
  return { label, description: typeof option.description === 'string' ? option.description : '' }
}

// questionItems reads the questions out of a question tool call's input. The
// opencode tool carries a `questions` array; a single question object (or a bare
// prompt) is tolerated as well. Unknown shapes yield no items and the caller
// falls back to the plain argument line.
export function questionItems(entry: ChatEntry): QuestionItem[] {
  let input = entry.input
  // Unwrap nested JSON strings if any (e.g. arguments serialized as string)
  if (typeof input === 'string') {
    try {
      input = JSON.parse(input)
    } catch {
      // ignore
    }
  }
  if (typeof input === 'string') {
    try {
      input = JSON.parse(input)
    } catch {
      // ignore
    }
  }

  if (!input || typeof input !== 'object' || Array.isArray(input)) return []
  const record = input as Record<string, unknown>
  const list = Array.isArray(record.questions)
    ? record.questions
    : record.question !== undefined || record.prompt !== undefined
      ? [record]
      : []
  const items: QuestionItem[] = []
  for (const raw of list) {
    if (typeof raw === 'string') {
      items.push({ header: '', question: raw, options: [], multiple: false })
      continue
    }
    if (!raw || typeof raw !== 'object') continue
    const question = raw as Record<string, unknown>
    const id = typeof question.id === 'string' ? question.id.trim() : undefined
    const text =
      typeof question.question === 'string'
        ? question.question
        : typeof question.prompt === 'string'
          ? question.prompt
          : typeof question.text === 'string'
            ? question.text
            : ''
    const options = Array.isArray(question.options)
      ? question.options
          .map(questionOption)
          .filter((option): option is QuestionOption => option !== null)
      : []
    const header =
      typeof question.header === 'string' && question.header.trim() !== ''
        ? question.header.trim()
        : typeof question.detail === 'string' && question.detail.trim() !== '' && text !== question.detail.trim()
          ? question.detail.trim()
          : ''
    const multiple =
      question.multiple === true ||
      question.multi_select === true ||
      question.multiSelect === true
    items.push({
      id,
      header,
      question: text,
      options,
      multiple,
    })
  }
  return items
}

// extractQuestionAnswers maps user answers to each question index for structured display.
export function extractQuestionAnswers(
  questions: QuestionItem[],
  result?: ChatEntry,
): Record<number, string> {
  const answers: Record<number, string> = {}
  if (!result || !result.text) return answers

  let raw = result.text.trim()
  raw = raw.replace(/^已回答[:：]\s*/, '')

  // 1. Try parsing as JSON (DeepSeek / standard form answers)
  try {
    let parsedJson: any = null
    if (raw.startsWith('{') || raw.startsWith('[')) {
      parsedJson = JSON.parse(raw)
    } else {
      const jsonStart = raw.indexOf('{') >= 0 ? raw.indexOf('{') : raw.indexOf('[')
      if (jsonStart >= 0) {
        parsedJson = JSON.parse(raw.slice(jsonStart))
      }
    }
    if (parsedJson) {
      const answerList = Array.isArray(parsedJson)
        ? parsedJson
        : Array.isArray(parsedJson.answers)
          ? parsedJson.answers
          : null

      if (answerList) {
        for (let i = 0; i < answerList.length; i++) {
          const item = answerList[i]
          if (item === null || item === undefined) continue

          let ansText = ''
          let id = ''

          if (typeof item === 'object') {
            id = (item.id || item.key || item.name || '').trim?.() ?? ''
            const selected = Array.isArray(item.selected)
              ? item.selected.filter(Boolean)
              : typeof item.selected === 'string' && item.selected.trim()
                ? [item.selected.trim()]
                : []
            const custom = typeof item.custom === 'string' && item.custom.trim() ? item.custom.trim() : ''
            const parts = [...selected]
            if (custom && !parts.includes(custom)) {
              parts.push(custom)
            }
            ansText = parts.join('、')
            if (!ansText && typeof item.answer === 'string') {
              ansText = item.answer
            } else if (!ansText && typeof item.value === 'string') {
              ansText = item.value
            } else if (!ansText && typeof item.text === 'string') {
              ansText = item.text
            }
          } else {
            ansText = String(item)
          }

          // Match by id or index
          let targetIdx = -1
          if (id) {
            targetIdx = questions.findIndex(
              (q) => q.id === id || (q.header && q.header === id) || (q.question && q.question === id),
            )
          }
          if (targetIdx === -1 && i < questions.length) {
            targetIdx = i
          }
          if (targetIdx >= 0) {
            answers[targetIdx] = ansText
          }
        }
        if (Object.keys(answers).length > 0) {
          return answers
        }
      } else if (typeof parsedJson === 'object' && parsedJson !== null) {
        // Simple key-value object: { "q1": "val1", ... }
        for (let i = 0; i < questions.length; i++) {
          const q = questions[i]
          const val =
            (q.id && parsedJson[q.id]) ||
            (q.header && parsedJson[q.header]) ||
            (q.question && parsedJson[q.question]) ||
            parsedJson[String(i)]
          if (val !== undefined && val !== null) {
            answers[i] = Array.isArray(val) ? val.join('、') : String(val)
          }
        }
        if (Object.keys(answers).length > 0) {
          return answers
        }
      }
    }
  } catch {
    // Ignore JSON parse errors and continue to text patterns
  }

  // 2. Try OpenCode "Question"="Answer" regex pattern
  // Example: User has answered your questions: "平时休息日您最喜欢怎么度过？"="不用继续了，测试..."
  const pairMatches = [...raw.matchAll(/"([^"]+)"\s*=\s*"([^"]*)"/g)]
  if (pairMatches.length > 0) {
    for (let i = 0; i < pairMatches.length; i++) {
      const qText = pairMatches[i][1]
      const aText = pairMatches[i][2]
      let targetIdx = questions.findIndex(
        (q) => q.question.includes(qText) || qText.includes(q.question) || (q.header && q.header === qText),
      )
      if (targetIdx === -1 && i < questions.length) {
        targetIdx = i
      }
      if (targetIdx >= 0) {
        answers[targetIdx] = aText
      }
    }
    if (Object.keys(answers).length > 0) {
      return answers
    }
  }

  // 3. If single question, assign full text
  if (questions.length === 1) {
    answers[0] = raw
    return answers
  }

  // 4. Fallback: if multiple lines, assign line i to question i
  const lines = raw.split('\n').map((l) => l.trim()).filter(Boolean)
  if (lines.length === questions.length) {
    for (let i = 0; i < questions.length; i++) {
      answers[i] = lines[i]
    }
    return answers
  }

  return answers
}

export interface FormattedQuestionAnswer {
  index: number
  text: string
}

export function formatQuestionAnswersList(
  questions: QuestionItem[],
  result?: ChatEntry,
): FormattedQuestionAnswer[] {
  const map = extractQuestionAnswers(questions, result)
  const list: FormattedQuestionAnswer[] = []
  for (let i = 0; i < questions.length; i++) {
    if (map[i] !== undefined && map[i] !== null && String(map[i]).trim() !== '') {
      list.push({ index: i, text: String(map[i]).trim() })
    }
  }
  return list
}

export type ToolKind = 'question' | 'read' | 'edit' | 'bash' | 'search' | 'fetch' | 'tool'

export function toolKind(entry: ChatEntry): ToolKind {
  if (isQuestionCall(entry)) return 'question'
  const name = (entry.name ?? entry.text ?? '').toLowerCase()
  if (name.includes('read')) return 'read'
  if (name.includes('edit') || name.includes('write')) return 'edit'
  if (
    name.includes('bash') ||
    name.includes('shell') ||
    name.includes('sh') ||
    name.includes('exec') ||
    name.includes('command')
  ) {
    return 'bash'
  }
  if (name.includes('grep') || name.includes('glob') || name.includes('search') || name.includes('find')) {
    return 'search'
  }
  if (name.includes('fetch') || name.includes('curl') || name.includes('http')) return 'fetch'
  return 'tool'
}

export function toolIcon(entry: ChatEntry): string {
  return toolKind(entry)
}

export function toolSummaryText(entry: ChatEntry, resultForCall?: ResultLookup): string {
  const res = resultForCall ? resultForCall(entry) : undefined
  if (res?.text) {
    return res.text
  }
  const name = toolName(entry).toLowerCase()
  const arg = toolArgument(entry)
  if (name.includes('read')) {
    return arg ? `读取 ${arg} 文件…` : '读取文件中…'
  }
  if (name.includes('write')) {
    return arg ? `写入 ${arg} 文件…` : '写入文件中…'
  }
  if (name.includes('edit')) {
    return arg ? `编辑 ${arg} 文件…` : '编辑文件中…'
  }
  if (
    name.includes('bash') ||
    name.includes('shell') ||
    name.includes('sh') ||
    name.includes('exec') ||
    name.includes('command')
  ) {
    return arg ? `执行 ${arg} 命令…` : '执行命令中…'
  }
  if (name.includes('grep') || name.includes('glob') || name.includes('search') || name.includes('find')) {
    return arg ? `检索 ${arg}…` : '检索中…'
  }
  if (name.includes('fetch') || name.includes('curl') || name.includes('http')) {
    return arg ? `访问 ${arg}…` : '网络请求中…'
  }
  return arg ? `${toolName(entry)}: ${arg}` : `${toolName(entry)} 执行中…`
}

export function flatExpandable(entry: ChatEntry): boolean {
  if (entry.kind === 'thinking') return true
  if (entry.kind !== 'tool_call') return false
  return isEditCall(entry) || isShellCall(entry) || isQuestionCall(entry)
}

// The name a subagent row shows: the task's description names the job better
// than the generic tool name does, so it wins when present.
export function subagentName(entry: ChatEntry): string {
  return toolArgument(entry) || toolName(entry)
}

// The file an edit acted on. pi uses {path|file_path|file} with
// edits[{oldText,newText}]; opencode uses {path,oldString,newString}.
// Preference order matches the backend normalizers.
export function editPathFor(entry: ChatEntry): string {
  const input = entry.input
  if (input && typeof input === 'object' && !Array.isArray(input)) {
    const record = input as Record<string, unknown>
    for (const key of ['path', 'file_path', 'filePath', 'file']) {
      const value = record[key]
      if (typeof value === 'string' && value.trim() !== '') return value
    }
  }
  return toolArgument(entry)
}

export function editDiffFor(entry: ChatEntry, resultForCall: ResultLookup): ChatEntry['diff'] {
  return resultForCall(entry)?.diff
}
