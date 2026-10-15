# 操作系统抽象层：Linux / WSL2 / Windows 原生

本文定义平台的操作系统层（OS 层）：哪些能力从业务代码中剥离、各平台如何实现、以及迁移顺序。与 `backend-boundaries.md` 的分层原则一致：业务层不得出现平台判断，平台差异只存在于 OS 层的实现里。

## 1. 目标与范围

| 项 | 决策 |
| --- | --- |
| 支持的运行环境 | Linux、WSL2（运行 Linux 构建）、Windows 原生 |
| WSL1 | 不支持。启动时检测到 WSL1 直接报错退出 |
| Linux 与 WSL2 | 使用同一套实现，不做区分 |
| Windows 回收站 | 不支持。删除项目始终走"永久删除 + 二次确认"分支（即 `ErrTrashUnavailable` 的既有路径） |
| opencode 在 Windows 上的形态 | 用户通过 npm 等方式安装 `opencode` 命令到 PATH，平台启动 `opencode serve --service`，行为与 Linux 相同 |
| 服务注册文件路径 | 已确认：Windows 上为 `%USERPROFILE%\.local\state\opencode\service.json`（如 `C:\Users\chenli\.local\state\opencode\service.json`），与 Linux 的 `~/.local/state/opencode/service.json` 使用同一规则，不需要 Windows 专用逻辑 |
| 权限模型 | Windows 接受降级：不实现 ACL 收紧，POSIX 权限位在 Windows 上不生效。依赖用户配置文件目录的默认 ACL 与运维约束，见第 5.7 节 |

**核心原则：** 平台只有两类实现，`unix`（Linux 与 WSL2 共用）和 `windows`。WSL2 不是第三个平台，它就是 Linux。

## 2. 分层

```
┌──────────────────────────────────────────────────────────────┐
│ 业务层：sessions · projects · settings · agents 协议 · http   │
│        resourcesync · auth · modelconfig                      │
│        禁止：import syscall / x/sys / 直接 os/exec / GOOS 判断 │
├──────────────────────────────────────────────────────────────┤
│ OS 端口：internal/core/platform（接口 + 平台无关策略）               │
│   Dirs · Instance · Process · Executables · Trash · Perms ·  │
│   Shutdown · Terminal · Environment                           │
├─────────────────────────────┬────────────────────────────────┤
│ unix 实现（Linux / WSL2）     │ windows 实现（原生 Windows）      │
│ internal/core/platform/unix      │ internal/core/platform/windows       │
│ flock · 进程组 · XDG 目录      │ LockFileEx · Job Object · 已知   │
│ POSIX 权限位 · /proc 探测      │ 文件夹 · PATHEXT · ACL           │
└─────────────────────────────┴────────────────────────────────┘
```

### 包结构

```
internal/core/platform/
  platform.go          # 端口接口、Platform 聚合、New()（按 build tag 选择实现）
  policy/              # 平台无关纯逻辑，两个平台都要测
  unix/                # //go:build unix：Linux 与 WSL2 共用
  windows/             # //go:build windows
  fake/                # 测试替身：可控的目录、进程、锁、可执行探测
```

- `internal/core/platform/unix` 与 `internal/core/platform/windows` 之外不得出现 `syscall`、`golang.org/x/sys/*`。
- 现有 `internal/app/runtime/single_instance_*.go`、`internal/core/agents/*/process_*.go`、`internal/app/projects/trash_*.go` 的平台文件迁入上述实现，不再散落在业务包中。
- 守卫测试：仿照 `internal/app/sessions/dependency_guard_test.go`，扫描业务包，禁止 import `syscall`、`golang.org/x/sys/*`，禁止在业务包中出现 `runtime.GOOS` 或 `runtime.GOARCH` 判断。

## 3. 端口定义

下面是接口草案，实际签名在第一阶段确定。

