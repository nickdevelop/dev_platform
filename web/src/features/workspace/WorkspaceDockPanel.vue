<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessageBox } from 'element-plus'
import { useWorkspaceDockStore, FILE_TREE_TAB_KEY } from '../../stores/workspaceDock'
import { useProjectsStore } from '../../stores/projects'
import { useWorktreesStore } from '../../stores/worktrees'
import { FileSidebar, FileViewer } from '../files'
import type { CodeCommentCard } from '../files/types'

const emit = defineEmits<{
  'add-comment': [card: CodeCommentCard]
}>()

const dockStore = useWorkspaceDockStore()
const projectsStore = useProjectsStore()
const worktreesStore = useWorktreesStore()

const dirtyMap = ref<Record<string, boolean>>({})

function handleDirtyChange(tabKey: string, dirty: boolean) {
  if (dirty) {
    dirtyMap.value = { ...dirtyMap.value, [tabKey]: true }
  } else {
    const next = { ...dirtyMap.value }
    delete next[tabKey]
    dirtyMap.value = next
  }
}

async function requestCloseTab(tabKey: string, event?: MouseEvent) {
  event?.stopPropagation()
  if (dirtyMap.value[tabKey]) {
    try {
      await ElMessageBox.confirm(
        '当前文件有尚未保存的修改，确定要放弃这些修改并关闭标签吗？',
        '放弃未保存的修改',
        {
          confirmButtonText: '放弃修改',
          cancelButtonText: '继续编辑',
          confirmButtonClass: 'el-button--danger',
          type: 'warning',
        },
      )
    } catch {
      return
    }
    delete dirtyMap.value[tabKey]
  }
  dockStore.closeTab(tabKey)
}

function getProjectName(projectId: number): string {
  const p = projectsStore.byId(projectId)
  return p ? p.name : `项目 ${projectId}`
}

function getScopeBadge(projectId: number, worktreeId?: number): string {
  const projName = getProjectName(projectId)
  if (!worktreeId) return projName
  const wt = worktreesStore.byId(projectId, worktreeId)
  return wt?.branch ? `${projName} · ${wt.branch}` : projName
}

// Resizer logic
const isResizing = ref(false)
let startX = 0
let startWidth = 0

function onResizerMouseDown(e: MouseEvent) {
  e.preventDefault()
  isResizing.value = true
  startX = e.clientX
  startWidth = dockStore.previewWidth
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'

  const onMouseMove = (ev: MouseEvent) => {
    if (!isResizing.value) return
    const delta = startX - ev.clientX
    const maxWidth = Math.max(400, Math.min(window.innerWidth * 0.7, window.innerWidth - 360))
    dockStore.setPreviewWidth(Math.min(maxWidth, Math.max(320, startWidth + delta)))
  }

  const onMouseUp = () => {
    if (!isResizing.value) return
    isResizing.value = false
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    window.removeEventListener('mousemove', onMouseMove)
    window.removeEventListener('mouseup', onMouseUp)
  }

  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
}

// Alt+W shortcut to close active tab
function onKeydown(e: KeyboardEvent) {
  if (dockStore.previewCollapsed) return
  if (!e.altKey || e.ctrlKey || e.metaKey || e.key.toLowerCase() !== 'w') return
  if (dockStore.activeTabKey === FILE_TREE_TAB_KEY) return
  if (!dockStore.openTabs.some((t) => t.key === dockStore.activeTabKey)) return
  e.preventDefault()
  void requestCloseTab(dockStore.activeTabKey)
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
})

const effectiveScope = computed(() => dockStore.effectiveScope)
const currentDirectory = computed(() => {
  if (!effectiveScope.value) return ''
  const proj = projectsStore.byId(effectiveScope.value.projectId)
  if (effectiveScope.value.worktreeId) {
    const wt = worktreesStore.byId(effectiveScope.value.projectId, effectiveScope.value.worktreeId)
    if (wt?.path) return wt.path
  }
  return proj?.path || ''
})
</script>

