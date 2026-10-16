# 交接：架构评审七项整改 —— 第 7 项（资源功能整体下线）实施中

> 分支：`refactor/remove-resources`（从 `refactor/typed-turn-interaction` 切出）
> 状态：**已完成并提交**。§3 六项全部落地：openapi 重校验（0 悬空 `$ref`）、`provider.go` 注释修正、
> 新增 `TestMigrateDropsResourceLibrary`、`docs/resource-sync.md` 已删除、`make verify` 通过、
> `make e2e` 失败集合与基线一致（6 failed，均为基线文件，无新增回归）。
> 恢复：`git checkout refactor/remove-resources && git status --short`。

## 1. 七项对齐决策（用户已全部确认）

| # | 问题 | 决定 |
|---|---|---|
| 1 | 项目删除不级联 | 删除项目改为只解除注册，不再删目录；停实例失败则中止；事务内清理 `agent_instances`、`agent_sessions`、`session_prompts`、`project_resources` 并删备份文件；移除 trash 端口与目录删除功能。未实施。 |
| 2 | agent 启用判断分散 | 取消项目级开关，只保留 `settings` 平台级开关；新建/继续/发送/启动实例统一经它判断；禁用后阻止新入口，不自动停实例。未实施。 |
| 3 | `instances` / `sessions` 过重 | `git-info` 移到 `projects`；服务控制拆为独立包 `agentservices`；`sessions` 按子包拆分。未实施。 |
| 4 | session 与 instance 绑定（含原第 5 项） | 会话归属改为 `(项目, 后端, 原生会话 id)`，实例仅作运行时宿主；无运行实例时自动启动并继续；删 `rebind` 与合并逻辑；turn 跟踪改键；迁移前去重。未实施。 |
| 6 | 领域层直接用 GORM 实体 | 每个领域引入 repository 接口。未实施。 |
| 7 | resourcesync 手工装配 | **整个资源同步功能下线，且资源功能整体下线**（含 `resources`、`resourcesync`、`/api/secrets`；保留 `secrets.Store` 供 `modelconfig` 使用；MCP 凭据不清理；四张表新增迁移删除）。**实施中，见 §2。** |

建议实施顺序：7 → 1 → 2 → 3 → 4 → 6，每项独立提交。

## 2. 第 7 项已完成

### 2.1 后端删除与接线

- 删除：`internal/app/resources/`（8 文件）、`internal/app/resourcesync/`（含 `adapters/`，16 文件）、`internal/app/secrets/handlers.go`。
- `cmd/platform/main.go`：删除 `resourceService`、`managedFiles`、`resourceSync` 装配与 `Resources` / `Secrets` / `ResourceSync` 路由依赖；`secretStore` 保留（`modelconfig` 仍用）。
- `internal/transport/http/router.go`：删除 `Resources` / `Secrets` / `ResourceSync` 三个依赖项、import 与注册块；`NewRouter` 尾部只剩 `}`（有意为之）。
- `internal/app/database/models.go`：`Resource`、`ProjectResource`、`SessionPrompt`、`ManagedResourceFile` 转为 legacy 注释（历史迁移 8–12 仍引用，不可删结构体），写法对标 `WorkspaceRoot`。
- `internal/app/database/migrations.go`：新增 `version 17 drop_resource_library`，按依赖逆序删表（`managed_resource_files` → `session_prompts` → `project_resources` → `resources`），`HasTable` 守卫，对标迁移 13。
- 测试删减：`internal/transport/http/router_test.go` 删除 `TestRouterServesResourcesBehindSessionAndCSRF`（:681–:797）及三个 import；`internal/app/secrets/store_test.go` 删除 `TestHandlersNeverReturnSecretValue`（73 行）及四个残留 import。`router.go` 的 import 顺序差异（`auth`/`instances`）在 HEAD 已存在，未动。
- 验证：`go vet ./...` 通过；`go test ./...` 全 ok（含 `cmd/platform`、`database`、`http`、`secrets`）。

### 2.2 前端删除与接线

