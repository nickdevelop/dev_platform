#!/usr/bin/env bash
#
# dev.sh — development helper for the AI development platform (Linux / WSL).
#
# Builds the frontend and the Go server, then runs the server locally. The
# server binds 0.0.0.0:18081 by default so the UI is reachable from the LAN
# (matching the platform's plain-HTTP LAN deployment model).
#
# build and run delegate to the cross-platform cmd/devctl tool (usable on
# Windows as `go run ./cmd/devctl build|run`); the background start/stop/
# restart/status/logs commands below stay bash-only for Linux/WSL.
#
# Usage:
#   ./dev.sh build             Build the frontend (web/dist) and server (bin/platform)
#   ./dev.sh run               Run the server in the foreground (Ctrl+C stops it)
#   ./dev.sh dev [vite args]   Run the frontend dev server (Vite HMR) against
#                              a locally running backend — the backend is NOT
#                              started here (./dev.sh run answers on 18080,
#                              the proxy default). The dev server binds every
#                              interface (LAN reachable, like the server),
#                              never loopback only; extra arguments go straight
#                              to Vite, e.g. `./dev.sh dev --port 3000`
#   ./dev.sh start [--build]   Start the server; builds first when the binary is
#                              missing or --build is given
#   ./dev.sh restart [--build] Stop, then start (rebuild first with --build)
#   ./dev.sh stop              Gracefully stop the running server
#   ./dev.sh status            Report whether the server is running
#   ./dev.sh logs              Follow the server log
#   ./dev.sh help              Show this help
#
# Environment overrides:
#   DEV_LISTEN_ADDR   Listen address           (default 0.0.0.0:18081)
#   DEV_DATA_DIR      Platform data directory  (default: the platform's own:
#                                               $PLATFORM_DATA_DIR, else the
#                                               user config dir)
#   DEV_WEB_DIR       Built web assets dir     (default web/dist)
#   DEV_API_PROXY     Backend the Vite dev server proxies /api to when running
#                     in dev mode (default http://127.0.0.1:18080, which is
#                     what ./dev.sh run binds by default; ./dev.sh start uses
#                     18081, so override this variable for it)
#   DEV_UI_PORT       Vite dev server port     (default Vite's own 5173)
#   DEV_UI_HOST       Vite dev server bind address (default 0.0.0.0: the dev
#                     server follows the platform's plain-HTTP LAN model, so it
#                     is reachable from other machines on the network; set
#                     127.0.0.1 or ::1 to keep it loopback only)
#
set -euo pipefail

# Always operate from the repository root so relative paths (web/dist,
# bin/platform) resolve the same regardless of where the script is invoked.
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$REPO_DIR"

BINARY="bin/platform"
WEB_DIR="${DEV_WEB_DIR:-web/dist}"
LISTEN_ADDR="${DEV_LISTEN_ADDR:-0.0.0.0:18081}"
DATA_DIR="${DEV_DATA_DIR:-}"
# Frontend dev mode (`dev`): the Vite dev server proxies same-origin /api to
# this backend, which must already be running (see the dev() function).
API_PROXY="${DEV_API_PROXY:-http://127.0.0.1:18080}"
UI_PORT="${DEV_UI_PORT:-}"
# Bind address of the dev server. Deliberately not loopback-only: the platform
# is a plain-HTTP LAN service, so the dev UI has to be reachable from other
# machines (phones, another laptop) the same way the built UI is. Vite's own
# default is `localhost`, which on a dual-stack host binds ::1 only — so the
# address is passed explicitly instead of left to the tool.
UI_HOST="${DEV_UI_HOST:-0.0.0.0}"

# Runtime state (PID + log) lives in an ignored directory so it never shows up
# in git status and `make clean` cannot delete a running server's PID file.
RUN_DIR=".dev"
PID_FILE="$RUN_DIR/dev.pid"
LOG_FILE="$RUN_DIR/dev.log"

