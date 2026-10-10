# 平台界面布局重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有多页面界面改为三栏工作台：最左大类栏、左侧项目/任务树（任务带 Agent 后端标签）、右侧按选中内容切换为项目设置、任务对话或新建任务。

**Architecture:** 单壳布局 `AppShell` 常驻大类栏与项目树，右侧由嵌套子路由决定内容。现有 `ProjectsView`、`ProjectAgentsView`、`SessionWorkbenchView` 的业务逻辑拆分为独立面板组件（`ProjectSettingsPanel`、`TaskChatPanel`、`NewTaskPanel`）与对话框组件（`ProjectManageDialog`），并复用现有 Pinia store 与 API 模块。后端与数据模型不变。

**Tech Stack:** Vue 3 (`<script setup>`)、vue-router 5、Pinia 4、Vitest 5 + @vue/test-utils（jsdom）、Playwright（e2e）、TypeScript。

**Spec:** `docs/superpowers/specs/2026-10-10-ui-layout-redesign-design.md`（同时参照已确认的原型 `/tmp/opencode/prototypes/01-03`）

## Global Constraints

- 不修改后端接口（`internal/`、`docs/api/openapi.yaml`）与数据模型。
- 大类栏首期仅 `project` 可用；`Chat` 为禁用的预留按钮。
- 旧路由 `/projects/:id/agents`、`/projects/:id/workbench` 直接删除，不设重定向。
- 项目树默认只展开当前选中的项目；其余项目保持折叠。
- 解除登记、移动、重命名、删除目录沿用现有对话框的确认规则：移动/重命名/删除需输入项目名；删除遇回收站不可用时需显式勾选永久删除。
- 不支持的能力显示为置灰并说明原因，或不渲染；不展示虚假的统一操作。
- 会话事件流只绑定当前选中的会话，切换会话必须先取消旧订阅。
- 资源区（Skill/MCP/提示词）为占位，不调用任何资源接口。
- `/login` 不渲染三栏壳。

## Review Focus

1. 搜索过滤后选中的任务被隐藏：右侧对话仍保持当前会话，不因过滤被清空或串到其他会话。（Task 1、Task 4）
2. 项目下没有任何任务：项目树显示空态与「+ 新建任务」，右侧不报错。（Task 1）
3. 直接访问不存在或已移除的项目 ID（如 `/projects/999`）：右侧显示「项目不存在」提示，不崩溃，不渲染其他项目的内容。（Task 6）
4. 在任务页面刷新浏览器：根据 URL 恢复同一个任务，树中对应项目展开并高亮该任务。（Task 4、Task 6）
5. 切换任务时旧任务的流式消息不得出现在新任务中。（Task 4）

---

### Task 1: 项目 store、后端标签与项目树

**Files:**
- Create: `web/src/stores/projects.ts`
- Create: `web/src/components/BackendTag.vue`
- Create: `web/src/components/ProjectTree.vue`
- Test: `web/src/tests/project-tree.spec.ts`

**Interfaces:**
- Consumes: `listProjects()`、`Project` from `web/src/api/projects.ts`；`listProjectSessions(projectId)`、`AgentSession` 与 `useSessionsStore` from `web/src/api/sessions.ts`、`web/src/stores/sessions.ts`。
- Produces:
  - `useProjectsStore()`：state `{ projects: Project[]; loading: boolean; error: string }`；action `load(): Promise<void>`；getter `byId(id: number): Project | undefined`。
  - `BackendTag`：props `{ backend: BackendId }`；显示文案 `opencode-v2` → `opencode`，`pi` → `pi`；class 为 `tag opencode` 或 `tag pi`。
  - `ProjectTree`：props `{ activeProjectId: number | null; activeSessionId: string | null; search: string }`；emits `select-project(projectId: number)`、`select-session(projectId: number, sessionId: string)`、`new-task(projectId: number)`。
  - 任务状态文案映射：`active` → 运行中，`stopped` → 空闲，`disconnected` → 断开。

- [ ] **Step 1: 写失败测试 `project-tree.spec.ts`**

  覆盖以下断言（使用 fake `listProjects` 与 `listProjectSessions`，沿用 `workbench.spec.ts` 中的 fetch/SSE 桩写法）：
  - 只展开 `activeProjectId` 对应项目，其他项目的任务列表不渲染。
  - 任务节点显示 `BackendTag` 与状态文案。
  - `search` 无匹配时，项目仍显示「没有匹配的任务」空态，`activeSessionId` 不被清除（不触发任何 emit）。
  - 项目无任务时显示「+ 新建任务」，点击 emit `new-task(projectId)`。

