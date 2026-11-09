<script setup lang="ts">
// Floating debug drawer for the SSE bus. Visible (and capturing) only while the
// settings toggle is on AND the drawer itself is open; closing it wipes the
// buffer entirely. Pure observation: unlike the quick-assistant dialog it never
// inherits modal focus trapping, so messages stay dispatchable while debugging.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useDebugPanelStore, type DebugRow } from '../../stores/debugPanel'
import { formatClock, prettyJSON, type DebugFilter } from './debugModel'
import { UiInput, UiSelect, UiButton } from '../../shared/ui'
import {
  useDraggablePanel,
} from '../../shared/composables/useDraggablePosition'

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
  { label: '全部频道', value: 'all' },
  { label: 'sessions · Agent 会话', value: 'sessions' },
  { label: 'chat · 纯对话', value: 'chat' },
  { label: 'tasks · 任务/协同流', value: 'tasks' },
  { label: 'system · 系统', value: 'system' },
]

const expandedSeq = ref<number | null>(null)
const listEl = ref<HTMLElement | null>(null)
// true while the user keeps the scroller pinned at the bottom, so new frames
// auto-scroll instead of yanking the list out from under a reading position.
const pinned = ref(true)

const statusLabel = computed(() => {
  switch (debug.status) {
    case 'open':
      return '总线连接 · open'
    case 'connecting':
      return '总线连接 · connecting'
    case 'reconnecting':
      return '总线连接 · reconnecting'
    case 'error':
      return '总线连接 · error'
    default:
      return '总线连接 · idle'
  }
})

const entries = computed(() => debug.visibleEntries)

function entryPayloadText(entry: DebugRow): string {
  if (!entry.pretty) entry.pretty = prettyJSON(entry.payload)
  return entry.pretty
}

