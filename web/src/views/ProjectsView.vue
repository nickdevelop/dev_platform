<script setup lang="ts">
// Projects view: lists registered projects with search, and offers the six
// management actions. Unregister (keeps the directory) and delete (removes
// the directory) are deliberately separate, explicitly labeled flows; delete
// always requires the typed project name and, when the server reports the
// OS trash unavailable, an explicit permanent-delete confirmation.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ApiError, describeApiError, UnauthorizedError } from '../api/client'
import {
  createProject,
  deleteProject,
  listProjects,
  moveProject,
  registerProject,
  renameProject,
  unregisterProject,
  type Project,
} from '../api/projects'
import { useAuthStore } from '../stores/auth'
import ProjectDialog from '../components/ProjectDialog.vue'

type DialogKind = 'register' | 'create' | 'move' | 'rename' | 'unregister' | 'delete'

const router = useRouter()
const auth = useAuthStore()

const projects = ref<Project[]>([])
const loading = ref(true)
const pageError = ref('')
const search = ref('')

const dialog = ref<{ kind: DialogKind; project: Project | null } | null>(null)
const dialogError = ref('')
const submitting = ref(false)
const pathInput = ref('')
const nameInput = ref('')
const confirmNameInput = ref('')
const permanent = ref(false)
const trashUnavailable = ref(false)

const dialogKind = computed(() => dialog.value?.kind ?? null)
const activeProject = computed(() => dialog.value?.project ?? null)

const dialogTitle = computed(() => {
  const name = activeProject.value?.name
  switch (dialogKind.value) {
    case 'register':
      return '注册已有项目目录'
    case 'create':
      return '新建项目目录'
    case 'move':
      return `移动项目 ${name}`
    case 'rename':
      return `重命名项目 ${name}`
    case 'unregister':
      return '移除平台记录（保留目录）'
    case 'delete':
      return '删除项目目录'
    default:
      return ''
  }
})

const confirmLabel = computed(() => {
  switch (dialogKind.value) {
    case 'register':
      return '注册'
    case 'create':
      return '创建'
    case 'move':
      return '移动'
    case 'rename':
      return '重命名'
    case 'unregister':
      return '移除记录'
    case 'delete':
      return '删除目录'
    default:
      return '确定'
  }
})

const needsTypedConfirmation = computed(
  () => dialogKind.value === 'move' || dialogKind.value === 'rename' || dialogKind.value === 'delete',
)

const typedNameMatches = computed(
  () => !!activeProject.value && confirmNameInput.value === activeProject.value.name,
)

const canSubmit = computed(() => {
  if (submitting.value) return false
  if (needsTypedConfirmation.value && !typedNameMatches.value) return false
  if (dialogKind.value === 'delete' && trashUnavailable.value && !permanent.value) return false
  return true
})

const filteredProjects = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (query === '') return projects.value
  return projects.value.filter(
    (project) =>
      project.name.toLowerCase().includes(query) || project.path.toLowerCase().includes(query),
  )
})

onMounted(load)

async function load() {
  loading.value = true
  try {
    const list = await listProjects()
    projects.value = list.projects
    pageError.value = ''
  } catch (error) {
    if (error instanceof UnauthorizedError) return // global handler went to login
    pageError.value = describeApiError(error)
  } finally {
    loading.value = false
  }
}

function openDialog(kind: DialogKind, project: Project | null = null) {
  dialog.value = { kind, project }
  dialogError.value = ''
  pathInput.value = ''
  nameInput.value = ''
  confirmNameInput.value = ''
  permanent.value = false
  trashUnavailable.value = false
}

function closeDialog() {
  dialog.value = null
}