```go
package platform

// Platform 聚合所有 OS 能力。组合根创建一次，注入各服务。
type Platform interface {
    Name() string // "linux" | "windows"，仅用于诊断展示，业务不得据此分支
    Dirs() Dirs
    Instance() Instance
    Process() Process
    Executables() Executables
    Trash() Trash
    Perms() Perms
    Shutdown() Shutdown
    Terminal() Terminal
    Environment() Environment
}

type Dirs interface {
    // 平台数据目录（默认值）。unix: $XDG_CONFIG_HOME 或 ~/.config；windows: %APPDATA%
    ConfigDir() (string, error)
    // 平台数据目录。unix: $XDG_DATA_HOME 或 ~/.local/share；windows: %LOCALAPPDATA%
    DataDir() (string, error)
    TempDir() string
    // 用户输入路径的规范化。unix 保留现有 C:\ → /mnt/c 翻译；windows 保留盘符与 UNC
    CanonicalizeUserPath(p string) (string, error)
    // 路径比较（越界检查等）。unix 区分大小写；windows 不区分
    SamePath(a, b string) bool
}

type Instance interface {
    // 对数据目录加独占锁。unix: flock；windows: LockFileEx
    Acquire(dataDir string) (Lock, error)
}

type Process interface {
    // 配置子进程属性。unix: Setpgid；windows: CREATE_NEW_PROCESS_GROUP 并挂入 Job Object
    Configure(cmd *exec.Cmd) error
    // 将已启动的平台子进程纳入监管。Start 之后立即调用；unix 为空实现，
    // windows 挂入平台 Job Object（KILL_ON_JOB_CLOSE）
    Attach(proc *os.Process) error
    Alive(pid int) bool
    IsZombie(pid int) bool   // windows 恒 false
    // 结束进程及其子进程。unix: kill(-pgid)；windows: TerminateProcess + Job Object
    Terminate(pid int) error
}

type Executables interface {
    // 在 PATH 中查找命令。windows 处理 PATHEXT（.exe、.cmd、.bat）
    Resolve(name string) (string, error)
    // 判断路径是否为可执行文件。unix 检查权限位；windows 检查扩展名与文件类型
    IsExecutable(path string) bool
}

type Trash interface {
    // windows 实现直接返回 ErrTrashUnavailable
    Move(path string) error
}

type Perms interface {
    PrivateDir(path string) error   // unix: 0700；windows: 不做 ACL 收紧，仅创建目录
    PrivateFile(path string) error  // unix: 0600；windows: 不做 ACL 收紧
    // Enforced 报告当前平台是否真正执行私有权限，供诊断与启动告警使用
    Enforced() bool                 // unix: true；windows: false
}

type Shutdown interface {
    // unix: SIGINT、SIGTERM；windows: os.Interrupt 与控制台关闭事件
    Signals() []os.Signal
}

type Terminal interface {
    IsInteractive() bool
    ReadSecret(prompt string) (string, error) // unix: term.ReadPassword；windows: 控制台读取
}

type Environment interface {
    // 诊断信息与启动前检查，如 WSL1 检测、数据目录所在文件系统
    Describe() []string
    Check() error // 不满足运行条件时返回错误，如 WSL1
}
```

## 4. 能力矩阵：现有耦合点到端口的映射

| 能力 | 当前位置 | Linux / WSL2 实现 | Windows 实现 |
| --- | --- | --- | --- |
| 单实例锁 | `internal/app/runtime/single_instance_*.go` | `flock(LOCK_EX\|LOCK_NB)` | `LockFileEx`，独占模式 |
| 子进程组 | `internal/core/agents/{pi,opencode}/process_*.go` | `Setpgid` + `kill(-pid)` | `CREATE_NEW_PROCESS_GROUP` + Job Object（`KILL_ON_JOB_CLOSE`） |
| 进程存活 | `internal/providers/opencode/discovery.go:116` | `kill(pid, 0)` | `OpenProcess` + `GetExitCodeProcess` |
| 僵尸进程 | `internal/providers/opencode/process_unix.go` 读 `/proc` | `/proc/<pid>/stat` | 恒 false |
| 服务注册文件位置 | `internal/providers/opencode/discovery.go:35-44` | `$XDG_STATE_HOME` 或 `~/.local/state` | 同一规则：`$XDG_STATE_HOME` 或 `%USERPROFILE%\.local\state`（`os.UserHomeDir` 在 Windows 返回 `USERPROFILE`） |
| 可执行查找 | `pi/adapter.go:215`、`opencode/adapter.go:216`、`settings/agents.go:211` | `LookPath` | `LookPath` + PATHEXT，忽略无扩展名的 sh shim，解析 `.cmd` 入口（见第 5.3 节） |
| 可执行判断 | `settings/handlers_agents.go:217-227` | 权限位 `0o111` | 扩展名与文件类型 |
| 路径规范化 | `internal/app/paths/paths.go` | 保留 `/mnt/<drive>` 翻译与拒绝规则 | 保留盘符与 UNC，大小写不敏感比较 |
| 回收站 | `internal/app/projects/trash_linux.go` | freedesktop | `ErrTrashUnavailable` |
| 文件权限 | `database.go`、`secrets/store.go`、`runtime`、`resourcesync/backup.go` | POSIX 模式位，真正生效 | 模式位调用保留但不生效；通过 `Perms.Enforced()` 告警（见第 5.7 节） |
| 关停信号 | `cmd/platform/main.go:388` | `SIGINT`、`SIGTERM` | `os.Interrupt`、控制台关闭 |
| 密码输入 | `cmd/platform/main.go:174-192` | `term.ReadPassword` | 控制台读取，管道输入时退回 stdin |
| 数据目录默认值 | `cmd/platform`（`resolveDataDir`） | `ConfigDir()` | `ConfigDir()` 下的平台子目录 |

