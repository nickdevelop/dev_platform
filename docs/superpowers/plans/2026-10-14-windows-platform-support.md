# Windows 原生支持与 OS 抽象层 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 平台支持 Linux、WSL2（Linux 构建）与 Windows 原生三种运行方式。操作系统相关代码收敛到 `internal/platform` 端口层，业务包不再 import `syscall`、`golang.org/x/sys` 或判断 `runtime.GOOS`。

**Architecture:** Go/Gin/GORM 后端 + Vue 3/TS/Pinia 前端。OS 层分为端口（`internal/platform`）、`unix` 实现（Linux 与 WSL2 共用）、`windows` 实现与测试替身（`fake`）。opencode 的 npm 安装布局属于后端知识，放在 `internal/agents/opencode`，通过平台的通用原语完成查找。

**Tech Stack:** Go 1.24（`go test ./...`、`go vet ./...`）、`golang.org/x/sys/windows`（新增直接依赖）、Gin、GORM/SQLite、Vue 3、Vitest（`cd web && npm test -- --run`）、Playwright（`cd web && npm run test:e2e`）。

**Design:** `docs/architecture/os-abstraction.md`（本计划的依据，编号"第 N 节"均指该文档）。

## 已确认的前提

| 项 | 结论 |
| --- | --- |
| 运行环境 | Linux、WSL2（运行 Linux 构建，共用 `unix` 实现）、Windows 原生。WSL1 启动时报错退出 |
| 回收站 | Windows 不支持，返回 `ErrTrashUnavailable`，走既有“永久删除 + 二次确认” |
| 权限 | Windows 接受降级：不做 ACL 收紧，`Perms.Enforced()` 为 false，并告警 |
| 服务注册文件 | `%USERPROFILE%\.local\state\opencode\service.json`，规则与 Linux 相同（先看 `XDG_STATE_HOME`） |
| opencode 安装 | npm 全局安装 `@opencode/cli`。直接定位 `<prefix>\node_modules\@opencode\cli\bin\opencode.exe`，不解析 `opencode.cmd` 文本 |
| 服务进程 | 注册文件中的 `pid` 是 opencode 服务进程本身，停止时按 pid 结束进程树 |
| CI | 仓库没有 CI 配置，阶段 0 新建 `.github/workflows/ci.yml` |
| Windows 运行方式 | 直接在命令行前台启动，不注册系统服务，不使用 NSSM 或任务计划程序。停止方式为 Ctrl+C 或关闭终端窗口 |
| 版本范围 | 只支持 opencode 2.x（`@opencode/cli`），不支持 1.x（`opencode-ai`） |

## Global Constraints

- **阶段 0 至 3 不得改变 Linux 与 WSL2 的可观察行为。** 每个任务结束时 `go test ./...` 在 Linux 上全绿。
- 业务包（`internal/sessions`、`internal/projects`、`internal/settings`、`internal/http`、`internal/database`、`internal/secrets`、`internal/resourcesync`、`internal/auth`、`internal/modelconfig`、`internal/agents/instances` 与两个适配器的协议代码）不得 import `syscall`、`golang.org/x/sys/*`，不得出现 `runtime.GOOS` 判断。该约束由阶段 0 的守卫测试执行。
- 平台差异只能通过 `internal/platform` 的接口或 build tag 文件表达。能力差异用 `Capabilities` 或 `Perms.Enforced()` 等布尔值表达，不用 `GOOS` 分支。
- 用户可见文案不出现“实例”二字（沿用既有决策）。新增的 Windows 相关文案需要同时给出中文说明。
- 不新增数据库迁移。不改变 opencode service 的启停语义：业务路径仍不启动、不停止、不重启 service。
- 每个任务结尾提交一次。提交前对应测试与 `go vet ./...` 必须干净。提交信息沿用 `refactor(platform): ...`、`feat(platform): ...`、`test(platform): ...`、`docs: ...` 风格。
- 守卫测试采用**允许清单**：清单只能缩短，不能新增条目。新增违规会直接失败。

## Review Focus

一个没看过这份计划的维护者，最容易在这些地方出错：

1. **`unix` 与 `linux` 的 build tag 不同。** 现有 `trash_linux.go` 使用 `//go:build linux`，迁移后 `unix` 包中 freedesktop 实现仍只能在 Linux 编译，其他 unix 系统必须返回 `ErrTrashUnavailable`。
2. **opencode 的 npm 查找逻辑不得放进 `internal/platform`。** 它属于 `internal/agents/opencode`，平台层只提供 `Executables.Resolve`、`IsExecutable`、`Dirs` 等通用原语。
3. **注册文件中的 `pid` 是服务进程，不是 shim 进程。** Windows 停止服务时必须结束进程树，但不能误杀平台自己的子进程。
4. **Windows 上 `Perms` 的模式位调用仍然保留**，只是不生效。测试必须按 `Perms.Enforced()` 跳过权限断言，而不是按 `GOOS` 跳过。
5. **pi 后端在 Windows 上的可执行解析尚未核实。** 它也是 npm 安装的 CLI，可能有相同的 shim 问题。阶段 4 开始前必须单独确认，见风险表。
6. **平台关停在 Windows 上没有 `SIGTERM`。** 必须保证 opencode service 不受影响，pi 子进程仍被回收，且关停不会因 opencode 实例已不在内存中而报错。

---

## Phase 0：基线与守卫

### Task 0.1: 记录基线

