# Resource management and configuration sync

This document is the explicit mapping for Skill, MCP and prompt resources:
where each backend's configuration is written, how conflicts are detected and
resolved, where credentials live, and what the guarantees and limits are.

The code that owns this behavior:

- `internal/resources/` — canonical definitions, project associations and
  session prompt selection.
- `internal/secrets/` — the file-backed MCP credential store.
- `internal/resourcesync/` — managed-file ownership, preview, conflict
  detection, backup and the sync coordinator.
- `internal/resourcesync/adapters/` — the per-backend renderers.
- `web/src/components/ResourcesSection.vue` and
  `web/src/components/resources/` — the project settings UI.

## Resource kinds

| Kind | Scope | Notes |
| --- | --- | --- |
| `skill` | project (per backend) | A reusable skill body. |
| `mcp` | project (per backend) | An MCP server definition; its credential is an opaque `secretRef`. |
| `prompt` | project or session | A project-level instruction is file-synchronized; a session prompt is selected per session and is not file-synchronized. |

A resource is associated with one or more projects. Only associated resources
are synchronized; a resource with an empty backend list applies to every
registered backend adapter.

## Synchronization targets and formats

Targets are project-relative, slash-separated paths. The trailing extension is
chosen by the adapter. These are the platform's managed formats; they are
deliberately simple and deterministic so the ownership, drift and backup
machinery is independent of the native file shape.

| Backend | Kind | Target path | Format |
| --- | --- | --- | --- |
| project | `prompt` | `.devplatform/prompts/<name>.md` | Markdown: `# <name>`, optional description, body. |
| `opencode-v2` | `prompt` | `.opencode/prompts/<name>.md` | Markdown. |
| `opencode-v2` | `skill` | `.opencode/skills/<name>.md` | Markdown. |
| `opencode-v2` | `mcp` | `.opencode/mcp/<name>.json` | JSON with `name`, `server` (the neutral definition) and, when a credential is set, `credentials.token`. |
| `pi` | `prompt` | `.pi/prompts/<name>.md` | Markdown. |
| `pi` | `skill` | `.pi/skills/<name>.md` | Markdown. |
| `pi` | `mcp` | `.pi/mcp/<name>.json` | JSON with `serverName`, `definition` and, when a credential is set, `env.MCP_TOKEN`. |

A resource name is a single path segment: no separators, no traversal, at
most 128 characters, Unicode allowed. The joined target path is prefix-checked
against the project directory, so a target can never escape the project.

> **Verification status.** The opencode v2 and pi file formats above are the
> platform's managed formats, not yet verified against a live installation's
> native configuration loader. When a backend's native format is confirmed,
> only the adapter's `Render` function changes; ownership, preview, conflict
> and backup behavior stay unchanged.

## Ownership, consent and conflicts

The platform only auto-maintains files it created or that a user explicitly
adopted.

- **First apply requires consent.** A preview lists each resource/target pair.
  Applying a pair whose target does not exist creates it and records ownership.
- **Unmanaged target.** If the target already exists and the platform does not
  own it, the pair is an `unmanaged-target` conflict and is not touched until
  the user resolves it.
- **External drift.** After the platform owns a target, it records the hash of
  the content it wrote. If the file is edited outside the platform, the hash
  no longer matches and the pair becomes an `external-drift` conflict; later
  changes pause instead of overwriting.
- **Unwritable target.** A target that is a symlink or a directory is a
  `target-invalid` conflict.
- **Resolution.** The user explicitly chooses to overwrite the target with the
  platform content. Every write first backs up the previous content, so the
  last known-good version is preserved.
- **Post-consent updates.** Once a target is owned, changing the resource
  re-renders and updates it automatically; a conflict still pauses the sync.

The coordinator reports one result per resource/target pair, so one failing
target never hides or aborts another pair's result.

## Recovery

- Every write is atomic: content is written to a temp file in the target
  directory, synced, then renamed over the target. A crash never leaves a
  partial target.
- Leftover temp files from an interrupted write are removed before the next
  write; the target keeps its last known-good content until the new write is
  installed.
- If the write or the ownership update fails, the target is rolled back to the
  backup (or removed if it did not exist before).
- Targets are tracked case-sensitively, matching Linux/WSL filesystems; two
  names differing only in case are distinct targets.

## Credentials

MCP credential **values** are stored separately from the database and from
project content, in `<data-dir>/secrets.json`:

- The file is created with mode `0600` and its directory with `0700` on
  Linux/WSL. Values are written atomically without widening permissions.
- A symlink planted at the secret path is refused, not followed, for both
  reads and writes.
- The API exposes only an opaque `secretRef`; the value is write-only. The UI
  never fetches or renders a stored value, and the store never logs it.
- A value is resolved only when an adapter renders a target that needs it, and
  adapter errors name the reference, never the value.

**Plaintext-at-rest risk.** The secret file is plaintext. Owner-only
permissions protect it from other local users, but they do **not** protect it
from a process running as the same user (or with higher privileges), nor from
a backup that includes the file. A resolved value is also written into the
managed target file the agent reads; that file is project content and carries
the same same-user caveat. Treat the platform data directory and any synced
configuration as sensitive.

## Session prompts

Session-scoped prompts are selected per session through
`/api/projects/{id}/sessions/{sessionId}/prompts`; they are stored as a
session resource selection and are not file-synchronized. Project-scoped
prompts are file-synchronized like other resources.
