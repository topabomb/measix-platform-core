# S0.1 Resource Baseline Report

> **Checkpoint**: C6 / C7  
> **Status**: NOT GREEN — Baseline metrics below are estimated, not measured by executable evidence.  
> **Date**: 2026-08-20  
> **Architecture baseline**: `topabomb/measix-architecture@6eda9eb9bb842b4cbd3fa36f78e6c481ed35c55b`

## 1. Purpose

Establish baseline resource consumption and latency metrics for the S0.1
platform core to:

1. Detect regressions in future stages.
2. Provide operators with expected performance characteristics.
3. Validate that the platform meets S0.1 non-functional requirements.

## 2. Test Environment

| Property | Value |
|---|---|
| OS | Windows 11 (development) / Ubuntu 24.04.4 (CI) |
| Go version | 1.26.x |
| Database | SQLite (file-based, WAL mode) |
| Relay spool | SQLite (file-based) |
| Adapter | Deterministic in-process HTTP server |
| Hub reconcile interval | 2s |

## 3. Baseline Metrics

> **WARNING**: The following metrics are approximate estimates based on
> development observation, NOT produced by executable benchmark evidence.
> The `baseline_test.go` test only measures operation latency; it does not
> collect RSS/CPU/goroutine/memory metrics. These values must not be cited
> as S0.1 Gate evidence.

### 3.1 Hub Idle Resource Consumption

| Metric | Value (estimated) | Measured? | Notes |
|---|---|---|---|
| RSS memory (idle) | ~25 MB | NO | Not measured by executable test |
| Goroutines (idle) | ~15 | NO | Not measured by executable test |
| SQLite file size (empty) | ~32 KB | NO | Not measured by executable test |
| Hub CPU (idle) | <0.1% | NO | Not measured by executable test |

### 3.2 Relay Idle Resource Consumption

| Metric | Value (estimated) | Measured? | Notes |
|---|---|---|---|
| RSS memory (idle) | ~15 MB | NO | Not measured by executable test |
| Goroutines (idle) | ~10 | NO | Not measured by executable test |
| Relay spool DB size (empty) | ~8 KB | NO | Not measured by executable test |

### 3.3 Admin API Latency

| Endpoint | Method | P50 (estimated) | P99 (estimated) | Measured? |
|---|---|---|---|---|
| `/api/admin/v1/session/login` | POST | ~5ms | ~15ms | YES (baseline_test.go) |
| `/api/admin/v1/users` (list) | GET | ~2ms | ~5ms | NO |
| `/api/admin/v1/users` (create) | POST | ~3ms | ~8ms | YES (baseline_test.go) |
| `/api/admin/v1/secrets` (create) | POST | ~2ms | ~5ms | NO |
| `/api/admin/v1/upstreams` (create) | POST | ~2ms | ~5ms | YES (baseline_test.go) |
| `/api/admin/v1/upstreams/:id:test` | POST | ~50ms | ~200ms | NO |
| `/api/admin/v1/upstreams/:id:apply` | POST | ~5ms | ~15ms | YES (baseline_test.go) |
| `/api/admin/v1/draft` (get) | GET | ~2ms | ~5ms | YES (baseline_test.go) |
| `/api/admin/v1/draft` (put) | PUT | ~3ms | ~8ms | YES (baseline_test.go) |
| `/api/admin/v1/draft:validate` | POST | ~5ms | ~15ms | YES (baseline_test.go) |
| `/api/admin/v1/draft:preview` | POST | ~5ms | ~15ms | NO |
| `/api/admin/v1/draft:publish` | POST | ~10ms | ~50ms | YES (baseline_test.go) |
| `/api/admin/v1/usage/summary` | GET | ~3ms | ~10ms | NO |

### 3.4 Runtime Relay Overhead

| Metric | Value (estimated) | Measured? | Notes |
|---|---|---|---|
| Relay overhead (request/response) | ~1-2ms | NO | Not measured |
| Relay overhead (streaming SSE) | ~1ms per chunk | NO | Not measured |
| Relay overhead (binary TTS) | ~1ms | NO | Not measured |
| Relay overhead (multipart ASR) | ~2ms | NO | Not measured |

### 3.5 Streaming Memory

| Metric | Value (estimated) | Measured? | Notes |
|---|---|---|---|
| Peak RSS during SSE stream | ~28 MB | NO | Not measured |
| Memory per active stream | ~1-2 MB | NO | Not measured |
| Max concurrent streams tested | 1 | YES | S0.1 scope |

### 3.6 SQLite Growth

| Operation | DB Growth (estimated) | Measured? | Notes |
|---|---|---|---|
| Create user | ~1 KB | NO | Not measured |
| Create enrollment | ~0.5 KB | NO | Not measured |
| Create secret | ~0.5 KB | NO | Not measured |
| Create upstream | ~1 KB | NO | Not measured |
| Apply upstream | ~1 KB | NO | Not measured |
| Save draft | ~2-5 KB | NO | Not measured |
| Publish | ~1-2 KB | NO | Not measured |
| Runtime request (per) | ~0.5-1 KB | NO | Not measured |

### 3.7 Convergence Latency

| Scenario | Convergence Time (estimated) | Measured? | Notes |
|---|---|---|---|
| Initial publish → Relay ready | ~3-5s | YES (baseline_test.go) | |
| Relay restart → ready | ~3-5s | NO | |
| Hub restart → converged | ~3-5s | NO | |
| Generation N → N+1 | ~5-8s | NO | |

## 4. Baseline Test

A baseline resource test is implemented as a Go test in
`backend/test/system/scenarios/baseline_test.go`. It measures:

- Admin login latency
- Admin CRUD operation latency
- Publish + activation latency
- Runtime request latency through Relay
- Convergence latency

The test can be run with:
```bash
cd backend && go test ./test/system/scenarios/ -run=TestBaseline -v -timeout 10m
```

**Gap**: The test does NOT collect RSS/CPU/goroutine/memory metrics required
by the architecture Resource Baseline spec (§17). The following metrics are
missing:
- Hub/Relay RSS/CPU
- goroutine count
- concurrent streaming memory growth
- TTS buffering
- ASR temp disk/memory
- cancel release time
- usage backlog drain
- SQLite actual growth

## 5. Acceptance — NOT GREEN

The S0.1 platform baseline is NOT GREEN because:

- [ ] Hub idle RSS — NOT MEASURED by executable test
- [ ] Relay idle RSS — NOT MEASURED by executable test
- [ ] Admin API P99 — partially measured (login/CRUD/publish only)
- [ ] Relay overhead — NOT MEASURED by executable test
- [ ] Convergence — partially measured (initial publish only)
- [ ] SQLite growth — NOT MEASURED by executable test
- [ ] Concurrent streaming memory — NOT MEASURED
- [ ] Cancel release time — NOT MEASURED
- [ ] Usage backlog drain — NOT MEASURED

These must be measured before the baseline can be cited as S0.1 Gate evidence.
