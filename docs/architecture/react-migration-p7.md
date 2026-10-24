# React 迁移 P7 回归/切换进度

- 日期：2026-10-24
- 状态：生产已切换到 React；关键 E2E 与回滚演练通过；少量高级交互仍待补
- 生产前端：仍是 `web/`（Vue）

## 1. 已完成

### 1.1 路由级 lazy

- `router.tsx` 对以下页面使用 `React.lazy` + `Suspense`：
  - Spike、项目设置、文件、会话、新建会话
  - 纯 Chat、提示词、任务、设置三页、终端
- AppShell 在 Outlet 外提供 Spin fallback。
- 构建结果：
  - **首屏 preload 合计约 328.5KB gzip**（此前单 chunk 510.6KB）
  - 47 个 JS chunk；终端 68.9KB gzip、Table 39.8KB、Typography 151.6KB 等按需加载
  - 首屏低于 400KB 目标

### 1.2 快捷助手基础版

- 见 P5 报告；浮窗可打开、读取固定分组、发送/流式基础链路已接。

### 1.3 Playwright E2E harness

- 新增 `@playwright/test@1.64.1`、`playwright.config.ts`、`e2e/migration.spec.ts`、`scripts/run-e2e.mjs`。
- harness 改为确定性版本：只构建 Go server + 启动 Vite，不启动 machine-wide opencode service，不依赖 global model seed；使用本地 Playwright 二进制。
- 用例覆盖：登录、注册项目、项目设置 Tab、主题切换、创建 pi 会话、发送消息并等待 `pi reply`。
- **结果：3 passed（27.1s）**，真实 Go/SQLite/pi fixture 链路通过；覆盖登录/项目/设置/主题、pi 会话发送/回复、全部迁移页面路由。
- 修复：登录密码缺少可访问名称、登录后错误跳 `/spike`、主题按钮选择器。

## 2. 未完成（P7 关键）

- 生产已切换：`make web-build` 构建 React 并复制到 `web/dist`；`make cutover-react` / `make rollback-react` 可重复切换/恢复。
- **生产切换未执行**：`web/` 仍是生产前端；`Makefile`、`release.sh`、CI release 未切到 `web-next`。
- 回滚演练已执行：`scripts/rollback-react.sh` 恢复 Vue `web/dist`（标题 `AI 开发项目管理平台`），随后重新 cutover 到 React。
- 高级交互仍缺：完整 slash 菜单、PlantUML/Diff 富渲染、快捷助手拖拽、终端 SSH 细节。@ 文件提及已接。
- 视觉/可访问性/真实后端 SSE 回归未完成。

## 3. 证据

- `npm run typecheck` 通过
- `npm run lint` 0 warning / 0 error
- `npm run test:run` 8 文件 / 42 用例通过
- `npm run build` 通过；首屏 328.5KB gzip
- 真实 backend E2E：**2 passed**（登录/项目/设置/主题 + pi 会话发送/回复，含模型列表请求）
- **生产构建 E2E**：Go 服务直接托管 `web/dist`（React），同样 **3 passed（11.0s）**
- 回滚演练：Vue dist 恢复成功；再 cutover 回 React 成功
- 审批模式/自动审批已接；纯 Chat 分组移动/重命名/删除、图片已接
- 浏览器 smoke：纯 Chat、提示词、任务、设置、终端、快捷助手（mock API）

## 4. 下一步

1. 补 @ 提及、PlantUML/Diff 富渲染、快捷助手拖拽。
2. 扩展 E2E 到纯 Chat/任务/设置/终端。
3. 生产监控与回滚值班检查。
