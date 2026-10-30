#!/usr/bin/env bash
#
# release.sh — build distributable web (server) and desktop packages (Linux / WSL).
#
# Two families of artifacts are written to release/, all covered by one
# SHA256SUMS file:
#
#   web    one self-contained package per server target: the platform server
#          binary plus the built web UI (web/dist) and the operations guide, so
#          the extracted package runs with its defaults — the binary finds
#          ./web/dist next to itself and serves the UI on :8080.
#   desktop  the Electron app from desktop/ packaged by electron-builder
#          (Linux: AppImage + tar.gz, Windows: zip). Each package bundles the
#          matching server sidecar (bin/platform*) and web/dist, so the
#          installed app can run its own local backend out of the box.
#
# The frontend is built once (it is plain static files, shared by every
# package); only the Go server is cross-compiled per target. SQLite is pure Go
# (modernc), so CGO is disabled and no cross toolchain is needed.
#
# Usage:
#   ./release.sh                    Build web + desktop packages -> release/
#   ./release.sh v0.2.0             Build with an explicit version label
#   ./release.sh --targets "linux/amd64 windows/amd64"
#   ./release.sh --desktop-targets "linux"      Only the Linux desktop app
#   ./release.sh --skip-desktop     Web packages only
#   ./release.sh --skip-server      Desktop packages only
#   ./release.sh --skip-web         Reuse the existing web/dist build
#   ./release.sh --clean            Remove release/ before building
#   ./release.sh --list             Show the default targets and exit
#   ./release.sh help               Show this help
#
# Environment overrides:
#   RELEASE_VERSION          Version label       (default: git describe, else "dev")
#   RELEASE_TARGETS          Space-separated GOOS/GOARCH pairs for web packages
#   RELEASE_DESKTOP_TARGETS  Space-separated desktop OS names (linux, windows)
#   RELEASE_DESKTOP_VERSION  Desktop app version (default: the release label when
#                            it is a semantic version, else desktop/package.json)
#   RELEASE_DIR              Output directory    (default: release)
#
set -euo pipefail

# Always operate from the repository root so relative paths (web/dist,
# cmd/platform, desktop/, release/) resolve the same regardless of invocation
# directory.
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$REPO_DIR"

OUT_DIR="${RELEASE_DIR:-release}"
OUT="$REPO_DIR/$OUT_DIR"
DEFAULT_TARGETS="linux/amd64 linux/arm64 windows/amd64"
DEFAULT_DESKTOP_TARGETS="linux windows"
TARGETS="${RELEASE_TARGETS:-$DEFAULT_TARGETS}"
DESKTOP_TARGETS="${RELEASE_DESKTOP_TARGETS:-$DEFAULT_DESKTOP_TARGETS}"
DESKTOP_VERSION="${RELEASE_DESKTOP_VERSION:-}"
VERSION="${RELEASE_VERSION:-}"
SKIP_WEB=0
SKIP_SERVER=0
SKIP_DESKTOP=0
CLEAN=0
STAGING=""
STAMP=""
ARTIFACTS=()

