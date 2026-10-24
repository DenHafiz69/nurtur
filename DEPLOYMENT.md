# Deployment

How to run Nurtur on a VPS using the image built by CI, alongside an existing
Docker Compose stack and Caddy reverse proxy.

CI publishes `ghcr.io/denhafiz69/nurtur` on every push to `main` and every `v*`
tag. `latest` follows `main`; `stable` advances only when a `v*` tag is pushed,
which makes it the right moving channel for a server that updates itself.
Runtime configuration stays on the VPS — nothing in `.env-example` belongs in
GitHub secrets. The workflow only uses the automatic `GITHUB_TOKEN` to push the
image.

## What you need

- A VPS with Docker and the Compose plugin
- Your existing `docker-compose.yml` and Caddy setup
- A DNS record such as `nurtur.example.com` pointing at the VPS
- Ports 80 and 443 reachable; port 8080 does **not** need to be public

## 1. Create the runtime config

Put the secrets outside the repository:

```sh
sudo install -d -m 755 /etc/nurtur
sudo install -m 600 -o root -g root /dev/null /etc/nurtur/nurtur.env
sudo editor /etc/nurtur/nurtur.env
```

`/etc/nurtur/nurtur.env`:

```sh
# Inside the container: listen on all interfaces and keep the DB on /data.
NURTUR_ADDR=:8080
NURTUR_DB=/data/nurtur.db
NURTUR_TZ=Asia/Kuala_Lumpur
TZ=Asia/Kuala_Lumpur

# Local-only mode. The database lives on the bind mount described below.
# For offsite sync, set this to false and provide both Turso values.
NURTUR_LOCAL_ONLY=true
# TURSO_DATABASE_URL=libsql://your-db.turso.io
# TURSO_AUTH_TOKEN=
# Optional pull cadence. Local writes still push within a few seconds.
# NURTUR_SYNC_INTERVAL=5m

# Required to create local accounts. Generate fresh values:
#   openssl rand -hex 16   (signup key)
#   openssl rand -hex 32   (session secret)
NURTUR_SIGNUP_KEY=replace-with-a-long-random-value
NURTUR_SESSION_SECRET=replace-with-a-long-random-value

# Optional admin screen at /admin. Sign in as "admin" with this password.
# NURTUR_ADMIN_PASSWORD=replace-with-an-8-plus-character-password
# Public origin used in generated invite links (important behind a proxy).
# NURTUR_PUBLIC_URL=https://nurtur.example.com

# true because Caddy serves HTTPS. Use false only for plain-HTTP testing.
NURTUR_COOKIE_SECURE=true

NURTUR_NIGHT_START=19:00
NURTUR_NIGHT_END=07:00
NURTUR_LOG_LEVEL=info
```

Do not reuse `.env-example` unchanged in the container: `NURTUR_DB=./data/nurtur.db`
resolves to `/data/data/nurtur.db` because the image's working directory is
`/data`. Use the absolute path above.

Compose reads `env_file` as the user invoking it. If the file is root-only, run
Compose with `sudo`, or make it `640 root:docker` and run as a user in the
`docker` group. The `docker` group is root-equivalent; root-only plus `sudo` is
the safer default.

Keep `NURTUR_SESSION_SECRET` stable. Changing it invalidates existing sessions.
Anyone who knows `NURTUR_SIGNUP_KEY` can create an account; each account gets
its own private baby profile and care log. Remove the key to close new signups
without affecting existing accounts.

To invite people without sharing the signup key, set `NURTUR_ADMIN_PASSWORD`
and open `/admin`. Sign in as `admin`, generate a magic link, and send it to the
person. The link opens signup directly, works once, and expires after the
period you choose (7 days by default). Set `NURTUR_PUBLIC_URL` to the exact
public origin so generated links work behind Caddy.

## 2. Add Nurtur to your Compose stack

Add this service to the **existing** `docker-compose.yml` that already runs your
other services. Merge the `volumes` section if you already have one.

