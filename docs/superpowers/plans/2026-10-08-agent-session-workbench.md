# Agent Session Workbench Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide a project-scoped unified session workbench for opencode v2 and pi, including multiple isolated Agent instances, streaming, native-session discovery where supported, and capability-aware controls.

**Architecture:** Define a stable Go `AgentAdapter` interface with one adapter per Agent. The platform stores instance/session indexes and normalized events while Agent engines remain authoritative for native execution and history. Run pi through argument-safe child-process APIs and opencode through its API; isolate event streams and lifecycle per instance. Expose JSON APIs and SSE streams consumed by the Vue workbench.

**Tech Stack:** Go 1.24+, Gin, GORM/SQLite (`glebarez/sqlite`), Vue 3 + TypeScript + Vite, Vue Router, Pinia, Vitest, Playwright; opencode v2 API and pi CLI.

**Spec:** `docs/superpowers/specs/2026-10-08-ai-development-platform-design.md`

## Global Constraints

- One platform service manages multiple projects and multiple Agent instances per project.
- opencode v2 uses API integration; pi uses controlled CLI process integration.
- Keep Agent-native session data in the Agent; persist only platform indexes and ID mappings unless a specific API requires a safe cache.
- Declare per-Agent capabilities; never expose unsupported controls as if they work.
- Keep project working directories within registered authorized project roots.
- Do not build user input into shell command strings; use executable plus argument arrays and bounded environment.
- One Agent failure must not stop unrelated sessions, instances, or projects.
- Redact secrets and credentials in logs and normalized error responses.
- Keep Gin handler JSON DTOs separate from GORM persistence models and document endpoints in OpenAPI.

## Review Focus

- Native session metadata is malformed, stale, or belongs to an unregistered project; do not attach it to the wrong project.
- A process/API disconnect happens mid-stream; mark the right instance/session disconnected and preserve isolation.
- Two instances of the same backend run in one project; events and stop requests must never cross instance boundaries.
- Agent emits unknown event kinds or partial JSON lines; preserve diagnosable raw metadata without crashing stream parsing.
- Session stop races with process exit or completion; operation is idempotent and final state is stable.

---

## File Structure

- `internal/agents/contracts.go`: backend, instance, session, capability, event and adapter interfaces.
- `internal/agents/registry.go`: adapter registry and backend probing.
- `internal/agents/instances/`: instance lifecycle and per-instance event fan-out.
- `internal/agents/opencode/`: opencode API client and adapter.
- `internal/agents/pi/`: pi process manager, stream parser, and adapter.
- `internal/sessions/`: session index, discovery, normalized stream service and Gin handlers.
- `internal/database/models.go`: Agent instances and session mapping GORM models.
- `web/src/stores/agents.ts`, `web/src/stores/sessions.ts`: Vue state and API wiring.
- `web/src/views/`, `web/src/components/`: Agent settings and session workbench.
- `internal/agents/**/*_test.go`, `internal/sessions/**/*_test.go`, `web/src/tests/`, `e2e/`: tests.

## Task 1: Define Agent contracts and capability model

**Files:**
- Create: `internal/agents/contracts.go`, `internal/agents/events.go`
- Test: `internal/agents/contracts_test.go`

**Interfaces:**
- Produces: `BackendID` constants `opencode-v2`, `pi`; `Capabilities`; `Instance`; `SessionRef`; `NormalizedEvent`; `AgentAdapter` with `Probe`, `StartInstance`, `StopInstance`, `ListSessions`, `CreateSession`, `ContinueSession`, `SendMessage`, `Subscribe`.
- All adapter methods accept `context.Context`; event envelopes include instance ID, session ID, sequence, timestamp, event type and optional backend metadata.

