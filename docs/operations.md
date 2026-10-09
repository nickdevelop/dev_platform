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
| `platform.db`   | SQLite database (password hash, sessions, workspace roots, projects) |
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