```yaml
services:
  nurtur:
    image: ghcr.io/denhafiz69/nurtur:stable
    restart: unless-stopped
    env_file:
      - /etc/nurtur/nurtur.env
    environment:
      # Soft ceiling for the Go heap, kept below the container limit.
      GOMEMLIMIT: 48MiB
      GOGC: "50"
    volumes:
      - /srv/nurtur/data:/data
    read_only: true
    tmpfs:
      - /tmp
    security_opt:
      - no-new-privileges:true
    stop_grace_period: 35s
    deploy:
      resources:
        limits:
          memory: 64M
    expose:
      - "8080"
```

`env_file` keeps the secrets out of the YAML itself, so the Compose file stays
safe to copy around or commit. If you prefer the inline `environment:` style
used by your other services, reference shell variables (`NURTUR_SIGNUP_KEY:
"${NURTUR_SIGNUP_KEY}"`) and export them from a root-only source instead of
pasting the values in.

Create the data directory and give it to the container user (UID/GID 10001):

```sh
sudo install -d -o 10001 -g 10001 -m 700 /srv/nurtur/data
```

The image runs as non-root and needs to write `nurtur.db` plus its WAL files
there. A bind mount is used here because it makes backups and inspection
straightforward; a named volume works too.

Do **not** add a `ports:` mapping when Caddy is in the same Compose project.
Both services share the default network, so Caddy can reach `nurtur:8080`
directly. Publishing the port would expose it on the host unnecessarily.

If your Caddy service already joins a named network (for example `web`), add
Nurtur to that same network. Compose's implicit shared network only applies to
services that do not declare their own network membership.

If Caddy runs on the host instead of in Compose, replace `expose` with:

```yaml
    ports:
      - "127.0.0.1:8005:8080"
```

If Caddy lives in a **different** Compose project, attach both services to a
shared network:

```yaml
services:
  caddy:
    networks: [web]

  nurtur:
    networks: [web]

networks:
  web:
    external: true
```

Create it once with `docker network create web`, and keep the Caddyfile target
as `nurtur:8080`.

### Container hardening

- `read_only: true` makes the image filesystem immutable. The only writable
  paths are the `/data` bind mount and `/tmp`. The Turso Go library loads its
  prebuilt native engine through `purego` and unpacks it into
  `TURSO_GO_CACHE_DIR=/tmp/turso-go`, so the `tmpfs` mount is required for the
  local database to open at all — not just for sync.
- `no-new-privileges:true` blocks setuid escalation. The process already runs as
  UID 10001.
- `GOMEMLIMIT` is a soft limit for the Go heap, not a hard cap. Set it around
  70–75% of the container limit so the runtime still has room for stacks,
  goroutine buffers, and the OS page cache. `GOGC: "50"` collects earlier than
  the default `100`, trading CPU for a smaller resident set. `GOMEMLIMIT` alone
  is enough; drop `GOGC` if startup feels sluggish on a tiny VPS.
- `deploy.resources.limits` is honored by Compose v2 outside Swarm. On Compose
  v1, replace it with `mem_limit: 64m`.

64M is a starting point for a small install. Watch
`docker stats nurtur` for a day, then lower the container limit and
`GOMEMLIMIT` together until the idle footprint plus headroom fits. Never set
`GOMEMLIMIT` above the container limit: the kernel OOM killer still wins, and
the runtime will not collect aggressively enough to avoid it.

The image already defines a `HEALTHCHECK`, so Compose reports `(healthy)`
without any extra configuration.

### Private image (optional)

This guide assumes the GHCR package is public, so the VPS needs no registry
login. If the package is private, log in on the VPS with a token that has
`read:packages`:

```sh
echo "$GHCR_TOKEN" | docker login ghcr.io -u DenHafiz69 --password-stdin
```

The image is built for `linux/amd64`.

## 3. Point Caddy at it

Add to your existing Caddyfile:

```
nurtur.example.com {
    encode zstd gzip
    reverse_proxy nurtur:8080
}
```

Use `reverse_proxy 127.0.0.1:8005` instead if Caddy runs on the host. Validate
and reload:

```sh
docker compose exec caddy caddy validate --config /etc/caddy/Caddyfile
docker compose exec caddy caddy reload --config /etc/caddy/Caddyfile
```

