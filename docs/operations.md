# Operations

This document owns the executable operating procedures for `measix-platform-core`. Architecture defines operational invariants and component semantics; this document records how the built software is configured, started, observed, backed up, restored and upgraded.

## 1. Scope

Production S0 server-side artifacts are:

```text
control-hub     long-running Go process
runtime-relay   long-running Go process
Admin Console   static SPA build served by Control Hub/Ingress
```

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
- static Admin build assets;
- service credentials/configuration;
- transient logs/temp/test data.

Concrete paths are documented here when implementation fixes them. Paths in source/config are the final implementation truth.

## 4. Health and readiness

Both server binaries expose liveness/readiness endpoints according to their component implementation specs.

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
→ start Control Hub / Runtime Relay
→ wait for bounded readiness
→ verify desired/applied runtime state where applicable
→ expose traffic
```

Runtime Relay restart begins fail-closed until valid control state is rehydrated, as defined by architecture. This document will record the exact commands once the binaries/tooling exist.

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

## 9. Observability

As implementation lands, document:

- structured log fields and correlation IDs;
- health/status endpoints;
- relevant queue/spool/backlog indicators;
- build/version identity;
- diagnostics that are safe to collect in CI/operations.

Never expose credentials, private signing material or secret plaintext through logs/status endpoints.

## 10. Incident/troubleshooting entries

Troubleshooting belongs here only for implementation/operations facts such as:

- migration failure;
- Relay not ready after restart;
- control revision mismatch;
- usage backlog not draining;
- Admin static asset/routing failure.

If troubleshooting requires explaining what a platform state *means*, link to `measix-architecture` instead of creating a local alternate semantic description.

## 11. RC operational proof

Before RC, operations are considered ready only when system tests exercise:

- clean deployment/bootstrap;
- migration replay/upgrade;
- Hub/Relay restart;
- backup/restore;
- usage replay;
- target-resource/load checks;
- version/build/manifest traceability.

See `docs/release.md` and `docs/testing.md`.
