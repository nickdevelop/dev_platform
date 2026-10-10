<script setup lang="ts">
// Projects view: lists registered projects with search and the management
// actions. The action dialogs are owned by ProjectManageDialog; this view only
// chooses which one is open and reloads the list after a successful change.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { describeApiError, UnauthorizedError } from '../api/client'
import { listProjects, type Project } from '../api/projects'
import { useAuthStore } from '../stores/auth'
import ProjectManageDialog from '../components/ProjectManageDialog.vue'
import WorkspaceRootsDialog from '../components/WorkspaceRootsDialog.vue'

type DialogKind = 'register' | 'create' | 'move' | 'rename' | 'unregister' | 'delete'

const router = useRouter()
const auth = useAuthStore()

const projects = ref<Project[]>([])
const loading = ref(true)
const pageError = ref('')
const search = ref('')

const dialog = ref<{ kind: DialogKind; project: Project | null } | null>(null)
const showRoots = ref(false)

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

function openAgents(project: Project) {
  void router.push({ name: 'project-agents', params: { id: String(project.id) } })
}

function openDialog(kind: DialogKind, project: Project | null = null) {
  dialog.value = { kind, project }
}

function closeDialog() {
  dialog.value = null
}

async function onSubmitted() {
  await load()
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
      <button type="button" class="manage-roots" @click="showRoots = true">管理授权工作区</button>
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
          <button type="button" class="agents" @click="openAgents(project)">Agent 设置</button>
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

    <WorkspaceRootsDialog v-if="showRoots" @close="showRoots = false" />

    <ProjectManageDialog
      v-if="dialog"
      :kind="dialog.kind"
      :project="dialog.project"
      @close="closeDialog"
      @submitted="onSubmitted"
    />
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
