# Platform operations guide (Linux / WSL)

How to build, run, expose on the LAN, and back up the AI development
platform service. The supported deployment targets are Linux and Windows
WSL; the service is one Go binary with an embedded SQLite database.

## Prerequisites

- Linux (any recent distribution) or Windows WSL2. WSL counts as Linux.
- Go 1.24+ to build the server.
- Node 22 + npm only to build the frontend. The built UI is plain static
  files; the running service needs no Node.
- No external database: SQLite is embedded.

## Build

From the repository root:

```sh
make web-install   # once, or after dependency changes
make web-build     # produces web/dist
make build         # produces bin/platform
```

`make verify` runs the full check suite (go vet, Go tests, frontend unit
tests, frontend build).

## First run: set the platform password

The platform has a single shared operator password. Configure it once per
data directory, from an interactive terminal:

```sh
platform set-password                 # default data directory
platform set-password -data-dir /srv/platform-data
```

The password is read from the TTY without echo. It is never accepted from
command-line arguments, the environment, or pipelines — `set-password`
refuses them — and it is never logged. Re-running `set-password` does not
overwrite an existing password.

After starting the service, register a project by its absolute path, or create
one under any directory the platform process can write to. Projects are no
longer limited to authorized roots.

## Start and stop

```sh
platform                              # defaults: data dir per below, :8080, UI if built
platform -data-dir /srv/platform-data -listen :8080 -web-dir /srv/platform-ui
```

Stop the service with `SIGINT` (Ctrl+C) or `SIGTERM` (`kill <pid>`). Both
trigger a graceful shutdown: the listener closes, in-flight requests finish
(10 second budget), then the database closes. The process exits 0.

For a long-running deployment, a minimal systemd unit:

```ini
[Unit]
Description=AI development platform
After=network.target

[Service]
ExecStart=/opt/platform/bin/platform -data-dir /srv/platform-data -listen :8080 -web-dir /opt/platform/web/dist
Environment=PLATFORM_DATA_DIR=/srv/platform-data
Restart=on-failure
User=platform

[Install]
WantedBy=multi-user.target
```

Logs go to stdout/stderr; journald captures them under systemd
(`journalctl -u platform`).

### One instance per data directory

A single-instance lock (an advisory `flock` on `<data-dir>/platform.lock`)
ensures only one platform service process uses a data directory. A second
process pointing at the same data directory refuses to start with:

```
platform: runtime: data directory /srv/platform-data is already in use by
another platform process
```

Different data directories may run concurrently. The lock is released on
graceful shutdown; a crashed process cannot pin it (the OS drops it when
the process dies). The zero-byte `platform.lock` file remains on disk; that
is normal.

## The settings center

Open **设置** (Settings) from the left rail. It has three sections:

- **外观 (Appearance)** — theme, font, font size and interface density. The
  configuration is shared platform state and is applied on the next page load;
  other already-open pages pick it up on refresh.
