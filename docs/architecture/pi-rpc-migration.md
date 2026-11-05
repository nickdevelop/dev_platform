# pi 后端对接方式重构计划：CLI → RPC

本文是 pi 后端从 CLI 模式（`pi --mode json`）迁移到 RPC 模式（`pi --mode rpc`）的设计与实施计划。迁移完成后，本文应更新为事实描述文档。

## 实施状态

RPC 传输、工具确认与收尾增强已实现并有测试覆盖（详见 `internal/providers/pi/` 下的 `rpc_*.go`、`permissions.go`，以及 `rpc_test.go`、`rpc_stub_test.go` 与 `testdata/rpc/session.jsonl` 回放用例）。迁移已完成并有两项后续反转：**CLI 模式已移除，pi 仅以 RPC 运行，`Mode` 选项/设置一并删除**；**平台内置审批扩展已移除**——pi core 没有内置工具审批，平台默认也不注入审批 gate，因此默认行为与 CLI 一致。仅保留 `extension_ui_request` `confirm` → `PermissionRequest` 的通用映射，operator 仍可通过 `PI_EXTENSIONS` 提供自己的扩展。已完成部分：

- Phase 0：test-only RPC stub、RPC 记录回放 fixture（`testdata/rpc/session.jsonl`：握手部分取自真实 pi 1.1.0 录制，单轮事件按协议形状补全）。最初的 `Mode` 选项与 Platform Settings per-backend `mode` 已随 CLI 模式一起删除。未包含“用真实 provider 录制的完整流式 fixture”（当前环境该 provider 无法完成回合）。
- Phase 1：`rpc_child.go` / `rpc_conn.go` / `rpc_commands.go` 传输内核、惰性启动、同 session 串行 + `followUp` 入队、有序停机、崩溃后同 session id 重启、空闲回收、`abort` 中断；`SendMessage`/`Interrupt`/`StopInstance` 直接走 RPC，CLI 路径已删除。
- Phase 2：`extension_ui_request` `confirm` → `agents.PermissionRequest`，`PermissionReplier`（`ListPermissions`/`ReplyPermission`），`once`/`reject`/`always` 映射与会话级 `always` 规则记忆，过期回复 `agents.ErrReplyExpired`；`Capabilities.ToolConfirmation` 固定为 `true`（表达协议支持）。平台不再安装审批扩展；默认会话不弹确认，与 CLI 一致。
- Phase 3：子进程存活时 `ListModels` 走 `get_available_models`（`--list-models` 兜底）、会话改名走 `set_session_name`、模型切换走 `set_model`；CLI 路径最终已移除。

对照本文的名词： `rpc_child.go` 对应“子进程生命周期”，`rpc_conn.go` 对应“严格 LF 分帧与按 id 关联”，`rpc_commands.go` 对应“命令构造与响应解析”。

参考资料：

- pi 协议文档：`rpc.md`、`rpc-commands.md`、`json.md`、`rpc-extension-ui.md`（随 pi 安装的 `docs/`）
- 现有适配器：`internal/providers/pi/`（RPC 模式实现）
- 平台契约：`internal/core/agents/contracts.go`、`internal/core/agents/extensions.go`
- 分层规则：`docs/architecture/backend-boundaries.md`

## 一、目标与非目标

### 目标

1. pi 后端从"每条 prompt 启动一次性 `pi --mode json` 进程"切换为"**每个活跃会话一个长驻 `pi --mode rpc` 子进程**（惰性启动、空闲回收）"。
2. 实现 `agents.PermissionReplier`（工具确认），补齐与 opencode 后端的能力差距（`Capabilities.ToolConfirmation` 从 `false` 翻转为 `true`）。
3. 中断改为 `abort` 命令（优雅取消），实例停止改为关闭 stdin（有序停机），进程组强杀仅作兜底。
4. 对上层完全透明：`AgentAdapter` / `HistoryReader` 契约不变，改动收敛在 `internal/providers/pi/` 包内。