# Progress goes to stderr so command substitutions can capture results cleanly.
log() { printf '[release] %s\n' "$*" >&2; }
die() { printf '[release] error: %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

cleanup() {
  if [ -n "$STAGING" ]; then rm -rf "$STAGING"; fi
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
# The same build feeds the web packages and the desktop app's bundled UI.
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
# the web package: <stage>/<name>/{platform[.exe], web/dist, OPERATIONS.md}.
stage_target() {
  local target="$1" version="$2"
  local goos="${target%%/*}" goarch="${target##*/}"
  local name="devplatform-${version}-${goos}-${goarch}"
  local bin="platform"
  [ "$goos" = "windows" ] && bin="platform.exe"

  local dest="$STAGING/$name"
  mkdir -p "$dest/web"

  log "build web package $name ($goos/$goarch)"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w" -o "$dest/$bin" ./cmd/platform

  # web/dist sits next to the binary so the default -web-dir resolution finds it.
  cp -R web/dist "$dest/web/dist"
  [ -f docs/operations.md ] && cp docs/operations.md "$dest/OPERATIONS.md"

  printf '%s\n' "$name"
}

# write_archives packages each staged web directory into release/ and records
# the archive names for the checksum step.
write_archives() {
  local -a names=("$@")
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
    ARTIFACTS+=("$name.$ext")
  done
}

# add_artifact records an artifact name once, so a name produced (or picked up
# again) by later steps never lands in SHA256SUMS twice.
add_artifact() {
  local name="$1" existing
  for existing in ${ARTIFACTS[@]+"${ARTIFACTS[@]}"}; do
    [ "$existing" = "$name" ] && return 0
  done
  ARTIFACTS+=("$name")
}

# ensure_desktop_deps installs the Electron app dependencies when missing.
ensure_desktop_deps() {
  require_cmd npm
  if [ ! -d desktop/node_modules ]; then
    log "install desktop dependencies"
    if [ -f desktop/package-lock.json ]; then
      (cd desktop && npm ci)
    else
      (cd desktop && npm install)
    fi
  fi
  [ -x desktop/node_modules/.bin/electron-builder ] \
    || die "electron-builder missing (run: cd desktop && npm install)"
}

# build_sidecar compiles the server binary electron-builder bundles as an extra
# resource (resources/bin/platform[.exe]) for one desktop OS. The package
# layout matches the web packages: web/dist is bundled next to the sidecar, so
# a local (self-hosted) desktop connection can start its own backend.
build_sidecar() {
  local os="$1" goos goarch bin
  case "$os" in
    linux) goos="linux"; goarch="$(go env GOARCH)"; bin="platform" ;;
    windows) goos="windows"; goarch="amd64"; bin="platform.exe" ;;
    *) die "unsupported desktop target \"$os\" (want linux or windows)" ;;
  esac

  mkdir -p bin
  # electron-builder bundles everything matching bin/platform*, so drop stale
  # sidecars of other platforms to keep each package lean.
  find bin -maxdepth 1 -type f \( -name 'platform' -o -name 'platform.exe' \) \
    ! -name "$bin" -delete
  log "build desktop sidecar bin/$bin ($goos/$goarch)"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w" -o "bin/$bin" ./cmd/platform
}

# resolve_desktop_version picks the desktop app version: RELEASE_DESKTOP_VERSION
# wins, then the release label when it is a plain semantic version (so
# `./release.sh v0.2.0` names every artifact 0.2.0), then desktop/package.json.
resolve_desktop_version() {
  local v="$1" semver
  if [ -n "$v" ]; then
    printf '%s' "$v"
    return
  fi
  semver="${VERSION#v}"
  if [[ "$semver" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
    printf '%s' "$semver"
    return
  fi
  sed -n 's/^[[:space:]]*"version":[[:space:]]*"\([^"]*\)".*/\1/p' desktop/package.json | head -n 1
}

# package_desktop packages the desktop app for one OS. Linux uses the targets
# from desktop/electron-builder.yml (AppImage + tar.gz); Windows gets a zip
# because the nsis installer needs Wine when cross-built on Linux.
package_desktop() {
  local os="$1" dver="$2"
  local -a args=() config_args=()
  case "$os" in
    linux) args=(--linux) ;;
    windows) args=(--win zip) ;;
    *) die "unsupported desktop target \"$os\" (want linux or windows)" ;;
  esac
  [ -n "$dver" ] && config_args=(-c.extraMetadata.version="$dver")

  build_sidecar "$os"

  log "build desktop app ($os, version ${dver:-from desktop/package.json})"
  (cd desktop && ./node_modules/.bin/electron-builder "${args[@]}" "${config_args[@]}")
}

