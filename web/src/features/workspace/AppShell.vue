<script setup lang="ts">
// Three-pane shell: category rail, project/session tree, and the right pane chosen
// by the child route. The shell owns the single project-manage dialog and turns
// tree and panel events into navigation, and pins the open-session tab bar to the
// very top of the right pane at full width so several sessions stay open at once
// while only the body below it scrolls. A project id that is not
// in the loaded list shows a not-found notice instead of any panel, so a stale
// or foreign URL never renders another project's content. The settings center
// hides the project sidebar so it occupies the full pane, and the rail's bottom
// toggle collapses that column on demand (the choice is persisted) so a
// conversation can take the full width.
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import type { Project } from '../../api/projects'
import { useAgentsStore } from '../../stores/agents'
import { useAuthStore } from '../../stores/auth'
import { useProjectsStore } from '../../stores/projects'
import { useSessionsStore } from '../../stores/sessions'
import { useOpenTabsStore, tabKey, type OpenTab } from '../../stores/openTabs'
import { useSettingsStore } from '../../stores/settings'
import CategoryRail from './CategoryRail.vue'
import ProjectManageDialog from '../projects/ProjectManageDialog.vue'
import ProjectTree from '../projects/ProjectTree.vue'
import SessionTabs from './SessionTabs.vue'

type DialogKind = 'register' | 'create' | 'move' | 'rename' | 'unregister'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const projects = useProjectsStore()
const agents = useAgentsStore()
const sessions = useSessionsStore()
const openTabs = useOpenTabsStore()
const settings = useSettingsStore()

const search = ref('')
const ready = ref(false)
const dialog = ref<{ kind: DialogKind; project: Project | null } | null>(null)

// The middle project/task column can be collapsed from the rail so the
// conversation takes the full width. The choice is persisted so a refresh keeps
// it; the default is expanded, and the settings center hides the column anyway.
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

const projectId = computed<number | null>(() =>
  route.params.projectId === undefined ? null : Number(route.params.projectId),
)
const sessionId = computed<string | null>(() =>
  route.params.sessionId === undefined ? null : String(route.params.sessionId),
)
// Which tab the URL is showing. A project-settings route has no session, and a
// session route is identified by its session id — the same keys the strip renders.
const settingsRoute = computed(() => route.name === 'project-settings')
const activeKey = computed<string | null>(() => {
  if (settingsRoute.value && projectId.value !== null) return `settings:${projectId.value}`
  return sessionId.value
})
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

// The tab bar mirrors the current route: opening a session or a project's settings
// URL registers (or refreshes) its tab, and a tab title follows the session list
// once it loads. Tabs are never removed implicitly except when their project is
// gone.
function syncTabWithRoute() {
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
  if (tab.kind === 'project-settings') {
    void router.push({ name: 'project-settings', params: { projectId: String(tab.projectId) } })
    return
  }
  void router.push({ name: 'session', params: { projectId: String(tab.projectId), sessionId: tab.sessionId } })
}

// focusAfterClose picks a new tab when the one the URL pointed at is gone: the
// last remaining tab, or the workbench home when nothing is left. Closing a tab
// that was not active leaves the view where it was.
function focusAfterClose(activeRemoved: boolean) {
  if (!activeRemoved) return
  const remaining = openTabs.tabs
  if (remaining.length > 0) {
    selectTab(remaining[remaining.length - 1])
  } else {
    void router.push({ name: 'home' })
  }
}

function releaseSessions(tabs: OpenTab[]) {
  // Releasing a closed session's streams keeps background sessions from
  // lingering; a settings page has no stream.
  for (const tab of tabs) {
    if (tab.kind === 'session') sessions.unsubscribe(tab.sessionId)
  }
}

function closeTab(tab: OpenTab) {
  // Pinned tabs are the user's anchors: the strip hides their close control and
  // ignores a middle click, and this guard covers any other path.
  if (tab.pinned === true) return
  const wasActive = tabKey(tab) === activeKey.value
  releaseSessions([tab])
  openTabs.closeTabByKey(tabKey(tab))
  focusAfterClose(wasActive)
}

// Closing the other tabs keeps the clicked one, and any pinned tabs, open.
function closeOtherTabs(tab: OpenTab) {
  const closed = openTabs.closeOtherTabsByKey(tabKey(tab))
  releaseSessions(closed)
  focusAfterClose(!openTabs.tabs.some((entry) => tabKey(entry) === activeKey.value))
}

// Closing all tabs empties the strip except for its pinned tabs.
function closeAllTabs() {
  const closed = openTabs.closeAllTabs()
  releaseSessions(closed)
  focusAfterClose(!openTabs.tabs.some((entry) => tabKey(entry) === activeKey.value))
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

function onCreated(sessionId: string) {
  if (projectId.value !== null) openSessionTab(projectId.value, sessionId)
}

function openNewSession(id: number) {
  void router.push({ name: 'session-new', params: { projectId: String(id) } })
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
  <div class="app-shell" :class="{ 'settings-mode': settingsMode, 'sidebar-collapsed': sidebarCollapsed }">
    <CategoryRail
      :sidebar-collapsed="sidebarCollapsed"
      :sidebar-collapsible="!settingsMode"
      @logout="logout"
      @toggle-sidebar="toggleSidebar"
      @expand-sidebar="setSidebarCollapsed(false)"
    />

    <aside v-if="!settingsMode && !sidebarCollapsed" class="sidebar" aria-label="项目导航">
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
      <div class="tree">
        <ProjectTree
          :active-project-id="projectId"
          :active-session-id="sessionId"
          :search="search"
          @open-settings="openSettings"
          @select-session="openSessionTab"
          @new-session="openNewSession"
        />
      </div>
    </aside>

    <section class="pane" :class="{ 'pane-full': fullHeightPane }">
      <SessionTabs
        v-if="!settingsMode && openTabs.tabs.length > 0"
        :tabs="openTabs.tabs"
        :active-key="activeKey"
        :project-names="projectNames"
        :running-session-ids="runningSessionIds"
        @select="selectTab"
        @close="closeTab"
        @toggle-pin="togglePinTab"
        @close-others="closeOtherTabs"
        @close-all="closeAllTabs"
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
  grid-template-columns: 56px 272px 1fr;
  /* The single row must be pinned to the viewport: an implicit auto row sizes
     to its content, so a long session tree stretched the whole shell (and with it
     the conversation's 100%-height panel) past the fold, pushing the composer
     out of view with body overflow hidden. minmax(0,1fr) caps the row so each
     column scrolls inside itself. */
  grid-template-rows: minmax(0, 1fr);
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

/* Collapsing the project/task column from the rail leaves only the rail and the
   content pane, so the conversation can use the full width. */
.app-shell.sidebar-collapsed {
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
  border-radius: 6px;
  padding: calc(var(--space-unit) * 0.500) calc(var(--space-unit) * 1.333);
  cursor: pointer;
  font-size: 0.857rem;
}

.search {
  margin: 0 calc(var(--space-unit) * 2.000) calc(var(--space-unit) * 1.333);
  padding: calc(var(--space-unit) * 1.000) calc(var(--space-unit) * 1.667);
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  font: inherit;
  font-size: 0.929rem;
}

.tree {
  overflow: auto;
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