async function submitDialog() {
  const current = dialog.value
  if (!current || !canSubmit.value) return
  submitting.value = true
  dialogError.value = ''
  try {
    switch (current.kind) {
      case 'register':
        await registerProject(pathInput.value.trim(), auth.csrfToken)
        break
      case 'create':
        await createProject(pathInput.value.trim(), nameInput.value.trim(), auth.csrfToken)
        break
      case 'move':
        await moveProject(
          current.project!.id,
          pathInput.value.trim(),
          confirmNameInput.value,
          auth.csrfToken,
        )
        break
      case 'rename':
        await renameProject(
          current.project!.id,
          nameInput.value.trim(),
          confirmNameInput.value,
          auth.csrfToken,
        )
        break
      case 'unregister':
        await unregisterProject(current.project!.id, auth.csrfToken)
        break
      case 'delete':
        await deleteProject(
          current.project!.id,
          confirmNameInput.value,
          permanent.value,
          auth.csrfToken,
        )
        break
    }
    closeDialog()
    await load()
  } catch (error) {
    if (error instanceof UnauthorizedError) return // global handler went to login
    if (error instanceof ApiError && error.code === 'trash_unavailable' && current.kind === 'delete') {
      // The server refused to delete without the trash; nothing was removed.
      // Offer the explicit permanent-delete confirmation.
      trashUnavailable.value = true
      dialogError.value = '系统回收站不可用，目录未被删除。如确认放弃该目录，请勾选永久删除后重试。'
    } else {
      dialogError.value = describeApiError(error)
    }
  } finally {
    submitting.value = false
  }
}

async function logout() {
  try {
    await auth.logout()
  } catch {
    // Logout failures are never fatal for the UI; state is reset either way.
  }
  await router.push({ name: 'login' })
}
</script>

<template>
  <main class="projects">
    <header class="projects-header">
      <h1>AI 开发项目管理平台</h1>
      <button type="button" class="logout" @click="logout">退出登录</button>
    </header>

    <div class="projects-toolbar">
      <input
        id="project-search"
        v-model="search"
        type="search"
        placeholder="按名称或路径筛选"
        aria-label="搜索项目"
      />
      <button type="button" class="register" @click="openDialog('register')">注册已有目录</button>
      <button type="button" class="create" @click="openDialog('create')">新建项目</button>
    </div>

    <p v-if="pageError" class="page-error" role="alert">{{ pageError }}</p>
    <p v-if="loading" class="loading">加载中…</p>

    <ul v-else class="project-list">
      <li v-for="project in filteredProjects" :key="project.id" class="project-item">
        <div class="project-info">
          <span class="project-name">{{ project.name }}</span>
          <span class="project-path">{{ project.path }}</span>
        </div>
        <div class="project-actions">
          <button type="button" @click="openDialog('rename', project)">重命名</button>
          <button type="button" @click="openDialog('move', project)">移动</button>
          <button type="button" class="unregister" @click="openDialog('unregister', project)">
            移除记录
          </button>
          <button type="button" class="delete" @click="openDialog('delete', project)">
            删除目录
          </button>
        </div>
      </li>
      <li v-if="filteredProjects.length === 0" class="empty">没有匹配的项目。</li>
    </ul>

    <ProjectDialog
      v-if="dialog"
      :title="dialogTitle"
      :danger="dialogKind === 'delete'"
      @close="closeDialog"
    >
      <form class="dialog-form" @submit.prevent="submitDialog">
        <template v-if="dialogKind === 'register'">
          <label for="dialog-path">目录的绝对路径</label>
          <input id="dialog-path" v-model="pathInput" type="text" autocomplete="off" spellcheck="false" />
          <p class="hint">目录必须位于已授权的工作区内；平台只记录路径，不会修改目录内容。</p>
        </template>

        <template v-else-if="dialogKind === 'create'">
          <label for="dialog-path">父目录的绝对路径</label>
          <input id="dialog-path" v-model="pathInput" type="text" autocomplete="off" spellcheck="false" />
          <label for="dialog-name">项目目录名</label>
          <input id="dialog-name" v-model="nameInput" type="text" autocomplete="off" spellcheck="false" />
          <p class="hint">将在父目录下创建同名新目录；已存在的目标不会被覆盖。</p>
        </template>

        <template v-else-if="dialogKind === 'move'">
          <p>把项目 {{ activeProject?.name }}（{{ activeProject?.path }}）移动到新位置。</p>
          <label for="dialog-path">目标完整路径</label>
          <input id="dialog-path" v-model="pathInput" type="text" autocomplete="off" spellcheck="false" />
          <label for="dialog-confirm-name">输入当前项目名“{{ activeProject?.name }}”以确认移动</label>
          <input id="dialog-confirm-name" v-model="confirmNameInput" class="confirm-name" type="text" autocomplete="off" />
        </template>

        <template v-else-if="dialogKind === 'rename'">
          <label for="dialog-name">新目录名</label>
          <input id="dialog-name" v-model="nameInput" type="text" autocomplete="off" spellcheck="false" />
          <label for="dialog-confirm-name">输入当前项目名“{{ activeProject?.name }}”以确认重命名</label>
          <input id="dialog-confirm-name" v-model="confirmNameInput" class="confirm-name" type="text" autocomplete="off" />
        </template>

        <template v-else-if="dialogKind === 'unregister'">
          <p>将从平台移除项目 {{ activeProject?.name }} 的记录。</p>
          <p class="safe-note">
            磁盘上的目录 {{ activeProject?.path }} 会被完整保留，不受影响。
          </p>
        </template>

        <template v-else-if="dialogKind === 'delete'">
          <p class="danger-note">项目 {{ activeProject?.name }} 的目录将被删除！</p>
          <p>平台会优先把目录 {{ activeProject?.path }} 移入系统回收站。</p>
          <p v-if="trashUnavailable" class="danger-note">
            系统回收站不可用。若继续，只能永久删除该目录，且无法恢复。
          </p>
          <label v-if="trashUnavailable" class="permanent-option">
            <input v-model="permanent" type="checkbox" />
            我确认永久删除该目录（不进回收站，无法恢复）
          </label>
          <label for="dialog-confirm-name">输入项目名“{{ activeProject?.name }}”以确认删除</label>
          <input id="dialog-confirm-name" v-model="confirmNameInput" class="confirm-name" type="text" autocomplete="off" />
        </template>

        <p v-if="dialogError" class="dialog-error" role="alert">{{ dialogError }}</p>

        <div class="dialog-actions">
          <button type="button" class="cancel" @click="closeDialog">取消</button>
          <button type="button" class="confirm" :disabled="!canSubmit" @click="submitDialog">
            {{ confirmLabel }}
          </button>
        </div>
      </form>
    </ProjectDialog>
  </main>
