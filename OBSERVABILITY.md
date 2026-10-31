# Observability and monitoring plan

Status: proposal. No monitoring stack is installed by the repository yet.

Nurtur is a local-first, single-binary Go app. It already emits structured JSON
logs and a useful `/healthz` endpoint, but it has no metrics, no alerting, no
log search, and no external uptime check. This plan adds those without turning
the product into an analytics product.

The target is a self-hosted, open-source stack that costs nothing beyond the
machines it runs on. No third-party JavaScript, no hosted analytics, and no
user behaviour tracking. Every signal described here is operational: is the app up,
is it fast, is sync working, is the backup fresh, is the disk filling.

## Recommended baseline

Start with **Prometheus + Alertmanager + node_exporter + Uptime Kuma + ntfy**,
with **Grafana** for dashboards. Add **Loki + Alloy** only when you actually
need log search. This covers availability, latency, errors, host capacity,
sync health, and backup freshness with a small, well-understood stack. Use the
lean alternative below if the VPS cannot spare the memory.

## Principles

1. **Operate the service, not the people using it.** Metrics carry route
   patterns, status codes, and aggregate event counts. They never carry an
   account ID, username, baby name, note text, IP address, or full URL.
2. **Local-first means monitoring must be honest.** A monitor on the same VPS
   cannot report that the VPS is down. At least one check must run elsewhere.
3. **Free and open source by default.** Prometheus, Grafana, Loki, Alloy,
   Alertmanager, node_exporter, Uptime Kuma, and ntfy are all openly licensed
   and run on your own hardware.
4. **Alert on symptoms a parent would notice.** "Cannot open the app", "logs
   are not saving", "sync is stuck", "the backup is missing". Not every
   transient 4xx.
5. **Keep the monitoring stack off the public internet.** Only Grafana and an
   optional status page get a hostname, both behind authentication.
6. **No 3 a.m. page for a warning.** Critical alerts are reserved for "the app
   is unusable or data is at risk". Everything else goes to a low-priority
   channel.

## What exists today

| Signal | Where | Notes |
|---|---|---|
| Structured logs | stdout via `log/slog` JSON handler | `cmd/nurtur/main.go` |
| Request logs | `logRequests` middleware | method, path, status, bytes, `duration_ms`; no query string |
| Panic logs | `recoverer` middleware | includes stack trace |
| Health | `GET /healthz` | JSON: DB ping, mode, version; `?verbose=1` adds sync stats |
| Sync telemetry | `/healthz?verbose=1` | pending ops, WAL bytes, last push/pull, errors, retries, stall state |
| Container health | Docker `HEALTHCHECK` | hits `/healthz` every 30s |
| Update/backup jobs | systemd timers | logs in the journal; no heartbeat or alert |
| Reverse proxy | Caddy | access logging not configured by this repo |

Gaps:

- No `/metrics` endpoint and no time-series history.
- No alerting. A failed backup or a wedged sync is only discovered by reading
  logs.
- No external uptime check, so a dead VPS or expired certificate is silent.
- No log retention or search. `docker compose logs` is the only tool.
- No dashboard for HTTP, memory, disk, or sync health.
- Request logs have no request ID, so a single request is hard to follow.
- Auth logs include raw client IPs (see the privacy section).

## Target architecture

```text
                    external probe (Uptime Kuma on a second host,
                    or a free hosted check)
                              |
                              | HTTPS /healthz
                              v
  Parent's browser ---> Caddy ---> nurtur:8080
                                    |  /healthz
                                    |  /metrics  (internal :9091)
                                    |
                          shared Docker network
                                    |
             +----------------------+----------------------+
             |                      |                      |
        Prometheus             node_exporter           cAdvisor
             |                      |                      |
      alert rules              host CPU/disk          container memory
             |
         Alertmanager ---> ntfy ---> phone
             |
         Grafana <--- Prometheus
             ^
             |
      Loki <--- Alloy <--- Docker stdout (JSON logs)

  Uptime Kuma push monitors <--- backup timer / update timer heartbeats
```

## Tooling