**Files:**
- Create: `docs/superpowers/plans/2026-10-14-windows-platform-support.md` 末尾的“基线记录”小节（本计划文件）

- [x] **Step 1: 记录 Linux 基线** — 运行 `go test ./... 2>&1 | tail -20`、`go vet ./...`、`cd web && npm run typecheck && npm test -- --run 2>&1 | tail -5`，把通过数量与时间写入本文件末尾。
- [x] **Step 2: 记录 Windows 交叉编译现状** — 运行 `GOOS=windows go vet ./... 2>&1 | head -50`，把失败清单写入本文件末尾。已知至少包括 `internal/agents/opencode/discovery.go:116`（`syscall.Kill` 未定义）。
- [ ] **Step 3: 提交** — `git commit -am "docs: record windows platform baseline"`

### Task 0.2: OS 依赖守卫测试（允许清单）

**Files:**
- Create: `internal/platform/guard_test.go`

**Interfaces:**
- Consumes: `go/parser`、`go/token`、`path/filepath`（标准库）
- Produces: 一个测试 `TestBusinessPackagesStayOSNeutral`。扫描 `internal/` 下除 `internal/platform/**`、`cmd/**`、`_test.go` 之外的非测试文件，断言不 import `syscall`、`golang.org/x/sys/*`，且不出现 `runtime.GOOS` 或 `runtime.GOARCH`。
- 允许清单 `allowlist` 为文件路径到 `reason` 的映射，初始内容为当前全部违规文件，逐条标注任务编号（例如 `internal/agents/opencode/discovery.go // Task 2.3`）。

- [x] **Step 1: 写测试与初始清单** — 测试失败时列出不在清单中的违规文件。
- [x] **Step 2: 确认当前通过** — `go test ./internal/platform/ -run TestBusinessPackagesStayOSNeutral -v` → PASS（清单覆盖全部现有违规）。
- [ ] **Step 3: 提交** — `git commit -am "test(platform): guard business packages against OS-specific imports"`

### Task 0.3: CI 增加 Windows 编译检查

**Files:**
- Create: `.github/workflows/ci.yml`（仓库当前没有 CI 配置）

**Interfaces:**
- Produces: `windows-latest` 任务，执行 `go build ./...`。阶段 0 结束前允许失败，但必须在 PR 中标注为“已知失败，阶段 4 修复”。

- [x] **Step 1: 增加工作流** — 只做 `go build ./...` 与 `go vet ./...`，不跑测试。
- [x] **Step 2: 确认失败项与 Task 0.1 清单一致**。
- [ ] **Step 3: 提交** — `git commit -am "ci: add windows build job (expected to fail until phase 4)"`

---

## Phase 1：端口层骨架（不改变行为）

### Task 1.1: 定义端口接口与聚合

**Files:**
- Create: `internal/platform/platform.go`
- Create: `internal/platform/new_unix.go`（`//go:build unix`）
- Create: `internal/platform/new_windows.go`（`//go:build windows`，返回未实现的占位，仅保证编译）
- Create: `internal/platform/platform_test.go`

**Interfaces:**
- Produces（与 `docs/architecture/os-abstraction.md` 第 3 节一致）：
  - `type Platform interface { Name() string; Dirs() Dirs; Instance() Instance; Process() Process; Executables() Executables; Trash() Trash; Perms() Perms; Shutdown() Shutdown; Terminal() Terminal; Environment() Environment }`
  - `Dirs`、`Instance`、`Process`、`Executables`、`Trash`、`Perms`（含 `Enforced() bool`）、`Shutdown`、`Terminal`、`Environment`
  - `func New() Platform`（按 build tag 选择实现）
  - `var ErrTrashUnavailable`：保留在 `internal/projects` 不动，由 `platform.Trash` 的实现引用同一错误值的副本时注意避免循环依赖，见 Task 2.5。

- [x] **Step 1: 写接口与 `New()`** — `unix` 版本暂返回 nil 实现占位，`windows` 版本返回占位。`Name()` 返回 `runtime.GOOS`，仅用于诊断。
- [x] **Step 2: 编译检查** — `go build ./...` 与 `GOOS=windows go build ./internal/platform/...` 通过。
- [ ] **Step 3: 提交** — `git commit -am "feat(platform): define OS port interfaces"`

### Task 1.2: 测试替身

**Files:**
- Create: `internal/platform/fake/fake.go`

**Interfaces:**
- Produces: `fake.New(opts...) *Platform`，可注入：目录映射、`Alive` 结果表、`Resolve` 结果表、`Trash` 错误、`Perms.Enforced` 值、`Terminal` 输入。所有端口都有确定性行为。

- [x] **Step 1: 实现 fake** — 每个端口一个 struct，零值即“不做任何事、返回 nil”。
- [x] **Step 2: 写 fake 的最小测试** — 验证注入值被原样返回。
- [ ] **Step 3: 提交** — `git commit -am "test(platform): add fake platform for unit tests"`

---

## Phase 2：把 Linux 实现迁入端口（不改变行为）

每个任务完成后，对应守卫清单条目删除。

### Task 2.1: 单实例锁

**Files:**
- Create: `internal/platform/unix/instance.go`（`//go:build unix`）
- Modify: `internal/runtime/single_instance.go`（改为调用 `platform.New().Instance()`）
- Delete: `internal/runtime/single_instance_unix.go`、`internal/runtime/single_instance_other.go`
- Modify: `internal/runtime/single_instance_test.go`

