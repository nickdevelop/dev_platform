# React 迁移 P2 Shell/项目进度

- 日期：2026-10-24
- 状态：P2 完成（会话内容仍为占位页，P4 接入）
- 生产前端：仍是 `web/`（Vue）

## 1. 已完成

### 1.1 Tabs 与持久化

- `stores/openTabs.ts` 从 Pinia 迁移为 Zustand：
  - 保留 `dev-platform-open-tabs-v2`、`dev-platform-active-tab-key` 与旧 key 迁移
  - 20 个标签上限、固定标签优先、关闭其他/全部、任务/平台设置标签
- `SessionTabs`：antd Dropdown 右键菜单、固定/关闭/关闭其他/全部、键盘 Enter/Space。

### 1.2 项目导航

- `ProjectTree`：
  - 项目/会话两级、搜索、展开、会话时间/模型/BackendTag
  - 会话删除使用 `App.useApp().modal.confirm`（不再 `window.confirm`）
  - 保留 `project-row` / `session-row` data-test
- `BackendTag`：品牌色使用 CSS `color-mix`，未知后端回退中性色。
- `stores/sessions.ts`：项目会话索引、加载/刷新、删除、错误；P4 会扩展为完整会话 store。
- `stores/projects.ts` / `stores/agents.ts` 已在 P1 完成，P2 接入树。

### 1.3 项目设置/管理

- `ProjectSettingsPage`：项目名称/路径与危险操作入口。
- `ProjectManageModal`：注册、新建、移动、重命名目录、修改显示名、解除登记；antd Form 校验 + 输入项目名确认；保留 `dialog-path`、`dialog-name`、`dialog-confirm-name` 等 id/label 语义。

### 1.4 路由与 Shell

- AppShell 三栏：
  - 左：项目搜索/注册/新建 + ProjectTree
  - 右：SessionTabs + Outlet
  - 顶栏：主题 Tag、退出登录、P0 验证页入口
- 路由新增：
  - `/projects/:projectId/settings`
  - `/projects/:projectId/sessions/new`、`/sessions/new`
  - `/projects/:projectId/sessions/:sessionId`
- 路由/Tab 同步：进入设置或新会话页自动登记标签，关闭当前标签回到剩余标签或首页。

## 2. 验证

| 项 | 结果 |
|---|---|
| typecheck | 通过 |
| lint | 0 warning / 0 error |
| 单测 | **7 文件 / 39 用例通过** |
| build | 通过；3210 modules |
| JS gzip | **384.4KB**（P1 376.8KB） |
| 静态 CSS gzip | 2.44KB |
| 浏览器 smoke（mock API） | 项目树展开、会话显示、设置标签打开/关闭、新会话标签、无 console error |

## 3. 已知缺口

- `/sessions/new` 与 `/sessions/:id` 仍是占位页，P4 接入 Composer/消息流。
- `drafts` store、会话标签运行态、关闭标签时释放 SSE 尚未迁移。
- Sidebar 折叠/浮动、P0 验证页入口、快捷助手尚未迁移。
- `activeProjectId` 仍传 null；P4 从路由解析。
- 没有真实后端 E2E；P2 smoke 使用 Playwright 路由 mock。

## 4. P3/P4 入口

1. P3：文件树（antd Tree）、文件预览、Git 状态；供 Chat 右侧预览复用。
2. P4：完整 sessions store + EventBus + 消息渲染 + Composer + 审批/表单；把两个占位页替换为真实页面。
3. 在 P4 前加入路由级 lazy，控制当前 384KB gzip。
