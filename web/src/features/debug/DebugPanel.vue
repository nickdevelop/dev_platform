<script setup lang="ts">
// Floating debug drawer for the SSE bus. The settings toggle only shows the
// launcher. Capture is explicit: the panel's start/stop controls attach or
// detach the envelope tap, and closing the drawer keeps both the running capture
// and the captured frames. The list's 序号 is the envelope's own SSE id /
// sequence; the store's local capture index is only a list key.
//
// The presentation is a light ledger: a capture summary strip, channel counts,
// a dense frame table with the wire sequence column, and an inline payload
// detail. Pure observation: it never inherits modal focus trapping, so messages
// stay dispatchable while debugging.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useDebugPanelStore, type DebugRow } from '../../stores/debugPanel'
import {
  channelCounts,
  formatClock,
  identityBadges,
  prettyJSON,
  type DebugBadge,
} from './debugModel'
import { useDraggablePanel } from '../../shared/composables/useDraggablePosition'

const debug = useDebugPanelStore()

const {
  panelStyle,
  draggingPanel,
  onPanelPointerDown,
  onPanelPointerMove,
  onPanelPointerUp,
  onPanelPointerCancel,
} = useDraggablePanel({
  panelStorageKey: 'dev-platform-debug-panel-position',
  panelSize: { width: 920, height: 620 },
})

const channels = [
  { label: '全部', value: 'all' },
  { label: 'sessions', value: 'sessions' },
  { label: 'chat', value: 'chat' },
  { label: 'tasks', value: 'tasks' },
  { label: 'system', value: 'system' },
]

const expandedSeq = ref<number | null>(null)
const listEl = ref<HTMLElement | null>(null)
// true while the user keeps the scroller pinned at the bottom, so new frames
// auto-scroll instead of yanking the list out from under a reading position.
const pinned = ref(true)
const copyFeedback = ref<{ seq: number; ok: boolean } | null>(null)
let copyTimer: ReturnType<typeof setTimeout> | null = null

const entries = computed(() => debug.visibleEntries)
const counts = computed(() => channelCounts(debug.entries))
const errorCount = computed(() => debug.entries.filter((entry) => isErrorEntry(entry)).length)
const filterActive = computed(
  () =>
    debug.filter.channel !== 'all' ||
    debug.filter.keyword.trim() !== '' ||
    debug.filter.eventType.trim() !== '',
)

// One badge lookup per visible frame; the template reads it twice per row
// (the chips and the folded +N count) without rebuilding the arrays.
const badgeMap = computed(() => {
  const map = new Map<number, { badges: DebugBadge[]; hidden: number }>()
  for (const entry of entries.value) {
    map.set(entry.seq, identityBadges(entry.identity, 3))
  }
  return map
})

// Clock parts let the phone drop the milliseconds without a second formatting
// call per cell; the desktop keeps the full HH:MM:SS.mmm value.
const clockMap = computed(() => {
  const map = new Map<number, { base: string; ms: string }>()
  for (const entry of entries.value) {
    const text = formatClock(entry.receivedAt)
    map.set(entry.seq, { base: text.slice(0, 8), ms: text.slice(8) })
  }
  return map
})

function clockOf(entry: DebugRow): { base: string; ms: string } {
  return clockMap.value.get(entry.seq) ?? { base: '--:--:--', ms: '.---' }
}

function isErrorEntry(entry: DebugRow): boolean {
  return entry.eventType === 'error'
}

function badgeSet(entry: DebugRow): { badges: DebugBadge[]; hidden: number } {
  return badgeMap.value.get(entry.seq) ?? { badges: [], hidden: 0 }
}

function entryPayloadText(entry: DebugRow): string {
  if (!entry.pretty) entry.pretty = prettyJSON(entry.payload)
  return entry.pretty
}

function toggleExpanded(entry: DebugRow) {
  const opening = expandedSeq.value !== entry.seq
  expandedSeq.value = opening ? entry.seq : null
  if (!opening) return
  // Expanding is a reading action: stop following new frames until the user
  // returns to the bottom, otherwise the row they opened scrolls away.
  pinned.value = false
  nextTick(() => {
    const detail = listEl.value?.querySelector<HTMLElement>(
      `.debug-entry[data-seq="${entry.seq}"] .debug-detail`,
    )
    detail?.scrollIntoView({ block: 'nearest' })
  })
}