- [ ] **Step 2: 运行测试确认失败**

  Run: `cd web && npx vitest run src/tests/project-tree.spec.ts`
  Expected: FAIL，提示组件或 store 不存在。

- [ ] **Step 3: 实现 store、`BackendTag.vue`、`ProjectTree.vue`**

  - store 直接包装 `listProjects`，错误用 `describeApiError`，`UnauthorizedError` 直接返回（由全局处理跳转登录），与 `ProjectsView.load` 保持一致。
  - 展开状态为组件内 ref，用 `watch(activeProjectId)` 将展开项切换为当前项目。
  - 任务列表读取 `useSessionsStore().sessions`，仅当展开项目为 `activeProjectId` 时调用 `loadSessions`。
  - 项目行上的任务计数只对展开项目显示，避免为每个项目发起请求。

- [ ] **Step 4: 运行测试确认通过**

  Run: `cd web && npx vitest run src/tests/project-tree.spec.ts`
  Expected: PASS

- [ ] **Step 5: 提交**

  ```bash
  git add web/src/stores/projects.ts web/src/components/BackendTag.vue web/src/components/ProjectTree.vue web/src/tests/project-tree.spec.ts
  git commit -m "feat: add project tree with backend tags"
  ```

---

### Task 2: 抽出项目管理对话框

**Files:**
- Create: `web/src/components/ProjectManageDialog.vue`
- Modify: `web/src/views/ProjectsView.vue`（逻辑迁出，本任务结束时仍保留旧页面可运行）
- Test: `web/src/tests/project-manage-dialog.spec.ts`

**Interfaces:**
- Consumes: `ProjectDialog.vue`；`createProject`、`deleteProject`、`moveProject`、`registerProject`、`renameProject`、`unregisterProject` from `web/src/api/projects.ts`；`useAuthStore` 提供 `csrfToken`。
- Produces: `ProjectManageDialog`：props `{ kind: 'register' | 'create' | 'move' | 'rename' | 'unregister' | 'delete'; project: Project | null }`；emits `close()`、`submitted(kind)`。

- [ ] **Step 1: 写失败测试**

  断言：
  - `move`、`rename`、`delete` 时，输入的确认名与项目名不一致则提交按钮禁用。
  - `delete` 收到 `trash_unavailable` 错误时，显示永久删除勾选框，未勾选时提交按钮禁用。
  - `unregister` 不需要确认名。
  - 提交成功后 emit `submitted(kind)` 并 emit `close()`。

- [ ] **Step 2: 运行测试确认失败**

  Run: `cd web && npx vitest run src/tests/project-manage-dialog.spec.ts`
  Expected: FAIL

- [ ] **Step 3: 实现**

  将 `ProjectsView.vue` 中 `dialogTitle`、`confirmLabel`、`canSubmit`、`submitDialog` 及对应模板迁入新组件；`ProjectsView` 改为使用该组件，行为不变。

- [ ] **Step 4: 运行测试与既有 e2e 相关用例**

  Run: `cd web && npx vitest run src/tests/project-manage-dialog.spec.ts && npm run typecheck`
  Expected: PASS，类型检查无错误。

- [ ] **Step 5: 提交**

  ```bash
  git add web/src/components/ProjectManageDialog.vue web/src/views/ProjectsView.vue web/src/tests/project-manage-dialog.spec.ts
  git commit -m "refactor: extract project manage dialog"
  ```

---

### Task 3: 项目设置面板

**Files:**
- Create: `web/src/components/ProjectSettingsPanel.vue`
- Test: `web/src/tests/project-settings-panel.spec.ts`

**Interfaces:**
- Consumes: `useProjectsStore`（Task 1）、`useAgentsStore`（`loadBackends`、`loadInstances`、`loadDiagnostics`、`loadBackendConfigs`、`setBackendEnabled`、`startInstance`、`stopInstance`、`backendEnabledFor`、`capabilitiesFor`）、`AgentInstanceList`、`BackendDiagnostics`；`ProjectAgentsView.vue` 的启用/启动/停止逻辑。
- Produces: `ProjectSettingsPanel`：props `{ projectId: number }`；emits `manage(kind: 'move' | 'rename' | 'unregister' | 'delete', project: Project)`。

