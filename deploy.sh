#!/usr/bin/env bash
#
# deploy.sh — 构建 dev_platform 并部署到本机运行目录（Linux / WSL）。
#
# 构建流程复用跨平台的 cmd/devctl（前端 web/dist + Go 服务端 bin/platform），
# 然后把产物拷贝到部署目录，保持与既有运行方式（platform.sh）一致的布局：
#
#   <DEPLOY_DIR>/bin/platform        服务端可执行文件
#   <DEPLOY_DIR>/bin/web/dist/       前端静态资源（服务端同源托管）
#   <DEPLOY_DIR>/data/               数据目录（由 platform.sh 管理，脚本不触碰）
#   <DEPLOY_DIR>/platform.sh         启停脚本（若存在则用于重启服务）
#
# 部署前会把被替换的旧产物备份到 <DEPLOY_DIR>/backups/<时间戳>/，便于回滚。
# 服务管理：仅当部署时服务正在运行才会重启（未运行则保持停止）；也可用
# --restart / --no-restart 显式控制。
#
# Usage:
#   ./deploy.sh                    构建前端+服务端并部署（按需重启服务）
#   ./deploy.sh --skip-web         跳过前端构建，复用现有 web/dist
#   ./deploy.sh --build-only       只构建，不部署
#   ./deploy.sh --deploy-only      只部署现有构建产物（bin/platform + web/dist）
#   ./deploy.sh --no-restart       部署后不重启服务（即使服务正在运行）
#   ./deploy.sh --restart          部署后一定重启服务（即使当前未运行）
#   ./deploy.sh --verify           部署后探测 /api/health
#   ./deploy.sh help               显示本帮助
#
# Environment overrides:
#   DEPLOY_DIR       部署目录        (default: /data/tools/devplatform)
#   DEPLOY_WEB_SUB   前端相对 bin 的子路径 (default: web/dist)
#   SKIP_WEB         置 1 等价于 --skip-web
#   PLATFORM_SCRIPT  服务管理脚本    (default: $DEPLOY_DIR/platform.sh)
#
set -euo pipefail

# 无论从何处调用，都切到仓库根目录，保证 web/dist、bin/platform 等相对路径一致。
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$REPO_DIR"

DEPLOY_DIR="${DEPLOY_DIR:-/data/tools/devplatform}"
BIN_NAME="platform"
WEB_SUBDIR="${DEPLOY_WEB_SUB:-web/dist}"

BUILD_BIN="$REPO_DIR/bin/$BIN_NAME"
BUILD_WEB="$REPO_DIR/$WEB_SUBDIR"

DIST_BIN="$DEPLOY_DIR/bin/$BIN_NAME"
DIST_WEB="$DEPLOY_DIR/bin/$WEB_SUBDIR"
PLATFORM_SCRIPT="${PLATFORM_SCRIPT:-$DEPLOY_DIR/platform.sh}"

SKIP_WEB="${SKIP_WEB:-0}"
BUILD_ONLY=0
DEPLOY_ONLY=0
NO_RESTART=0
FORCE_RESTART=0
VERIFY=0

log() { printf '[deploy] %s\n' "$*"; }
die() { printf '[deploy] 错误: %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "缺少必需的命令: $1"
}

# ---- 构建 ------------------------------------------------------------------

# build 通过 devctl 依次构建前端与服务端（devctl 会在 node_modules 缺失时自动 npm ci）。
build() {
  require_cmd go
  if [ "$SKIP_WEB" = "1" ]; then
    [ -f "$BUILD_WEB/index.html" ] || die "--skip-web 指定了但 $WEB_SUBDIR 尚未构建"
    log "跳过前端构建，复用现有 $WEB_SUBDIR"
    go build -trimpath -ldflags "-s -w" -o "$BUILD_BIN" ./cmd/platform
  else
    go run ./cmd/devctl build
  fi
  [ -x "$BUILD_BIN" ] || die "构建产物缺失: bin/$BIN_NAME"
  [ -f "$BUILD_WEB/index.html" ] || die "前端产物缺失: $WEB_SUBDIR/index.html"
}

# ---- 服务管理（优先使用部署目录里的 platform.sh）----------------------------

# service_running 判断服务是否在运行（pid 文件或直接匹配进程）。
service_running() {
  if [ -x "$PLATFORM_SCRIPT" ] && [ -f "$DEPLOY_DIR/data/platform.pid" ]; then
    local pid
    pid="$(cat "$DEPLOY_DIR/data/platform.pid" 2>/dev/null || true)"
    [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null && return 0
    return 1
  fi
  pgrep -f "$DIST_BIN" >/dev/null 2>&1
}

restart_service() {
  if [ -x "$PLATFORM_SCRIPT" ]; then
    log "通过 platform.sh 重启服务"
    "$PLATFORM_SCRIPT" restart
  else
    log "未找到可执行的 $PLATFORM_SCRIPT，跳过服务管理"
    log "如需手动重启：$DEPLOY_DIR/platform.sh restart"
  fi
}

# verify_health 轮询健康检查接口（从 platform.sh 的运行记录里推断端口）。
verify_health() {
  require_cmd curl
  local addr host port url i
  addr="$(cat "$DEPLOY_DIR/data/platform.run" 2>/dev/null || true)"
  [ -n "$addr" ] || addr="${PLATFORM_LISTEN_ADDR:-:18080}"
  port="${addr##*:}"
  host="${addr%:*}"
  case "$host" in
    "" | "0.0.0.0" | "::" | "[::]") host="127.0.0.1" ;;
  esac
  url="http://${host}:${port}/api/health"

  for i in $(seq 1 40); do
    if curl -fsS -o /dev/null "$url" 2>/dev/null; then
      log "健康检查通过: $url"
      return 0
    fi
    sleep 0.5
  done
  log "健康检查失败: $url（请查看 $DEPLOY_DIR/data/platform.log）"
  return 1
}

