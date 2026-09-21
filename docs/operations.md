# Operations

This document owns concrete operating procedures, configuration and current limitations. Architecture owns required behavior; [current status](s0-execution-progress.md) records implemented behavior and remaining stage gates. A documented target is not an implemented production package.

## 1. Implemented topology

Current daemons are `backend/cmd/control-hub` and `backend/cmd/runtime-relay`. `devmigrate` and `generate-android-wire` are utilities, not services. The S0.2 internal Preview package targets NVIDIA DGX Spark Linux ARM64 and supplies a PM2 ecosystem, the service-root `run.sh`, a remote-Caddy reference template and runbooks under `deploy/preview`; it does not include the planned Enterprise Tool Gateway or multi-node/HA operation. See [S0.2 Preview deployment](s02-preview-deployment.md).

Admin is a static Quasar SPA. Supply `--admin-assets-dir <console/dist/spa>` (or `HUB_ADMIN_ASSETS_DIR`) to the Hub daemon; startup rejects a missing `index.html`, and the existing static handler owns `/admin` and deep links. Omitting the option leaves static hosting disabled. Production ingress must route `/api/client/v1`, `/api/admin/v1`, `/admin` to Hub and `/runtime/v1` to Relay under one origin; test-library hosting does not qualify production TLS/ingress.

`npm start`, `concurrently`, `go run`, and Node/Go harness process orchestration are development/test tools, not a production supervisor. See [development](development.md) for local startup; Relay usage delivery uses the private Hub listener.

### One public origin

The repository-root Caddy example below remains a local development recipe. The Spark Preview does not install Caddy: Hub public binds `0.0.0.0:9004`, Relay public binds `0.0.0.0:9002`, and the remote Tailscale ingress proxies those two ports. Hub/Relay internal ports `9001`/`9003` stay on loopback. Do not apply the development bind variables or local Caddy commands to the Spark runbook.

The checked-in [Caddyfile](../deploy/Caddyfile) routes Discovery, Admin, Client control and Portal to Hub, `/runtime/v1` to Relay, and rejects `/internal` and its children. With its loopback defaults, start Hub public on `127.0.0.1:9004` (private `9001`), Relay public on `127.0.0.1:9002` (private `9003`), then run from the repository root:

```text
caddy validate --config deploy/Caddyfile --adapter caddyfile
caddy run --config deploy/Caddyfile --adapter caddyfile
```

Clients use `http://127.0.0.1:9000`; they resolve `clientApiBase` and `runtimeApiBase` from `/.well-known/measix`, never the internal component ports. For the standard Portal, also set Hub `--public-origin http://127.0.0.1:9000 --portal-assets-dir ../../measix-enterprise-portal/dist` when running from `backend/`. To use an independently deployed enterprise Portal, set `--portal-upstream-url http://portal.example/` instead; Android still opens Hub `/portal/`. Admin assets remain `--admin-assets-dir ../console/dist/spa`.

For a device deployment, set `MEASIX_PUBLIC_ADDRESS` to the device-reachable HTTP or HTTPS origin and `MEASIX_BIND` to the intended ingress interface. Set Hub `--public-origin` to that same origin for the first startup. For example, `http://192.0.2.20:9000` is a documentation-only LAN-shaped origin; replace it with the private deployment value. IP addresses, domain names and custom ports are supported. HTTP does not require DNS or certificates. HTTPS termination belongs to the ingress when selected. `MEASIX_HUB_UPSTREAM` and `MEASIX_RELAY_UPSTREAM` override the private backend addresses. Expose only the public ingress; loopback on Android refers to the device, not this computer. The application does not configure router forwarding or firewall rules; verify the selected address from the device network.

The first successful Hub startup persists `--public-origin` into Deployment settings. Afterwards Admin **Global settings** is authoritative and can change it without restarting Hub; the startup flag remains a seed for a clean database, not an override of a reviewed Admin change. For a Caddy deployment, set the persisted value to the external address such as `https://core.example.com`, while Hub may continue listening on a private HTTP address. Configure and verify DNS, TLS and Caddy before saving: Core does not provision them. A change affects new enrollment material, Portal URLs/origin checks and Secure-cookie policy; it revokes existing Portal browser sessions but preserves Deployment, User, Device and Android Session identity. Existing Android clients can continue the same enterprise session after changing their enterprise address to the new origin.