- [ ] **Step 1: 写失败测试**

  断言：
  - 渲染项目基本信息（名称、路径）。
  - 后端启用开关调用 `setBackendEnabled`，禁用的后端其启动按钮不可用。
  - 点击「解除登记」「删除目录」「移动」「重命名」分别 emit 对应 `manage`。
  - 资源区显示「待资源管理功能接入」占位，且 `fetch` 未被调用于任何资源接口。

- [ ] **Step 2: 运行测试确认失败**

  Run: `cd web && npx vitest run src/tests/project-settings-panel.spec.ts`
  Expected: FAIL

- [ ] **Step 3: 实现**

  - 布局对照原型 `01-project-settings.html`；删除原型中的「打开任务」按钮与「提示词」行，因为无对应接口。
  - 启动、停止、后端启用逻辑从 `ProjectAgentsView.vue` 原样迁移。
  - 「危险操作」区保留说明文字。

- [ ] **Step 4: 运行测试确认通过**

  Run: `cd web && npx vitest run src/tests/project-settings-panel.spec.ts`
  Expected: PASS

- [ ] **Step 5: 提交**

  ```bash
  git add web/src/components/ProjectSettingsPanel.vue web/src/tests/project-settings-panel.spec.ts
  git commit -m "feat: add project settings panel"
  ```

---

### Task 4: 任务对话面板

**Files:**
- Create: `web/src/components/TaskChatPanel.vue`
- Test: `web/src/tests/task-chat-panel.spec.ts`

**Interfaces:**
- Consumes: `useSessionsStore`（`loadSessions`、`activateSession`、`sendMessage`、`stopSession`、`continueSession`、`resubscribe`、`unsubscribeAll`、`messagesFor`、`statusFor`、`activeSession`）、`useAgentsStore`、`MessageView`、`CapabilityControls`、`BackendTag`（Task 1）。
- Produces: `TaskChatPanel`：props `{ projectId: number; sessionId: string }`；无 emits。

- [ ] **Step 1: 写失败测试**

  断言：
  - 挂载时若会话列表尚未加载，先 `loadSessions(projectId)` 再 `activateSession(projectId, sessionId)`（覆盖直接刷新 URL 的场景）。
  - 会话不存在时显示「任务不存在」，不抛错。
  - `sessionId` 变化时先取消旧会话订阅，旧会话的消息不出现在新会话中（Review Focus 5）。
  - 不支持 `toolConfirmation` 的会话不渲染确认入口；不支持 `modelSelection` 时模型输入不渲染。
  - 页头显示 `BackendTag` 与状态文案，不显示重命名按钮。

- [ ] **Step 2: 运行测试确认失败**

  Run: `cd web && npx vitest run src/tests/task-chat-panel.spec.ts`
  Expected: FAIL

- [ ] **Step 3: 实现**

  - 对话区的消息、停止、继续、发送、重连逻辑从 `SessionWorkbenchView.vue` 迁移，结构与原有测试期望保持一致。
  - 监听 `sessionId` 变化：调用 `sessions.activateSession`，并在切换前由 store 取消旧订阅（store 已支持）。
  - 删除原型中的「重命名」按钮与「提示词」选择（无接口）。
  - 页头保留历史消息提示文案（`history-note`）。

- [ ] **Step 4: 运行测试确认通过**

  Run: `cd web && npx vitest run src/tests/task-chat-panel.spec.ts`
  Expected: PASS

- [ ] **Step 5: 提交**

  ```bash
  git add web/src/components/TaskChatPanel.vue web/src/tests/task-chat-panel.spec.ts
  git commit -m "feat: add task chat panel"
  ```

---

### Task 5: 新建任务面板

**Files:**
- Create: `web/src/components/NewTaskPanel.vue`
- Test: `web/src/tests/new-task-panel.spec.ts`

**Interfaces:**
- Consumes: `useAgentsStore`（`runningInstances`、`capabilitiesFor`、`backends`）、`useSessionsStore`（`createSession`、`discoverNativeSessions`）、`BackendTag`。
- Produces: `NewTaskPanel`：props `{ projectId: number }`；emits `created(sessionId: string)`。

- [ ] **Step 1: 写失败测试**

  断言：
  - 实例下拉只列出运行中的实例，每项带 `BackendTag`。
  - 选中的实例不支持 `modelSelection` 时，模型输入不渲染；支持时可填写并随创建请求提交。
  - 选中的实例不支持 `sessionDiscovery` 时，「发现原生会话」按钮不渲染。
  - 创建成功后 emit `created(newSessionId)`。
  - 无运行中实例时创建按钮禁用，并提示「请先在项目设置中启动实例」。

