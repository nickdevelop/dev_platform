<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useNotesStore, type NoteViewMode } from '../../stores/notes'
import { useProjectsStore } from '../../stores/projects'
import { renderMarkdown } from '../../shared/markdown'
import { UiEmpty, UiSpinner } from '../../shared/ui'

const props = defineProps<{
  noteId?: number
}>()

const route = useRoute()
const notesStore = useNotesStore()
const projectsStore = useProjectsStore()

const localTitle = ref('')
const localContent = ref('')
const localProjectId = ref(0)
const saveTimer = ref<ReturnType<typeof setTimeout> | null>(null)
const lastSavedContent = ref('')
const lastSavedTitle = ref('')
const lastSavedProjectId = ref(0)

const activeNote = computed(() => notesStore.activeNote)
const viewMode = computed(() => notesStore.viewMode)

watch(
  () => props.noteId ?? route.params.noteId,
  async (newId) => {
    if (newId) {
      const numId = Number(newId)
      if (notesStore.activeNoteId !== numId) {
        await notesStore.selectNote(numId)
      }
    }
  },
  { immediate: true },
)

watch(
  () => activeNote.value,
  (note) => {
    if (note) {
      localTitle.value = note.title || ''
      localContent.value = note.content || ''
      localProjectId.value = note.projectId || 0
      lastSavedTitle.value = note.title || ''
      lastSavedContent.value = note.content || ''
      lastSavedProjectId.value = note.projectId || 0
    } else {
      localTitle.value = ''
      localContent.value = ''
      localProjectId.value = 0
      lastSavedTitle.value = ''
      lastSavedContent.value = ''
      lastSavedProjectId.value = 0
    }
  },
  { immediate: true },
)

const renderedHtml = computed(() => {
  if (!localContent.value.trim()) {
    return '<p class="text-platform-muted italic">（空文档，在此输入内容开始记录）</p>'
  }
  return renderMarkdown(localContent.value)
})

function scheduleAutoSave() {
  if (!activeNote.value) return
  if (saveTimer.value) {
    clearTimeout(saveTimer.value)
  }
  saveTimer.value = setTimeout(async () => {
    await doSave()
  }, 800)
}

async function doSave() {
  if (!activeNote.value) return
  if (saveTimer.value) {
    clearTimeout(saveTimer.value)
    saveTimer.value = null
  }

  const titleChanged = localTitle.value.trim() !== lastSavedTitle.value
  const contentChanged = localContent.value !== lastSavedContent.value
  const projectChanged = localProjectId.value !== lastSavedProjectId.value

  if (!titleChanged && !contentChanged && !projectChanged) {
    return
  }

  const finalTitle = localTitle.value.trim() || '无标题笔记'
  await notesStore.saveNote(activeNote.value.id, {
    title: finalTitle,
    content: localContent.value,
    projectId: localProjectId.value,
  })

  lastSavedTitle.value = finalTitle
  lastSavedContent.value = localContent.value
  lastSavedProjectId.value = localProjectId.value
}

function onTitleBlur() {
  void doSave()
}

function onTitleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    e.preventDefault()
    void doSave()
  }
}

function onContentInput() {
  scheduleAutoSave()
}

function onProjectChange(e: Event) {
  const target = e.target as HTMLSelectElement
  localProjectId.value = Number(target.value)
  void doSave()
}

function handleKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    void doSave()
  }
}

// Tab key indentation support in textarea
function onTextareaKeydown(e: KeyboardEvent) {
  if (e.key === 'Tab') {
    e.preventDefault()
    const target = e.target as HTMLTextAreaElement
    const start = target.selectionStart
    const end = target.selectionEnd
    localContent.value =
      localContent.value.substring(0, start) + '  ' + localContent.value.substring(end)
    scheduleAutoSave()
    void nextTick(() => {
      target.selectionStart = target.selectionEnd = start + 2
    })
  }
}

function setMode(mode: NoteViewMode) {
  notesStore.setViewMode(mode)
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
  if (saveTimer.value) {
    clearTimeout(saveTimer.value)
  }
})
</script>