With Caddy on the host, use `sudo caddy validate` / `sudo systemctl reload caddy`
instead. Caddy obtains and renews the certificate automatically once DNS points
at the VPS.

Firewall: allow 80/443, and leave 8080/8005 closed.

```sh
sudo ufw allow 80,443/tcp
```

## 4. Start and onboard

```sh
cd /path/to/your/compose
sudo docker compose pull nurtur
sudo docker compose up -d nurtur
sudo docker compose logs -f nurtur
```

Wait for the log line indicating the server is listening. Check health from
inside the container:

```sh
sudo docker compose exec nurtur curl -fsS http://127.0.0.1:8080/healthz
sudo docker compose exec nurtur curl -fsS 'http://127.0.0.1:8080/healthz?verbose=1'
```

Then open `https://nurtur.example.com`, enter `NURTUR_SIGNUP_KEY`, choose a
username/password, and set the baby's name. Repeat with the same key for each
person who needs their own account. Or set `NURTUR_ADMIN_PASSWORD`, open
`/admin`, and send each person a one-use magic link instead. Onboarding writes
the account and baby name into the database; later logins use that
username/password and only see that account's data.

If login appears to succeed but you are bounced back to the sign-in page, check
`NURTUR_COOKIE_SECURE`: it must be `true` over HTTPS and `false` only when
testing over plain HTTP.

## 5. Updates

The VPS can follow the `stable` channel on its own. CI only moves `stable` when
a `v*` tag is pushed, so a normal push to `main` never deploys automatically.

### Automatic updates with a systemd timer

First switch the Nurtur service to the moving channel and remove any local
build block:

```yaml
    image: ghcr.io/denhafiz69/nurtur:stable
    # no build: block
```

Then run the new image once by hand:

```sh
cd /path/to/your/compose
sudo docker compose pull nurtur
sudo docker compose up -d nurtur
```

The repository ships the timer, service, and updater script in `deploy/`.
Run these from a checkout of the repository (on the VPS or copied over):

```sh
git clone --depth 1 https://github.com/DenHafiz69/nurtur.git /tmp/nurtur-deploy
cd /tmp/nurtur-deploy

sudo install -m 755 deploy/nurtur-update.sh /usr/local/bin/nurtur-update
sudo install -d -m 755 /etc/nurtur
sudo install -m 644 deploy/nurtur-update.env.example /etc/nurtur/update.env
sudo editor /etc/nurtur/update.env
sudo install -m 644 deploy/nurtur-update.service /etc/systemd/system/
sudo install -m 644 deploy/nurtur-update.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now nurtur-update.timer
rm -rf /tmp/nurtur-deploy
```

Set these two values in `/etc/nurtur/update.env`:

```sh
NURTUR_COMPOSE_FILE=/srv/nurtur/docker-compose.yml
NURTUR_COMPOSE_SERVICE=nurtur
```

`NURTUR_COMPOSE_FILE` is the standalone Compose file that contains the Nurtur
service. `NURTUR_COMPOSE_SERVICE` is the Compose service name, not the
`container_name`.

The timer runs three minutes after boot and then every fifteen minutes. It:

1. pulls the current `stable` image;
2. recreates only the Nurtur service with `up -d --no-deps`;
3. waits for the container health check;
4. removes dangling layers from old releases.

Run a check immediately instead of waiting:

```sh
sudo systemctl start nurtur-update.service
sudo journalctl -u nurtur-update.service -n 50 --no-pager
```

Check the timer itself with:

```sh
systemctl list-timers nurtur-update.timer
```

Pause automatic updates while doing maintenance or restoring a backup:

```sh
sudo systemctl disable --now nurtur-update.timer
```

Re-enable with `sudo systemctl enable --now nurtur-update.timer`.

### Releasing

```sh
git tag v0.3.8
git push origin v0.3.8
```

CI publishes the semver tag and moves `stable`; the VPS picks it up on the next
timer tick. If the new container fails its health check, the unit exits with an
error and the failure is visible in `journalctl`. Migrations are forward-only,
so keep the backup schedule from §6 running; rolling the image back does not
undo a schema change.

