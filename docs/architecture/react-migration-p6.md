# React 迁移 P6 平台模块（进行中）

- 日期：2026-10-24
- 状态：提示词、任务、设置、终端基础完成；快捷助手与部分高级能力待补
- 生产前端：仍是 `web/`（Vue）

## 1. 已完成

### 1.1 提示词

- `stores/prompts.ts`：列表、创建、更新、删除、按 id 查询。
- [PromptsPage](web-next/src/features/prompts/PromptsPage.tsx)：
  - `/prompts`、`/prompts/:promptId`
  - 左侧搜索/选择/新建；右侧详情、模版、`{user_prompt}` 占位符标记
  - 模拟输入 + 实时合成预览
  - 新建/编辑 antd Modal + Form；删除确认
- AppShell 在 `/prompts` 下隐藏项目侧栏，页面自带列表栏。
- 复用 `promptTemplate.ts` 的 `applyUserPromptTemplate` / `hasUserPromptPlaceholder`。

### 1.2 任务

- `stores/tasks.ts`：列表、按 id 获取、创建、更新、删除、触发、执行历史。
- /tasks、/tasks/new、/tasks/:taskId 与 `TasksPage`：
  - 左侧搜索/列表/新建；右侧创建表单或详情
  - 创建：项目、Agent、模型、提示词模板、手动/单次/Cron、调度开关
  - 详情：状态、调度、提示词、执行历史 Table、立即执行、删除
  - 运行记录可跳转到对应会话
- AppShell 在 /tasks 下隐藏项目侧栏。

### 1.3 设置

- `/settings` + `SettingsLayout`（外观/模型/Agent 导航）。
- 外观：4 主题、3 密度、字号、PlantUML 模式/地址、快捷助手开关；复用 appearance store 与 settings store。
- 模型：服务列表、模型 Table、新增服务/模型、删除服务。
- Agent：后端列表、启用开关、手动路径、检测、版本/模式展示。

### 1.4 终端

- `/terminal` + `TerminalPage`：
  - 本机终端 + SSH 服务器列表、添加/删除服务器
  - `XtermPane`：xterm + FitAddon + WebSocket + ResizeObserver + 清理
- 终端深色 palette 为领域例外，未强行主题化。

## 2. 验证

| 项 | 结果 |
|---|---|
| typecheck | 通过 |
| lint | 0 warning / 0 error |
| 单测 | 8 文件 / 42 用例通过（提示词 UI 尚未新增组件测试） |
| build | 通过 |
| JS gzip | **510.6KB**（xterm 被首屏打入，P7 必须 lazy） |
| 浏览器 smoke（mock API） | prompts、tasks、settings（外观/模型/Agent）；terminal 页面与 xterm 容器渲染（mock 下 WS 握手失败属预期） |

## 3. 待完成

- 快捷助手浮窗/固定分组（P5 剩余）。
- 任务编辑、Cron 预设/校验、任务会话弹窗。
- 设置：服务编辑/模型编辑/模型测试/诊断/服务启停。
- 终端：多标签、SSH 认证细节、终端字体 metrics、断线重连 UI。
- 终端：xterm + WebSocket + 服务器管理。
- 快捷助手浮窗（P5 剩余）。
- 提示词组件测试、编辑/删除 API 的浏览器 smoke。
- 路由级 lazy（P7，当前 510.6KB 超预算）。

## 4. 下一步

1. 设置中心（外观/模型/Agent）。
3. 快捷助手与 P5 剩余交互。
4. P7 全量回归、lazy、切换、回滚。