### 非目标（留待后续迭代）

- steering / follow_up / compaction / thinking-level 等新契约方法。RPC 协议支持，但平台契约暂无对应接口，本计划不扩契约面。
- fork / clone / get_tree 等会话操作。
- opencode 侧不做任何改动。

## 二、现状盘点：组件去留

| 文件 / 逻辑 | 现状 | 处置 |
| --- | --- | --- |
| `adapter.go` `SendMessage` / `startProcess` | 每条 prompt 启动一次性子进程 | **重写**：确保会话子进程存活 → 发送 `prompt` 命令 |
| `adapter.go` instance / subscriber / publish | 逻辑句柄 + 事件扇出、背压 | **保留**（正是 RPC 需要的骨架），子进程按 sessionID 挂到 instance 上 |
| `event_parser.go` | 解析 stdout JSONL，假设首条记录是 session header | **基本复用**：pi 文档明确 RPC 发送相同的 session 事件形状，只是没有 session header；需去掉 header 依赖，新增 `response` 与 `extension_ui_request` 记录的分流解析 |
| `history.go` `ListHistory` | 读磁盘会话文件回放 | **保留不动**（与进程方式无关，instance-free） |
| `ListSessions` | 磁盘扫描（`--data-projects-demo--` 目录命名） | **保留不动**（RPC 模式 pi 照样写磁盘 JSONL 会话文件） |
| `models.go` `parseModelList` | 解析 `pi --list-models` 文本表格 | 保留为兜底；子进程存活时可升级为 `get_available_models`（Phase 3） |
| `process.go` / `process_unix.go` | 进程组启动、强杀、wait | **保留**，扩展 stdin 写入与有序停机 |
| `CreateSession`（只分配 UUID，不触碰 pi） | 元数据式会话创建 | **保留**：子进程惰性启动时带 `--session-id`，启动后 `get_state` 校验绑定 |
| `Interrupt`（杀该会话进程组） | 进程组强杀，回合结算为 aborted | **重写**：发送 `abort` 命令；子进程无响应超时后杀进程组兜底 |
| `StopInstance`（cancel context + kill） | 取消共享 context 强杀进程组 | **重写**：逐子进程关闭 stdin（有序停机）→ 有界等待 → 杀进程组兜底 |
| `descriptor.go` | tested 1.1.0，范围 `[1.0.0, 2.0.0)`，`ServiceSupported: false` | RPC 可用性在目标版本上重测后按实调整；`ServiceSupported` 仍为 false（无机器级服务） |
| `Diagnostics` | `LookPath` + `--version` 区间校验，不启动任何进程 | 保留；协议兼容性问题由 fixture 回归测试与运行时 `parse_error` 事件兜底 |

## 三、目标架构与关键设计决策

```
Instance（逻辑句柄，结构不变）
  └─ 按 sessionID 挂 RPC 子进程（懒启动、空闲回收、崩溃后同 id 重启）
       stdin  → JSONL 命令（prompt / abort / get_state / …），按 id 关联响应
       stdout → response 记录（走关联表）+ session 事件（复用 event_parser
                → NormalizedEvent → publish 扇出给订阅者）
       stderr → 诊断（沿用 maxStderrBytes 限流）
```

### 决策 1：每会话一个子进程，而非每 instance 一个

- pi 的 RPC 运行时同一时刻只有一个活跃 run（排队靠 steering/followUp 语义）；而平台的 instance 下多会话并发是既有行为（`procs` 按 session 维度挂靠、`Interrupt` 按 session 中断）。按会话拆子进程才能保住并发模型，且失败域退化为单会话。
- `switch_session` 以**会话文件路径**为参数，表明 pi 的 RPC 进程与会话是绑定的；不共享进程。
- 空闲回收（如 10 分钟内无活跃 run 且无订阅者即关闭 stdin）控制进程数上限。

### 决策 2：会话身份映射保持"平台分配 UUID"

