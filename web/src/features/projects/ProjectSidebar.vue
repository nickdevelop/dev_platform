<script setup lang="ts">
// Column 2 for the project workspace: project tree, search, filter and management actions.
// Follows design-taste-frontend (Data Terminal / App Screens) compact three-zone standard:
//   Zone A: Standardized 26px actions (打开项目 / 新建项目) with 1.5px fine-line vector icons
//   Zone B: Compact search bar with inset 1.5px magnifier icon and task-session filter toggle
//   Zone C: Tree of projects with their respective agent session items
import { ref } from 'vue'
import ProjectTree from './ProjectTree.vue'

defineProps<{
  activeProjectId: number | null
  activeSessionId: string | null
}>()

const emit = defineEmits<{
  manage: [kind: 'register' | 'create']
  'open-settings': [projectId: number]
  'select-session': [projectId: number, sessionId: string]
  'new-session': [projectId: number]
  'delete-session': [projectId: number, sessionId: string]
}>()

const search = ref('')

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
        class="action-btn register"
        title="打开已有项目目录"
        @click="emit('manage', 'register')"
      >
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M2 13h12M2 3h4l2 2h6v8H2V3z"/>
        </svg>
        <span>打开项目</span>
      </button>
      <button
        type="button"
        class="action-btn create"
        title="新建并初始化项目"
        @click="emit('manage', 'create')"
      >
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <line x1="8" y1="3" x2="8" y2="13" />
          <line x1="3" y1="8" x2="13" y2="8" />
        </svg>
        <span>新建项目</span>
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