| Layer | Choice | Licence | Why |
|---|---|---|---|
| App metrics | `prometheus/client_golang` | Apache-2.0 | Standard exposition format; Go and process collectors included |
| Metrics store | Prometheus | Apache-2.0 | Single binary, simple config, 15-day retention is enough |
| Dashboards | Grafana | AGPL-3.0 | Best dashboard ecosystem; provision dashboards as JSON |
| Alerting | Alertmanager | Apache-2.0 | Dedupe, grouping, inhibition, silences |
| Host metrics | node_exporter | Apache-2.0 | CPU, memory, disk, filesystem, network |
| Container metrics | cAdvisor | Apache-2.0 | Per-container memory, CPU, restarts |
| Logs (phase 3) | Loki + Grafana Alloy | AGPL-3.0 / Apache-2.0 | Label-indexed logs, cheap on disk; Alloy replaces Promtail |
| Uptime | Uptime Kuma | MIT | HTTP/TCP/push checks, public status page, ntfy notifications |
| Alert delivery | ntfy | Apache-2.0 / GPL-2.0 | Self-hosted push to phone; no account, no cloud |
| Optional error tracking | GlitchTip (self-hosted) | Open source | Frontend JS errors without a third-party SDK; only if needed |

### Lean alternative for a very small VPS

The standard stack costs roughly 700 MB–1.1 GB of RAM. If the VPS cannot
spare that, replace Prometheus + Grafana + Loki with:

- **VictoriaMetrics** single-node (includes `vmui` dashboards and a PromQL
  compatible query UI), **vmalert**, and **Alertmanager**; or
- just **Uptime Kuma + node_exporter + ntfy** for phase 0, and use
  `docker logs`/`journalctl` manually for the rest.

Do not run the monitoring stack on the same 512 MB VPS as a 64 MB app if it
leaves no headroom. Put it on a second small host and scrape Nurtur over a
private WireGuard/Tailscale link, or use a free-tier VM. Only metrics and logs
cross that link; no user data is exposed.

## Phased rollout

Each phase is independently useful. Stop after any phase if the budget or the
time is not there.

### Phase 0 — Stop the silent failures (half a day, no app changes)

Goal: know when the app or its housekeeping jobs die.

1. Add Docker log rotation to the Nurtur service, otherwise `json-file` logs
   grow until the disk fills:

   ```yaml
   logging:
     driver: json-file
     options:
       max-size: "10m"
       max-file: "3"
   ```

2. Run **Uptime Kuma** on a second host (or a free-tier VM) and create:
   - an HTTPS monitor for `https://nurtur.example.com/healthz` every 60s,
     accepting only status 200 and a body containing `"status": "ok"` or
     `"degraded"`;
   - a certificate-expiry monitor for the same hostname;
   - push monitors for the backup and update timers.
3. Add a heartbeat to `deploy/nurtur-backup.sh` after the final log line:

   ```sh
   if [ -n "${NURTUR_BACKUP_HEARTBEAT_URL:-}" ]; then
     curl -fsS --max-time 10 --retry 3 "$NURTUR_BACKUP_HEARTBEAT_URL" >/dev/null || true
   fi
   ```

   Use the push URL from Uptime Kuma, with a period of 24h and a grace period
   of at least 2h. A missing ping means the timer did not run or the backup
   failed. Add `NURTUR_BACKUP_HEARTBEAT_URL` to `deploy/nurtur-backup.env.example`
   and `/etc/nurtur/backup.env`.
4. Do the same for `deploy/nurtur-update.sh` if automatic updates are enabled.
   A stale update heartbeat is a warning, not a critical alert.
5. Add ntfy as the notification channel in Uptime Kuma. Use two topics:
   `nurtur-critical` and `nurtur-warnings`.
6. Optionally publish a Uptime Kuma status page at `status.example.com`. It
   shows only service names, never household or usage data.

Acceptance: stop the container and confirm a push notification arrives within
three minutes; run the backup by hand and confirm its heartbeat clears.

### Phase 1 — Application metrics (one to two days)

Goal: replace "read the logs" with a metrics endpoint and a privacy-safe
catalogue.

1. Add `github.com/prometheus/client_golang` and a small `internal/metrics`
   package with its own registry, the Go collector, the process collector, and
   the Nurtur collectors.
2. Serve `/metrics` on a **separate internal listener**, not on the main mux:

   ```text
   NURTUR_METRICS_ADDR=:9091
   ```

   Default is empty (disabled) so existing installs do not change. Set
   `NURTUR_METRICS_ADDR=:9091` in the image and the deployment guides. The
   listener binds inside the container only; do not publish the port and do not
   proxy it through Caddy. This keeps metrics out of the auth middleware and
   out of the public URL.
3. Start and shut down the metrics server alongside the main HTTP server in
   `cmd/nurtur/main.go`, with the same signal handling and a short timeout.
