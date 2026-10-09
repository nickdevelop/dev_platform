// Application router: a login view and the session-protected projects view.
// The router owns the two navigation security behaviors:
//   - a navigation guard that redirects unauthenticated users to /login, and
//   - the global handler that returns the user to /login whenever an API
//     call reports an expired session (401).

import { createRouter, createWebHistory } from 'vue-router'
import { setOnUnauthorized } from './api/client'
import { useAuthStore } from './stores/auth'
import LoginView from './views/LoginView.vue'
import ProjectsView from './views/ProjectsView.vue'

export function createAppRouter() {
  const router = createRouter({
    history: createWebHistory(),
    routes: [
      {
        path: '/',
        name: 'projects',
        component: ProjectsView,
        meta: { requiresAuth: true },
      },
      {
        path: '/login',
        name: 'login',
        component: LoginView,
      },
      {
        path: '/:pathMatch(.*)*',
        redirect: { name: 'projects' },
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
