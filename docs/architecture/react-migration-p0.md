# React 迁移 P0 技术验证报告

- 日期：2026-10-24
- 基线提交：`d78dfbc fix(assistant): 漂浮按钮改为纯图标并支持全屏拖拽`
- 范围：仅 P0（基线固化 + 技术验证），**不包含业务迁移、不切换生产前端**
- 生产前端仍是 `web/`；`web-next/` 为验证脚手架，未纳入 `Makefile`、`release.sh`、CI 与 `web/dist` 发布路径

## 1. 结论

P0 通过。React 19.3.1 + Ant Design 6.6.7 + React Router 8.5 + Zustand 5 + CSS Modules + Vite 8 的组合在当前项目上可构建、可测试、可主题化，未出现阻塞性能力缺口。进入 P1 前需要处理两项工程问题：

1. **包体**：验证页单 chunk 约 **373KB gzip**，高于旧版初始 JS 的 **230KB gzip**；P1 必须用路由级懒加载与更细的 antd 使用面控制预算。
2. **主题桥**：antd 深色算法会派生主色，需用 CSS 变量把主色钉回平台 palette；这是一处有意保留的桥接点，必须在 P1 扩展到 hover/active/border/fill 等 token 并做组件级验收。

## 2. 迁移基线（本次实测）

| 指标 | 基线 |
|---|---|
| 旧前端单测 | 68 个文件 / **638 用例通过**（21.38s） |
| 旧前端类型检查 | `tsc` 通过，但不覆盖 `.vue` 模板 |
| 旧前端构建 | 初始 JS **230KB gzip**、CSS **61KB gzip**；`dist` 约 11MB（含 5MB 字体与 PlantUML chunk） |
| 旧前端规模 | 66 个 `.vue`，脚本 7,805 行、模板 4,651 行、scoped CSS 9,390 行；14 个 Pinia store |
| 旧前端测试钩子 | P0 为关键选择器补充 `data-test`，旧测试仍全部通过 |
| API | OpenAPI 79 个 path；协议不变 |

P0 期间为旧 Vue 补充的稳定测试钩子：

- `project-row` / `session-row`（ProjectTree）
- `project-tree-scroll` / `workspace-pane`（AppShell）
- `message-input`（SessionChatPanel）
- `first-message-input`（NewSessionPanel）
- `terminal-tab`（TerminalTabItem）
- `file-tree-row`（FileTree）
- `tool-output` 原本已有

## 3. 验证环境与脚手架

| 项 | 版本 |
|---|---|
| React / React DOM | 19.3.1 |
| antd / @ant-design/icons | 6.6.7 / 6.3.4 |
| react-router | 8.5.0 |
| zustand / immer | 5.0.16 / 11.2.9 |
| Vite / @vitejs/plugin-react | 8.3.5 / 6.1.2 |
| TypeScript | 7.0.2 |
| Vitest / jsdom | 5.0.5 / 30.1.3 |
| Testing Library | React 16.3.3、user-event 14.6.8、jest-dom 7.0.1 |
| Node | 22.23.2（react-router 要求 ≥22.22） |
| Lint | oxlint 1.89.0（react-hooks、jsx-a11y） |

`web-next/` 当前包含：

- `src/app/`：`createBrowserRouter`、鉴权 loader、错误边界、`ConfigProvider` + `AntApp` + 中文 locale。
- `src/theme/`：从旧版复制的 4 主题/3 密度 preset、`applyAppearance`、antd token 映射。
- `src/stores/`：Zustand appearance/auth 小样。
- `src/spike/`：antd 表单/表格/弹窗/树/Tabs/Splitter、StrictMode 单例探针。
- `src/tests/`：主题、spike、StrictMode 测试。
- `vite.config.ts`：复用旧版的 `/api` 代理、`ws:true`、SSE `flushHeaders`、woff2 不内联、`dist` 布局。

## 4. 验证结果

| 验证项 | 结果 |
|---|---|
| `npm run typecheck` | 通过（`.ts` + `.tsx` 全量） |
| `npm run test:run` | **3 文件 / 8 用例通过** |
| `npm run lint` | **0 warnings / 0 errors** |
| `npm run build` | 通过；Vite 8.3.5，1582 modules |
| 产物体积 | JS 1,166.65 kB / **约 373.1 kB gzip**；静态 CSS 4.51 kB / 1.26 kB gzip；dist 1.2MB |
| 浏览器 smoke | 登录守卫 → `/spike`；弹窗焦点进入输入框；Esc 关闭；无 console error |
| 主题矩阵 | 4 主题 × 3 密度 = **12 组合**；正文/输入对比度最低 **11.21**；无横向溢出 |
| StrictMode | 模拟 window 级单例：累计连接 **1**、活跃监听 **1**（setup→cleanup→setup 未重复建连） |

### 4.1 主题矩阵明细

| 主题 | body 背景 | 卡片背景 | 输入背景 | 主按钮 | 正文对比度 |
|---|---|---|---|---|---|
| light | `#f4f5f7` | `#ffffff` | `#ffffff` | `#2563eb` | 14.48 |
| dark | `#0f1115` | `#171a21` | `#171a21` | `#4f8cff` | 15.44 |
| warm | `#f6f1e7` | `#fffaf0` | `#fffaf0` | `#b45309` | 11.21 |
| cool | `#eef1f5` | `#f8fafc` | `#f8fafc` | `#0f766e` | 12.91 |

