import { Suspense, useEffect, useMemo, useRef, useState } from 'react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router'
import { Alert, Button, Input, Space, Spin, Tag, Typography } from 'antd'
import { useAgentsStore } from '../stores/agents'
import { useAppearanceStore } from '../stores/appearance'
import { useAuthStore } from '../stores/auth'
import { useOpenTabsStore, tabKey, type OpenTab } from '../stores/openTabs'
import { useProjectsStore } from '../stores/projects'
import { useSettingsStore } from '../stores/settings'
import { ProjectTree } from '../features/projects/ProjectTree'
import { ProjectManageModal, type ProjectDialogKind } from '../features/projects/ProjectManageModal'
import { SessionTabs } from '../features/workspace/SessionTabs'
import { QuickAssistant } from '../features/chat/quick/QuickAssistant'
import styles from './AppShell.module.css'

function routeTab(pathname: string): { key: string | null; open: (() => void) | null } {
  const settings = pathname.match(/^\/projects\/(\d+)\/settings$/)
  if (settings) {
    const projectId = Number(settings[1])
    return {
      key: `settings:${projectId}`,
      open: () => useOpenTabsStore.getState().openProjectSettings(projectId),
    }
  }
  if (pathname === '/sessions/new' || /^\/projects\/\d+\/sessions\/new$/.test(pathname)) {
    return { key: 'new-session', open: () => useOpenTabsStore.getState().openNewSession() }
  }
  const session = pathname.match(/^\/projects\/(\d+)\/sessions\/([^/]+)$/)
  if (session) {
    return {
      key: session[2],
      open: () => useOpenTabsStore.getState().openTab(Number(session[1]), session[2]),
    }
  }
  return { key: null, open: null }
}

