<script setup lang="ts">
// Leftmost category rail. The project and pure-chat categories are the two primary
// work areas, so chat sits directly below the project entry; the remaining centers
// follow. Every category is a route link so the rail is always an exit from the
// current area: the
// settings center hides the project sidebar, and without a project link here the
// user would be trapped in /settings. The footer links to the settings center and
// logs out; collapsing the middle column is not the rail's job any more — the shell
// renders a handle on the rail's right edge for that (see AppShell's
// .rail-edge-toggle), which keeps a single affordance for both pinned and floating
// columns. The project category depends on that column, so activating it
// still asks the shell to show the column again.
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { tabKey } from '../../stores/openTabs'
import { useOpenTabsStore } from '../../stores/openTabs'
import { useDraftsStore } from '../../stores/drafts'

const emit = defineEmits<{
  logout: []
  // A rail entry that needs the project/task column asks the shell to show it,
  // so a collapsed column comes back in one click even when the entry is the
  // one already active (no route change to react to).
  expandSidebar: []
}>()

const route = useRoute()
const openTabs = useOpenTabsStore()
const draftsStore = useDraftsStore()

// The org routes declare `meta.org` or start with /org.
// The tasks routes declare `meta.tasks` or start with /tasks.
// The notes routes declare `meta.notes` or start with /notes.
// The engines routes declare `meta.engines` or start with /engines.
// The settings routes declare `meta.settings`.
const orgActive = computed(() => route.meta.org === true || route.path.startsWith('/org'))
const tasksActive = computed(() => route.meta.tasks === true || route.path.startsWith('/tasks'))
const notesActive = computed(() => route.meta.notes === true || route.path.startsWith('/notes'))
const enginesActive = computed(() => route.meta.engines === true || route.path.startsWith('/engines'))
const settingsActive = computed(() => route.meta.settings === true)
const projectActive = computed(
  () =>
    !settingsActive.value &&
    !enginesActive.value &&
    !orgActive.value &&
    !tasksActive.value &&
    !notesActive.value,
)

const projectDestination = computed(() => {
  // Only restore to tabs that actually belong to the workspace area.
  const workspaceTabs = openTabs.tabs.filter(
    (tab) =>
      tab.kind === 'session' ||
      tab.kind === 'new-session' ||
      tab.kind === 'projects-management',
  )
  if (workspaceTabs.length > 0) {
    const preferred =
      (openTabs.lastActiveWorkspaceKey || openTabs.lastActiveKey
        ? workspaceTabs.find(
            (tab) =>
              tabKey(tab) === openTabs.lastActiveWorkspaceKey ||
              tabKey(tab) === openTabs.lastActiveKey,
          )
        : null) ?? workspaceTabs[workspaceTabs.length - 1]

    if (preferred.kind === 'projects-management') {
      return { name: 'projects-management' }
    }
    if (preferred.kind === 'new-session') {
      const draft = draftsStore.newSessionDraft
      if (draft.projectId) {
        return { name: 'session-new', params: { projectId: String(draft.projectId) } }
      }
      return { name: 'session-new-global' }
    }
    if (preferred.kind === 'session') {
      return { name: 'session', params: { projectId: String(preferred.projectId), sessionId: preferred.sessionId } }
    }
  }
  return { name: 'home' }
})

function onProjectClick(event: MouseEvent) {
  emit('expandSidebar')
  if (projectActive.value) {
    event.preventDefault()
  }
}

function onOrgClick(event: MouseEvent) {
  emit('expandSidebar')
  if (orgActive.value) {
    event.preventDefault()
  }
}

function onTasksClick(event: MouseEvent) {
  emit('expandSidebar')
  if (tasksActive.value) {
    event.preventDefault()
  }
}

function onNotesClick(event: MouseEvent) {
  emit('expandSidebar')
  if (notesActive.value) {
    event.preventDefault()
  }
}

function onEnginesClick() {
  emit('expandSidebar')
}

function onSettingsClick() {
  emit('expandSidebar')
}
</script>