- [ ] **Step 2: 运行测试确认失败**

  Run: `cd web && npx vitest run src/tests/new-task-panel.spec.ts`
  Expected: FAIL

- [ ] **Step 3: 实现**

  表单字段与能力判断沿用 `SessionWorkbenchView.vue` 中 `create-session` 表单的逻辑。成功后由父组件负责路由跳转（见 Task 6）。

- [ ] **Step 4: 运行测试确认通过**

  Run: `cd web && npx vitest run src/tests/new-task-panel.spec.ts`
  Expected: PASS

- [ ] **Step 5: 提交**

  ```bash
  git add web/src/components/NewTaskPanel.vue web/src/tests/new-task-panel.spec.ts
  git commit -m "feat: add new task panel"
  ```

---

### Task 6: 三栏壳、大类栏与路由

**Files:**
- Create: `web/src/components/AppShell.vue`
- Create: `web/src/components/CategoryRail.vue`
- Create: `web/src/views/EmptyWorkspace.vue`
- Modify: `web/src/router.ts`
- Test: `web/src/tests/app-shell.spec.ts`

**Interfaces:**
- Consumes: `ProjectTree`（Task 1）、`ProjectSettingsPanel`（Task 3）、`TaskChatPanel`（Task 4）、`NewTaskPanel`（Task 5）、`ProjectManageDialog`（Task 2）、`WorkspaceRootsDialog.vue`（既有）、`useProjectsStore`（Task 1）、`useAuthStore`（既有）。
- Produces:
  - 路由（均在 `/` 的子路由下，除 `/login` 外均带 `requiresAuth`）：
    - `''` → `EmptyWorkspace`，name `home`
    - `projects/:projectId` → `ProjectSettingsPanel`，name `project`
    - `projects/:projectId/tasks/new` → `NewTaskPanel`，name `task-new`
    - `projects/:projectId/tasks/:sessionId` → `TaskChatPanel`，name `task`
    - `/login` → `LoginView`（不带壳）
    - `/:pathMatch(.*)*` → 重定向到 `home`
  - `AppShell`：无 props；管理 `ProjectManageDialog` 与 `WorkspaceRootsDialog` 的打开状态；将 `ProjectSettingsPanel` 的 `manage` 事件转为对话框；`NewTaskPanel` 的 `created` 事件转为 `router.push({ name: 'task', ... })`。
  - `CategoryRail`：无 props；emits `open-roots()`、`logout()`。

- [ ] **Step 1: 写失败测试**

  断言：
  - 已登录访问 `/` 显示大类栏与项目树，右侧为 `EmptyWorkspace`。
  - 访问 `/projects/1` 时右侧渲染项目设置；访问 `/projects/1/tasks/s1` 时右侧渲染对话。
  - 访问 `/projects/999` 且项目不存在时右侧显示「项目不存在或已移除」，不渲染其他项目的内容（Review Focus 3）。
  - 刷新式直接访问 `/projects/1/tasks/s1` 时，树中项目 1 展开且会话 s1 高亮（Review Focus 4）。
  - 新建任务返回 `created` 后路由跳转到对应任务页。
  - 未登录访问任何壳内路由跳转 `/login`。
  - `/login` 不渲染大类栏。
  - 大类栏中 `Chat` 按钮为 `disabled`。

- [ ] **Step 2: 运行测试确认失败**

  Run: `cd web && npx vitest run src/tests/app-shell.spec.ts`
  Expected: FAIL

- [ ] **Step 3: 实现**

  - `router.ts`：按上面路由表重写；保留 `setOnUnauthorized` 与 `beforeEach` 守卫逻辑，守卫判断 `to.meta.requiresAuth`。
  - `AppShell.vue` 三栏布局对照原型 `02-task-chat.html`；`projectId` 从 `route.params` 解析，`useProjectsStore().load()` 在挂载时调用。
  - 项目不存在判断：`projects` 加载完成后 `byId(projectId)` 为空则显示提示，子面板不挂载。
  - 项目被移除或删除后（`submitted` 且 kind 为 `unregister`/`delete`）路由回到 `home`。
  - 侧栏头部提供「注册」与「新建」两个按钮，分别打开 `ProjectManageDialog`（kind `register` / `create`）。
  - `CategoryRail` 底部放置设置（打开 `WorkspaceRootsDialog`）与退出登录。