<template>
  <aside
    v-if="!dockStore.previewCollapsed"
    id="session-preview-pane"
    class="workspace-dock-pane"
    :class="{ 'is-resizing': isResizing }"
    :style="{ width: `${dockStore.previewWidth}px`, flex: `0 0 ${dockStore.previewWidth}px` }"
    aria-label="工作区文件面板"
    data-test="activity-pane"
    data-test-preview="preview-pane"
  >
    <!-- Left edge resizer -->
    <div
      class="dock-resizer"
      data-test="preview-resizer"
      title="拖拽调整工作区面板宽度"
      @mousedown="onResizerMouseDown"
    >
      <span class="dock-resizer-line"></span>
    </div>

    <div class="dock-inner">
      <!-- Header Tab Bar (flush with SessionTabs) -->
      <header class="dock-header">
        <div class="dock-tabs-list" role="tablist">
          <!-- Permanent Tab: Workspace File Tree -->
          <button
            type="button"
            class="dock-tab filetree-permanent-tab"
            :class="{ 'is-active': dockStore.activeTabKey === FILE_TREE_TAB_KEY }"
            role="tab"
            :aria-selected="dockStore.activeTabKey === FILE_TREE_TAB_KEY"
            title="工作区文件树"
            data-test="tab-filetree"
            @click="dockStore.selectTab(FILE_TREE_TAB_KEY)"
          >
            <svg class="filetree-tab-icon" viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M3 7v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-6l-2-2H5a2 2 0 0 0-2 2z" />
            </svg>
            <span class="file-tab-name">工作区文件</span>
          </button>

          <!-- Dynamic Opened File Tabs -->
          <div
            v-for="fileTab in dockStore.openTabs"
            :key="fileTab.key"
            class="dock-tab file-item-tab"
            :class="{
              'is-active': dockStore.activeTabKey === fileTab.key,
              'is-foreign': dockStore.isTabForeign(fileTab),
            }"
            role="tab"
            :aria-selected="dockStore.activeTabKey === fileTab.key"
            :title="dockStore.isTabForeign(fileTab)
              ? `${fileTab.path} (非当前工作区: ${getScopeBadge(fileTab.projectId, fileTab.worktreeId)})`
              : fileTab.path"
            data-test="tab-file"
            @click="dockStore.selectTab(fileTab.key)"
            @mousedown.middle.prevent="requestCloseTab(fileTab.key)"
          >
            <span class="file-tab-name">{{ fileTab.name }}</span>

            <!-- Foreign Badge for cross-workspace files -->
            <span
              v-if="dockStore.isTabForeign(fileTab)"
              class="foreign-badge"
              :title="getScopeBadge(fileTab.projectId, fileTab.worktreeId)"
            >
              {{ getProjectName(fileTab.projectId) }}
            </span>

            <span
              v-if="dirtyMap[fileTab.key] || fileTab.isDirty"
              class="dirty-dot"
              title="有未保存修改"
              data-test="tab-dirty-dot"
            ></span>

            <span
              v-if="fileTab.mode === 'diff'"
              class="diff-badge"
            >
              Diff
            </span>

            <button
              type="button"
              class="tab-close-icon"
              title="关闭标签"
              aria-label="关闭标签"
              data-test="close-file-tab"
              @click.stop="requestCloseTab(fileTab.key, $event)"
            >
              ×
            </button>
          </div>
        </div>

        <!-- Trailing action controls: Lock & Collapse -->
        <div class="dock-actions">
          <button
            type="button"
            class="action-icon-btn lock-btn"
            :class="{ 'is-locked': dockStore.isLocked }"
            :title="dockStore.isLocked ? '当前工作区已锁定 (不跟随会话切换，点击解除)' : '锁定当前工作区 (切换会话不改变文件树)'"
            :aria-label="dockStore.isLocked ? '解除锁定工作区' : '锁定工作区'"
            @click="dockStore.toggleLock()"
          >
            <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="5" y="11" width="14" height="10" rx="2" />
              <path v-if="dockStore.isLocked" d="M8 11V7a4 4 0 0 1 8 0v4" />
              <path v-else d="M8 11V7a4 4 0 0 1 7.5-2" />
            </svg>
          </button>

          <button
            type="button"
            class="action-icon-btn close-dock-btn"
            data-test="close-preview-pane-btn"
            title="收起工作区面板 (Alt+2)"
            aria-label="收起工作区面板"
            @click="dockStore.setPreviewCollapsed(true)"
          >
            <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>
      </header>

      <!-- Main Dock Body -->
      <div class="dock-body">
        <!-- Tab 1: File Tree (Directory View) -->
        <div
          v-show="dockStore.activeTabKey === FILE_TREE_TAB_KEY"
          id="session-filetree-drawer"
          class="filetree-container"
          data-test="filetree-drawer"
        >
          <FileSidebar
            v-if="effectiveScope"
            :key="`${effectiveScope.projectId}:${effectiveScope.worktreeId}:${effectiveScope.sessionId || ''}`"
            :project-id="effectiveScope.projectId"
            :session-id="effectiveScope.sessionId"
            :worktree-id="effectiveScope.worktreeId"
            :directory="currentDirectory"
            :active-path="dockStore.currentTab?.path"
            @select="(path, options) => dockStore.openFile({
              projectId: effectiveScope!.projectId,
              worktreeId: effectiveScope!.worktreeId,
              path,
              ...options
            })"
          />
          <div v-else class="dock-empty-hint">
            <span>暂无选中的项目或工作区</span>
          </div>
        </div>

        <!-- Tab 2..N: Opened File Viewers (Preserved Instances for Draft Protection) -->
        <div
          v-for="fileTab in dockStore.openTabs"
          v-show="dockStore.activeTabKey === fileTab.key"
          :key="fileTab.key"
          class="file-viewer-wrapper"
        >
          <FileViewer
            :project-id="fileTab.projectId"
            :worktree-id="fileTab.worktreeId"
            :active-path="fileTab.path"
            :hide-tabs="true"
            :mode="fileTab.mode || 'view'"
            :commit-ref="fileTab.commitRef"
            :commit-message="fileTab.commitMessage"
            :initial-line="fileTab.initialLine"
            @select="(path, options) => dockStore.openFile({
              projectId: fileTab.projectId,
              worktreeId: fileTab.worktreeId,
              path,
              ...options
            })"
            @add-comment="emit('add-comment', $event)"
            @dirty-change="(p, dirty) => handleDirtyChange(fileTab.key, dirty)"
          />
        </div>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.workspace-dock-pane {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--color-surface);
  border-left: 1px solid var(--color-border);
  box-sizing: border-box;
  overflow: hidden;
  z-index: 10;
  transition: width 0.15s ease-out;
}