- 删除：`web/src/api/resources.ts`、`web/src/stores/resources.ts`、`web/src/features/resources/`（5 文件）、`web/src/tests/resources.spec.ts`、`web/e2e/resources.spec.ts`、`web/e2e/resource-security.spec.ts`。
- `web/src/features/projects/ProjectSettingsPanel.vue`：删除 `ResourcesSection` 引用与挂载点，文件头注释同步。
- `web/src/tests/project-settings-panel.spec.ts`：删除 resources store mock 与“资源管理”用例，文件头注释同步。
- `web/src/api/client.ts`：删除 `resource_conflict`、`invalid_secret_value` 两条错误文案。
- 保留确认：`models.ts` / `ModelSettings.vue` 的 `secretRef` 属于模型服务；`interactions.spec.ts` 等的 `resources: []` 属于权限载荷；`SessionEventStream.ts` 的 "browser resources" 是注释英文。均与资源库无关。
- 验证：`npx tsc -p tsconfig.typecheck.json` 零错误；`npx vitest run` 33 文件 / 386 项全过（之前 34/396，减少的 10 项即被删的资源测试）。

### 2.3 OpenAPI 删除

- `docs/api/openapi.yaml`：删除 paths `:1815–:2469`（`/resources*`、`/projects/{id}/resources*`、`/prompts*`、`/secrets*`、`/resources/sync*`）、schemas `:3826–:4063`（`ResourceDefinition` … `ApplySyncRequest`）、parameters `:2981–:2988`（`ResourceId`）、`:3004–:3010`（`SecretId`）。从下往上按行号删除，无行号漂移。
- 保留确认：`:3086` 的 `resources` 属于 `PermissionRequest`；`:1447/:1501` 的 `resync` 是 SSE 帧名；`secretRef` 属于模型服务 schema。
- `ResourceDefinition` / `SyncPreview*` / `SecretMetadata` / `SecretWriteRequest` / `ResourceReferenceRequest` 在全仓库已无引用（grep 为空）。

## 3. 收尾完成记录（§4 步骤已执行完毕）

1. **openapi 重校验**：已做。用 `github.com/goccy/go-yaml` 临时 Go 程序解析，paths 48 / schemas 53 / parameters 6，悬空 `$ref` 0 个；临时程序已删除。
2. **注释残留**：已改。`internal/app/secrets/provider.go:5` 改为 “the model configuration depends on”。
3. **迁移测试**：已加。`TestMigrateDropsResourceLibrary`（`database_test.go`）断言四表（`managed_resource_files`、`session_prompts`、`project_resources`、`resources`）均不存在；`go test ./internal/app/database/ ./internal/app/secrets/` 通过。
4. **文档同步**：已做。`AGENTS.md` 分层列表去掉 `resources,resourcesync`（前后端各一处）；`backend-boundaries.md` 删除 resourcesync 耦合行；`docs/resource-sync.md` 整篇删除（`git rm`，无活跃文档再引用它）；`os-abstraction.md` 分层图、`:152` 表格、`:223` 与 `:232` 的 `resourcesync/backup.go` 引用同步（`:232` 改写为下线说明，不再引用已删文档）。历史计划与 `claudian-refactor-plan.md` 有意不改；`agent-compatibility.md` 无需改动。
5. **全量验证**：`make verify` exit=0（vet + Go 全 ok + vitest 33 文件/386 项 + 前端构建成功）。`make e2e`：6 failed / 9 passed，失败文件为 `agent-service`、`layout`、`plantuml`、`session-history`×2、`sessions`，全部落在 `typed-turn-interaction-handoff.md` 基线集合内，无新增回归（基线 7 项含并行 flake 波动）。
6. **提交**：本项独立提交（见本文件顶部状态）。

## 4. 新会话继续步骤

```bash
git checkout refactor/remove-resources && git status --short
# 1) 改 provider.go:5 注释；2) 加迁移测试并 go test ./internal/app/database/
# 3) 临时 Go 程序校验 openapi.yaml；4) 同步 §3.4 文档；5) make verify
```
