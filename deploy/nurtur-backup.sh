#!/bin/sh
# Take a consistent snapshot of the Nurtur data directory.
#
# Installed on the VPS as /usr/local/bin/nurtur-backup and run by
# nurtur-backup.timer. The service is stopped only for the few seconds it takes
# to copy the database, so the WAL is checkpointed before the archive is made.
#
# Run it by hand with:
#
#   sudo systemctl start nurtur-backup.service
#
# Archives stay under NURTUR_BACKUP_DIR. This script does not copy them off the
# VPS; see DEPLOYMENT.md for the offsite step.
set -eu

COMPOSE_FILE="${NURTUR_COMPOSE_FILE:-/srv/nurtur/docker-compose.yml}"
SERVICE="${NURTUR_COMPOSE_SERVICE:-nurtur}"
BACKUP_DIR="${NURTUR_BACKUP_DIR:-/var/backups/nurtur}"
KEEP_DAYS="${NURTUR_BACKUP_KEEP_DAYS:-14}"
DOCKER="${DOCKER:-$(command -v docker || true)}"

log() {
  printf 'nurtur-backup: %s\n' "$*"
}

die() {
  printf 'nurtur-backup: %s\n' "$*" >&2
  exit 1
}

[ -r "$COMPOSE_FILE" ] || die "cannot read $COMPOSE_FILE"
[ -n "$DOCKER" ] && [ -x "$DOCKER" ] || die "docker is not executable"
case "$KEEP_DAYS" in
  ''|*[!0-9]*) die "NURTUR_BACKUP_KEEP_DAYS must be a whole number" ;;
esac
[ "$KEEP_DAYS" -ge 1 ] || die "NURTUR_BACKUP_KEEP_DAYS must be at least 1"

container="$("$DOCKER" compose -f "$COMPOSE_FILE" ps -q --all "$SERVICE")"
[ -n "$container" ] || die "compose did not report a container for service $SERVICE"

data_dir="$("$DOCKER" inspect -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Source}}{{end}}{{end}}' "$container")"
[ -n "$data_dir" ] || die "could not find the /data mount for $SERVICE"
[ -d "$data_dir" ] || die "data directory $data_dir does not exist"

install -d -m 700 "$BACKUP_DIR"

stamp="$(date -u +%Y-%m-%dT%H%M%SZ)"
part="$BACKUP_DIR/.nurtur-$stamp.tgz.part"
final="$BACKUP_DIR/nurtur-$stamp.tgz"
stopped=0

restart() {
  if [ "$stopped" -eq 1 ]; then
    "$DOCKER" compose -f "$COMPOSE_FILE" start "$SERVICE" >/dev/null || true
    stopped=0
  fi
}

cleanup() {
  rm -f "$part"
  restart
}

trap cleanup EXIT HUP INT TERM

was_running="$("$DOCKER" inspect -f '{{.State.Running}}' "$container")"
if [ "$was_running" = "true" ]; then
  log "stopping $SERVICE for a clean snapshot"
  "$DOCKER" compose -f "$COMPOSE_FILE" stop "$SERVICE"
  stopped=1
else
  log "$SERVICE is already stopped; copying the current data directory"
fi

log "writing $final"
tar czf "$part" -C "$data_dir" .
chmod 600 "$part"
mv "$part" "$final"

# Restart before pruning so a failed restart does not leave the app down while
# the timer considers the backup complete.
restart
trap - EXIT HUP INT TERM

if [ "$was_running" = "true" ]; then
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
    *) die "$SERVICE is $status after the snapshot; inspect docker compose logs" ;;
  esac
fi

find "$BACKUP_DIR" -maxdepth 1 -type f -name 'nurtur-*.tgz' -mtime +"$KEEP_DAYS" -delete
log "backup complete: $final"
