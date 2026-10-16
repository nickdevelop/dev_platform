# 交接：架构评审七项整改 —— 第 1、2、7 项已完成，第 3/4/6 项待实施

> 分支：`refactor/remove-resources`，HEAD `c9128d4`，工作树干净，未推送。
> 恢复：`git checkout refactor/remove-resources && git status --short && git log --oneline -2`。
> 详细过程见 `docs/architecture/remove-resources-handoff.md`（已随第 7 项提交）。

## 1. 已完成

| # | 问题 | 落点 |
|---|---|---|
| 7 | resourcesync 手工装配 → **资源功能整体下线** | 提交 `c9128d4`（53 文件，+105/−7862，独立提交） |
| 1 | 项目删除不级联 → **改为解除登记并级联清理** | 分支 `refactor/project-delete-cascade`（见下方第 1 项落地说明） |

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

## 2. 未完成（均未开始，均需新会话先读码再动手）

建议实施顺序：3 → 4 → 6，**每项独立分支、独立提交，不混交**。
新分支从上一已完成项的分支切出（第 2 项从 `refactor/project-delete-cascade` 切出，它含第 1 项）。

| # | 问题 | 已确认的决定 |
|---|---|---|
| 2 | agent 启用判断分散 → **已完成** | 取消项目级开关，只保留 `settings` 平台级开关；新建/继续/发送/启动实例统一经它判断；禁用后阻止新入口，不自动停实例。落地：分支 `refactor/agent-platform-switch`，迁移 v18 删 `agent_backend_configs`，`/projects/:id/agent-backends` 两条路由删除。 |
| 3 | `instances` / `sessions` 过重 | `git-info` 移到 `projects`；服务控制拆为独立包 `agentservices`；`sessions` 按子包拆分。 |
| 4 | session 与 instance 绑定（含原第 5 项） | 会话归属改为 `(项目, 后端, 原生会话 id)`，实例仅作运行时宿主；无运行实例时自动启动并继续；删 `rebind` 与合并逻辑；turn 跟踪改键；迁移前去重。 |
| 6 | 领域层直接用 GORM 实体 | 每个领域引入 repository 接口。 |

注意：原第 1 项提到的 `session_prompts` / `project_resources` 表已在第 7 项迁移 v17 中删除，清理逻辑已省略，不要试图清理不存在的表。

## 3. 通用约束（每项都适用）

- 历史计划文档（`docs/superpowers/plans/2026-10-0*`、`claudian-refactor-plan.md` 中的过程记录）**有意不改**，只记录历史事实。
- 每项完成后跑 `make verify`；`make e2e` 结果与 §1 基线对比，无新增失败文件方可提交。
- 提交信息沿用 `refactor:` 前缀中文格式（如 `c9128d4`）。

## 4. 新会话继续步骤

```bash
git checkout refactor/remove-resources && git status --short
git checkout -b refactor/<本项短名>
# 读码 → 实施 → make verify → make e2e（对 §1 基线）→ 独立提交
```
