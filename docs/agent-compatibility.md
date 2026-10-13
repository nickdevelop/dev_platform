# Agent backend compatibility

This document is the explicit compatibility matrix for the agent session
workbench: what each backend adapter declares, which versions were tested, and
which behaviors are **provisional** because they cannot be verified without a
configured model provider.

The tested version ranges live in each adapter's `Descriptor` (`internal/agents/opencode/descriptor.go`,
`internal/agents/pi/descriptor.go`) and are pinned by their `descriptor_test.go`, so the code and this document
cannot drift apart. Each backend's capability declaration lives in its
adapter's `Capabilities()`. The method is part of the `AgentAdapter` contract,
so a backend cannot be registered without one, and it is pinned by that
adapter's own tests (for example `TestAdapterCapabilitiesDeclared` in
`internal/agents/opencode`), not by the diagnostics tests. Each adapter also
exports a `Descriptor` (display label, executable name, default enablement,
tested version range, machine-service support) that the platform reads for
settings, diagnostics and the UI; `internal/agents/*/descriptor_test.go` pins
those values to the tables below. See `docs/architecture/backend-boundaries.md`.

Tested environment: Go 1.27.1, Node 22, `opencode` 2.0.24, `pi` 1.1.0. No live
model provider was configured during verification (an opencode probe without
credentials returned 502), so everything that needs a real model call is
listed as provisional below.

## Tested version ranges

| Backend | Tested version | Supported range | Notes |
| --- | --- | --- | --- |
| `opencode-v2` | **2.0.24** | `[2.0.0, 3.0.0)` | One binary provides both the CLI and the v2 API server, so the CLI version is also the API version. |
| `pi` | **1.1.0** | `[1.0.0, 2.0.0)` | CLI (`--mode json`) or RPC (`--mode rpc`); the transport is selectable, CLI is the default. |

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
| `stop` | ✅ — drops the platform's reference to its instance; the machine-wide `opencode serve --service` itself is never stopped here (see *Machine-level service* below). | ✅ — RPC closes the session child's stdin for an orderly shutdown, bounded then backed by a process-group kill; CLI terminates the per-prompt process group. Verified. |
| `modelSelection` | ⚠️ — session-create `model` is accepted (verified); per-message switching via `session.switchModel` is **provisional** (see below). | ⚠️ — the `--model` flag is passed through (verified argv plumbing); real model resolution is **provisional**. |
| `toolCallEvents` | ✅ — `session.tool.*` normalized into tool call/result events. Verified against the mock server. | ✅ — `tool_execution_start/update/end` records. Verified against the fake CLI. |
| `toolConfirmation` | ❌ — the v2 API emits `permission.asked`, but the frozen `AgentAdapter` contract exposes no method to answer it, so no confirmation control is advertised. | ✅ in RPC mode — `extension_ui_request` `confirm` becomes a `PermissionRequest` (`agents.PermissionReplier`); the platform installs a bundled approval extension so pi actually asks. ❌ in CLI mode, which has no dialog channel. |
| `sessionHistory` | ✅ — `GET /api/session/{id}/message` read back and normalized (user text, assistant reasoning/text/tool blocks). Verified against the mock server. | ✅ — the adapter re-reads the session's own on-disk JSONL transcript. Verified against the fake CLI. |
| `rawEvents` | ✅ — the shared server's untouched SSE stream is proxied for the workbench's backend-specific views (`GET /api/projects/{id}/sessions/{sessionId}/raw-events`). | ❌ — no untouched event stream is exposed. |

`rawEvents` gates only that proxy. A backend that does not declare it gets
`unsupported_capability` on the raw-events route, and the workbench never
subscribes to it. Tool approval and question forms are not a separate flag:
they travel on the optional `agents.PermissionReplier` extension, and the
workbench shows them when `toolConfirmation` is declared.

### pi transports (CLI vs RPC)

The pi adapter supports two transports, selected per backend in Platform
Settings (`PUT /api/settings/agents/pi/mode`) and applied on restart:

| | `cli` (default) | `rpc` |
| --- | --- | --- |
| Process model | one `pi --mode json` process per prompt | one long-lived `pi --mode rpc` child per active session, lazily started and reclaimed after 10 idle minutes |
| Events | one-shot stdout JSONL | long-lived stdout JSONL: response records correlated by id, session events normalized the same way |
| Interrupt | kills the prompt's process group | `abort` command (process-group kill as backstop) |
| Stop | cancels and kills the process group | closes child stdin for an orderly shutdown, bounded then killed |
| Tool confirmation | unsupported | supported through the extension UI subprotocol and the bundled approval extension (`internal/agents/pi/extension/approval.ts`) |
| Same-session concurrency | parallel one-shot prompts | serialized per session; a prompt during a run is queued with `followUp` |

