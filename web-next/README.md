# dev-platform-web-next（P0）

React 迁移的 P0 技术验证脚手架，**不参与生产构建/发布**。生产前端仍是 `../web`。

## 运行

```bash
npm ci
npm run typecheck
npm run test:run
npm run lint
npm run build
npm run preview -- --host 127.0.0.1 --port 15174
```

开发：

```bash
npm run dev
```

`vite.config.ts` 与生产前端保持同一 `/api` 代理约定：`ws: true`，SSE 响应在 `proxyRes` 后立即 `flushHeaders()`；构建输出 `dist/`，字体不内联。

## P0 验证范围

- React 19.3.1 + antd 6.6.7 + react-router 8.5 + Zustand 5
- `ConfigProvider` 主题桥：4 主题（light/dark/warm/cool）× 3 密度（compact/standard/relaxed）× 字号
- antd Form/Table/Tree/Tabs/Splitter/Modal/Dropdown/Alert
- React Router 鉴权 loader 与 `/login` → `/spike`
- StrictMode 下单例连接不重复创建
- RTL + jsdom + oxlint + tsc

## 关键限制

- `src/theme/presets.ts` 与 `src/theme/apply.ts` 是旧版 `web/src/theme` 的 P0 副本；切换时以新应用为准，不要双向修改。
- antd 深色算法会派生主色，`src/styles/base.css` 用 `#app .platform` 把主色钉回平台 palette；升级 antd 时必须复测。
- P0 未接入真实 API/CSRF/SSE/终端/PlantUML；`src/stores/auth.ts` 是 fake。
- 未复制 5MB 字体，P0 使用系统 fallback；P1 接管真实字体。

详细结果见 [`docs/architecture/react-migration-p0.md`](../docs/architecture/react-migration-p0.md)。
