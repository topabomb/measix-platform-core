# Operations

This document owns concrete operating procedures, configuration and current limitations. Architecture owns required behavior; [current status](s0-execution-progress.md) records implemented behavior and remaining stage gates. A documented target is not an implemented production package.

## 1. Implemented topology

Current daemons are `backend/cmd/control-hub` and `backend/cmd/runtime-relay`. `devmigrate` and `generate-android-wire` are utilities, not services. Enterprise Tool Gateway, service units and a production installation package do not exist yet.

Admin is a static Quasar SPA. Supply `--admin-assets-dir <console/dist/spa>` (or `HUB_ADMIN_ASSETS_DIR`) to the Hub daemon; startup rejects a missing `index.html`, and the existing static handler owns `/admin` and deep links. Omitting the option leaves static hosting disabled. Production ingress must route `/api/client/v1`, `/api/admin/v1`, `/admin` to Hub and `/runtime/v1` to Relay under one origin; test-library hosting does not qualify production TLS/ingress.

`npm start`, `concurrently`, `go run`, and Node/Go harness process orchestration are development/test tools, not a production supervisor. See [development](development.md) for local startup; Relay usage delivery uses the private Hub listener.

### One public origin

The checked-in [Caddyfile](../deploy/Caddyfile) routes Discovery, Admin, Client control and Portal to Hub, `/runtime/v1` to Relay, and rejects `/internal` and its children. With its loopback defaults, start Hub public on `127.0.0.1:9004` (private `9001`), Relay public on `127.0.0.1:9002` (private `9003`), then run from the repository root:

```text
caddy validate --config deploy/Caddyfile --adapter caddyfile
caddy run --config deploy/Caddyfile --adapter caddyfile
```

Clients use `http://127.0.0.1:9000`; they resolve `clientApiBase` and `runtimeApiBase` from `/.well-known/measix`, never the internal component ports. For the standard Portal, also set Hub `--public-origin http://127.0.0.1:9000 --portal-assets-dir ../../measix-enterprise-portal/dist` when running from `backend/`. To use an independently deployed enterprise Portal, set `--portal-upstream-url http://portal.example/` instead; Android still opens Hub `/portal/`. Admin assets remain `--admin-assets-dir ../console/dist/spa`.

For a device deployment, set `MEASIX_PUBLIC_ADDRESS` to the device-reachable HTTP or HTTPS origin and `MEASIX_BIND` to the intended ingress interface. Set Hub `--public-origin` to that same origin. For example, `http://192.168.31.235:9000` is a valid LAN deployment; IP addresses, domain names and custom ports are supported. HTTP does not require DNS or certificates. HTTPS termination belongs to the ingress when selected. `MEASIX_HUB_UPSTREAM` and `MEASIX_RELAY_UPSTREAM` override the private backend addresses. Expose only the public ingress; loopback on Android refers to the device, not this computer. The application does not configure router forwarding or firewall rules; verify the selected address from the device network.

Use Caddy's native [WebSocket and streaming proxy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy#streaming). No body buffering, retry or `flush_interval -1` override is required; SSE is flushed automatically, and forcing negative flush intervals disables upstream cancellation on client disconnect. The Caddy administration API is disabled. This recipe is ingress configuration, not the S0.3 production supervisor/package. Local HTTP evidence does not qualify public TLS or device connectivity.

## 2. Configuration actually implemented

Source: `backend/internal/hub/config/config.go`, `backend/internal/relay/config/config.go`. CLI flags override environment defaults; duration environment values are parsed before flags, so an invalid environment duration can fail loading even with a valid flag. All options are startup configuration; there is no hot reload.

### Control Hub

| Flag | Environment | Default / requirement |
| --- | --- | --- |
| `--listen` | `HUB_LISTEN_ADDR` | `:8080` |
| `--internal-listen` | `HUB_INTERNAL_LISTEN_ADDR` | `127.0.0.1:8081`; keep private |
| `--admin-assets-dir` | `HUB_ADMIN_ASSETS_DIR` | Optional production SPA directory |
| `--public-origin` | `HUB_PUBLIC_ORIGIN` | Public HTTP/HTTPS platform origin; IP/domain and optional port |
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

Logout revokes the durable Android session and clears rotation recovery. The existing Hub reconciler projects pending session denies through a SECURITY_CHANGE Activation; HTTP 204 is not Relay acknowledgement. Previously issued access may remain usable until the deny applies or its short expiry. Disable/revoke also invalidate sessions; enabling a user does not resurrect credentials. A new administrator-issued enrollment may replace sessions on the same ACTIVE installation/user, but cannot revive a revoked device or transfer another user's installation.

### Runtime Relay

| Flag | Environment | Default / requirement |
| --- | --- | --- |
| `--public-listen` | `RELAY_PUBLIC_LISTEN_ADDR` | `:8090` |
| `--internal-listen` | `RELAY_INTERNAL_LISTEN_ADDR` | `127.0.0.1:8091`; must differ from public listen string |
| `--spool` | `RELAY_SPOOL_PATH` | `relay-spool.db`; nonempty |
| `--hub-usage-url` | `HUB_USAGE_URL` | Required; Hub **private** usage-ingest endpoint |
| `--hub-service-token-file` | `RELAY_HUB_SERVICE_TOKEN_FILE` | Required token file; secret |
| `--usage-batch-size` | `RELAY_USAGE_BATCH_SIZE` | `100`; range `1..200` |
| `--usage-flush-interval` | `RELAY_USAGE_FLUSH_INTERVAL` | `1s`; positive |
| `--shutdown-grace` | `RELAY_SHUTDOWN_GRACE` | `30s`; positive |

