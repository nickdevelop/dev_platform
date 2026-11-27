<script setup lang="ts">
import { onActivated, onDeactivated, onMounted, onUnmounted, ref } from 'vue'
import { ElMessageBox } from 'element-plus'
import { FileSidebar, FileViewer } from '../../files'
import type { CodeCommentCard } from '../../files/types'

export interface OpenFileTab {
  path: string
  name: string
  mode?: 'view' | 'diff'
  commitRef?: string
  commitMessage?: string
  initialLine?: number
}

const props = defineProps<{
  projectId: number
  sessionId?: string
  worktreeId?: number
  directory?: string
  branch?: string
  hideSideRail?: boolean
  previewCollapsed: boolean
  previewWidth: number
  isPreviewResizing: boolean
  activePreviewTab: string
  openFileTabs: OpenFileTab[]
  currentActivePreviewFile?: OpenFileTab | null
  fileTreeTabKey: string
}>()

const emit = defineEmits<{
  collapse: [collapsed: boolean]
  'resizer-mousedown': [event: MouseEvent]
  'select-tab': [tabKey: string]
  'close-tab': [path: string, event?: MouseEvent]
  'select-workspace-file': [
    filePath: string,
    options?: { mode?: 'view' | 'diff'; commitRef?: string; commitMessage?: string }
  ]
  'branch-changed': [branch: string]
  'open-file-preview': [filePath: string]
  'add-comment': [card: CodeCommentCard]
}>()

const dirtyPaths = ref<Record<string, boolean>>({})

function handleDirtyChange(path: string, dirty: boolean) {
  if (dirty) {
    dirtyPaths.value = { ...dirtyPaths.value, [path]: true }
  } else {
    const next = { ...dirtyPaths.value }
    delete next[path]
    dirtyPaths.value = next
  }
}

// All file-tab close paths (× button, middle click, Alt+W) funnel through here
// so unsaved drafts always get a discard confirmation before the tab goes away.
async function requestCloseTab(path: string, event?: MouseEvent) {
  event?.stopPropagation()
  if (dirtyPaths.value[path]) {
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
    delete dirtyPaths.value[path]
  }
  emit('close-tab', path)
}

// Alt+W closes the active file preview tab. A dedicated chord instead of VSCode's
// Ctrl+W: browsers reserve Ctrl+W (closes the whole page tab, page JS cannot
// preventDefault it), so Alt+W is the conflict-free equivalent.
// KeepAlive keeps background session panes mounted, so their window listeners
// must stay silent: only the activated instance reacts.
let isDeactivated = false

onActivated(() => {
  isDeactivated = false
})

onDeactivated(() => {
  isDeactivated = true
})

