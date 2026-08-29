# Operations

This document owns the executable operating procedures for `measix-platform-core`. Architecture defines operational invariants and component semantics; this document records how the built software is configured, started, observed, backed up, restored and upgraded.

## 1. Scope

Architecture target production S0 server-side artifacts are:

```text
control-hub     long-running Go process
runtime-relay   long-running Go process
enterprise-tool-gateway long-running Go process (S0.3; not implemented yet)
Admin Console   static SPA build served by Control Hub/Ingress
```

Current source contains only `backend/cmd/control-hub` and `backend/cmd/runtime-relay`. No Gateway binary, production service units or packaging exist yet. `npm start`/`concurrently`, `go run` commands and both Node/Go system harnesses are development/test orchestration, not a production process manager.

This document must not redefine product state such as Publish, Activation or Managed Generation. It documents the operational actions around the implementation.

## 2. Configuration ownership

The complete implemented configuration surface belongs to source/config definitions plus this document. Architecture documents may require specific categories of configuration but should not maintain a duplicate exhaustive environment-variable list.

When configuration code lands, document each production option here with:

```text
name
component
required/default
format/range
secret? yes/no
restart required?
operational effect
```

Do not document an environment variable until it actually exists in code.

Production secrets are referenced through configured secret material/files/services as implemented; plaintext values never belong in Git, docs, CI logs or test artifacts.

## 3. Filesystem/persistence ownership

At minimum operations must distinguish:

- Control Hub database and its migration/backup lifecycle;
- Runtime Relay local durable spool and its restart/replay lifecycle;
- Enterprise Tool Gateway rebuildable applied state/index/session data once implemented;
- static Admin build assets;
- service credentials/configuration;
- transient logs/temp/test data.

Concrete paths are documented here when implementation fixes them. Paths in source/config are the final implementation truth.

## 4. Health and readiness

Every production daemon exposes liveness/readiness/status according to its component implementation spec.

Operational checks must distinguish:

- process alive;
- component ready to serve its public/internal responsibility;
- degraded subsystems such as control synchronization or usage spool;
- version/build identity.

A load balancer/readiness probe must not convert a meaningful degraded state into a false “healthy” interpretation.

## 5. Startup

The production start sequence must be reproducible from a clean deployment:

```text
validate configuration
→ apply/verify required DB migrations
→ start Control Hub / Enterprise Tool Gateway / Runtime Relay
→ wait for bounded readiness
→ verify desired/applied runtime state where applicable
→ expose traffic
```

Runtime Relay and Enterprise Tool Gateway restart fail closed until valid control state is rehydrated, as defined by architecture. This document records exact commands only when binaries/tooling exist.

## 5.1 S0.3 production supervision target

S0.3 must implement the architecture supervision contract using host-native service management. The reference package is Linux `systemd` + `journald`; an equivalent platform supervisor is acceptable only when the same executable behavior is proven.

Implementation deliverables, not yet present, must include:

- one unit per daemon plus one aggregate target/group for operator start/stop/status;
- installed, immutable binaries with build identity; no `go run`, Admin dev server or `concurrently`;
- dedicated least-privilege identity, explicit config/credential/persistence paths and private/public bind boundaries;
- startup ordering without treating ordering as readiness; bounded readiness checks before exposing traffic;
- `on-failure` restart with bounded delay/backoff and start-rate limiting; permanent config/migration failure classification that prevents an endless crash loop;
- independent failure domains: one daemon crash does not automatically restart every daemon;
- SIGTERM graceful drain, explicit stop timeout and supervisor force-kill only after timeout;
- install/start/stop/restart/status/upgrade/uninstall and failure-recovery runbook commands;
- executable tests for clean boot, crash restart, rate limit, config failure, graceful stop and Gateway/Relay rehydrate.

Concrete unit names, `After`/`Requires` relationships, environment/config files, exit codes and timeout values are deliberately deferred until the implementation exists. Do not copy illustrative values from architecture into fake operations facts.

