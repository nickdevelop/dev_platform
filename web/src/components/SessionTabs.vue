<script setup lang="ts">
// Tab bar for the workbench. Two kinds of tab share one strip — an open session and
// a project's settings page — because to the user they are the same thing: pages
// they have open. Identity, activation and closing all go through the tab's key,
// so the strip never branches on kind except to label and decorate.
import { tabKey, type OpenTab } from '../stores/openTabs'

const props = defineProps<{
  tabs: OpenTab[]
  activeKey: string | null
  projectNames?: Record<number, string>
}>()

const emit = defineEmits<{
  select: [tab: OpenTab]
  close: [tab: OpenTab]
}>()

function label(tab: OpenTab): string {
  if (tab.kind === 'project-settings') {
    const project = props.projectNames?.[tab.projectId]
    return project ? `${project} · 设置` : '项目设置'
  }
  return tab.title !== '' ? tab.title : tab.sessionId.slice(0, 8)
}

function titleOf(tab: OpenTab): string {
  if (tab.kind === 'project-settings') return label(tab)
  const project = props.projectNames?.[tab.projectId]
  const name = label(tab)
  return project ? `${name} · ${project}` : name
}

function isSettings(tab: OpenTab): boolean {
  return tab.kind === 'project-settings'
}
</script>

<template>
  <div v-if="props.tabs.length > 0" class="session-tabs" role="tablist" aria-label="已打开的会话">
    <div
      v-for="tab in props.tabs"
      :key="tabKey(tab)"
      class="tab"
      :class="{ active: tabKey(tab) === props.activeKey }"
      role="tab"
      :aria-selected="tabKey(tab) === props.activeKey"
      :title="titleOf(tab)"
      data-test="open-tab"
      :data-tab-key="tabKey(tab)"
      :data-session-id="tab.kind === 'session' ? tab.sessionId : undefined"
    >
      <button type="button" class="tab-select" @click="emit('select', tab)">
        <span v-if="isSettings(tab)" class="tab-icon" aria-hidden="true">⚙</span>
        <span v-else class="tab-dot" aria-hidden="true"></span>
        <span class="tab-label">{{ label(tab) }}</span>
      </button>
      <button
        type="button"
        class="tab-close"
        :aria-label="isSettings(tab) ? '关闭项目设置' : `关闭会话：${label(tab)}`"
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
.session-tabs {
  display: flex;
  align-items: stretch;
  gap: 0.35rem;
  overflow-x: auto;
  /* Pinned to the very top of the pane and full width: no top inset. */
  padding: 0 0.35rem;
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

.tab-icon {
  flex-shrink: 0;
  font-size: 0.8rem;
  color: var(--color-text-muted);
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