- `CreateSession` 仍只分配 id（契约语义不变）；首次 `SendMessage` 懒启动子进程，启动参数：`--mode rpc --session-dir <dir> --session-id <uuid> [--model …] [--name …]`。
- 启动后发送 `get_state` 校验 `sessionId` 与 `sessionFile`，记录 sessionFile 供诊断。
- 崩溃重启策略：子进程意外退出 → 发布 `process_exit` 错误事件、标记死亡；该会话下次发送时**用同一 session-id 重新拉起**（transcript 在磁盘，对话不丢）。

### 决策 3：同会话串行发送（行为变化，需产品确认）

- 现状 CLI 模式允许同会话并行多条 prompt（各自独立进程）；RPC 下子进程一次只跑一个 run。
- 方案：子进程忙碌时（`get_state.isStreaming`）第二条 prompt 以 `streamingBehavior: "followUp"` 入队——这正是 RPC 的排队语义，事件流自然回放，无需平台自建队列。

### 决策 4：工具确认 = "传输 + 扩展 UI" 两件事

- 前提：pi 核心默认**不**在工具调用前请求批准（pi `docs/security.md` 明确）。RPC 只提供 extension UI 子协议通道：`extension_ui_request`（`confirm`/`select` 对话框，带 id 与可选 timeout）→ 客户端回 `extension_ui_response`。
- 因此确认请求只能来自扩展。平台**不再**内置审批扩展：默认会话不弹确认，与 CLI 一致；需要审批时由 operator 通过 `PI_EXTENSIONS` 提供。
- 决策映射：`once` → `confirmed: true`；`reject` → `confirmed: false`；`always` 是 yes/no 对话框没有的语义 → 平台侧按会话记忆审批规则（记录在 adapter 的 sessionState），后续工具命中规则直接放行。
- 过期回答返回 `agents.ErrReplyExpired`（HTTP 409 路径已存在，opencode 侧已验证）。
- `ListPermissions` 基于适配器侧 pending 表实现；pi 无原生列表命令。

### 决策 5：协议实现要点（来自 pi 文档的硬约束）

- 严格 LF 分帧：每条记录一个 JSON 对象以 `\n` 结尾；**不得**使用会把 `U+2028`/`U+2029` 当记录边界的通用行读取器（如 Node `readline`）。
- stdout 必须持续读取（pi 遵守 stdout 背压，客户端停止读取会卡住进程）；stdin 写入需遵守背压并加锁串行化。
- 命令与响应按可选 `id` 关联，命令处理是异步的，多命令并发时必须按 id 匹配而非顺序。
- `prompt` 的成功响应只表示"已接受/已排队/已处理"（`disposition: started|queued|handled`），完成以 `agent_settled` 事件为准；先订阅事件流再发 prompt，避免错过快速完成的回合。
- 有序停机：关闭子进程 stdin；仍需处理信号与意外退出。
- RPC 模式拒绝 `@file` 命令行参数；prompt 一律走 `prompt` 命令发送（本平台一直如此，不受影响）。

## 四、分阶段实施

### Phase 0 — 协议资产（约 0.5 天）

1. 用真实 pi 1.1.x 录制 RPC 会话 fixture（golden stdin/stdout 流），存入 `internal/providers/pi/testdata/rpc/`。
2. ~~`pi.Options` 增加 `Mode` 字段~~（已反转：CLI 路径与 `Mode` 均已删除，RPC 为唯一传输；回滚改为回退二进制/提交）。
3. 新增 test-only RPC stub 可执行文件（模拟 pi：脚本化响应/事件序列），供全部单测注入（沿用现有测试用 stub executable 的模式）。

### Phase 1 — RPC 传输内核（约 2–3 天，行为等价替换）

