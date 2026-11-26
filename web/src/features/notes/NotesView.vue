<script setup lang="ts">
// NotesView: the notes centre's workspace.
//
// The library owns the controls (title, project binding, save state, view
// mode, body); this file owns what is specific to a note: the 800ms autosave,
// Ctrl/Cmd+S, two-space Tab indentation, and the Markdown preview. Everything
// the user can act on is an Element Plus control — see the selection table in
// docs/architecture/frontend-ui-system.md §五.
import { computed, markRaw, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Check, Loading } from '@element-plus/icons-vue'
import { useNotesStore, type NoteViewMode } from '../../stores/notes'
import { useProjectsStore } from '../../stores/projects'
import { renderMarkdown } from '../../shared/markdown'
import { handleCodeCopyClick } from '../../shared/markdown-code-copy'

const props = defineProps<{
  noteId?: number
}>()

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

const viewModeOptions = [
  { label: '分屏', value: 'split' },
  { label: '编辑', value: 'edit' },
  { label: '预览', value: 'preview' },
]

const saveStateIcon = computed(() => (notesStore.saving ? markRaw(Loading) : markRaw(Check)))

/** `el-segmented` writes back whatever `value` the option carried, so the
    store's union type is restored here rather than cast at every call site. */
const viewMode = computed<string>({
  get: () => notesStore.viewMode,
  set: (value) => notesStore.setViewMode(value as NoteViewMode),
})

watch(
  () => props.noteId,
  async (newId) => {
    if (!newId) return
    if (notesStore.activeNoteId !== newId) {
      await notesStore.selectNote(newId)
    }
  },
  { immediate: true },
)

// Load the note into the editor when a *different* note becomes active.
// Watching the id rather than the note object matters: an autosave replaces
// `currentNote` with the server copy, and watching the object would overwrite
// whatever was typed between the request and the response.
watch(
  () => activeNote.value?.id ?? null,
  () => {
    const note = activeNote.value
    localTitle.value = note?.title ?? ''
    localContent.value = note?.content ?? ''
    localProjectId.value = note?.projectId ?? 0
    lastSavedTitle.value = localTitle.value
    lastSavedContent.value = localContent.value
    lastSavedProjectId.value = localProjectId.value
  },
  { immediate: true },
)

const renderedHtml = computed(() => {
  if (!localContent.value.trim()) {
    return '<p class="preview-empty">空文档，在此输入内容开始记录</p>'
  }
  return renderMarkdown(localContent.value)
})

function scheduleAutoSave() {
  if (!activeNote.value) return
  if (saveTimer.value) clearTimeout(saveTimer.value)
  saveTimer.value = setTimeout(() => void doSave(), 800)
}

async function doSave() {
  if (!activeNote.value) return
  if (saveTimer.value) {
    clearTimeout(saveTimer.value)
    saveTimer.value = null
  }

  const finalTitle = localTitle.value.trim() || '无标题笔记'
  const changed =
    finalTitle !== lastSavedTitle.value ||
    localContent.value !== lastSavedContent.value ||
    localProjectId.value !== lastSavedProjectId.value
  if (!changed) return

  try {
    await notesStore.saveNote(activeNote.value.id, {
      title: finalTitle,
      content: localContent.value,
      projectId: localProjectId.value,
    })
    lastSavedTitle.value = finalTitle
    lastSavedContent.value = localContent.value
    lastSavedProjectId.value = localProjectId.value
  } catch {
    // The store already holds the message; the status line shows it, so the
    // failure is reported once instead of a toast on every retry.
  }
}

function handleKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    void doSave()
  }
}

// Tab indents instead of walking out of the editor; the event target is the
// library's inner textarea, which is the only place the selection makes sense.
// `el-input` types its keydown payload as `Event | KeyboardEvent`.
function onBodyKeydown(event: Event | KeyboardEvent) {
  if (!(event instanceof KeyboardEvent) || event.key !== 'Tab') return
  event.preventDefault()
  const target = event.target as HTMLTextAreaElement
  const start = target.selectionStart
  const end = target.selectionEnd
  localContent.value =
    localContent.value.slice(0, start) + '  ' + localContent.value.slice(end)
  scheduleAutoSave()
  void nextTick(() => {
    target.selectionStart = target.selectionEnd = start + 2
  })
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
  void projectsStore.load()
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
  if (saveTimer.value) clearTimeout(saveTimer.value)
})
</script>