4. Add a metrics middleware around the mux:
   - `nurtur_http_requests_total{method,route,status}`
   - `nurtur_http_request_duration_seconds{method,route}` (histogram)
   - `nurtur_http_in_flight_requests`
   - `nurtur_http_response_bytes_total{route}`
   - `nurtur_panics_recovered_total`

   Use the matched route pattern, not the raw path. Go 1.23+ populates
   `r.Pattern` on the same request pointer while `ServeMux` serves it, so
   record the label after `next.ServeHTTP`. Place the metrics middleware
   *inside* `security`, which clones the request with the CSP nonce; a
   middleware outside that clone will not see the populated pattern. Fall
   back to `unmatched` when the pattern is empty (for example, an auth
   redirect that never reaches the mux). Never label with the raw path:
   `/feeds/123` and `/feeds/456` would create unbounded cardinality and leak
   row IDs. Skip `/static/`, `/healthz`, and `/metrics` from request metrics.
5. Add aggregate business counters at the handler boundaries. These answer
   "is logging working?" without identifying anyone:
   - `nurtur_feeds_started_total{kind="nursing|bottle"}`
   - `nurtur_diapers_logged_total{kind="wet|dirty|mixed"}`
   - `nurtur_sleeps_started_total`
   - `nurtur_logins_total{result="ok|failed"}`
   - `nurtur_accounts_created_total`
   - `nurtur_invites_created_total{result="ok|failed"}`

   No account ID, username, baby name, note text, or IP label. Do not add a
   per-account dimension "for later".
6. Add sync and database gauges by reading the existing `SyncWorker.Status()`
   and `SyncWorker.Stats(ctx)` at scrape time. `Stats` is already non-blocking
   and returns `ErrSyncBusy` when an operation is in flight; emit the last
   known values and increment `nurtur_sync_stats_errors_total` instead of
   failing the scrape.
7. Add `internal/metrics/metrics_test.go` that asserts the endpoint returns
   200, the exposition parses, and no forbidden labels appear.

Acceptance: `curl http://nurtur:9091/metrics` inside the Compose network shows
HTTP, Go runtime, process, DB, and sync series. A review of the output finds no
account, baby, note, IP, or raw-path values.

### Phase 2 — Prometheus, Grafana, and alerts (one day)

1. Create a separate `deploy/monitoring/` Compose project. Keep it out of
   `docker-compose.yaml`, which is the local development file.
2. Attach Prometheus to a private network shared with Nurtur. If Nurtur lives
   in the user's existing stack, create an external `monitoring` network and
   attach only the Nurtur service and Prometheus to it. Do not attach the
   whole monitoring stack to the public `web` network.
3. Scrape:

   ```yaml
   scrape_configs:
     - job_name: nurtur
       metrics_path: /metrics
       static_configs:
         - targets: ["nurtur:9091"]
     - job_name: node
       static_configs:
         - targets: ["node-exporter:9100"]
     - job_name: cadvisor
       static_configs:
         - targets: ["cadvisor:8080"]
     - job_name: nurtur-blackbox
       metrics_path: /probe
       params:
         module: [http_2xx]
       static_configs:
         - targets: ["https://nurtur.example.com/healthz"]
       relabel_configs:
         - source_labels: [__address__]
           target_label: __param_target
         - source_labels: [__param_target]
           target_label: instance
         - target_label: __address__
           replacement: blackbox-exporter:9115
   ```

   The blackbox probe is the outside-in view: it catches Caddy, TLS, DNS, and
   network failures that an in-container scrape cannot see. If Uptime Kuma
   already covers this, the blackbox job is optional but gives Prometheus the
   same signal for dashboards and alert rules.
4. Provision Grafana with:
   - anonymous access disabled, sign-up disabled;
   - a strong admin password from `/etc/nurtur/monitoring.env`;
   - dashboards and datasources committed as JSON/YAML under
     `deploy/monitoring/grafana/`;
   - the public URL behind Caddy at `grafana.example.com`, HTTPS only, with a
     firewall rule that leaves Prometheus, Loki, and Alertmanager unexposed.
5. Alertmanager routes to ntfy. Alertmanager has no native ntfy receiver, so
   use a small webhook bridge (`apprise-api` or `alertmanager-ntfy`) or the
   native Telegram receiver if a third-party relay is acceptable. Alerts
   contain only alert names, instance names, and numeric values.
6. Add the alert rules in the appendix and keep them in
   `deploy/monitoring/prometheus/rules/nurtur.yml`.
7. Configure a silence for warnings during 22:00–07:00 local time. Critical
   alerts still fire.