function onListScroll() {
  const el = listEl.value
  if (!el) return
  pinned.value = el.scrollHeight - el.scrollTop - el.clientHeight <= 48
}

function followIfPinned() {
  const el = listEl.value
  if (!el || !pinned.value) return
  el.scrollTop = el.scrollHeight
}

function onViewportResize() {
  nextTick(followIfPinned)
}

watch(
  () => [entries.value.length, debug.filter.channel, debug.filter.keyword, debug.filter.eventType],
  () => {
    nextTick(followIfPinned)
  },
)

// Esc closes the drawer — the same dismiss contract as the platform's dialogs.
function onGlobalKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.stopPropagation()
    close()
  }
}

onMounted(() => {
  window.addEventListener('keydown', onGlobalKeydown, true)
  window.addEventListener('resize', onViewportResize)
  // If the drawer is mounted with frames already in the buffer (a hot reload or
  // a restored dev session), start at the newest frame like the open flow does.
  nextTick(followIfPinned)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onGlobalKeydown, true)
  window.removeEventListener('resize', onViewportResize)
  if (copyTimer) clearTimeout(copyTimer)
})

function close() {
  debug.closeDrawer()
}

function clearBuffer() {
  debug.clearBuffer()
  expandedSeq.value = null
  copyFeedback.value = null
}

function resetFilters() {
  debug.filter.channel = 'all'
  debug.filter.keyword = ''
  debug.filter.eventType = ''
}

// The error count doubles as a filter: one click narrows the current view to
// the failure frames, which is the first thing a debugging session looks for.
function toggleErrorFilter() {
  if (debug.filter.eventType === 'error') {
    debug.filter.eventType = ''
    return
  }
  if (errorCount.value === 0) return
  // Jump to the failures wherever they are in the buffer: an error hidden
  // behind the current channel or keyword would otherwise stay invisible.
  debug.filter.channel = 'all'
  debug.filter.keyword = ''
  debug.filter.eventType = 'error'
}

function copyLabel(entry: DebugRow): string {
  const feedback = copyFeedback.value
  if (!feedback || feedback.seq !== entry.seq) return '复制'
  return feedback.ok ? '已复制' : '复制失败'
}

async function copyPayload(entry: DebugRow) {
  const ok = await writeClipboard(entryPayloadText(entry))
  copyFeedback.value = { seq: entry.seq, ok }
  if (copyTimer) clearTimeout(copyTimer)
  copyTimer = setTimeout(() => {
    copyFeedback.value = null
  }, 1500)
}

// Clipboard API first (localhost/HTTPS); a textarea fallback keeps the copy
// action usable on plain-HTTP deployments where navigator.clipboard is absent.
async function writeClipboard(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // Fall through to the legacy path.
  }
  let area: HTMLTextAreaElement | null = null
  try {
    area = document.createElement('textarea')
    area.value = text
    area.setAttribute('readonly', '')
    area.style.position = 'fixed'
    area.style.top = '-1000px'
    area.style.opacity = '0'
    document.body.appendChild(area)
    area.select()
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    area?.remove()
  }
}
</script>