.workspace-dock-pane.is-resizing {
  transition: none !important;
}

.dock-resizer {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 5px;
  cursor: col-resize;
  z-index: 25;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s ease-out;
}

.dock-resizer:hover,
.workspace-dock-pane.is-resizing .dock-resizer {
  background: var(--color-primary-soft);
}

.dock-resizer-line {
  width: 2px;
  height: 24px;
  border-radius: 1px;
  background: var(--color-border-strong);
}

.dock-inner {
  display: flex;
  flex-direction: column;
  flex: 1 1 0%;
  min-width: 0;
  height: 100%;
  overflow: hidden;
}

.dock-header {
  height: 35px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  padding: 0 8px;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface);
  flex-shrink: 0;
  box-sizing: border-box;
}

.dock-tabs-list {
  display: flex;
  align-items: stretch;
  gap: 2px;
  height: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  flex: 1;
  min-width: 0;
  scrollbar-width: thin;
}

.dock-tab {
  border: none;
  background: transparent;
  padding: 0 8px;
  height: 100%;
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
  user-select: none;
  border-bottom: 2px solid transparent;
  transition: color 0.12s ease-out, border-color 0.12s ease-out, background-color 0.12s ease-out;
}

.dock-tab:hover {
  color: var(--color-text);
  background: var(--color-surface-muted);
}

.dock-tab.is-active {
  color: var(--color-text);
  border-bottom-color: var(--color-primary);
  font-weight: 600;
}

.file-item-tab {
  max-width: 170px;
}

/* Foreign Tab: Highlight for cross-workspace files */
.dock-tab.is-foreign {
  background: var(--color-warning-soft);
}

.dock-tab.is-foreign.is-active {
  border-bottom-color: var(--color-warning);
}

.foreign-badge {
  font-size: var(--text-size-10);
  line-height: 1;
  padding: 1px 4px;
  border-radius: 2px;
  background: var(--color-warning-soft);
  color: var(--color-warning);
  max-width: 60px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-tab-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dirty-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-warning);
  flex-shrink: 0;
}

.diff-badge {
  font-size: var(--text-size-10);
  padding: 1px 4px;
  border-radius: 2px;
  background: var(--color-warning-soft);
  color: var(--color-warning);
  font-family: var(--font-mono);
}

.tab-close-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 2px;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  font-size: var(--text-size-11);
  line-height: 1;
  cursor: pointer;
  padding: 0;
  margin-left: 2px;
}

.tab-close-icon:hover {
  background: var(--color-danger-soft);
  color: var(--color-danger);
}

.dock-actions {
  display: flex;
  align-items: center;
  gap: 3px;
  flex-shrink: 0;
}

.action-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 3px;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all 0.12s ease-out;
}

.action-icon-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-border);
  color: var(--color-text);
}

.lock-btn.is-locked {
  color: var(--color-primary);
  background: var(--color-primary-soft);
  border-color: var(--color-primary-soft);
}

.dock-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
}

.filetree-container {
  flex: 1;
  min-height: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.file-viewer-wrapper {
  flex: 1;
  min-height: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.dock-empty-hint {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--color-text-muted);
  font-size: var(--text-size-12);
}
</style>
