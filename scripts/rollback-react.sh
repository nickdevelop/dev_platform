#!/usr/bin/env bash
# Restore the Vue build captured by scripts/cutover-react.sh.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKUP="$ROOT/web/dist.legacy-vue-backup"
WEB_DIST="$ROOT/web/dist"

if [ ! -d "$BACKUP" ]; then
  echo "no backup at $BACKUP; rebuild the Vue app with: (cd web && npm ci && npm run build)" >&2
  exit 1
fi
rm -rf "$WEB_DIST"
cp -R "$BACKUP" "$WEB_DIST"
test -f "$WEB_DIST/index.html" || { echo "rollback produced no index.html" >&2; exit 1; }
echo "rollback complete: web/dist restored from $BACKUP"
