<script setup lang="ts">
// Three-pane shell: category rail, middle column, and the right pane chosen by
// the child route. The middle column renders the menu's own list — the project/
// session tree, the task list or the settings categories — and can be collapsed
// from the rail (the choice is persisted). The pane always mounts the tab strip
// pinned to the very top at full width: sessions, project settings, the
// new-session form, task details and the platform settings center all appear as
// tabs, so several pages stay open at once while only the body below scrolls. A
// project id that is not in the loaded list shows a not-found notice instead of
// any panel, so a stale or foreign URL never renders another project's content.
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import type { Project } from '../../api/projects'
import { useAgentsStore } from '../../stores/agents'
import { useAuthStore } from '../../stores/auth'
import { useProjectsStore } from '../../stores/projects'
import { useSessionsStore } from '../../stores/sessions'
import { useOpenTabsStore, tabKey, type OpenTab } from '../../stores/openTabs'
import { useSettingsStore } from '../../stores/settings'
import { useTasksStore } from '../../stores/tasks'
import CategoryRail from './CategoryRail.vue'
import ProjectManageDialog from '../projects/ProjectManageDialog.vue'
import ProjectTree from '../projects/ProjectTree.vue'
import SessionTabs from './SessionTabs.vue'
import TasksSidebar from '../tasks/TasksSidebar.vue'
import PromptsSidebar from '../prompts/PromptsSidebar.vue'
import SettingsNav from '../settings/SettingsNav.vue'

type DialogKind = 'register' | 'create' | 'move' | 'rename' | 'rename-name' | 'unregister'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const projects = useProjectsStore()
const agents = useAgentsStore()
const sessions = useSessionsStore()
const openTabs = useOpenTabsStore()
const settings = useSettingsStore()
const tasksStore = useTasksStore()

const search = ref('')
const ready = ref(false)
const dialog = ref<{ kind: DialogKind; project: Project | null } | null>(null)

// The middle column (project tree, task list or settings menu) can be collapsed
// from the rail so the conversation takes the full width. The choice is
// persisted so a refresh keeps it; the default is expanded, and every area
// shares one choice because the column is the same slot in all of them.
const SIDEBAR_STORAGE_KEY = 'dev-platform-sidebar-v1'
const sidebarCollapsed = ref(false)
try {
  sidebarCollapsed.value = localStorage.getItem(SIDEBAR_STORAGE_KEY) === 'collapsed'
} catch {
  sidebarCollapsed.value = false
}
function setSidebarCollapsed(collapsed: boolean) {
  sidebarCollapsed.value = collapsed
  try {
    localStorage.setItem(SIDEBAR_STORAGE_KEY, collapsed ? 'collapsed' : 'expanded')
  } catch {
    // Persistence is best-effort only.
  }
}
function toggleSidebar() {
  setSidebarCollapsed(!sidebarCollapsed.value)
}

// In settings mode the navigation categories are essential and should not be collapsed
// by the workspace's project sidebar toggle.
const isSidebarVisible = computed(() => {
  if (settingsMode.value) return true
  return !sidebarCollapsed.value
})

// Whether task-generated sessions are shown in the project tree.
// Defaults to false (Option B: hidden by default). Persisted in localStorage.
const SHOW_TASK_SESSIONS_STORAGE_KEY = 'dev-platform-show-task-sessions'
const showTaskSessions = ref(false)
try {
  const saved = localStorage.getItem(SHOW_TASK_SESSIONS_STORAGE_KEY)
  if (saved !== null) {
    showTaskSessions.value = saved === 'true'
  }
} catch {
  showTaskSessions.value = false
}

function onShowTaskSessionsChange() {
  try {
    localStorage.setItem(SHOW_TASK_SESSIONS_STORAGE_KEY, String(showTaskSessions.value))
  } catch {
    // Persistence is best-effort only.
  }
}

