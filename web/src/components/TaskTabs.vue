<script setup lang="ts">
// Tab bar for open tasks. One tab per opened session; the active tab follows
// the current route. Switching navigates, closing drops the tab (its stream
// and cached transcript are released by the caller).
import type { OpenTab } from '../stores/openTabs'

const props = defineProps<{
  tabs: OpenTab[]
  activeSessionId: string | null
  projectNames?: Record<number, string>
}>()

const emit = defineEmits<{
  select: [tab: OpenTab]
  close: [tab: OpenTab]
}>()

function label(tab: OpenTab): string {
  return tab.title !== '' ? tab.title : tab.sessionId.slice(0, 8)
}

function titleOf(tab: OpenTab): string {
  const project = props.projectNames?.[tab.projectId]
  const name = label(tab)
  return project ? `${name} · ${project}` : name
}
</script>

<template>
  <div v-if="props.tabs.length > 0" class="task-tabs" role="tablist" aria-label="已打开的任务">
    <div
      v-for="tab in props.tabs"
      :key="tab.sessionId"
      class="tab"
      :class="{ active: tab.sessionId === props.activeSessionId }"
      role="tab"
      :aria-selected="tab.sessionId === props.activeSessionId"
      :title="titleOf(tab)"
      data-test="open-tab"
      :data-session-id="tab.sessionId"
    >
      <button type="button" class="tab-select" @click="emit('select', tab)">
        <span class="tab-dot" aria-hidden="true"></span>
        <span class="tab-label">{{ label(tab) }}</span>
      </button>
      <button
        type="button"
        class="tab-close"
        :aria-label="`关闭任务：${label(tab)}`"
        title="关闭"
        data-test="close-tab"
        @click.stop="emit('close', tab)"
      >
        ×
      </button>
    </div>
  </div>
</template>

<style scoped>
.task-tabs {
  display: flex;
  align-items: stretch;
  gap: 0.35rem;
  overflow-x: auto;
  padding: 0.35rem 0.35rem 0;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface);
  flex-shrink: 0;
}

.tab {
  display: flex;
  align-items: center;
  gap: 0.15rem;
  max-width: 12rem;
  border: 1px solid transparent;
  border-bottom: none;
  border-radius: 8px 8px 0 0;
  padding: 0.25rem 0.25rem 0.3rem 0.5rem;
  background: transparent;
  white-space: nowrap;
}

.tab.active {
  background: var(--color-bg);
  border-color: var(--color-border);
}

.tab-select {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  min-width: 0;
  border: none;
  background: transparent;
  cursor: pointer;
  font: inherit;
  font-size: 0.85rem;
  color: var(--color-text-muted);
  padding: 0.15rem 0.2rem;
}

.tab.active .tab-select {
  color: var(--color-text);
  font-weight: 600;
}

.tab-dot {
  width: 0.45rem;
  height: 0.45rem;
  border-radius: 999px;
  background: var(--color-border-strong);
  flex-shrink: 0;
}

.tab.active .tab-dot {
  background: var(--color-primary);
}

.tab-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tab-close {
  border: none;
  background: transparent;
  cursor: pointer;
  font: inherit;
  font-size: 0.95rem;
  line-height: 1;
  padding: 0.15rem 0.35rem;
  border-radius: 6px;
  color: var(--color-text-muted);
}

.tab-close:hover {
  background: var(--color-surface-muted);
  color: var(--color-text);
}
</style>