</template>

<style scoped>
.projects {
  max-width: 60rem;
  margin: 0 auto;
  padding: 1rem;
}

.projects-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.projects-header h1 {
  font-size: 1.25rem;
  margin: 0;
}

.projects-toolbar {
  display: flex;
  gap: 0.5rem;
  margin: 1rem 0;
}

.projects-toolbar input[type='search'] {
  flex: 1;
  padding: 0.4rem 0.6rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
}

.project-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.project-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.6rem 0.9rem;
}

.project-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.project-name {
  font-weight: 600;
}

.project-path {
  color: #64748b;
  font-size: 0.85rem;
  word-break: break-all;
}

.project-actions {
  display: flex;
  gap: 0.4rem;
  flex-shrink: 0;
}

.project-actions button,
.projects-toolbar button,
.dialog-actions button {
  padding: 0.35rem 0.7rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  cursor: pointer;
}

.project-actions .delete {
  color: #b91c1c;
  border-color: #fca5a5;
}

.page-error {
  color: #b91c1c;
}

.dialog-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dialog-form label {
  font-weight: 600;
  margin-top: 0.4rem;
}

.dialog-form input[type='text'],
.dialog-form input[type='search'] {
  padding: 0.4rem 0.6rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
}

.dialog-form .hint {
  color: #64748b;
  font-size: 0.85rem;
  margin: 0;
}

.dialog-form .safe-note {
  color: #15803d;
}

.dialog-form .danger-note {
  color: #b91c1c;
}

.dialog-form .permanent-option {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-weight: 400;
}

.dialog-error {
  color: #b91c1c;
  margin: 0;
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 0.75rem;
}

.dialog-actions .confirm {
  background: #2563eb;
  border-color: #2563eb;
  color: #fff;
}

.dialog-actions .confirm:disabled {
  background: #93c5fd;
  border-color: #93c5fd;
  cursor: default;
}

.dialog.danger .dialog-actions .confirm {
  background: #b91c1c;
  border-color: #b91c1c;
}

.dialog.danger .dialog-actions .confirm:disabled {
  background: #fca5a5;
  border-color: #fca5a5;
}
</style>