const projectId = computed<number | null>(() =>
  route.params.projectId === undefined ? null : Number(route.params.projectId),
)
const sessionId = computed<string | null>(() =>
  route.params.sessionId === undefined ? null : String(route.params.sessionId),
)
// Which tab the URL is showing. Sessions, project settings, the new-session
// form, task details and the platform settings each map onto their tab key the
// strip renders.
const settingsRoute = computed(() => route.name === 'project-settings')
const platformSettingsRoute = computed(
  () => typeof route.name === 'string' && route.name.startsWith('settings-'),
)
const newSessionRoute = computed(
  () => route.name === 'session-new' || route.name === 'session-new-global',
)
const taskRoute = computed(() => route.name === 'task-detail')
// The settings center renders its category links in the middle column and its
// panels in the pane.
const settingsMode = computed(() => route.meta.settings === true)
// The tasks center manages tasks with their own list column and detail pane.
const tasksMode = computed(() => route.meta.tasks === true)
// The prompts center manages prompt templates with their own list column and detail pane.
const promptsMode = computed(() => route.meta.prompts === true)
// The project/workspace area is the default when neither settings, tasks nor prompts is open.
const projectActive = computed(() => !settingsMode.value && !tasksMode.value && !promptsMode.value)

// Tabs belong strictly to the project workspace: sessions, project settings, and
// new session creation. Task detail and platform settings are managed by their own
// respective centers and do not appear as workspace tabs.
const workspaceTabs = computed(() =>
  openTabs.tabs.filter(
    (entry) =>
      entry.kind === 'session' ||
      entry.kind === 'new-session' ||
      entry.kind === 'project-settings',
  ),
)

const activeKey = computed<string | null>(() => {
  if (!projectActive.value) return null
  if (newSessionRoute.value) return 'new-session'
  if (settingsRoute.value && projectId.value !== null) return `settings:${projectId.value}`
  return sessionId.value
})
const missingProject = computed(
  () => ready.value && projectId.value !== null && !projects.byId(projectId.value),
)
// A route whose panel fills the pane (the conversation) declares it, so the pane
// stops scrolling and the panel's own message log is the only scroller.
const fullHeightPane = computed(() => route.meta.fullHeight === true)
// The middle column carries one list at a time; its accessible name should say
// which, since screen-reader users navigate columns by name.
const columnLabel = computed(() =>
  settingsMode.value
    ? '功能导航'
    : tasksMode.value
      ? '任务列表'
      : promptsMode.value
        ? '提示词列表'
        : '项目导航',
)

onMounted(async () => {
  // The backend catalog facts (labels, order, service support) are read by
  // every session tag and session row, so they load with the shell, once.
  void agents.loadBackends()
  await projects.load()
  ready.value = true
  // The appearance is shared platform config: fetch it after auth and apply
  // it over the cached value painted before mount.
  void settings.loadAppearance()
  syncTabWithRoute()
})

// The tab bar mirrors the current route in the project workspace: opening a session,
// a project's settings, or the new-session form registers (or refreshes) its tab.
// Tab titles follow the session list once it loads, and the new-session tab is replaced
// by the created session's tab on success. Tabs are never removed implicitly except
// when their project is gone.
function syncTabWithRoute() {
  // Only sync tabs when within the project workspace area.
  // Settings and tasks have their own dedicated views and do not participate in workspace tabs.
  if (!projectActive.value) return

  if (newSessionRoute.value) {
    openTabs.openNewSession()
    return
  }
  if (projectId.value === null) return
  if (settingsRoute.value) {
    openTabs.openProjectSettings(projectId.value)
    return
  }
  if (sessionId.value === null) return
  const known = sessions.sessionById(sessionId.value)
  openTabs.openTab(projectId.value, sessionId.value, known?.title ?? '')
}

watch([() => route.fullPath], syncTabWithRoute)

watch(
  activeKey,
  (key) => {
    if (key) {
      openTabs.setActiveKey(key)
    }
  },
  { immediate: true },
)

// Session titles arrive after the list loads; keep the matching tab label
// fresh without touching other tabs.
watch(
  () => sessions.sessions.map((entry) => `${entry.id}:${entry.title ?? ''}`).join(','),
  () => {
    for (const entry of sessions.sessions) {
      if (entry.title) openTabs.updateTitle(entry.id, entry.title)
    }
    syncTabWithRoute()
  },
)

