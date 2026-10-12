# 内置字体（移除外观设置中的字体项）设计

日期：2026-10-12
状态：已实现
关联：`docs/superpowers/specs/2026-10-11-platform-settings-design.md`（本文替换其"字体"一节）

## 意图与理由

外观设置里的"字体"下拉（4 套预设 + 自定义 Google Fonts）把选字体的责任推给了使用者，
而自定义项依赖 `fonts.googleapis.com`：内网或离线部署时字体永远加载不上，界面观感随
网络状况漂移。改为平台自带字体后，无论是否有外网，界面在任何一次部署里都是同一套字形，
少一个需要解释的设置项，也少一个运行时外部依赖。

选定的组合：**Google Sans Code**（等宽、含连字的可变字重编程字体）承担中文以外的全部文
字，**Noto Sans SC**（中文可变字重）承担中文。整站呈等宽观感，代码、路径、时间与中文同
屏时基线稳定。

## 目标

- 平台不再请求任何外部字体源；字体文件随构建产物一起从自身来源（`/assets`）分发。
- 中文用 Noto Sans SC，其余用 Google Sans Code；由 CSS 字体回退机制自然分工，无需按脚本
  分类的规则。
- 外观设置只剩主题配色、字号、界面密度；API 与文档同步收窄。

## 非目标

- 不提供字体选择，也不提供"自定义 Google Fonts"。将来若要恢复，是重新设计，不是在旧字段
  上打补丁。
- 不做字体子集自产（不自建 pyftsubset 流水线）：直接用 Google Fonts 已切好的 unicode-range
  切片。
- 不改字号（12–20 px）与主题、密度的既有行为。

## 约束

- 字体是二进制资产，需要进仓库；仓库因此增加约 5 MB（123 个 woff2 切片）。
- 生产环境由 Go 服务托管构建产物，字体走 `/assets` 的不可变缓存路径。

## 方案

### 字体来源与打包

`web/scripts/fetch-fonts.mjs` 是**手动运行**的取源工具（不是构建步骤）：它带着桌面 UA 抓取
Google Fonts 的 CSS，下载全部 woff2 切片，把 CSS 里的远程 URL 改写为本地路径，输出

- `web/src/fonts/fonts.css`（生成文件，文件头注明不可手改），
- `web/src/fonts/files/*.woff2`（123 个切片；文件名 = 家族 slug + URL 哈希，因此重跑只会覆盖
  真正变化的切片），
- `web/src/fonts/MANIFEST.json`（切片 → 原始 Google Fonts URL 的映射，便于日后核对版本与来源）。

两个家族各取的区间：Google Sans Code `ital,wght@0,300..800;1,300..800`（22 片，约 320 KB），
Noto Sans SC `wght@100..900`（101 片，约 4.6 MB）。保留 `unicode-range` 切片是整个方案的前提：
浏览器只下载页面文字真正命中的切片。实测（Chromium，中英混排一段文字）4 个切片、182 KB。

`web/src/style.css` 顶部 `@import './fonts/fonts.css'`，字体因此进入 Vite 构建，输出到
`web/dist/assets/` 带 hash 的文件名，落到 Go 侧 `public, max-age=31536000, immutable` 的缓存策略上。
`vite.config.ts` 的 `assetsInlineLimit` 显式不内联 woff2：小切片转成 data URL 会把 123 个文件
压进一次 CSS 下载，并把切片级缓存的意义抹掉。

### 字体栈

`--font-family: 'Google Sans Code', 'Noto Sans SC', sans-serif`（`style.css` 的 `:root`）。
顺序即机制：浏览器逐字符取第一个含该字形的家族，因此拉丁、数字、ASCII 标点、代码走 Google
Sans Code，中文与中文标点走 Noto Sans SC。原先散落在组件里的等宽栈（消息代码块、工具输出、
后端实例目录、资源同步路径、冲突目标）统一改用同一个变量，等宽来源只剩一处。

字体族不再由配置决定，所以 `theme/apply.ts` 不再设置 `--font-family`，也不再注入字体
`<link>`；`style.css` 是唯一持有者，首屏与 JS 前后一致。

### 数据与服务端

`settings.Appearance` 去掉 `font`、`customFont` 两个字段，校验与 `DefaultAppearance` 同步收窄，
`docs/api/openapi.yaml` 的 `Appearance` 同步。无需数据迁移：读取是"在默认值上叠加解码"，旧
JSON 里多出的 `font`、`customFont` 被 `encoding/json` 忽略；写入不再包含它们。handler 用的是
非严格绑定，所以升级前加载的页面继续 PUT 这两个键仍得到 200，响应中不再有它们。

### 生产分发的媒体类型

Go 内置表把 `.woff2` 映射为 `font/woff`。`internal/http` 的 `init` 用
`mime.AddExtensionType(".woff2", "font/woff2")` 纠正——媒体类型不对是浏览器可能拒绝字体的那类
问题，且这个映射属于"以后一定会有人踩"的地面。

## 测试

- `web/src/tests/fonts.spec.ts`：`style.css` 引入内置字体样式表；字体栈中 Google Sans Code 在前、
  Noto Sans SC 紧随；两个家族都有 `@font-face`；切片均带 `unicode-range`；CSS 声明的每个文件都存在
  且不含任何远程 URL；`main.ts` 与 `apply.ts` 不出现 fonts.googleapis.com。
- `web/src/tests/theme.spec.ts`：应用外观不再设置 `--font-family`、不新增任何 `<link>`。
- `web/src/tests/appearance-settings.spec.ts`：分组只剩主题配色 / 字号 / 界面密度，无字体控件；
  保存字号时的请求体只有 `theme`、`fontSize`、`density`。
- `internal/settings/{service,handlers}_test.go`：默认值不含字体字段；存储的 JSON 不再包含字体键；
  含字体键的旧数据能正常读取；PUT 带字体键被忽略且响应中不含它们。
- `internal/http/static_test.go`：`/assets/*.woff2` 以 `font/woff2` 提供。

## 人的伙伴的决定

- 中文用 Noto Sans SC，其余用 Google Sans Code；接受整站等宽观感。
- 移除外观设置中的字体项，字体内置且不可配置。
- 保留字号设置。

## 我的判断（可否推翻）

- 镜像 Google Fonts 的 unicode-range 切片，而不是自产子集或塞一个整包 CJK 字体：覆盖全、
  按需下载，代价是仓库多约 5 MB 二进制。
- 两个家族都取可变字重（Google Sans Code 含斜体），一次 vendoring 覆盖 100–900 全区间。
- 顺带统一组件里硬编码的等宽栈、修正 `.woff2` 媒体类型、为测试引入 `@types/node`。
