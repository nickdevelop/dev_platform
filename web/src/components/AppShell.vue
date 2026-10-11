<script setup lang="ts">
// Three-pane shell: category rail, project/task tree, and the right pane chosen
// by the child route. The shell owns the single project-manage dialog and turns
// tree and panel events into navigation. A project id that is not in the loaded
// list shows a not-found notice instead of any panel, so a stale or foreign URL
// never renders another project's content. The settings center hides the
// project sidebar so it occupies the full pane.
import { computed, onMounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import type { Project } from '../api/projects'
import { useAuthStore } from '../stores/auth'
import { useProjectsStore } from '../stores/projects'
import { useSettingsStore } from '../stores/settings'
import CategoryRail from './CategoryRail.vue'
import ProjectManageDialog from './ProjectManageDialog.vue'
import ProjectTree from './ProjectTree.vue'

type DialogKind = 'register' | 'create' | 'move' | 'rename' | 'unregister' | 'delete'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const projects = useProjectsStore()
const settings = useSettingsStore()

const search = ref('')
const ready = ref(false)
const dialog = ref<{ kind: DialogKind; project: Project | null } | null>(null)

const projectId = computed<number | null>(() =>
  route.params.projectId === undefined ? null : Number(route.params.projectId),
)
const sessionId = computed<string | null>(() =>
  route.params.sessionId === undefined ? null : String(route.params.sessionId),
)
const missingProject = computed(
  () => ready.value && projectId.value !== null && !projects.byId(projectId.value),
)
// A route whose panel fills the pane (the conversation) declares it, so the pane
// stops scrolling and the panel's own message log is the only scroller.
const fullHeightPane = computed(() => route.meta.fullHeight === true)
// The settings center is a platform-wide area: the project tree would only be
// noise there, so the shell hides it and lets the settings view use the pane.
const settingsMode = computed(() => route.meta.settings === true)

onMounted(async () => {
  await projects.load()
  ready.value = true
  // The appearance is shared platform config: fetch it after auth and apply
  // it over the cached value painted before mount.
  void settings.loadAppearance()
})

function openProject(id: number) {
  void router.push({ name: 'project', params: { projectId: String(id) } })
}

function openTask(id: number, taskId: string) {
  void router.push({ name: 'task', params: { projectId: String(id), sessionId: taskId } })
}

function onCreated(taskId: string) {
  if (projectId.value !== null) openTask(projectId.value, taskId)
}

function openNewTask(id: number) {
  void router.push({ name: 'task-new', params: { projectId: String(id) } })
}

function openManage(kind: DialogKind, project: Project | null = null) {
  dialog.value = { kind, project }
}

// A removed project's open pages must not outlive it, so the shell returns home
// after unregister or delete. Rename and move keep the id and stay where they are.
async function onSubmitted(kind: DialogKind) {
  const removed = dialog.value?.project ?? null
  await projects.load()
  if ((kind === 'unregister' || kind === 'delete') && removed && projectId.value === removed.id) {
    void router.push({ name: 'home' })
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
  <div class="app-shell" :class="{ 'settings-mode': settingsMode }">
    <CategoryRail @logout="logout" />

    <aside v-if="!settingsMode" class="sidebar" aria-label="项目导航">
      <div class="sidebar-head">
        <h2>项目</h2>
        <div class="head-actions">
          <button type="button" class="register" @click="openManage('register')">注册</button>
          <button type="button" class="create" @click="openManage('create')">新建</button>
        </div>
      </div>
      <input
        v-model="search"
        class="search"
        type="search"
        placeholder="搜索项目或任务"
        aria-label="搜索项目或任务"
      />
      <div class="tree">
        <ProjectTree
          :active-project-id="projectId"
          :active-session-id="sessionId"
          :search="search"
          @select-project="openProject"
          @select-session="openTask"
          @new-task="openNewTask"
        />
      </div>
    </aside>

    <section class="pane" :class="{ 'pane-full': fullHeightPane }">
      <p v-if="projects.error" class="page-error" role="alert">{{ projects.error }}</p>
      <p v-else-if="projectId !== null && !ready" class="loading">加载中…</p>
      <div v-else-if="missingProject" class="missing">
        <p>项目不存在或已移除。</p>
        <RouterLink :to="{ name: 'home' }">返回</RouterLink>
      </div>
      <RouterView v-else v-slot="{ Component }">
        <component
          :is="Component"
          v-if="Component"
          @manage="openManage"
          @created="onCreated"
        />
      </RouterView>
    </section>

    <ProjectManageDialog
      v-if="dialog"
      :kind="dialog.kind"
      :project="dialog.project"
      @close="dialog = null"
      @submitted="onSubmitted"
    />
  </div>
</template>

<style scoped>
.app-shell {
  display: grid;
  grid-template-columns: 56px 272px 1fr;
  height: 100%;
  background: var(--color-bg);
  color: var(--color-text);
  font-family: var(--font-family);
  font-size: 1rem;
}

/* The settings center has no project tree, so the middle column collapses. */
.app-shell.settings-mode {
  grid-template-columns: 56px 1fr;
}

.sidebar {
  background: var(--color-surface);
  border-right: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.sidebar-head {
  padding: 14px 14px 8px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.sidebar-head h2 {
  font-size: 15px;
  margin: 0;
}

.head-actions {
  display: flex;
  gap: 4px;
}

.head-actions button {
  border: 1px solid #d0d5dd;
  background: #fff;
  border-radius: 6px;
  padding: 3px 8px;
  cursor: pointer;
  font-size: 12px;
}

.search {
  margin: 0 12px 8px;
  padding: 6px 10px;
  border: 1px solid #d0d5dd;
  border-radius: 6px;
  font: inherit;
  font-size: 13px;
}

.tree {
  overflow: auto;
  padding: 4px 6px 12px;
  flex: 1;
}

.pane {
  min-width: 0;
  overflow: auto;
  padding: 18px 20px;
}

/* The conversation pane fills the viewport and never scrolls itself: its message
   area scrolls instead. A flex column lets the panel resolve height: 100%. */
.pane-full {
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.page-error {
  color: #b91c1c;
}

.loading {
  color: #64748b;
}

.missing {
  color: #64748b;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  align-items: flex-start;
}
</style>
