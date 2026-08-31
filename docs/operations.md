# Operations

This document owns concrete operating procedures, configuration and current limitations. Architecture owns required behavior; [the alignment audit](architecture-alignment-audit.md) records the 2026-08-31 source baseline and remediation plan. A documented target is not an implemented production package.

## 1. Implemented topology

Current daemons are `backend/cmd/control-hub` and `backend/cmd/runtime-relay`. `devmigrate` and `generate-android-wire` are utilities, not services. Enterprise Tool Gateway, service units and a production installation package do not exist yet.

Admin is a static Quasar SPA. Hub's runtime library accepts optional `AdminAssets`, but the production main supplies no assets and exposes no assets flag. A working test harness/static handler does not prove the shipped daemon serves `/admin`. Production integration still needs explicit static hosting/ingress packaging and same-origin routing verification.

`npm start`, `concurrently`, `go run`, and Node/Go harness process orchestration are development/test tools, not a production supervisor. See [development](development.md) for local startup and the root-script usage URL defect.

## 2. Configuration actually implemented

Source: `backend/internal/hub/config/config.go`, `backend/internal/relay/config/config.go`. CLI flags override environment defaults; duration environment values are parsed before flags, so an invalid environment duration can fail loading even with a valid flag. All options are startup configuration; there is no hot reload.

### Control Hub

| Flag | Environment | Default / requirement |
| --- | --- | --- |
| `--listen` | `HUB_LISTEN_ADDR` | `:8080` |
| `--internal-listen` | `HUB_INTERNAL_LISTEN_ADDR` | `:8081`; explicitly bind a private address |
| `--public-base-url` | `HUB_PUBLIC_BASE_URL` | Required absolute HTTP(S) URL |
| `--runtime-api-base` | `HUB_RUNTIME_API_BASE` | Required absolute HTTP(S) URL |
| `--db` | `HUB_DB_PATH` | Required SQLite path |
| `--master-key-file` | `HUB_MASTER_KEY_FILE` | Required AES-256 key file; secret |
| `--jwt-private-key-file` | `HUB_JWT_PRIVATE_KEY_FILE` | Required Ed25519 key file; secret |
| `--relay-internal-url` | `RELAY_INTERNAL_URL` | Required absolute HTTP(S) URL; private |
| `--relay-service-token-file` | `HUB_RELAY_SERVICE_TOKEN_FILE` | Required token file; secret |
| `--access-token-ttl` | `HUB_ACCESS_TOKEN_TTL` | `10m`; positive, at most `10m` |
| `--refresh-token-ttl` | `HUB_REFRESH_TOKEN_TTL` | `720h`; validated positive, but **not wired into identity service** |
| `--reconcile-interval` | `HUB_RECONCILE_INTERVAL` | `10s`; positive |

Refresh credentials currently expire at a fixed 30-day deadline; refresh does not rotate them or extend an idle deadline. Do not use the refresh TTL option as an operational control until fixed. Absolute discovery URLs also differ from the architecture's same-origin path-only contract. These are gaps, not alternative supported semantics.

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

Hub's default private listener is not loopback-only. Isolate both internal listeners; never publish them through public ingress. The servers use HTTP listeners, not built-in TLS termination. Current wiring reuses one token for Hub→Relay control and Relay→Hub usage: separate configuration names do not establish separate trust scopes.

Use restricted secret files and persistent, explicitly resolved DB/spool paths. Key decoding/accepted formats belong to `backend/internal/hub/security`; verify against it when provisioning. Never place secret values in command history, Git, logs or support bundles.

## 3. Bootstrap and startup

`control-hub` has `run`, `bootstrap-admin`, `check` and `backup` subcommands. Inspect each subcommand's flags with `--help`; maintenance commands do not use the full run configuration. Default bootstrap refuses an existing deployment; `--add-admin` is an explicit separate operation, not idempotent setup. Use its password-file input, not a password printed into shared logs.

Apply/review migrations before startup; `run` does not auto-migrate. Startup opens/checks the database, requires the deployment invariant and initializes runtime services. See [database migrations](database-migrations.md) for limits of the check and development helper.

Start Hub/Relay, wait for explicit readiness, verify desired/applied control state, then expose traffic. Relay cannot serve authorized runtime traffic before valid control state is applied. Process liveness does not prove activation, usage delivery or static hosting.

## 4. Health and status

| Component | Public probes | Status | Interpretation |
| --- | --- | --- | --- |
| Hub | `/live`, `/ready` | Authenticated Admin System API | Ready after initialization; Relay runtime can still be `DEGRADED` |
| Relay | `/live`, `/ready` | Private `/internal/v1/control/status`, service authentication | Ready once control state exists; spool degradation is separate |

