<script setup lang="ts">
// Workspace-root management dialog. Only directories inside an authorized
// workspace root can be registered or created as projects, so a fresh
// deployment must add at least one root here before its first project. List,
// add and remove all go through the authenticated /api/workspaces/roots
// endpoints with the session CSRF token.
import { onMounted, ref } from 'vue'
import { describeApiError, UnauthorizedError } from '../api/client'
import {
  addWorkspaceRoot,
  listWorkspaceRoots,
  removeWorkspaceRoot,
  type WorkspaceRoot,
} from '../api/workspaces'
import { useAuthStore } from '../stores/auth'
import ProjectDialog from './ProjectDialog.vue'

const emit = defineEmits<{ close: [] }>()

const auth = useAuthStore()

const roots = ref<WorkspaceRoot[]>([])
const loading = ref(true)
const error = ref('')
const newPath = ref('')
const submitting = ref(false)

onMounted(load)

async function load() {
  loading.value = true
  try {
    const list = await listWorkspaceRoots()
    roots.value = list.roots
    error.value = ''
  } catch (caught) {
    if (caught instanceof UnauthorizedError) return // global handler went to login
    error.value = describeApiError(caught)
  } finally {
    loading.value = false
  }
}

async function add() {
  const path = newPath.value.trim()
  if (submitting.value || path === '') return
  submitting.value = true
  error.value = ''
  try {
    await addWorkspaceRoot(path, auth.csrfToken)
    newPath.value = ''
    await load()
  } catch (caught) {
    if (caught instanceof UnauthorizedError) return
    error.value = describeApiError(caught)
  } finally {
    submitting.value = false
  }
}

async function remove(root: WorkspaceRoot) {
  if (submitting.value) return
  submitting.value = true
  error.value = ''
  try {
    await removeWorkspaceRoot(root.id, auth.csrfToken)
    await load()
  } catch (caught) {
    if (caught instanceof UnauthorizedError) return
    error.value = describeApiError(caught)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <ProjectDialog title="管理授权工作区" @close="emit('close')">
    <p class="hint">
      只有位于下列已授权工作区根目录内的路径才能注册或创建项目；移除根目录不会删除磁盘内容。
    </p>

    <ul class="root-list">
      <li v-for="root in roots" :key="root.id" class="root-item">
        <span class="root-path">{{ root.path }}</span>
        <button type="button" class="remove-root" @click="remove(root)">移除</button>
      </li>
      <li v-if="!loading && roots.length === 0" class="empty">尚未授权任何工作区根目录。</li>
    </ul>

    <form class="add-root" @submit.prevent="add">
      <label for="new-root-path">新的工作区根目录绝对路径</label>
      <input
        id="new-root-path"
        v-model="newPath"
        type="text"
        autocomplete="off"
        spellcheck="false"
      />
      <button type="submit" class="add-root-button" :disabled="submitting || newPath.trim() === ''">
        添加
      </button>
    </form>

    <p v-if="error" class="dialog-error" role="alert">{{ error }}</p>

    <div class="dialog-actions">
      <button type="button" class="cancel" @click="emit('close')">关闭</button>
    </div>
  </ProjectDialog>
</template>

<style scoped>
.hint {
  color: #64748b;
  font-size: 0.85rem;
  margin: 0 0 0.5rem;
}

.root-list {
  list-style: none;
  margin: 0 0 0.75rem;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.root-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 0.35rem 0.6rem;
}

.root-path {
  word-break: break-all;
}

.empty {
  color: #64748b;
  font-size: 0.9rem;
}

.add-root {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.add-root label {
  font-weight: 600;
}

.add-root input[type='text'] {
  padding: 0.4rem 0.6rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
}

.add-root button,
.remove-root,
.dialog-actions button {
  padding: 0.35rem 0.7rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
}

.dialog-error {
  color: #b91c1c;
  margin: 0.5rem 0 0;
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 0.75rem;
}
</style>