<template>
  <div
    class="debug-panel"
    :class="{ 'dragging-panel': draggingPanel }"
    :style="panelStyle"
    data-test="debug-panel"
    role="dialog"
    aria-labelledby="debug-panel-title"
  >
    <!-- Header: drag grip + title + bus status + actions -->
    <header
      class="debug-head"
      data-test="debug-panel-head"
      @pointerdown="onPanelPointerDown"
      @pointermove="onPanelPointerMove"
      @pointerup="onPanelPointerUp"
      @pointercancel="onPanelPointerCancel"
    >
      <h1 id="debug-panel-title" class="debug-title">SSE 总线调试</h1>
      <span
        class="debug-status"
        :class="`is-${debug.status}`"
        :title="`总线连接状态：${debug.status}`"
      >
        <span class="debug-status-dot" aria-hidden="true"></span>{{ debug.status }}
      </span>
      <span class="debug-scope">本浏览器总线</span>
      <div class="debug-actions" @pointerdown.stop>
        <div class="debug-capture" role="group" aria-label="捕获控制">
          <span
            class="debug-capture-dot"
            :class="{ 'is-live': debug.capturing }"
            :title="debug.capturing ? '捕获已开启' : '捕获已停止'"
            aria-hidden="true"
          ></span>
          <button
            type="button"
            class="debug-text-btn"
            :disabled="debug.capturing"
            aria-label="开启捕获"
            @click="debug.startCapture()"
          >开启</button>
          <button
            type="button"
            class="debug-text-btn"
            :disabled="!debug.capturing"
            aria-label="停止捕获"
            @click="debug.stopCapture()"
          >停止</button>
          <span class="debug-sr" aria-live="polite">{{ debug.capturing ? '捕获已开启' : '捕获已停止' }}</span>
        </div>
        <button
          type="button"
          class="debug-text-btn"
          data-test="debug-clear"
          @click="clearBuffer"
        >清空</button>
        <button
          type="button"
          class="debug-icon-btn"
          aria-label="关闭调试面板"
          title="关闭调试面板"
          @click="close"
        >
          <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
            <path d="M3 3l10 10M13 3L3 13" />
          </svg>
        </button>
      </div>
    </header>

    <!-- Capture summary: one container split by 1px rules -->
    <section class="debug-stats" aria-label="捕获摘要">
      <div class="debug-stat">
        <span class="debug-stat-label" title="环形缓冲中保留的帧数，上限 500">缓冲</span>
        <div class="debug-stat-value">
          <b>{{ debug.entries.length }}</b><em>/ 500 帧</em>
        </div>
      </div>
      <div class="debug-stat">
        <span class="debug-stat-label" title="当前过滤条件下命中的帧数">当前命中</span>
        <div class="debug-stat-value">
          <b>{{ entries.length }}</b><em>帧</em>
        </div>
      </div>
      <div class="debug-stat">
        <span class="debug-stat-label" title="本次面板会话中因环形缓冲上限被淘汰的帧数">缓冲丢弃</span>
        <div class="debug-stat-value">
          <b :class="{ 'is-warn': debug.dropped > 0 }">{{ debug.dropped }}</b><em>帧</em>
        </div>
      </div>
      <button
        type="button"
        class="debug-stat debug-stat-action"
        :class="{ 'is-active': debug.filter.eventType === 'error' }"
        :disabled="errorCount === 0 && debug.filter.eventType !== 'error'"
        :aria-pressed="debug.filter.eventType === 'error'"
        title="点击跳转到错误帧"
        @click="toggleErrorFilter"
      >
        <span class="debug-stat-label" title="本次面板会话中 eventType 为 error 的帧数；点击跳转">错误</span>
        <div class="debug-stat-value">
          <b :class="{ 'is-danger': errorCount > 0 }">{{ errorCount }}</b><em>帧</em>
        </div>
      </button>
    </section>

    <!-- Channel filter: the active tab is a 2px accent line on the nearest edge -->
    <nav class="debug-channels" aria-label="频道过滤" data-test="debug-filter-channel">
      <button
        v-for="channel in channels"
        :key="channel.value"
        type="button"
        class="debug-channel"
        :aria-pressed="debug.filter.channel === channel.value"
        @click="debug.filter.channel = channel.value"
      >
        {{ channel.label }}<span class="debug-channel-count">{{ counts[channel.value] ?? 0 }}</span>
      </button>
    </nav>

    <!-- Toolbar: keyword + event type; reset appears only when a filter is on -->
    <div class="debug-toolbar" data-test="debug-filters">
      <label class="debug-field debug-field-search">
        <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
          <circle cx="7" cy="7" r="4.2" />
          <path d="M10.2 10.2L14 14" />
        </svg>
        <input
          v-model="debug.filter.keyword"
          type="text"
          placeholder="会话 / chat / 实例 / run ID 或后端名"
          aria-label="按标识搜索"
          data-test="debug-filter-keyword"
        />
      </label>
      <label class="debug-field debug-field-type">
        <input
          v-model="debug.filter.eventType"
          type="text"
          placeholder="事件类型，如 delta"
          aria-label="按事件类型过滤"
          data-test="debug-filter-type"
        />
      </label>
      <button
        v-if="filterActive"
        type="button"
        class="debug-text-btn debug-reset"
        data-test="debug-filter-reset"
        @click="resetFilters"
      >重置</button>
    </div>

    <div class="debug-columns" aria-hidden="true">
      <span title="报文序号（SSE id / sequence）；缺失显示 —">序号</span>
      <span>时间</span>
      <span>事件</span>
      <span>频道</span>
      <span>目标</span>
      <span>标识</span>
      <span></span>
    </div>

    <!-- Frames, oldest-first with bottom follow -->
    <div class="debug-list" data-test="debug-list" ref="listEl" @scroll.passive="onListScroll">
      <div v-if="entries.length === 0" class="debug-empty" data-test="debug-empty">
        <template v-if="debug.entries.length > 0">
          <strong>当前过滤条件下没有命中的帧</strong>
          <span>调整频道、关键字或事件类型，或重置过滤条件。</span>
          <button type="button" class="debug-text-btn" @click="resetFilters">重置过滤</button>
        </template>
        <template v-else-if="!debug.capturing">
          <strong>捕获已停止</strong>
          <span>点击「开启」开始接收本浏览器的总线帧；已捕获的帧会保留在内存中。</span>
          <button type="button" class="debug-text-btn" @click="debug.startCapture()">开启捕获</button>
        </template>
        <template v-else>
          <strong>等待总线消息…</strong>
          <span>捕获开启后，本浏览器收到的会话、对话、任务与系统事件会实时出现在这里。</span>
        </template>
      </div>

      <div
        v-for="entry in entries"
        :key="entry.seq"
        class="debug-entry"
        :class="{ 'is-open': expandedSeq === entry.seq }"
        :data-seq="entry.seq"
        data-test="debug-row"
      >
        <button
          type="button"
          class="debug-row"
          :aria-expanded="expandedSeq === entry.seq"
          :aria-controls="expandedSeq === entry.seq ? `debug-detail-${entry.seq}` : undefined"
          @click="toggleExpanded(entry)"
        >
          <span class="debug-row-primary">
            <span class="debug-seq" :title="entry.sequence === null ? '报文中没有 sequence / SSE id' : `报文序号 ${entry.sequence}`">{{ entry.sequence ?? '—' }}</span>
            <span class="debug-event" :class="{ 'is-error': isErrorEntry(entry) }" :title="entry.eventType">
              <span v-if="isErrorEntry(entry)" class="debug-error-dot" aria-hidden="true"></span>
              <span class="debug-event-name">{{ entry.eventType }}</span>
            </span>
            <span class="debug-channel-name">{{ entry.channel }}</span>
            <span class="debug-chevron" aria-hidden="true">
              <svg viewBox="0 0 16 16" focusable="false"><path d="M6 3l5 5-5 5" /></svg>
            </span>
          </span>
          <span class="debug-row-secondary">
            <span class="debug-time" :title="formatClock(entry.receivedAt)">{{ clockOf(entry).base }}<span class="debug-time-ms">{{ clockOf(entry).ms }}</span></span>
            <span class="debug-target" :title="entry.targetId">{{ entry.targetId || '—' }}</span>
            <span class="debug-ids">
              <span
                v-for="badge in badgeSet(entry).badges"
                :key="`${badge.key}:${badge.value}`"
                class="debug-chip"
                :class="{ 'is-backend': badge.backend }"
                :title="`${badge.key}: ${badge.value}`"
              >{{ badge.label }}</span>
              <span
                v-if="badgeSet(entry).hidden"
                class="debug-chip"
                :title="`其余 ${badgeSet(entry).hidden} 个标识在展开载荷中`"
              >+{{ badgeSet(entry).hidden }}</span>
            </span>
          </span>
        </button>

        <div
          v-if="expandedSeq === entry.seq"
          :id="`debug-detail-${entry.seq}`"
          class="debug-detail"
        >
          <div class="debug-detail-head">
            <span>原始载荷 · JSON</span>
            <button
              type="button"
              class="debug-text-btn"
              :class="{ 'is-danger-text': copyFeedback?.seq === entry.seq && !copyFeedback.ok }"
              @click="copyPayload(entry)"
            >{{ copyLabel(entry) }}</button>
          </div>
          <pre class="debug-json" data-test="debug-json">{{ entryPayloadText(entry) }}</pre>
        </div>
      </div>
    </div>

    <!-- Footer semantics reminder -->
    <footer class="debug-foot">
      <span v-if="debug.capturing">捕获中 · 关闭面板不会停止捕获，已捕获帧保留在内存</span>
      <span v-else>已停止 · 已捕获帧保留在内存，点击「开启」继续捕获</span>
      <span class="debug-foot-right" title="捕获数据不写入任何存储">纯观测 · 不落库</span>
    </footer>
  </div>