function toggleExpanded(entry: DebugRow) {
  expandedSeq.value = expandedSeq.value === entry.seq ? null : entry.seq
  nextTick(followIfPinned)
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

watch(
  () => [entries.value.length, debug.filter],
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

onMounted(() => window.addEventListener('keydown', onGlobalKeydown, true))
onBeforeUnmount(() => window.removeEventListener('keydown', onGlobalKeydown, true))

function close() {
  debug.closeDrawer()
}
</script>

<template>
  <div
    class="debug-panel"
    :class="{ 'dragging-panel': draggingPanel }"
    :style="panelStyle"
    data-test="debug-panel"
    role="dialog"
    aria-label="SSE 总线调试面板"
  >
    <!-- Header: drag grip + title + status -->
    <header
      class="debug-head"
      data-test="debug-panel-head"
      @pointerdown="onPanelPointerDown"
      @pointermove="onPanelPointerMove"
      @pointerup="onPanelPointerUp"
      @pointercancel="onPanelPointerCancel"
    >
      <div class="min-w-0 flex items-center gap-2">
        <span class="debug-head-title">SSE 总线调试</span>
        <span class="debug-head-status" :class="`status-${debug.status}`">{{ statusLabel }}</span>
        <span class="debug-head-count">{{ entries.length }}/{{ debug.entries.length }} 帧<template v-if="debug.dropped"> · 丢弃 {{ debug.dropped }}</template></span>
      </div>
      <div class="flex items-center gap-1.5" @pointerdown.stop>
        <UiButton size="sm" variant="secondary" data-test="debug-clear" @click="debug.clearBuffer()">清空</UiButton>
        <UiButton size="sm" variant="secondary" aria-label="关闭调试面板" @click="close">关闭</UiButton>
      </div>
    </header>

    <!-- Zone B filters: channel + identity keyword + event type -->
    <div class="debug-filters" data-test="debug-filters">
      <UiSelect
        :model-value="debug.filter.channel"
        :options="channels"
        size="sm"
        class="w-44"
        data-test="debug-filter-channel"
        @update:model-value="(v) => (debug.filter.channel = String(v))"
      />
      <UiInput
        v-model="debug.filter.keyword"
        type="text"
        size="sm"
        placeholder="会话 / chat / 实例 / run ID 或后端名…"
        clearable
        class="min-w-0 flex-1"
        data-test="debug-filter-keyword"
      />
      <UiInput
        v-model="debug.filter.eventType"
        type="text"
        size="sm"
        placeholder="事件类型，如 delta、turn…"
        clearable
        class="w-52"
        data-test="debug-filter-type"
      />
    </div>

    <!-- Zone C captured frames, oldest-first with bottom follow -->
    <div class="debug-list" data-test="debug-list" @scroll.passive="onListScroll" ref="listEl">
      <div v-if="entries.length === 0" class="debug-empty">
        <template v-if="debug.entries.length > 0">当前过滤条件下没有命中的帧</template>
        <template v-else>等待总线消息…（应用内产生会话/对话/任务事件后此处实时出现）</template>
      </div>
      <div
        v-for="entry in entries"
        :key="entry.seq"
        class="debug-row"
        data-test="debug-row"
        @click="toggleExpanded(entry)"
      >
        <div class="debug-row-line">
          <span class="debug-clock" :title="`#${entry.seq}`">{{ formatClock(entry.receivedAt) }}</span>
          <span
            class="debug-event"
            :class="{ 'debug-event-error': entry.eventType === 'error' }"
          >{{ entry.eventType }}</span>
          <span class="debug-channel">{{ entry.channel }}</span>
          <span class="debug-target" :title="entry.targetId">{{ entry.targetId || '—' }}</span>
          <span class="flex min-w-0 items-center gap-1">
            <span v-for="sid in entry.identity.sessionIds" :key="`s:${sid}`" class="debug-badge" :title="`sessionId: ${sid}`">{{ sid }}</span>
            <span v-for="cid in entry.identity.chatIds" :key="`c:${cid}`" class="debug-badge" :title="`chatId: ${cid}`">chat {{ cid }}</span>
            <span v-for="rid in entry.identity.runIds" :key="`r:${rid}`" class="debug-badge" :title="`runId: ${rid}`">run {{ rid }}</span>
            <span v-for="iid in entry.identity.instanceIds" :key="`i:${iid}`" class="debug-badge" :title="`instanceId: ${iid}`">{{ iid }}</span>
            <span v-for="backend in entry.identity.backends" :key="`b:${backend}`" class="debug-badge debug-badge-backend" :title="`backend: ${backend}`">{{ backend }}</span>
          </span>
        </div>
        <pre v-if="expandedSeq === entry.seq" class="debug-json" data-test="debug-json">{{ entryPayloadText(entry) }}</pre>
      </div>
    </div>

    <!-- Footer semantics reminder -->
    <footer class="debug-foot">
      仅捕获面板打开期间推送到本浏览器的总线帧 · 关闭即停止捕获并清空
    </footer>
  </div>
</template>

<style scoped>
.debug-panel {
  position: fixed;
  z-index: 64;
  display: flex;
  flex-direction: column;
  box-shadow: 0 18px 48px rgba(15, 23, 42, 0.28);
  border-radius: calc(var(--radius) * 1.5);
  overflow: hidden;
  background: var(--color-surface, #14171f);
  border: 1px solid var(--color-border, #222631);
}

.debug-panel.dragging-panel {
  user-select: none;
}

.debug-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 7px 10px;
  border-bottom: 1px solid var(--color-border, #222631);
  cursor: grab;
  touch-action: none;
  user-select: none;
}

.debug-head-title {
  font-size: var(--text-size-13);;
  font-weight: 600;
  color: var(--color-text, #e5e7eb);
}

.debug-head-status {
  font-size: var(--text-size-11);;
  color: var(--color-text-muted, #8b929e);
  border: 1px solid var(--color-border, #222631);
  border-radius: var(--radius, 4px);
  padding: 1px 6px;
}

.debug-head-status.status-error,
.debug-head-status.status-error.status-reconnecting {
  color: #ef4444;
  border-color: rgba(239, 68, 68, 0.4);
}

.debug-head-count {
  font-size: var(--text-size-11);;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-muted, #8b929e);
}

/* Filters: one compact bar, native to the Data Terminal dense rules. */
.debug-filters {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  border-bottom: 1px solid var(--color-border, #222631);
}

.debug-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  font-variant-numeric: tabular-nums;
}

.debug-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 24px;
  font-size: var(--text-size-12);;
  color: var(--color-text-muted, #8b929e);
  text-align: center;
}

.debug-row {
  border-bottom: 1px solid var(--color-border, #222631);
  cursor: pointer;
}

.debug-row:hover {
  background: var(--color-surface-muted);
}

.debug-row-line {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  font-size: var(--text-size-12);;
  min-width: 0;
}

.debug-clock {
  flex-shrink: 0;
  color: var(--color-text-muted, #8b929e);
  font-size: var(--text-size-11);;
}

.debug-event {
  flex-shrink: 0;
  color: var(--color-text, #e5e7eb);
  font-weight: 600;
}

.debug-event-error {
  color: #ef4444;
}

.debug-channel {
  flex-shrink: 0;
  color: var(--color-text-muted, #8b929e);
}

.debug-target {
  flex-shrink: 0;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-muted, #8b929e);
}

.debug-badge {
  display: inline-flex;
  align-items: center;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 0 5px;
  border: 1px solid var(--color-border, #222631);
  border-radius: var(--radius, 4px);
  font-size: var(--text-size-11);;
  color: var(--color-text-muted, #8b929e);
  height: 16px;
}

.debug-badge-backend {
  color: var(--color-accent, #4a8ad6);
  border-color: rgba(74, 138, 214, 0.4);
}

.debug-json {
  margin: 0;
  padding: 8px 10px 10px 10px;
  border-top: 1px dashed var(--color-border, #222631);
  background: var(--color-bg);
  color: var(--color-text, #e5e7eb);
  font-family: var(--font-mono, ui-monospace, 'Google Sans Code', monospace);
  font-size: var(--text-size-12);;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 320px;
  overflow: auto;
}

.debug-foot {
  padding: 5px 10px;
  border-top: 1px solid var(--color-border, #222631);
  font-size: var(--text-size-11);;
  color: var(--color-text-muted, #8b929e);
}
</style>
