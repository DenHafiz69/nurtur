#!/bin/sh
# Pull the current Nurtur release and recreate only that Compose service.
#
# Installed on the VPS as /usr/local/bin/nurtur-update and run by
# nurtur-update.timer. Safe to run by hand with:
#
#   sudo systemctl start nurtur-update.service
#
# The image tag is expected to be the moving "stable" release channel. The
# script never touches the database volume or the other services in the stack.
set -eu

COMPOSE_FILE="${NURTUR_COMPOSE_FILE:-/srv/nurtur/docker-compose.yml}"
SERVICE="${NURTUR_COMPOSE_SERVICE:-nurtur}"
DOCKER="${DOCKER:-$(command -v docker || true)}"

log() {
  printf 'nurtur-update: %s\n' "$*"
}

die() {
  printf 'nurtur-update: %s\n' "$*" >&2
  exit 1
}

[ -r "$COMPOSE_FILE" ] || die "cannot read $COMPOSE_FILE"
[ -n "$DOCKER" ] && [ -x "$DOCKER" ] || die "docker is not executable"

log "pulling $SERVICE from $COMPOSE_FILE"
"$DOCKER" compose -f "$COMPOSE_FILE" pull --quiet "$SERVICE"

# --no-deps keeps the rest of the stack alone. Compose is a no-op when the
# image digest and service configuration have not changed.
"$DOCKER" compose -f "$COMPOSE_FILE" up -d --no-deps "$SERVICE"

container="$("$DOCKER" compose -f "$COMPOSE_FILE" ps -q "$SERVICE")"
if [ -n "$container" ]; then
  log "waiting for $SERVICE to become healthy"
  tries=0
  status=unknown
  while [ "$tries" -lt 30 ]; do
    status="$("$DOCKER" inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container" 2>/dev/null || echo unknown)"
    case "$status" in
      healthy|running|exited|dead) break ;;
    esac
    tries=$((tries + 1))
    sleep 2
  done
  case "$status" in
    healthy|running) log "$SERVICE is $status" ;;
    *) die "$SERVICE is $status after the update; inspect docker compose logs" ;;
  esac
else
  die "compose did not report a container for service $SERVICE"
fi

# Remove dangling layers left by old releases. Images still referenced by a
# running or stopped container are kept.
"$DOCKER" image prune -f >/dev/null 2>&1 || true

log "update check complete"