**Interfaces:**
- Consumes: `platform.Instance`
- Produces: `unix.Instance.Acquire(dataDir) (Lock, error)`，内部为 `flock(LOCK_EX|LOCK_NB)`。`errLockUnsupported` 随 `single_instance_other.go` 删除。

- [x] **Step 1: 迁移** — 保持文件名、锁文件路径、权限模式（0700 目录、0600 锁文件）不变。
- [x] **Step 2: 测试** — 现有 `single_instance_test.go` 全绿；`goruntime.GOOS == "windows"` 分支改为按 `Perms.Enforced()` 跳过权限断言（为阶段 3 做准备）。
- [ ] **Step 3: 提交** — `git commit -am "refactor(platform): move single-instance lock behind Instance port"`

### Task 2.2: 子进程组与终止

**Files:**
- Create: `internal/platform/unix/process.go`
- Modify: `internal/agents/pi/process.go:59`（`exec.CommandContext` 之后调用 `Process().Configure(cmd)`）
- Modify: `internal/agents/pi/process_unix.go`、`internal/agents/pi/process_other.go`（删除，逻辑移入平台）
- Modify: `internal/agents/opencode/process_unix.go`、`internal/agents/opencode/process_other.go`（删除）
- Modify: `internal/agents/opencode/sidecar.go`、`internal/agents/opencode/adapter.go`（`cmd.SysProcAttr` 与 kill 调用改为端口）
- Modify: `internal/agents/opencode/service_test.go:194`（`Setpgid` 改为 `Process().Configure`）

**Interfaces:**
- Produces: `Process.Configure(cmd) error`（unix：`Setpgid: true`）、`Process.Terminate(pid) error`（unix：`kill(-pid, SIGKILL)`，ESRCH 视为成功）。
- Consumes: 无。

- [x] **Step 1: 迁移 pi 与 opencode 的 kill 逻辑** — 语义与现有 `killProcessGroup` 完全一致，包括 ESRCH 处理。
- [x] **Step 2: 测试** — `go test ./internal/agents/... -v` 全绿。
- [ ] **Step 3: 提交** — `git commit -am "refactor(platform): process group configuration and termination behind Process port"`

### Task 2.3: 进程存活与僵尸判断

**Files:**
- Modify: `internal/platform/unix/process.go`（补充 `Alive`、`IsZombie`，`IsZombie` 读取 `/proc/<pid>/stat`）
- Modify: `internal/agents/opencode/discovery.go:111-121`（`processAlive` 改为调用 `Process().Alive`，删除 `syscall` import）
- Modify: `internal/agents/opencode/process_unix.go:29,58`（删除 `processIsZombie`，改为端口）
- Modify: `internal/agents/pi/interrupt_test.go:107-108`、`internal/agents/pi/adapter_test.go:318-319,600`（`syscall.Kill(pid, 0)` 改为 `Process().Alive`）
- Modify: `internal/agents/opencode/process_unix_test.go`（`/proc` 读取逻辑随实现移入平台，测试迁移到 `internal/platform/unix/process_test.go`）

**Interfaces:**
- Produces: `Process.Alive(pid int) bool`、`Process.IsZombie(pid int) bool`。

- [x] **Step 1: 迁移** — `discovery.go` 不再 import `syscall`，它的“注册文件读取 + 校验”逻辑保持不变。
- [x] **Step 2: 测试** — `discovery_test.go`、`service_gate_test.go` 全绿。
- [ ] **Step 3: 提交** — `git commit -am "refactor(platform): liveness and zombie checks behind Process port"`

### Task 2.4: 目录解析

**Files:**
- Create: `internal/platform/unix/dirs.go`
- Modify: `cmd/platform/main.go:68`（`os.UserConfigDir` 改为 `Dirs.ConfigDir()`）
- Modify: `internal/agents/opencode/discovery.go:35-45`（`defaultServiceFile` 改为 `Dirs.StateDir()` 拼接 `opencode/service.json`）
- Modify: `internal/agents/pi/adapter.go:1389`（`os.UserHomeDir` 改为 `Dirs.HomeDir()`，`defaultSessionDir` 随之迁移）
- Modify: `cmd/platform/config_test.go:71-85`（XDG 语义测试保留，`runtime.GOOS != "linux"` 跳过条件改为 `Dirs` 能力判断）

**Interfaces:**
- Produces: `Dirs.HomeDir()`、`Dirs.ConfigDir()`、`Dirs.DataDir()`、`Dirs.StateDir()`、`Dirs.TempDir()`。unix：`$XDG_CONFIG_HOME` 或 `~/.config`，`$XDG_DATA_HOME` 或 `~/.local/share`，`$XDG_STATE_HOME` 或 `~/.local/state`。

- [x] **Step 1: 迁移** — 默认值与 Linux 行为逐字相同。
- [x] **Step 2: 测试** — `go test ./cmd/... ./internal/agents/... -v` 全绿。
- [ ] **Step 3: 提交** — `git commit -am "refactor(platform): directory resolution behind Dirs port"`

### Task 2.5: 回收站

**Files:**
- Create: `internal/platform/unix/trash_linux.go`（`//go:build linux`，内容来自 `internal/projects/trash_linux.go`）
- Create: `internal/platform/unix/trash_other.go`（`//go:build unix && !linux`，返回不可用）
- Modify: `internal/projects/service.go:111`（`systemTrash{}` 改为 `platform.New().Trash()`）
- Delete: `internal/projects/trash_linux.go`、`internal/projects/trash_fallback.go`
- Move: `internal/projects/trash_linux_test.go` → `internal/platform/unix/trash_linux_test.go`

