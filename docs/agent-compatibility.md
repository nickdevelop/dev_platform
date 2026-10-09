# Agent backend compatibility

This document is the explicit compatibility matrix for the agent session
workbench: what each backend adapter declares, which versions were tested, and
which behaviors are **provisional** because they cannot be verified without a
configured model provider.

The tested version ranges live in `internal/agents/diagnostics.go` and are
pinned by `internal/agents/diagnostics_test.go`, so the code and this document
cannot drift apart. Each backend's capability declaration lives in its
adapter's `Capabilities()` and is pinned by that adapter's own tests (for
example `TestAdapterCapabilitiesDeclared` in `internal/agents/opencode`), not
by the diagnostics tests.

Tested environment: Go 1.27.1, Node 22, `opencode` 2.0.24, `pi` 1.1.0. No live
model provider was configured during verification (an opencode probe without
credentials returned 502), so everything that needs a real model call is
listed as provisional below.

## Tested version ranges

| Backend | Tested version | Supported range | Notes |
| --- | --- | --- | --- |
| `opencode-v2` | **2.0.24** | `[2.0.0, 3.0.0)` | One binary provides both the CLI and the v2 API server, so the CLI version is also the API version. |
| `pi` | **1.1.0** | `[1.0.0, 2.0.0)` | CLI only; `--mode json` protocol. |

Diagnostics classify a detected version as follows:

- inside the range → `ok`;
- outside the range or undeterminable → `warning` (the backend may still work;
  the message names the detected version, the tested range and the tested
  version);
- missing executable → `error`.

## Capability matrix

A capability the backend does not declare is **visibly false**; the workbench
must never expose a control for a capability a backend did not declare.

| Capability | `opencode-v2` | `pi` |
| --- | --- | --- |
| `streaming` | ✅ — `session.text.delta` normalized into incremental message events. Verified against the mock v2 server. | ✅ — `--mode json` `text_delta` provider events. Verified against the fake CLI. |
| `sessionDiscovery` | ✅ — `GET /api/session?directory=…`. Verified. | ✅ — bounded scan of pi's on-disk JSONL session files for the instance directory. Verified. |
| `sessionContinue` | ✅ — `GET /api/session/{id}` re-attach. Verified. | ✅ — the next prompt re-uses `--session-id`. Verified at the adapter level; end-to-end transcript continuity is **provisional** (see below). |
| `stop` | ✅ — terminates the per-instance `opencode serve` process group. Verified. | ✅ — terminates the per-prompt process group. Verified. |
| `modelSelection` | ⚠️ — session-create `model` is accepted (verified); per-message switching via `session.switchModel` is **provisional** (see below). | ⚠️ — the `--model` flag is passed through (verified argv plumbing); real model resolution is **provisional**. |
| `toolCallEvents` | ✅ — `session.tool.*` normalized into tool call/result events. Verified against the mock server. | ✅ — `tool_execution_start/update/end` records. Verified against the fake CLI. |
| `toolConfirmation` | ❌ — the v2 API emits `permission.asked`, but the frozen `AgentAdapter` contract exposes no method to answer it, so no confirmation control is advertised. | ❌ — print/JSON mode cannot pause a tool call for confirmation. |

## Diagnostics

`GET /api/agent-backends/diagnostics` (session-authenticated) returns one
report per registered backend: the overall status (the worst check), the
detected CLI version, the tested version, the individual checks and the
capability declaration. Every message is actionable and **redacted** —
credentials (passwords, tokens, key-value secrets) are masked before a
diagnostic is built, and server startup output is bounded.

| Check | `opencode-v2` | `pi` |
| --- | --- | --- |
| `executable` | ✅ resolves the binary (never starts it). Missing → `error` with install/configuration guidance. | ✅ same. |
| `version` | ✅ runs `opencode --version` and classifies against `[2.0.0, 3.0.0)`. | ✅ runs `pi --version` and classifies against `[1.0.0, 2.0.0)`. |
| `api` | ✅ probes `GET /api/info` of every running instance. Unreachable → `error` naming the instance and "stop and restart"; server version outside the range → `warning`. | — (no API server; the check is omitted rather than faked). |
| `event_stream` | runtime only: a malformed SSE payload surfaces as an error event with code `malformed_event`, the connection is dropped and reconnected with backoff — one bad payload never kills the instance. | runtime only: a malformed JSONL record surfaces as `parse_error` with the offending record preserved as raw bytes, and the stream continues. |

A backend whose adapter fails to produce diagnostics (including a crash) is
reported as an `adapter`-kind error report; it never prevents the other
backends' reports, and the registry walk always completes.

## Session history

The platform does **not** read or replay an agent-native session's prior
messages. Discovery and continue re-attach the platform's index to the native
session id, but the workbench renders only the live normalized event stream it
receives after a session is opened, and it says so explicitly in the
conversation pane (a history note above the message log). Agent-native history
remains the authority and is never copied into the platform.

This is a deliberate, documented scope boundary: reading native history would
require a new adapter operation, and the frozen `AgentAdapter` contract
deliberately exposes no history method, so a continuation shows no earlier
turns rather than a partially reconstructed one.

## Project backend enable/disable

`AgentBackendConfig` stores a per-project `enabled` flag for each backend
(spec §5.3). The default is **enabled**: a registered backend is usable in a
project until an operator disables it from the project Agent settings
(`GET`/`PUT /api/projects/{id}/agent-backends[/{backend}]`). Starting an
instance of a disabled backend is refused with `backend_disabled` (409);
already-running instances are unaffected, and re-enabling restores starting
immediately. The `(project_id, backend)` pair is unique, so toggles are
idempotent upserts.

## Resilience and isolation (tested)

- **Instance isolation** — every instance owns its stream, lifecycle and
  subscription; stopping or failing one instance never touches another
  (`internal/agents/instances`, workbench e2e).
- **Backend isolation** — diagnostics and lifecycle go through the registry
  per backend; a broken adapter is reported, never fatal
  (`agents.CollectDiagnostics`).
- **Stream resilience** — bounded SSE payloads, idle-timeout watchdog,
  reconnect backoff with a cap, and redacted stream diagnostics (opencode);
  bounded record sizes and continued parsing after malformed records (pi).
- **Redaction** — launch diagnostics, stream errors and stderr events pass
  through the redaction helper; the generated server password is passed via
  the environment, never argv, and never logged.

## Provisional behaviors — requires a configured provider

The following behaviors could not be verified in this environment and are
documented as **provisional — requires a configured provider**. They are
implemented and unit-tested at the contract/adapter level, but end-to-end
verification against a real model provider is outstanding:

1. **opencode `session.switchModel`** — per-message model switching
   (`SendMessageRequest.Model`). The mock server accepts the call; behavior
   against a real provider (including unsupported-model errors) is unverified.
2. **opencode full prompt flow with a live model** — streamed deltas, tool
   call/result events, retry and structured provider errors produced by a real
   model run.
3. **opencode model resolution** — provider catalogs and model ids available
   on a configured instance.
4. **pi continue semantics end-to-end** — the adapter re-sends `--session-id`;
   that pi extends the same transcript across prompts must be confirmed with
   a provider configured.
5. **pi `--name` semantics** — the adapter passes the stored title as
   `--name`; that pi persists it as the session display name (and it then
   appears in discovery) is unverified.
6. **pi model resolution** — `--model` patterns resolve only with provider
   catalogs/credentials configured.

Everything not listed as provisional is covered by the Go test suite
(`go test ./...`), the adapter unit tests with mock servers/CLIs, and the
Playwright e2e suite under `web/e2e`.
