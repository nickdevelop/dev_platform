<script setup lang="ts">
// ServerDirectoryPicker: browse the server's filesystem and pick a directory.
//
// Shared by the proxy editor (选择服务器目录) and the project manager (选择工作
// 目录), so its props and emits are a contract with two callers — the migration
// to Element Plus changed the chrome only, never the interface.
//
// The row list stays a list rather than an `el-table`: this is a single-select
// file browser where double-click descends, and a table would buy nothing but
// column chrome. Everything that *is* a control — the dialog, the buttons, the
// loading overlay — comes from the library.
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Folder } from '@element-plus/icons-vue'
import { browseDirectories, type DirectoryEntry } from '../../api/proxies'

const props = defineProps<{
  initialPath?: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'select', path: string): void
}>()

const visible = ref(true)
const currentPath = ref('')
const parentPath = ref('')
const entries = ref<DirectoryEntry[]>([])
const loading = ref(false)
const selectedPath = ref('')

// `canConfirm` replaces the old "show an error if the read failed" banner: with
// nothing selected and nothing loaded there is nothing to confirm, so the button
// says so instead of a message the user has to dismiss.
const canConfirm = computed(() => !loading.value && Boolean(selectedPath.value || currentPath.value))

async function load(path?: string) {
  loading.value = true
  try {
    const res = await browseDirectories(path)
    currentPath.value = res.current
    parentPath.value = res.parent
    entries.value = res.entries
    selectedPath.value = res.current
  } catch (err: any) {
    // Keep whatever is on screen: a failed descent must not blank the list the
    // user was reading. The failure is reported once, as a light toast.
    ElMessage.error(err?.error || err?.message || '读取目录失败')
  } finally {
    loading.value = false
  }
}

function selectEntry(entry: DirectoryEntry) {
  selectedPath.value = entry.path
}

function openEntry(entry: DirectoryEntry) {
  load(entry.path)
}

function goParent() {
  if (parentPath.value) load(parentPath.value)
}

function confirmSelection() {
  const chosen = selectedPath.value || currentPath.value
  if (chosen) {
    visible.value = false
    emit('select', chosen)
  }
}

function onClose() {
  emit('close')
}

onMounted(() => {
  load(props.initialPath)
})
</script>

<template>
  <el-dialog
    v-model="visible"
    class="entity-dialog dir-picker-dialog"
    width="620"
    :close-on-click-modal="false"
    @closed="onClose"
  >
    <template #header>
      <div class="entity-dialog-head">
        <div class="min-w-0">
          <h2 class="entity-dialog-title">选择服务器目录</h2>
          <p class="entity-dialog-subtitle">单击选中目录，双击进入下一级</p>
        </div>
      </div>
    </template>

    <div class="dir-picker-body">
      <div class="dir-path-bar">
        <el-button
          link
          type="primary"
          :icon="ArrowLeft"
          :disabled="!parentPath || loading"
          title="返回上一级目录"
          data-test="dir-picker-up"
          @click="goParent"
        >
          上一级
        </el-button>
        <span class="dir-path-sep" aria-hidden="true">|</span>
        <code class="dir-path font-mono" :title="currentPath">{{ currentPath || '正在定位路径…' }}</code>
      </div>

      <div v-loading="loading" class="dir-list" data-test="dir-picker-list">
        <p v-if="!loading && entries.length === 0" class="dir-empty">当前目录下无其他子目录</p>
        <ul v-else>
          <li
            v-for="entry in entries"
            :key="entry.path"
            class="dir-row"
            :class="{ 'is-selected': selectedPath === entry.path }"
            :data-test="`dir-entry-${entry.name}`"
            @click="selectEntry(entry)"
            @dblclick="openEntry(entry)"
          >
            <span class="dir-row-name">
              <el-icon class="dir-glyph" :size="14"><Folder /></el-icon>
              <span class="truncate">{{ entry.name }}</span>
            </span>
            <el-button
              link
              type="primary"
              size="small"
              title="进入此目录"
              @click.stop="openEntry(entry)"
            >
              进入
            </el-button>
          </li>
        </ul>
      </div>

      <p class="dir-selected">
        <span class="text-platform-text-muted">选中路径：</span>
        <code class="font-mono">{{ selectedPath || currentPath }}</code>
      </p>
    </div>

    <template #footer>
      <div class="entity-dialog-actions">
        <el-button @click="visible = false">取消</el-button>
        <el-button
          type="primary"
          :disabled="!canConfirm"
          data-test="dir-picker-confirm"
          @click="confirmSelection"
        >
          选择此目录
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.dir-picker-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.dir-path-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-muted);
}

.dir-path-sep {
  color: var(--color-border-strong);
  user-select: none;
}

.dir-path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--text-size-12);
  user-select: all;
}

/* The scroller is fixed so the dialog never grows past the viewport; the
   library's dialog already caps its own height (element-tokens.css). */
.dir-list {
  min-height: 240px;
  max-height: 320px;
  overflow-y: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
}

.dir-list ul {
  margin: 0;
  padding: 0;
  list-style: none;
}

.dir-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 10px;
  border-bottom: 1px solid var(--color-border);
  font-size: var(--text-size-12);
  cursor: pointer;
  user-select: none;
}

.dir-row:last-child {
  border-bottom: none;
}

.dir-row:hover {
  background: var(--color-surface-muted);
}

/* Selection is a hairline accent inset rather than a filled row: the row also
   has to read as clickable, and a solid block would compete with the button. */
.dir-row.is-selected {
  background: var(--color-surface-muted);
  box-shadow: inset 2px 0 0 0 var(--color-accent);
  font-weight: 500;
}

.dir-row-name {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.dir-glyph {
  flex: none;
  color: var(--color-warning);
}

.dir-empty {
  margin: 0;
  padding: 48px 12px;
  text-align: center;
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
}

.dir-selected {
  display: flex;
  gap: 4px;
  margin: 0;
  font-size: var(--text-size-11);
}

.dir-selected code {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
