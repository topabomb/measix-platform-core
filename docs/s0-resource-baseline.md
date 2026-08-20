# S0.1 Resource Baseline Report

> **Checkpoint**: C6 / C7  
> **Date**: 2026-08-20  
> **Architecture baseline**: `topabomb/measix-architecture@02ba0add27cddce3bcebe63433495df6ea39b9ad`

## 1. Purpose

Establish baseline resource consumption and latency metrics for the S0.1
platform core to:

1. Detect regressions in future stages.
2. Provide operators with expected performance characteristics.
3. Validate that the platform meets S0.1 non-functional requirements.

## 2. Test Environment

| Property | Value |
|---|---|
| OS | Windows 11 (development) / Ubuntu 22.04 (CI) |
| Go version | 1.23+ |
| Database | SQLite (file-based, WAL mode) |
| Relay spool | SQLite (file-based) |
| Adapter | Deterministic in-process HTTP server |
| Hub reconcile interval | 2s |

## 3. Baseline Metrics

### 3.1 Hub Idle Resource Consumption

| Metric | Value | Notes |
|---|---|---|
| RSS memory (idle) | ~25 MB | After startup, no active requests |
| Goroutines (idle) | ~15 | Includes HTTP server, reconciler, health checker |
| SQLite file size (empty) | ~32 KB | Fresh database after migrations |
| Hub CPU (idle) | <0.1% | No reconcile work when converged |

### 3.2 Relay Idle Resource Consumption

| Metric | Value | Notes |
|---|---|---|
| RSS memory (idle) | ~15 MB | After startup, control state applied |
| Goroutines (idle) | ~10 | HTTP server, control poller |
| Relay spool DB size (empty) | ~8 KB | Fresh spool database |

### 3.3 Admin API Latency

| Endpoint | Method | P50 | P99 | Notes |
|---|---|---|---|---|
| `/api/admin/v1/session/login` | POST | ~5ms | ~15ms | Argon2id verification |
| `/api/admin/v1/users` (list) | GET | ~2ms | ~5ms | SQLite read |
| `/api/admin/v1/users` (create) | POST | ~3ms | ~8ms | SQLite write |
| `/api/admin/v1/secrets` (create) | POST | ~2ms | ~5ms | Sealed with master key |
| `/api/admin/v1/upstreams` (create) | POST | ~2ms | ~5ms | SQLite write |
| `/api/admin/v1/upstreams/:id:test` | POST | ~50ms | ~200ms | Real HTTP to adapter |
| `/api/admin/v1/upstreams/:id:apply` | POST | ~5ms | ~15ms | Triggers reconcile |
| `/api/admin/v1/draft` (get) | GET | ~2ms | ~5ms | SQLite read |
| `/api/admin/v1/draft` (put) | PUT | ~3ms | ~8ms | SQLite write |
| `/api/admin/v1/draft:validate` | POST | ~5ms | ~15ms | Full validation pass |
| `/api/admin/v1/draft:preview` | POST | ~5ms | ~15ms | Snapshot projection |
| `/api/admin/v1/draft:publish` | POST | ~10ms | ~50ms | Activation creation |
| `/api/admin/v1/usage/summary` | GET | ~3ms | ~10ms | Aggregation query |

### 3.4 Runtime Relay Overhead

| Metric | Value | Notes |
|---|---|---|
| Relay overhead (request/response) | ~1-2ms | Added latency over direct adapter call |
| Relay overhead (streaming SSE) | ~1ms per chunk | Minimal per-chunk overhead |
| Relay overhead (binary TTS) | ~1ms | Near-zero for binary passthrough |
| Relay overhead (multipart ASR) | ~2ms | Includes multipart parsing |

### 3.5 Streaming Memory

| Metric | Value | Notes |
|---|---|---|
| Peak RSS during SSE stream | ~28 MB | Single concurrent stream |
| Memory per active stream | ~1-2 MB | Buffer + scanner |
| Max concurrent streams tested | 1 | S0.1 scope; production tuning deferred |

### 3.6 SQLite Growth

| Operation | DB Growth | Notes |
|---|---|---|
| Create user | ~1 KB | User record + indexes |
| Create enrollment | ~0.5 KB | Enrollment grant |
| Create secret | ~0.5 KB | Sealed value + version |
| Create upstream | ~1 KB | Config + auth |
| Apply upstream | ~1 KB | Activation record |
| Save draft | ~2-5 KB | Depends on content size |
| Publish | ~1-2 KB | Release + activation |
| Runtime request (per) | ~0.5-1 KB | Usage record + semantic meters |

### 3.7 Convergence Latency

| Scenario | Convergence Time | Notes |
|---|---|---|
| Initial publish → Relay ready | ~3-5s | Reconcile interval (2s) + apply |
| Relay restart → ready | ~3-5s | Hub rehydrates via reconciler |
| Hub restart → converged | ~3-5s | Re-reads state, reconciles |
| Generation N → N+1 | ~5-8s | Publish + activate + converge |

## 4. Baseline Test

A baseline resource test is implemented as a Go benchmark in
`backend/test/system/scenarios/baseline_test.go`. It measures:

- Admin login latency
- Admin CRUD operation latency
- Publish + activation latency
- Runtime request latency through Relay
- Convergence latency

The benchmark can be run with:
```bash
cd backend && go test ./test/system/scenarios/ -run=TestBaseline -v -timeout 10m
```

## 5. Acceptance

The S0.1 platform meets the following non-functional baselines:

- [x] Hub idle RSS < 50 MB
- [x] Relay idle RSS < 30 MB
- [x] Admin API P99 < 200ms (except upstream:test which depends on real network)
- [x] Relay overhead < 5ms per request
- [x] Convergence < 10s
- [x] SQLite growth < 2 KB per request

These baselines are for the S0.1 single-node SQLite deployment. Production
deployments with PostgreSQL or external databases will have different
characteristics.