### Manual update

If the timer is disabled, the old command still works:

```sh
cd /path/to/your/compose
sudo docker compose pull nurtur
sudo docker compose up -d nurtur
```

`stable` moves only on release tags. If you need a specific build, pin it:

```yaml
    image: ghcr.io/denhafiz69/nurtur:0.3.8
```

Remember to return the service to `stable` before relying on the timer again.

## 6. Backups and restore

### Do I already have backups?

The repository does not install a backup schedule by itself. Unless the VPS
already has a cron job, systemd timer, restic/borg job, or provider snapshot,
you only have the manual commands below.

Check the VPS:

```sh
systemctl list-timers --all | grep -Ei 'backup|restic|borg|nurtur'
sudo ls -lah /var/backups/nurtur 2>/dev/null
sudo crontab -l
command -v restic borg rclone duplicity
```

A VPS provider snapshot is useful for disaster recovery, but it is not a
database backup: restoring the whole VPS is slower and loses everything written
after the snapshot. Keep at least one copy off the VPS.

### Automatic local snapshots

The repository ships an optional backup script, service, and timer in
`deploy/`. Install them from a checkout of the repository:

```sh
git clone --depth 1 https://github.com/DenHafiz69/nurtur.git /tmp/nurtur-deploy
cd /tmp/nurtur-deploy

sudo install -m 755 deploy/nurtur-backup.sh /usr/local/bin/nurtur-backup
sudo install -d -m 755 /etc/nurtur
sudo install -m 644 deploy/nurtur-backup.env.example /etc/nurtur/backup.env
sudo editor /etc/nurtur/backup.env
sudo install -m 644 deploy/nurtur-backup.service /etc/systemd/system/
sudo install -m 644 deploy/nurtur-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now nurtur-backup.timer
rm -rf /tmp/nurtur-deploy
```

`/etc/nurtur/backup.env` controls the destination and retention:

```sh
NURTUR_BACKUP_DIR=/var/backups/nurtur
NURTUR_BACKUP_KEEP_DAYS=14
```

The backup unit reads `NURTUR_COMPOSE_FILE` and `NURTUR_COMPOSE_SERVICE` from
`/etc/nurtur/update.env`. If you are not installing the update timer, set those
two values in `backup.env` instead.

Every day at 04:15 (with up to ten minutes of random delay, and once after boot
if the machine was off), the timer:

1. finds the host directory mounted at `/data`;
2. stops Nurtur cleanly so the WAL is checkpointed;
3. writes a gzip tar snapshot of the whole data directory;
4. starts Nurtur again and waits for its health check;
5. deletes snapshots older than `NURTUR_BACKUP_KEEP_DAYS`.

The stop/start is usually one or two seconds. The backup and update units share
the same lock, so they never run at the same time.

Run a snapshot immediately and inspect it:

```sh
sudo systemctl start nurtur-backup.service
sudo journalctl -u nurtur-backup.service -n 50 --no-pager
sudo ls -lah /var/backups/nurtur
sudo tar tzf /var/backups/nurtur/$(ls -1t /var/backups/nurtur | head -1) | head
```

### Manual snapshot

With local-only mode, `/srv/nurtur/data` is the only copy. Stop the service
cleanly so the WAL is checkpointed and pending sync work is flushed:

```sh
sudo docker compose stop nurtur
sudo tar czf /var/backups/nurtur-$(date +%F).tgz -C /srv/nurtur/data .
sudo docker compose start nurtur
```

### Restore

Restore into an empty data directory, preserving ownership:

```sh
sudo docker compose stop nurtur
sudo tar xzf /var/backups/nurtur-YYYY-MM-DD.tgz -C /srv/nurtur/data
sudo chown -R 10001:10001 /srv/nurtur/data
sudo docker compose start nurtur
```

The backup archive is not encrypted and contains your baby's care history. If
you copy it off the VPS, use encrypted storage such as restic, borg, or an
rclone crypt remote:

```sh
# After configuring an rclone crypt remote:
sudo rclone sync /var/backups/nurtur encrypted-remote:nurtur-backups
```