Acceptance: `kill -9` the Nurtur process and get a critical notification within
three minutes; stop the backup timer and get a warning after its grace period;
open Grafana and see request rate, error rate, p95 latency, memory, disk, and
sync age.

### Phase 3 — Logs (one to two days, optional)

Goal: search by request ID and alert on error spikes without keeping logs
forever.

1. Add a random `request_id` to each request log line and return it as
   `X-Request-ID`. Generate 16 bytes from `crypto/rand` and hex-encode. Never
   accept a client-supplied ID without validation; if you do accept one,
   truncate and restrict it to `[a-zA-Z0-9-]`.
2. Add Loki single-binary with filesystem storage, 14-day retention, and a
   compactor. Keep the volume on the same disk but cap it with a size-based
   retention rule.
3. Ship Docker stdout with Alloy. Mounting the Docker socket read-only is the
   simplest discovery method, but the Docker socket is root-equivalent; if
   that is unacceptable, switch the Nurtur service to the `journald` log driver
   and use Alloy's journald source instead. Do not mount the socket into the
   app container.
4. If Caddy access logs are enabled, strip query strings before shipping them.
   `GET /signup?invite=...` contains a one-use invite token, and the default
   Caddy access log records the full URI. Either disable Caddy access logging
   (the app already logs method, path, status, and duration), exclude
   `/signup`, or use a custom log encoder that drops the query. The app's own
   request log deliberately logs `r.URL.Path`, not `RequestURI`, so it does not
   leak invite tokens.
5. Keep `NURTUR_LOG_LEVEL=info` in production. Debug adds static-asset noise
   and more sync detail.

Acceptance: search a request ID and see the matching request line; a log query
for `level=error` has no note text, baby names, or session cookies.

### Phase 4 — SLOs and runbooks (ongoing)

Goal: make the alerts mean something and keep them from becoming noise.

1. Adopt the SLOs below and record them in Grafana.
2. Write a short runbook for each alert (see the appendix).
3. Review alerts monthly. Delete any alert that has not fired or that was
   always ignored. An ignored alert is worse than no alert.
4. Run a restore drill quarterly and record the result. A backup heartbeat
   proves the script ran, not that the archive restores.

### Phase 5 — Optional and explicitly deferred

- **Frontend error tracking:** self-hosted GlitchTip or Sentry, with PII
  scrubbing, sampling, and no session replay. Defer until there is a real
  frontend bug you cannot reproduce.
- **Tracing:** OpenTelemetry + Tempo is not worth it for one process with a
  local database. Revisit only if Nurtur splits into multiple services.
- **Continuous profiling:** `pprof` on a debug-only internal listener is
  enough if memory or CPU becomes mysterious.
- **Business analytics:** do not add per-user funnels, retention cohorts, or
  session tracking. The brand promise is that the app does not watch the
  parent. Aggregate counters in phase 1 are the limit.

## Metrics catalogue

All names use the `nurtur_` prefix. Labels in `{}` are the only permitted
dimensions.

| Metric | Type | Labels | Purpose |
|---|---|---|---|
| `nurtur_build_info` | gauge | `version` | One series per running build |
| `nurtur_http_requests_total` | counter | `method`, `route`, `status` | Traffic and error rate |
| `nurtur_http_request_duration_seconds` | histogram | `method`, `route` | Latency SLO |
| `nurtur_http_in_flight_requests` | gauge | — | Saturation |
| `nurtur_http_response_bytes_total` | counter | `route` | Payload size regressions |
| `nurtur_panics_recovered_total` | counter | — | Should be zero |
| `nurtur_feeds_started_total` | counter | `kind` | Logging still works |
| `nurtur_diapers_logged_total` | counter | `kind` | Logging still works |
| `nurtur_sleeps_started_total` | counter | — | Logging still works |
| `nurtur_logins_total` | counter | `result` | Auth failures and lockout risk |
| `nurtur_accounts_created_total` | counter | — | Onboarding works |
| `nurtur_invites_created_total` | counter | `result` | Admin flow works |
| `nurtur_db_up` | gauge | — | 1 when `Ping` succeeds |
| `nurtur_db_ping_duration_seconds` | histogram | — | Local DB health |
| `nurtur_db_size_bytes` | gauge | — | Growth and disk pressure |
| `nurtur_sync_up` | gauge | — | Sync worker running |
| `nurtur_sync_dirty` | gauge | — | Local changes not pushed |
| `nurtur_sync_pending_ops` | gauge | — | Queue depth |
| `nurtur_sync_pending_age_seconds` | gauge | — | Freshness SLO |
| `nurtur_sync_last_success_timestamp_seconds` | gauge | `operation` | Push/pull/checkpoint freshness |
| `nurtur_sync_operation_duration_seconds` | histogram | `operation` | Cloud latency |
| `nurtur_sync_operations_total` | counter | `operation`, `result` | Failure rate |
| `nurtur_sync_stalled` | gauge | — | Worker watchdog fired |
| `nurtur_sync_wal_bytes` | gauge | — | Uncheckpointed WAL growth |
| `go_*`, `process_*` | various | — | Runtime memory, goroutines, GC, FDs |