1. 新文件 `rpc_child.go`：子进程生命周期（启动参数、stdin 写入锁、空闲回收、崩溃标记与同 id 重启、有序停机）。
2. 新文件 `rpc_conn.go`：严格 LF JSONL 分帧、命令/响应按 id 关联（pending map + 超时）、双向背压。
3. 新文件 `rpc_commands.go`：命令构造与响应解析（`prompt` / `abort` / `get_state`）。
4. 改造 `event_parser.go`：容忍无 session header（子进程已知 sessionID）；新增 `response` 记录分流（不进事件流，走关联表）与 `extension_ui_request` 解析占位。
5. 重写 `SendMessage` / `Interrupt` / `StopInstance`（见第三节）。
6. **能力声明不变**（`ToolConfirmation` 仍为 `false`），保证 Phase 1 是纯传输层替换；现有 `adapter_test.go` / `interrupt_test.go` / `malformed_stream_test.go` 等改为对 RPC stub 跑通。

### Phase 2 — 工具确认（约 1–2 天）

1. 解析 `extension_ui_request`（`confirm` / `select`）→ 归一化为 `agents.PermissionRequest` 发布。
2. 实现 `PermissionReplier`：`ReplyPermission` → 回写 `extension_ui_response`（匹配 id）；`ListPermissions` 返回待决列表；过期 → `ErrReplyExpired`。
3. ~~平台侧配置审批扩展并为 pi 会话默认启用~~（已反转：平台不再安装审批扩展；`Capabilities.ToolConfirmation` 固定为 `true`，仅表达 extension UI 协议支持）。
4. 验收（仅针对 operator 提供的审批扩展）：confirm 能形成权限卡片，回复走现有 HTTP 回复端点，过期回复 409；默认无扩展时不应出现卡片。

### Phase 3 — 收尾与增强（约 1 天，可选）

1. 子进程存活时 `ListModels` 走 `get_available_models`（带缓存），保留 `--list-models` 兜底。
2. 会话改名走 `set_session_name`，替换现在每次发送携带 `--name` 的做法。
3. ~~dev 环境以 RPC 模式浸泡后切默认值~~（已完成：RPC 成为唯一模式，CLI 路径移除）。

## 五、风险与对策

| 风险 | 对策 |
| --- | --- |
| RPC 协议随 pi 版本漂移（协议面远大于 print 模式） | fixture 回归测试 + descriptor 版本区间硬校验；协议相关代码集中在 `rpc_commands.go` 一个文件便于对版 |
| 守护进程管理（孤儿、僵尸、泄漏） | 复用现有进程组强杀与 `WaitDelay`；有序停机（关 stdin）；空闲回收；崩溃即标记、下次懒重启 |
| 同会话并发语义变化（CLI 允许并行 prompt） | followUp 入队（更合理的排队语义）；Phase 1 动工前与产品确认 |
| extension UI 弹窗无客户端响应而阻塞 | pi 侧 timeout 自动解析默认值；平台侧对无订阅者的 confirm 默认 `rejected` |
| 子进程崩溃丢回合 | `process_exit` 错误事件 + 同 session-id 重启续聊；磁盘会话文件兜底 |
| 回滚 | 回退提交/二进制；`Mode` 开关已随 CLI 路径删除，不再作为回滚手段 |

## 六、验收标准

1. 现有全部单测在 RPC stub 下通过；新增分帧/命令关联/崩溃重启/空闲回收单测。
2. e2e：新建会话 → 流式回复 → 工具事件 → 中断（走 `abort`）→ 停止实例（有序停机）→ 会话发现与历史回放结果与历史 CLI 行为一致。
3. 工具确认：operator 扩展的 confirm 能形成卡片、回复生效、过期回复返回 409；无扩展时默认不弹。
4. 资源：空闲 10 分钟后无常驻 pi 进程残留；并发 5 会话稳定运行。
5. 文档同步：本文件改为事实描述；`docs/agent-compatibility.md` 更新 pi 行（能力矩阵 `ToolConfirmation: true`、协议说明）；`docs/architecture/backend-boundaries.md` 无需改动（异构层实现细节不出包）。
