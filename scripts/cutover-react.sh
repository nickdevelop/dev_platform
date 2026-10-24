#!/usr/bin/env bash
# Cut production web/dist over to the React app while keeping the previous
# Vue build as a restorable backup. The Vue source under web/ stays untouched.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIST="$ROOT/web/dist"
BACKUP="$ROOT/web/dist.legacy-vue-backup"
REACT_DIST="$ROOT/web-next/dist"

if [ ! -d "$ROOT/web-next/node_modules" ]; then
  (cd "$ROOT/web-next" && npm ci)
fi
(cd "$ROOT/web-next" && npm run build)

if [ -d "$WEB_DIST" ] && [ ! -d "$BACKUP" ]; then
  cp -R "$WEB_DIST" "$BACKUP"
  echo "backed up Vue build -> $BACKUP"
fi

rm -rf "$WEB_DIST"
cp -R "$REACT_DIST" "$WEB_DIST"
test -f "$WEB_DIST/index.html" || { echo "missing web/dist/index.html" >&2; exit 1; }
grep -q '/assets/' "$WEB_DIST/index.html" || { echo "web/dist/index.html has no assets" >&2; exit 1; }
echo "cutover complete: web/dist now serves the React build"
