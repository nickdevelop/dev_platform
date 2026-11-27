<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  useWorkspaceDockStore,
  MIN_FILE_TREE_WIDTH,
  MAX_FILE_TREE_WIDTH,
  MIN_SPLIT_PREVIEW_WIDTH,
} from '../../stores/workspaceDock'
import { useProjectsStore } from '../../stores/projects'
import { useWorktreesStore } from '../../stores/worktrees'
import { FileSidebar } from '../files'

const dockStore = useWorkspaceDockStore()
const projectsStore = useProjectsStore()
const worktreesStore = useWorktreesStore()

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

// Left-edge resizer logic
const isResizing = ref(false)
let startX = 0
let startWidth = 0

function onResizerMouseDown(e: MouseEvent) {
  if (e.button !== 0) return
  e.preventDefault()
  isResizing.value = true
  startX = e.clientX
  startWidth = dockStore.fileTreeWidth
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'

  const onMouseMove = (ev: MouseEvent) => {
    if (!isResizing.value) return
    const delta = startX - ev.clientX
    // When preview dock is open, dragging the divider between preview dock and file tree
    // must only redistribute width between those two panels without resizing the session pane.
    const maxByNeighbor = dockStore.previewCollapsed
      ? window.innerWidth - 380
      : dockStore.previewWidth - MIN_SPLIT_PREVIEW_WIDTH
    const maxWidth = Math.max(MIN_FILE_TREE_WIDTH, Math.min(MAX_FILE_TREE_WIDTH, maxByNeighbor))
    dockStore.setFileTreeWidth(Math.min(maxWidth, Math.max(MIN_FILE_TREE_WIDTH, startWidth + delta)))
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
</script>

<template>
  <aside
    v-if="dockStore.fileTreeVisible"
    id="workspace-filetree-panel"
    class="filetree-dock-pane"
    :class="{ 'is-resizing': isResizing }"
    :style="{ width: `${dockStore.fileTreeWidth}px`, flex: `0 0 ${dockStore.fileTreeWidth}px` }"
    aria-label="文件树面板"
    data-test="filetree-panel"
  >
    <!-- Left edge resizer -->
    <div
      class="filetree-resizer"
      data-test="filetree-resizer"
      title="拖拽调整文件树面板宽度"
      @mousedown="onResizerMouseDown"
    >
      <span class="filetree-resizer-line"></span>
    </div>

    <div class="filetree-panel-inner">
      <header class="filetree-panel-header">
        <div class="filetree-panel-title">
          <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
            <path d="M3 7v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-6l-2-2H5a2 2 0 0 0-2 2z" />
          </svg>
          <span>文件树</span>
        </div>

        <div class="filetree-panel-actions">
          <button
            type="button"
            class="panel-action-btn lock-btn"
            :class="{ 'is-locked': dockStore.isLocked }"
            :title="dockStore.isLocked ? '当前工作区已锁定 (不跟随会话切换，点击解除)' : '锁定当前工作区 (切换会话不改变文件树)'"
            :aria-label="dockStore.isLocked ? '解除锁定工作区' : '锁定工作区'"
            data-test="filetree-lock-btn"
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
            class="panel-action-btn"
            title="隐藏文件树面板 (Ctrl+\)"
            aria-label="隐藏文件树面板"
            data-test="close-filetree-panel-btn"
            @click="dockStore.setFileTreeVisible(false)"
          >
            <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>
      </header>

      <div class="filetree-panel-body" data-test="filetree-drawer">
        <FileSidebar
          v-if="effectiveScope"
          :key="`files:${effectiveScope.projectId}:${effectiveScope.worktreeId}:${effectiveScope.sessionId || ''}`"
          :project-id="effectiveScope.projectId"
          :session-id="effectiveScope.sessionId"
          :worktree-id="effectiveScope.worktreeId"
          :directory="currentDirectory"
          :active-path="dockStore.currentTab?.path"
          view-mode="files-only"
          @select="(path, options) => dockStore.openFile({
            projectId: effectiveScope!.projectId,
            worktreeId: effectiveScope!.worktreeId,
            path,
            ...options
          })"
        />
        <div v-else class="filetree-empty-hint">
          <span>暂无选中的项目或工作区</span>
        </div>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.filetree-dock-pane {
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
}

.filetree-dock-pane.is-resizing {
  transition: none !important;
}

.filetree-resizer {
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

.filetree-resizer:hover,
.filetree-dock-pane.is-resizing .filetree-resizer {
  background: var(--color-primary-soft);
}

.filetree-resizer-line {
  width: 2px;
  height: 24px;
  border-radius: 1px;
  background: var(--color-border-strong);
}

.filetree-panel-inner {
  display: flex;
  flex-direction: column;
  flex: 1 1 0%;
  min-width: 0;
  height: 100%;
  overflow: hidden;
}

.filetree-panel-header {
  height: 35px;
  min-height: 35px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  padding: 0 8px 0 10px;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface);
  flex-shrink: 0;
  box-sizing: border-box;
}

.filetree-panel-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--text-size-12);
  font-weight: 600;
  color: var(--color-text);
  min-width: 0;
}

.filetree-panel-actions {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex-shrink: 0;
}

.panel-action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: var(--radius);
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: color 0.12s ease-out, background-color 0.12s ease-out, border-color 0.12s ease-out;
}

.panel-action-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-border);
  color: var(--color-text);
}

.panel-action-btn.is-locked {
  color: var(--color-primary);
  background: var(--color-primary-soft);
  border-color: var(--color-primary-soft);
}

.filetree-panel-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.filetree-empty-hint {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--color-text-muted);
  font-size: var(--text-size-12);
}
</style>
