# Resource Management and Configuration Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Manage reusable Skill, MCP, and prompt definitions and safely synchronize selected resources into project or Agent configuration files.

**Architecture:** Keep canonical resource definitions and sync state in GORM-backed platform storage. A Go `ResourceSyncAdapter` translates a definition into a backend-specific managed-file patch; an explicit first-apply action establishes ownership, after which updates are automatic. Detect external edits and unmanaged conflicts before applying any patch, and preserve backups for managed files. Store MCP credential values in a separate platform-private plaintext file with restrictive OS permissions; API/UI/logging only expose opaque references. This does not protect against same-user processes or backups containing the secret file.

**Tech Stack:** Go 1.24+, Gin, GORM/SQLite (`glebarez/sqlite`), Vue 3 + TypeScript + Vite, Vue Router, Pinia, Vitest, Playwright, filesystem-backed credential store.

**Spec:** `docs/superpowers/specs/2026-10-08-ai-development-platform-design.md`

## Global Constraints

- Manage Skill, MCP, and prompt definitions and associate them with one or more projects.
- Prompt definitions support project-level instructions and prompts selected in a session.
- Synchronize resources automatically after the user has explicitly confirmed the initial target and management scope.
- Only update files or marked fragments owned by the platform; do not silently overwrite unmanaged same-name config or externally modified managed content.
- Display target path and change summary before first apply or target change.
- Store MCP credentials in a separate file under the platform-private data directory; use `0600` permissions on Linux/WSL, never return plaintext through API/UI, and redact credentials from logs/errors.
- Document that plaintext file storage does not protect against same-user or higher-privilege processes, nor backups that include the file.
- If a backend does not support a resource or format, report it unavailable rather than ignoring it.
- Keep platform resource data separate from project content and Agent native session data.
- Keep Gin JSON DTOs separate from GORM models; document endpoints in OpenAPI and maintain matching Vue TypeScript DTOs.

## Review Focus

- MCP secret is missing, rotated, malformed, or unavailable from the OS secret store; do not write empty or plaintext secret values.
- Project/Agent target config contains an unmanaged same-name entry; do not modify it before user resolution.
- A managed file was externally edited after last sync; detect drift and pause automatic sync.
- Resource names include Unicode, separators, reserved filenames, or traversal strings; never allow target path escape.
- Multi-resource sync partially fails; report per-resource status and preserve the last known-good managed content.

---

## File Structure

- `internal/resources/`: GORM model, repository, CRUD service and Gin handlers for Skill/MCP/prompts.
- `internal/secrets/`: file-based secret store, permission enforcement and redaction helpers.
- `internal/resourcesync/`: adapter registry, target resolution, diff/ownership manifest, backup and apply services.
- `internal/resourcesync/adapters/`: project-prompt, opencode and pi translators.
- `internal/database/models.go`: resource/project association, opaque secret reference and sync ownership models.
- `web/src/api/resources.ts`: typed API client and DTOs.
- `web/src/views/ResourcesView.vue`, `web/src/components/resources/`: resource editor, project association, preview and conflict resolution UI.
- `internal/resources/**/*_test.go`, `internal/resourcesync/**/*_test.go`, `web/src/tests/resources.spec.ts`, `e2e/`: test coverage.

## Task 1: Define resource and sync contracts

**Files:**
- Create: `internal/resources/contracts.go`, `internal/resourcesync/contracts.go`
- Test: `internal/resources/contracts_test.go`, `internal/resourcesync/contracts_test.go`

**Interfaces:**
- Produces: `ResourceKind` constants `skill`, `mcp`, `prompt`; `ResourceDefinition`; `ResourceTarget`; `SyncPreview`; `SyncConflict`; `SyncState`; backend support declaration. MCP DTO contains opaque `SecretRef`, never raw secret.