Use Caddy's native [WebSocket and streaming proxy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy#streaming). No body buffering, retry or `flush_interval -1` override is required; SSE is flushed automatically, and forcing negative flush intervals disables upstream cancellation on client disconnect. The Caddy administration API is disabled. This recipe is ingress configuration, not the S0.3 production supervisor/package. Local HTTP evidence does not qualify public TLS or device connectivity.

## 2. Configuration actually implemented

Source: `backend/internal/hub/config/config.go`, `backend/internal/relay/config/config.go`. CLI flags override environment defaults; duration environment values are parsed before flags, so an invalid environment duration can fail loading even with a valid flag. All options except the persisted public origin are startup configuration; there is no general-purpose config hot reload.

### Control Hub

| Flag | Environment | Default / requirement |
| --- | --- | --- |
| `--listen` | `HUB_LISTEN_ADDR` | `:8080` |
| `--internal-listen` | `HUB_INTERNAL_LISTEN_ADDR` | `127.0.0.1:8081`; keep private |
| `--admin-assets-dir` | `HUB_ADMIN_ASSETS_DIR` | Optional production SPA directory |
| `--diagnostics-log-dir` | `HUB_DIAGNOSTICS_LOG_DIR` | Optional fixed directory containing Hub/Relay PM2 log files for authenticated Admin diagnostics |
| `--public-origin` | `HUB_PUBLIC_ORIGIN` | Initial public HTTP/HTTPS platform origin seed; later changes are persisted through Admin Global settings |
| `--portal-assets-dir` | `HUB_PORTAL_ASSETS_DIR` | Standard `measix-enterprise-portal/dist`; requires approved origin |
| `--portal-upstream-url` | `HUB_PORTAL_UPSTREAM_URL` | Optional custom enterprise HTTP/HTTPS static site; takes precedence over assets and does not fall back |
| `--db` | `HUB_DB_PATH` | Required SQLite path |
| `--master-key-file` | `HUB_MASTER_KEY_FILE` | Required AES-256 key file; secret |
| `--jwt-private-key-file` | `HUB_JWT_PRIVATE_KEY_FILE` | Required Ed25519 key file; secret |
| `--relay-internal-url` | `RELAY_INTERNAL_URL` | Required absolute HTTP(S) URL; private |
| `--relay-service-token-file` | `HUB_RELAY_SERVICE_TOKEN_FILE` | Required token file; secret |
| `--access-token-ttl` | `HUB_ACCESS_TOKEN_TTL` | `10m`; positive, at most `10m` |
| `--reconcile-interval` | `HUB_RECONCILE_INTERVAL` | `10s`; positive |

Android sessions have a seven-day rolling idle deadline, renewed only by refresh. Refresh rotates credentials and requires a stable per-command `Idempotency-Key`; the same old credential/key recovers the identical encrypted response for two minutes without extending the lease twice. Rotation recovery survives Hub restart using master-key-derived encryption. Persist client pending refresh input/key before sending; a different key conflicts, and an expired recovery window requires re-enrollment. Old fixed-TTL and absolute Discovery URL flags were removed; Discovery returns same-origin paths.

Admin user deletion is a deny-first destructive workflow, not ordinary disable/logout. The operator must enter the exact username and a reason. Hub first blocks new Client, Runtime and refresh operations, then completes the durable full purge of the user's devices, sessions, configuration, budgets, usage and Portal-private state. Audit keeps only the required non-reversible subject summary; credential digests are stored as irreversible tombstones so every old access/runtime or refresh credential returns `401 enterprise_identity_deleted` instead of entering refresh/retry or becoming a generic invalid credential. A failed purge remains visible and retryable; only `COMPLETED` means deletion succeeded. Reusing the username later creates a different principal ID with no inherited state; issue a new one-time enrollment. Because deletion removes the old Device, the same installation may bind the fresh principal, while all old credentials remain terminally rejected.

Logout revokes the durable Android session and clears rotation recovery. The existing Hub reconciler projects pending session denies through a SECURITY_CHANGE Activation; HTTP 204 is not Relay acknowledgement. Previously issued access may remain usable until the deny applies or its short expiry. Disable/revoke also invalidate sessions; enabling a user does not resurrect credentials. A new administrator-issued enrollment may replace sessions on the same ACTIVE installation/user, but cannot revive a revoked device or transfer another user's installation.

### Runtime Relay

