# React 迁移 P3 文件预览进度

- 日期：2026-10-24
- 状态：P3 完成（独立文件浏览路由；P4 复用为 Chat 右侧预览）
- 生产前端：仍是 `web/`（Vue）

## 1. 已完成

### 1.1 文件树

- `FileSidebar` 基于 antd `Tree`：
  - 目录懒加载 `loadData`
  - 文件/目录图标、Git 状态 Tag
  - 「所有文件 / 变更」Segmented
  - 搜索走 `searchProjectFiles`，150ms 防抖
  - `file-tree-row` / `file-sidebar` data-test
- 使用 `data-test` 而非 class 作为稳定选择器。

### 1.2 文件查看

- `FileViewer`：
  - Code：按行号渲染，保留 `truncated` 提示
  - Markdown：`renderMarkdown`（marked + DOMPurify）后注入
  - Image：`getFileRawUrl` 原图
  - Binary：明确提示不可文本预览
  - 加载/错误/重试/空状态
- `FilesPage`：`/projects/:projectId/files`，左侧树 + 右侧预览。
- 项目设置页新增「浏览文件」入口。

### 1.3 关键修复

`requireAuth` 之前捕获的是 await 之前的 Zustand 快照：

```ts
const auth = useAuthStore.getState()
await auth.ensureSession()
if (!auth.authenticated) throw redirect('/login') // stale
```

结果：直接访问任意嵌套路由先跳 `/login`，登录页 loader 又因 store 已认证跳到 `/`。  
现改为 await 后重新 `useAuthStore.getState().authenticated`。P2 的嵌套路由（设置/新建会话）一并恢复。

## 2. 验证

| 项 | 结果 |
|---|---|
| typecheck | 通过 |
| lint | 0 warning / 0 error |
| 单测 | 7 文件 / 39 用例通过 |
| build | 通过；JS 412.6KB gzip，静态 CSS 2.99KB gzip |
| 浏览器 smoke（mock API） | `/projects/1/files` 直接进入；文件树加载、Markdown 渲染、无 console error |
| 嵌套路由回归 | settings / sessions/new 直接访问均保持 URL，Tab 正确登记 |

## 3. 已知缺口

- FileViewer 暂未实现多文件 Tab 条（P4 预览面板会统一处理）。
- Markdown 相对图片路径、内部链接跳转未做（P4/P6 补）。
- 代码无语法高亮/搜索（保持轻量，后续按需）。
- 文件树变更列表与文件树状态未联动刷新。
- 真实后端 E2E 尚未运行。

## 4. P4 入口

1. 完整 sessions store + EventBus/SSE，实现真实会话页。
2. Composer、MessageLog、审批/表单、活动面板。
3. 把 `FileSidebar/FileViewer` 接入会话右侧预览。
4. 加入路由级 lazy，压回 400KB gzip 以内。