const projectNames = computed<Record<number, string>>(() =>
  Object.fromEntries(projects.projects.map((project) => [project.id, project.name])),
)

// Ids of the open sessions whose turn is running. The tab strip reads it to mark
// a running session with an animated icon before its label, matching the tree.
const runningSessionIds = computed<string[]>(() =>
  sessions.sessions.filter((entry) => entry.turn === 'running').map((entry) => entry.id),
)

function selectTab(tab: OpenTab) {
  openTabs.setActiveKey(tabKey(tab))
  if (tab.kind === 'project-settings') {
    void router.push({ name: 'project-settings', params: { projectId: String(tab.projectId) } })
    return
  }
  if (tab.kind === 'new-session') {
    void router.push({ name: 'session-new-global' })
    return
  }
  if (tab.kind === 'session') {
    void router.push({ name: 'session', params: { projectId: String(tab.projectId), sessionId: tab.sessionId } })
  }
}

// focusAfterClose picks a new tab when the one the URL pointed at is gone: the
// last remaining tab in the workspace, or the workbench home when nothing is left.
// Closing a tab that was not active leaves the view where it was.
function focusAfterClose(activeRemoved: boolean, _closed?: OpenTab) {
  if (!activeRemoved) return
  const remaining = workspaceTabs.value
  if (remaining.length > 0) {
    selectTab(remaining[remaining.length - 1])
  } else {
    void router.push({ name: 'home' })
  }
}

function releaseSessions(tabs: OpenTab[]) {
  // Releasing a closed session's streams keeps background sessions from
  // lingering; the other kinds have no stream.
  for (const tab of tabs) {
    if (tab.kind === 'session') sessions.unsubscribe(tab.sessionId)
  }
}

// Automatically unsubscribe from SSE streams when a session tab leaves the openTabs set
// (whether via manual close, close others, close all, or store capacity trimming).
watch(
  () =>
    openTabs.tabs
      .filter((t): t is Extract<OpenTab, { kind: 'session' }> => t.kind === 'session')
      .map((t) => t.sessionId),
  (currentIds, oldIds) => {
    if (!oldIds) return
    const currentSet = new Set(currentIds)
    for (const oldId of oldIds) {
      if (!currentSet.has(oldId)) {
        sessions.unsubscribe(oldId)
      }
    }
  },
)

function closeTab(tab: OpenTab) {
  // Pinned tabs are the user's anchors: the strip hides their close control and
  // ignores a middle click, and this guard covers any other path.
  if (tab.pinned === true) return
  const wasActive = tabKey(tab) === activeKey.value
  releaseSessions([tab])
  openTabs.closeTabByKey(tabKey(tab))
  focusAfterClose(wasActive, tab)
}

// Closing the other tabs keeps the clicked one, and any pinned tabs, open.
function closeOtherTabs(tab: OpenTab) {
  const closed = openTabs.closeOtherTabsByKey(tabKey(tab))
  releaseSessions(closed)
  focusAfterClose(!workspaceTabs.value.some((entry) => tabKey(entry) === activeKey.value))
}

// Closing all tabs empties the strip except for its pinned tabs.
function closeAllTabs() {
  const closed = openTabs.closeAllTabs()
  releaseSessions(closed)
  focusAfterClose(!workspaceTabs.value.some((entry) => tabKey(entry) === activeKey.value))
}

function togglePinTab(tab: OpenTab) {
  openTabs.togglePin(tabKey(tab))
}

// A project's gear opens its settings as a tab. The project name in the tree
// only expands its sessions — there is no project page to navigate to.
function openSettings(id: number) {
  void router.push({ name: 'project-settings', params: { projectId: String(id) } })
}

function openSessionTab(id: number, sessionId: string) {
  void router.push({ name: 'session', params: { projectId: String(id), sessionId } })
}