<template>
  <div class="notes-workspace" aria-label="笔记编辑工作区">
    <div v-if="!activeNote" class="notes-empty">
      <el-empty :image-size="0" description="从左侧选择一篇笔记，或新建一篇" />
    </div>

    <div v-else class="notes-editor">
      <header class="notes-toolbar">
        <div class="toolbar-left">
          <el-input
            v-model="localTitle"
            class="note-title entity-bare-input"
            size="small"
            placeholder="无标题笔记"
            aria-label="笔记标题"
            data-test="note-title"
            @blur="doSave()"
            @keyup.enter="doSave()"
          />
          <el-select
            v-model="localProjectId"
            class="note-project"
            size="small"
            aria-label="关联项目"
            data-test="note-project-select"
            @change="doSave()"
          >
            <el-option label="全局笔记" :value="0" />
            <el-option
              v-for="project in projectsStore.projects"
              :key="project.id"
              :label="project.name"
              :value="project.id"
            >
              <span>{{ project.name }}</span>
              <span class="entity-option-sub">绑定到该项目</span>
            </el-option>
          </el-select>
        </div>

        <div class="toolbar-right">
          <span
            class="save-state"
            :class="{ 'save-state--saving': notesStore.saving }"
            :title="notesStore.error || '修改后自动保存，Ctrl+S 立即保存'"
            data-test="note-save-state"
          >
            <el-icon :class="{ 'is-loading': notesStore.saving }" :size="12">
              <component :is="saveStateIcon" />
            </el-icon>
            <span>{{ notesStore.saving ? '保存中' : notesStore.error ? '未保存' : '已保存' }}</span>
          </span>
          <el-segmented
            v-model="viewMode"
            :options="viewModeOptions"
            size="small"
            data-test="note-view-mode"
          />
        </div>
      </header>

      <main class="notes-body" :class="`mode-${viewMode}`">
        <section
          v-show="viewMode !== 'preview'"
          class="notes-pane editor-pane"
          aria-label="Markdown 源码编辑"
        >
          <el-input
            v-model="localContent"
            class="note-body-input entity-fill-textarea entity-mono-input"
            type="textarea"
            resize="none"
            placeholder="支持 Markdown 语法，在这里记录你的设计或备忘…"
            aria-label="笔记正文"
            data-test="note-body"
            :input-style="{
              height: '100%',
              padding: '14px 16px',
              fontFamily: 'var(--font-mono)',
              fontSize: 'var(--text-size-13)',
              lineHeight: '1.6',
              tabSize: 2,
            }"
            @input="scheduleAutoSave"
            @keydown="onBodyKeydown"
          />
        </section>

        <section
          v-show="viewMode !== 'edit'"
          class="notes-pane preview-pane"
          aria-label="Markdown 实时预览"
        >
          <div
            class="markdown-body preview-body"
            data-test="note-preview"
            v-html="renderedHtml"
            @click="handleCodeCopyClick"
          />
        </section>
      </main>
    </div>
  </div>
</template>

<style scoped>
.notes-workspace {
  display: flex;
  flex: 1;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-height: 0;
  overflow: hidden;
  background: var(--color-bg);
}

.notes-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
}

.notes-editor {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

/* Toolbar: one row, every control on the same 24px tier. */
.notes-toolbar {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  height: 40px;
  padding: 0 12px;
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
}

.toolbar-left {
  display: flex;
  flex: 1;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.note-title {
  width: 260px;
  max-width: 44%;
  font-size: var(--text-size-14);
  font-weight: 600;
}

.note-project {
  flex: none;
  width: 150px;
}

.toolbar-right {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 10px;
}

/* The save state is a status read-out, not a button: quiet at rest, and it
   only takes colour when a write is in flight or a save failed. */
.save-state {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
  cursor: default;
}

.save-state--saving {
  color: var(--color-accent);
}

.notes-body {
  display: flex;
  flex: 1;
  width: 100%;
  min-height: 0;
  overflow: hidden;
}

.notes-body.mode-split .notes-pane {
  flex: 1;
  width: 50%;
}

.notes-body.mode-split .editor-pane {
  border-right: 1px solid var(--color-border);
}

.notes-body.mode-edit .editor-pane,
.notes-body.mode-preview .preview-pane {
  flex: 1;
  width: 100%;
}

.notes-pane {
  position: relative;
  height: 100%;
  overflow: hidden;
}

/* The body is a document surface, not a form field: it fills the pane and
   drops the field shell. Both halves of that are reached through the token
   layer's `entity-fill-textarea` hook — nothing here names a library part. */
.note-body-input {
  height: 100%;
  background: var(--color-bg);
  color: var(--color-text);
}

.preview-pane {
  overflow-y: auto;
  background: var(--color-surface);
}

.preview-body {
  max-width: 900px;
  padding: 14px 20px 32px;
  margin: 0 auto;
}

.preview-body :deep(.preview-empty) {
  color: var(--color-text-muted);
  font-style: italic;
}
</style>