- 深色必须使用 `darkAlgorithm`：若改用 `defaultAlgorithm`，`Alert` 等状态背景会回到浅色，出现白底浅字的严重问题。
- `darkAlgorithm` 会把主色派生成 `#467adc`（与 palette 的 `#4f8cff` 不同）。P0 用 `#app .platform` 覆盖 `--ant-color-primary/-hover/-active`，使实际组件使用平台主色；`theme.useToken().colorPrimary` 仍返回派生值，因此**不要把 `useToken` 的主色当作展示色**，展示/自定义 CSS 继续使用 `--color-*`。
- 这是 P0 唯一保留的 antd 内部 CSS 变量桥接；P1 需要审计 `border/bg/fill/active` 等派生 token，并锁定 antd 版本。

### 4.2 组件覆盖验证

已实际渲染并通过测试/截图检查：

- `Form` + `Input` + `Select` + `Switch`：必填校验、错误文案、提交反馈。
- `Table`：列定义、状态 Tag、空/数据行。
- `Tree`：默认展开、选中。
- `Tabs`：内容切换。
- `Splitter`：左右分栏、尺寸回调。
- `Modal`：焦点进入第一个输入框、Esc 关闭、取消关闭。
- `Dropdown`、`Alert`、`Card`、`Segmented`、`Slider`、`Tooltip`（按钮/菜单场景）。
- React Router：公共 `/login` 与受保护 `/spike`，loader redirect。

未纳入 P0、留到 P1/P3 的：真实 API/CSRF、真实 SSE、xterm、PlantUML WASM、Diff、Composer 的 slash/@、文件预览、复杂标签/项目树。

## 5. 发现与处理

| 发现 | 影响 | P0 处理 |
|---|---|---|
| antd v6 `Alert` 的 `message` 已弃用 | 控制台 warning | 改用 `title` |
| antd Modal 在 jsdom 中不会完成 leave motion，`role=dialog` 会留在 DOM | 单测若断言“移除”会误报 | 单测断言关闭状态；真实浏览器 smoke 断言 `isVisible=false` |
| antd 深色算法派生主色 | 与平台 palette 不一致 | CSS 变量桥接主色；记录限制 |
| antd 使用 `.platform` 作用域 CSS 变量 | 不能用普通根变量覆盖 | 用 `#app .platform` 提高优先级；P1 需评估是否长期保留 |
| CSS-in-JS 样式运行时注入，静态 CSS 只有 1.26KB | 不能只看静态 CSS 估算样式成本 | 以总 JS 包体 + 运行时样式评估 |
| 单 chunk 373KB gzip | 高于旧版 230KB | P1 路由级 lazy + 控制 antd 组件面；重新设预算 |
| `typescript-eslint@8` peer 不支持 TS 7 | ESLint 方案受阻 | 改用 oxlint（react-hooks/jsx-a11y 可用）；`tsc` 仍是硬门禁 |
| react-router 8.5 要求 Node ≥22.22 | CI 可能用旧 Node 22 | CI/engines 显式锁定 ≥22.22 |
| P0 未复制 5MB 字体 | 字体 metrics 未验证 | P1 接管 `web/src/fonts`，P8 终端字体测量回归 |

## 6. P1 入口条件

1. 确认包体策略：路由级懒加载、antd 使用面、是否引入 `@ant-design/icons` 按需导入；初始 gzip 目标在 P1 重新设定（建议 ≤ 400KB，并在 P1 结束前复测）。
2. 确认主题桥长期方案：保留 `#app .platform` 主色覆盖，扩展组件 token 审计，或改为自定义 token 生成；不允许同时存在两套可编辑主题源。
3. 接入真实 `api/client.ts`、会话/CSRF store 与 401 跳转；P0 的 fake auth 必须替换。
4. 接入真实 SSE 单例（EventBusClient）到 Zustand，先补 `setStreamStatus` 等 setter，禁止外部直接改 state。
5. 把 `web-next` 纳入独立 CI job；在切换前**不要**并入 `release.sh`/默认 `web/dist`。
6. 继续给旧 Vue 测试补 `data-test` 并把 E2E 选择器从 class 迁到 data-test；P0 只做了第一批。

## 7. 复现命令

```bash
cd web-next
npm ci
npm run typecheck
npm run test:run
npm run lint
npm run build
npm run preview -- --host 127.0.0.1 --port 15174
```

浏览器 smoke 需要本机 Chromium；P0 使用 `web/node_modules/playwright` + 已安装的 Chromium 执行，未把 Playwright 加入 `web-next` 依赖。

## 8. 明确未做

- 未迁移任何业务模块。
- 未接入真实后端、CSRF、SSE、终端、PlantUML。
- 未修改 `Makefile`、`release.sh`、CI；仅在 `PROJECT.md` 增加 P0 脚手架说明，生产前端描述不变。
- 未把 `web-next` 纳入发布包。
