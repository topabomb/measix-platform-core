# S0.1 Resource Baseline Report

> **Checkpoint**: C6 / C7  
> **Status**: NOT GREEN — No baseline metrics have been measured by executable evidence. All values below are `NOT MEASURED`.  
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

> All metrics below are NOT MEASURED. Estimated values have been removed
> because they have no engineering value and risk being cited as baseline
> evidence. Each metric must be produced by an executable test before it
> can appear in this report.

### 3.1 Hub Idle Resource Consumption

| Metric | Value | Measured? |
|---|---|---|
| RSS memory (idle) | NOT MEASURED | NO |
| Goroutines (idle) | NOT MEASURED | NO |
| SQLite file size (empty) | NOT MEASURED | NO |
| Hub CPU (idle) | NOT MEASURED | NO |

### 3.2 Relay Idle Resource Consumption

| Metric | Value | Measured? |
|---|---|---|
| RSS memory (idle) | NOT MEASURED | NO |
| Goroutines (idle) | NOT MEASURED | NO |
| Relay spool DB size (empty) | NOT MEASURED | NO |

### 3.3 Admin API Latency

| Endpoint | Method | P50 | P99 | Measured? |
|---|---|---|---|---|
| `/api/admin/v1/session/login` | POST | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/users` (list) | GET | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/users` (create) | POST | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/secrets` (create) | POST | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/upstreams` (create) | POST | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/upstreams/:id:test` | POST | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/upstreams/:id:apply` | POST | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/draft` (get) | GET | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/draft` (put) | PUT | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/draft:validate` | POST | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/draft:preview` | POST | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/draft:publish` | POST | NOT MEASURED | NOT MEASURED | NO |
| `/api/admin/v1/usage/summary` | GET | NOT MEASURED | NOT MEASURED | NO |

### 3.4 Runtime Relay Overhead

| Metric | Value | Measured? |
|---|---|---|
| Relay overhead (request/response) | NOT MEASURED | NO |
| Relay overhead (streaming SSE) | NOT MEASURED | NO |
| Relay overhead (binary TTS) | NOT MEASURED | NO |
| Relay overhead (multipart ASR) | NOT MEASURED | NO |

### 3.5 Streaming Memory

| Metric | Value | Measured? |
|---|---|---|
| Peak RSS during SSE stream | NOT MEASURED | NO |
| Memory per active stream | NOT MEASURED | NO |
| Max concurrent streams tested | 1 | YES (S0.1 scope) |

### 3.6 SQLite Growth

| Operation | DB Growth | Measured? |
|---|---|---|
| Create user | NOT MEASURED | NO |
| Create enrollment | NOT MEASURED | NO |
| Create secret | NOT MEASURED | NO |
| Create upstream | NOT MEASURED | NO |
| Apply upstream | NOT MEASURED | NO |
| Save draft | NOT MEASURED | NO |
| Publish | NOT MEASURED | NO |
| Runtime request (per) | NOT MEASURED | NO |

### 3.7 Convergence Latency

| Scenario | Convergence Time | Measured? |
|---|---|---|
| Initial publish → Relay ready | NOT MEASURED | NO |
| Relay restart → ready | NOT MEASURED | NO |
| Hub restart → converged | NOT MEASURED | NO |
| Generation N → N+1 | NOT MEASURED | NO |

## 4. Baseline Test

A baseline resource test is implemented as a Go test in
`backend/test/system/scenarios/baseline_test.go`. It currently measures
operation latency only (login, CRUD, publish, convergence). It does NOT
collect the RSS/CPU/goroutine/memory metrics required by the architecture
Resource Baseline spec (§17).

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
- [ ] Admin API P99 — NOT MEASURED by executable test
- [ ] Relay overhead — NOT MEASURED by executable test
- [ ] Convergence — NOT MEASURED by executable test
- [ ] SQLite growth — NOT MEASURED by executable test
- [ ] Concurrent streaming memory — NOT MEASURED
- [ ] Cancel release time — NOT MEASURED
- [ ] Usage backlog drain — NOT MEASURED

These must be measured before the baseline can be cited as S0.1 Gate evidence.