# collect_desktop_artifacts copies the installers/archives produced by this run
# (anything newer than the run stamp) from desktop/dist-package into release/.
collect_desktop_artifacts() {
  local f name
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    name="$(basename "$f")"
    cp -f "$f" "$OUT/$name"
    add_artifact "$name"
  done < <(find desktop/dist-package -maxdepth 1 -type f -newer "$STAMP" \
    \( -name '*.AppImage' -o -name '*.tar.gz' -o -name '*.zip' \
    -o -name '*.dmg' -o -name '*.deb' -o -name '*.exe' \) | sort)
}

# write_sums records digests for every artifact produced in this run.
write_sums() {
  if [ "${#ARTIFACTS[@]}" -eq 0 ]; then
    log "warning: no artifacts were built, SHA256SUMS not written"
    return
  fi
  log "write SHA256SUMS"
  (cd "$OUT" && sha256sum "${ARTIFACTS[@]}" >SHA256SUMS)
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
    --targets | --server-targets)
      [ $# -ge 2 ] || die "$1 needs a value"
      TARGETS="$2"; shift 2 ;;
    --desktop-targets)
      [ $# -ge 2 ] || die "--desktop-targets needs a value"
      DESKTOP_TARGETS="$2"; shift 2 ;;
    --desktop-version)
      [ $# -ge 2 ] || die "--desktop-version needs a value"
      DESKTOP_VERSION="$2"; shift 2 ;;
    --skip-web) SKIP_WEB=1; shift ;;
    --skip-server) SKIP_SERVER=1; shift ;;
    --skip-desktop) SKIP_DESKTOP=1; shift ;;
    --clean) CLEAN=1; shift ;;
    --list)
      printf 'web:     %s\n' "$TARGETS"
      printf 'desktop: %s\n' "$DESKTOP_TARGETS"
      exit 0 ;;
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
if [ "$SKIP_SERVER" = "1" ]; then
  log "web targets:     (skipped)"
else
  log "web targets:     $TARGETS"
fi
if [ "$SKIP_DESKTOP" = "1" ]; then
  log "desktop targets: (skipped)"
else
  log "desktop targets: $DESKTOP_TARGETS"
fi
log "output:          $OUT_DIR/"

[ "$CLEAN" = "1" ] && rm -rf "$OUT"
mkdir -p "$OUT"

if [ "$SKIP_WEB" = "1" ]; then
  [ -f web/dist/index.html ] || die "--skip-web given but web/dist is not built"
  log "reusing existing web/dist"
else
  build_web
fi

STAGING="$(mktemp -d "${TMPDIR:-/tmp}/devplatform-release.XXXXXX")"
STAMP="$STAGING/.stamp"
: >"$STAMP"

if [ "$SKIP_DESKTOP" != "1" ] && [ -z "$DESKTOP_TARGETS" ]; then
  die "--desktop-targets is empty but desktop packaging was not skipped"
fi

if [ "$SKIP_DESKTOP" != "1" ]; then
  ensure_desktop_deps
  log "build desktop TypeScript -> desktop/dist"
  (cd desktop && npm run build)
fi

if [ "$SKIP_SERVER" != "1" ]; then
  names=()
  for target in $TARGETS; do
    goos="${target%%/*}"
    goarch="${target##*/}"
    [ "$goos" = "$target" ] || [ -z "$goos" ] || [ -z "$goarch" ] \
      && die "invalid target \"$target\" (want GOOS/GOARCH, e.g. linux/amd64)"
    names+=("$(stage_target "$target" "$VERSION")")
  done
  write_archives "${names[@]}"
fi

if [ "$SKIP_DESKTOP" != "1" ]; then
  DESKTOP_VERSION="$(resolve_desktop_version "$DESKTOP_VERSION")"
  [ -n "$DESKTOP_VERSION" ] || die "cannot determine the desktop app version"
  for os in $DESKTOP_TARGETS; do
    package_desktop "$os" "$DESKTOP_VERSION"
  done
  collect_desktop_artifacts
fi

write_sums

log "done — artifacts in $OUT_DIR/:"
(cd "$OUT" && ls -1)
