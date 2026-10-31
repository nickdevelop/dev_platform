<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getFileContent, getFileRawUrl } from '../../api/files'
import type { FileContentResponse, FileTab } from './types'
import CodeView from './CodeView.vue'
import MarkdownArtifactView from './MarkdownArtifactView.vue'
import HtmlArtifactView from './HtmlArtifactView.vue'
import ArtifactImageView from './ArtifactImageView.vue'
import DiffViewer from './DiffViewer.vue'

const props = withDefaults(
  defineProps<{
    projectId: number
    activePath?: string
    hideTabs?: boolean
    mode?: 'view' | 'diff'
    commitRef?: string
    commitMessage?: string
  }>(),
  {
    hideTabs: false,
    mode: 'view',
  },
)

const emit = defineEmits<{
  select: [path: string, options?: { mode?: 'view' | 'diff'; commitRef?: string; commitMessage?: string }]
  close: [path: string]
}>()

const tabs = ref<FileTab[]>([])
const activeTabPath = ref<string>('')
const loading = ref(false)
const error = ref('')
const fileData = ref<FileContentResponse | null>(null)

function getFileName(filePath: string): string {
  const parts = filePath.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1] || filePath
}

function openTab(path: string, options?: { mode?: 'view' | 'diff'; commitRef?: string; commitMessage?: string }) {
  if (!path) return
  const cleanPath = path.replace(/\\/g, '/').replace(/^\/+/, '')
  const reqMode = options?.mode ?? props.mode ?? 'view'
  const reqCommit = options?.commitRef ?? props.commitRef
  const reqMsg = options?.commitMessage ?? props.commitMessage

  const existing = tabs.value.find((t) => t.path === cleanPath)
  if (!existing) {
    tabs.value.push({
      path: cleanPath,
      name: getFileName(cleanPath),
      mode: reqMode,
      commitRef: reqCommit,
    })
  } else if (options?.mode || options?.commitRef !== undefined) {
    existing.mode = reqMode
    existing.commitRef = reqCommit
  }
  activeTabPath.value = cleanPath
  emit('select', cleanPath, { mode: reqMode, commitRef: reqCommit, commitMessage: reqMsg })
}

function closeTab(path: string, event?: MouseEvent) {
  event?.stopPropagation()
  const idx = tabs.value.findIndex((t) => t.path === path)
  if (idx === -1) return

  tabs.value.splice(idx, 1)
  emit('close', path)

  if (activeTabPath.value === path) {
    if (tabs.value.length > 0) {
      const nextIdx = Math.min(idx, tabs.value.length - 1)
      const nextTab = tabs.value[nextIdx]
      activeTabPath.value = nextTab.path
      emit('select', nextTab.path, { mode: nextTab.mode, commitRef: nextTab.commitRef })
    } else {
      activeTabPath.value = ''
      fileData.value = null
    }
  }
}

watch(
  [() => props.activePath, () => props.mode, () => props.commitRef],
  ([newPath, newMode, newCommit]) => {
    if (newPath) {
      const cleanPath = newPath.replace(/\\/g, '/').replace(/^\/+/, '')
      if (props.hideTabs) {
        activeTabPath.value = cleanPath
      } else {
        openTab(cleanPath, { mode: newMode, commitRef: newCommit, commitMessage: props.commitMessage })
      }
    } else if (props.hideTabs) {
      activeTabPath.value = ''
      fileData.value = null
    }
  },
  { immediate: true },
)

const activeTab = computed(() => tabs.value.find((t) => t.path === activeTabPath.value))

const isDiffMode = computed(() => {
  if (props.hideTabs) {
    return props.mode === 'diff'
  }
  return activeTab.value?.mode === 'diff'
})

const activeCommitRef = computed(() => {
  if (props.hideTabs) {
    return props.commitRef
  }
  return activeTab.value?.commitRef || props.commitRef
})

async function loadCurrentFile() {
  const projId = props.projectId
  const currentPath = activeTabPath.value
  if (!projId || !currentPath || isDiffMode.value) {
    fileData.value = null
    return
  }
  loading.value = true
  error.value = ''
  try {
    const data = await getFileContent(projId, currentPath)
    fileData.value = data
  } catch (err) {
    error.value = err instanceof Error ? err.message : '读取文件失败'
    fileData.value = null
  } finally {
    loading.value = false
  }
}

watch(
  [() => props.projectId, activeTabPath, isDiffMode],
  () => {
    if (!isDiffMode.value) {
      void loadCurrentFile()
    }
  },
  { immediate: true },
)

const isEmpty = computed(() => {
  if (props.hideTabs) {
    return !activeTabPath.value
  }
  return tabs.value.length === 0
})

const currentFileType = computed<'markdown' | 'html' | 'image' | 'binary' | 'code'>(() => {
  if (!activeTabPath.value) return 'code'
  const ext = activeTabPath.value.split('.').pop()?.toLowerCase() || ''
  if (ext === 'md' || ext === 'markdown') return 'markdown'
  if (ext === 'html' || ext === 'htm') return 'html'
  if (['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp'].includes(ext)) return 'image'
  if (fileData.value?.isBinary) return 'binary'
  return 'code'
})
</script>

