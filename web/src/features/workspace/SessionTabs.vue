<script setup lang="ts">
// Tab bar for the workbench. Two kinds of tab share one strip — an open session and
// a project's settings page — because to the user they are the same thing: pages
// they have open. Identity, activation and closing all go through the tab's key,
// so the strip never branches on kind except to label and decorate.
//
// A tab can be closed with a middle click, or the strip's context menu: pin the
// tab, close it, close the other unpinned tabs, or close them all. Pinned tabs
// render no close control and ignore a middle click — they are the anchors the
// user pinned on purpose — and the store keeps them at the front.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { tabKey, type OpenTab } from '../../stores/openTabs'
import RunningIndicator from '../../shared/components/RunningIndicator.vue'

const props = defineProps<{
  tabs: OpenTab[]
  activeKey: string | null
  projectNames?: Record<number, string>
  // Session ids whose turn is running. The strip marks each with an animated
  // icon before its label, the same marker the session tree uses.
  runningSessionIds?: string[]
}>()

const emit = defineEmits<{
  select: [tab: OpenTab]
  close: [tab: OpenTab]
  togglePin: [tab: OpenTab]
  closeOthers: [tab: OpenTab]
  closeAll: []
  newTab: []
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

function isPinned(tab: OpenTab): boolean {
  return tab.pinned === true
}

// A session tab whose turn is running: the strip shows the animated marker in
// place of the plain dot, so running sessions are visible without opening them.
function isRunning(tab: OpenTab): boolean {
  return tab.kind === 'session' && (props.runningSessionIds ?? []).includes(tab.sessionId)
}

// A middle click on a tab closes it, the way every browser-like tab strip works.
// Pinned tabs are exempt; the click must not scroll the page either.
function onAuxClick(tab: OpenTab, event: MouseEvent) {
  if (event.button !== 1) return
  if (isPinned(tab)) return
  emit('close', tab)
}

// --- Context menu ---

const menu = ref<{ key: string; x: number; y: number } | null>(null)

const menuTab = computed(() => props.tabs.find((tab) => tabKey(tab) === menu.value?.key) ?? null)

function openMenu(tab: OpenTab, event: MouseEvent) {
  event.preventDefault()
  menu.value = { key: tabKey(tab), x: event.clientX, y: event.clientY }
}

function closeMenu() {
  menu.value = null
}

function onMenuPin() {
  const tab = menuTab.value
  if (tab) emit('togglePin', tab)
  closeMenu()
}

function onMenuClose() {
  const tab = menuTab.value
  if (tab && !isPinned(tab)) emit('close', tab)
  closeMenu()
}

function onMenuCloseOthers() {
  const tab = menuTab.value
  if (tab) emit('closeOthers', tab)
  closeMenu()
}

function onMenuCloseAll() {
  emit('closeAll')
  closeMenu()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') closeMenu()
}

// A menu left open while the window scrolls or resizes would point at the wrong
// tab, so both events dismiss it.
onMounted(() => {
  window.addEventListener('resize', closeMenu)
  window.addEventListener('scroll', closeMenu, true)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', closeMenu)
  window.removeEventListener('scroll', closeMenu, true)
})
</script>

<template>
  <div v-if="props.tabs.length > 0" class="session-tabs" role="tablist" aria-label="已打开的会话" @keydown="onKeydown">
    <div
      v-for="tab in props.tabs"
      :key="tabKey(tab)"
      class="tab"
      :class="{ active: tabKey(tab) === props.activeKey, pinned: isPinned(tab) }"
      role="tab"
      :aria-selected="tabKey(tab) === props.activeKey"
      :title="titleOf(tab)"
      data-test="open-tab"
      :data-tab-key="tabKey(tab)"
      :data-pinned="isPinned(tab) ? 'true' : undefined"
      :data-session-id="tab.kind === 'session' ? tab.sessionId : undefined"
      @contextmenu.prevent="openMenu(tab, $event)"
      @auxclick="onAuxClick(tab, $event)"
      @mousedown.middle.prevent
    >
      <button type="button" class="tab-select" @click="emit('select', tab)">
        <span v-if="isPinned(tab)" class="tab-pin" aria-hidden="true">📌</span>
        <span v-else-if="isSettings(tab)" class="tab-icon" aria-hidden="true">⚙</span>
        <span v-else-if="!isRunning(tab)" class="tab-dot" aria-hidden="true"></span>
        <RunningIndicator v-if="isRunning(tab)" />
        <span class="tab-label">{{ label(tab) }}</span>
      </button>
      <button
        v-if="!isPinned(tab)"
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

    <button
      type="button"
      class="tab-add-btn"
      title="新建会话"
      aria-label="新建会话"
      data-test="tab-add-session"
      @click="emit('newTab')"
    >
      +
    </button>

    <div
      v-if="menu"
      class="tab-menu-backdrop"
      data-test="tab-menu-backdrop"
      @click="closeMenu"
      @contextmenu.prevent="closeMenu"
    ></div>
    <div
      v-if="menu"
      class="tab-menu"
      data-test="tab-menu"
      role="menu"
      :style="{ left: `${menu.x}px`, top: `${menu.y}px` }"
    >
      <button type="button" role="menuitem" data-test="menu-toggle-pin" @click="onMenuPin">
        {{ menuTab && isPinned(menuTab) ? '取消固定' : '固定标签页' }}
      </button>
      <button
        type="button"
        role="menuitem"
        data-test="menu-close"
        :disabled="!menuTab || isPinned(menuTab)"
        @click="onMenuClose"
      >
        关闭标签页
      </button>
      <button type="button" role="menuitem" data-test="menu-close-others" @click="onMenuCloseOthers">
        关闭其他标签页
      </button>
      <button type="button" role="menuitem" data-test="menu-close-all" @click="onMenuCloseAll">
        关闭全部标签页
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

.tab-pin {
  flex-shrink: 0;
  font-size: 0.7rem;
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

.tab-menu-backdrop {
  position: fixed;
  inset: 0;
  z-index: 40;
}

.tab-menu {
  position: fixed;
  z-index: 41;
  min-width: 10rem;
  display: flex;
  flex-direction: column;
  padding: 0.25rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 8px;
  background: var(--color-surface);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.18);
}

.tab-menu button {
  border: none;
  background: transparent;
  text-align: left;
  padding: 0.35rem 0.6rem;
  border-radius: 6px;
  cursor: pointer;
  font: inherit;
  font-size: 0.85rem;
  color: var(--color-text);
  white-space: nowrap;
}

.tab-menu button:hover:not(:disabled) {
  background: var(--color-surface-muted);
}

.tab-menu button:disabled {
  color: var(--color-text-muted);
  cursor: default;
}

.tab-add-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 24px;
  width: 24px;
  margin-left: 6px;
  align-self: center;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  color: var(--color-text-muted);
  font-size: 1.1rem;
  line-height: 1;
  cursor: pointer;
  transition: all 0.15s ease;
}

.tab-add-btn:hover {
  background: var(--color-surface);
  border-color: var(--color-border);
  color: var(--color-text);
}
</style>