# ---- 部署 ------------------------------------------------------------------

# backup 把即将被替换的旧产物存进 backups/<时间戳>/。
backup() {
  [ -e "$DIST_BIN" ] || [ -e "$DIST_WEB" ] || return 0
  local dir="$DEPLOY_DIR/backups/$(date +%Y%m%d-%H%M%S)"
  mkdir -p "$dir"
  if [ -e "$DIST_BIN" ]; then
    cp -a "$DIST_BIN" "$dir/$BIN_NAME"
    log "已备份旧二进制 -> ${dir#$DEPLOY_DIR/}/$BIN_NAME"
  fi
  if [ -d "$DIST_WEB" ]; then
    tar -czf "$dir/web-dist.tar.gz" -C "$(dirname "$DIST_WEB")" "$(basename "$DIST_WEB")"
    log "已备份旧前端资源 -> ${dir#$DEPLOY_DIR/}/web-dist.tar.gz"
  fi
}

deploy() {
  [ -d "$DEPLOY_DIR" ] || die "部署目录不存在: $DEPLOY_DIR（请先创建，或设置 DEPLOY_DIR）"
  mkdir -p "$DEPLOY_DIR/bin"

  # 二进制是运行中的文件，先移到 tmp 再拷贝新文件，避免 "text file busy"。
  if [ -e "$DIST_BIN" ]; then
    mv -f "$DIST_BIN" "$DIST_BIN.old"
    trap 'rm -f "$DIST_BIN.old"' EXIT
  fi
  install -m 0755 "$BUILD_BIN" "$DIST_BIN"
  rm -f "$DIST_BIN.old"
  trap - EXIT
  log "已部署二进制 -> $DIST_BIN"

  # 前端资源整目录替换（先写新目录再原子改名，减少空窗期）。
  local web_root tmp
  web_root="$(dirname "$DIST_WEB")"
  tmp="$web_root/.dist.new"
  mkdir -p "$web_root"
  rm -rf "$tmp"
  mkdir -p "$tmp"
  cp -a "$BUILD_WEB/." "$tmp/"
  if [ -d "$DIST_WEB" ]; then
    mv -f "$DIST_WEB" "$web_root/.dist.old"
    mv -f "$tmp" "$DIST_WEB"
    rm -rf "$web_root/.dist.old"
  else
    mv -f "$tmp" "$DIST_WEB"
  fi
  log "已部署前端资源 -> $DIST_WEB"
}

usage() {
  awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
}

# ---- 参数解析 ---------------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    --skip-web) SKIP_WEB=1; shift ;;
    --build-only) BUILD_ONLY=1; shift ;;
    --deploy-only) DEPLOY_ONLY=1; shift ;;
    --no-restart) NO_RESTART=1; shift ;;
    --restart) FORCE_RESTART=1; shift ;;
    --verify) VERIFY=1; shift ;;
    --dir) [ $# -ge 2 ] || die "--dir 需要一个参数"; DEPLOY_DIR="$2"
      DIST_BIN="$DEPLOY_DIR/bin/$BIN_NAME"
      DIST_WEB="$DEPLOY_DIR/bin/$WEB_SUBDIR"
      PLATFORM_SCRIPT="${PLATFORM_SCRIPT:-$DEPLOY_DIR/platform.sh}"
      shift 2 ;;
    -h | --help | help) usage; exit 0 ;;
    *) die "未知参数: $1（见 ./deploy.sh help）" ;;
  esac
done

require_cmd tar

log "仓库目录: $REPO_DIR"
log "部署目录: $DEPLOY_DIR"

if [ "$DEPLOY_ONLY" = "1" ]; then
  [ -x "$BUILD_BIN" ] || die "构建产物缺失: bin/$BIN_NAME（请先构建或去掉 --deploy-only）"
  [ -f "$BUILD_WEB/index.html" ] || die "前端产物缺失: $WEB_SUBDIR/index.html"
  log "跳过构建，使用现有产物"
elif [ "$BUILD_ONLY" = "1" ]; then
  build
  log "仅构建完成（未部署）：bin/$BIN_NAME + $WEB_SUBDIR"
  exit 0
else
  build
fi

was_running=0
service_running && was_running=1

backup
deploy

if [ "$NO_RESTART" = "1" ]; then
  log "按 --no-restart 要求，未重启服务"
elif [ "$FORCE_RESTART" = "1" ] || [ "$was_running" = "1" ]; then
  restart_service
  [ "$VERIFY" = "1" ] && verify_health
else
  log "服务当前未运行，保持停止（需启动：$DEPLOY_DIR/platform.sh start）"
fi

log "部署完成 ✅"
[ "$VERIFY" = "1" ] || log "如需健康探测可加 --verify；查看日志：$DEPLOY_DIR/platform.sh log -f"
