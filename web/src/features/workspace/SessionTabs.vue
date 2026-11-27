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
import BackendIcon from '../../shared/components/BackendIcon.vue'
import RunningIndicator from '../../shared/components/RunningIndicator.vue'
import WorkbenchTabStrip from '../../shared/components/WorkbenchTabStrip.vue'
import { useActivityPane } from '../../shared/composables/useActivityPane'
import { useWorkspaceDockStore } from '../../stores/workspaceDock'

const { previewCollapsed, setPreviewCollapsed } = useActivityPane()
let dockStore: ReturnType<typeof useWorkspaceDockStore> | null = null
try {
  dockStore = useWorkspaceDockStore()
} catch {
  dockStore = null
}

const isPreviewCollapsed = computed(() =>
  dockStore ? dockStore.previewCollapsed : previewCollapsed.value,
)

function togglePreviewPane() {
  const next = !isPreviewCollapsed.value
  dockStore?.setPreviewCollapsed(next)
  setPreviewCollapsed(next)
}

const props = withDefaults(
  defineProps<{
    tabs: OpenTab[]
    activeKey: string | null
    projectNames?: Record<number, string>
    // Session ids whose turn is running. The strip marks each with an animated
    // icon before its label, the same marker the session tree uses.
    runningSessionIds?: string[]
    newTabTitle?: string
  }>(),
  {
    newTabTitle: '新建会话',
  },
)

const emit = defineEmits<{
  select: [tab: OpenTab]
  close: [tab: OpenTab]
  togglePin: [tab: OpenTab]
  closeOthers: [tab: OpenTab]
  closeAll: []
  newTab: []
}>()

function label(tab: OpenTab): string {
  if (tab.kind === 'new-session') return 'new'
  if (tab.kind === 'new-chat') return '新对话'
  if (tab.kind === 'platform-settings') return '设置'
  if (tab.kind === 'task') return tab.title !== '' ? tab.title : `任务 ${tab.taskId}`
  if (tab.kind === 'chat') return tab.title !== '' ? tab.title : '未命名对话'
  return tab.title !== '' ? tab.title : tab.sessionId.slice(0, 8)
}

function titleOf(tab: OpenTab): string {
  if (tab.kind === 'new-session') return '新建会话'
  if (tab.kind === 'new-chat') return '新对话'
  if (tab.kind === 'platform-settings') return '平台设置'
  if (tab.kind === 'task') return label(tab)
  if (tab.kind === 'chat') return label(tab)
  const project = props.projectNames?.[tab.projectId]
  const name = label(tab)
  return project ? `${name} · ${project}` : name
}

function isSettings(tab: OpenTab): boolean {
  return tab.kind === 'platform-settings'
}

// The strip's close control label and the pin metadata branch on kind only to
// talk about the right thing; identity stays a tab key everywhere.
function closeLabel(tab: OpenTab): string {
  if (isSettings(tab)) return '关闭设置'
  if (tab.kind === 'task') return `关闭任务：${label(tab)}`
  if (tab.kind === 'new-session') return '关闭新建会话'
  if (tab.kind === 'new-chat') return '关闭新对话'
  if (tab.kind === 'chat') return `关闭对话：${label(tab)}`
  return `关闭会话：${label(tab)}`
}

// The agent behind a session tab, shown as its logo. Other kinds, and a session
// whose backend is not known yet, show no logo.
function backendOf(tab: OpenTab): string {
  return tab.kind === 'session' ? (tab.backend ?? '') : ''
}

function isPinned(tab: OpenTab): boolean {
  return tab.pinned === true
}