- [ ] **Step 1: Write failing tests** for valid Skill/MCP/project and session prompts, targets, previews, conflicts, invalid paths/names, and JSON serialization excluding secret material.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/resources ./internal/resourcesync -run TestContracts -v`.
- [ ] **Step 3: Implement Go contract structs, typed enums and validation methods**; use explicit JSON tags and avoid serializing GORM fields.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/resources ./internal/resourcesync -run TestContracts -v`.
- [ ] **Step 5: Commit** as `feat: define resource and sync contracts`.

## Task 2: Implement canonical resource CRUD and project associations

**Files:**
- Create: `internal/resources/models.go`, `internal/resources/repository.go`, `internal/resources/service.go`, `internal/resources/handlers.go`
- Modify: `internal/database/models.go`, `internal/http/router.go`, `docs/api/openapi.yaml`
- Test: `internal/resources/service_test.go`, `internal/resources/handlers_test.go`

**Interfaces:**
- Consumes: foundation authentication/project repositories and Task 1 contracts.
- Produces: authenticated CRUD/list/apply-association APIs for Skill, MCP and prompt records; project-resource relation; handlers map GORM models to JSON DTOs and never return secret material.

- [ ] **Step 1: Write failing tests** for create/read/update/delete, project association, session-prompt selection, project scoping, invalid names and secret omission from responses.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/resources -v`.
- [ ] **Step 3: Implement GORM repositories, service validation and Gin handlers**; use transactions for resource/project associations and explicit DTO mapping.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/resources -v`.
- [ ] **Step 5: Commit** as `feat: manage project resources`.

## Task 3: Add protected MCP secret storage

**Files:**
- Create: `internal/secrets/store.go`, `internal/secrets/provider.go`, `internal/secrets/handlers.go`, `internal/secrets/redact.go`
- Modify: `internal/resources/service.go`, `internal/database/models.go`
- Test: `internal/secrets/store_test.go`

**Interfaces:**
- Consumes: Task 2 MCP CRUD service and platform data directory.
- Produces: `SecretStore.Set(ctx,id,value) error`, `Get(ctx,id) (string,error)`, `Delete(ctx,id) error`; MCP definitions persist opaque references only; secret values are stored in a separate private file with mode `0600` on Linux/WSL.

