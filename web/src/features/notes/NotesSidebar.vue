<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { Note } from '../../api/notes'
import { useNotesStore } from '../../stores/notes'
import { useProjectsStore } from '../../stores/projects'
import { UiConfirmDialog, UiEmpty, UiSpinner } from '../../shared/ui'
import NoteItem from './NoteItem.vue'

const router = useRouter()
const notesStore = useNotesStore()
const projectsStore = useProjectsStore()

const deletingNote = ref<Note | null>(null)
const deleteLoading = ref(false)

onMounted(async () => {
  await projectsStore.load()
  await notesStore.loadNotes()
})

const activeId = computed(() => notesStore.activeNoteId)
const filteredNotes = computed(() => notesStore.filteredNotes)

function onSearchInput(e: Event) {
  const target = e.target as HTMLInputElement
  notesStore.setSearchQuery(target.value)
}

function clearSearch() {
  notesStore.setSearchQuery('')
}

function onProjectFilterChange(e: Event) {
  const target = e.target as HTMLSelectElement
  const val = target.value
  if (val === 'all') {
    notesStore.setSelectedProjectId(null)
  } else if (val === 'global') {
    notesStore.setSelectedProjectId(0)
  } else {
    notesStore.setSelectedProjectId(Number(val))
  }
}

async function onNewNote() {
  const created = await notesStore.createNewNote()
  if (created) {
    void router.push({ name: 'note-detail', params: { noteId: String(created.id) } })
  }
}

function onSelectNote(id: number) {
  void notesStore.selectNote(id)
  void router.push({ name: 'note-detail', params: { noteId: String(id) } })
}

function askDelete(note: Note) {
  deletingNote.value = note
}

function cancelDelete() {
  deletingNote.value = null
}

async function confirmDelete() {
  if (!deletingNote.value) return
  deleteLoading.value = true
  try {
    const id = deletingNote.value.id
    await notesStore.removeNote(id)
    deletingNote.value = null
    if (notesStore.activeNoteId) {
      void router.push({ name: 'note-detail', params: { noteId: String(notesStore.activeNoteId) } })
    } else {
      void router.push({ name: 'notes' })
    }
  } finally {
    deleteLoading.value = false
  }
}
</script>

<template>
  <div class="notes-sidebar sidebar-panel" aria-label="笔记列表">
    <!-- Zone A: Top dedicated actions row -->
    <div class="sidebar-actions">
      <button
        type="button"
        class="sidebar-action-btn"
        data-test="new-note-btn"
        title="新建笔记"
        @click="onNewNote"
      >
        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <line x1="12" y1="5" x2="12" y2="19" />
          <line x1="5" y1="12" x2="19" y2="12" />
        </svg>
        <span>新建笔记</span>
      </button>
    </div>

    <!-- Zone B: Search input & Project filter -->
    <div class="sidebar-search-bar">
      <svg class="sidebar-search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor">
        <circle cx="11" cy="11" r="8" />
        <line x1="21" y1="21" x2="16.65" y2="16.65" />
      </svg>
      <input
        type="text"
        class="sidebar-search-input"
        placeholder="搜索笔记..."
        :value="notesStore.searchQuery"
        aria-label="搜索笔记"
        @input="onSearchInput"
      />
      <button
        v-if="notesStore.searchQuery"
        type="button"
        class="sidebar-search-clear"
        title="清空搜索"
        aria-label="清空搜索"
        @click="clearSearch"
      >
        <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <line x1="18" y1="6" x2="6" y2="18" />
          <line x1="6" y1="6" x2="18" y2="18" />
        </svg>
      </button>
    </div>

    <!-- Zone B: Micro filter select for Projects -->
    <div class="sidebar-filter-bar">
      <select
        class="notes-project-select"
        :value="notesStore.selectedProjectId === null ? 'all' : notesStore.selectedProjectId === 0 ? 'global' : String(notesStore.selectedProjectId)"
        aria-label="项目筛选"
        @change="onProjectFilterChange"
      >
        <option value="all">全部笔记</option>
        <option value="global">全局笔记</option>
        <option
          v-for="project in projectsStore.projects"
          :key="project.id"
          :value="String(project.id)"
        >
          项目: {{ project.name }}
        </option>
      </select>
    </div>

    <!-- Zone C: Note list items -->
    <div class="sidebar-list-container">
      <div v-if="notesStore.loading && filteredNotes.length === 0" class="notes-loading">
        <UiSpinner size="sm" />
        <span>加载中...</span>
      </div>

      <UiEmpty
        v-else-if="filteredNotes.length === 0"
        title="暂无笔记"
        description="点击上方新建笔记，随时记录您的思路"
        class="my-auto py-8"
      />

      <NoteItem
        v-for="note in filteredNotes"
        :key="note.id"
        :note="note"
        :active="note.id === activeId"
        @select="onSelectNote"
        @delete="askDelete"
      />
    </div>

    <!-- Delete Confirmation Dialog -->
    <UiConfirmDialog
      v-if="deletingNote"
      title="删除笔记"
      :message="`确定要删除笔记 “${deletingNote.title || '无标题笔记'}” 吗？`"
      detail="此操作将永久删除该笔记，删除后不可恢复。"
      confirm-label="确认删除"
      variant="danger"
      :loading="deleteLoading"
      @close="cancelDelete"
      @confirm="confirmDelete"
    />
  </div>
</template>

<style scoped>
.notes-sidebar {
  height: 100%;
}

.notes-project-select {
  width: 100%;
  height: 24px;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  color: var(--color-text);
  border-radius: var(--radius, 4px);
  padding: 0 6px;
  font-size: var(--text-size-12);
  outline: none;
  cursor: pointer;
  transition: border-color 0.12s ease-out;
}

.notes-project-select:hover {
  border-color: var(--color-border-strong);
}

.notes-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px 0;
  color: var(--color-text-muted);
  font-size: var(--text-size-12);
}
</style>