function onDeleteSession(_id: number, sessionId: string) {
  const tab = openTabs.tabs.find((t) => t.kind === 'session' && t.sessionId === sessionId)
  if (tab) {
    closeTab(tab)
  }
}

// A created session replaces the "new" tab in place: the strip's position is
// kept, the focus follows the created session, and its tab title follows the
// session list as soon as the server-derived one arrives.
function onCreated(sessionId: string, targetProjectId?: number) {
  const pId = targetProjectId ?? projectId.value
  if (pId === null || pId === undefined) return
  openTabs.promoteNewSession(pId, sessionId)
  void router.push({ name: 'session', params: { projectId: String(pId), sessionId } })
}

function openNewSession(id: number) {
  void router.push({ name: 'session-new', params: { projectId: String(id) } })
}

function onNewTab() {
  if (projectId.value !== null) {
    openNewSession(projectId.value)
  } else if (projects.projects.length > 0) {
    openNewSession(projects.projects[0].id)
  } else {
    void router.push({ name: 'session-new-global' })
  }
}

function openManage(kind: DialogKind, project: Project | null = null) {
  dialog.value = { kind, project }
}

// A removed project's open pages must not outlive it, so the shell returns home
// after unregister and drops that project's tabs. Rename and move keep the id
// and stay where they are.
async function onSubmitted(kind: DialogKind) {
  const removed = dialog.value?.project ?? null
  await projects.load()
  if (kind === 'unregister' && removed) {
    for (const tab of openTabs.tabs.filter((entry) => entry.projectId === removed.id)) {
      if (tab.kind === 'session') sessions.unsubscribe(tab.sessionId)
      openTabs.closeTabByKey(tabKey(tab))
    }
    if (projectId.value === removed.id) {
      void router.push({ name: 'home' })
    }
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
  <div
    class="app-shell"
    :class="{
      'settings-mode': settingsMode,
      'tasks-mode': tasksMode,
      'prompts-mode': promptsMode,
      'sidebar-collapsed': !isSidebarVisible,
    }"
  >
    <CategoryRail
      :sidebar-collapsed="!isSidebarVisible"
      :sidebar-collapsible="!settingsMode"
      @logout="logout"
      @toggle-sidebar="toggleSidebar"
      @expand-sidebar="setSidebarCollapsed(false)"
    />

    <!-- The column's accessible name follows the list it currently carries. -->
    <aside
      v-if="isSidebarVisible"
      class="sidebar"
      :aria-label="columnLabel"
    >
      <!-- The middle column carries whichever list the open menu owns: the
           tasks' task list, the prompts' prompt list, the settings' categories, or the project tree. -->
      <TasksSidebar v-if="tasksMode" />
      <PromptsSidebar v-else-if="promptsMode" />
      <SettingsNav v-else-if="settingsMode" />
      <template v-else>
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
          placeholder="搜索项目或会话"
          aria-label="搜索项目或会话"
        />
        <div class="sidebar-filter-bar">
          <label class="filter-task-checkbox">
            <input
              v-model="showTaskSessions"
              type="checkbox"
              data-test="show-task-sessions"
              @change="onShowTaskSessionsChange"
            />
            <span>显示任务会话</span>
          </label>
        </div>
        <div class="tree">
          <ProjectTree
            :active-project-id="projectId"
            :active-session-id="sessionId"
            :search="search"
            :show-task-sessions="showTaskSessions"
            @open-settings="openSettings"
            @select-session="openSessionTab"
            @new-session="openNewSession"
            @delete-session="onDeleteSession"
          />
        </div>
      </template>
    </aside>

    <section class="pane" :class="{ 'pane-full': fullHeightPane }">
      <SessionTabs
        v-if="projectActive && workspaceTabs.length > 0"
        :tabs="workspaceTabs"
        :active-key="activeKey"
        :project-names="projectNames"
        :running-session-ids="runningSessionIds"
        @select="selectTab"
        @close="closeTab"
        @toggle-pin="togglePinTab"
        @close-others="closeOtherTabs"
        @close-all="closeAllTabs"
        @new-tab="onNewTab"
      />
      <div class="pane-body">
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
      </div>
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
  /* The layout is always the same three columns: the category rail, the menu's
     middle column and the tabbed pane. minmax(0,1fr) caps the single row so
     each column scrolls inside itself, and 1fr on the last column keeps it from
     growing past the viewport (no horizontal scrollbar). */
  grid-template-columns: 56px 272px minmax(0, 1fr);
  /* The single row must be pinned to the viewport: an implicit auto row sizes
     to its content, so a long session tree stretched the whole shell (and with it
     the conversation's 100%-height panel) past the fold, pushing the composer
     out of view with body overflow hidden. minmax(0,1fr) caps the row so each
     column scrolls inside itself. */
  grid-template-rows: minmax(0, 1fr);
  height: 100%;
  min-width: 0;
  background: var(--color-bg);
  color: var(--color-text);
  font-family: var(--font-family);
  font-size: 1rem;
}