## 6. Graceful shutdown

Operational stop/restart uses the binaries' graceful shutdown behavior rather than process killing as the normal path.

Tests and runbooks must cover:

- stop accepting new work;
- bounded drain;
- cancellation after grace timeout;
- durable data/spool preservation;
- restart and readiness recovery.

Exact signals/timeouts are documented here when implemented/configurable.

## 7. Backup and restore

Before S0 RC, repository tooling and this document must provide a tested Control Hub backup/restore procedure.

The procedure must prove that after restore the facts required by the S0 System Testing Spec remain correct, including stable IDs, releases/generations and usage state where applicable.

A backup is not considered valid merely because a database file exists; restore must be exercised in automated/system testing.

Relay spool backup is not a substitute for its own durable replay semantics. Operational handling should avoid silently discarding pending usage.

## 8. Upgrade

Upgrade procedure includes:

```text
pin release artifacts
→ verify architecture/release manifest
→ backup as required
→ apply migrations
→ deploy binaries/static assets
→ readiness/control-state verification
→ smoke/system checks
```

If rollback requires database restore or forward migration rather than binary downgrade, the release documentation must say so explicitly.

## 9. Observability and production log contract

Current Hub and Relay instantiate Go `slog.JSONHandler` on stdout, so output is line-delimited JSON with standard `time`/`level`/`msg`. This is only a partial baseline: the current mains do not consistently attach `service`, `buildVersion`, stable `event`, lifecycle transitions or correlation fields, and no production collector/retention package exists.

S0.3 implementation must use a shared logging initializer that attaches to every record:

```text
time
level
msg
service          control-hub | runtime-relay | enterprise-tool-gateway
buildVersion
event            stable machine-searchable event name
```

Add only when applicable: `requestId`, `interactionId`, `activationId`, `deploymentId`, `managedGeneration`, `controlRevision`, `gatewayControlRevision`, `resourceId`, `gatewayToolId`, `durationMs`, `outcome`, `errorCode`. Use a route template instead of raw URL/query. Stable event families cover process lifecycle, readiness transition, control apply/reconcile, degraded/recovery, request completion and security rejection. INFO is the production default; DEBUG is explicit and obeys identical redaction.

All daemons write stdout/stderr only. `journald` (or equivalent supervisor collector) owns collection, rotation and retention; applications do not share or rotate log files. The package/runbook must define safe service-scoped collection, time/size retention and export commands when the units land.

Never log Authorization/Cookie/Secret/credential/enrollment/session material, private signing material, private endpoint, toolRef or claims, full prompt/request/response body, tool arguments/results, or direct identity such as username/email. Status endpoints follow the same boundary. Automated tests must search normal/failure diagnostics for forbidden material.

Also document health/status endpoints, queue/spool/backlog indicators and build/version identity as implementation lands. A centralized log/search/alerting stack is outside S0.3; safe host collection is required.

## 10. Incident/troubleshooting entries

Troubleshooting belongs here only for implementation/operations facts such as:

- migration failure;
- Relay not ready after restart;
- Gateway not ready after restart or gatewayControl revision mismatch;
- supervisor restart-rate limit reached or permanent configuration failure;
- control revision mismatch;
- usage backlog not draining;
- Admin static asset/routing failure.

If troubleshooting requires explaining what a platform state *means*, link to `measix-architecture` instead of creating a local alternate semantic description.

## 11. RC operational proof

Before RC, operations are considered ready only when system tests exercise:

- clean deployment/bootstrap;
- migration replay/upgrade;
- independent Hub/Gateway/Relay restart plus aggregate lifecycle;
- supervisor restart/rate-limit/graceful-stop behavior;
- JSON log collection, correlation and forbidden-field redaction;
- backup/restore;
- usage replay;
- target-resource/load checks;
- version/build/manifest traceability.

See `docs/release.md` and `docs/testing.md`.