<template>
  <nav class="category-rail" aria-label="大类">
    <div class="rail-top">
      <RouterLink
        class="category"
        :class="{ active: projectActive }"
        title="项目"
        :aria-current="projectActive ? 'page' : undefined"
        :to="projectDestination"
        @click="onProjectClick"
      >
        <svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
          <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
        </svg>
        <span>项目</span>
      </RouterLink>
      <RouterLink
        class="category"
        :class="{ active: orgActive }"
        title="组织"
        :aria-current="orgActive ? 'page' : undefined"
        :to="{ name: 'org-prompts' }"
        @click="onOrgClick"
      >
        <svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="9" y="3" width="6" height="4" rx="1" />
          <path d="M12 7v4" />
          <path d="M5 11h14" />
          <path d="M5 11v3" />
          <path d="M19 11v3" />
          <rect x="2" y="14" width="6" height="4" rx="1" />
          <rect x="16" y="14" width="6" height="4" rx="1" />
          <path d="M12 11v3" />
          <rect x="9" y="14" width="6" height="4" rx="1" />
        </svg>
        <span>组织</span>
      </RouterLink>
      <RouterLink
        class="category"
        :class="{ active: tasksActive }"
        title="任务管理"
        :aria-current="tasksActive ? 'page' : undefined"
        :to="{ name: 'tasks' }"
        @click="onTasksClick"
      >
        <svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
          <path d="M9 11l3 3L22 4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
          <path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        <span>任务</span>
      </RouterLink>
      <RouterLink
        class="category"
        :class="{ active: notesActive }"
        title="笔记备忘"
        :aria-current="notesActive ? 'page' : undefined"
        :to="{ name: 'notes' }"
        @click="onNotesClick"
      >
        <svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
          <polyline points="14 2 14 8 20 8" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
          <line x1="16" y1="13" x2="8" y2="13" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
          <line x1="16" y1="17" x2="8" y2="17" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
          <polyline points="10 9 9 9 8 9" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
        </svg>
        <span>笔记</span>
      </RouterLink>
    </div>

    <div class="rail-bottom">
      <div class="rail-divider" aria-hidden="true"></div>
      <RouterLink
        class="footer-btn engines"
        title="引擎"
        :aria-current="enginesActive ? 'page' : undefined"
        :to="{ name: 'engines-assistant' }"
        @click="onEnginesClick"
      >
        <svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z" />
          <path d="M12 15l-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z" />
          <path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 0 5 0" />
          <path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5" />
        </svg>
        <span>引擎</span>
      </RouterLink>
      <RouterLink
        class="footer-btn settings"
        title="设置"
        :aria-current="settingsActive ? 'page' : undefined"
        :to="{ name: 'settings-appearance' }"
        @click="onSettingsClick"
      >
        <svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
          <circle cx="12" cy="12" r="3" />
          <path
            d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1.08-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1.08 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
          />
        </svg>
        <span>设置</span>
      </RouterLink>
      <button type="button" class="footer-btn logout" title="退出登录" @click="emit('logout')">
        <svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
          <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
          <polyline points="16 17 21 12 16 7" />
          <line x1="21" y1="12" x2="9" y2="12" />
        </svg>
        <span>退出</span>
      </button>
    </div>
  </nav>
</template>

<style scoped>
.category-rail {
  position: relative;
  z-index: 60;
  background: var(--rail-bg);
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  align-items: center;
  height: 100%;
  min-height: 0;
  box-sizing: border-box;
  padding: calc(var(--space-unit) * 1.5) 0;
  overflow: hidden;
  user-select: none;
}

.rail-top,
.rail-bottom {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: calc(var(--space-unit) * 0.75);
  width: 100%;
}

.rail-top {
  overflow-y: auto;
  scrollbar-width: none;
}

.rail-top::-webkit-scrollbar {
  display: none;
}

.rail-bottom {
  flex-shrink: 0;
}

.rail-divider {
  width: 24px;
  height: 1px;
  background: rgba(255, 255, 255, 0.12);
  margin: 4px 0 6px;
}

.category,
.footer-btn {
  width: 40px;
  padding: 6px 0 5px;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  color: var(--rail-text);
  font-size: 0.6875rem;
  font-weight: 500;
  line-height: 1.1;
  cursor: pointer;
  text-align: center;
  text-decoration: none;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  position: relative;
  transition: color 0.15s ease, background-color 0.15s ease, border-color 0.15s ease;
}

.category:hover,
.footer-btn:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #fff;
}

.category.active {
  background: var(--rail-active);
  color: #ffffff;
}

.category.active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 18px;
  border-radius: 0 2px 2px 0;
  background: var(--color-accent, #3b82f6);
}

.footer-btn.logout:hover {
  background: rgba(239, 68, 68, 0.15);
  color: #fca5a5;
}

.footer-btn.collapse {
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.footer-btn.collapse:hover {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: #fff;
}

.rail-icon {
  width: 18px;
  height: 18px;
  display: block;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
  transition: transform 0.15s ease;
}

.category:hover .rail-icon {
  transform: scale(1.05);
}

.category.reserved,
.category:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
</style>
