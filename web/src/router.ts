// Application router: the login view outside the shell, and the three-pane
// shell for every authenticated view. The shell's child routes select the right
// pane (project settings, new task, or task conversation) and pass the ids from
// the URL, so a refreshed or shared task URL restores the same selection.
// The router owns the two navigation security behaviors:
//   - a navigation guard that redirects unauthenticated users to /login, and
//   - the global handler that returns the user to /login whenever an API
//     call reports an expired session (401).

import { createRouter, createWebHistory } from 'vue-router'
import { setOnUnauthorized } from './api/client'
import { useAuthStore } from './stores/auth'
import AppShell from './components/AppShell.vue'
import EmptyWorkspace from './views/EmptyWorkspace.vue'
import LoginView from './views/LoginView.vue'
import ProjectSettingsPanel from './components/ProjectSettingsPanel.vue'
import NewTaskPanel from './components/NewTaskPanel.vue'
import TaskChatPanel from './components/TaskChatPanel.vue'
import SettingsView from './views/SettingsView.vue'
import AppearanceSettings from './components/settings/AppearanceSettings.vue'
import ModelSettings from './components/settings/ModelSettings.vue'
import AgentSettings from './components/settings/AgentSettings.vue'

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
            path: 'projects/:projectId',
            name: 'project',
            component: ProjectSettingsPanel,
            props: (route) => ({ projectId: Number(route.params.projectId) }),
          },
          {
            path: 'settings',
            name: 'settings',
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
            path: 'projects/:projectId/tasks/new',
            name: 'task-new',
            component: NewTaskPanel,
            props: (route) => ({ projectId: Number(route.params.projectId) }),
          },
          {
            path: 'projects/:projectId/tasks/:sessionId',
            name: 'task',
            component: TaskChatPanel,
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