Deliberately absent: `account_id`, `username`, `baby_name`, `note_id`,
`invite_token`, `client_ip`, `user_agent`, raw `path`, raw `query`, and any
label whose value comes from user input.

## SLOs

| SLO | Target | Measurement |
|---|---|---|
| Public availability | 99.5% over 30 days | External HTTPS probe every 60s |
| HTTP error rate | 5xx < 1% of requests over 5m | `nurtur_http_requests_total` |
| Server latency | 95% < 500ms, 99% < 1.5s | Request histogram, excluding static |
| Sync freshness | Pending changes pushed within 5m; alert at 10m | `nurtur_sync_pending_age_seconds` |
| Backup freshness | One successful archive per 24h; alert at 26h | Uptime Kuma push monitor |
| TLS certificate | Renewed with at least 14 days left | Blackbox or Uptime Kuma |
| Restore confidence | Successful drill every 90 days | Runbook record |

99.5% allows about 3h 39m of unscheduled downtime per 30 days. A single VPS
with no HA will not beat that by much; do not promise 99.9% in a status page.
The error budget also gives permission to stop shipping non-critical changes
after a bad week.

## Alerts

| Alert | Condition | For | Severity |
|---|---|---|---|
| `NurturDown` | `up{job="nurtur"} == 0` | 2m | critical |
| `NurturPublicDown` | `probe_success{job="nurtur-blackbox"} == 0` | 2m | critical |
| `NurturHealthDegraded` | health endpoint reports `degraded` for sync errors | 10m | warning |
| `NurturHigh5xx` | 5xx share > 2% of requests | 10m | critical if >10%, else warning |
| `NurturSlowRequests` | p95 > 1s | 10m | warning |
| `NurturPanic` | `increase(nurtur_panics_recovered_total[5m]) > 0` | 0m | critical |
| `NurturSyncStalled` | `nurtur_sync_stalled == 1` | 1m | critical |
| `NurturSyncStale` | `nurtur_sync_pending_age_seconds > 600` | 10m | warning |
| `NurturSyncFailing` | push or pull failure rate > 0 for 15m | 15m | warning |
| `NurturDBDown` | `nurtur_db_up == 0` | 2m | critical |
| `NurturMemoryPressure` | container working set > 90% of limit | 10m | warning |
| `NurturRestarting` | more than 2 container starts in 1h | 10m | warning |
| `HostDiskFilling` | free space < 15% | 30m | warning |
| `HostDiskCritical` | free space < 5% | 5m | critical |
| `BackupOverdue` | no heartbeat for 26h | — | critical |
| `UpdateOverdue` | no update check for 2h (if enabled) | — | warning |
| `TLSCertExpiring` | < 14 days remaining | 1h | warning |

Alert design notes:

- Group by `alertname` and `severity`; repeat a firing alert every 4h, not every
  minute.
- Inhibit `NurturHigh5xx`, `NurturSlowRequests`, and `NurturSync*` when
  `NurturDown` or `NurturPublicDown` is firing. One outage should produce one
  notification.
- Do not alert on individual 4xx responses. Login failures and CSRF rejections
  are expected; only a sustained spike is interesting.
- `NurturHealthDegraded` needs a small exporter or a blackbox body check. If
  that is awkward, derive the same condition from `nurtur_sync_dirty`,
  `nurtur_sync_pending_age_seconds`, and the failure counters instead.

## Dashboards

Keep dashboards small and opinionated. A parent does not need a dashboard; you
do, at 2 a.m., on a phone.

1. **Nurtur Overview** — up, request rate, 5xx %, p95 latency, memory vs limit,
   goroutines, sync pending age, last backup age, certificate days left.
2. **HTTP RED** — rate, errors, duration by route; top routes; in-flight
   requests.
