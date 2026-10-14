#!/usr/bin/env bash
#
# release.sh — build distributable Linux and Windows packages (Linux / WSL).
#
# Produces one self-contained package per target: the platform server binary
# plus the built web UI (web/dist) and the operations guide, so the extracted
# package runs with its defaults — the binary finds ./web/dist next to itself
# and serves the UI on :8080. Archives are written to the release/ directory
# together with a SHA256SUMS file.
#
# The frontend is built once (it is plain static files, shared by every
# target); only the Go server is cross-compiled per target. SQLite is pure Go
# (modernc), so CGO is disabled and no cross toolchain is needed.
#
# Usage:
#   ./release.sh                    Build every default target -> release/
#   ./release.sh v0.2.0             Build with an explicit version label
#   ./release.sh --targets "linux/amd64 windows/amd64"
#   ./release.sh --skip-web         Reuse the existing web/dist build
#   ./release.sh --clean            Remove release/ before building
#   ./release.sh --list             Show the default targets and exit
#   ./release.sh help               Show this help
#
# Environment overrides:
#   RELEASE_VERSION   Version label       (default: git describe, else "dev")
#   RELEASE_TARGETS   Space-separated GOOS/GOARCH pairs
#   RELEASE_DIR       Output directory    (default: release)
#
set -euo pipefail

# Always operate from the repository root so relative paths (web/dist,
# cmd/platform, release/) resolve the same regardless of invocation directory.
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$REPO_DIR"

OUT_DIR="${RELEASE_DIR:-release}"
OUT="$REPO_DIR/$OUT_DIR"
DEFAULT_TARGETS="linux/amd64 linux/arm64 windows/amd64"
TARGETS="${RELEASE_TARGETS:-$DEFAULT_TARGETS}"
VERSION="${RELEASE_VERSION:-}"
SKIP_WEB=0
CLEAN=0
STAGING=""

# Progress goes to stderr so command substitutions can capture results cleanly.
log() { printf '[release] %s\n' "$*" >&2; }
die() { printf '[release] error: %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

cleanup() {
  [ -n "$STAGING" ] && rm -rf "$STAGING"
}
trap cleanup EXIT

# resolve_version derives the version label: the explicit value wins, then
# git describe (tags when present, else the short hash), then "dev". Anything
# that would be awkward in a filename is replaced with "-".
resolve_version() {
  local v="$1"
  if [ -z "$v" ]; then
    v="$(git describe --tags --always --dirty 2>/dev/null || true)"
  fi
  [ -n "$v" ] || v="dev"
  printf '%s' "$v" | sed 's#[^A-Za-z0-9._-]#-#g'
}

# build_web installs frontend dependencies when missing, then builds web/dist.
build_web() {
  require_cmd npm
  if [ ! -d web/node_modules ]; then
    log "install frontend dependencies"
    (cd web && npm ci)
  fi
  log "build frontend -> web/dist"
  (cd web && npm run build)
  [ -f web/dist/index.html ] || die "web/dist/index.html missing after frontend build"
}

# stage_target cross-compiles the server for one GOOS/GOARCH pair and lays out
# the package: <stage>/<name>/{platform[.exe], web/dist, OPERATIONS.md}.
stage_target() {
  local target="$1" version="$2"
  local goos="${target%%/*}" goarch="${target##*/}"
  local name="devplatform-${version}-${goos}-${goarch}"
  local bin="platform"
  [ "$goos" = "windows" ] && bin="platform.exe"

  local dest="$STAGING/$name"
  mkdir -p "$dest/web"

  log "build $name ($goos/$goarch)"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w" -o "$dest/$bin" ./cmd/platform

  # web/dist sits next to the binary so the default -web-dir resolution finds it.
  cp -R web/dist "$dest/web/dist"
  [ -f docs/operations.md ] && cp docs/operations.md "$dest/OPERATIONS.md"

  printf '%s\n' "$name"
}

# write_archives packages each staged directory into release/ and records the
# archive digests in SHA256SUMS.
write_archives() {
  local -a names=("$@")
  local -a archives=()
  local name ext
  for name in "${names[@]}"; do
    case "$name" in
      *-windows-*) ext="zip" ;;
      *) ext="tar.gz" ;;
    esac
    log "package $name.$ext"
    if [ "$ext" = "zip" ]; then
      require_cmd zip
      (cd "$STAGING" && zip -qr -X "$OUT/$name.zip" "$name")
    else
      require_cmd tar
      tar -czf "$OUT/$name.tar.gz" -C "$STAGING" "$name"
    fi
    archives+=("$(basename "$name.$ext")")
  done

  (cd "$OUT" && sha256sum "${archives[@]}" >SHA256SUMS)
}

usage() {
  awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
}

# ---- argument parsing -------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      [ $# -ge 2 ] || die "--version needs a value"
      VERSION="$2"; shift 2 ;;
    --targets)
      [ $# -ge 2 ] || die "--targets needs a value"
      TARGETS="$2"; shift 2 ;;
    --skip-web) SKIP_WEB=1; shift ;;
    --clean) CLEAN=1; shift ;;
    --list) printf '%s\n' $TARGETS; exit 0 ;;
    -h | --help | help) usage; exit 0 ;;
    -*) die "unknown option: $1 (see ./release.sh help)" ;;
    *) VERSION="$1"; shift ;;
  esac
done

# ---- run --------------------------------------------------------------------

require_cmd go
require_cmd git

VERSION="$(resolve_version "$VERSION")"
log "version: $VERSION"
log "targets: $TARGETS"
log "output:  $OUT_DIR/"

[ "$CLEAN" = "1" ] && rm -rf "$OUT"
mkdir -p "$OUT"

if [ "$SKIP_WEB" = "1" ]; then
  [ -f web/dist/index.html ] || die "--skip-web given but web/dist is not built"
  log "reusing existing web/dist"
else
  build_web
fi

STAGING="$(mktemp -d "${TMPDIR:-/tmp}/devplatform-release.XXXXXX")"

names=()
for target in $TARGETS; do
  goos="${target%%/*}"
  goarch="${target##*/}"
  [ "$goos" = "$target" ] || [ -z "$goos" ] || [ -z "$goarch" ] \
    && die "invalid target \"$target\" (want GOOS/GOARCH, e.g. linux/amd64)"
  names+=("$(stage_target "$target" "$VERSION")")
done

write_archives "${names[@]}"

log "done — artifacts in $OUT_DIR/:"
(cd "$OUT" && ls -1)
