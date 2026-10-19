<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getFileContent, getFileRawUrl } from '../../api/files'
import type { FileContentResponse, FileTab } from './types'
import CodeView from './CodeView.vue'
import MarkdownArtifactView from './MarkdownArtifactView.vue'
import ArtifactImageView from './ArtifactImageView.vue'

const props = defineProps<{
  projectId: number
  activePath?: string
}>()

const emit = defineEmits<{
  select: [path: string]
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

function openTab(path: string) {
  if (!path) return
  const cleanPath = path.replace(/\\/g, '/').replace(/^\/+/, '')
  const existing = tabs.value.find((t) => t.path === cleanPath)
  if (!existing) {
    tabs.value.push({
      path: cleanPath,
      name: getFileName(cleanPath),
    })
  }
  activeTabPath.value = cleanPath
  emit('select', cleanPath)
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
      activeTabPath.value = tabs.value[nextIdx].path
      emit('select', activeTabPath.value)
    } else {
      activeTabPath.value = ''
      fileData.value = null
    }
  }
}

watch(
  () => props.activePath,
  (newPath) => {
    if (newPath && newPath !== activeTabPath.value) {
      openTab(newPath)
    }
  },
  { immediate: true },
)

watch(
  [() => props.projectId, activeTabPath],
  async ([projId, currentPath]) => {
    if (!projId || !currentPath) {
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
  },
  { immediate: true },
)

const currentFileType = computed<'markdown' | 'image' | 'binary' | 'code'>(() => {
  if (!activeTabPath.value) return 'code'
  const ext = activeTabPath.value.split('.').pop()?.toLowerCase() || ''
  if (ext === 'md' || ext === 'markdown') return 'markdown'
  if (['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp'].includes(ext)) return 'image'
  if (fileData.value?.isBinary) return 'binary'
  return 'code'
})
</script>

<template>
  <div class="file-viewer" data-test="file-viewer">
    <!-- Top tab strip -->
    <div v-if="tabs.length > 0" class="tab-strip" data-test="viewer-tab-strip">
      <div
        v-for="tab in tabs"
        :key="tab.path"
        class="viewer-tab"
        :class="{ 'is-active': activeTabPath === tab.path }"
        :title="tab.path"
        @click="openTab(tab.path)"
      >
        <span class="tab-title">{{ tab.name }}</span>
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
      <div v-if="tabs.length === 0" class="empty-state">
        <span class="empty-icon">📁</span>
        <p class="empty-text">在左侧文件树中点击文件进行查看</p>
      </div>

      <!-- Loading state -->
      <div v-else-if="loading" class="state-loading">
        正在加载文件内容…
      </div>

      <!-- Error state -->
      <div v-else-if="error" class="state-error">
        <p>{{ error }}</p>
        <button
          type="button"
          class="retry-btn"
          @click="openTab(activeTabPath)"
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

        <!-- Image view -->
        <ArtifactImageView
          v-else-if="currentFileType === 'image'"
          :raw-url="getFileRawUrl(props.projectId, activeTabPath)"
          :file-name="getFileName(activeTabPath)"
          :file-size="fileData.size"
        />

        <!-- Binary placeholder view -->
        <div v-else-if="currentFileType === 'binary'" class="binary-state">
          <span class="binary-icon">📦</span>
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
  border-radius: 4px;
  font-size: 0.78rem;
  color: var(--color-text-muted);
  cursor: pointer;
  user-select: none;
  background: transparent;
  transition: all 0.15s ease;
  max-width: 180px;
}

.viewer-tab:hover {
  background: rgba(0, 0, 0, 0.04);
  color: var(--color-text);
}

.viewer-tab.is-active {
  background: var(--color-surface, #ffffff);
  color: var(--color-primary, #2563eb);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
}

.tab-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tab-close-btn {
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 0.95rem;
  line-height: 1;
  padding: 0 2px;
  border-radius: 2px;
  cursor: pointer;
}

.tab-close-btn:hover {
  background: rgba(0, 0, 0, 0.08);
  color: var(--color-danger, #b91c1c);
}

.viewer-body {
  flex: 1;
  min-height: 0;
  position: relative;
}

.empty-state,
.state-loading,
.state-error,
.binary-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 24px;
  text-align: center;
  color: var(--color-text-muted);
}

.empty-icon,
.binary-icon {
  font-size: 2.2rem;
  margin-bottom: 8px;
}

.empty-text {
  font-size: 0.85rem;
}

.binary-title {
  font-size: 0.95rem;
  font-weight: 500;
  color: var(--color-text);
  margin-bottom: 4px;
}

.binary-meta {
  font-size: 0.78rem;
  color: var(--color-text-muted);
}

.state-error {
  color: var(--color-danger, #b91c1c);
  gap: 10px;
}

.retry-btn {
  border: 1px solid var(--color-danger-border, #fca5a5);
  background: var(--color-danger-soft, #fee2e2);
  color: var(--color-danger, #b91c1c);
  padding: 4px 12px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.8rem;
}
</style>
