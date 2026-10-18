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
import ProjectSettingsPanel from './features/projects/ProjectSettingsPanel.vue'
import NewSessionPanel from './features/chat/composer/NewSessionPanel.vue'
import SessionChatPanel from './features/chat/SessionChatPanel.vue'
import SettingsView from './features/settings/SettingsView.vue'
import AppearanceSettings from './features/settings/AppearanceSettings.vue'
import ModelSettings from './features/settings/ModelSettings.vue'
import AgentSettings from './features/settings/AgentSettings.vue'
import TasksView from './features/tasks/TasksView.vue'
import PromptsView from './features/prompts/PromptsView.vue'

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
            // A project is not a page of its own any more: its settings open as
            // a workbench tab, and its name in the tree only expands its sessions.
            // The old URL would otherwise render a panel no tab points to.
            path: 'projects/:projectId',
            redirect: { name: 'home' },
          },
          {
            path: 'projects/:projectId/settings',
            name: 'project-settings',
            component: ProjectSettingsPanel,
            props: (route) => ({ projectId: Number(route.params.projectId) }),
          },
          {
            path: 'settings',
            component: SettingsView,
            meta: { settings: true },
            children: [
              { path: '', redirect: { name: 'settings-appearance' } },
              { path: 'appearance', name: 'settings-appearance', component: AppearanceSettings },
              { path: 'models', name: 'settings-models', component: ModelSettings },
              { path: 'agents', name: 'settings-agents', component: AgentSettings },
            ],
          },
          {
            path: 'tasks',
            name: 'tasks',
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
            name: 'prompts',
            component: PromptsView,
            meta: { prompts: true, fullHeight: true },
          },
          {
            path: 'prompts/:promptId',
            name: 'prompt-detail',
            component: PromptsView,
            meta: { prompts: true, fullHeight: true },
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
