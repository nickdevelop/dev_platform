# 会话标题自动生成与同步（Plan A）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 任务标题不再泄漏平台内部 ID：新建任务在首条消息时自动回填"首条用户消息截断"标题并随下次 `--name` 落盘到 pi；同步（Discover）时以 pi 显式名优先、首条用户消息预览兜底、绝不覆盖已有标题。

**Architecture:** 三层改动——`internal/agents` 增加共享的标题派生函数和 `SessionRef.Preview` 字段；pi adapter 在 `readSessionRef` 里从会话 JSONL 提取首条用户消息作 Preview、在 `SendMessage` 里把平台标题透传给 `--name`；sessions service 在 `Send` 回填空标题、在 `upsertRef`/`newSessionRow` 实现标题优先级；前端把 `title || id` 兜底改为 `title || '未命名任务'`。

**Tech Stack:** Go（仅标准库）、Vue 3 + Vitest。

**Spec:** 决策记录（本计划即其实现，要点如下）：
1. pi 核心**不会**自动生成会话名（`session_info` 仅由 `/name`、`--name`、扩展 API 写入），故 auto-title 由平台侧生成（方案 A：首条用户消息截断，零模型成本）。
2. 标题优先级：pi 显式名（`session_info`）> 平台已有非空标题 > pi 首条用户消息预览（`Preview`）> 空（UI 显示"未命名任务"）。
3. `Preview` 只在创建索引行或行标题为空时填入，永不覆盖非空标题。
4. 平台标题经 `SendMessage` 的 `--name` 回写 pi 会话文件（复用 `main.js` 既有行为，使 pi CLI 与平台同步收敛）。

## Global Constraints

- Go 代码仅用标准库；遵循仓库现有注释风格（解释"为什么"）。
- 派生标题为单行：所有空白折叠为单空格，按 **rune** 截断至 60，超长追加 `…`。
- 平台内部 session ID（32 位 hex）从任何 UI 展示位消失。
- UI 空标题兜底文案固定为 `未命名任务`。
- 每个 Task 结束时全量跑该包测试再 commit。

## Review Focus

- 会话文件没有任何用户消息 / 首条用户消息是图片或空文本：Preview 必须为空，UI 落到"未命名任务"，不得显示 ID。
- 中文（多字节）标题：截断按 rune 而非字节，不产生乱码。
- Discover 同步：Preview 兜底**绝不**覆盖已存在的非空平台标题（会被反复同步冲掉）。
- pi 端 `/name` 改名后同步：显式 `session_info` 名仍要覆盖平台旧标题（现有规则不能被优先级改动破坏）。
- 前端搜索过滤路径（ProjectTree 的 filter）与列表/树展示一致，也不泄漏 ID。

---

### Task 1: agents.DeriveTitle 共享标题派生函数

**Files:**
- Create: `internal/agents/title.go`
- Test: `internal/agents/title_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `func agents.DeriveTitle(text string) string` — 折叠空白（含换行/制表）为单空格，按 rune 截断至 60 并追加 `…`；输入全空白或空返回 `""`。Task 2/4 依赖此签名。

- [ ] **Step 1: 写失败测试** `internal/agents/title_test.go`，用例：
  ```go
  func TestDeriveTitle(t *testing.T) {
      // 多行折叠
      // got := DeriveTitle("fix the login\n\nbug\tplease") → "fix the login bug please"
      // 空与全空白: DeriveTitle(""), DeriveTitle(" \n\t ") → ""
      // CJK 按 rune 截断: 100 个 "问" → 前 60 个 "问" + "…", 用 utf8.RuneCountInString == 61 断言
      // 恰好 60 rune 不截断不加省略号
  }
  ```

- [ ] **Step 2: 运行确认失败** — `go test ./internal/agents/ -run TestDeriveTitle -v`，预期编译失败（undefined: DeriveTitle）。

- [ ] **Step 3: 实现** `internal/agents/title.go`：常量 `maxTitleRunes = 60`；`strings.Join(strings.Fields(text), " ")` 折叠；`utf8.RuneCountInString` 判断后 `[]rune` 截断 + `"…"`。注释说明：pi 不写自动名（session_info 仅显式命名产生），平台必须自己派生标题。

- [ ] **Step 4: 运行确认通过** — `go test ./internal/agents/ -run TestDeriveTitle -v` → PASS。

- [ ] **Step 5: Commit** — `git add internal/agents/title.go internal/agents/title_test.go && git commit -m "feat(agents): add DeriveTitle for platform-derived session titles"`

---

### Task 2: SessionRef.Preview 字段 + pi readSessionRef 提取首条用户消息

**Files:**
- Modify: `internal/agents/contracts.go`（`SessionRef` struct，约 214–224 行）
- Modify: `internal/agents/pi/adapter.go`（`readSessionRef`，约 928–966 行）
- Test: `internal/agents/pi/adapter_test.go`（新增 `TestReadSessionRefPreview`）

**Interfaces:**
- Consumes: `agents.DeriveTitle`（Task 1）。
- Produces: `agents.SessionRef` 新增 `Preview string \`json:"preview,omitempty"\``（注释：agent 侧降级预览文本——首条用户消息；不是显式命名，不得覆盖显式标题）。`readSessionRef` 返回值填充该字段。`SessionRef.Validate` 不变（Preview 可选）。