- [ ] **Step 1: Write failing tests** for set/get/delete, missing secret, secret rotation, API redaction, diagnostic/log redaction, and Linux/WSL file mode `0600`.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/secrets -v`.
- [ ] **Step 3: Implement the file-based secret store** under the platform-private data directory; create files with owner-only permissions (`0600` on Linux/WSL), avoid following symlinks, and atomically replace files without widening permissions. Document that same-user processes and backups can read the plaintext file.
- [ ] **Step 4: Run tests to verify they pass** with temporary data directories and OS-specific permission assertions.
- [ ] **Step 5: Commit** as `feat: protect mcp credentials`.

## Task 4: Implement managed-file ownership, preview, conflict detection, and backup

**Files:**
- Create: `internal/resourcesync/manifest.go`, `internal/resourcesync/diff.go`, `internal/resourcesync/target_policy.go`, `internal/resourcesync/backup.go`
- Modify: `internal/database/models.go`
- Test: `internal/resourcesync/managed_file_test.go`

**Interfaces:**
- Consumes: Task 1 contracts and foundation workspace path policy.
- Produces: `PreviewSync(ctx,projectID,resourceIDs) ([]SyncPreview,error)`; ownership manifest with canonical target path, managed hash, backup reference, and last applied resource version; conflict states `unmanaged-target`, `external-drift`, `target-invalid`.

- [ ] **Step 1: Write failing tests** for preview without mutation, first-apply unmanaged collision, managed-file update, external drift, target traversal, backup creation, and rollback after write failure.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/resourcesync -run TestManagedFile -v`.
- [ ] **Step 3: Implement canonical target checks, patch previews, hash-based drift checks, and atomic backup/write**; mark ownership only after successful explicit apply.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/resourcesync -run TestManagedFile -v`.
- [ ] **Step 5: Commit** as `feat: track managed resource files safely`.

## Task 5: Implement resource adapters and sync coordinator

**Files:**
- Create: `internal/resourcesync/adapter.go`, `internal/resourcesync/coordinator.go`, `internal/resourcesync/adapters/project_prompts.go`, `internal/resourcesync/adapters/opencode.go`, `internal/resourcesync/adapters/pi.go`
- Test: `internal/resourcesync/coordinator_test.go`, `internal/resourcesync/adapters/adapters_test.go`

**Interfaces:**
- Consumes: Agent registry/capability declarations and Task 4 managed-file service.
- Produces: `ResourceSyncAdapter.Supports(kind,backendID) bool`; `Render(ctx,resource,context) (ManagedPatch,error)`; coordinator `Preview`, `Apply`, `SyncChangedResource`, `ResolveConflict`.

- [ ] **Step 1: Write failing tests** for per-Agent format translation, unsupported resource declaration, first-apply consent, post-consent automatic update, external conflict pause, multi-resource partial failure, and secret resolution without logging.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/resourcesync -run 'TestCoordinator|TestAdapters' -v`.
- [ ] **Step 3: Implement adapters and coordinator** using verified native config formats; after initial confirmed apply, automatically sync later definition changes and stop on conflicts.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/resourcesync -run 'TestCoordinator|TestAdapters' -v`.
- [ ] **Step 5: Commit** as `feat: synchronize resources to agent configs`.

## Task 6: Build Vue resource management and conflict-resolution UI

**Files:**
- Create: `web/src/api/resources.ts`, `web/src/stores/resources.ts`, `web/src/views/ResourcesView.vue`, `web/src/components/resources/ResourceEditor.vue`, `web/src/components/resources/SyncPreview.vue`, `web/src/components/resources/ConflictDialog.vue`, `web/src/components/resources/SecretField.vue`
- Test: `web/src/tests/resources.spec.ts`, `e2e/resources.spec.ts`

**Interfaces:**
- Consumes: Tasks 1–5 resource APIs and DTOs.
- Produces: resource CRUD screens, project associations, sync-target preview/confirmation, automatic sync status and explicit conflict resolution choices.

- [ ] **Step 1: Write failing Vue tests** for CRUD, secret masking, target/change preview, initial apply consent, unsupported resource indication and conflict resolution.
- [ ] **Step 2: Run tests to verify they fail** with `cd web && npm test -- --run resources.spec.ts`.
- [ ] **Step 3: Implement accessible Vue resource UI**; never fetch or display stored secret values; show per-target sync state and errors.
- [ ] **Step 4: Run tests to verify they pass** with `cd web && npm test -- --run resources.spec.ts`.
- [ ] **Step 5: Add E2E sync workflow** covering an external edit, paused sync and user resolution; verify with `cd web && npm run test:e2e -- resources.spec.ts`.
- [ ] **Step 6: Commit** as `feat: add resource management interface`.

## Task 7: Verify cross-platform sync recovery and secret safety

**Files:**
- Create: `docs/resource-sync.md`
- Modify: sync coordinator/adapters where verification requires it.
- Test: `internal/resourcesync/platform_recovery_test.go`, `e2e/resource-security.spec.ts`

**Interfaces:**
- Consumes: resource manager and sync adapters.
- Produces: documented per-Agent file formats/targets, recovery/conflict workflow, credential file location and permission behavior on Linux/WSL, and plaintext-at-rest risk.

- [ ] **Step 1: Write failing tests** for interrupted atomic write, Windows/WSL path case/encoding, restrictive credential-file permissions, secret rotation during sync, and multi-target partial failure.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/resourcesync -run TestPlatformRecovery -v` and `cd web && npm run test:e2e -- resource-security.spec.ts`.
- [ ] **Step 3: Implement recovery and documentation**; preserve last known-good files, enforce private file permissions where supported, and report exact per-target result without sensitive data or overstating protection from same-user access/backups.
- [ ] **Step 4: Run full verification** with `go test ./...`, `go vet ./...`, and `cd web && npm test -- --run && npm run build && npm run test:e2e`.
- [ ] **Step 5: Commit** as `feat: harden resource sync and recovery`.