<template>
  <div class="notes-workspace" aria-label="笔记编辑工作区">
    <div v-if="!activeNote" class="notes-empty-state">
      <UiEmpty
        title="未选择任何笔记"
        description="从左侧列表选择一篇笔记，或点击左上角新建"
      />
    </div>

    <div v-else class="notes-editor-container">
      <!-- 顶部工作区工具栏 -->
      <header class="notes-toolbar">
        <div class="toolbar-left">
          <input
            v-model="localTitle"
            type="text"
            class="note-title-input"
            placeholder="无标题笔记"
            aria-label="笔记标题"
            @blur="onTitleBlur"
            @keydown="onTitleKeydown"
          />
          <div class="project-binding">
            <label class="project-label" for="note-project-select">项目:</label>
            <select
              id="note-project-select"
              class="note-project-selector"
              :value="localProjectId"
              aria-label="关联项目"
              @change="onProjectChange"
            >
              <option :value="0">全局 (未关联)</option>
              <option
                v-for="project in projectsStore.projects"
                :key="project.id"
                :value="project.id"
              >
                {{ project.name }}
              </option>
            </select>
          </div>
        </div>

        <div class="toolbar-right">
          <!-- 保存状态 -->
          <div class="save-status">
            <span v-if="notesStore.saving" class="status-saving">
              <UiSpinner size="xs" />
              <span>保存中...</span>
            </span>
            <span v-else class="status-saved" title="修改后自动保存 (快捷键 Ctrl+S)">
              <svg class="w-3 h-3 text-emerald-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                <polyline points="20 6 9 17 4 12" />
              </svg>
              <span>已保存</span>
            </span>
          </div>

          <!-- 模式切换按钮组 -->
          <div class="view-mode-tabs" role="tablist" aria-label="视图模式切换">
            <button
              type="button"
              class="mode-tab-btn"
              :class="{ active: viewMode === 'split' }"
              title="双栏分屏"
              @click="setMode('split')"
            >
              <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="3" y="3" width="18" height="18" rx="2" />
                <line x1="12" y1="3" x2="12" y2="21" />
              </svg>
              <span>分屏</span>
            </button>
            <button
              type="button"
              class="mode-tab-btn"
              :class="{ active: viewMode === 'edit' }"
              title="仅编辑"
              @click="setMode('edit')"
            >
              <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 20h9" />
                <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
              </svg>
              <span>编辑</span>
            </button>
            <button
              type="button"
              class="mode-tab-btn"
              :class="{ active: viewMode === 'preview' }"
              title="仅预览"
              @click="setMode('preview')"
            >
              <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                <circle cx="12" cy="12" r="3" />
              </svg>
              <span>预览</span>
            </button>
          </div>
        </div>
      </header>

      <!-- 编辑与预览主体区 -->
      <main class="notes-content-grid" :class="`mode-${viewMode}`">
        <!-- 编辑器面板 -->
        <section v-show="viewMode !== 'preview'" class="notes-pane editor-pane" aria-label="Markdown 源码编辑">
          <textarea
            v-model="localContent"
            class="note-textarea"
            placeholder="支持 Markdown 语法，在这里记录你的设计或备忘..."
            aria-label="笔记正文"
            @input="onContentInput"
            @keydown="onTextareaKeydown"
          />
        </section>

        <!-- 渲染预览面板 -->
        <section v-show="viewMode !== 'edit'" class="notes-pane preview-pane" aria-label="Markdown 实时预览">
          <div class="markdown-body preview-body" v-html="renderedHtml" />
        </section>
      </main>
    </div>
  </div>
</template>

<style scoped>
.notes-workspace {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  overflow: hidden;
  background: var(--color-bg);
}

.notes-empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  width: 100%;
}

.notes-editor-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  min-height: 0;
  overflow: hidden;
}

/* 顶部紧凑工具栏 */
.notes-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 16px;
  height: 42px;
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1;
}

.note-title-input {
  font-size: var(--text-size-14);
  font-weight: 600;
  color: var(--color-text);
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--radius, 4px);
  padding: 2px 6px;
  outline: none;
  width: 280px;
  max-width: 50%;
  transition: border-color 0.12s ease-out, background-color 0.12s ease-out;
}

.note-title-input:hover,
.note-title-input:focus {
  background: var(--color-surface-muted);
  border-color: var(--color-border);
}

.project-binding {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.project-label {
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
}

.note-project-selector {
  height: 24px;
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  color: var(--color-text);
  border-radius: var(--radius, 3px);
  padding: 0 6px;
  font-size: var(--text-size-12);
  outline: none;
  cursor: pointer;
  transition: border-color 0.12s ease-out;
}

.note-project-selector:hover {
  border-color: var(--color-border-strong);
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.save-status {
  display: flex;
  align-items: center;
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
}

.status-saving,
.status-saved {
  display: flex;
  align-items: center;
  gap: 4px;
}

.status-saved {
  color: var(--color-text-muted);
}

.view-mode-tabs {
  display: flex;
  align-items: center;
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 4px);
  padding: 2px;
  gap: 2px;
}

.mode-tab-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  height: 22px;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  border-radius: 2px;
  font-size: var(--text-size-12);
  cursor: pointer;
  transition: color 0.12s ease-out, background-color 0.12s ease-out;
}

.mode-tab-btn:hover {
  color: var(--color-text);
}

.mode-tab-btn.active {
  background: var(--color-surface);
  color: var(--color-accent, #4a8ad6);
  font-weight: 500;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
}

/* 主编辑与预览网格 */
.notes-content-grid {
  display: flex;
  flex: 1;
  min-height: 0;
  width: 100%;
  overflow: hidden;
}

.notes-content-grid.mode-split .notes-pane {
  flex: 1;
  width: 50%;
}

.notes-content-grid.mode-split .editor-pane {
  border-right: 1px solid var(--color-border);
}

.notes-content-grid.mode-edit .editor-pane {
  flex: 1;
  width: 100%;
}

.notes-content-grid.mode-preview .preview-pane {
  flex: 1;
  width: 100%;
}

.notes-pane {
  height: 100%;
  overflow: hidden;
  position: relative;
}

.note-textarea {
  width: 100%;
  height: 100%;
  padding: 16px 20px;
  background: var(--color-bg);
  color: var(--color-text);
  font-family: var(--font-mono);
  font-size: var(--text-size-13);
  line-height: 1.6;
  border: none;
  outline: none;
  resize: none;
  tab-size: 2;
  box-sizing: border-box;
}

.preview-pane {
  overflow-y: auto;
  background: var(--color-surface);
}

.preview-body {
  padding: 16px 24px;
  max-width: 900px;
  margin: 0 auto;
}
</style>