log() { printf '[dev] %s\n' "$*"; }
die() { printf '[dev] error: %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

# read_pid echoes the recorded PID, or nothing when there is none.
read_pid() {
  [ -f "$PID_FILE" ] || return 0
  local pid
  pid="$(cat "$PID_FILE" 2>/dev/null || true)"
  [ -n "$pid" ] && printf '%s' "$pid"
}

# is_running succeeds while the recorded process is alive.
is_running() {
  local pid
  pid="$(read_pid)"
  [ -n "$pid" ] || return 1
  kill -0 "$pid" 2>/dev/null
}

# build compiles the frontend then the Go server via the cross-platform
# devctl tool (no make or npm knowledge needed here).
build() {
  go run ./cmd/devctl build
}

# run executes the server in the foreground via devctl; Ctrl+C stops it.
run() {
  go run ./cmd/devctl run "$@"
}

# dev runs the frontend in development mode (Vite dev server with HMR) and
# proxies same-origin /api to the already-running backend at $API_PROXY. The
# backend is deliberately left alone: this command never starts or stops it, so
# a running session, agent instance or the machine-wide opencode service stays
# untouched while the UI reloads.
#
# It binds $UI_HOST (0.0.0.0 by default, never loopback-only) so the dev UI is
# reachable on the LAN like the built UI; Vite's own default `localhost` binds
# ::1 only on a dual-stack host, so the address is always passed explicitly.
dev() {
  require_cmd npm
  if [ ! -d web/node_modules ]; then
    log "installing frontend dependencies"
    (cd web && npm ci)
  fi

  local -a vite_args=(--host "$UI_HOST")
  [ -n "$UI_PORT" ] && vite_args+=(--port "$UI_PORT")
  # Any remaining CLI arguments go straight to Vite and win over the defaults
  # above (a later --host / --port replaces the one passed here).
  vite_args+=("$@")

  log "vite dev server on $UI_HOST (LAN reachable), /api proxied to $API_PROXY — Ctrl+C stops it"
  log "the backend is not started here: ./dev.sh run answers on 18080 by default"
  # `--` forwards the args to vite; DEV_API_PROXY is read by web/vite.config.ts.
  (
    cd web
    DEV_API_PROXY="$API_PROXY" npm run dev -- ${vite_args[@]+"${vite_args[@]}"}
  )
}

# probe_host extracts a loopback-reachable host:port pair from the configured
# listen address, so the health check does not depend on binding to 0.0.0.0.
probe_host_port() {
  local addr="$1" host port
  host="${addr%:*}"
  port="${addr##*:}"
  case "$host" in
    "" | "0.0.0.0" | "::" | "[::]") host="127.0.0.1" ;;
  esac
  printf '%s %s' "$host" "$port"
}

# wait_for_health polls /api/health until the server answers or it dies.
wait_for_health() {
  local hp host port url i
  hp="$(probe_host_port "$LISTEN_ADDR")"
  host="${hp% *}"
  port="${hp#* }"
  url="http://${host}:${port}/api/health"

  for i in $(seq 1 60); do
    if curl -fsS -o /dev/null "$url" 2>/dev/null; then
      return 0
    fi
    if ! is_running; then
      return 1
    fi
    sleep 0.5
  done
  return 1
}

# start launches the server in the background and waits until it is healthy.
start() {
  local force_build="${1:-0}"
  require_cmd curl

  if is_running; then
    log "already running (pid $(read_pid)) on $LISTEN_ADDR"
    return 0
  fi
  # Clear a stale PID file left behind by an unclean exit.
  rm -f "$PID_FILE"

  if [ "$force_build" = "1" ] || [ ! -x "$BINARY" ]; then
    build
  fi

  mkdir -p "$RUN_DIR"

  local -a args=(-listen "$LISTEN_ADDR" -web-dir "$WEB_DIR")
  [ -n "$DATA_DIR" ] && args+=(-data-dir "$DATA_DIR")

  log "starting server on $LISTEN_ADDR (log: $LOG_FILE)"
  # nohup + disown so the server outlives this script; the platform handles
  # SIGTERM/SIGINT with a graceful shutdown.
  nohup "$BINARY" "${args[@]}" >>"$LOG_FILE" 2>&1 &
  local pid=$!
  disown "$pid" 2>/dev/null || true
  printf '%s' "$pid" >"$PID_FILE"

  if wait_for_health; then
    log "server ready (pid $pid) — http://<host>:${LISTEN_ADDR##*:}/"
  else
    log "server failed to become healthy; last log lines:"
    tail -n 20 "$LOG_FILE" >&2 || true
    stop >/dev/null 2>&1 || true
    return 1
  fi
}

# stop signals the server and waits for a graceful exit before forcing it.
stop() {
  if ! is_running; then
    log "not running"
    rm -f "$PID_FILE"
    return 0
  fi

  local pid
  pid="$(read_pid)"
  log "stopping server (pid $pid)"
  kill -TERM "$pid" 2>/dev/null || true

  local i
  for i in $(seq 1 30); do
    if ! kill -0 "$pid" 2>/dev/null; then
      rm -f "$PID_FILE"
      log "stopped"
      return 0
    fi
    sleep 0.5
  done

  log "graceful shutdown timed out; sending SIGKILL"
  kill -KILL "$pid" 2>/dev/null || true
  rm -f "$PID_FILE"
  log "stopped"
}

status() {
  if is_running; then
    log "running (pid $(read_pid)) on $LISTEN_ADDR"
  else
    log "not running"
    return 1
  fi
}

logs() {
  [ -f "$LOG_FILE" ] || die "no log file at $LOG_FILE (has the server ever started?)"
  tail -n 50 -f "$LOG_FILE"
}

usage() {
  # Print the leading comment banner (skipping the shebang) up to the first
  # non-comment line, with the leading "# " stripped.
  awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
}

cmd="${1:-help}"
shift || true

case "$cmd" in
  build)
    build
    ;;
  run)
    run "$@"
    ;;
  dev)
    dev "$@"
    ;;
  start)
    force=0
    [ "${1:-}" = "--build" ] && force=1
    start "$force"
    ;;
  restart)
    force=0
    [ "${1:-}" = "--build" ] && force=1
    stop
    start "$force"
    ;;
  stop)
    stop
    ;;
  status)
    status
    ;;
  logs)
    logs
    ;;
  help | -h | --help)
    usage
    ;;
  *)
    usage
    exit 1
    ;;
esac