<template>
  <div class="file-viewer" data-test="file-viewer">
    <!-- Top tab strip -->
    <div v-if="!props.hideTabs && tabs.length > 0" class="tab-strip" data-test="viewer-tab-strip">
      <div
        v-for="tab in tabs"
        :key="tab.path"
        class="viewer-tab"
        :class="{ 'is-active': activeTabPath === tab.path }"
        :title="tab.path"
        @click="openTab(tab.path, { mode: tab.mode, commitRef: tab.commitRef })"
      >
        <span class="tab-title">{{ tab.name }}</span>
        <span
          v-if="tab.mode === 'diff'"
          class="tab-diff-badge text-[10px] px-1 py-0.2 rounded bg-amber-500/15 text-amber-600 dark:text-amber-400 font-mono font-medium"
        >
          Diff
        </span>
        <button
          type="button"
          class="tab-close-btn"
          title="关闭标签"
          aria-label="关闭标签"
          @click="closeTab(tab.path, $event)"
        >
          ×
        </button>
      </div>
    </div>

    <!-- Main viewer body -->
    <div class="viewer-body">
      <!-- Empty state -->
      <div v-if="isEmpty" class="empty-state">
        <span class="empty-icon">
          <svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M3 4h6l2 2h10v12H3V4z" />
          </svg>
        </span>
        <p class="empty-text">在左侧文件树中点击文件进行查看</p>
      </div>

      <!-- Diff mode view -->
      <template v-else-if="isDiffMode">
        <DiffViewer
          :project-id="props.projectId"
          :file-path="activeTabPath"
          :commit-ref="activeCommitRef"
          :commit-message="props.commitMessage"
        />
      </template>

      <!-- Loading state (for normal file view) -->
      <div v-else-if="loading" class="state-loading">
        正在加载文件内容…
      </div>

      <!-- Error state (for normal file view) -->
      <div v-else-if="error" class="state-error">
        <p>{{ error }}</p>
        <button
          type="button"
          class="retry-btn"
          @click="loadCurrentFile"
        >
          重试
        </button>
      </div>

      <!-- Content view by file type -->
      <template v-else-if="fileData">
        <!-- Markdown view -->
        <MarkdownArtifactView
          v-if="currentFileType === 'markdown'"
          :project-id="props.projectId"
          :file-path="activeTabPath"
          :content="fileData.content || ''"
          :file-size="fileData.size"
          :lines="fileData.lines"
          @open-file="openTab"
        />

        <!-- HTML view -->
        <HtmlArtifactView
          v-else-if="currentFileType === 'html'"
          :project-id="props.projectId"
          :file-path="activeTabPath"
          :content="fileData.content || ''"
          :file-size="fileData.size"
          :lines="fileData.lines"
        />

        <!-- Image view -->
        <ArtifactImageView
          v-else-if="currentFileType === 'image'"
          :raw-url="getFileRawUrl(props.projectId, activeTabPath)"
          :file-name="getFileName(activeTabPath)"
          :file-size="fileData.size"
        />

        <!-- Binary placeholder view -->
        <div v-else-if="currentFileType === 'binary'" class="binary-state">
          <span class="binary-icon">
            <svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M12 2l9 5v10l-9 5-9-5V7l9-5z" />
              <path d="M12 22V12" />
              <path d="M21 7l-9 5-9-5" />
            </svg>
          </span>
          <p class="binary-title">二进制文件无法在线直接显示</p>
          <p class="binary-meta">文件大小: {{ (fileData.size / 1024).toFixed(1) }} KB · 类型: {{ fileData.mimeType }}</p>
        </div>

        <!-- Plain text / code view -->
        <CodeView
          v-else
          :content="fileData.content || ''"
          :file-path="activeTabPath"
        />
      </template>
    </div>
  </div>
</template>

<style scoped>
.file-viewer {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--color-surface, #ffffff);
  overflow: hidden;
}

.tab-strip {
  display: flex;
  align-items: center;
  height: 36px;
  background: var(--color-surface-muted, #f8fafc);
  border-bottom: 1px solid var(--color-border, #e3e6eb);
  overflow-x: auto;
  flex-shrink: 0;
  padding: 0 4px;
  gap: 2px;
}

.viewer-tab {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px;
  border: none;
  border-bottom: 2px solid transparent;
  border-radius: 0;
  font-size: 0.78rem;
  color: var(--color-text-muted);
  cursor: pointer;
  user-select: none;
  background: transparent;
  transition: color 0.12s ease-out, border-color 0.12s ease-out;
  max-width: 180px;
}

.viewer-tab:hover {
  color: var(--color-text);
}

.viewer-tab.is-active {
  background: transparent;
  color: var(--color-text);
  border-bottom-color: var(--color-accent);
  font-weight: 600;
}

.tab-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tab-close-btn {
  border: none;
  background: transparent;
  width: 16px;
  height: 16px;
  border-radius: 2px;
  font-size: 0.85rem;
  line-height: 1;
  color: var(--color-text-muted);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0.6;
}

.tab-close-btn:hover {
  opacity: 1;
  background: rgba(0, 0, 0, 0.08);
  color: var(--color-text);
}

.viewer-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  position: relative;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--color-text-muted);
  gap: 8px;
}

.empty-icon {
  opacity: 0.4;
}

.empty-text {
  font-size: 0.82rem;
}

.state-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  font-size: 0.82rem;
  color: var(--color-text-muted);
}

.state-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 24px;
  text-align: center;
  color: var(--color-danger, #b91c1c);
  gap: 8px;
  font-size: 0.82rem;
}

.retry-btn {
  border: 1px solid var(--color-danger-border, #fca5a5);
  background: var(--color-danger-soft, #fee2e2);
  color: var(--color-danger, #b91c1c);
  padding: 4px 12px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.78rem;
}

.binary-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--color-text-muted);
  gap: 8px;
}

.binary-icon {
  opacity: 0.4;
}

.binary-title {
  font-size: 0.9rem;
  font-weight: 500;
  color: var(--color-text);
}

.binary-meta {
  font-size: 0.75rem;
  color: var(--color-text-muted);
}
</style>
