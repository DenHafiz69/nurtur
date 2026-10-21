#!/usr/bin/env bash
# Rebuild and restart Nurtur when source files change.
# Deliberately dependency-free: no watcher binary required.
set -euo pipefail

cd "$(dirname "$0")/.."

export NURTUR_LOCAL_ONLY="${NURTUR_LOCAL_ONLY:-true}"
BIN="${TMPDIR:-/tmp}/nurtur-dev"
PID=0

fingerprint() {
  {
    find assets.go go.mod go.sum cmd internal web migrations -type f \
      \( -name '*.go' -o -name '*.html' -o -name '*.css' -o -name '*.js' -o -name '*.sql' -o -name '*.woff2' \) \
      -printf '%T@ %p\n' 2>/dev/null
    # Restart when the local dotenv changes too.
    if [ -f .env ]; then
      printf '%s %s\n' "$(stat -c '%y' .env)" ".env"
    fi
  } | sort | sha256sum | cut -d' ' -f1
}

stop_server() {
  if [ "$PID" -ne 0 ]; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
    PID=0
  fi
}

start_server() {
  if ! go build -o "$BIN" ./cmd/nurtur; then
    echo "build failed — watching for changes"
    return
  fi
  "$BIN" &
  PID=$!
}

cleanup() {
  stop_server
  exit 0
}
trap cleanup INT TERM

LAST="$(fingerprint)"
start_server

while true; do
  sleep 1
  CURRENT="$(fingerprint)"
  if [ "$CURRENT" != "$LAST" ]; then
    LAST="$CURRENT"
    echo "change detected — restarting"
    stop_server
    start_server
  fi
done
