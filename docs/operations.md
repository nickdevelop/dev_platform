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

To build and install into a running checkout in one step, run `./deploy.sh`
(Linux/WSL): it builds through `cmd/devctl`, then copies `bin/platform` and
`web/dist` into the deployment directory (`/data/tools/devplatform` by
default, override with `DEPLOY_DIR` or `--dir`), backing up what it replaces
under `backups/<timestamp>/`. The data directory is never touched, and the
service is restarted through the deployment's `platform.sh` only when it was
already running — `--restart` / `--no-restart` force either way, and
`--build-only`, `--deploy-only`, `--skip-web` and `--verify` (probe
`/api/health`) cover the variants.

To produce distributable packages instead, run `./release.sh` (or
`make release`, Linux/WSL only): it builds the frontend once, then
cross-compiles the server for Linux (amd64, arm64) and Windows (amd64) and
writes self-contained archives to `release/`, each holding the binary, a
copied `web/dist`, and this guide, plus a `SHA256SUMS` file. Unpack a
package and run `./platform` (Windows: `platform.exe`) from its root — the
binary serves the bundled UI on `:8080` with no further configuration.
Pass `--version <label>` to name the artifacts, or `--targets "linux/amd64
windows/amd64"` to build a subset.

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

### Access logs and response times

Every HTTP request (API, static assets, SPA fallback, 404s) emits one
access line carrying its response time:

```
http access method=GET path=/api/projects status=200 latency=3.2ms ip=127.0.0.1
```

Filter slow or stuck operations by their `latency=` value, e.g.
`journalctl -u platform | grep "http access"`. Only the URL path is
logged — never the query string, request body, or headers — so tokens and
secrets cannot leak into logs (see Secret redaction below).

A request refused by the CSRF middleware carries an extra marker:

```
http access method=POST path=/api/projects status=403 latency=0.1ms ip=10.0.0.7 csrf=rejected
```

A steady stream of these from one address means a client is not echoing the
token (a script that forgot the header, or a page whose CSRF cookie expired).
The web UI attaches the header itself and recovers once automatically, so
`csrf=rejected` from a browser is normally followed by a successful retry.

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

## Windows (native)

Windows runs the same server binary from a console window: there is no
system-service registration, no NSSM and no Task Scheduler involved.
Start it in the foreground and stop it with `Ctrl+C` or by closing the
terminal window.

```powershell
# One-time setup: Node.js (LTS), then the agent CLIs
npm install -g @opencode/cli
npm install -g @earendil-works/pi-coding-agent

# Build once, then run in the foreground
go run ./cmd/devctl build
go run ./cmd/devctl run --listen 127.0.0.1:18080
```

WSL1 is not supported: the platform refuses to start there — upgrade to
WSL2 and run the Linux build. WSL1 不受支持：平台在 WSL1 下启动直接报错
退出，请升级到 WSL2 并运行 Linux 构建。A data directory under a DrvFs
mount (`/mnt/c` and friends) gets a startup warning: `flock` and mode bits
are unreliable there, so prefer a directory in the native WSL filesystem.
数据目录放在 `/mnt/c` 等 DrvFs 挂载点上时启动日志会告警，请把数据目录
放在 WSL 原生文件系统中。

### 数据目录

| 目录 | 位置 |
| --- | --- |
| 配置 | `%APPDATA%`（漫游） |
| 数据 | `%LOCALAPPDATA%`（本地） |
| 状态（含 `opencode/service.json` 的推导） | `$XDG_STATE_HOME`（若设置），否则 `%USERPROFILE%\.local\state` |

例如 `service.json` 默认位于
`C:\Users\<你>\.local\state\opencode\service.json`，与 Linux 的
`~/.local/state/opencode/service.json` 为同一规则。

### 权限降级

Windows 不做 ACL 收紧：`secrets.json`（明文）、数据库、锁文件与备份文件
对同机其他本地用户的隔离，依赖用户配置文件目录（`%APPDATA%` /
`%LOCALAPPDATA%`）的默认 ACL，而非平台自身。启动日志在每次启动时说明
该降级。请勿通过 `--data-dir` 把数据目录指向共享或同步目录——此时启动
日志会额外告警，但不会阻止启动。不对 `secrets.json` 加密（密钥管理超出
本次范围）。

### 关停与项目移除

- `Ctrl+C` 走正常优雅关停；直接关闭终端窗口时，平台拉起的子进程（pi、
  per-instance 服务）由 Job Object 随平台退出而回收（Windows 没有
  `SIGTERM`，优雅程度低于 Unix）。
- 平台退出（任何方式）都不影响机器级 `opencode serve --service`：它不是
  平台拉起的进程，启停只能在设置页由操作员决定。
- 移除项目（解除登记）只删除平台记录：先停止该项目的运行中 Agent 实例，
  再清除其实例与会话索引；任一实例停止失败则整体中止，项目保持登记。磁盘上
  的项目目录从不由平台删除，因此 Windows 与 Linux 行为一致。

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
  paths and versions. Only enabled agents can be chosen for new sessions;
  disabling never stops existing instances or invalidates existing sessions.

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

# 2b. Login rotates the CSRF token: re-read it from the cookie jar before
# the next mutation, otherwise every later write answers 403.
CSRF=$(awk '$6=="platform_csrf"{print $7}' cookies.txt)

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
  session. The web UI's HTTP transport attaches it to every write (no call
  site passes it) and refreshes it once when a token has expired; login and
  logout rotate it.
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
- The access log records only `method`, URL `path` (without the query
  string), response `status`, response `latency`, and client `ip`. Query
  strings, request bodies, and headers are never logged.

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
- **A request returns 403 `invalid_csrf_token`**: the `X-CSRF-Token` header
  did not match the `platform_csrf` cookie. In the browser this cannot happen
  by accident: the transport attaches the header and refreshes the token once
  on a rejection. Check, in order: (a) the request came from a script and the
  token was not re-read after login, which rotates it — see the
  copy-paste walkthrough; (b) the page has been open longer than the session
  TTL, so the cookie expired — reload it; (c) the request bypassed the
  transport (`fetch` directly) and sent no header. Every rejection is logged
  with `csrf=rejected`.
- **`single-instance locking requires a Unix platform`**: the binary was
  launched outside Linux/WSL; run it under Linux or WSL.