**Interfaces:**
- Produces: `Trash.Move(path) error`。`ErrTrashUnavailable` 的归属：定义在 `internal/platform`，`internal/projects` 通过 `platform.ErrTrashUnavailable` 引用，`handlers.go:53` 的 `errors.Is` 判断保持不变。
- 注意：`selectTrashRoot`、`escapeTrashPath` 等纯函数不依赖 OS，迁入 `internal/platform/policy/trash.go`，使两个平台都能测试。

- [x] **Step 1: 迁移** — 纯逻辑迁入 `policy`，系统调用留在 `linux` 文件。
- [x] **Step 2: 测试** — `go test ./internal/projects/... ./internal/platform/... -v` 全绿。
- [ ] **Step 3: 提交** — `git commit -am "refactor(platform): trash behind Trash port"`

### Task 2.6: 路径规范化

**Files:**
- Create: `internal/platform/unix/pathpolicy.go`
- Modify: `internal/paths/paths.go`（保留对外的 `Resolve`，内部改为调用 `Dirs.CanonicalizeUserPath`；`translateWindowsPath` 迁入 unix 实现）
- Modify: `internal/paths/paths_test.go`（保持全部现有用例）

**Interfaces:**
- Produces: `Dirs.CanonicalizeUserPath(p) (string, error)`、`Dirs.SamePath(a, b) bool`。unix：保留 `C:\work` → `/mnt/c/work` 翻译与 UNC 拒绝，`SamePath` 区分大小写。

- [x] **Step 1: 迁移** — 所有现有用例保持通过。
- [x] **Step 2: 提交** — `git commit -am "refactor(platform): path canonicalization behind Dirs port"`

### Task 2.7: 关停信号与测试辅助

**Files:**
- Create: `internal/platform/unix/shutdown.go`
- Modify: `cmd/platform/main.go:388`（`signal.NotifyContext` 使用 `Shutdown().Signals()`）
- Modify: `cmd/platform/main_test.go:123`（`syscall.SIGTERM` 改为 `Shutdown().Interrupt(p.cmd.Process)` 辅助，unix 实现发送 SIGTERM）
- Modify: `cmd/platform/main_test.go:29`（删除 `syscall` import）

**Interfaces:**
- Produces: `Shutdown.Signals() []os.Signal`、`Shutdown.Interrupt(p *os.Process) error`。

- [x] **Step 1: 迁移** — `TestServerShutsDownGracefullyOnSigterm` 名称保留，语义不变。
- [x] **Step 2: 测试** — `go test ./cmd/platform/... -v` 全绿。
- [ ] **Step 3: 提交** — `git commit -am "refactor(platform): shutdown signals behind Shutdown port"`

### Task 2.8: 清空守卫清单（阶段 2 出口）

- [x] **Step 1: 删除允许清单中已迁移的条目**，剩余条目只允许是阶段 3 与阶段 4 任务。（清单已为空：7 条初始条目在 2.1–2.7 中逐条消除，无剩余。）
- [x] **Step 2: 全量验证** — `go test ./... && go vet ./...`、`cd web && npm run typecheck && npm test -- --run`。（23 包全绿、vet 干净、前端 30 文件/338 用例全绿、Windows build/vet 干净，均与基线一致。）
- [ ] **Step 3: 提交** — `git commit -am "test(platform): shrink OS guard allowlist after phase 2"`

---

## Phase 3：修正平台无关的错误假设（不改变 Linux 行为）

### Task 3.1: 可执行判断与查找

**Files:**
- Modify: `internal/platform/unix/executables.go`（新建）：`Resolve` 调用 `exec.LookPath`，`IsExecutable` 检查 `0o111`
- Modify: `internal/settings/handlers_agents.go:217-227`（`isExecutable` 改为 `Executables().IsExecutable`，删除函数）
- Modify: `internal/settings/agents.go:211`（`exec.LookPath` 改为 `Executables().Resolve`）
- Modify: `internal/agents/pi/adapter.go:215`、`internal/agents/opencode/adapter.go:216`（`exec.LookPath` 改为 `Executables().Resolve`）
- Modify: `internal/settings/agents_test.go:64`（补充：通过 fake 注入查找结果）

**Interfaces:**
- Produces: `Executables.Resolve(name) (string, error)`、`Executables.IsExecutable(path) bool`。

- [x] **Step 1: 迁移** — Linux 行为不变。
- [x] **Step 2: 测试** — 使用 `fake.Platform` 覆盖“手动路径优先，其次 PATH”的分支。
- [ ] **Step 3: 提交** — `git commit -am "refactor(platform): executable lookup and checks behind Executables port"`

### Task 3.2: 私有权限（Perms）

**Files:**
- Create: `internal/platform/unix/perms.go`
- Modify: `internal/database/database.go:81,105,113,120,128`（`MkdirAll 0o700`、`Chmod 0o600` 改为 `Perms`；`bestEffortChmod` 随之迁移）
- Modify: `internal/secrets/store.go:31,34,197`（`FileMode`、`dirMode` 常量保留给 unix；写入改为 `Perms.PrivateFile`）
- Modify: `internal/runtime/single_instance.go:45,50,55`（改为 `Perms`）
- Modify: `internal/resourcesync/backup.go:21,28,73`（改为 `Perms`）
- Modify: `internal/database/database_test.go:330-352`、`internal/runtime/single_instance_test.go:77-100`、`internal/secrets/store_test.go:101-131`（权限断言改为 `if !perms.Enforced() { t.Skip(...) }`）