OpenAPI/router registrations own exact responses. Hub System's schema revision is a hardcoded initial revision, not queried applied Atlas history. Ingest lag is not a complete pending-backlog monitor. Hub build identity defaults to `dev` unless supplied at build time; release provenance must pin binaries and static assets.

## 5. Shutdown and durability

Shared HTTP serving handles SIGINT/SIGTERM and invokes bounded shutdown: Hub uses 30 seconds, Relay its configured grace. The helper does not explicitly force-close/cancel surviving handlers after timeout. Relay's server-error `os.Exit` path bypasses deferred cleanup. These gaps must be fixed/tested before claiming the architecture's complete drain/cancel guarantee.

Normal Relay shutdown attempts one final usage flush, capped at two seconds, and preserves the durable spool. It does not guarantee the entire backlog reaches Hub before exit. Sender retry/backoff and poison-batch splitting exist; the recorder degraded flag remains latched until restart. Diagnose failures before restart; never delete the spool as routine recovery.

## 6. Persistence, backup and restore

Hub owns its control/identity/usage SQLite database. Relay owns its local durable usage spool. Neither reads the other's database. Keep both outside replaceable binary directories; handle SQLite auxiliary files correctly when moving a stopped database.

Current commands, with paths supplied by the operator:

```text
control-hub check --db <hub.db>
control-hub backup --db <hub.db> --output <new-backup.db>
```

Backup uses SQLite `VACUUM INTO` and writes an adjacent `.metadata.json`. Use a destination where neither file exists: the database overwrite check does not protect orphan metadata. Metadata records a fixed initial schema label. Reopening a backup is not an integrity check or restore test.

`check` runs SQLite integrity/foreign-key checks and checks a fixed initial table set; it does not verify every current column, Enterprise Update table, or complete migration history. Success is necessary but insufficient for release/upgrade.

There is no restore CLI or fully packaged production restore runbook. Before replacing any deployment database, restore a copy in an isolated environment with matching binaries, required keys and migrations; check integrity, identities, releases/generations and usage, then run recovery scenarios. Never experiment on the only production copy; keep the original recoverable until acceptance.

## 7. S0.3 supervision and logging deliverables

Implement supervision **within S0.3**, using host-native service management, not a fourth custom orchestration daemon. The reference is Linux `systemd` + `journald`; other platforms must prove equivalent behavior. Architecture owns the [Gateway operational contract](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-enterprise-tool-gateway-contract-spec.md); unit names, concrete timeouts, paths and commands belong here once implemented.

Required package: one unit per Hub/Relay/Gateway, one aggregate target/group, independent failure domains, least privilege, immutable builds, private/public binding, readiness separate from ordering, bounded restart delay/rate limiting, permanent configuration-failure handling, graceful stop followed by supervisor termination after timeout, install/upgrade/recovery commands and failure-injection tests. None is production-qualified merely by being listed here.

Hub/Relay currently use `slog.JSONHandler` on stdout (`time`, `level`, `msg`). They do not consistently attach `service`, `buildVersion`, stable `event` or correlations. Raw error logging does not establish redaction.

The S0.3 shared initializer must attach `time`, `level`, `msg`, `service`, `buildVersion`, `event`; add request/interaction/activation/deployment/generation/control/resource/tool IDs, duration/outcome/errorCode only when applicable. Log route templates, not raw query strings. Supervisor collection owns rotation, bounded size/time retention and safe export; services do not share/rotate log files. No centralized log-search platform is required.

Never emit tokens, cookies, credentials, enrollment/session/signing material, private endpoints, toolRef/claims, raw prompts/bodies/tool arguments/results or direct personal identity. Test normal and failure diagnostics for forbidden material. References: [systemd service lifecycle](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html), [journald retention](https://www.freedesktop.org/software/systemd/man/252/journald.conf.html); documentation is not runtime qualification.

## 8. Troubleshooting and release gate

| Symptom | Safe first checks |
| --- | --- |
| Usage never arrives | URL must use Hub private port (default `8081`, not `8080`); inspect token, spool status and ingest errors |
| Relay not ready after restart | Inspect Hub reconcile and Relay applied revision; do not bypass authentication/inject state |
| Hub ready but runtime degraded | Compare desired/applied revision and activation; readiness is not convergence |
| `/admin` missing or deep links fail | Check actual static host/ingress; main supplies no `AdminAssets` |
| Migration/check disagreement | Inspect reviewed migration history/schema; the table check is incomplete |
| Repeated process crash | Preserve diagnostics/persistent data; no production restart-rate-limit package exists yet |

Upgrade must pin artifacts, verify applicable evidence, back up, apply reviewed migrations, deploy, validate readiness/control/static routing and run smoke/recovery checks. Binary downgrade is not assumed safe after schema changes. RC also needs isolated restore, spool replay, resource/load, supervision and log-redaction proof; see [release](release.md) and [testing](testing.md).