3. **Sync** — dirty, pending ops, pending age, push/pull success timestamps,
   operation duration, WAL bytes, stall state.
4. **Host** — CPU, load, memory, disk free, disk I/O, network, container
   restarts.
5. **Logs** — error rate by route, recent errors, request-ID search.
6. **Capacity** — DB size, WAL size, log volume, backup size, projected days
   until disk full.

Provision them as code under `deploy/monitoring/grafana/dashboards/` and mount
the directory read-only. Do not click dashboards into existence on the server;
they will be lost with the container.

## Privacy guardrails

This is the part that matters most for Nurtur.

- **No third-party scripts or SDKs** in the web app. Monitoring is
  server-side and self-hosted. The README promise stays true.
- **No user identifiers in metrics.** Route patterns and aggregate counts only.
  A metric label is effectively a permanent, queryable record; treat it like a
  database column.
- **No request bodies, note text, baby names, or cookies in logs.** The current
  request log is already safe; keep it that way.
- **Hash IP addresses on auth events.** `internal/server/auth.go` currently
  logs raw `clientIP(r)` on login failures, signup rejection, and account
  creation. For abuse detection, log
  `sha256(salt || ip)[:12]` with a per-install salt, or drop the IP entirely and
  rely on Caddy/edge rate limiting. If raw IPs stay, keep auth logs for no
  more than 7 days and document it in the privacy note.
- **Invite tokens never reach logs.** The app logs paths, not query strings.
  If Caddy access logs are enabled, strip queries as described in phase 3.
- **Retention is short and explicit.** Metrics 15 days, logs 14 days, backups
  follow the existing 14-day policy plus the offsite copy. Delete on a schedule;
  do not keep "just in case" data forever.
- **Access is limited.** Grafana, Prometheus, Loki, Alertmanager, and Uptime
  Kuma are reachable only on the private network or through an authenticated
  reverse proxy. No public `/metrics`, no public Prometheus, no public Loki.
- **A status page contains service state only.** No account counts, no usage
  charts, no "12 parents logged today".

If the app ever adds first-party aggregate telemetry, update the README and
privacy note in the same change. "No analytics" must keep meaning "no tracking
of people", not "no numbers on a dashboard".

## Resource budget and placement

Approximate steady-state memory:

| Component | RAM | Disk |
|---|---|---|
| Prometheus (15s scrape, 15d) | 100–200 MB | 200–500 MB |
| Grafana | 150–250 MB | < 100 MB |
| Loki + Alloy (14d, low volume) | 200–350 MB | 1–3 GB |
| Alertmanager | 30 MB | < 50 MB |
| node_exporter | 20 MB | — |
| cAdvisor | 50–100 MB | — |
| Uptime Kuma | 100–150 MB | < 100 MB |
| ntfy | 20 MB | < 50 MB |
| **Total (standard)** | **~700 MB–1.1 GB** | **2–4 GB** |

Recommendations:

- If the VPS has less than 1 GB free, use the lean alternative or put the stack
  on a second host. Do not squeeze Prometheus and Grafana next to a 64 MB app
  and then blame the app when the kernel OOM killer fires.
- Run Uptime Kuma **off the Nurtur VPS**. It is the only component that can
  tell you the VPS is gone.
- Put Prometheus's data and Loki's chunks on a named volume with a size cap,
  and alert on disk free space before either fills the root filesystem.
- Keep the app's `GOMEMLIMIT` (48 MiB) below the container limit (64 MiB). Add
  the monitoring stack's own limits; do not let it consume the host.

## Security checklist

- [ ] `/metrics` listens on an internal port and is not published or proxied.
- [ ] Prometheus, Loki, Alertmanager, node_exporter, and cAdvisor are not
      reachable from the internet.
- [ ] Grafana has sign-up disabled, anonymous access disabled, and a unique
      admin password stored in `/etc/nurtur/monitoring.env` (mode 600).
- [ ] Grafana is behind Caddy with HTTPS; consider an IP allowlist or basic
      auth in addition to the login.
- [ ] The monitoring Compose project uses a dedicated network, not the public
      `web` network.
- [ ] The Docker socket, if mounted for Alloy, is read-only and only in the
      Alloy container.
- [ ] Alert webhooks contain no user data.
- [ ] A retention policy is configured for Prometheus, Loki, and Docker logs.
- [ ] Backups of the monitoring configuration exist; dashboards and rules are
      in Git, not only in a volume.

## Runbooks

**NurturDown / NurturPublicDown**