**Interfaces:**
- Produces: `Perms.PrivateDir(path) error`、`Perms.PrivateFile(path) error`、`Perms.Enforced() bool`。unix：行为与原调用逐字相同，`Enforced()` 为 true。

- [x] **Step 1: 迁移** — 业务代码不再直接写模式位。
- [x] **Step 2: 测试** — 现有权限断言在 Linux 上仍执行。
- [ ] **Step 3: 提交** — `git commit -am "refactor(platform): private permissions behind Perms port"`

### Task 3.3: 换行符处理

**Files:**
- Modify: `internal/agents/pi/models.go:15`（`strings.Split(output, "\n")` 之后对每行 `TrimSuffix("\r")`，与 `event_parser.go:146`、`opencode/client.go:676` 的处理一致）
- Modify: `internal/resourcesync/adapters/project_prompts.go:64`（`HasSuffix(content, "\n")` 的判断先统一换行符，避免 CRLF 检出的仓库每次判脏）

- [x] **Step 1: 写测试** — 为 CRLF 输入补充用例（`models_test.go`、`project_prompts` 的测试文件）。
- [x] **Step 2: 修改并确认 LF 输入的行为不变。**
- [ ] **Step 3: 提交** — `git commit -am "fix: tolerate CRLF output from agent CLIs and prompt files"`

### Task 3.4: 环境检查（Environment）

**Files:**
- Create: `internal/platform/unix/environment.go`
- Create: `internal/platform/unix/environment_test.go`
- Modify: `cmd/platform/main.go`（`runServer` 启动时调用 `Environment().Check()`，失败则报错退出；`Describe()` 输出到启动日志）

**Interfaces:**
- Produces: `Environment.Check() error`、`Environment.Describe() []string`。
- unix 实现：读取 `/proc/version`（可注入读取函数以便测试），含 `Microsoft` 且不含 `WSL2` 时返回错误，提示升级到 WSL2。数据目录位于 `/mnt/<drive>` 下时，`Describe()` 给出告警，但不阻止启动。

- [ ] **Step 1: 写测试** — 覆盖原生 Linux、WSL2（`WSL2` 标记）、WSL1、DrvFs 数据目录四种输入。
- [ ] **Step 2: 实现并接入启动流程。**
- [ ] **Step 3: 提交** — `git commit -am "feat(platform): startup environment checks, reject WSL1"`

### Task 3.5: 密码输入

**Files:**
- Create: `internal/platform/unix/terminal.go`
- Modify: `cmd/platform/main.go:174-192`（`term.IsTerminal`、`term.ReadPassword` 改为 `Terminal().IsInteractive()`、`Terminal().ReadSecret`）

**Interfaces:**
- Produces: `Terminal.IsInteractive() bool`、`Terminal.ReadSecret(prompt) (string, error)`。

- [ ] **Step 1: 迁移** — 错误文案与现有 `requires an interactive terminal` 保持一致。
- [ ] **Step 2: 测试** — `go test ./cmd/platform/... -v` 全绿。
- [ ] **Step 3: 提交** — `git commit -am "refactor(platform): secret prompt behind Terminal port"`

### Task 3.6: 清空守卫清单（阶段 3 出口）

- [ ] **Step 1: 删除允许清单中已迁移的条目**，仅保留阶段 4 的 Windows 实现任务相关条目（如 `internal/projects/service.go:506` 的路径前缀比较，改由 `Dirs.SamePath` 承担）。
- [ ] **Step 2: 全量验证** — `go test ./... && go vet ./...`、前端测试。
- [ ] **Step 3: 提交** — `git commit -am "test(platform): phase 3 complete, guard allowlist reduced"`

---

## Phase 4：Windows 实现

前置条件：Task 4.0 完成。

### Task 4.0: 确认 pi 的 Windows 安装形态

**Files:**
- Modify: `docs/architecture/os-abstraction.md`（补充 pi 的可执行解析结论）

- [ ] **Step 1: 确认** — 在 Windows 实机确认 pi 的安装方式与 PATH 中的文件形态（是否同样是 npm shim，是否存在 `.exe`）。
- [ ] **Step 2: 结论写入设计文档**。若 pi 的形态与 opencode 相同，则 Task 4.3 的解析方案同时适用于 pi；否则为 pi 单独设计。
- [ ] **Step 3: 提交** — `git commit -am "docs: record pi windows installation layout"`

### Task 4.1: Windows 单实例锁

**Files:**
- Create: `internal/platform/windows/instance.go`（`//go:build windows`，`golang.org/x/sys/windows` 的 `LockFileEx`）
- Create: `internal/platform/windows/instance_test.go`

- [ ] **Step 1: 写测试** — 同一数据目录第二次加锁失败；进程退出后锁释放。
- [ ] **Step 2: 实现** — 锁文件路径与 unix 一致。
- [ ] **Step 3: 提交** — `git commit -am "feat(platform): windows single-instance lock"`

### Task 4.2: Windows 进程管理

**Files:**
- Create: `internal/platform/windows/process.go`
- Create: `internal/platform/windows/job.go`（Job Object，`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`）
- Create: `internal/platform/windows/process_test.go`