/* Collapsing the middle column from the rail leaves only the rail and the
   content pane, so the conversation can use the full width. */
.app-shell.sidebar-collapsed {
  grid-template-columns: 56px minmax(0, 1fr);
}

.sidebar {
  background: var(--color-surface);
  border-right: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.sidebar-head {
  padding: calc(var(--space-unit) * 2.333) calc(var(--space-unit) * 2.333) calc(var(--space-unit) * 1.333);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.sidebar-head h2 {
  font-size: 1.071rem;
  margin: 0;
}

.head-actions {
  display: flex;
  gap: calc(var(--space-unit) * 0.667);
}

.head-actions button {
  border: 1px solid var(--color-border-strong);
  background: var(--color-surface);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 0.500) calc(var(--space-unit) * 1.333);
  cursor: pointer;
  font-size: 0.857rem;
}

.search {
  margin: 0 calc(var(--space-unit) * 2.000) calc(var(--space-unit) * 1.333);
  padding: calc(var(--space-unit) * 1.000) calc(var(--space-unit) * 1.667);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.929rem;
}

.sidebar-filter-bar {
  display: flex;
  align-items: center;
  padding: 0 calc(var(--space-unit) * 2.000) calc(var(--space-unit) * 1.000);
  margin-top: calc(var(--space-unit) * -0.5);
}

.filter-task-checkbox {
  display: inline-flex;
  align-items: center;
  gap: calc(var(--space-unit) * 0.667);
  font-size: 0.8rem;
  color: var(--color-text-muted);
  cursor: pointer;
  user-select: none;
}

.filter-task-checkbox input[type="checkbox"] {
  cursor: pointer;
  accent-color: var(--color-primary);
}

.filter-task-checkbox:hover {
  color: var(--color-text);
}

.tree {
  box-sizing: border-box;
  overflow-y: auto;
  overflow-x: hidden;
  padding: calc(var(--space-unit) * 0.667) calc(var(--space-unit) * 1.000) calc(var(--space-unit) * 2.000);
  flex: 1;
}

.pane {
  min-width: 0;
  min-height: 0;
  /* The pane never scrolls and carries no padding: the tab bar sits flush at the
     top and stretches the full width, while the body below is the scroller. */
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* Ordinary pages scroll inside the padded body; the tab bar stays pinned above. */
.pane-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: calc(var(--space-unit) * 3.000) calc(var(--space-unit) * 3.333);
}

/* The conversation pane fills the viewport and never scrolls itself: its message
   area scrolls instead. The body keeps the same inset, and the panel's
   height: 100% resolves against this definite box. */
.pane-full .pane-body {
  overflow: hidden;
}

/* The tasks center owns a full-bleed two-column layout of its own, so its pane
   body drops the inset: the task list column then sits flush against the rail
   exactly like the project sidebar does, instead of floating inside a padded box. */
.tasks-mode .pane-body {
  padding: 0;
}

.page-error {
  color: var(--color-danger);
}

.loading {
  color: var(--color-text-muted);
}

.missing {
  color: var(--color-text-muted);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  align-items: flex-start;
}
</style>