The CLI default is the rollback lever: if RPC misbehaves, switch the mode back
and restart — no deployment rollback. Both transports share the same session
files, so a transcript opened under one can be continued under the other.
Operator-supplied pi extensions can be added with the `PI_EXTENSIONS`
environment variable (comma-separated paths).

## Machine-level service (`opencode-v2`)

`opencode serve --service` is a **third-party process**, not a resource the
platform owns. One service serves the whole machine, and clients other than the
platform (the desktop `opencode` app, a CLI session) share it. The rules the
adapter enforces:

| Situation | Behaviour |
| --- | --- |
| Work on a backend whose service is down | Refused with `service_not_running` (HTTP 409). No session, instance, or other request starts the service as a side effect. |
| A registered, live service exists | Adopted, whatever started it — including one started before the platform booted, or by another client. |
| A registration names a dead process | Reported `stopped`; never relaunched from a business path. |
| Platform shutdown / restart | The service keeps running. Only the platform's own child processes (pi CLIs, per-instance servers) are reclaimed. |
| Operator starts or stops the service | Only through `GET/POST /api/agent-services/opencode-v2{,/start,/stop}`, driven by Platform Settings → Agent, and only after the operator confirms the consequence for other clients. |
| `service.json` | Owned by opencode. The platform reads it and never writes or deletes it. |

Two different checks back this: the business path uses the cheap one (read the
registration file, `kill(pid, 0)`), so a task request never waits on an HTTP
round trip; the settings page and diagnostics use the full probe (one
`/api/info` call with a bounded timeout) and report `running` or `unreachable`
with a reason.

`pi` has no machine-level service: `/api/agent-services/pi` answers
`supported: false`, and no start/stop control is shown for it.

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

History stays the agent's property: the platform never copies a transcript into
its own storage and never builds an index over one. What changed is that a task
can now **replay** it. When a task is opened the platform re-attaches the
session and asks the adapter for the prior turns through the optional
`agents.HistoryReader` operation (declared by `sessionHistory`); the adapter
reads the agent's own record and normalizes it into the same event envelope the
live stream uses, marked `replayed`. Reasoning, replies and tool activity all
become their own event kinds, so a replayed conversation shows what the user
watched. The workbench renders history first, then subscribes, and draws a
separator where the replay ends.

Each assistant message also names the model that produced it (`MessagePayload.Model`,
in the platform's `provider/model` form with the variant alongside). A replay
uses the model recorded on the message itself, so a conversation that switched
models mid-session replays accurately per message; a live assistant event
carries no model of its own, so the opencode adapter stamps it with the
session's current model, learned at session create/continue/list and refreshed
on every switch. Backends that do not report a per-message model (pi) simply
omit the field.

Because the read is scoped by working directory and agent-native session id
rather than by an instance, a task whose instance has stopped — or whose
platform was restarted — still replays.

| | `opencode-v2` | `pi` |
| --- | --- | --- |
| Source | `GET /api/session/{id}/message` | the session's JSONL transcript (`<session dir>/<directory slug>/<timestamp>_<id>.jsonl`, version 3) |
| Reasoning | `reasoning` blocks and the `session.reasoning.*` events become `thinking` events | `thinking` blocks and the `thinking_*` events become `thinking` events |
| Skipped | `idle` and `model-switched` entries | the `system` record, `model_change` and `thinking_level_change` |
| Tool activity | a `tool` block becomes a call/result pair (`state.input`, `state.output`, `state.status != "completed"` marks an error) | a `toolCall` block becomes the call; the `toolResult` record's `toolCallId`/`isError` link its result |

Reasoning is a first-class event kind rather than something the platform drops:
both backends expose it live (`session.reasoning.started/delta/ended` for
opencode, `thinking_start/delta/end` for pi, both verified against the installed
binaries) and in their stored records, so live and replay agree. No capability
flag gates it, because it drives no control — a backend that never reasons
simply never emits it. The workbench renders it as a collapsed block of its own
rather than folding it into the reply.

Reads are bounded by `agents.MaxHistoryEvents = 5000`: a reader keeps the most
recent events, renumbers them from one and reports the truncation through
`HistoryResult.Truncated` (surfaced as `truncated` on
`GET /api/projects/{id}/sessions/{sessionId}/history`), so the workbench can say
an older part was left out instead of presenting a partial transcript as
complete. A transcript whose last line is half-written ends the read with the
records already parsed and **without** reporting truncation — that line is the
turn currently being written, so nothing earlier was lost. An unreadable or
oversized record anywhere else does report truncation, because everything after
it is gone and the workbench must not present the remainder as the whole
conversation.

opencode's history read needs a running server, because that is where the
message log lives; the platform starts one if none is running before reading.
pi's read touches only the filesystem, so it needs no process at all.

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