**Interfaces:**
- Produces:
  - `Configure(cmd)`：设置 `CREATE_NEW_PROCESS_GROUP`。
  - 子进程在 `Start` 之后加入平台的 Job Object，平台退出时自动结束。
  - `Alive(pid)`：`OpenProcess` + `GetExitCodeProcess == STILL_ACTIVE`。
  - `IsZombie(pid)`：恒 false。
  - `Terminate(pid)`：对平台外的进程（服务进程）执行 `taskkill /T /F /PID <pid>`；对平台拉起的进程直接结束 Job。

- [ ] **Step 1: 写测试** — 启动一个子进程后 `Alive` 为 true；`Terminate` 后为 false；子进程的子进程也被结束。
- [ ] **Step 2: 实现。** 注意 Job Object 在 `Start` 之后分配，存在极短的竞争窗口，需要在文档中说明，或使用 `CREATE_SUSPENDED` 后分配再恢复。
- [ ] **Step 3: 提交** — `git commit -am "feat(platform): windows process management with job objects"`

### Task 4.3: 可执行解析（通用原语 + opencode 专用布局）

**Files:**
- Create: `internal/platform/windows/executables.go`（PATHEXT 解析，排除无扩展名文件，`IsExecutable` 检查扩展名与文件类型）
- Create: `internal/agents/opencode/executable_windows.go`（`//go:build windows`，npm 布局解析）
- Create: `internal/agents/opencode/executable_windows_test.go`
- Modify: `internal/agents/opencode/adapter.go:216`（Windows 上走 opencode 专用解析）
- Modify: `internal/agents/contracts.go` 或 `internal/agents/descriptor.go`（`BackendDescriptor` 增加可选的 `ResolveExecutable` 字段，默认为 `Executables().Resolve`）
- Modify: `internal/settings/agents.go:211`（通过描述符的解析函数查找，不再直接调用平台查找）

**Interfaces:**
- Produces: `Executables.Resolve`（PATHEXT 感知）、`Executables.IsExecutable`（扩展名感知）。
- opencode 专用解析顺序（与设计文档第 5.3 节一致）：
  1. 取 PATH 中 `opencode.cmd` 所在目录作为 npm prefix（不调用 npm）。
  2. 定位 `<prefix>\node_modules\@opencode\cli\bin\opencode.exe`。
  3. 校验文件头为 `MZ`；否则查找 `@opencode\cli-windows-x64`（arm64 为 `cli-windows-arm64`）中的 `bin\opencode.exe`。
  4. 失败时才运行 `npm root -g` 作为兜底。
  5. 仍失败时才退回运行 `opencode.cmd`，参数只允许固定列表 `serve --service ...`。

- [ ] **Step 1: 测试夹具** — 在临时目录构造 `prefix\opencode.cmd`、`prefix\node_modules\@opencode\cli\bin\opencode.exe`（内容复制当前测试二进制，满足 `MZ` 头）、占位文件、平台包目录。路径包含空格（如 `C:\Users\Some User\...`）。
- [ ] **Step 2: 实现并覆盖每一步的分支。**
- [ ] **Step 3: 提交** — `git commit -am "feat(agents/opencode): resolve opencode.exe from npm global layout on windows"`

### Task 4.4: Windows 目录与路径

**Files:**
- Create: `internal/platform/windows/dirs.go`
- Create: `internal/platform/windows/pathpolicy.go`
- Create: `internal/platform/windows/dirs_test.go`
- Modify: `internal/projects/service.go:506`（前缀比较改为 `Dirs.SamePath` 与大小写不敏感的子路径判断）

**Interfaces:**
- Produces:
  - `ConfigDir()`：`%APPDATA%`。
  - `DataDir()`：`%LOCALAPPDATA%`。
  - `StateDir()`：`$XDG_STATE_HOME` 若设置，否则 `%USERPROFILE%\.local\state`（与 Linux 规则一致）。
  - `HomeDir()`：`%USERPROFILE%`。
  - `CanonicalizeUserPath`：保留盘符与 UNC，`filepath` 规范化，不做 WSL 翻译。
  - `SamePath`：不区分大小写。

- [ ] **Step 1: 写测试** — `C:\Users\chenli\.local\state\opencode\service.json` 的推导；大小写不同的同一路径判定为相同；UNC 路径保留。
- [ ] **Step 2: 实现。**
- [ ] **Step 3: 提交** — `git commit -am "feat(platform): windows directories and path policy"`

### Task 4.5: Windows 权限降级

**Files:**
- Create: `internal/platform/windows/perms.go`（`PrivateDir`、`PrivateFile` 仅创建，`Enforced()` 返回 false）
- Modify: `internal/platform/windows/environment.go`（新建）：`Check()` 在数据目录不位于 `%APPDATA%` 或 `%LOCALAPPDATA%` 下时输出告警，不阻止启动

- [ ] **Step 1: 测试** — `Enforced()` 为 false；数据目录位于共享目录时 `Describe()` 包含告警文案。
- [ ] **Step 2: 实现。**
- [ ] **Step 3: 提交** — `git commit -am "feat(platform): windows permission degradation with startup warnings"`

### Task 4.6: Windows 关停、回收站、终端

**Files:**
- Create: `internal/platform/windows/shutdown.go`（`os.Interrupt` 对应 Ctrl+C；`syscall.SIGTERM` 对应关闭终端窗口与注销。Go 运行时把 `CTRL_CLOSE_EVENT` 映射为 SIGTERM，需要在实机上确认）
- Create: `internal/platform/windows/trash.go`（`Move` 返回 `ErrTrashUnavailable`）
- Create: `internal/platform/windows/terminal.go`（`IsInteractive` 与 `ReadSecret` 使用 `golang.org/x/term`，它支持 Windows 控制台）