- [ ] **Step 1: 写失败测试** `TestReadSessionRefPreview`（直接单测 `readSessionRef`，同包，用 `t.TempDir()` 写 JSONL；header 的 cwd 用 expectedDirectory 原值）：
  - header + `{"type":"message",...,"message":{"role":"user","content":"部署日志系统"}}`（content 为字符串）→ `Title=="" && Preview=="部署日志系统"`。
  - content 为块数组 `[{\"type\":\"text\",\"text\":\"修复 登录\\n\\n问题\"}]`（同包已有 `decodeHistoryContent` 可复用解析）→ Preview 折叠空白后等于 `"修复 登录 问题"`。
  - 首条 user 消息文本为空、次条有文本 → Preview 取次条。
  - role 为 assistant/system 的消息不产生 Preview。
  - 存在 `session_info` 时 `Title` 与 `Preview` 各自独立填充（Title 仍为 name）。
  - 无任何用户消息 → Preview 为 `""`。

- [ ] **Step 2: 运行确认失败** — `go test ./internal/agents/pi/ -run TestReadSessionRefPreview -v` → FAIL（字段不存在/Preview 空）。

- [ ] **Step 3: 实现。** contracts.go 的 `SessionRef` 加 `Preview` 字段。`readSessionRef` 的扫描循环里，entry struct 增加 `Message *struct{ Role string; Content json.RawMessage }`；对 `type=="message"` 且 `Role=="user"` 且 Preview 尚未填充的行，用 `decodeHistoryContent` 提取文本、`agents.DeriveTitle` 归一化，非空则赋给 `ref.Preview`。保持 4MB 扫描上限与现有 session_info 逻辑不动。同步更新 `ListSessions` 上方注释（发现结果现在携带预览）。

- [ ] **Step 4: 运行确认通过** — `go test ./internal/agents/pi/ -v` 全绿（含既有 `TestAdapterListSessions` 不破）。

- [ ] **Step 5: Commit** — `git commit -am "feat(pi): surface first user message as session ref preview"`

---

### Task 3: pi SendMessage 用平台标题透传 --name（回写 pi 会话文件）

**Files:**
- Modify: `internal/agents/pi/adapter.go`（`SendMessage` 内 name 决策，约 468–475 行）
- Test: `internal/agents/pi/adapter_test.go`

**Interfaces:**
- Consumes: 无新增。
- Produces: `SendMessage` 的 `--name` 取值优先级：`ref.Title`（非空时，且写回 `sessionState.title`）> `sessionState.title`。现有 `promptArgs`/`promptOptions` 不改。

- [ ] **Step 1: 写失败测试** `TestAdapterSendMessageUsesRefTitleAsName`：参照 `TestAdapterSafeArgsAndWorkingDirectory` 的 fake pi 记录 argv 的方式——`CreateSession` 时不带 Title，随后 `SendMessage(agents.SessionRef{...,Title: "部署日志系统"}, ...)`，断言 argv 含 `["--name", "部署日志系统"]`；且第二次 `SendMessage` 用空 `ref.Title` 时仍带该 name（状态已更新）。

- [ ] **Step 2: 运行确认失败** — `go test ./internal/agents/pi/ -run TestAdapterSendMessageUsesRefTitleAsName -v` → FAIL。

- [ ] **Step 3: 实现：** 在 `SendMessage` 中，`ref.Title != ""` 时以其为 name，并在持有 `a.mu` 时把 `state.title` 更新为 `ref.Title`（state 存在时），注释说明"平台侧新生成的标题随下一次启动写进 pi 会话文件（pi 对 --name 会 append session_info）"。

- [ ] **Step 4: 运行确认通过** — `go test ./internal/agents/pi/ -v` 全绿。

- [ ] **Step 5: Commit** — `git commit -am "feat(pi): send platform session title back to pi via --name"`

---

### Task 4: sessions service 标题回填与同步优先级

**Files:**
- Modify: `internal/sessions/service.go`（`Send` ≈584–625；`newSessionRow` ≈983–1000；`upsertRef` ≈953–978）
- Test: `internal/sessions/service_test.go`

**Interfaces:**
- Consumes: `agents.DeriveTitle`（Task 1）、`SessionRef.Preview`（Task 2）、Task 3 的 ref.Title→--name 行为。
- Produces: 行为变更，无新导出签名。