- [ ] **Step 4: 运行测试确认通过**

  Run: `cd web && npx vitest run src/tests/app-shell.spec.ts && npm run typecheck`
  Expected: PASS，类型检查无错误。

- [ ] **Step 5: 提交**

  ```bash
  git add web/src/components/AppShell.vue web/src/components/CategoryRail.vue web/src/views/EmptyWorkspace.vue web/src/router.ts web/src/tests/app-shell.spec.ts
  git commit -m "feat: add three-pane app shell and routes"
  ```

---

### Task 7: 删除旧页面并迁移测试与 e2e

**Files:**
- Delete: `web/src/views/ProjectsView.vue`、`web/src/views/ProjectAgentsView.vue`、`web/src/views/SessionWorkbenchView.vue`
- Modify: `web/src/tests/login.spec.ts`、`web/src/tests/workbench.spec.ts`、`web/src/tests/diagnostics.spec.ts`
- Modify: `web/e2e/projects.spec.ts`、`web/e2e/sessions.spec.ts`、`web/e2e/agent-failures.spec.ts`、`web/e2e/security.spec.ts`

**Interfaces:**
- Consumes: 所有新组件（Task 1–6）。
- Produces: 无新接口。

- [ ] **Step 1: 查找旧引用**

  Run: `cd web && grep -rn "ProjectsView\|ProjectAgentsView\|SessionWorkbenchView\|project-agents\|session-workbench\|/workbench\|/agents'" src e2e`
  Expected: 列出所有需要迁移的位置。

- [ ] **Step 2: 更新单元测试**

  将 `login.spec.ts`、`workbench.spec.ts`、`diagnostics.spec.ts` 中挂载旧视图的用例改为挂载 `AppShell` 或对应新面板，断言保持原有安全与隔离行为（每个实例的事件不串流、停止不影响其他实例、诊断信息脱敏）；不得删减这些断言。

- [ ] **Step 3: 更新 e2e**

  - 选择器从 `.project-item` 改为树中的项目行（`.project-row`）；按钮文案保持不变时直接复用。
  - 「注册已有目录」「新建项目」改为侧栏「注册」「新建」。
  - 「Agent 设置」「打开会话工作台」入口改为点击项目与任务。
  - 安全相关用例（认证、CSRF、路径校验）不得放宽。

- [ ] **Step 4: 删除旧文件并运行测试**

  Run: `cd web && rm src/views/ProjectsView.vue src/views/ProjectAgentsView.vue src/views/SessionWorkbenchView.vue && npx vitest run && npm run typecheck`
  Expected: 全部 PASS。

- [ ] **Step 5: 提交**

  ```bash
  git add -A web/src web/e2e
  git commit -m "refactor: remove legacy project and workbench views"
  ```

---

### Task 8: 最终验证与视觉核对

**Files:**
- 无代码改动，除非验证发现问题。

**Interfaces:**
- Consumes: 全部前序任务产出。
- Produces: 验证记录（命令与结果）。

- [ ] **Step 1: 运行全部单元与类型检查**

  Run: `cd web && npx vitest run && npm run typecheck`
  Expected: 全部 PASS。

- [ ] **Step 2: 构建**

  Run: `cd web && npm run build`
  Expected: 构建成功，`web/dist` 更新。

- [ ] **Step 3: 运行 e2e**

  Run: `cd web && npm run test:e2e`
  Expected: 全部 PASS。

- [ ] **Step 4: 视觉核对**

  用 Playwright 截取项目设置、任务对话、新建任务三个页面，与原型逐项比对：大类栏、项目树展开规则、标签颜色、右侧区块顺序。偏差需记录到最终说明中。

- [ ] **Step 5: 提交验证记录（如有修改）**

  ```bash
  git add -A
  git commit -m "chore: verify ui layout redesign"
  ```

---

## 与原型的已知差异

以下差异因后端接口不存在或成本考虑而调整，已由用户确认：

1. 任务页「重命名」按钮：后端无会话重命名接口，移除。
2. 任务页「提示词」选择：依赖计划 3 的提示词接口，移除，待计划 3 完成后补入。
3. 项目树中非展开项目的任务计数：不显示，避免为每个项目发起请求。
4. 项目设置「打开任务」按钮：移除，点击项目树中的任务即可。