- [ ] **Step 1: 写测试** — 回收站返回不可用错误；`ReadSecret` 在非交互输入下退回 stdin 读取。
- [ ] **Step 2: 实现。**
- [ ] **Step 3: 提交** — `git commit -am "feat(platform): windows shutdown, trash and terminal"`

### Task 4.7: 测试平台覆盖

**Files:**
- Modify: 所有阶段 2、3 中“按 `Enforced()` 或能力跳过”的测试，确认 Windows 上不再因 `syscall` 或模式位失败。
- Modify: `internal/projects/trash_linux_test.go` 迁移后的测试，确认 Windows 上不被编译。
- Modify: `web/e2e/fixtures/bin/opencode` 相关测试：Windows 上 fixture 需要 `opencode.cmd` 形式（见阶段 5）。

- [ ] **Step 1: `GOOS=windows go vet ./...`** — 必须通过。
- [ ] **Step 2: 在 Windows 实机运行 `go test ./...`** — 允许 e2e 之外的测试失败，但每个失败必须记录在本计划的“已知限制”中。
- [ ] **Step 3: 清空守卫清单**，并确认 `internal/projects/service.go:506` 等条目已移除。
- [ ] **Step 4: 提交** — `git commit -am "test(platform): windows test suite passes"`

---

## Phase 5：工具链与文档

### Task 5.1: 跨平台开发工具

**Files:**
- Create: `cmd/devctl/main.go`（子命令 `build` 与 `run`，两者在 Windows 与 Linux 上都可用。`run` 为前台运行。`start`、`stop`、`restart`、`status`、`logs` 的后台模式保留在 `dev.sh` 中，只用于 Linux 与 WSL2）
- Modify: `dev.sh`（改为调用 `go run ./cmd/devctl "$@"`，保留入口）
- Modify: `Makefile`（目标改为调用 `devctl`，`help` 输出不变）

**Interfaces:**
- Produces: Windows 上 `devctl run` 在前台运行，Ctrl+C 或关闭终端窗口触发平台的正常关停。Windows 上没有 SIGTERM，关闭窗口后回收子进程依赖 Job Object（平台退出时结束子进程），需要在运维文档中说明。
- `devctl` 不得停止 opencode service。它只停止平台自己的进程。

- [ ] **Step 1: 写测试** — `build` 与 `run` 的参数拼装；`run` 在 Windows 上为前台进程。
- [ ] **Step 2: 实现。**
- [ ] **Step 3: 提交** — `git commit -am "feat(devctl): cross-platform dev tooling"`

### Task 5.2: e2e 脚本

**Files:**
- Modify: `web/scripts/run-e2e.mjs:25`（`TMPDIR ?? '/tmp/opencode'` 改为 `os.tmpdir()`）
- Modify: `web/scripts/run-e2e.mjs:88,104-105`（`child.kill('SIGTERM')` 在 Windows 上改为 `taskkill /T /F`）
- Modify: `web/scripts/run-e2e.mjs:96`（`pkill` 在 Windows 上改为 `taskkill /IM`）
- Create: `web/e2e/fixtures/bin/opencode.cmd`（Windows 版 mock，调用同目录的 Node 脚本）

- [ ] **Step 1: 修改并在 Linux 上确认 e2e 行为不变。**
- [ ] **Step 2: 在 Windows 上运行 `npm run test:e2e`。**
- [ ] **Step 3: 提交** — `git commit -am "fix(e2e): windows-compatible temp dir, process handling and fixtures"`

### Task 5.3: 运维与安全文档

**Files:**
- Modify: `docs/operations.md`（新增 Windows 章节：直接在命令行前台运行（不注册系统服务）、安装 Node 与 `npm install -g @opencode/cli`、数据目录、权限降级的限制与缓解、WSL2 要求、WSL1 不支持、DrvFs 数据目录告警、不支持回收站、停止服务的行为）
- Modify: `internal/secrets/store.go:1-10`（威胁模型注释补充：Windows 上 0600 不生效，保护依赖用户配置目录的默认 ACL）
- Modify: `docs/resource-sync.md:100`（备份文件在 Windows 上的敏感性标注）
- Modify: `docs/agent-compatibility.md`（新增 Windows 行：opencode 服务注册文件路径、可执行解析方式）
- Modify: `docs/architecture/os-abstraction.md`（阶段完成后，把“待确认”项改为“已确认”，并链接本计划）

- [ ] **Step 1: 写文档**，所有中文说明与现有文档风格一致。
- [ ] **Step 2: 检查文档中的路径与命令** 在 Windows 实机上能执行。
- [ ] **Step 3: 提交** — `git commit -am "docs: windows operations, permissions and compatibility"`

---

## Phase 6：CI 与验收

### Task 6.1: 双平台 CI

**Files:**
- Modify: `.github/workflows/ci.yml`（Task 0.3 创建）

**Interfaces:**
- Produces: `ubuntu-latest` 与 `windows-latest` 同时运行 `go vet ./...` 与 `go test ./...`；前端测试只在 `ubuntu-latest` 运行；e2e 在两个平台各运行一次。

- [ ] **Step 1: 配置矩阵。**
- [ ] **Step 2: 确认两个平台全绿。**
- [ ] **Step 3: 提交** — `git commit -am "ci: run go tests and e2e on linux and windows"`

