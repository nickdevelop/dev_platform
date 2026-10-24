# React 迁移 P5 纯 Chat（基础版）进度

- 日期：2026-10-24
- 状态：纯 Chat 与快捷助手基础版完成；分组移动/重命名/删除、图片已接；regenerate/编辑消息待补
- 生产前端：仍是 `web/`（Vue）

## 1. 已完成

### 1.1 纯 Chat 状态

- `stores/chats.ts` 从 Pinia 迁移为 Zustand：
  - 分组/对话列表、active 对话、消息日志、running/streamStatus/truncated
  - 打开、新建并发送、发送、停止、删除、重命名、切换模型
  - 快照/delta/message_update/conversation/turn/closed 回调
- `features/chat/pure/useChatStream.ts` 增加 `setStreamStatus` 接口，避免直接改 store 字段。
- 复用 `chatEntries` 的 entries/upsert/delta/settle 逻辑。

### 1.2 快捷助手

- `QuickAssistant`：右下角入口 + 480×600 浮窗，基础版固定定位（未做拖拽）。
- 打开时读取 `systemKey=quick-assistant` 固定分组、最近对话与可用模型。
- 无对话时首条消息创建；有对话时发送；独立 EventSource 处理 snapshot/delta/message_update/turn。
- 复用 MessageList/ChatComposer/chatEntries。

### 1.3 纯 Chat UI

- 新增 `/chat`、`/chat/:chatId` 与 `PureChatPage`：
  - 左侧分组/对话列表，新建/分组创建
  - 右侧标题、模型选择、删除、消息列表、Composer
  - 模型列表来自 `/model-services`，过滤 enabled/usable/supportsText
  - 新建对话首条消息走 `createAndSend`，随后切到 `/chat/:id`
- 复用 P4 的 `MessageList` 与 `ChatComposer`。
- AppShell 在 `/chat` 下隐藏项目侧栏，避免双栏重复。

## 2. 验证

| 项 | 结果 |
|---|---|
| typecheck | 通过 |
| lint | 0 warning / 0 error |
| 单测 | **8 文件 / 42 用例通过** |
| build | 通过 |
| JS gzip | **431.6KB** |
| 浏览器 smoke（mock API/SSE） | `/chat/1` 打开、历史消息渲染、Composer 可用、无 console error |
| 修复 | Zustand selector 每次返回新数组导致 React error #185；改为稳定 `EMPTY_ENTRIES` |

## 3. 已知缺口

- 纯 Chat regenerate/编辑消息未接；图片已随发送上传。
- 快捷助手未做拖拽、位置持久化、固定分组锁定校验。
- 分组重命名/删除、对话移动到分组、重命名对话、图片附件、重新生成/编辑消息未接 UI。
- 发送后新消息依赖 SSE；浏览器 smoke 只验证了请求/Composer，未模拟 delta 回显。
- 模型切换只更新当前对话或 draft，不刷新所有会话。
- 自动标题、快捷助手固定分组规则未接。
- 包体 431.6KB，需 lazy。

## 4. 下一步

1. 快捷助手浮窗 + 固定分组。
2. 分组/移动/重命名/图片/regenerate。
3. P6 任务/提示词/设置/终端。
4. 路由级 lazy，P7 全量 E2E。