## 5. Windows 实现要点

### 5.1 单实例锁

- 在数据目录下打开锁文件，`LockFileEx(LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY)`。
- 进程退出时句柄关闭，锁自动释放，与 unix 的语义一致。

### 5.2 子进程与服务进程

- 平台拉起的 `pi`、`opencode serve` 等子进程，都挂入同一个 Job Object，并设置 `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`。这样平台异常退出时子进程不会残留。
- 挂接发生在 `Start` 之后（`Process.Attach`，句柄此时才存在），存在极短的竞争窗口：文档在此说明，不采用 `CREATE_SUSPENDED`（平台子进程均为长驻服务，窗口内退出无实际意义）。
- 停止机器级 opencode service 的路径：读取服务注册文件中的 `pid`，调用 `Terminate`（`TerminateProcess` 结合子进程树）。注意：注册文件中的 `pid` 是 opencode 服务进程本身（Node 进程），而不是 `opencode.cmd` 或 `cmd.exe` 的 pid。服务进程不是平台拉起的，不在平台的 Job Object 中，因此 `Terminate` 需要按 pid 结束进程树（`taskkill /T /F /PID`，或枚举子进程后逐个结束）。Windows 没有 `SIGTERM`，无法做与 unix 等价的优雅关停，因此需要先确认 opencode 是否提供 HTTP 关停接口（查看其 `/openapi.json`），有则优先使用，超时后再强制终止。
- 业务路径仍然只读注册文件并做便宜的存活探测，不发 HTTP 请求，与 `docs/superpowers/specs/2026-10-13-opencode-service-external-control-design.md` 保持一致。

### 5.3 可执行文件解析

npm 全局安装的 prefix 目录（如 `C:\Users\chenli\AppData\Roaming\npm`）下同时存在两个文件：

| 文件 | 用途 | Go 的处理 |
| --- | --- | --- |
| `opencode`（无扩展名，sh 脚本） | 给 Git Bash / MSYS 使用 | 不可执行，`LookPath` 不会匹配，必须忽略 |
| `opencode.cmd` | 给 cmd / PowerShell 使用，经 `cmd.exe` 调用 Node 入口 | `LookPath` 按 PATHEXT 匹配到它，需要特殊处理 |

- `Executables.Resolve` 在 Windows 上只接受带 PATHEXT 扩展名的文件，并排除无扩展名的 sh shim。
- 不解析 `opencode.cmd` 的文本，改为从 npm 全局目录定位包文件，直接启动 `.exe`，避免经过 `cmd.exe` 的参数转义问题。解析顺序：
  1. 取 PATH 中 `opencode.cmd` 所在目录作为 npm 全局 prefix。npm 在 Windows 上把 shim 放在 prefix 根目录，包位于 `<prefix>\node_modules`，因此不需要调用 npm。
  2. 定位 `<prefix>\node_modules\@opencode\cli\bin\opencode.exe`。
  3. 校验文件头为 PE（前两个字节 `MZ`）。如果不是（例如 postinstall 之前的 JS 占位文件），改查平台可选依赖 `@opencode\cli-windows-x64`（arm64 为 `cli-windows-arm64`）中的 `bin\opencode.exe`。
  4. 以上都失败时，才用 `npm root -g` 获取全局 node_modules 作为兜底。
  5. 全部失败才运行 `opencode.cmd`，参数只允许固定列表。
- 退回 `cmd.exe` 的情况下，参数只允许来自平台内部的固定列表（`serve --service --hostname ... --port ...`），不得拼接用户输入。
- 解析逻辑需要用真实 npm 安装的 shim 做测试（不同 prefix、带空格的用户名路径，如 `C:\Users\Administrator\...`）。
- 包信息依据 npm registry 元数据（`@opencode/cli` 最新版 2.0.28）：`bin.opencode` 指向 `bin/opencode.exe`，`os` 包含 `win32`，可选依赖包含 `@opencode/cli-windows-x64` 与 `@opencode/cli-windows-arm64`。
- 该包的 `postinstall.mjs` 可能改变 `bin/opencode.exe` 的内容，因此第 3 步的文件头校验不可省略。