Both default private listeners are loopback-only. Isolate both internal listeners; never publish them through public ingress. The servers use HTTP listeners, not built-in TLS termination. Current wiring reuses one token for Hub→Relay control and Relay→Hub usage: separate configuration names do not establish separate trust scopes.

Use restricted secret files and persistent, explicitly resolved DB/spool paths. Key decoding/accepted formats belong to `backend/internal/hub/security`; verify against it when provisioning. Never place secret values in command history, Git, logs or support bundles.

## 3. Bootstrap and startup

`control-hub` has `run`, `bootstrap-admin`, `check` and `backup` subcommands. Inspect each subcommand's flags with `--help`; maintenance commands do not use the full run configuration. Default bootstrap refuses an existing deployment; `--if-empty` skips an initialized deployment without resetting credentials, while `--add-admin` explicitly adds an administrator. They are mutually exclusive. Initial bootstrap accepts `--timezone <IANA zone>` (default UTC) for Enterprise Update date boundaries. Use its password-file input, not a password printed into shared logs.

Initialize a clean database from the reviewed current SQL before startup; `run` does not create or alter schema. Startup opens/checks the database, requires the deployment invariant and initializes runtime services. See [database initialization](database-migrations.md) for limits of the check and development helper.

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

Backup uses SQLite `VACUUM INTO` and writes an adjacent `.metadata.json`. Both targets are exclusively reserved; existing database or orphan metadata is not overwritten. Source and copied database pass integrity/foreign-key/current-column checks before metadata is synced. Metadata records the binary's current SQL content identity. These checks do not replace an isolated restore/business replay.

`check` derives required tables/columns from current Ent schema, including Enterprise Update and session recovery; it checks SQLite integrity/foreign keys. It does not attest every index or column type equivalence. Success is necessary but insufficient for release.

There is no restore CLI or fully packaged production restore runbook. Before replacing any deployment database, restore a copy in an isolated environment with matching binaries, required keys and the same current schema identity; check integrity, identities, releases/generations and usage, then run recovery scenarios. Never experiment on the only production copy; keep the original recoverable until acceptance.

## 7. S0.3 supervision and logging deliverables

Implement supervision **within S0.3**, using host-native service management, not a fourth custom orchestration daemon. The reference is Linux `systemd` + `journald`; other platforms must prove equivalent behavior. Architecture owns the [Gateway operational contract](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-enterprise-tool-gateway-contract-spec.md); unit names, concrete timeouts, paths and commands belong here once implemented.

Required package: one unit per Hub/Relay/Gateway, one aggregate target/group, independent failure domains, least privilege, immutable builds, private/public binding, readiness separate from ordering, bounded restart delay/rate limiting, permanent configuration-failure handling, graceful stop followed by supervisor termination after timeout, clean install/recovery commands and failure-injection tests. None is production-qualified merely by being listed here.

Hub/Relay currently use `slog.JSONHandler` on stdout (`time`, `level`, `msg`). They do not consistently attach `service`, `buildVersion`, stable `event` or correlations. Raw error logging does not establish redaction.

The S0.3 shared initializer must attach `time`, `level`, `msg`, `service`, `buildVersion`, `event`; add request/interaction/activation/deployment/generation/control/resource/tool IDs, duration/outcome/errorCode only when applicable. Log route templates, not raw query strings. Supervisor collection owns rotation, bounded size/time retention and safe export; services do not share/rotate log files. No centralized log-search platform is required.

Never emit tokens, cookies, credentials, enrollment/session/signing material, private endpoints, toolRef/claims, raw prompts/bodies/tool arguments/results or direct personal identity. Test normal and failure diagnostics for forbidden material. References: [systemd service lifecycle](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html), [journald retention](https://www.freedesktop.org/software/systemd/man/252/journald.conf.html); documentation is not runtime qualification.

## 8. Troubleshooting and release gate

| Symptom | Safe first checks |
| --- | --- |
| Usage never arrives | URL must use Hub private port (default `8081`, not `8080`); inspect token, spool status and ingest errors |
| Relay not ready after restart | Inspect Hub reconcile and Relay applied revision; do not bypass authentication/inject state |
| Hub ready but runtime degraded | Compare desired/applied revision and activation; readiness is not convergence |
| `/admin` missing or deep links fail | Check actual static host/ingress; verify `--admin-assets-dir`, `index.html` and same-origin ingress |
| Schema/check disagreement | Delete a non-current database or inspect the reviewed current initialization SQL; do not convert it in place |
| Repeated process crash | Preserve diagnostics/persistent data; no production restart-rate-limit package exists yet |

Deployment must pin artifacts, verify applicable evidence, initialize the reviewed current schema on a clean database, validate readiness/control/static routing and run smoke/recovery checks. A database with another schema identity is deleted and recreated; there is no binary/database upgrade or downgrade path before the first release. RC also needs isolated restore of the same current schema, spool replay, resource/load, supervision and log-redaction proof; see [release](release.md) and [testing](testing.md).