1. `docker compose -f <file> ps nurtur` and `docker compose logs --tail=100 nurtur`.
2. `curl -fsS http://127.0.0.1:8080/healthz` from the host or inside the
   container.
3. If Caddy returns 502, check that both containers share a network and the
   proxy target is `nurtur:8080`.
4. If the container is restarting, check the first log lines for a config error,
   a missing `/data` mount, or an OOM kill (`docker inspect` exit code 137).
5. If the host is unreachable, use the provider console. Restore from the
   latest archive only if the data directory is damaged.

**NurturSyncStalled**

1. This is the known Turso engine wedge. The process exits on its own so Docker
   or systemd can restart it.
2. Confirm the restart happened: `docker compose ps` should show a new uptime,
   and `/healthz?verbose=1` should show a fresh `last_push`.
3. If it stalls repeatedly, check network egress and Turso status; consider
   `NURTUR_LOCAL_ONLY=true` temporarily. Local writes are durable in the WAL.
4. Do not delete the database or the WAL to "fix" sync.

**NurturSyncStale / NurturSyncFailing**

1. Read `/healthz?verbose=1` for `last_push_error`, `last_pull_error`, and
   `pending_age_seconds`.
2. Check DNS, egress, and the Turso token. A rotated token needs a restart.
3. If only pulls fail, local logging still works; escalate as a warning.
4. If pending age keeps growing, follow the stalled runbook.

**BackupOverdue**

1. `systemctl status nurtur-backup.timer` and
   `journalctl -u nurtur-backup.service -n 100 --no-pager`.
2. Check that `/var/backups/nurtur` is writable and has free space.
3. Run `sudo systemctl start nurtur-backup.service` and confirm the archive.
4. If the backup fails because Nurtur will not restart, fix the app first; do
   not leave it stopped.

**HostDiskFilling**

1. `df -h`, `docker system df`, and `du -sh /var/lib/docker/* /var/backups/nurtur`.
2. Prune dangling images (`docker image prune -f`) and old backup archives
   beyond the retention policy.
3. Check Loki retention and Docker log rotation.
4. Raise the disk or move Loki/backups before it hits 5%.

**TLSCertExpiring**

1. Caddy renews automatically; a failure usually means port 80/443 is blocked
   or DNS changed.
2. `docker compose logs caddy | tail -100` and `caddy validate`.
3. Reload Caddy after fixing DNS or firewall rules.

## Implementation checklist

### Repository changes

- [ ] `internal/metrics/` package with registry, collectors, and tests.
- [ ] `internal/config`: `NURTUR_METRICS_ADDR` (default empty/disabled).
- [ ] `cmd/nurtur/main.go`: second internal HTTP server for `/metrics`, with
      graceful shutdown.
- [ ] `internal/server`: instrumentation middleware using route patterns;
      business counters in feed/diaper/sleep/auth handlers; request ID in logs.
- [ ] `internal/store/sync.go`: expose a metrics snapshot without importing
      Prometheus into the store package.
- [ ] `deploy/nurtur.env.example`, `deploy/nurtur-backup.env.example`, and
      `DEPLOYMENT.md`: document the metrics port and heartbeat URL.
- [ ] `deploy/monitoring/`: Compose file, Prometheus config and rules,
      Alertmanager config, Grafana provisioning, Alloy/Loki config, Uptime Kuma
      notes.
- [ ] Docker log rotation in every example Compose file.
- [ ] Link this document from `README.md` and `DEPLOYMENT.md`.
- [ ] Privacy review of every new metric label and log field before merge.

### Verification

- [ ] `curl -fsS http://nurtur:9091/metrics | grep '^nurtur_'` returns the
      expected series.
- [ ] `grep -E 'account|username|baby|note|invite|client_ip|user_agent'` on the
      metrics output returns nothing.
- [ ] Killing the app triggers `NurturDown` within 3 minutes.
- [ ] Stopping the backup timer triggers `BackupOverdue` after the grace period.
- [ ] A 5xx storm triggers `NurturHigh5xx` and not five separate alerts.
- [ ] The Grafana overview loads on a phone-sized screen.
- [ ] A restore drill from the latest archive succeeds.
- [ ] The app container still runs under 64 MB with metrics enabled; if not,
      lower histogram bucket count or drop the response-bytes counter.

## Appendix: example configuration

### Prometheus rules

