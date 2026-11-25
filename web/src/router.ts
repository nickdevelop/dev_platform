// Application router: the login view outside the shell, and the three-pane
// shell for every authenticated view. The shell's child routes select the right
// pane (project settings, new session, or session conversation) and pass the ids from
// the URL, so a refreshed or shared session URL restores the same selection.
// The router owns the two navigation security behaviors:
//   - a navigation guard that redirects unauthenticated users to /login, and
//   - the global handler that returns the user to /login whenever an API
//     call reports an expired session (401).

import { createRouter, createWebHistory } from 'vue-router'
import { setOnUnauthorized } from './api/client'
import { useAuthStore } from './stores/auth'
import AppShell from './features/workspace/AppShell.vue'
import EmptyWorkspace from './features/workspace/EmptyWorkspace.vue'
import LoginView from './views/LoginView.vue'
import ProjectsManagementView from './features/projects/ProjectsManagementView.vue'
import ProjectSettingsView from './features/projects/ProjectSettingsView.vue'
import NewSessionPanel from './features/chat/composer/NewSessionPanel.vue'
import SessionChatPanel from './features/chat/SessionChatPanel.vue'
import SettingsView from './features/settings/SettingsView.vue'
import EnginesView from './features/engines/EnginesView.vue'
import AssistantEngineView from './features/engines/AssistantEngineView.vue'
import CoderEngineView from './features/engines/CoderEngineView.vue'
import OrchestratorEngineView from './features/engines/OrchestratorEngineView.vue'
import CoreEngineView from './features/engines/CoreEngineView.vue'
import AgentSettingsView from './features/engines/AgentSettingsView.vue'
import AppearanceSettings from './features/settings/AppearanceSettings.vue'
import SettingsPromptsView from './features/settings/SettingsPromptsView.vue'
import OrgPromptsView from './features/organization/prompts/OrgPromptsView.vue'
import ModelSettings from './features/settings/ModelSettings.vue'
import OrganizationView from './features/organization/OrganizationView.vue'
import AssistantListView from './features/organization/assistants/AssistantListView.vue'
import EmployeeListView from './features/organization/employees/EmployeeListView.vue'
import TeamListView from './features/organization/teams/TeamListView.vue'
import TasksView from './features/tasks/TasksView.vue'
import NotesView from './features/notes/NotesView.vue'
import ProxySettings from './features/settings/ProxySettings.vue'
import ConnectionSettings from './features/settings/ConnectionSettings.vue'
import TerminalSettingsView from './features/settings/TerminalSettingsView.vue'
import DebugSettings from './features/settings/DebugSettings.vue'