### 5.3.1 pi 的安装形态（Task 4.0 结论：与 opencode 不同，为 pi 单独设计）

- pi 通过 npm 全局安装（`npm install -g @earendil-works/pi-coding-agent`，见 `internal/providers/pi/adapter.go` 的诊断文案）。
- npm registry 元数据（`@earendil-works/pi-coding-agent` 1.2.0）：`bin.pi` 指向 `dist/bundle/cli.js`——纯 JS 入口，没有原生 `.exe`，也没有平台可选依赖（opencode 有 `bin/opencode.exe` 与 `@opencode/cli-windows-*`）。
- 结论：Task 4.3 的 `.exe` 直解方案不适用于 pi。pi 在 Windows 上走自己的解析：从 npm prefix 定位包内 JS 入口（以包内 `package.json` 的 `bin` 字段为准，不硬编码 `dist/bundle` 路径），用 `node` 直接运行；`node` 经端口 `Executables.Resolve("node")` 查找（npm 可用则 `node` 必在 PATH）。
- 兜底顺序与 opencode 同形（`npm root -g`，最后才退回 `pi.cmd`），退回时的参数同样只允许平台内部固定列表。
- 仍待 Windows 实机核实（记入 Task 6.2 验收）：prefix 下 shim 文件的实际形态（`pi.cmd`／`pi.ps1` 是否存在）、`node` 是否总在 PATH、`package.json` 的 `bin` 字段在 1.1.0（仓库实测版本）与 1.2.0（registry 最新）是否一致。若实机形态与假设不符，只改 pi 包内的配置（包名、入口相对路径），不改机制。

### 5.4 路径与文件系统