| Flag | Environment | Default / requirement |
| --- | --- | --- |
| `--public-listen` | `RELAY_PUBLIC_LISTEN_ADDR` | `:8090` |
| `--internal-listen` | `RELAY_INTERNAL_LISTEN_ADDR` | `127.0.0.1:8091`; must differ from public listen string |
| `--spool` | `RELAY_SPOOL_PATH` | `relay-spool.db`; nonempty |
| `--hub-internal-url` | `RELAY_HUB_INTERNAL_URL` | Required Hub **private** API base for budget admission, lifecycle, and durable usage settlement |
| `--hub-service-token-file` | `RELAY_HUB_SERVICE_TOKEN_FILE` | Required token file; secret |
| `--usage-batch-size` | `RELAY_USAGE_BATCH_SIZE` | `100`; range `1..200` |
| `--usage-flush-interval` | `RELAY_USAGE_FLUSH_INTERVAL` | `1s`; positive |
| `--shutdown-grace` | `RELAY_SHUTDOWN_GRACE` | `30s`; positive |

Both default private listeners are loopback-only. Isolate both internal listeners; never publish them through public ingress. The servers use HTTP listeners, not built-in TLS termination. Current wiring reuses one token for Hub→Relay control and Relay→Hub usage: separate configuration names do not establish separate trust scopes.

Use restricted secret files and persistent, explicitly resolved DB/spool paths. Key decoding/accepted formats belong to `backend/internal/hub/security`; verify against it when provisioning. Never place secret values in command history, Git, logs or support bundles.

## 3. Bootstrap and startup

`control-hub` has `run`, `migrate`, `bootstrap-admin`, `check` and `backup` subcommands. Inspect each subcommand's flags with `--help`; maintenance commands do not use the full run configuration. Default bootstrap refuses an existing deployment; `--if-empty` skips an initialized deployment without resetting credentials, while `--add-admin` explicitly adds an administrator. They are mutually exclusive. Initial bootstrap accepts `--timezone <IANA zone>` (default UTC) for Enterprise Update date boundaries. Use its password-file input, not a password printed into shared logs.

Run `migrate` before bootstrap/startup; it initializes an empty database or applies pending append-only migrations. `run` does not create or alter schema. Startup verifies recorded versions/checksums, opens the database, requires the deployment invariant and initializes runtime services. See [database migrations](database-migrations.md).

Start Hub/Relay, wait for explicit readiness, verify desired/applied control state, then expose traffic. Relay cannot serve authorized runtime traffic before valid control state is applied. Process liveness does not prove activation, usage delivery or static hosting.

## 4. Health and status

| Component | Public probes | Status | Interpretation |
| --- | --- | --- | --- |
| Hub | `/live`, `/ready` | Authenticated Admin System API | Ready after initialization; Relay runtime can still be `DEGRADED` |
| Relay | `/live`, `/ready` | Private `/internal/v1/control/status`, service authentication | Ready once control state exists; spool degradation is separate |

OpenAPI/router registrations own exact responses. The unauthenticated System health endpoint only probes the local DB connection; full schema/Relay/usage diagnostics stay in authenticated System status and the maintenance command. Hub System reports the current schema identity expected by the binary, computed from the embedded initialization SQL. It forwards Relay spool state, pending count and oldest age; absent observations remain unknown, not zero. Ingest lag is separate from backlog. Hub build identity defaults to `dev` unless supplied at build time; release provenance must pin binaries and static assets.

## 5. Shutdown and durability

Shared HTTP serving handles SIGINT/SIGTERM and invokes bounded drain: Hub uses 30 seconds, Relay its configured grace. After drain/deadline it cancels request contexts, closes connections, rejects new admission and waits up to five seconds for handler cleanup before returning. Relay returns server errors through its owner so final flush and deferred spool close still run. Handlers must honor cancellation; production supervisor hard-stop qualification remains S0.3.

Normal Relay shutdown attempts one final usage flush, capped at two seconds, and preserves the durable spool. It does not guarantee the entire backlog reaches Hub before exit. Sender retry/backoff and poison-batch splitting exist; the recorder degraded flag remains latched until restart. Diagnose failures before restart; never delete the spool as routine recovery.

Runtime response cleanup records usage even when ReverseProxy aborts a mid-stream response. Such facts preserve the already-sent HTTP status and captured control/resource attribution, with `CLIENT_CANCELLED`, `UPSTREAM_TIMEOUT` or `UPSTREAM_UNAVAILABLE` distinguishing the failure. An upstream 200 alone does not prove a stream completed; inspect its error classification.

## 6. Persistence, backup and restore