The app also exports JSON/CSV/SQL from Settings. The SQL export is a logical
dump and includes `schema_migrations`; see the README's restore drill. To keep
an offsite copy instead, enable Turso Cloud:

```sh
NURTUR_LOCAL_ONLY=false
TURSO_DATABASE_URL=libsql://your-db.turso.io
TURSO_AUTH_TOKEN=...
NURTUR_SYNC_INTERVAL=5m
```

Local writes push within a few seconds regardless of this value; the interval
only controls how often the server pulls changes made outside the app (for
example, in the Turso dashboard). Five minutes is the recommendation for this
single-server topology. The built-in default is `60s`; going below roughly
`30s` adds cloud requests and exposes the sync engine to more network failures
without making anything fresher.

Startup fails loudly if sync is enabled with only one of the two Turso values.
Check `/healthz?verbose=1` for the last push/pull and pending-sync age.

## What the VPS agent needs to do

This is the one-time setup for a standalone Compose stack. It assumes Nurtur is
already running and that the GHCR package is public. Replace
`/path/to/your/compose` and `/srv/nurtur/docker-compose.yml` with the real path.

### 1. Switch Nurtur to the stable image

In the standalone Compose file:

```yaml
  nurtur:
    image: ghcr.io/denhafiz69/nurtur:stable
    # remove any build: block or pinned version
```

Then pull it once. This must happen after the first `v*` release tag has been
pushed, otherwise `stable` does not exist yet.

```sh
cd /path/to/your/compose
sudo docker compose pull nurtur
sudo docker compose up -d nurtur
```

### 2. Install the update timer

```sh
git clone --depth 1 https://github.com/DenHafiz69/nurtur.git /tmp/nurtur-deploy
cd /tmp/nurtur-deploy

sudo install -m 755 deploy/nurtur-update.sh /usr/local/bin/nurtur-update
sudo install -d -m 755 /etc/nurtur
sudo install -m 644 deploy/nurtur-update.env.example /etc/nurtur/update.env
sudo editor /etc/nurtur/update.env
sudo install -m 644 deploy/nurtur-update.service /etc/systemd/system/
sudo install -m 644 deploy/nurtur-update.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now nurtur-update.timer
rm -rf /tmp/nurtur-deploy
```

`/etc/nurtur/update.env` must contain:

```sh
NURTUR_COMPOSE_FILE=/srv/nurtur/docker-compose.yml
NURTUR_COMPOSE_SERVICE=nurtur
```

`NURTUR_COMPOSE_SERVICE` is the Compose service name, not `container_name`.

### 3. Install the backup timer

```sh
git clone --depth 1 https://github.com/DenHafiz69/nurtur.git /tmp/nurtur-deploy
cd /tmp/nurtur-deploy

sudo install -m 755 deploy/nurtur-backup.sh /usr/local/bin/nurtur-backup
sudo install -d -m 755 /etc/nurtur
sudo install -m 644 deploy/nurtur-backup.env.example /etc/nurtur/backup.env
sudo editor /etc/nurtur/backup.env
sudo install -m 644 deploy/nurtur-backup.service /etc/systemd/system/
sudo install -m 644 deploy/nurtur-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now nurtur-backup.timer
rm -rf /tmp/nurtur-deploy
```

`/etc/nurtur/backup.env` controls the destination and retention:

```sh
NURTUR_BACKUP_DIR=/var/backups/nurtur
NURTUR_BACKUP_KEEP_DAYS=14
```

### 4. Verify both timers

```sh
sudo systemctl start nurtur-update.service
sudo journalctl -u nurtur-update.service -n 50 --no-pager

sudo systemctl start nurtur-backup.service
sudo journalctl -u nurtur-backup.service -n 50 --no-pager
sudo ls -lah /var/backups/nurtur

systemctl list-timers nurtur-update.timer nurtur-backup.timer
sudo docker compose -f /srv/nurtur/docker-compose.yml ps nurtur
```