- [ ] **Step 1: Write failing tests** for supported backend IDs, capability combinations, valid state transitions, normalized events, unknown raw events and context cancellation.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/agents -run TestContracts -v`.
- [ ] **Step 3: Implement Go types and validation methods**; make capability values explicit booleans and preserve unknown backend event metadata in a bounded `json.RawMessage`.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/agents -run TestContracts -v`.
- [ ] **Step 5: Commit** as `feat: define agent adapter contracts`.

## Task 2: Add GORM persistence, adapter registry, and isolated instance lifecycle

**Files:**
- Create: `internal/agents/registry.go`, `internal/agents/instances/service.go`, `internal/agents/instances/handlers.go`
- Modify: `internal/database/models.go`, `internal/http/router.go`, `docs/api/openapi.yaml`
- Test: `internal/agents/instances/service_test.go`

**Interfaces:**
- Consumes: foundation project repository and Task 1 contracts.
- Produces: `Registry.Get(id BackendID) (AgentAdapter, bool)`; GORM models `AgentBackendConfig`, `AgentInstance`; authenticated project instance JSON routes.

- [ ] **Step 1: Write failing tests** for adapter registration, project-to-instance association, multiple same-backend instances, isolated state/event subscriptions, stop-one-only behavior, invalid project rejection, and adapter startup failure.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/agents/instances -v`.
- [ ] **Step 3: Implement registry and lifecycle service** with durable IDs, project path validation, idempotent stop, GORM transaction boundaries, context cancellation and per-instance event channels.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/agents/instances -v`.
- [ ] **Step 5: Commit** as `feat: manage isolated agent instances`.

## Task 3: Implement pi CLI adapter with safe process and event handling

**Files:**
- Create: `internal/agents/pi/process.go`, `internal/agents/pi/event_parser.go`, `internal/agents/pi/adapter.go`
- Test: `internal/agents/pi/event_parser_test.go`, `internal/agents/pi/adapter_test.go`

**Interfaces:**
- Consumes: Task 1 `AgentAdapter`; Task 2 lifecycle/registry.
- Produces: pi adapter implementing probe, instance lifecycle, session discovery/create/continue, send, stop and subscribe according to verified CLI capabilities.

- [ ] **Step 1: Write failing parser tests** for partial JSON lines, malformed lines, unknown events, stderr diagnostics and end-of-stream; add adapter tests using a fake executable for safe args, working directory, stream delivery and stop.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/agents/pi -v`.
- [ ] **Step 3: Implement process wrapper and adapter** with `exec.CommandContext` (never `sh -c`), validated executable and args, project working directory, bounded output, cancellation, process-group cleanup where supported, and capabilities derived from verified pi CLI behavior.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/agents/pi -v`.
- [ ] **Step 5: Commit** as `feat: integrate pi cli agent`.

## Task 4: Implement opencode v2 API adapter

**Files:**
- Create: `internal/agents/opencode/client.go`, `internal/agents/opencode/adapter.go`
- Test: `internal/agents/opencode/client_test.go`, `internal/agents/opencode/adapter_test.go`

**Interfaces:**
- Consumes: Task 1 `AgentAdapter`; Task 2 lifecycle/registry.
- Produces: opencode v2 adapter implementing probe, API connection lifecycle, session discovery/create/continue, message streaming, stop and capability declaration.

- [ ] **Step 1: Write failing client tests** with `httptest.Server` for success, auth/config failure, timeout, disconnect mid-stream, unknown event and stop; add adapter tests for native session mapping.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/agents/opencode -v`.
- [ ] **Step 3: Implement typed API client and adapter**; verify endpoint/event mapping against supported opencode v2 API, set request limits/timeouts, redact credentials, and isolate each connection.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/agents/opencode -v`.
- [ ] **Step 5: Commit** as `feat: integrate opencode v2 api`.

## Task 5: Add session indexes, discovery, and normalized SSE APIs

**Files:**
- Create: `internal/sessions/repository.go`, `internal/sessions/service.go`, `internal/sessions/handlers.go`
- Modify: `internal/database/models.go`, `internal/http/router.go`, `docs/api/openapi.yaml`
- Test: `internal/sessions/service_test.go`, `internal/sessions/stream_test.go`

