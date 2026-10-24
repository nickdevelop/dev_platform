import { lazy } from 'react'
import { createBrowserRouter, Navigate, redirect } from 'react-router'
import { setOnUnauthorized } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { LoginPage } from './LoginPage'
import { RouteError } from './RouteError'
const SpikePage = lazy(() => import('../spike/SpikePage').then((m) => ({ default: m.SpikePage })))
import { AppShell } from './AppShell'
import { HomePage } from './HomePage'
const ProjectSettingsPage = lazy(() => import('../features/projects/ProjectSettingsPage').then((m) => ({ default: m.ProjectSettingsPage })))
const NewSessionPage = lazy(() => import('../features/chat/composer/NewSessionPage').then((m) => ({ default: m.NewSessionPage })))
const SessionChatPage = lazy(() => import('../features/chat/SessionChatPage').then((m) => ({ default: m.SessionChatPage })))
const FilesPage = lazy(() => import('../features/files/FilesPage').then((m) => ({ default: m.FilesPage })))
const PureChatPage = lazy(() => import('../features/chat/pure/PureChatPage').then((m) => ({ default: m.PureChatPage })))
const PromptsPage = lazy(() => import('../features/prompts/PromptsPage').then((m) => ({ default: m.PromptsPage })))
const TasksPage = lazy(() => import('../features/tasks/TasksPage').then((m) => ({ default: m.TasksPage })))

const SettingsLayout = lazy(() => import('../features/settings/SettingsLayout').then((m) => ({ default: m.SettingsLayout })))
const AppearanceSettings = lazy(() => import('../features/settings/AppearanceSettings').then((m) => ({ default: m.AppearanceSettings })))
const ModelSettings = lazy(() => import('../features/settings/ModelSettings').then((m) => ({ default: m.ModelSettings })))
const AgentSettings = lazy(() => import('../features/settings/AgentSettings').then((m) => ({ default: m.AgentSettings })))
const TerminalPage = lazy(() => import('../features/terminal/TerminalPage').then((m) => ({ default: m.TerminalPage })))

async function requireAuth() {
  const auth = useAuthStore.getState()
  try {
    await auth.ensureSession()
  } catch {
    throw redirect('/login')
  }
  // ensureSession() updates the store, so re-read it: the snapshot captured
  // before the await is stale until the request settles.
  if (!useAuthStore.getState().authenticated) {
    throw redirect('/login')
  }
  return null
}

function redirectIfAuthenticated() {
  if (useAuthStore.getState().authenticated) {
    throw redirect('/')
  }
  return null
}

export const router = createBrowserRouter([
  {
    path: '/login',
    element: <LoginPage />,
    loader: redirectIfAuthenticated,
    errorElement: <RouteError />,
  },
  {
    path: '/',
    element: <AppShell />,
    loader: requireAuth,
    errorElement: <RouteError />,
    children: [
      { index: true, element: <HomePage /> },
      { path: 'spike', element: <SpikePage /> },
      { path: 'projects/:projectId/settings', element: <ProjectSettingsPage /> },
      { path: 'projects/:projectId/files', element: <FilesPage /> },
      { path: 'chat', element: <PureChatPage /> },
      { path: 'chat/:chatId', element: <PureChatPage /> },
      { path: 'prompts', element: <PromptsPage /> },
      { path: 'prompts/:promptId', element: <PromptsPage /> },
      { path: 'tasks', element: <TasksPage /> },
      { path: 'tasks/new', element: <TasksPage /> },
      { path: 'tasks/:taskId', element: <TasksPage /> },
      { path: 'terminal', element: <TerminalPage /> },
      {
        path: 'settings',
        element: <SettingsLayout />,
        children: [
          { index: true, element: <Navigate to="/settings/appearance" replace /> },
          { path: 'appearance', element: <AppearanceSettings /> },
          { path: 'models', element: <ModelSettings /> },
          { path: 'agents', element: <AgentSettings /> },
        ],
      },
      { path: 'projects/:projectId/sessions/new', element: <NewSessionPage /> },
      { path: 'sessions/new', element: <NewSessionPage /> },
      { path: 'projects/:projectId/sessions/:sessionId', element: <SessionChatPage /> },
    ],
  },
  {
    path: '*',
    element: <Navigate to="/" replace />,
  },
])

// A 401 from any API call resets the session and returns to login. The
// transport calls this once, so no component has to recognise an expiry.
setOnUnauthorized(() => {
  useAuthStore.getState().reset()
  void router.navigate('/login')
})
