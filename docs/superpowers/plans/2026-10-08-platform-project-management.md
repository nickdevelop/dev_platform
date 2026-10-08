# Platform and Project Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the single-user local web platform foundation with LAN password access and safe management of multiple project directories.

**Architecture:** Use a Go module with a Gin HTTP server and GORM persistence over SQLite using a pure-Go driver. The backend owns authentication, workspace authorization, filesystem operations, and project records. The Vue browser communicates through a documented JSON API and never accesses the filesystem directly.

**Tech Stack:** Go 1.24+, Gin, GORM, `github.com/glebarez/sqlite`, SQLite, Vue 3, TypeScript, Vite, Vue Router, Pinia, Vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-10-08-ai-development-platform-design.md`

## Global Constraints

- Support Linux and Windows WSL; run one platform service per data directory.
- Use one shared password for the single-user product; do not add user or role management.
- Allow LAN browser access, but require authentication for all management APIs.
- Persist platform data separately from managed project directories.
- Restrict filesystem operations to configured authorized workspace roots and validate canonical real paths.
- Prefer moving deleted directories to the OS trash; permanent deletion requires explicit confirmation when trash is unavailable.
- Never log passwords, session tokens, or MCP credentials.
- Do not execute filesystem or process operations from browser code.
- Keep all API DTOs JSON-stable and documented; Vue uses generated or manually maintained TypeScript API types, never GORM model shapes directly.

## Review Focus

- A symlink inside an authorized root points outside it; reject operations on the escaped target.
- A path contains traversal segments, platform-specific separators, Unicode, or case variants; authorization must remain correct on Linux and WSL.
- Two requests concurrently create/move to the same target; do not overwrite data or leave stale project records.
- Trash integration is unavailable or fails; do not report success or permanently delete without an explicit second confirmation.
- Login CSRF, expired cookies, or repeated wrong passwords; reject the operation and avoid leaking authentication material.

---

## File Structure

- `go.mod`, `go.sum`: Go module and pinned dependencies.
- `cmd/platform/main.go`: process startup, configuration, single-instance lock, graceful shutdown.
- `internal/http/`: Gin router, middleware, handlers, JSON error envelope.
- `internal/database/`: GORM models, connection, versioned migrations, repositories.
- `internal/auth/`: password verification, sessions, CSRF, login throttling.
- `internal/workspaces/`: configured roots and safe path authorization.
- `internal/projects/`: project records and directory lifecycle service/routes.
- `internal/config/`, `internal/logging/`: runtime config and secret redaction.
- `web/package.json`, `web/src/`: Vue 3 application, login, project list, and operation dialogs.
- `web/src/api/`: typed API client, auth/session handling, request/response DTOs.
- `web/tests/`, `e2e/`: component, backend, and browser tests.
- `docs/api/`: API contract documentation or OpenAPI specification.

## Task 1: Bootstrap Go service, Vue workspace, and health API

**Files:**
- Create: `go.mod`, `cmd/platform/main.go`, `internal/http/router.go`, `internal/http/health.go`
- Create: `web/package.json`, `web/tsconfig.json`, `web/vite.config.ts`, `web/src/main.ts`, `web/src/App.vue`
- Create: `docs/api/openapi.yaml`
- Test: `internal/http/health_test.go`, `web/src/tests/app.test.ts`

**Interfaces:**
- Produces: `NewRouter(deps Dependencies) *gin.Engine`; `GET /api/health` returns HTTP 200 and `{"status":"ok"}`.
- Produces: Vite dev server and production Vue build; API routes are same-origin in deployment.

- [ ] **Step 1: Write the failing Go test** asserting the health endpoint status and JSON body using `httptest`.
- [ ] **Step 2: Run test to verify it fails** with `go test ./internal/http -run TestHealth -v`.
- [ ] **Step 3: Implement minimal Gin router and health handler**, initialize the Go module and Vue 3 + TypeScript + Vite workspace, and define the health endpoint in OpenAPI.
- [ ] **Step 4: Run verification** with `go test ./...` and `cd web && npm test -- --run`.
- [ ] **Step 5: Commit** as `chore: bootstrap go and vue workspace`.

## Task 2: Add GORM database connection and platform models

**Files:**
- Create: `internal/database/database.go`, `internal/database/models.go`, `internal/database/migrations.go`
- Create: `internal/config/platform.go`
- Test: `internal/database/database_test.go`

**Interfaces:**
- Consumes: Task 1 server package.
- Produces: `Open(dataDir string) (*gorm.DB, error)` using `glebarez/sqlite`; versioned migration function; models/tables `Setting`, `AuthSession`, `WorkspaceRoot`, `Project`; `PlatformConfig` with authorized roots.

- [ ] **Step 1: Write failing tests** for fresh DB creation/migration, config persistence across reopen, unique project path, and DB location under `dataDir` rather than any project root.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/database -v`.
- [ ] **Step 3: Implement GORM models, SQLite connection and explicit versioned migrations**; enable foreign keys, busy timeout and WAL where supported; do not rely on `AutoMigrate` as the production schema migration strategy.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/database -v`.
- [ ] **Step 5: Commit** as `feat: persist platform configuration with gorm`.

## Task 3: Implement password login and protected API sessions

**Files:**
- Create: `internal/auth/password.go`, `internal/auth/session_store.go`, `internal/auth/middleware.go`, `internal/auth/handlers.go`, `internal/auth/csrf.go`
- Modify: `internal/http/router.go`, `internal/database/models.go`
- Test: `internal/auth/auth_test.go`

**Interfaces:**
- Consumes: Task 2 database and auth session model.
- Produces: `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/session`; Gin `RequireSession` middleware; HttpOnly, SameSite=Strict, expiring session cookie; CSRF token required on mutations; first-run interactive password setup command reading hidden input from a TTY.

- [ ] **Step 1: Write failing tests** for valid/invalid login, expiry, unauthenticated API rejection, CSRF rejection, bounded repeated-failure throttling, and setup refusing to overwrite a configured password.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/auth -v`.
- [ ] **Step 3: Implement password setup/verification and middleware** using Argon2id, cryptographically random session IDs, CSRF tokens, constant-time comparison where applicable, login throttling, and TTY-only hidden first-run setup (never accept password in argv or logs).
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/auth -v`.
- [ ] **Step 5: Commit** as `feat: add password-protected web sessions`.

## Task 4: Add authorized workspace roots and safe path resolution

**Files:**
- Create: `internal/workspaces/path_policy.go`, `internal/workspaces/handlers.go`
- Modify: `internal/http/router.go`, `docs/api/openapi.yaml`
- Test: `internal/workspaces/path_policy_test.go`

**Interfaces:**
- Consumes: Task 2 DB, Task 3 auth middleware.
- Produces: `ResolveAuthorizedPath(input string, roots []string) (ResolvedPath, error)` where `ResolvedPath` includes canonical path and root; authenticated workspace-root list/add/remove APIs.

- [ ] **Step 1: Write failing tests** for valid descendants, `..`, outside absolute paths, symlink escape, nonexistent descendants with validated parents, Windows/WSL path normalization, and case behavior on supported filesystems.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/workspaces -v`.
- [ ] **Step 3: Implement canonical path authorization** using `filepath.Abs`, `EvalSymlinks` for existing components, root containment checks, and explicit rejection of ambiguous or escaped paths.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/workspaces -v`.
- [ ] **Step 5: Commit** as `feat: constrain filesystem access to workspace roots`.

## Task 5: Implement project registration and directory lifecycle

**Files:**
- Create: `internal/projects/service.go`, `internal/projects/handlers.go`, `internal/projects/trash_linux.go`, `internal/projects/trash_fallback.go`
- Modify: `internal/database/models.go`, `internal/http/router.go`, `docs/api/openapi.yaml`
- Test: `internal/projects/service_test.go`

**Interfaces:**
- Consumes: Task 4 `ResolveAuthorizedPath`; Task 2 project repository/models.
- Produces: JSON DTO `Project { id, name, path }`; service methods `Register`, `Create`, `Move`, `Rename`, `Unregister`, `Delete`; authenticated `/api/projects` routes with typed confirmation payload.

- [ ] **Step 1: Write failing service tests** for register, create, rename, move, unregister-without-disk-delete, trash deletion, unavailable-trash requiring explicit permanent-delete confirmation, target collision, missing directory, and concurrent target collision.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/projects -v`.
- [ ] **Step 3: Implement project operations** with GORM transactions around record changes, no-overwrite filesystem semantics, ownership checks, OS trash integration where supported, and separately confirmed permanent-delete fallback.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/projects -v`.
- [ ] **Step 5: Commit** as `feat: manage project directories safely`.

## Task 6: Build Vue login and project-management UI

**Files:**
- Create: `web/src/router.ts`, `web/src/stores/auth.ts`, `web/src/api/client.ts`, `web/src/api/projects.ts`, `web/src/views/LoginView.vue`, `web/src/views/ProjectsView.vue`, `web/src/components/ProjectDialog.vue`
- Test: `web/src/tests/login.spec.ts`, `e2e/projects.spec.ts`

**Interfaces:**
- Consumes: Task 3 auth API and Task 5 project API/DTOs.
- Produces: login/logout, project list/search, register/create/move/rename/unregister/delete dialogs with explicit path and impact confirmation; typed DTOs from OpenAPI.

- [ ] **Step 1: Write failing Vue component tests** asserting unauthenticated route guard, successful login, 401 session expiry redirect, and unregister/delete distinction.
- [ ] **Step 2: Run tests to verify they fail** with `cd web && npm test -- --run login.spec.ts`.
- [ ] **Step 3: Implement Vue Router/Pinia state and API client** with same-origin calls, CSRF handling, accessible dialogs, and generated or explicitly synchronized TypeScript DTOs.
- [ ] **Step 4: Run tests to verify they pass** with `cd web && npm test -- --run login.spec.ts`.
- [ ] **Step 5: Add Playwright lifecycle test** against a temporary authorized workspace; verify with `cd web && npm run test:e2e -- projects.spec.ts`.
- [ ] **Step 6: Commit** as `feat: add vue project management interface`.

## Task 7: Verify Linux/WSL deployment and hardening

**Files:**
- Create: `internal/runtime/single_instance.go`, `docs/operations.md`
- Modify: `cmd/platform/main.go`, `Makefile`, `web/vite.config.ts`
- Test: `internal/runtime/single_instance_test.go`, `e2e/security.spec.ts`

**Interfaces:**
- Consumes: all prior server and UI tasks.
- Produces: configurable host/port/data directory, single-instance lock scoped to data directory, graceful shutdown, documented Linux/WSL start/stop and LAN binding.

- [ ] **Step 1: Write failing tests** for second process lock rejection, LAN bind config, traversal through API inputs, and no secret values in captured logs.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/runtime -v` and `cd web && npm run test:e2e -- security.spec.ts`.
- [ ] **Step 3: Implement lock, graceful shutdown and deployment config**; serve the built Vue assets from the Go service in production and document safe LAN exposure.
- [ ] **Step 4: Run full verification** with `go test ./...`, `go vet ./...`, `cd web && npm test -- --run && npm run build && npm run test:e2e`.
- [ ] **Step 5: Commit** as `feat: harden local platform runtime`.
