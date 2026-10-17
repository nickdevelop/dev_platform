<script setup lang="ts">
// Leftmost category rail. Only the project category is available; the chat
// category is reserved for a later release and is shown disabled. Every category
// is a route link so the rail is always an exit from the current area: the
// settings center hides the project sidebar, and without a project link here the
// user would be trapped in /settings. The footer links to the settings center,
// logs out, and — at the very bottom — collapses the project/task column so a
// conversation can take the full width; the shell owns that column's visibility
// and its persisted choice, so the rail only reports the click and reflects the
// current state. The project category depends on that column, so activating it
// asks the shell to show the column again.
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'

defineProps<{
  // Whether the middle project/task column is currently collapsed.
  sidebarCollapsed?: boolean
  // The settings center has no project column, so the toggle is hidden there.
  sidebarCollapsible?: boolean
}>()

const emit = defineEmits<{
  logout: []
  toggleSidebar: []
  // A rail entry that needs the project/task column asks the shell to show it,
  // so a collapsed column comes back in one click even when the entry is the
  // one already active (no route change to react to).
  expandSidebar: []
}>()

const route = useRoute()
// The tasks routes declare `meta.tasks` or start with /tasks.
// The settings routes declare `meta.settings`.
const tasksActive = computed(() => route.meta.tasks === true || route.path.startsWith('/tasks'))
const settingsActive = computed(() => route.meta.settings === true)
const projectActive = computed(() => !settingsActive.value && !tasksActive.value)

function onProjectClick(event: MouseEvent) {
  emit('expandSidebar')
  if (projectActive.value) {
    event.preventDefault()
  }
}

function onTasksClick(event: MouseEvent) {
  emit('expandSidebar')
  if (tasksActive.value) {
    event.preventDefault()
  }
}
</script>

<template>
  <nav class="category-rail" aria-label="大类">
    <RouterLink
      class="category"
      :class="{ active: projectActive }"
      title="项目"
      :aria-current="projectActive ? 'page' : undefined"
      :to="{ name: 'home' }"
      @click="onProjectClick"
    >
      <svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
      </svg>
      <span>项目</span>
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
    <button type="button" class="category reserved" title="Chat（预留）" disabled>
      <svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <path
          d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8z"
        />
      </svg>
      <span>Chat</span>
    </button>
    <div class="spacer"></div>
    <RouterLink
      class="footer-btn settings"
      title="设置"
      :aria-current="settingsActive ? 'page' : undefined"
      :to="{ name: 'settings-appearance' }"
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
    <button
      v-if="sidebarCollapsible !== false"
      type="button"
      class="footer-btn collapse"
      data-test="sidebar-toggle"
      :title="sidebarCollapsed ? '展开项目导航' : '收起项目导航'"
      :aria-label="sidebarCollapsed ? '展开项目导航' : '收起项目导航'"
      :aria-expanded="(!sidebarCollapsed).toString()"
      @click="emit('toggleSidebar')"
    >
      <svg
        v-if="sidebarCollapsed"
        class="rail-icon"
        viewBox="0 0 24 24"
        aria-hidden="true"
        focusable="false"
      >
        <polyline points="13 17 18 12 13 7" />
        <polyline points="6 17 11 12 6 7" />
      </svg>
      <svg v-else class="rail-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <polyline points="11 17 6 12 11 7" />
        <polyline points="18 17 13 12 18 7" />
      </svg>
      <span>{{ sidebarCollapsed ? '展开' : '收起' }}</span>
    </button>
  </nav>
</template>

<style scoped>
.category-rail {
  background: var(--rail-bg);
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: calc(var(--space-unit) * 2.000) 0;
  gap: calc(var(--space-unit) * 1.333);
}

.category,
.footer-btn {
  width: 44px;
  padding: calc(var(--space-unit) * 1.000) 0;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--rail-text);
  font-size: 0.857rem;
  cursor: pointer;
  text-align: center;
  text-decoration: none;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: calc(var(--space-unit) * 0.500);
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
}

.category.active {
  background: var(--rail-active);
  color: var(--color-surface);
}

.category.reserved,
.category:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.spacer {
  flex: 1;
}
</style>
