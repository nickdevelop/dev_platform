<script setup lang="ts">
// NotesSidebar: the notes centre's second column.
//
// Three zones, same as every other second column: an action row (Zone A), a
// filter row (Zone B) and the list itself (Zone C). The list used to be a stack
// of hand-drawn `.sidebar-card` buttons, which meant we owned the roving
// tabindex, the aria roles and the active edge ourselves; it is now an
// `el-menu` in router mode, the same shape as OrgSidebar / EnginesSidebar /
// SettingsNav, and the visual contract for those rows lives in
// `theme/element-tokens.css`.
//
// Zone B filters apply live. That is deliberate and differs from a list page's
// toolbar (where the keyword is a draft until 搜索): this is navigation, not a
// query — there is no search button here, and a note you cannot see is a note
// you cannot open.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Notebook, Plus, Search } from '@element-plus/icons-vue'
import type { Note } from '../../api/notes'
import { useNotesStore } from '../../stores/notes'
import { useProjectsStore } from '../../stores/projects'
import ListEmptyState from '../../shared/components/ListEmptyState.vue'
import NoteItem from './NoteItem.vue'

const route = useRoute()
const router = useRouter()
const notesStore = useNotesStore()
const projectsStore = useProjectsStore()

/** el-select needs one primitive for "all / global / a project"; the store
    keeps the same three cases as null / 0 / id. */
type ProjectFilter = 'all' | 'global' | number

const deletingNote = ref<Note | null>(null)

onMounted(async () => {
  await projectsStore.load()
  await notesStore.loadNotes()
})

const filteredNotes = computed(() => notesStore.filteredNotes)

const activeIndex = computed(() => route.path)

const isFiltered = computed(
  () => notesStore.searchQuery.trim().length > 0 || notesStore.selectedProjectId !== null,
)

const emptyState = computed<'loading' | 'filtered' | 'empty'>(() => {
  if (notesStore.loading && filteredNotes.value.length === 0) return 'loading'
  return isFiltered.value ? 'filtered' : 'empty'
})

const searchQuery = computed({
  get: () => notesStore.searchQuery,
  set: (value: string) => notesStore.setSearchQuery(value),
})

const projectFilter = computed<ProjectFilter>({
  get: () =>
    notesStore.selectedProjectId === null
      ? 'all'
      : notesStore.selectedProjectId === 0
        ? 'global'
        : notesStore.selectedProjectId,
  set: (value) =>
    notesStore.setSelectedProjectId(value === 'all' ? null : value === 'global' ? 0 : value),
})

function resetFilters() {
  notesStore.setSearchQuery('')
  notesStore.setSelectedProjectId(null)
}

async function onNewNote() {
  const created = await notesStore.createNewNote()
  if (created) {
    void router.push({ name: 'note-detail', params: { noteId: String(created.id) } })
  }
}

async function askDelete(note: Note) {
  deletingNote.value = note
  const title = note.title?.trim() || '无标题笔记'
  try {
    await ElMessageBox.confirm(`确定要删除笔记「${title}」吗？此操作不可恢复。`, '删除笔记', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      confirmButtonType: 'danger',
    })
  } catch {
    // Dismissed is an answer, not an error.
    deletingNote.value = null
    return
  }
  deletingNote.value = null
  try {
    await notesStore.removeNote(note.id)
    ElMessage.success(`已删除笔记「${title}」`)
  } catch {
    // describeApiError already landed in notesStore.error; the failure is
    // reported once, by the workspace, not here on top of it.
    return
  }
  if (notesStore.activeNoteId) {
    void router.push({ name: 'note-detail', params: { noteId: String(notesStore.activeNoteId) } })
  } else {
    void router.push({ name: 'notes' })
  }
}
</script>

<template>
  <nav class="notes-sidebar sidebar-panel" data-test="notes-sidebar" aria-label="笔记列表">
    <!-- Zone A: the one action this column offers -->
    <div class="sidebar-actions">
      <el-button
        type="primary"
        class="w-full"
        :icon="Plus"
        data-test="new-note-btn"
        @click="onNewNote"
      >
        新建笔记
      </el-button>
    </div>

    <!-- Zone B: live navigation filters -->
    <div class="notes-filters">
      <el-input
        v-model="searchQuery"
        class="notes-search"
        size="small"
        placeholder="搜索笔记"
        clearable
        :prefix-icon="Search"
        aria-label="搜索笔记"
        data-test="notes-search"
      />
      <el-select
        v-model="projectFilter"
        class="notes-project-select"
        size="small"
        data-test="notes-project-filter"
      >
        <el-option label="全部笔记" value="all" />
        <el-option label="全局笔记" value="global" />
        <el-option
          v-for="project in projectsStore.projects"
          :key="project.id"
          :label="project.name"
          :value="project.id"
        >
          <span>{{ project.name }}</span>
          <span class="entity-option-sub">项目笔记</span>
        </el-option>
      </el-select>
    </div>

    <!-- Zone C: the note list -->
    <div class="notes-menu-scroll">
      <el-menu
        v-if="filteredNotes.length"
        class="notes-menu"
        router
        :default-active="activeIndex"
        :default-openeds="[]"
        data-test="notes-menu"
      >
        <el-menu-item
          v-for="note in filteredNotes"
          :key="note.id"
          :index="`/notes/${note.id}`"
          class="entity-row"
          :data-test="`note-item-${note.id}`"
        >
          <NoteItem :note="note" @delete="askDelete" />
        </el-menu-item>
      </el-menu>

      <ListEmptyState
        v-else
        dense
        :rows="4"
        :state="emptyState"
        :icon="Notebook"
        title="还没有笔记"
        hint="笔记是工作区之外的随手记录，可以绑定到某个项目。"
        reset-label="重置筛选"
        @reset="resetFilters"
      >
        <el-button type="primary" size="small" :icon="Plus" @click="onNewNote">新建笔记</el-button>
      </ListEmptyState>
    </div>
  </nav>
</template>

<style scoped>
.notes-sidebar {
  height: 100%;
}

/* Zone B: 248px of content width fits one search box and one filter on a
   single line (the library's own small tier is 24px, so both match). */
.notes-filters {
  display: flex;
  flex-shrink: 0;
  gap: 6px;
  align-items: center;
  padding: 0 12px 8px 12px;
}

.notes-search {
  min-width: 0;
  flex: 1;
}

.notes-project-select {
  flex: none;
  width: 104px;
}

/* The scroller is ours so the menu keeps its own block layout: putting the
   scroll container on the menu root would fight the library's list styles. */
.notes-menu-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0 6px 12px 6px;
}

.notes-menu {
  width: 100%;
}
</style>