// A session or chat tab whose turn is running: the strip shows the animated marker in
// place of the plain dot, so running conversations are visible without opening them.
function isRunning(tab: OpenTab): boolean {
  if (tab.kind === 'session') {
    return (props.runningSessionIds ?? []).includes(tab.sessionId)
  }
  if (tab.kind === 'chat') {
    return (
      (props.runningSessionIds ?? []).includes(String(tab.chatId)) ||
      (props.runningSessionIds ?? []).includes(`chat:${tab.chatId}`)
    )
  }
  return false
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
  <div v-if="props.tabs.length > 0" class="session-tabs-host" @keydown="onKeydown">
    <WorkbenchTabStrip aria-label="已打开的页面" :active-key="props.activeKey">
      <div
        v-for="tab in props.tabs"
        :key="tabKey(tab)"
        class="wb-tab tab"
        :class="{ active: tabKey(tab) === props.activeKey, pinned: isPinned(tab) }"
        role="tab"
        :aria-selected="tabKey(tab) === props.activeKey"
        :title="titleOf(tab)"
        data-test="open-tab"
        :data-tab-key="tabKey(tab)"
        :data-pinned="isPinned(tab) ? 'true' : undefined"
        :data-session-id="tab.kind === 'session' ? tab.sessionId : undefined"
        :data-chat-id="tab.kind === 'chat' ? tab.chatId : undefined"
        @contextmenu.prevent="openMenu(tab, $event)"
        @auxclick="onAuxClick(tab, $event)"
        @mousedown.middle.prevent
      >
        <button type="button" class="wb-tab-select tab-select" @click="emit('select', tab)">
          <BackendIcon v-if="backendOf(tab)" :backend="backendOf(tab)" size="sm" class="tab-logo" />
          <span v-if="isPinned(tab)" class="tab-pin" aria-hidden="true">
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
              <line x1="8" y1="11" x2="8" y2="15" />
              <path d="M4 11h8l-1.5-6h-5L4 11z" />
              <line x1="5.5" y1="2" x2="10.5" y2="2" />
            </svg>
          </span>
          <span v-else-if="isSettings(tab)" class="tab-icon" aria-hidden="true">
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.5">
              <circle cx="8" cy="8" r="2.5" />
              <path d="M8 1v2M8 13v2M1 8h2M13 8h2M3.05 3.05l1.41 1.41M11.54 11.54l1.41 1.41M3.05 12.95l1.41-1.41M11.54 4.46l1.41-1.41" />
            </svg>
          </span>
          <span v-else-if="tab.kind === 'task'" class="tab-icon" aria-hidden="true">
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.5">
              <rect x="3" y="3" width="10" height="11" rx="1" />
              <path d="M6 1.5h4v2H6v-2zM5.5 7h5M5.5 10h3" />
            </svg>
          </span>
          <span v-else-if="tab.kind === 'new-session' || tab.kind === 'new-chat'" class="tab-icon" aria-hidden="true">
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M11 2.5l2.5 2.5L5 13.5H2.5V11L11 2.5z" />
            </svg>
          </span>
          <span v-else-if="!isRunning(tab) && !backendOf(tab)" class="tab-dot" aria-hidden="true"></span>
          <RunningIndicator v-if="isRunning(tab)" />
          <span class="wb-tab-label tab-label">{{ label(tab) }}</span>
        </button>
        <button
          v-if="!isPinned(tab)"
          type="button"
          class="wb-tab-close tab-close"
          :aria-label="closeLabel(tab)"
          title="关闭"
          data-test="close-tab"
          @click.stop="emit('close', tab)"
        >
          <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <line x1="3" y1="3" x2="13" y2="13" />
            <line x1="13" y1="3" x2="3" y2="13" />
          </svg>
        </button>
      </div>

      <template #trailing>
        <button
          type="button"
          class="wb-tab-action-btn tab-add-btn"
          :title="props.newTabTitle"
          :aria-label="props.newTabTitle"
          data-test="tab-add-session"
          @click="emit('newTab')"
        >
          +
        </button>
        <button
          type="button"
          class="wb-tab-action-btn layout-toggle-btn"
          :class="{ 'is-active': !isPreviewCollapsed }"
          data-test="toggle-layout-btn"
          data-test-preview="toggle-preview-btn"
          :aria-pressed="!isPreviewCollapsed ? 'true' : 'false'"
          :title="isPreviewCollapsed ? '展开右侧工作区面板' : '收起右侧工作区面板'"
          :aria-label="isPreviewCollapsed ? '展开右侧工作区面板' : '收起右侧工作区面板'"
          @click="togglePreviewPane"
        >
          <svg class="action-icon" viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="1.8">
            <rect x="3" y="3" width="18" height="18" rx="2" />
            <line x1="15" y1="3" x2="15" y2="21" stroke-width="1.8" />
          </svg>
        </button>
      </template>
    </WorkbenchTabStrip>

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
.session-tabs-host {
  flex-shrink: 0;
}

.tab-add-btn {
  font-size: var(--text-size-14);
  font-weight: 400;
  line-height: 1;
}

.tab-dot {
  width: 6px;
  height: 6px;
  border-radius: 999px;
  background: var(--color-border-strong);
  flex-shrink: 0;
}

.tab-logo {
  margin-right: 2px;
}

.tab-icon,
.tab-pin {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--color-text-muted);
}

.tab.active .tab-dot {
  background: var(--color-accent);
}

.tab.active .tab-icon,
.tab.active .tab-pin {
  color: var(--color-accent);
}

.tab-menu-backdrop {
  position: fixed;
  inset: 0;
  z-index: 40;
}

.tab-menu {
  position: fixed;
  z-index: 41;
  min-width: 10.5rem;
  display: flex;
  flex-direction: column;
  padding: 6px;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: var(--color-surface);
  box-shadow: 0 10px 25px -3px rgba(0, 0, 0, 0.15), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
}

.tab-menu button {
  border: none;
  background: transparent;
  text-align: left;
  padding: 6px 10px;
  border-radius: 5px;
  cursor: pointer;
  font: inherit;
  font-size: var(--text-size-12);
  color: var(--color-text);
  white-space: nowrap;
  transition: background-color 0.12s ease;
}

.tab-menu button:hover:not(:disabled) {
  background: var(--color-surface-muted);
}

.tab-menu button:disabled {
  color: var(--color-text-muted);
  cursor: default;
  opacity: 0.5;
}
</style>
