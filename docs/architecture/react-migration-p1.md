# React 迁移 P1 基础设施进度

- 日期：2026-10-24
- 状态：P1 完成；P2 未开始
- 生产前端：仍是 `web/`（Vue）；`web-next/` 不参与发布

## 1. 已完成

### 1.1 API 与协议层

- 从 `web/src/api/` 原样迁移 15 个 API 模块（79 个 OpenAPI path）。
- `api/client.ts` 的 `get()/mutate()`、CSRF 注入、401 处理、错误码映射保持不变。
- 迁移 `chat/state`、`chat/execution`、`plantuml/renderer`、markdown、modelCache、文件路径、任务/提示词纯逻辑。
- 安装领域依赖：`@plantuml/core`、`marked`、`dompurify`、`xterm`、`@xterm/addon-fit`。

### 1.2 真实鉴权与 CSRF

- `stores/auth.ts` 改为 Zustand 真实实现：
  - `ensureSession` 每页只查一次 `/auth/session`
  - `login` 走 `mutate()`，密码 401 不触发全局过期处理
  - `logout` 失败也清理本地状态
- `main.tsx` 在 render 前 `installCSRF({ get, refresh })`。
- 路由 loader `requireAuth` 真实等待 session；未认证重定向 `/login`。
- `setOnUnauthorized` 统一 reset + 跳转 `/login`。
- 删除 P0 fake auth。

### 1.3 平台状态

- `stores/settings.ts`：真实加载/保存 appearance、快捷助手开关；与本地 `appearance` store 同步。
- `stores/projects.ts`：真实项目列表加载、401 透传、错误文案。
- `stores/agents.ts`：后端目录加载、标签/品牌色/Capabilities 查询；实例列表基础能力。

### 1.4 Shell 与路由

- `AppShell` 真实挂载：
  - 加载 projects/agents/appearance/quick assistant
  - 项目侧栏（P2 会替换为 ProjectTree + Tabs）
  - 真实 logout
  - 主题 Tag
- `/` 为 HomePage，`/login` 为真实密码登录，`/spike` 保留为 P0 验证页（切换前删除）。
- `StrictMode` + effect guard，避免开发环境重复 bootstrap。

### 1.5 测试与 CI

- 迁移 `csrf-client.spec`（13）、`api-contracts.spec`（9）、`csrf-coverage.spec`（6）。
- 新增 `auth.spec`（3）。
- CSRF 守卫已扩展到 `.ts/.tsx/.jsx`，入口改为 `src/main.tsx`。
- `web-next` 独立 CI job：Node 22.22、`npm ci`、typecheck、lint、test、build；不进入 release。
- 当前 `web-next`：**7 文件 / 39 用例通过**，typecheck/lint 0 error，build 通过。

## 2. 当前包体

| 项 | P0 | P1 |
|---|---|---|
| JS gzip | 373.1KB | **376.8KB** |
| 静态 CSS gzip | 1.26KB | 1.57KB |
| 构建 modules | 1593 | 1593 |

包体仍高于旧版 230KB；P2 起必须做路由级懒加载，并在 P4 前复测。

## 3. 已知问题

- `AppShell` 目前是 P1 最小壳：没有 ProjectTree、SessionTabs、会话页、设置页；P2 替换。
- `settings.ts` 通过 `useAppearanceStore.subscribe` 同步本地主题，P2 需要确认不会造成多余渲染。
- 没有真实后端 E2E；CSRF/契约测试使用 fetch stub。P7 才跑完整 E2E。
- `/spike` 验证页仍在路由中；切换前必须移除。
- `hashed` 保持 antd 默认；主题桥仍依赖 `#app .platform` 主色覆盖。

## 4. P2 入口

1. 迁移 `ProjectTree`、`SessionTabs`、`openTabs`、`drafts`、`AppShell` 三栏与侧栏持久化。
2. 迁移项目设置/注册/新建/移动/重命名/解除登记。
3. 加入路由级 lazy，先拆 `/spike`、设置/终端等非首屏页面。
4. 给 `web-next` 增加 Playwright 共享 E2E 入口（P7 全量跑）。