function onKeydown(e: KeyboardEvent) {
  if (isDeactivated) return
  if (props.previewCollapsed || props.hideSideRail) return
  if (!e.altKey || e.ctrlKey || e.metaKey || e.key.toLowerCase() !== 'w') return
  if (props.activePreviewTab === props.fileTreeTabKey) return
  if (!props.openFileTabs.some((t) => t.path === props.activePreviewTab)) return
  e.preventDefault()
  void requestCloseTab(props.activePreviewTab)
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <!-- Column 2: Unified Right Workspace Panel (FileTree and Previews as Tabs) -->
  <aside
    v-if="!props.hideSideRail && !props.previewCollapsed"
    id="session-preview-pane"
    class="preview-pane activity-pane unified-workspace-pane"
    :class="{ 'is-resizing': props.isPreviewResizing }"
    :style="{ width: `${props.previewWidth}px`, flex: `0 0 ${props.previewWidth}px` }"
    aria-label="文件预览面板"
    data-test="activity-pane"
    data-test-preview="preview-pane"
  >
    <div
      class="preview-resizer"
      data-test="preview-resizer"
      title="拖拽调整预览栏宽度"
      @mousedown="emit('resizer-mousedown', $event)"
    >
      <span class="preview-resizer-line"></span>
    </div>

    <div class="workspace-tabbed-pane">
      <header class="preview-head activity-head">
        <div class="preview-tabs-list" role="tablist">
          <!-- Permanent Tab: Workspace File Tree -->
          <div
            class="pane-tab preview-file-tab filetree-permanent-tab"
            :class="{ 'is-active': activePreviewTab === fileTreeTabKey }"
            role="tab"
            :aria-selected="activePreviewTab === fileTreeTabKey"
            title="工作区文件树"
            data-test="tab-filetree"
            @click="emit('select-tab', fileTreeTabKey)"
          >
            <svg class="filetree-tab-icon" viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M3 7v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-6l-2-2H5a2 2 0 0 0-2 2z" />
            </svg>
            <span class="file-tab-name">工作区文件</span>
          </div>

          <!-- Dynamic Tabs: Opened files -->
          <div
            v-for="fileTab in openFileTabs"
            :key="fileTab.path"
            class="pane-tab preview-file-tab"
            :class="{ 'is-active': activePreviewTab === fileTab.path }"
            role="tab"
            :aria-selected="activePreviewTab === fileTab.path"
            :title="fileTab.path"
            data-test="tab-file"
            @click="emit('select-tab', fileTab.path)"
            @mousedown.middle.prevent="requestCloseTab(fileTab.path)"
          >
            <span class="file-tab-name">{{ fileTab.name }}</span>
            <span
              v-if="dirtyPaths[fileTab.path]"
              class="inline-block w-1.5 h-1.5 rounded-full bg-amber-500 ml-1 shrink-0"
              title="有未保存修改"
              data-test="tab-dirty-dot"
            ></span>
            <span
              v-if="fileTab.mode === 'diff'"
              class="tab-diff-badge text-2xs px-1.5 py-0.5 rounded bg-amber-500/15 text-amber-600 dark:text-amber-400 font-mono font-medium ml-1"
            >
              Diff
            </span>
            <button
              type="button"
              class="tab-close-icon"
              title="关闭标签"
              aria-label="关闭标签"
              data-test="close-file-tab"
              @click.stop="requestCloseTab(fileTab.path, $event)"
            >
              ×
            </button>
          </div>
        </div>

        <div class="head-actions">
          <button
            type="button"
            class="head-action-icon-btn preview-head-close"
            data-test="close-preview-pane-btn"
            title="收起工作区面板"
            aria-label="收起工作区面板"
            @click="emit('collapse', true)"
          >
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>
      </header>

      <div class="workspace-pane-body">
        <!-- When active tab is the file tree -->
        <div
          v-show="activePreviewTab === fileTreeTabKey"
          id="session-filetree-drawer"
          class="filetree-pane-view"
          data-test="filetree-drawer"
        >
          <FileSidebar
            :project-id="projectId"
            :session-id="sessionId"
            :worktree-id="worktreeId"
            :directory="props.directory"
            :branch="props.branch"
            :active-path="activePreviewTab"
            @select="(path, options) => emit('select-workspace-file', path, options)"
            @branch-changed="emit('branch-changed', $event)"
          />
        </div>

        <!-- When active tab is a file viewer -->
        <div
          v-show="activePreviewTab !== fileTreeTabKey && currentActivePreviewFile"
          class="preview-file-viewer"
        >
          <FileViewer
            :project-id="projectId"
            :worktree-id="worktreeId"
            :active-path="activePreviewTab"
            :hide-tabs="true"
            :mode="currentActivePreviewFile?.mode || 'view'"
            :commit-ref="currentActivePreviewFile?.commitRef"
            :commit-message="currentActivePreviewFile?.commitMessage"
            :initial-line="currentActivePreviewFile?.initialLine"
            @select="emit('open-file-preview', $event)"
            @add-comment="emit('add-comment', $event)"
            @dirty-change="handleDirtyChange"
          />
        </div>
      </div>
    </div>
  </aside>

  <!-- Collapsed edge toggle button (pure icon, zero layout jitter, accessible in panel) -->
  <button
    v-if="!props.hideSideRail && props.previewCollapsed"
    type="button"
    class="workspace-collapsed-edge-btn"
    data-test="toggle-preview-btn"
    title="展开预览面板"
    aria-label="展开预览面板"
    :aria-pressed="!props.previewCollapsed ? 'true' : 'false'"
    @click="emit('collapse', false)"
  >
    <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
      <polyline points="15 18 9 12 15 6" />
    </svg>
  </button>
</template>

<style scoped>
.preview-pane {
  background: var(--color-surface);
  position: relative;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-sizing: border-box;
}

.unified-workspace-pane {
  height: 100%;
}

.workspace-tabbed-pane {
  display: flex;
  flex-direction: column;
  flex: 1 1 0%;
  min-width: 0;
  height: 100%;
  overflow: hidden;
  position: relative;
}

.workspace-pane-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
}

