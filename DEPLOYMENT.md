# Deployment

How to run Nurtur on a VPS using the image built by CI, alongside an existing
Docker Compose stack and Caddy reverse proxy.

CI publishes `ghcr.io/denhafiz69/nurtur` on every push to `main` and every `v*`
tag. Runtime configuration stays on the VPS — nothing in `.env-example` belongs
in GitHub secrets. The workflow only uses the automatic `GITHUB_TOKEN` to push
the image.

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

## 2. Add Nurtur to your Compose stack

Add this service to the **existing** `docker-compose.yml` that already runs your
other services. Merge the `volumes` section if you already have one.

```yaml
services:
  nurtur:
    image: ghcr.io/denhafiz69/nurtur:latest
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

### Private image

The GHCR package is private until you change it in the package settings. If it
stays private, log in on the VPS with a token that has `read:packages`:

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
person who needs their own account. Onboarding writes the account and baby name
into the database; later logins use that username/password and only see that
account's data.

If login appears to succeed but you are bounced back to the sign-in page, check
`NURTUR_COOKIE_SECURE`: it must be `true` over HTTPS and `false` only when
testing over plain HTTP.

## 5. Updates

```sh
cd /path/to/your/compose
sudo docker compose pull nurtur
sudo docker compose up -d nurtur
```

`latest` moves with every green `main` build. For predictable upgrades, pin a
release tag in the service definition:

```yaml
    image: ghcr.io/denhafiz69/nurtur:1.2.3
```

Take a snapshot before upgrading. Migrations run at startup and a new schema is
not guaranteed to work with an older binary, so rolling the image tag back is
not always a complete rollback.

## 6. Backups and restore

With local-only mode, `/srv/nurtur/data` is the only copy. Stop the service
cleanly so the WAL is checkpointed and pending sync work is flushed:

```sh
sudo docker compose stop nurtur
sudo tar czf /var/backups/nurtur-$(date +%F).tgz -C /srv/nurtur/data .
sudo docker compose start nurtur
```

Restore into an empty data directory, preserving ownership:

```sh
sudo docker compose stop nurtur
sudo tar xzf /var/backups/nurtur-YYYY-MM-DD.tgz -C /srv/nurtur/data
sudo chown -R 10001:10001 /srv/nurtur/data
sudo docker compose start nurtur
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
- **Exit code 137 / container killed while idle**: the memory limit is too
  tight. Raise `memory` and `GOMEMLIMIT` together, then check
  `docker stats nurtur` again.

## Security checklist

- [ ] `/etc/nurtur/nurtur.env` is `600 root:root` and not in the repository
- [ ] `NURTUR_SIGNUP_KEY` and `NURTUR_SESSION_SECRET` are random and unique
- [ ] `NURTUR_COOKIE_SECURE=true` behind HTTPS
- [ ] Only 80/443 are exposed; Nurtur has no public port
- [ ] The container runs `read_only` with a `/tmp` tmpfs and
      `no-new-privileges`
- [ ] A memory limit is set and `GOMEMLIMIT` sits below it
- [ ] Backups run on a schedule and a restore has been tested
- [ ] The GHCR token or package visibility is intentional
- [ ] Runtime secrets were **not** added to GitHub; CI needs none

## CI/CD boundary

The GitHub Actions workflow stops at publishing the image. This deployment is
pull-based: the VPS pulls the tag you choose. If you later want automatic
deploys, add a separate workflow with SSH credentials stored as GitHub
environment secrets, and keep runtime `.env` values on the VPS rather than
passing them as Docker build arguments.

The systemd alternative remains in `deploy/nurtur.service` and
`deploy/nurtur.env.example` for running the bare binary; do not use both.
