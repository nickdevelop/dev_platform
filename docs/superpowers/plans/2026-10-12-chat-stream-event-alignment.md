# 对话事件流对齐(告警/空thinking/工具输出重复/64KiB超限)实施计划

**Goal:** 消除任务对话中四个已复现缺陷:新任务首发消息的 stderr 告警、交错 thinking 产生的空块、toolResult 被当聊天消息重复展示、pi 记录超 64KiB 被丢弃报错,并修复复现中暴露的三个连带缺陷(用户消息回显重复、空 assistant 气泡、卡死的 streaming 光标)。

**Architecture:** 后端只改 `internal/agents/pi` 的事件解析(记录上限、stderr 白名单、role=toolResult 丢弃、thinking_end 权威 content、空 message_end 不发事件);前端只改 `web/src/stores/sessions.ts` 的流式合并策略(回溯合并到仍在 streaming 的条目、孤立终止事件丢弃、回合结束结算)与用户回显去重(按待发回显文本而非相邻比较)。`MessageView.vue`、API 契约、事件类型均不变。

**Tech Stack:** Go 1.x(`go test ./...`)、Vue 3 + Pinia + vitest(`npx vitest run`)、Playwright e2e(`npm run test:e2e`)。

**Spec:** 无独立 spec;根因分析见本对话前文(证据:`/tmp/pitest/stream.jsonl`、`/tmp/pitest/stream2.jsonl` 为 pi 1.1.0 真实事件流;沙箱复现脚本 `/tmp/repro.mjs`)。

## Global Constraints

- 事件 JSON 契约(`internal/agents/events.go` 的 NormalizedEvent 与 `web/src/api/sessions.ts` 的镜像)不新增字段、不改语义。
- `MaxRawEventBytes = 64 * 1024`(事件携带 raw 的上限)不变;只放宽 stdout 读流上限。
- 历史回放路径(`internal/agents/pi/history.go`)不改:live 流向回放看齐。
- 所有仓库提交信息使用现有风格(`fix:` / `test:` / `feat:`)。
- Go 测试命令:`go test ./internal/agents/pi/... -run <Name> -v`,收尾 `go test ./...`;前端:`cd web && npx vitest run src/tests/<file>`。

## Review Focus

计划未逐一钉死、但最可能在真实输入上出问题的行为,执行时按行补测试:

1. 交错事件流(thinking_delta 夹在 text_delta 之间,pi 实测存在)→ Task 5 测试 (a) 必须用真实交错序列。
2. 超过新 8MiB 上限的记录仍要报错且不卡流 → Task 1 保留 overflow 测试(把构造线放大到新上限之上)。
3. echo 消费后同一文本再次出现(steering/重发)必须显示,不得被吞 → Task 6 测试 (c)。
4. 历史回放不受合并改动影响(重复的两段历史思考是两个条目)→ Task 5 跑现有回放测试并新增相邻两条 final thinking 的用例。
5. 回合以 `process_exit`/`execution_failed` 结束时,未闭合的 streaming 条目要结算,光标不能永远闪 → Task 5 测试 (d) 覆盖 lifecycle idle 与 error 两种入口。

---

### Task 1: pi stdout 读流上限提升到 8 MiB(问题 4)

**Files:**
- Modify: `internal/agents/pi/event_parser.go`(`maxLineBytes` 常量区、`run()`、`run()` 的 overflow 文案)
- Test: `internal/agents/pi/event_parser_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `const maxStreamRecordBytes = 8 * 1024 * 1024`(包内);`run()` 行为:≤8MiB 记录正常解析,>8MiB 记录丢弃并发 `line_too_large`(文案中的数字随之变化)。stderr 通道(`runStderr`)保持 64KiB 不变。

- [ ] **Step 1: 写失败测试** `TestParserKeepsLargeRecord`:一条 `text_delta` 记录,delta 用 `strings.Repeat("x", 200000)`(约 200KB > 旧 64KiB、< 8MiB),断言:无 `line_too_large` 事件、消息事件 Text 为该 200KB 字符串。
- [ ] **Step 2: 运行确认失败** `go test ./internal/agents/pi/ -run TestParserKeepsLargeRecord -v` → FAIL(出现 line_too_large)。
- [ ] **Step 3: 实现**:在 event_parser.go 增加 `maxStreamRecordBytes`(带注释:与 history.go 的 `maxHistoryRecordBytes` 对齐;pi 的 turn_end/agent_end 携带完整消息,实测 transcript 单条可达 106KB+);`run()` 改为 `scanLines(r, maxStreamRecordBytes, …)`,overflow 文案改用新常量。**确认点:** 现有 `TestParserMalformedAndPartialLines` 里构造超长行的用例要改到 8MiB 之上;若嫌测试过重,可把该行构造改为 `strings.Repeat("y", maxStreamRecordBytes+1)`(编译期常量拼接,内存可接受)。
- [ ] **Step 4: 运行** `go test ./internal/agents/pi/ -run 'TestParser' -v` → 全部 PASS。
- [ ] **Step 5: Commit** `fix: raise the pi stdout record bound to 8 MiB so full turn_end/agent_end records survive`

### Task 2: 抑制 pi 的良性 stderr 警告(问题 1)

**Files:**
- Modify: `internal/agents/pi/event_parser.go`(`runStderr` + 新 helper `isBenignStderr`)
- Test: `internal/agents/pi/event_parser_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `isBenignStderr(line string) bool`(包内):前缀匹配 `Warning: No project session found with id`(pi 1.1.0 `dist/main.js:349`,新建会话首发消息必然出现;pi 随后正常创建会话)。`runStderr` 对命中行**不发事件**,其余 stderr 行行为不变。