export function createAppRouter() {
  const router = createRouter({
    history: createWebHistory(),
    routes: [
      {
        path: '/',
        component: AppShell,
        meta: { requiresAuth: true },
        children: [
          { path: '', name: 'home', component: EmptyWorkspace },
          {
            // A workspace is not a page of its own any more: its settings open as
            // a workbench tab, and its name in the tree only expands its sessions.
            path: 'projects/:projectId',
            redirect: { name: 'home' },
          },
          {
            path: 'projects/management',
            name: 'projects-management',
            component: ProjectsManagementView,
          },
          {
            path: 'projects/:projectId/settings',
            name: 'project-settings',
            component: ProjectSettingsView,
            props: (route) => ({ projectId: Number(route.params.projectId) }),
          },
          {
            path: 'chat',
            redirect: { name: 'home' },
          },
          {
            path: 'chat/:chatId',
            redirect: { name: 'home' },
          },
          {
            path: 'org',
            component: OrganizationView,
            meta: { org: true, fullHeight: true },
            children: [
              { path: '', redirect: { name: 'org-prompts' } },
              { path: 'prompts', name: 'org-prompts', component: OrgPromptsView },
              { path: 'assistants', name: 'org-assistants', component: AssistantListView },
              {
                path: 'assistants/new',
                redirect: { name: 'org-assistants', query: { action: 'new' } },
              },
              {
                path: 'assistants/:assistantId',
                redirect: (route) => ({ name: 'org-assistants', query: { edit: route.params.assistantId } }),
              },
              { path: 'employees', name: 'org-employees', component: EmployeeListView },
              {
                path: 'employees/new',
                redirect: { name: 'org-employees', query: { action: 'new' } },
              },
              {
                path: 'employees/:employeeId',
                redirect: (route) => ({ name: 'org-employees', query: { edit: route.params.employeeId } }),
              },
              { path: 'teams', name: 'org-teams', component: TeamListView },
              {
                path: 'teams/new',
                redirect: { name: 'org-teams', query: { action: 'new' } },
              },
              {
                path: 'teams/:teamId',
                redirect: (route) => ({ name: 'org-teams', query: { edit: route.params.teamId } }),
              },
            ],
          },
          {
            path: 'engines',
            component: EnginesView,
            meta: { engines: true },
            children: [
              { path: '', redirect: { name: 'engines-assistant' } },
              { path: 'assistant', name: 'engines-assistant', component: AssistantEngineView },
              { path: 'steward', redirect: { name: 'engines-assistant' } },
              { path: 'coder', name: 'engines-coder', component: CoderEngineView },
              { path: 'orchestrator', name: 'engines-orchestrator', component: OrchestratorEngineView },
              { path: 'agents', name: 'engines-agents', component: AgentSettingsView },
              { path: 'core', redirect: { name: 'engines-assistant' } },
            ],
          },
          {
            path: 'settings',
            component: SettingsView,
            meta: { settings: true },
            children: [
              { path: '', redirect: { name: 'settings-appearance' } },
              { path: 'appearance', name: 'settings-appearance', component: AppearanceSettings },
              { path: 'roles', redirect: { name: 'org-employees' } },
              { path: 'prompts', name: 'settings-prompts', redirect: { name: 'org-prompts' } },
              { path: 'models', name: 'settings-models', component: ModelSettings },
              { path: 'agents', redirect: { name: 'engines-agents' } },
              { path: 'proxies', name: 'settings-proxies', component: ProxySettings },
              { path: 'connections', name: 'settings-connections', component: ConnectionSettings },
              { path: 'terminal', name: 'settings-terminal', component: TerminalSettingsView },
              { path: 'debug', name: 'settings-debug', component: DebugSettings },
            ],
          },
          {
            path: 'groups',
            redirect: { name: 'org-teams' },
          },
          {
            path: 'groups/new',
            redirect: { name: 'org-teams', query: { action: 'new' } },
          },
          {
            path: 'groups/:groupId',
            redirect: (route) => ({ name: 'org-teams', query: { edit: route.params.groupId } }),
          },
          {
            path: 'tasks',
            name: 'tasks',
            component: TasksView,
            meta: { tasks: true, fullHeight: true },
          },
          {
            path: 'tasks/new',
            name: 'task-new',
            component: TasksView,
            meta: { tasks: true, fullHeight: true },
          },
          {
            path: 'tasks/:taskId',
            name: 'task-detail',
            component: TasksView,
            meta: { tasks: true, fullHeight: true },
          },
          {
            path: 'prompts',
            redirect: { name: 'org-prompts' },
          },
          {
            path: 'prompts/:promptId',
            redirect: { name: 'org-prompts' },
          },
          {
            path: 'terminal',
            redirect: { name: 'settings-terminal' },
          },
          {
            path: 'notes',
            name: 'notes',
            component: NotesView,
            meta: { notes: true, fullHeight: true },
          },
          {
            path: 'notes/:noteId',
            name: 'note-detail',
            component: NotesView,
            meta: { notes: true, fullHeight: true },
            props: (route) => ({ noteId: Number(route.params.noteId) }),
          },
          {
            path: 'projects/:projectId/sessions/new',
            name: 'session-new',
            component: NewSessionPanel,
            props: (route) => ({ projectId: Number(route.params.projectId) }),
          },
          {
            path: 'sessions/new',
            name: 'session-new-global',
            component: NewSessionPanel,
          },
          {
            path: 'projects/:projectId/sessions/:sessionId',
            name: 'session',
            component: SessionChatPanel,
            // The conversation fills the pane and owns the only scroller (its
            // message log); the shell must not scroll it as well.
            meta: { fullHeight: true },
            props: (route) => ({
              projectId: Number(route.params.projectId),
              sessionId: String(route.params.sessionId),
            }),
          },
        ],
      },
      {
        path: '/login',
        name: 'login',
        component: LoginView,
      },
      {
        path: '/:pathMatch(.*)*',
        redirect: { name: 'home' },
      },
    ],
  })

  // Session-expired API responses reset auth state and return to login.
  setOnUnauthorized(() => {
    useAuthStore().reset()
    void router.push({ name: 'login' })
  })

  router.beforeEach(async (to) => {
    if (to.meta.requiresAuth !== true) return
    const auth = useAuthStore()
    await auth.ensureSession()
    if (!auth.isAuthenticated) {
      return { name: 'login' }
    }
  })

  return router
}
