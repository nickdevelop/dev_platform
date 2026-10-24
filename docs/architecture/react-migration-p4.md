# React 迁移 P4 核心 Chat（基础版）进度

- 日期：2026-10-24
- 状态：核心会话可打开、回放历史、发送/停止、新建会话、模型选择、图片附件、文件预览、审批模式、活动面板、基础 slash 与 @ 文件提及已接入；富渲染待补
- 生产前端：仍是 `web/`（Vue）

## 1. 已完成

### 1.1 会话状态与 SSE

- `stores/sessions.ts` 从 Pinia 迁移为 Zustand：
  - 项目会话列表、历史回放、消息/思考增量合并、turn 状态、模型选择、错误
  - `openSession` 顺序：continue → history → subscribe
  - `sendMessage` 乐观用户消息 + echo 去重
  - `deleteSession` 清理消息/状态/模型
- `stores/interactions.ts` 迁移为 Zustand：权限/表单快照、事件折叠、回复。
- 执行层适配：
  - `StreamStore` 新增 `setStreamStatus` / `clearEchoPending` / `consumeHistoryTailGuard`
  - `EventBusClient` 不再直接改 store 字段，统一走 setter
  - `consumeFrame` 导出，便于单测驱动流事件

### 1.2 会话 UI

- `SessionChatPage`：
  - 真实路由参数、loadSessions + openSession
  - 标题、后端 Tag、连接状态、运行中标记
  - 历史截断提示、historyError 重试
  - 消息滚动跟随/跳转最新
  - 权限确认卡（允许一次/始终/拒绝）
  - 动态表单卡（string/number/boolean/multiselect/options）
- `MessageList`：
  - user/assistant 气泡
  - Markdown（marked + DOMPurify）assistant 渲染
  - thinking/tool/notice/error 折叠/提示
  - 图片附件、model/timestamp
- `ChatComposer`：
  - Enter 发送、Shift+Enter 换行、IME 保护
  - 发送/停止按钮、错误提示

### 1.3 修复

- `requireAuth` 读取 await 前 Zustand 快照导致嵌套路由重定向 `/` 的问题已修复（P3 记录）。
- Composer 不再因 SSE reconnecting 禁用，避免 mock/短暂断线无法发送。
- antd `App` 的包装 div 没有高度，导致会话页只有内容高度；已用 `#app > .ant-app { height:100% }` 修正，会话页恢复全高与独立滚动。

### 1.3 Composer 高级项

- `ChatComposer` 支持：
  - 工具条 slot（会话页放模型选择）
  - 图片选择/粘贴/校验/缩略图/移除，随 `sendMessage` 上传
  - `allowImages` 控制纯 Chat 是否启用
- 审批模式：默认审批 / 自动审批 / 托管；自动审批自动回复工具确认，托管额外按推荐答案提交表单；模式持久化到 `dev-platform-approval-mode`。
- 会话页模型列表来自 `/agent-backends/{backend}/models`，可清空回退后端默认。

### 1.4 新建会话与文件预览

- `NewSessionPage`：项目、Agent 后端、模型、提示词模板、补充内容；创建后跳转新会话。
- 会话页「文件」按钮打开右侧 `FileSidebar + FileViewer`，复用 P3 组件。
- 新建会话仍不包含图片附件/审批模式/发送快捷键设置。

## 2. 验证

| 项 | 结果 |
|---|---|
| typecheck | 通过 |
| lint | 0 warning / 0 error |
| 单测 | **8 文件 / 42 用例通过** |
| build | 通过 |
| JS gzip | **427.6KB**（P3 412.6KB） |
| 浏览器 smoke（mock API/SSE） | 历史消息渲染、Markdown、发送用户消息、无 console error |
| 新增 sessions 单测 | 增量合并、echo 去重、turn started/ended |

## 3. 已知缺口

- 新建会话未接草稿持久化、图片附件、审批模式。
- Composer 未接：模型选择器、审批模式、图片粘贴/拖拽、slash/@ 下拉、发送快捷键设置。
- 消息渲染是基础版：工具卡片/Diff/PlantUML/ToolOutput 尚未复用 P3 组件；活动面板未接。
- 会话页文件预览已接；活动面板（思考/工具/文件多 Tab）仍未接。
- SSE 真实后端 E2E 未跑；浏览器 smoke 用 mock EventSource 与历史接口。
- 包体 427.6KB 已超 400KB 目标，需路由级 lazy。

## 4. 下一步

1. 新建会话表单 + 模型选择 + 草稿。
2. Composer 高级能力（模型、审批模式、图片、slash/@）。
3. 工具/Diff/PlantUML 渲染与活动面板、文件预览接入。
4. 路由级 lazy，压回预算。
5. P5 纯 Chat/快捷助手，P6 任务/设置/终端。