export function AppShell() {
  const navigate = useNavigate()
  const location = useLocation()
  const authenticated = useAuthStore((state) => state.authenticated)
  const logout = useAuthStore((state) => state.logout)
  const projects = useProjectsStore((state) => state.projects)
  const projectsLoading = useProjectsStore((state) => state.loading)
  const projectsError = useProjectsStore((state) => state.error)
  const loadProjects = useProjectsStore((state) => state.load)
  const loadBackends = useAgentsStore((state) => state.loadBackends)
  const loadAppearance = useSettingsStore((state) => state.loadAppearance)
  const loadQuickAssistant = useSettingsStore((state) => state.loadQuickAssistant)
  const quickAssistantEnabled = useSettingsStore((state) => state.quickAssistant.enabled)
  const appearance = useAppearanceStore((state) => state.appearance)
  const tabs = useOpenTabsStore((state) => state.tabs)
  const lastActiveKey = useOpenTabsStore((state) => state.lastActiveKey)
  const bootstrapped = useRef(false)
  const [search, setSearch] = useState('')
  const [dialog, setDialog] = useState<{ kind: ProjectDialogKind } | null>(null)

  useEffect(() => {
    if (bootstrapped.current) return
    bootstrapped.current = true
    void loadBackends()
    void loadProjects()
    void loadAppearance()
    void loadQuickAssistant()
  }, [loadBackends, loadProjects, loadAppearance, loadQuickAssistant])

  const route = routeTab(location.pathname)
  const isFullMode = ['/chat', '/prompts', '/tasks', '/terminal', '/settings'].some((prefix) =>
    location.pathname.startsWith(prefix),
  )

  useEffect(() => {
    routeTab(location.pathname).open?.()
  }, [location.pathname])

  const workspaceTabs = useMemo(
    () =>
      tabs.filter(
        (tab) =>
          tab.kind === 'session' ||
          tab.kind === 'new-session' ||
          tab.kind === 'project-settings',
      ),
    [tabs],
  )

  const activeKey = route.key ?? lastActiveKey

  function navigateToTab(tab: OpenTab) {
    if (tab.kind === 'project-settings') {
      navigate(`/projects/${tab.projectId}/settings`)
      return
    }
    if (tab.kind === 'new-session') {
      navigate('/sessions/new')
      return
    }
    if (tab.kind === 'session') {
      navigate(`/projects/${tab.projectId}/sessions/${tab.sessionId}`)
    }
  }

  function closeTab(tab: OpenTab) {
    if (tab.pinned === true) return
    const wasActive = tabKey(tab) === activeKey
    useOpenTabsStore.getState().closeTabByKey(tabKey(tab))
    if (wasActive) {
      const remaining = workspaceTabs.filter((entry) => tabKey(entry) !== tabKey(tab))
      if (remaining.length > 0) navigateToTab(remaining[remaining.length - 1])
      else navigate('/')
    }
  }

  function closeOthers(tab: OpenTab) {
    useOpenTabsStore.getState().closeOtherTabsByKey(tabKey(tab))
    navigateToTab(tab)
  }

  function closeAll() {
    useOpenTabsStore.getState().closeAllTabs()
    navigate('/')
  }

  function togglePin(tab: OpenTab) {
    useOpenTabsStore.getState().togglePin(tabKey(tab))
  }

  async function onLogout() {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <div className={styles.shell}>
      <header className={styles.header}>
        <div className={styles.brand}>
          <strong>dev_platform</strong>
          <span className={styles.subtitle}>多 Agent 协同研发工作台</span>
          <Tag color="processing">{appearance.theme}</Tag>
        </div>
        <div className={styles.headerActions}>
          <Link className={styles.devLink} to="/chat">
            Chat
          </Link>
          <Link className={styles.devLink} to="/prompts">
            提示词
          </Link>
          <Link className={styles.devLink} to="/tasks">
            任务
          </Link>
          <Link className={styles.devLink} to="/terminal">
            终端
          </Link>
          <Link className={styles.devLink} to="/spike">
            P0 验证页
          </Link>
          <Button size="small" onClick={() => void onLogout()} disabled={!authenticated}>
            退出登录
          </Button>
        </div>
      </header>

      <div className={`${styles.body} ${isFullMode ? styles.bodyChat : ''}`}>
        {!isFullMode && (
        <aside className={styles.sidebar} aria-label="项目导航">
          <div className={styles.sidebarHead}>
            <Typography.Text strong>项目</Typography.Text>
            <Typography.Text type="secondary">{projects.length}</Typography.Text>
          </div>
          <Space size={4}>
            <Button size="small" onClick={() => setDialog({ kind: 'register' })}>
              注册项目
            </Button>
            <Button size="small" type="primary" onClick={() => setDialog({ kind: 'create' })}>
              新建项目
            </Button>
          </Space>
          <Input.Search
            allowClear
            size="small"
            placeholder="搜索项目或会话"
            aria-label="搜索项目或会话"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          {projectsError !== '' && <Alert type="error" showIcon title={projectsError} />}
          <ProjectTree
            projects={projects}
            activeProjectId={null}
            activeSessionId={route.key}
            search={search}
            loading={projectsLoading}
            onNewSession={(projectId) => navigate(`/projects/${projectId}/sessions/new`)}
            onOpenSettings={(projectId) => navigate(`/projects/${projectId}/settings`)}
            onSelectSession={(projectId, sessionId) =>
              navigate(`/projects/${projectId}/sessions/${sessionId}`)
            }
          />
        </aside>
        )}

        <main className={styles.pane}>
          <SessionTabs
            tabs={workspaceTabs}
            activeKey={activeKey}
            onSelect={navigateToTab}
            onClose={closeTab}
            onTogglePin={togglePin}
            onCloseOthers={closeOthers}
            onCloseAll={closeAll}
          />
          <div className={styles.paneBody}>
            <Suspense
              fallback={
                <div style={{ display: 'flex', justifyContent: 'center', padding: '3rem' }}>
                  <Spin />
                </div>
              }
            >
              <Outlet />
            </Suspense>
          </div>
        </main>
      </div>

      {quickAssistantEnabled && <QuickAssistant />}

      {dialog && (
        <ProjectManageModal
          key={dialog.kind}
          kind={dialog.kind}
          project={null}
          onClose={() => setDialog(null)}
          onSubmitted={() => void loadProjects()}
        />
      )}
    </div>
  )
}