**Interfaces:**
- Consumes: Agent registry/adapters from Tasks 2–4.
- Produces: GORM session index with platform ID, project ID, backend ID, instance ID, native session ID, title and status; authenticated list/create/continue/send/stop JSON routes; authenticated SSE subscription route.

- [ ] **Step 1: Write failing tests** for create/index mapping, native session discovery, stale-session behavior, project scoping, event sequence ordering, disconnect state, malformed event and stop race/idempotency.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/sessions -v`.
- [ ] **Step 3: Implement repository/service/handlers**; validate project/instance/session relationships, preserve unknown backend payload, set correct SSE headers, and keep native history Agent-owned.
- [ ] **Step 4: Run tests to verify they pass** with `go test ./internal/sessions -v`.
- [ ] **Step 5: Commit** as `feat: add unified session APIs`.

## Task 6: Build Vue project Agent settings and session workbench

**Files:**
- Create: `web/src/api/agents.ts`, `web/src/api/sessions.ts`, `web/src/stores/agents.ts`, `web/src/stores/sessions.ts`, `web/src/views/ProjectAgentsView.vue`, `web/src/views/SessionWorkbenchView.vue`, `web/src/components/AgentInstanceList.vue`, `web/src/components/SessionList.vue`, `web/src/components/MessageView.vue`, `web/src/components/CapabilityControls.vue`
- Test: `web/src/tests/workbench.spec.ts`, `e2e/sessions.spec.ts`

**Interfaces:**
- Consumes: Task 5 JSON/SSE APIs and DTOs; foundation project API.
- Produces: project Agent settings and instance controls; unified session workbench showing history, streaming, status, stop and only supported model/tool controls.

- [ ] **Step 1: Write failing Vue tests** for multiple instances, streamed messages, instance-isolated events, stop state, omission of unsupported controls and tool confirmation rendering when capability is advertised.
- [ ] **Step 2: Run tests to verify they fail** with `cd web && npm test -- --run workbench.spec.ts`.
- [ ] **Step 3: Implement Vue views/stores** with EventSource or fetch-based SSE handling, reconnect/error state, safe text rendering and accessible status announcements.
- [ ] **Step 4: Run tests to verify they pass** with `cd web && npm test -- --run workbench.spec.ts`.
- [ ] **Step 5: Add Playwright workflow** running opencode mock API and fake pi executable concurrently in one project; verify events and stop remain isolated with `cd web && npm run test:e2e -- sessions.spec.ts`.
- [ ] **Step 6: Commit** as `feat: add unified agent session workbench`.

## Task 7: Verify compatibility matrix, resilience, and observability

**Files:**
- Create: `internal/agents/diagnostics.go`, `docs/agent-compatibility.md`
- Modify: adapter implementations and Vue diagnostics view where needed.
- Test: `internal/agents/diagnostics_test.go`, `e2e/agent-failures.spec.ts`

**Interfaces:**
- Consumes: completed adapters and workbench.
- Produces: startup/API/CLI version diagnostics and explicit per-backend capability matrix.

- [ ] **Step 1: Write failing tests** for missing executable, unsupported CLI/API version, disconnected API, malformed event stream and one adapter failure while another remains operational.
- [ ] **Step 2: Run tests to verify they fail** with `go test ./internal/agents -run TestDiagnostics -v` and `cd web && npm run test:e2e -- agent-failures.spec.ts`.
- [ ] **Step 3: Implement diagnostics and compatibility docs** based on tested versions; expose actionable redacted errors and declared capabilities in the UI.
- [ ] **Step 4: Run full verification** with `go test ./...`, `go vet ./...`, and `cd web && npm test -- --run && npm run build && npm run test:e2e`.
- [ ] **Step 5: Commit** as `feat: add agent diagnostics and resilience`.