### Task 6.2: 人工验收清单

**Files:**
- Modify: 本计划末尾的“验收记录”小节

- [ ] **Step 1: 在 Windows 原生环境逐项验证**：
  1. `npm install -g @opencode/cli` 后，`opencode --version` 可用。
  2. 平台启动后能识别已运行的 `opencode serve --service`（注册文件存在、pid 存活、HTTP 探活通过）。
  3. 平台未启动 opencode service 时，Agent 设置页显示“未启动”，新建任务显示既定提示文案。
  4. 新建任务、发送消息、中断、会话列表正常。
  5. 平台在命令行中按 `Ctrl+C` 退出，以及直接关闭终端窗口，两种情况下平台拉起的 pi 子进程都被回收，opencode service 仍存活。
  6. 第二个平台进程启动时单实例锁报错。
  7. 删除项目时提示“回收站不可用，是否永久删除”，确认后目录被删除。
  8. `set-password` 在交互终端与管道输入两种方式下都能工作。
  9. 启动日志输出权限降级告警。
  10. WSL2 中同一构建可正常启动，WSL1 中启动报错退出。
- [ ] **Step 2: 记录结果** 到“验收记录”小节，失败项开 issue 并链接。

---

## 风险与依赖

| 风险 | 影响 | 应对 |
| --- | --- | --- |
| pi 的 npm 安装形态未核实 | Task 4.3 的解析方案可能不适用于 pi | Task 4.0 前置确认；必要时为 pi 单独设计 |
| `bin/opencode.exe` 可能是 JS 启动器或占位文件 | 解析直接失败 | Task 4.3 的 `MZ` 校验与平台包回退 |
| Job Object 在 `Start` 之后分配存在竞争窗口 | 极短时间内子进程可能逃逸 | 使用 `CREATE_SUSPENDED` 或在文档中说明 |
| Windows 没有 `SIGTERM` | 平台关停的优雅程度低于 unix | 关停只回收平台自己的子进程；不依赖 opencode 的优雅退出 |
| 权限降级 | `secrets.json` 明文存储，保护依赖用户目录 ACL | 启动告警、运维文档、威胁模型注释 |
| 守卫允许清单被滥用 | 违规代码悄悄进入业务包 | 清单只能缩短，PR 审查时检查新增条目 |
| `golang.org/x/sys` 由间接依赖变为直接依赖 | `go.mod` 变更 | 在 Task 4.1 中执行 `go mod tidy` 并单独审查 |

## 已确认事项

1. 仓库没有 CI 配置，Task 0.3 与 6.1 新建 `.github/workflows/ci.yml`。
2. Windows 上平台直接在命令行启动，不注册系统服务，不提供 NSSM 或任务计划程序方案。
3. 不支持 opencode 1.x（`opencode-ai`），只支持 `@opencode/cli`（2.x），与后端 ID `opencode-v2` 一致。

## 基线记录

记录时间：2026-10-14（UTC），环境 `go1.27.1 linux/amd64`。

- `go test ./...`（`-count=1` 全量）：19 个包 `ok`，0 失败（`e2e/setup` 无测试文件）。耗时约 20s，其中 `cmd/platform` ~9s。
- `go vet ./...`（Linux）：干净，退出码 0。
- 前端：`npm run typecheck` 通过；`npm test -- --run`：30 个文件、338 个用例全部通过（Vitest v5.0.3，约 10s）。
- `GOOS=windows go vet ./...` / `GOOS=windows go build ./...` 失败，唯一非测试编译错误：
  - `internal/agents/opencode/discovery.go:116:20: undefined: syscall.Kill`（`processAlive` 用 `syscall.Kill(pid, 0)` 探测存活，Windows 无此符号）
  - 另有测试文件在 Windows 下同样引用 `syscall`（`go vet` 以 `_test.go` 形式报出，不影响 `go build`）：
    - `internal/agents/pi/adapter_test.go:319,600`（`syscall.Kill(pid, 0)` 存活断言）
    - `internal/agents/pi/interrupt_test.go:108`（同上）
    - `internal/agents/opencode/service_test.go:194`（`syscall.SysProcAttr{Setpgid: true}`）
    - `cmd/platform/main_test.go:123`（`Signal(syscall.SIGTERM)`）
- `syscall` 非测试引用清单（守卫测试初始允许清单的来源）：
  - `internal/projects/trash_linux.go:143-144`（`syscall.Stat_t`/`Stat`）
  - `internal/runtime/single_instance_unix.go`（`syscall.Flock`）
  - `internal/agents/opencode/discovery.go:116`（`syscall.Kill`）
  - `internal/agents/opencode/process_unix.go`（`Setpgid`/`Getpgid`/`Kill`）
  - `internal/agents/pi/process_unix.go`（`Setpgid`/`Kill`）
  - `cmd/platform/main.go:388`（`syscall.SIGINT/SIGTERM`）
- `runtime.GOOS/GOARCH` 引用清单：`internal/database/database_test.go:330`、`internal/runtime/single_instance_test.go:77`、`internal/agents/pi/adapter_test.go:47,318,502`、`internal/agents/pi/interrupt_test.go:107`、`internal/secrets/store_test.go:101,131`、`cmd/platform/config_test.go:71`。
- `golang.org/x/sys`：无直接或间接 import（`go.mod` 未声明，为间接依赖也不存在）；`x/sys/windows` 为待新增直接依赖（Task 4.1）。

## 验收记录

（Task 6.2 完成后填写）