</template>

<style scoped>
/* Light ledger direction: one ground, neutral data, 1px rules, and a single
   accent line on the active channel / expanded row. No gradients, no shadows. */
.debug-panel {
  position: fixed;
  z-index: 64;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
  background: var(--color-surface, #ffffff);
  border: 1px solid var(--color-border, #e3e6eb);
  border-radius: calc(var(--radius, 3px) + 1px);
  color: var(--color-text, #1f2328);
}

.debug-panel.dragging-panel {
  user-select: none;
}

.debug-head {
  flex: none;
  height: 44px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px 0 10px;
  border-bottom: 1px solid var(--color-border, #e3e6eb);
  cursor: grab;
  touch-action: none;
  user-select: none;
}

.dragging-panel .debug-head {
  cursor: grabbing;
}

.debug-title {
  margin: 0;
  font-size: var(--text-size-14);
  font-weight: 600;
  letter-spacing: -0.01em;
  white-space: nowrap;
}

.debug-status {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 19px;
  padding: 0 6px;
  border: 1px solid var(--color-border, #e3e6eb);
  border-radius: var(--radius, 3px);
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-11);
  white-space: nowrap;
}

.debug-status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-text-muted, #64748b);
}

.debug-status.is-open {
  color: var(--color-text, #1f2328);
}

.debug-status.is-open .debug-status-dot {
  background: var(--color-success, #15803d);
}

.debug-status.is-connecting .debug-status-dot,
.debug-status.is-reconnecting .debug-status-dot {
  background: var(--color-warning, #92400e);
}

.debug-status.is-error {
  border-color: var(--color-danger, #b91c1c);
  color: var(--color-danger, #b91c1c);
}

.debug-status.is-error .debug-status-dot {
  background: var(--color-danger, #b91c1c);
}

.debug-scope {
  margin-left: 2px;
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-11);
  white-space: nowrap;
}

.debug-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 2px;
}

/* Capture switch: the dot is the state, the enabled button is the next action */
.debug-capture {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-right: 2px;
}

.debug-capture-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-text-muted, #64748b);
}

.debug-capture-dot.is-live {
  background: var(--color-success, #15803d);
}

.debug-capture .debug-text-btn {
  color: var(--color-text, #1f2328);
}

.debug-capture .debug-text-btn:disabled {
  color: var(--color-text-muted, #64748b);
  opacity: 0.65;
  cursor: default;
}

.debug-capture .debug-text-btn:disabled:hover {
  color: var(--color-text-muted, #64748b);
  background: transparent;
}

.debug-sr {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
  border: 0;
}

.debug-text-btn {
  height: 26px;
  padding: 0 8px;
  border: 1px solid transparent;
  border-radius: var(--radius, 3px);
  background: transparent;
  color: var(--color-text-muted, #64748b);
  font: inherit;
  font-size: var(--text-size-11);
  cursor: pointer;
  transition: color 120ms ease-out, background-color 120ms ease-out, border-color 120ms ease-out;
}

.debug-text-btn:hover {
  color: var(--color-text, #1f2328);
  background: var(--color-surface-muted, #f8fafc);
}

.debug-text-btn.is-danger-text {
  color: var(--color-danger, #b91c1c);
}

.debug-icon-btn {
  width: 26px;
  height: 26px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 1px solid transparent;
  border-radius: var(--radius, 3px);
  background: transparent;
  color: var(--color-text-muted, #64748b);
  cursor: pointer;
  transition: color 120ms ease-out, background-color 120ms ease-out;
}

.debug-icon-btn:hover {
  color: var(--color-text, #1f2328);
  background: var(--color-surface-muted, #f8fafc);
}

.debug-icon-btn svg {
  width: 14px;
  height: 14px;
  stroke: currentColor;
  stroke-width: 1.6;
  fill: none;
  stroke-linecap: round;
  stroke-linejoin: round;
}

/* Capture summary: a divided band, values as type not tiles */
.debug-stats {
  flex: none;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  height: 58px;
  border-bottom: 1px solid var(--color-border, #e3e6eb);
}

.debug-stat {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 2px;
  min-width: 0;
  padding: 0 10px;
  border-left: 1px solid var(--color-border, #e3e6eb);
}

.debug-stat:first-child {
  border-left: 0;
}

.debug-stat-action {
  width: 100%;
  border-top: 0;
  border-right: 0;
  border-bottom: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.debug-stat-action:hover:not(:disabled),
.debug-stat-action.is-active {
  background: var(--color-surface-muted, #f8fafc);
}

.debug-stat-action:disabled {
  cursor: default;
}

.debug-stat-action:focus-visible {
  outline-offset: -2px;
}

.debug-stat-label {
  overflow: hidden;
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-11);
  white-space: nowrap;
  text-overflow: ellipsis;
}

.debug-stat-value {
  display: flex;
  align-items: baseline;
  gap: 5px;
  min-width: 0;
}

.debug-stat-value b {
  color: var(--color-text, #1f2328);
  font-size: calc(var(--text-size-14) * 1.28);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  line-height: 1.1;
}

.debug-stat-value b.is-warn {
  color: var(--color-warning, #92400e);
}

.debug-stat-value b.is-danger {
  color: var(--color-danger, #b91c1c);
}

.debug-stat-value em {
  overflow: hidden;
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-11);
  font-style: normal;
  white-space: nowrap;
  text-overflow: ellipsis;
}

/* Channel filter */
.debug-channels {
  flex: none;
  display: flex;
  align-items: stretch;
  gap: 18px;
  height: 32px;
  padding: 0 10px;
  overflow-x: auto;
  border-bottom: 1px solid var(--color-border, #e3e6eb);
  scrollbar-width: none;
}

.debug-channels::-webkit-scrollbar {
  display: none;
}

.debug-channel {
  flex: none;
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  padding: 0;
  border: 0;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--color-text-muted, #64748b);
  font: inherit;
  font-size: var(--text-size-12);
  white-space: nowrap;
  cursor: pointer;
  transition: color 120ms ease-out, border-color 120ms ease-out;
}

.debug-channel:hover {
  color: var(--color-text, #1f2328);
}

.debug-channel[aria-pressed='true'] {
  color: var(--color-text, #1f2328);
  border-bottom-color: var(--color-accent, #4a8ad6);
}

.debug-channel-count {
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-11);
  font-variant-numeric: tabular-nums;
}

/* Filters */
.debug-toolbar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-bottom: 1px solid var(--color-border, #e3e6eb);
}

.debug-field {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  min-width: 0;
  padding: 0 8px;
  border: 1px solid var(--color-border, #e3e6eb);
  border-radius: var(--radius, 3px);
  background: var(--color-surface, #ffffff);
  color: var(--color-text-muted, #64748b);
  transition: border-color 120ms ease-out;
}

.debug-field:focus-within {
  border-color: var(--color-accent, #4a8ad6);
}

.debug-field svg {
  width: 12px;
  height: 12px;
  flex: none;
  stroke: currentColor;
  stroke-width: 1.8;
  fill: none;
  stroke-linecap: round;
}

.debug-field input {
  width: 100%;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--color-text, #1f2328);
  font: inherit;
  font-size: var(--text-size-12);
}

.debug-field input::placeholder {
  color: var(--color-text-muted, #64748b);
}

.debug-field-search {
  flex: 1;
}

.debug-field-type {
  width: 168px;
  flex: none;
}

.debug-reset {
  flex: none;
}

/* Table head and rows */
.debug-columns {
  flex: none;
  display: grid;
  grid-template-columns: 56px 96px minmax(150px, 1.2fr) 68px 128px minmax(0, 1.8fr) 14px;
  align-items: center;
  gap: 8px;
  height: 24px;
  padding: 0 10px;
  border-bottom: 1px solid var(--color-border, #e3e6eb);
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-11);
}

.debug-list {
  flex: 1;
  min-height: 0;
  overflow-x: hidden;
  overflow-y: auto;
}

.debug-entry {
  border-bottom: 1px solid var(--color-border, #e3e6eb);
}

/* The two wrappers exist for the phone composition: on desktop they dissolve
   with display:contents, so the seven cells remain direct grid items. */
.debug-row-primary,
.debug-row-secondary {
  display: contents;
}

.debug-row {
  display: grid;
  grid-template-columns: 56px 96px minmax(150px, 1.2fr) 68px 128px minmax(0, 1.8fr) 14px;
  align-items: center;
  gap: 8px;
  width: 100%;
  height: 32px;
  padding: 0 10px;
  border: 0;
  border-left: 2px solid transparent;
  background: transparent;
  color: var(--color-text, #1f2328);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: background-color 120ms ease-out, border-color 120ms ease-out;
}

.debug-row:hover {
  background: var(--color-surface-muted, #f8fafc);
}

.debug-entry.is-open > .debug-row {
  border-left-color: var(--color-accent, #4a8ad6);
  background: var(--color-surface-muted, #f8fafc);
}

.debug-seq {
  grid-column: 1;
}

.debug-time {
  grid-column: 2;
}

.debug-event {
  grid-column: 3;
}

.debug-channel-name {
  grid-column: 4;
}

.debug-target {
  grid-column: 5;
}

.debug-ids {
  grid-column: 6;
}

.debug-chevron {
  grid-column: 7;
}

/* Explicit row keeps every cell on the single desktop line: without it the
   auto-placement cursor advances past column 7 and pushes later cells to row 2. */
.debug-seq,
.debug-time,
.debug-event,
.debug-channel-name,
.debug-target,
.debug-ids,
.debug-chevron {
  grid-row: 1;
}

.debug-seq,
.debug-time {
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-11);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.debug-time-ms {
  display: inline;
}

.debug-seq {
  overflow: hidden;
  text-overflow: ellipsis;
}

.debug-event {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  overflow: hidden;
  color: var(--color-text, #1f2328);
  font-size: var(--text-size-12);
  font-weight: 600;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.debug-event-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.debug-error-dot {
  width: 5px;
  height: 5px;
  flex: none;
  border-radius: 50%;
  background: var(--color-danger, #b91c1c);
}

.debug-channel-name,
.debug-target {
  overflow: hidden;
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-11);
  white-space: nowrap;
  text-overflow: ellipsis;
}

.debug-ids {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  overflow: hidden;
}

.debug-chip {
  display: inline-flex;
  align-items: center;
  max-width: 150px;
  height: 17px;
  padding: 0 4px;
  overflow: hidden;
  border: 1px solid var(--color-border, #e3e6eb);
  border-radius: var(--radius, 3px);
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-11);
  white-space: nowrap;
  text-overflow: ellipsis;
}

.debug-chevron {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--color-text-muted, #64748b);
}

.debug-chevron svg {
  width: 10px;
  height: 10px;
  stroke: currentColor;
  stroke-width: 1.8;
  fill: none;
  stroke-linecap: round;
  stroke-linejoin: round;
  transition: transform 120ms ease-out;
}

.debug-entry.is-open .debug-chevron svg {
  transform: rotate(90deg);
}

/* Inline payload */
.debug-detail {
  border-top: 1px solid var(--color-border, #e3e6eb);
  background: var(--color-surface-muted, #f8fafc);
}

.debug-detail-head {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 26px;
  padding: 0 10px;
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-11);
}

.debug-detail-head .debug-text-btn {
  height: 20px;
  margin-left: auto;
  padding: 0 5px;
}

.debug-json {
  margin: 0;
  padding: 6px 10px 10px;
  max-height: 190px;
  overflow: auto;
  border-top: 1px solid var(--color-border, #e3e6eb);
  background: var(--color-surface, #ffffff);
  color: var(--color-text, #1f2328);
  font-family: var(--font-mono, ui-monospace, monospace); /* taste-skill-ignore: raw JSON payload code */
  font-size: var(--text-size-11);
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

/* States */
.debug-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 100%;
  min-height: 180px;
  padding: 24px;
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-12);
  text-align: center;
}

.debug-empty strong {
  color: var(--color-text, #1f2328);
  font-size: var(--text-size-13);
  font-weight: 600;
}

.debug-empty .debug-text-btn {
  margin-top: 2px;
  color: var(--color-text, #1f2328);
}

.debug-foot {
  flex: none;
  height: 28px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  border-top: 1px solid var(--color-border, #e3e6eb);
  color: var(--color-text-muted, #64748b);
  font-size: var(--text-size-11);
}

.debug-foot-right {
  margin-left: auto;
  white-space: nowrap;
}

/* Focus: one accent line, matching the panel's selection language */
.debug-panel :focus-visible {
  outline: 1px solid var(--color-accent, #4a8ad6);
  outline-offset: 1px;
}

.debug-row:focus-visible {
  outline-offset: -2px;
}

.debug-field input:focus-visible {
  outline: none;
}

/* Tablet: the table becomes two-line rows once seven columns stop fitting */
@media (max-width: 760px) {
  .debug-columns {
    display: none;
  }

  .debug-row {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 4px;
    height: auto;
    padding: 7px 8px;
  }

  .debug-row-primary,
  .debug-row-secondary {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .debug-row-primary .debug-event {
    flex: 1 1 auto;
    min-width: 0;
  }

  .debug-row-primary .debug-chevron {
    margin-left: auto;
  }

  .debug-row-secondary .debug-time {
    flex: none;
  }

  .debug-row-secondary .debug-target {
    flex: 0 1 90px;
    min-width: 0;
  }

  .debug-row-secondary .debug-ids {
    flex: 1 1 0;
    min-width: 0;
  }
}

/* Phone */
@media (max-width: 640px) {
  .debug-head {
    height: 42px;
    gap: 6px;
    padding: 0 6px 0 8px;
  }

  .debug-title {
    font-size: var(--text-size-13);
  }

  .debug-scope {
    display: none;
  }

  .debug-status {
    height: 18px;
    padding: 0 5px;
  }

  .debug-actions {
    gap: 0;
  }

  .debug-text-btn {
    padding: 0 6px;
  }

  .debug-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    height: auto;
  }

  .debug-stat {
    height: 48px;
    padding: 0 10px;
  }

  .debug-stat:nth-child(3) {
    border-left: 0;
    border-top: 1px solid var(--color-border, #e3e6eb);
  }

  .debug-stat:nth-child(4) {
    border-top: 1px solid var(--color-border, #e3e6eb);
  }

  .debug-stat-value b {
    font-size: calc(var(--text-size-14) * 1.1);
  }

  .debug-channels {
    gap: 14px;
    padding: 0 8px;
  }

  .debug-toolbar {
    flex-wrap: wrap;
    gap: 6px;
    padding: 6px 8px;
  }

  .debug-field-search {
    flex: 1 1 100%;
  }

  .debug-field-type {
    width: auto;
    flex: 1;
  }

  .debug-chip {
    max-width: 120px;
  }

  .debug-time-ms {
    display: none;
  }

  .debug-json {
    max-height: 240px;
  }

  .debug-foot {
    height: auto;
    min-height: 30px;
    flex-wrap: wrap;
    padding: 5px 10px;
  }
}
</style>