```yaml
groups:
  - name: nurtur
    rules:
      - alert: NurturDown
        expr: up{job="nurtur"} == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: Nurtur metrics endpoint is unreachable
          runbook: OBSERVABILITY.md#runbooks

      - alert: NurturPublicDown
        expr: probe_success{job="nurtur-blackbox"} == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: Nurtur is not reachable through the public URL

      - alert: NurturHigh5xx
        expr: |
          sum(rate(nurtur_http_requests_total{status=~"5.."}[5m]))
            /
          sum(rate(nurtur_http_requests_total[5m])) > 0.02
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: More than 2% of requests are failing with 5xx

      - alert: NurturSyncStalled
        expr: nurtur_sync_stalled == 1
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: The sync engine watchdog fired; the process should restart

      - alert: NurturSyncStale
        expr: nurtur_sync_pending_age_seconds > 600
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: Local changes have not reached Turso for 10 minutes

      - alert: HostDiskCritical
        expr: |
          node_filesystem_avail_bytes{mountpoint="/",fstype!~"tmpfs|overlay"}
            /
          node_filesystem_size_bytes{mountpoint="/",fstype!~"tmpfs|overlay"} < 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: Root filesystem is below 5% free
```

Add inhibition rules so `NurturDown` suppresses the secondary HTTP and sync
alerts.

### Alertmanager route

```yaml
route:
  receiver: ntfy-critical
  group_by: [alertname, severity]
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  routes:
    - matchers: [severity="warning"]
      receiver: ntfy-warnings
      repeat_interval: 12h

receivers:
  - name: ntfy-critical
    webhook_configs:
      - url: http://alertmanager-ntfy:8080/critical
        send_resolved: true
  - name: ntfy-warnings
    webhook_configs:
      - url: http://alertmanager-ntfy:8080/warnings
        send_resolved: true
```

The bridge turns Alertmanager's JSON payload into an ntfy message. Keep the
bridge on the private network.

### Compose sketch

```yaml
services:
  prometheus:
    image: prom/prometheus:latest
    restart: unless-stopped
    command:
      - --config.file=/etc/prometheus/prometheus.yml
      - --storage.tsdb.retention.time=15d
    volumes:
      - ./prometheus:/etc/prometheus:ro
      - prometheus-data:/prometheus
    networks: [monitoring]

  alertmanager:
    image: prom/alertmanager:latest
    restart: unless-stopped
    volumes:
      - ./alertmanager:/etc/alertmanager:ro
    networks: [monitoring]

  grafana:
    image: grafana/grafana-oss:latest
    restart: unless-stopped
    env_file:
      - /etc/nurtur/monitoring.env
    environment:
      GF_SECURITY_ADMIN_PASSWORD: "${GRAFANA_ADMIN_PASSWORD}"
      GF_USERS_ALLOW_SIGN_UP: "false"
      GF_AUTH_ANONYMOUS_ENABLED: "false"
    volumes:
      - grafana-data:/var/lib/grafana
      - ./grafana:/etc/grafana/provisioning:ro
    networks: [monitoring]

  node-exporter:
    image: prom/node-exporter:latest
    restart: unless-stopped
    command: ["--path.rootfs=/host"]
    pid: host
    volumes:
      - /:/host:ro,rslave
    networks: [monitoring]

  cadvisor:
    image: gcr.io/cadvisor/cadvisor:latest
    restart: unless-stopped
    privileged: true
    volumes:
      - /:/rootfs:ro
      - /var/run:/var/run:ro
      - /sys:/sys:ro
      - /var/lib/docker:/var/lib/docker:ro
    networks: [monitoring]

  ntfy:
    image: binwiederhier/ntfy:latest
    restart: unless-stopped
    command: ["serve"]
    volumes:
      - ntfy-data:/var/cache/ntfy
    networks: [monitoring]

volumes:
  prometheus-data:
  grafana-data:
  ntfy-data:

networks:
  monitoring:
    external: true
```

Pin image tags before production. `latest` is shown only to keep the plan
readable. Add CPU/memory limits, health checks, and log rotation to each
service, and put Grafana behind Caddy rather than publishing its port.

## References

- Prometheus: <https://prometheus.io/docs/>
- Grafana provisioning: <https://grafana.com/docs/grafana/latest/administration/provisioning/>
- Loki: <https://grafana.com/docs/loki/latest/>
- Uptime Kuma: <https://github.com/louislam/uptime-kuma>
- ntfy: <https://ntfy.sh/docs/>
- VictoriaMetrics (lean alternative): <https://docs.victoriametrics.com/>
- Existing deployment guide: `DEPLOYMENT.md`
- Product privacy stance: `README.md`, `NATIVE.md` §11.4
