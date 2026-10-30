<script setup lang="ts">
import { computed, ref } from 'vue'
import { createPreviewTicket, getPreviewFileUrl } from '../../api/files'
import CodeView from './CodeView.vue'
import HtmlPreviewModal from './HtmlPreviewModal.vue'

const props = defineProps<{
  projectId: number
  filePath: string
  content: string
  fileSize?: number
  lines?: number
}>()

const isModalVisible = ref(false)
const previewUrl = ref('')
const isLoadingTicket = ref(false)
const errorMessage = ref('')

const fileName = computed(() => {
  const parts = props.filePath.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1] || props.filePath
})

function formatBytes(bytes?: number): string {
  if (bytes === undefined || bytes === null || isNaN(bytes)) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

const formattedSize = computed(() => formatBytes(props.fileSize))

async function openInBrowserModal() {
  if (isLoadingTicket.value) return
  isLoadingTicket.value = true
  errorMessage.value = ''
  try {
    const ticket = await createPreviewTicket(props.projectId)
    previewUrl.value = getPreviewFileUrl(props.projectId, ticket, props.filePath)
    isModalVisible.value = true
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '获取预览凭据失败'
  } finally {
    isLoadingTicket.value = false
  }
}
</script>

<template>
  <div class="html-artifact-view" data-test="html-artifact-view">
    <!-- Top toolbar with '预览' and file metadata -->
    <div class="html-toolbar" data-test="html-toolbar">
      <div class="toolbar-left">
        <button
          type="button"
          class="browse-btn"
          :disabled="isLoadingTicket"
          title="在独立弹窗中渲染预览此 HTML 页面"
          data-test="browse-in-browser-btn"
          @click="openInBrowserModal"
        >
          <span class="btn-icon">
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6">
              <circle cx="8" cy="8" r="6.5" />
              <ellipse cx="8" cy="8" rx="3" ry="6.5" />
              <line x1="1.5" y1="8" x2="14.5" y2="8" />
            </svg>
          </span>
          <span>{{ isLoadingTicket ? '准备预览中…' : '预览' }}</span>
        </button>
        <span v-if="errorMessage" class="error-inline">{{ errorMessage }}</span>
      </div>

      <div class="toolbar-meta">
        <span v-if="props.lines !== undefined && props.lines > 0" class="meta-item">
          {{ props.lines }} 行
        </span>
        <span v-if="formattedSize" class="meta-item">
          {{ formattedSize }}
        </span>
        <span v-if="props.filePath" class="meta-item path-item" :title="props.filePath">
          {{ props.filePath }}
        </span>
      </div>
    </div>

    <!-- Code view body -->
    <div class="html-source-area">
      <CodeView
        :content="props.content"
        :file-path="props.filePath"
      />
    </div>

    <!-- HTML Preview Modal -->
    <HtmlPreviewModal
      :visible="isModalVisible"
      :file-name="fileName"
      :preview-url="previewUrl"
      :file-path="props.filePath"
      @close="isModalVisible = false"
    />
  </div>
</template>

<style scoped>
.html-artifact-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--color-surface, #ffffff);
  overflow: hidden;
}

.html-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 36px;
  padding: 0 12px;
  background: var(--color-surface-muted, #f8fafc);
  border-bottom: 1px solid var(--color-border, #e3e6eb);
  flex-shrink: 0;
  font-size: 0.78rem;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.browse-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 10px;
  border-radius: 4px;
  border: 1px solid var(--color-primary, #2563eb);
  background: var(--color-primary, #2563eb);
  color: #ffffff;
  font-size: 0.76rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  box-shadow: 0 1px 2px rgba(37, 99, 235, 0.15);
}

.browse-btn:hover:not(:disabled) {
  background: var(--color-primary-hover, #1d4ed8);
  border-color: var(--color-primary-hover, #1d4ed8);
}

.browse-btn:disabled {
  opacity: 0.65;
  cursor: not-allowed;
}

.btn-icon {
  font-size: 0.85rem;
  line-height: 1;
}

.error-inline {
  color: var(--color-danger, #b91c1c);
  font-size: 0.75rem;
}

.toolbar-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--color-text-muted);
  font-size: 0.75rem;
  overflow: hidden;
}

.meta-item {
  white-space: nowrap;
}

.path-item {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  direction: rtl;
  text-align: left;
}

.html-source-area {
  flex: 1;
  min-height: 0;
  position: relative;
}
</style>
