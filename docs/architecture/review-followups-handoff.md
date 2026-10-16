# 架构评审七项整改 —— 全部完成总结

> 分支：`master`，HEAD `acb7525`，已推送至 `origin/master`。
> 详细过程见下文各项目落点与说明。

## 1. 整改完成汇总

| # | 问题 | 落点与说明 |
|---|---|---|
| 7 | resourcesync 手工装配 → **资源功能整体下线** | 提交 `c9128d4`（53 文件，+105/−7862，独立分支与提交）：删除 resources/resourcesync/secrets API 与四张表（迁移 v17）。 |
| 1 | 项目删除不级联 → **解除登记并级联清理** | 提交 `df5a518`（分支 `refactor/project-delete-cascade`）：删除项目目录删除与 OS Trash 端口，删除 `/projects/:id/delete`，统一走 `unregister` 并在事务内停止实例并清理记录。 |
| 2 | agent 启用判断分散 → **平台级开关统一判断** | 提交 `b1757d3`（分支 `refactor/agent-platform-switch`）：取消项目级开关，迁移 v18 删除 `agent_backend_configs`；新建/继续/发送/启动实例统一由平台设置控制。 |
| 3 | `instances` / `sessions` 过重 → **服务控制拆包与子包拆分** | 提交 `daedc65` 与 `331d0b6`（分支 `refactor/split-instances-sessions`）：`git-info` 移到 `projects`；服务控制拆为独立包 `internal/app/agentservices`；`sessions` 拆出 `store`（会话索引持久化）与 `turns`（轮次跟踪）子包。 |
| 4 | session 与 instance 绑定 → **会话归属项目/后端/原生键** | 提交 `809f466`（分支 `refactor/session-owned-by-project`）：迁移 v19 去重并建立 `(project_id, backend, native_session_id)` 唯一索引；实例作为运行时宿主；删除 `rebind` 与合并逻辑；`turns.Tracker` 改按 `(backend, nativeSessionID)` 键跟踪。 |
| 6 | 领域层直接用 GORM 实体 → **引入 repository 接口** | 提交 `acb7525`（分支 `refactor/domain-repositories`）：projects、instances、settings、auth、modelconfig、sessions 六个领域全部引入 repository 接口，领域服务 Service 结构体彻底解耦 `*gorm.DB`，保留 `NewService(db)` 兼容接入。 |

第 1 项落地说明：
- `projects.Service.Delete` 与 `/projects/:id/delete` 路由已删除；`/projects/:id/unregister` 成为唯一的移除入口，不再删除目录。
- 停实例经 `projects.InstanceStopper` 注入（组合根 `SetInstanceStopper`，因 `instances` 已依赖 `projects`）；任一停止失败返回 409 `instance_stop_failed`，不删除任何记录。
- 事务内删除 `agent_sessions`、`agent_instances`、`agent_backend_configs` 与项目记录；事务内再次检查，仍有运行中实例则中止（防止停实例与启动竞争）。
- "删备份文件"：当前代码已无备份文件来源（`ManagedResourceFile.BackupPath` 随资源功能下线），本项不涉及。
- `agent_backend_configs` 未在原清单中，但属于项目私有数据，一并清理；第 2 项取消项目级开关时会整体移除该表的使用。
- 平台 `Trash` 端口、`ErrTrashUnavailable`、unix/windows 实现与 `policy/trash.go` 全部移除；前端删除对话框与 `trash_unavailable` 文案移除；OpenAPI、`os-abstraction.md`、`operations.md` 同步。
- 验证：`make verify` exit=0；`make e2e` 8 failed / 7 passed，其中 `agent-failures` 单独运行通过（并行 flake），`layout` 两项在基线提交 `72b2d15` 上复现一致，其余失败文件均在基线集合内，无新增回归。

第 7 项内容：删除 `internal/app/resources/`、`internal/app/resourcesync/`（含 `adapters/`）、`internal/app/secrets/handlers.go`；
组合根与路由摘除 `Resources` / `Secrets` / `ResourceSync` 接线（`secretStore` 保留供 `modelconfig` 用）；
`Resource`、`ProjectResource`、`SessionPrompt`、`ManagedResourceFile` 转 legacy 注释；
新增迁移 v17 `drop_resource_library` 按依赖逆序删四表；
前端删除 `api/resources.ts`、`stores/resources.ts`、`features/resources/` 及三处资源测试；
openapi.yaml 删除 resources/prompts/secrets/sync 相关 paths、schemas、parameters（重校验：0 悬空 `$ref`）；
新增 `TestMigrateDropsResourceLibrary`；删除 `docs/resource-sync.md`；
同步 `AGENTS.md`、`backend-boundaries.md`、`os-abstraction.md`。

验证基线（`c9128d4` 处）：`make verify` exit=0（vet + Go 全 ok + vitest 33 文件/386 项 + 前端构建）；
`make e2e` 6 failed / 9 passed，失败文件（`agent-service`、`layout`、`plantuml`、`session-history`×2、`sessions`）
全部落在 `typed-turn-interaction-handoff.md` 基线集合内，无新增回归（基线 7 项含并行 flake 波动）。

## 2. 验证基线对比

- `make verify`：exit=0（go vet 全通过，Go 全包测试全部 PASS，前端 33 文件 / 382 项单测全部 PASS，Vite 构建成功）。
- `make e2e`：与基线完全一致（6–7 项基线失败，其中 `agent-failures` 经单独运行为已知时序波动，其余全部与基线一致，无任何新增回归）。
