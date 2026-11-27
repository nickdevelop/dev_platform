<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessageBox } from 'element-plus'
import {
  useWorkspaceDockStore,
  FILE_TREE_TAB_KEY,
  MIN_PREVIEW_WIDTH,
  MIN_SPLIT_PREVIEW_WIDTH,
} from '../../stores/workspaceDock'
import { useProjectsStore } from '../../stores/projects'
import { useWorktreesStore } from '../../stores/worktrees'
import { FileSidebar, FileViewer } from '../files'
import type { CodeCommentCard } from '../files/types'
import QuickFileSearchDialog from '../chat/pane/QuickFileSearchDialog.vue'
import WorkbenchTabStrip from '../../shared/components/WorkbenchTabStrip.vue'

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
    const minWidth = dockStore.fileTreeVisible
      ? MIN_SPLIT_PREVIEW_WIDTH + dockStore.fileTreeWidth
      : MIN_PREVIEW_WIDTH
    const maxWidth = Math.max(minWidth, window.innerWidth - 400)
    dockStore.setPreviewWidth(Math.min(maxWidth, Math.max(minWidth, startWidth + delta)))
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

// Ctrl+P / Cmd+P toggles quick search for the file tree's active/locked directory;
// Ctrl+W / Cmd+W (Electron) and Alt+W close the active preview file tab when expanded.
function onKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === 'p') {
    if (!effectiveScope.value) return
    e.preventDefault()
    dockStore.quickSearchOpen = !dockStore.quickSearchOpen
    return
  }
  if (dockStore.previewCollapsed) return
  if (dockStore.activeTabKey === FILE_TREE_TAB_KEY) return
  if (!dockStore.openTabs.some((t) => t.key === dockStore.activeTabKey)) return

  const isAltW = e.altKey && !e.ctrlKey && !e.metaKey && e.key.toLowerCase() === 'w'
  const isCtrlW = (e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === 'w'
  if (!isAltW && !isCtrlW) return

  e.preventDefault()
  if (isCtrlW) {
    e.stopPropagation()
  }
  void requestCloseTab(dockStore.activeTabKey)
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown, true)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown, true)
  dockStore.closeQuickSearch()
})

const effectiveScope = computed(() => dockStore.effectiveScope)

function onQuickSearchSelect(path: string, line?: number) {
  dockStore.closeQuickSearch()
  if (!effectiveScope.value) return
  dockStore.openFile({
    projectId: effectiveScope.value.projectId,
    worktreeId: effectiveScope.value.worktreeId,
    path,
    initialLine: line,
  })
}
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
    :class="{ 'is-resizing': isResizing, 'is-split-with-tree': dockStore.fileTreeVisible }"
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
      <!-- Header Tab Bar (unified with SessionTabs) -->
      <WorkbenchTabStrip aria-label="工作区文件标签" :active-key="dockStore.activeTabKey">
        <!-- Permanent Tab: Git Changes (Workspace Files) -->
        <div
          class="wb-tab dock-tab filetree-permanent-tab"
          :class="{ 'is-active': dockStore.activeTabKey === FILE_TREE_TAB_KEY }"
          role="tab"
          :aria-selected="dockStore.activeTabKey === FILE_TREE_TAB_KEY"
          title="工作区 Git 变更"
          data-test="tab-filetree"
          @click="dockStore.selectTab(FILE_TREE_TAB_KEY)"
        >
          <button type="button" class="wb-tab-select">
            <svg class="filetree-tab-icon" viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M3 7v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-6l-2-2H5a2 2 0 0 0-2 2z" />
            </svg>
            <span class="wb-tab-label file-tab-name">工作区文件</span>
          </button>
        </div>

        <!-- Dynamic Opened File Tabs -->
        <div
          v-for="fileTab in dockStore.openTabs"
          :key="fileTab.key"
          class="wb-tab dock-tab file-item-tab"
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
          <button type="button" class="wb-tab-select">
            <span class="wb-tab-label file-tab-name">{{ fileTab.name }}</span>

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
          </button>

          <button
            type="button"
            class="wb-tab-close tab-close-icon"
            title="关闭标签 (Ctrl+W)"
            aria-label="关闭标签"
            data-test="close-file-tab"
            @click.stop="requestCloseTab(fileTab.key, $event)"
          >
            <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
              <line x1="3" y1="3" x2="13" y2="13" />
              <line x1="13" y1="3" x2="3" y2="13" />
            </svg>
          </button>
        </div>

        <!-- Trailing action controls: FileTree Toggle, Lock & Collapse -->
        <template #trailing>
          <button
            type="button"
            class="wb-tab-action-btn filetree-toggle-btn"
            :class="{ 'is-active': dockStore.fileTreeVisible }"
            data-test="toggle-filetree-panel-btn"
            :title="dockStore.fileTreeVisible ? '隐藏文件树面板 (Ctrl+\\)' : '显示文件树面板 (Ctrl+\\)'"
            :aria-label="dockStore.fileTreeVisible ? '隐藏文件树面板' : '显示文件树面板'"
            @click="dockStore.toggleFileTree()"
          >
            <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M3 7v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-6l-2-2H5a2 2 0 0 0-2 2z" />
            </svg>
          </button>

          <button
            type="button"
            class="wb-tab-action-btn lock-btn"
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
            class="wb-tab-action-btn close-dock-btn"
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
        </template>
      </WorkbenchTabStrip>

      <!-- Main Dock Body -->
      <div class="dock-body">
        <!-- Tab 1: Git Changes (Workspace Files) -->
        <div
          v-show="dockStore.activeTabKey === FILE_TREE_TAB_KEY"
          id="session-filetree-drawer"
          class="filetree-container"
          data-test="workspace-git-pane"
        >
          <FileSidebar
            v-if="effectiveScope"
            :key="`changes:${effectiveScope.projectId}:${effectiveScope.worktreeId}:${effectiveScope.sessionId || ''}`"
            :project-id="effectiveScope.projectId"
            :session-id="effectiveScope.sessionId"
            :worktree-id="effectiveScope.worktreeId"
            :directory="currentDirectory"
            :active-path="dockStore.currentTab?.path"
            view-mode="changes-only"
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

  <QuickFileSearchDialog
    v-if="effectiveScope"
    :open="dockStore.quickSearchOpen"
    :project-id="effectiveScope.projectId"
    :worktree-id="effectiveScope.worktreeId"
    @close="dockStore.closeQuickSearch()"
    @select="onQuickSearchSelect"
  />
</template>

<style scoped>
.workspace-dock-pane {
  position: relative;
  display: flex;
  flex-direction: column;
  flex: 1 1 0%;
  min-width: 320px;
  height: 100%;
  min-height: 0;
  background: var(--color-surface);
  border-left: 1px solid var(--color-border);
  box-sizing: border-box;
  overflow: hidden;
  z-index: 10;
}

.workspace-dock-pane.is-split-with-tree {
  min-width: 240px;
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

.filetree-tab-icon {
  flex-shrink: 0;
  color: var(--color-text-muted);
}

.dock-tab.is-active .filetree-tab-icon {
  color: var(--color-accent);
}

/* Foreign Tab: Highlight for cross-workspace files */
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