Hub owns its control/identity/usage SQLite database. Relay owns its local durable usage spool. Neither reads the other's database. Keep both outside replaceable binary directories; handle SQLite auxiliary files correctly when moving a stopped database.

Current commands, with paths supplied by the operator:

```text
control-hub check --db <hub.db>
control-hub backup --db <hub.db> --output <new-backup.db>
```

Backup uses SQLite `VACUUM INTO` and writes an adjacent `.metadata.json`. Both targets are exclusively reserved; existing database or orphan metadata is not overwritten. Source and copied database pass migration-history, integrity, foreign-key and current-column checks before metadata is synced. Metadata records the binary schema identity and integer schema version. These checks do not replace an isolated restore/business replay.

`check` derives required tables/columns from current Ent schema, including Enterprise Update and session recovery; it checks SQLite integrity/foreign keys. It does not attest every index or column type equivalence. Success is necessary but insufficient for release.

There is no in-place restore CLI. Restore uses a stopped-service file replacement described by the S0.2 Preview deployment runbook. First restore a copy in an isolated environment with matching binaries and required keys, run `check`, and verify identities, releases/generations and usage. Never experiment on the only production copy; keep the original recoverable until acceptance.

## 7. Preview supervision, logging and recent telemetry

The S0.2 Preview uses root-owned PM2 with one forked instance each for Hub and Relay. The root PM2 daemon calls the single service-root `run.sh hub|relay`; it does not assemble binary flags itself. Caddy is managed only on the separate Tailscale ingress server, not on Spark. This is intentionally smaller than the later Gateway/S0.3 topology. Concrete lifecycle and rotation settings are in the packaged ecosystem and [deployment runbook](s02-preview-deployment.md).

Hub and Relay run as the unprivileged `measix` user and retain independent failure domains. The Preview `run.sh` fixes public listeners to `0.0.0.0:9004` and `0.0.0.0:9002`, fixes internal listeners to loopback `9001` and `9003`, uses bounded restart delay/count and a 40-second kill timeout, and writes separate stdout/stderr files below the explicit deployment root. PM2 lifecycle state is not application readiness.

Hub/Relay use the common safe JSON logger on stdout with `service`, `buildVersion` and stable `event`. HTTP completion middleware records route templates, method, status and duration, excludes successful health probes, and feeds fixed 60 one-minute in-memory buckets. Error values are reduced to safe classes, sensitive field names are redacted and text values are bounded/scrubbed.

Authenticated Admin exposes 15/60-minute telemetry and up to 200 recent redacted events from exactly four fixed Hub/Relay stdout/stderr files. Reads are tail-bounded to 2 MiB per file and 4 KiB per line; arbitrary paths, PM2 manager logs and remote-Caddy logs are never accepted. The System page uses at most 60 lightweight SVG points and Quasar virtual scrolling. PM2 logrotate owns file rotation; no centralized log-search platform is required.

Never emit tokens, cookies, credentials, enrollment/session/signing material, private endpoints, toolRef/claims, raw prompts/bodies/tool arguments/results or direct personal identity. Test normal and failure diagnostics for forbidden material. References: [systemd service lifecycle](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html), [journald retention](https://www.freedesktop.org/software/systemd/man/252/journald.conf.html); documentation is not runtime qualification.

## 8. Troubleshooting and release gate

| Symptom | Safe first checks |
| --- | --- |
| Usage never arrives | URL must use Hub private port (default `8081`, not `8080`); inspect token, spool status and ingest errors |
| Relay not ready after restart | Inspect Hub reconcile and Relay applied revision; do not bypass authentication/inject state |
| Hub ready but runtime degraded | Compare desired/applied revision and activation; readiness is not convergence |
| `/admin` missing or deep links fail | Check actual static host/ingress; verify `--admin-assets-dir`, `index.html` and same-origin ingress |
| Schema/check disagreement | Preserve the database and migration error, compare the packaged migration set, and restore the pre-upgrade backup if needed; never edit history rows |
| Repeated process crash | Preserve diagnostics/persistent data; no production restart-rate-limit package exists yet |

Deployment must pin artifacts, verify checksums, back up, apply the packaged forward migrations, validate readiness/control/static routing and run smoke/recovery checks. Downgrade means restoring both the pre-upgrade release and its backup; migration files/history are never reversed in place. RC also needs isolated restore, spool replay, resource/load, supervision and log-redaction proof; see [release](release.md) and [testing](testing.md).