- 用户输入的路径保留盘符（`C:\work`）与 UNC（`\\server\share`），内部统一用 `filepath` 处理。
- 越界检查与路径比较使用 `Dirs.SamePath`，Windows 下不区分大小写。
- 超过 `MAX_PATH` 的路径需要在 Windows 实机上验证。原则上不在业务代码中手工拼接 `\\?\` 前缀，若必须支持，由 `Dirs.CanonicalizeUserPath` 统一处理。

### 5.5 回收站

- Windows 实现返回 `ErrTrashUnavailable`。
- `internal/app/projects/service.go` 已有的"回收站不可用时要求永久删除二次确认"逻辑直接生效，不需要额外改动业务代码。

### 5.6 终端与密码输入

- 交互式终端使用控制台 API 读取，不回显。
- 非交互输入（管道、重定向）保留现有的 stdin 读取分支，与 `e2e/setup/main.go` 的做法一致。

### 5.7 权限降级（已确认）

Windows 不实现 ACL 收紧，`PrivateDir` 与 `PrivateFile` 只创建目录或文件，不设置访问控制。这是已确认的决策，影响范围如下。

**实际影响：**

- `secrets.json` 是明文存储，威胁模型中“防止同机其他本地用户”的保护在 Windows 上不再由平台本身提供。`internal/app/secrets/store.go` 的包注释需要增加平台说明。
- `database.go`、`runtime` 的锁文件、`resourcesync/backup.go` 的备份文件，同样不再对其他本地用户隐藏。
- 进程级隔离（同一用户下的其他进程）本来就不受 0600 保护，这一点两平台一致。

**缓解措施：**

- 数据目录默认位于 `%APPDATA%` 或 `%LOCALAPPDATA%`。Windows 用户配置文件目录的默认 ACL 只允许该用户、管理员与 SYSTEM 访问，这是目前最主要的隔离来源。
- `Environment.Check()` 在数据目录不位于当前用户配置文件目录下时输出告警，例如用户通过 `--data-dir` 指向了共享目录。
- `Perms.Enforced()` 为 false 时，启动日志与诊断页面都要说明私有权限未生效，不静默处理。
- `docs/operations.md` 增加 Windows 章节，写明上述限制、默认数据目录，以及不要把数据目录放在共享或同步目录中。
- 备份与导出：Windows 上的备份文件与 `secrets.json` 同等敏感，`docs/resource-sync.md` 中的备份说明需要同步标注。

**不做的事：**

- 不调用 `icacls` 或 `SetNamedSecurityInfo` 修改 ACL。
- 不对 `secrets.json` 加密。加密需要密钥管理，超出本次范围，若以后需要应单独立项。

## 6. Linux 与 WSL2 要点

- Linux 与 WSL2 使用同一套 `unix` 实现，不增加 WSL 专用分支。
- 现有 `paths.go` 中的 `/mnt/<drive>` 翻译保留在 `unix` 的路径策略中。这是为了兼容在 WSL2 中输入 Windows 风格路径的场景，它不是 Windows 原生行为。
- 启动时 `Environment.Check()` 检测 WSL1（`/proc/version` 含 `Microsoft` 且不含 `WSL2`），检测到则报错退出，并提示升级到 WSL2。
- 数据目录放在 `/mnt/c` 等 DrvFs 挂载点上时，`flock` 与模式位语义不可靠。`Environment.Describe()` 将其列为告警，`docs/operations.md` 写明数据目录应放在 WSL 原生文件系统中。

## 7. 待确认项

1. ~~权限模型~~ 已确认：Windows 接受降级，见第 5.7 节。
2. ~~opencode 服务注册文件在 Windows 上的位置~~ 已确认：`%USERPROFILE%\.local\state\opencode\service.json`，与 Linux 规则一致，见第 1 节。
3. ~~opencode 的安装形态~~ 已确认：npm 全局安装，prefix 下同时有无扩展名的 sh shim 与 `opencode.cmd`，见第 5.3 节。
4. **`bin/opencode.exe` 的实际内容（实机待确认，记入 Task 6.2）。** 需要在实机安装后确认它是原生 PE 程序，还是 JS 启动器或占位文件，以决定第 3 步之后是否会走到平台可选依赖。实现侧已按“非 MZ 则回退平台包”防御（Task 4.3）。
5. ~~Windows 上平台自身的部署方式~~ 已确认：直接在命令行前台启动，不注册系统服务。停止方式为 Ctrl+C 或关闭终端窗口。
6. **pi 的安装形态** 已确认（无需实机，依据 npm registry 元数据与仓库内诊断文案）：`bin.pi` 是纯 JS 入口（`dist/bundle/cli.js`，1.2.0），无原生 `.exe`、无平台可选依赖，见第 5.3.1 节。残留的实机核实项（shim 文件形态、`node` 在 PATH、`package.json` 的 `bin` 在 1.1.0/1.2.0 是否一致）记入 Task 6.2 验收。

实现计划：`docs/superpowers/plans/2026-10-14-windows-platform-support.md`（阶段 0–5 已执行，阶段 6 与实机验收待 Windows 环境）。

## 8. 测试策略

- `internal/core/platform/fake` 提供可控的目录、进程、锁和可执行探测，业务包的单元测试全部使用它，不依赖真实 OS 行为。
- 平台实现的测试按**能力**而非 `GOOS` 跳过，例如 `t.Skip` 的条件是"平台不支持回收站"，而不是"当前是 Windows"。
- CI 矩阵：`ubuntu-latest` 与 `windows-latest` 同时运行 `go test ./...` 与 `go vet ./...`。前端测试只在一个平台运行。
- 现有测试中的 `syscall.Kill` 调用（`internal/providers/pi/interrupt_test.go:108`、`adapter_test.go:319,600`、`opencode/process_unix_test.go`）迁移为调用 `platform.Process().Alive()`，以便 Windows 上编译通过。

## 9. 迁移顺序

| 阶段 | 内容 | 验收 |
| --- | --- | --- |
| 1 | 建立 `internal/core/platform` 与 `fake`，定义端口；加入依赖守卫测试 | 守卫测试对现有违规项失败并列出清单 |
| 2 | 把现有 unix 代码迁入 `platform/unix`，业务包改为注入 `Platform` | Linux 行为不变，`go test ./...` 全绿 |
| 3 | 修正平台无关的错误假设：`isExecutable`、`LookPath`、`\r\n` 处理、硬编码的 XDG 与 `~/.local` | Linux 行为不变，新增用 fake 的单元测试 |
| 4 | 实现 `platform/windows`：锁、进程、可执行解析、路径、权限、关停、密码输入、回收站占位 | Windows 上 `go build` 与 `go test ./...` 通过 |
| 5 | 跨平台工具链：用 Go 实现 `cmd/devctl`，替代 `Makefile` 与 `dev.sh` 的 Unix 特有部分；补充 Windows 运维文档 | Windows 上可完成构建、启动、停止 |
| 6 | CI 矩阵与 e2e | Linux 与 Windows 上 e2e 通过 |

第 1 至 3 阶段不改变行为，可以独立合并。第 4 阶段的前置确认（第 7 节第 4、6 项）已转为“元数据确认 + 实机复核记入 Task 6.2”；`internal/app/secrets/store.go` 的威胁模型注释与 `docs/operations.md` 的 Windows 章节随第 5 阶段补齐（见上文实现计划链接）。