Do not leave the VPS until the update check reports the service healthy, the
backup archive exists, and both timers have a next-elapse time. Also arrange an
offsite copy of `/var/backups/nurtur` or provider snapshots; local snapshots do
not survive a lost VPS.

## Troubleshooting

- **Caddy returns 502**: both containers must share a network, and the proxy
  target must be `nurtur:8080` (service name, not `localhost`). Confirm the app
  is healthy with `docker compose exec nurtur curl -fsS http://127.0.0.1:8080/healthz`.
- **`permission denied` opening the database**: fix the bind mount owner:
  `sudo chown -R 10001:10001 /srv/nurtur/data`.
- **Container exits at startup**: read the first lines of
  `docker compose logs nurtur`. Common causes are an invalid `NURTUR_*` value,
  a malformed env file, or `NURTUR_LOCAL_ONLY=false` with missing Turso values.
- **Can't pull the image**: the GHCR package is private. Log in with a
  `read:packages` token, or make the package public.
- **Data looks empty after redeploy**: confirm the bind mount path did not
  change and that `NURTUR_DB` is `/data/nurtur.db`.
- **`read-only file system` in the logs**: with `read_only: true`, only `/data`
  and the `/tmp` tmpfs are writable. Keep both mounts; the Turso cache lives at
  `/tmp/turso-go`.
- **Automatic updates do nothing**: confirm the service uses the `stable` tag,
  that `/etc/nurtur/update.env` points at the right Compose file, and that the
  timer is active with `systemctl list-timers nurtur-update.timer`. Run
  `sudo systemctl start nurtur-update.service` and read
  `journalctl -u nurtur-update.service`.
- **Automatic update fails its health check**: inspect
  `sudo docker compose -f /path/to/compose logs nurtur`. The updater exits
  non-zero so the failure is visible in the journal. Restore the previous
  semver tag and the matching backup if the schema changed.
- **Backup timer runs but no archive appears**: check
  `journalctl -u nurtur-backup.service`, confirm `NURTUR_BACKUP_DIR` is writable,
  and verify `/etc/nurtur/update.env` or `backup.env` points at the right
  Compose file. The script needs to be able to find the `/data` mount.
- **Nurtur stays stopped after a backup**: run
  `sudo docker compose -f /path/to/compose start nurtur` and inspect
  `journalctl -u nurtur-backup.service`. The script restarts Nurtur on both
  success and failure, but a broken image or config can still prevent startup.
- **Exit code 137 / container killed while idle**: the memory limit is too
  tight. Raise `memory` and `GOMEMLIMIT` together, then check
  `docker stats nurtur` again.

## Security checklist

- [ ] `/etc/nurtur/nurtur.env` is `600 root:root` and not in the repository
- [ ] `NURTUR_SIGNUP_KEY` and `NURTUR_SESSION_SECRET` are random and unique
- [ ] `NURTUR_ADMIN_PASSWORD` is set to a long, unique password (or left empty
      to keep the admin screen disabled)
- [ ] `NURTUR_PUBLIC_URL` points at the public HTTPS origin when invites are used
- [ ] `NURTUR_COOKIE_SECURE=true` behind HTTPS
- [ ] Only 80/443 are exposed; Nurtur has no public port
- [ ] The container runs `read_only` with a `/tmp` tmpfs and
      `no-new-privileges`
- [ ] A memory limit is set and `GOMEMLIMIT` sits below it
- [ ] `nurtur-update.timer` is enabled and `/etc/nurtur/update.env` points at the
      standalone Compose file
- [ ] `nurtur-backup.timer` is enabled, `/var/backups/nurtur` has recent
      archives, and an **offsite** copy exists
- [ ] A restore has been tested, not just the backup command
- [ ] The GHCR token or package visibility is intentional
- [ ] Runtime secrets were **not** added to GitHub; CI needs none

## CI/CD boundary

The GitHub Actions workflow stops at publishing the image. Deployment is
pull-based: the VPS systemd timer pulls the `stable` tag, so GitHub never needs
SSH access to the server and no runtime `.env` values leave the VPS.

The systemd alternative remains in `deploy/nurtur.service` and
`deploy/nurtur.env.example` for running the bare binary; do not use both.