.filetree-pane-view {
  flex: 1;
  min-height: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
}

.filetree-permanent-tab {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  cursor: pointer;
}

.filetree-tab-icon {
  flex-shrink: 0;
  opacity: 0.8;
}

.head-action-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 3px;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: color 0.12s ease-out, background-color 0.12s ease-out, border-color 0.12s ease-out;
}

.head-action-icon-btn:hover {
  background: var(--color-surface);
  border-color: var(--color-border);
  color: var(--color-text);
}

.head-action-icon-btn.is-active {
  color: var(--color-accent);
  background: var(--color-accent-soft);
}

.workspace-collapsed-edge-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 48px;
  position: absolute;
  right: 0;
  top: 50%;
  transform: translateY(-50%);
  border: 1px solid var(--color-border);
  border-right: none;
  border-radius: 4px 0 0 4px;
  background: var(--color-surface);
  color: var(--color-text-muted);
  cursor: pointer;
  z-index: 15;
  transition: width 0.12s ease-out, background-color 0.12s ease-out, color 0.12s ease-out;
}

.workspace-collapsed-edge-btn:hover {
  width: 18px;
  color: var(--color-accent);
  background: var(--color-surface-muted);
}

.preview-pane.is-resizing {
  transition: none !important;
}

.preview-resizer {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 6px;
  cursor: col-resize;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s ease-out;
}

.preview-resizer:hover,
.preview-pane.is-resizing .preview-resizer {
  background: var(--color-primary-soft);
}

.preview-resizer-line {
  width: 2px;
  height: 24px;
  border-radius: 1px;
  background: var(--color-border-strong);
  transition: background-color 0.15s ease-out, height 0.15s ease-out;
}

.preview-resizer:hover .preview-resizer-line,
.preview-pane.is-resizing .preview-resizer-line {
  background: var(--color-accent);
  height: 36px;
}

@container (max-width: 900px) {
  .preview-pane {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    width: min(32rem, 86%) !important;
    max-width: 86%;
    z-index: 25;
    border-left: 1px solid var(--color-border-strong);
  }

  .preview-resizer {
    display: none;
  }
}

.preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.4rem 0.6rem;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface-muted, #f8fafc);
  flex-shrink: 0;
}

.preview-tabs-list {
  display: flex;
  align-items: center;
  gap: 3px;
  overflow-x: auto;
  flex: 1;
  min-width: 0;
  scrollbar-width: thin;
}

.preview-tabs-list::-webkit-scrollbar {
  height: 3px;
}

.pane-tab {
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  padding: 4px 8px;
  border-radius: 0;
  font-size: 0.78rem;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: color 0.12s ease-out, border-color 0.12s ease-out;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
  user-select: none;
}

.pane-tab:hover {
  color: var(--color-text);
  background: transparent;
}

.pane-tab.is-active {
  background: transparent;
  color: var(--color-text);
  border-bottom-color: var(--color-accent);
  font-weight: 600;
}

.preview-file-tab {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: 160px;
}

.file-tab-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
  font-size: 0.85rem;
  line-height: 1;
  cursor: pointer;
  padding: 0;
  margin-left: 2px;
  transition: color 0.12s ease-out, background-color 0.12s ease-out;
}

.tab-close-icon:hover {
  background: var(--color-danger-soft);
  color: var(--color-danger);
}

.preview-file-viewer {
  flex: 1;
  min-height: 0;
  height: 100%;
}
</style>