- [ ] **Step 1: 写失败测试** `TestParserStderrSuppressesBenignWarning`:输入两行 —— `Warning: No project session found with id 'uuid-x'; creating a new session with that id.` 与 `Warning: something else`;断言只产出 1 个 error 事件(code `stderr`,含 "something else")。
- [ ] **Step 2: 运行确认失败** → FAIL(产出 2 个事件)。
- [ ] **Step 3: 实现** `isBenignStderr`(TrimSpace 后 `strings.HasPrefix`),在 `runStderr` 的行回调里命中即 `return`。**确认点:** 用 `pi --session-id <新uuid> --mode json "hi"` 在沙箱目录跑一次,确认本机 pi 版本该警告的确切前缀;若前缀随版本变化,把匹配放宽为同时匹配 `No project session found with id` 子串(在测试中钉死所选匹配)。
- [ ] **Step 4: 运行** `go test ./internal/agents/pi/ -run TestParserStderr -v` → PASS。
- [ ] **Step 5: Commit** `fix: suppress pi's benign "no project session found" stderr warning`

### Task 3: 不再把 role=toolResult 的 message_start 发成聊天消息(问题 3)

**Files:**
- Modify: `internal/agents/pi/event_parser.go`(`normalize` 的 `message_start` 分支)
- Test: `internal/agents/pi/event_parser_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `message_start` 且 `message.role == "toolResult"` → 返回 nil(工具输出仅由 `tool_execution_end` 的 tool_result 事件呈现,与 history.go 回放一致)。`message_end` 已对非 assistant 返回 nil,不改。

- [ ] **Step 1: 写失败测试** `TestParserToolResultMessageIsNotChat`:输入 session 头 + `{"type":"message_start","message":{"role":"toolResult","content":[{"type":"text","text":"TOOL_OUTPUT_MARKER_42"}],"toolCallId":"call_1"}}`(取自 /tmp/pitest/stream2.jsonl 实录)+ assistant `message_end`;断言:不存在任何 `EventMessage`。
- [ ] **Step 2: 运行确认失败** → FAIL(存在 message 事件 "TOOL_OUTPUT_MARKER_42")。
- [ ] **Step 3: 实现**:在 `message_start` 分支把 `toolResult` 与 assistant 一样提前返回 nil。**确认点:** grep 前端/测试确认没有消费 `role: "toolResult"` 消息事件的地方(预期只有 MessageView 把非 user 当 agent 气泡渲染——正是要修的)。
- [ ] **Step 4: 运行** `go test ./internal/agents/pi/ -v` → PASS(pi 包全绿)。
- [ ] **Step 5: Commit** `fix: drop pi toolResult message_start records instead of rendering them as chat`

### Task 4: thinking_end 携带权威 content;空 assistant message_end 不发事件(问题 2 后端半边 + 空气泡)

**Files:**
- Modify: `internal/agents/pi/event_parser.go`(`piAssistantEvent` 增加 `Content json.RawMessage`;`parser` 增加 `thinkingStreamed bool`;`normalize` 的 `thinking_end` 与 `message_end` 分支)
- Test: `internal/agents/pi/event_parser_test.go`

**Interfaces:**
- Consumes: 现有 `contentText(raw)`(同时处理字符串与 block 数组;实测 `thinking_end.content` 为字符串,已确认见 /tmp/pitest/stream2.jsonl)。
- Produces:
  - `thinking_end`:若 `thinkingOpen`:已流出过 delta → 维持现状(空文本 final 终止事件,`TestParserStreamsThinking` 已钉死);未流出过 delta(非流式 provider)→ final thinking 事件,Text = `contentText(Content)`(空则仍发空终止)。未 open → nil(现状)。`thinking_delta` 置 `thinkingStreamed = true`;每次 `thinking_start`/消息边界重置。
  - `message_end`(assistant):`streamed == false 且 text == ""`(如纯 thinking+toolCall 消息)→ 不发 message 事件(仍先执行 closeThinking)。

- [ ] **Step 1: 写失败测试** 两个:
  - `TestParserThinkingEndCarriesContentWhenNotStreamed`:thinking_start → thinking_end(content="完整推理",无 delta);断言产出 1 个 final thinking 事件 Text=="完整推理"。
  - `TestParserEmptyAssistantMessageEndIsSilent`:assistant `message_end`,content 仅 `[{"type":"thinking",...},{"type":"toolCall",...}]`(无 text、无 delta);断言:无 message 事件。
- [ ] **Step 2: 运行确认失败** → 两个 FAIL。
- [ ] **Step 3: 实现**(如上)。
- [ ] **Step 4: 运行** `go test ./internal/agents/pi/ -v` → 全绿(特别确认 `TestParserStreamsThinking`、`TestParserClosesDanglingThinking`、`TestParserNonStreamedAssistantKeepsText` 未回归)。
- [ ] **Step 5: Commit** `fix: take thinking_end content as authoritative and silence empty assistant message_end`

### Task 5: 前端流式合并改为"回溯合并 + 孤立终止丢弃 + 回合结束结算"(问题 2 前端半边)

**Files:**
- Modify: `web/src/stores/sessions.ts`(`appendMessage`、`appendThinking`、`applyLifecycle`、error 结束码分支)
- Test: `web/src/tests/sessions-store.spec.ts`

**Interfaces:**
- Consumes: 现有 `ChatEntry`、`handleEvent`、`applyLifecycle`。
- Produces:
  - 新私有 `findOpenStreaming(list: ChatEntry[], kind: ChatEntryKind, role: string): ChatEntry | undefined`:自尾向前找**第一条 `kind`(+message 时同 `role`)且 `streaming === true`** 的条目。
  - `appendThinking(sessionId, text, final, sequence)` 新规则:① 回溯找到 open thinking → 追加 text、final 时置 `streaming=false`;② 未找到且 `final === false && text !== ""` → push 新 streaming 条目;③ 未找到且(text === "" 或 final)→ **不产生条目**(孤立终止/空块直接丢弃)。
  - `appendMessage` 同构:role 匹配;`final && text === ""` 且无 open → 丢弃;非 final 空 delta 丢弃;其余 push。**保留**现有 `dedupeEcho` 相邻检查供 Task 6 之前不回归(Task 6 会移除)。
  - 新私有 action `settleStreaming(sessionId)`:该 session 所有 `streaming=true` 条目置 false;在 `applyLifecycle` 的全部 idle 态(`execution_succeeded/interrupted/agent_settled/agent_end`)与 `handleEvent` 的 `execution_failed|process_exit` error 分支调用。

- [ ] **Step 1: 写失败测试**(sessions-store.spec.ts 新 describe):
  - (a) 真实交错序列:`thinking('甲',false)` → `message(assistant,'A',false)` → `thinking('乙',false)` → `message(assistant,'B',false)` → `thinking('',true)` → `message(assistant,'',true)`;断言 kinds == `['thinking','message']`,text == `'甲乙'` / `'AB'`,streaming 均 false。
  - (b) 孤立终止:`message(assistant,'答',true)` 后 `thinking('',true)` → kinds == `['message']`(无空 thinking)。
  - (c) 空非 final delta:`thinking('',false)`、`message(assistant,'',false)` → 0 条目。
  - (d1) idle 结算:streaming message 后 lifecycle `agent_settled` → `streaming === false`;(d2) error 结算:streaming message 后 error 事件 code `process_exit` → `streaming === false`。
  - (e) 回放不受影响:history 里两条 final thinking 相邻 → 两个 thinking 条目(新增用例)。
- [ ] **Step 2: 运行确认失败** `cd web && npx vitest run src/tests/sessions-store.spec.ts` → 新用例 FAIL,旧用例 PASS。
- [ ] **Step 3: 实现**(如上规则;`ChatEntryKind` 类型如需提炼就在 stores/sessions.ts 内导出别名)。
- [ ] **Step 4: 运行** `npx vitest run src/tests/sessions-store.spec.ts src/tests/message-view.spec.ts src/tests/task-chat-panel.spec.ts` → 全绿。若有旧用例钉死了"空 final thinking 生成空条目"的旧行为,更新该用例并在 commit message 里说明。**确认点:** 检查 `historyTailGuard`/`consumeHistoryGuard` 与空事件新规则的交互(守卫消耗时机不应改变)。
- [ ] **Step 5: Commit** `fix: merge streamed thinking/message across interleaved events and drop orphan terminators`

### Task 6: 用户回显去重改为"待回显文本"(消除消息重复)

**Files:**
- Modify: `web/src/stores/sessions.ts`(`state.echoPending`、`sendMessage`、`handleEvent` message 分支、`applyLifecycle`、`appendMessage` 移除 `dedupeEcho`)
- Test: `web/src/tests/sessions-store.spec.ts`

**Interfaces:**
- Consumes: Task 5 的 `settleStreaming`(同文件,勿冲突)。
- Produces: state 新字段 `echoPending: Record<string, string>`;`sendMessage` 在本地 push 后设 `echoPending[sessionId] = trimmed`;`handleEvent` 的 live message 分支:`role === 'user' && echoPending[sessionId] === text` → 删除该 pending 并 return;收到任何 assistant live message 或回合 idle 结算时清空该 session pending;`appendMessage` 删除 `dedupeEcho` 参数及相邻比较逻辑(loadHistory 的调用点随之简化,回放语义不变:注释保留)。

- [ ] **Step 1: 写失败测试**:
  - (a) `sendMessage` 后收到等文 user echo 事件 → 仅 1 条 user 条目;
  - (b) 复现序:sendMessage → 插入 error 事件 → 等文 user echo → 仍仅 1 条(钉死本 bug);
  - (c) pending 消费后,再收到一条等文 user 事件(模拟 steering)→ 第 2 条出现。
- [ ] **Step 2: 运行确认失败** → (b)(c) FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** `npx vitest run src/tests/sessions-store.spec.ts` → 全绿。**确认点:** 跑整个 vitest 套件 + `npm run test:e2e`(e2e 的 fake pi/opencode 桩会回显用户消息,确认去重不回归);若 e2e 桩不回显,记录事实即可。
- [ ] **Step 5: Commit** `fix: dedupe user echo by pending text instead of adjacent entry`

### Task 7: 全量验证与沙箱端到端复验

**Files:**
- 无新文件(复验脚本可临时使用 /tmp/repro.mjs,不入库;若希望保留,落到 `web/scripts/repro-chat.mjs` 由人工决定)

**Interfaces:**
- Consumes: Task 1–6 全部产出。
- Produces: 验证结论(写入最终提交说明或回复用户)。

- [ ] **Step 1:** `go test ./...` → 全绿。
- [ ] **Step 2:** `cd web && npx vitest run` → 全绿;`npm run test:e2e` → 全绿。
- [ ] **Step 3:** `./dev.sh restart --build` 重启正式实例;在沙箱(127.0.0.1:18099,data-dir /tmp/dpdata)重建环境:`make build` 后用新 `bin/platform` 重启沙箱,跑 `node web/scripts/…` 或 /tmp/repro.mjs(登录 test1234 → 项目 /tmp/pitest → 新建 pi 任务 → 发送"先思考再运行 echo TOOL_OUTPUT_MARKER_42")。
- [ ] **Step 4: 断言复验通过**(对照修复前复现输出):
  1. 首发消息后无 "Warning: No project session found" 错误行;
  2. live 条目里无 `thinking: ""`、无 `message: ""`、最终消息无卡死光标;
  3. 无独立于工具块的 `TOOL_OUTPUT_MARKER_42` 气泡;
  4. 用户消息仅一条;
  5. 刷新前后条目结构一致(除 separator)。
  - 若环境允许(可人工在真实项目跑一次大输出工具任务),确认不再出现 "pi record exceeds the … byte limit"。**确认点:** 若仍能复现出该错误,记录是哪类记录超限并回报,不要现场放宽。
- [ ] **Step 5:** 清理沙箱(`kill` 沙箱进程,删除 /tmp/dpdata 中注册的 /tmp/pitest 项目无需保留)。

---

## 执行时需要确认的不确定点汇总

1. Task 2:本机/部署机 pi 版本该警告的确切前缀(以实测为准选定匹配串并在测试钉死)。
2. Task 3:确认无其他消费者依赖 live 的 `role:"toolResult"` message 事件。
3. Task 5:既有前端测试中是否有钉死"空 thinking 条目"旧行为的用例(有则更新)。
4. Task 5/6:e2e 桩(fake pi / mock opencode)是否回显用户消息与是否发送交错 delta;若否,补桩内数据或在单测层钉死。
5. Task 7:8MiB 后仍超限时(>8MiB 单记录)保留报错可见,不静默——出现则带记录类型上报,不擅自再放宽。
