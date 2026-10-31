<script setup lang="ts">
// Column 2 for the project workspace: project tree, search, filter and management actions.
// Follows design-taste-frontend (Data Terminal / App Screens) compact three-zone standard:
//   Zone A: Standardized 26px actions (新建项目 / 项目设置) with 1.5px fine-line vector icons
//   Zone B: Compact search bar with inset 1.5px magnifier icon and task-session filter toggle
//   Zone C: Tree of projects with their respective agent session items
import { ref } from 'vue'
import ProjectTree from './ProjectTree.vue'
import { useOpenTabsStore } from '../../stores/openTabs'

defineProps<{
  activeProjectId: number | null
  activeSessionId: string | null
}>()

const emit = defineEmits<{
  manage: [kind: 'register' | 'create']
  'open-settings': [projectId: number]
  'open-projects-management': []
  'select-session': [projectId: number, sessionId: string]
  'new-session': [projectId: number]
  'delete-session': [projectId: number, sessionId: string]
}>()

const openTabsStore = useOpenTabsStore()
const search = ref('')

function openProjectsSettings() {
  openTabsStore.openProjectsManagement()
  emit('open-projects-management')
}

const SHOW_TASK_SESSIONS_STORAGE_KEY = 'dev-platform-show-task-sessions'
const showTaskSessions = ref(false)
try {
  const saved = localStorage.getItem(SHOW_TASK_SESSIONS_STORAGE_KEY)
  if (saved !== null) {
    showTaskSessions.value = saved === 'true'
  }
} catch {
  showTaskSessions.value = false
}

function onShowTaskSessionsChange() {
  try {
    localStorage.setItem(SHOW_TASK_SESSIONS_STORAGE_KEY, String(showTaskSessions.value))
  } catch {
    // Persistence is best-effort only.
  }
}
</script>

<template>
  <div class="project-sidebar sidebar-panel" data-test="project-sidebar">
    <!-- Zone A: Dedicated actions row -->
    <div class="sidebar-actions">
      <button
        type="button"
        class="action-btn create flex-1"
        title="新建并初始化项目"
        data-test="btn-create-project"
        @click="emit('manage', 'create')"
      >
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <line x1="8" y1="3" x2="8" y2="13" />
          <line x1="3" y1="8" x2="13" y2="8" />
        </svg>
        <span>新建项目</span>
      </button>
      <button
        type="button"
        class="action-btn project-settings-btn"
        title="项目列表与显示设置"
        aria-label="项目列表与显示设置"
        data-test="btn-projects-management"
        @click="openProjectsSettings"
      >
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <circle cx="8" cy="8" r="2.5" />
          <path d="M8 1.5v1.5M8 13v1.5M1.5 8H3M13 8h1.5M3.4 3.4l1.1 1.1M11.5 11.5l1.1 1.1M3.4 12.6l1.1-1.1M11.5 4.5l1.1-1.1" />
        </svg>
        <span>设置</span>
      </button>
    </div>

    <!-- Zone B: Search input with inset magnifier and task filter -->
    <div class="sidebar-search-bar">
      <svg class="sidebar-search-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <circle cx="7" cy="7" r="4.5" />
        <line x1="10.5" y1="10.5" x2="14" y2="14" stroke-linecap="round" />
      </svg>
      <input
        v-model="search"
        class="search sidebar-search-input"
        type="search"
        placeholder="搜索项目或会话"
        aria-label="搜索项目或会话"
      />
      <button
        v-if="search"
        type="button"
        class="sidebar-search-clear"
        title="清空搜索"
        aria-label="清空搜索"
        @click="search = ''"
      >
        <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="4" y1="4" x2="12" y2="12" />
          <line x1="12" y1="4" x2="4" y2="12" />
        </svg>
      </button>
    </div>

    <div class="sidebar-filter-bar">
      <label class="filter-task-checkbox">
        <input
          v-model="showTaskSessions"
          type="checkbox"
          data-test="show-task-sessions"
          @change="onShowTaskSessionsChange"
        />
        <span>显示任务会话</span>
      </label>
    </div>

    <!-- Zone C: Project Tree list container -->
    <div class="tree">
      <ProjectTree
        :active-project-id="activeProjectId"
        :active-session-id="activeSessionId"
        :search="search"
        :show-task-sessions="showTaskSessions"
        @open-settings="emit('open-settings', $event)"
        @select-session="(pId, sId) => emit('select-session', pId, sId)"
        @new-session="emit('new-session', $event)"
        @delete-session="(pId, sId) => emit('delete-session', pId, sId)"
      />
    </div>
  </div>
</template>

<style scoped>
.project-sidebar {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.project-settings-btn {
  flex: 0 0 auto !important;
  padding: 0 8px;
}

.tree {
  box-sizing: border-box;
  overflow-y: auto;
  overflow-x: hidden;
  scrollbar-gutter: stable;
  padding: 4px 6px 16px;
  flex: 1;
}

.filter-task-checkbox {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  color: var(--color-text-muted);
  cursor: pointer;
  user-select: none;
}

.filter-task-checkbox input[type="checkbox"] {
  cursor: pointer;
  accent-color: var(--color-primary);
}

.filter-task-checkbox:hover {
  color: var(--color-text);
}
</style>