- **模型 (Models)** — OpenAI-compatible model services and their models, with
  "fetch models" (browse a service's `/models` and tick the ones to add) and a
  per-model connectivity test. API keys live in the platform-private secret
  file (`0600`) and are never returned by the API; an empty key on save clears
  the stored one.
- **Agent** — enable/disable OpenCode and Pi Agent and detect their executable
  paths and versions. Only enabled agents can be chosen for new tasks;
  disabling never stops existing instances or invalidates existing tasks.

## Register or create a project

Project paths are no longer restricted to authorized roots. Register any
absolute existing directory, or create a new one under any absolute parent the
platform process can write to. The process's own operating-system file
permissions are the only boundary; paths are still canonicalized and relative
or ambiguous inputs are rejected.

### From the web UI

1. Log in and open the projects view.
2. Click **注册** (Register) and enter an absolute path, or **新建** (Create)
   and enter a parent directory plus a name.

### Copy-paste fallback (no browser)

Management mutations require an authenticated session and the CSRF token, so
log in first. The following uses `cookies.txt` for the session and CSRF
cookies:

```sh
BASE=http://localhost:8080

# 1. Fetch a CSRF token (public endpoint; sets the platform_csrf cookie).
curl -s -c cookies.txt "$BASE/api/auth/session" >/dev/null
CSRF=$(awk '$6=="platform_csrf"{print $7}' cookies.txt)

# 2. Log in (the password is never written to disk or logs).
read -rs PASSWORD
curl -s -b cookies.txt -c cookies.txt -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" \
  -d "{\"password\":\"$PASSWORD\"}"
unset PASSWORD

# 3. Register an existing project directory by absolute path.
curl -s -b cookies.txt -X POST "$BASE/api/projects" \
  -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" \
  -d '{"path":"/home/you/projects/demo"}'

# 4. Create a new project directory under any absolute parent.
curl -s -b cookies.txt -X POST "$BASE/api/projects/create" \
  -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" \
  -d '{"parent":"/home/you/projects","name":"new-app"}'

# 5. Read the appearance and agent configuration.
curl -s -b cookies.txt "$BASE/api/settings/appearance"
curl -s -b cookies.txt "$BASE/api/agents"
```

`POST /api/projects` returns `201` with the canonicalized path.

## Configuration reference

Every setting is configurable by flag, then environment variable, then
default:

| Setting      | Flag        | Environment variable | Default |
|--------------|-------------|----------------------|---------|
| Data dir     | `-data-dir` | `PLATFORM_DATA_DIR`  | `$XDG_CONFIG_HOME/devplatform` (`~/.config/devplatform`), falling back to `./devplatform-data` |
| Listen addr  | `-listen`   | `PLATFORM_LISTEN_ADDR` | `:8080` |
| Web UI dir   | `-web-dir`  | `PLATFORM_WEB_DIR`   | `./web/dist` when it contains a built `index.html`, otherwise static hosting off |

## LAN binding

The default `:8080` binds every interface, so the service is reachable from
the LAN at `http://<host>:8080`. Bind loopback only (not LAN-reachable) with
`-listen 127.0.0.1:8080`. Pick a different port when 8080 is taken:

```sh
PLATFORM_LISTEN_ADDR=:9090 platform   # or: platform -listen :9090
```

Open the port in the host firewall when needed:

```sh
sudo ufw allow 8080/tcp
```

Security properties that hold on any bind address:

- Every management API (`/api/projects`, `/api/workspaces`, `/api/auth/*`
  mutations) requires a session; only `/api/health` and the login/session
  endpoints are public.
- Mutating requests require the double-submit CSRF token issued with the
  session.
- The login endpoint is throttled per client address and always answers
  generically (it never reveals whether a password is configured).
- No proxy headers (`X-Forwarded-*`) are trusted; client addressing uses
  the connection's remote address.

The service targets plain-HTTP LAN use. If you must expose it beyond a
trusted LAN, put it behind a TLS-terminating reverse proxy that forwards to
a loopback bind.

> **Login throttle behind a proxy.** The per-client login throttle keys on
> the connection's remote address, and no `X-Forwarded-*` headers are
> trusted. Behind a TLS-terminating reverse proxy that shares one connection
> address, every client therefore collapses into a single throttle key: one
> client's failed logins can (temporarily) lock out all clients, and the
> per-attacker limit no longer isolates clients. When fronting the service
> with a proxy, keep login attempts low and monitor the auth endpoint, or
> terminate TLS at a proxy that preserves the client address and have the
> proxy apply its own rate limit.

## Production UI serving (same origin)

When a built UI is available (`web/dist` or `-web-dir`/`PLATFORM_WEB_DIR`),
the Go service serves it from the same origin as the API:

- `/api/...` routes always take precedence; unknown API paths return JSON
  404s, never HTML.
- Hashed bundles under `/assets/` are served with a permanent immutable
  cache lifetime.
- Any other path serves the SPA entry (`index.html`), so client-side routes
  like `/login` survive a hard refresh.
- File access is confined to the web directory; traversal requests cannot
  escape it.

Without a built UI the service runs as a pure API on the same port.

## Data, logs, and backups

The data directory contains:

| File            | Purpose |
|-----------------|---------|
| `platform.db`   | SQLite database (password hash, sessions, projects, model definitions, settings) |
| `platform.db-wal`, `platform.db-shm` | SQLite WAL sidecars; deleted automatically on clean shutdown |
| `platform.lock` | single-instance lock file (zero bytes, stays behind) |

The platform only ever writes inside its data directory (plus project
directories it manages under authorized workspace roots).

Logs are the process's stdout/stderr (no log files are written). Under
systemd they land in journald; under `nohup`/screen they go wherever the
operator redirects them.

Back up by copying the whole data directory while the service is stopped.
For a live backup use SQLite's consistent snapshot instead of a raw copy:

```sh
sqlite3 /srv/platform-data/platform.db ".backup '/backups/platform-$(date +%F).db'"
```

Restore by stopping the service, replacing the data directory contents, and
starting again. Rotating sessions and the password hash travel with the
database; a restored backup invalidates sessions issued after it.

## Secret redaction

- The database logger never emits bound SQL values: queries are logged with
  `?` placeholders only, so session tokens, password hashes, and workspace
  paths are not leaked into slow-query or error logs.
- The password never appears in argv, environment, configuration files, or
  logs; it exists only as a prompt-accepted TTY input and as a bcrypt hash
  in the database.
- Session identifiers live in an HttpOnly cookie and are never echoed in
  API response bodies (only session metadata and the CSRF token are).
- There is no request-logging middleware, so query strings and bodies are
  not logged either.

Protect server logs like credentials anyway: keep journald access
restricted, and avoid redirecting logs to world-readable files.

## Troubleshooting

- **Port already in use** (e.g. a docker-proxy holds 8080): the process
  exits with `listen on :8080: ... address already in use`. Choose another
  port with `-listen`/`PLATFORM_LISTEN_ADDR`.
- **"data directory ... is already in use"**: another platform process is
  running against the same data directory. Stop it, or point this process
  at a different `-data-dir`.
- **"web UI assets not found"** at `/`: the configured web directory has no
  built `index.html`. Run `make web-build`, or point `-web-dir` at an
  existing build.
- **Login returns 403 `invalid_csrf_token`**: the client dropped the CSRF
  cookie or header; reload the page so the app fetches a session first.
- **`single-instance locking requires a Unix platform`**: the binary was
  launched outside Linux/WSL; run it under Linux or WSL.