- [ ] **Step 1: 写失败测试**（沿用 service_test 现有 stub adapter 基建）：
  - `TestSendBackfillsEmptyTitleFromFirstMessage`：索引行 Title 为空，`Send(..., text: "帮我\n部署日志系统", ...)` → 返回的 Session.Title == `"帮我 部署日志系统"`，且 stub adapter 收到的 `ref.Title` 等于该值（Task 3 借此回写 pi）。
  - `TestSendKeepsExistingTitle`：行 Title 为 `"已有标题"` → Send 后不变，传给 adapter 的 `ref.Title=="已有标题"`。
  - `TestSendDoesNotBackfillOnBlankDerivation`：text 为 `"  \n "` 时 Send 本就报 `ErrInvalidRequest`（已有守卫），补一例 text 有效但 DeriveTitle 结果非空的确认即可，不新增分支。
  - `TestDiscoverPreviewFillsOnlyEmptyTitles`：stub adapter `ListSessions` 返回 `agents.SessionRef{SessionID:"n1", Preview:"预览", Title:""}`——新行 Title=="预览"；已存在行 Title=="保留" 时同步后仍为 "保留"；已存在行 Title=="" 时同步后变 "预览"。
  - `TestDiscoverExplicitTitleStillWins`：ref.Title=="显式名"、行 Title=="旧名" → 同步后 "显式名"（保护现有覆盖规则）。
  - `TestCreateWithoutTitleLeavesTitleEmpty`：Create 传空 title → 索引行与返回 DTO 的 Title 均为 `""`（不得写入 ID）。

- [ ] **Step 2: 运行确认失败** — `go test ./internal/sessions/ -run 'TestSendBackfills|TestDiscoverPreview|TestDiscoverExplicit|TestCreateWithoutTitle' -v` → FAIL。

- [ ] **Step 3: 实现：**
  - `Send`：通过全部前置校验后、构造 `ref` 之前，`if row.Title == "" { row.Title = agents.DeriveTitle(text) }`（注释：pi 无自动命名，平台以首条用户消息派生；行更新走既有的 `s.repo.update(row)`）。
  - `newSessionRow`：`Title` 取 `ref.Title`，为空则取 `ref.Preview`。
  - `upsertRef`：保留显式 Title 覆盖分支；其 `else` 路径补 `if existing.Title == "" && ref.Preview != ""` 时填 Preview 并 update。注释写明优先级：显式名 > 已有标题 > 预览。

- [ ] **Step 4: 运行确认通过** — `go test ./internal/sessions/ -v` 全绿。

- [ ] **Step 5: Commit** — `git commit -am "feat(sessions): backfill titles from first message and preview, never from platform ids"`

---

### Task 5: 前端空标题兜底改为"未命名任务"

**Files:**
- Modify: `web/src/components/SessionList.vue:45`
- Modify: `web/src/components/ProjectTree.vue:53,151`
- Test: `web/src/tests/workbench.spec.ts`、`web/src/tests/project-tree.spec.ts`

**Interfaces:**
- Consumes: 无。
- Produces: 两个组件对空标题渲染 `未命名任务`；ProjectTree 的搜索匹配串同样使用兜底文案。

- [ ] **Step 1: 写失败测试**：在 project-tree.spec.ts（及涉及列表断言的 workbench.spec.ts 用例）各加一例——`session({ id: 's-x', title: '' })` 渲染文本包含 `未命名任务` 且 **不包含** `s-x`；搜索框输入 `未命名` 能命中该条。

- [ ] **Step 2: 运行确认失败** — `cd web && npx vitest run` → 新用例 FAIL。

- [ ] **Step 3: 实现：** SessionList.vue 把 `{{ session.title || session.id }}` 改为 `{{ session.title || '未命名任务' }}`；ProjectTree.vue 的 `{{ entry.title || entry.id }}` 与 filter 的 `(entry.title || entry.id)` 同改为兜底文案（可在组件内提一个 `const UNTITLED = '未命名任务'`）。`grep -rn "|| session.id\|entry.id }}" web/src` 确认无其他 ID 泄漏点。

- [ ] **Step 4: 运行确认通过** — `cd web && npx vitest run` 全绿。

- [ ] **Step 5: Commit** — `git add web/src && git commit -m "fix(web): show untitled placeholder instead of internal session id"`

---

### Task 6: 全量回归

**Files:** 无新改（如暴露问题则修复后一并计入前序 task）。

- [ ] Step 1: `go vet ./... && go test ./...` 全绿。
- [ ] Step 2: `cd web && npx vitest run` 全绿；`e2e/` 若有会话标题相关断言则同步更新。
- [ ] Step 3: 手工冒烟（可选，有环境时）：新建不填标题的任务 → 发一条消息 → 刷新列表应显示消息摘要而非"未命名任务"；pi 会话文件应出现 `session_info` 条目。
