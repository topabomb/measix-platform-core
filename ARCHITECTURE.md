# Implementation Architecture

> Authority boundary: this document defines the implementation structure and dependency rules of `measix-platform-core`. Product semantics and S0 architecture remain authoritative in `topabomb/measix-architecture`.

## 1. Repository role

This repository implements three S0 logical components:

```text
Control Hub     → Go binary
Runtime Relay   → Go binary
Admin Console   → Quasar/Vue SPA build
```

It also owns executable API contracts, database migrations, qualification/system-test infrastructure, CI and operational procedures needed to prove those components satisfy architecture.

This document must not restate Publish semantics, Managed State semantics, stable ID meaning, Runtime admission rules or S0 Exit requirements. Those belong to `measix-architecture`.

## 2. Source ownership and current layout

The architecture implementation decisions define logical repository responsibilities. The **actual source tree is authoritative for concrete physical file locations** once implementation exists.

Current implementation is organized as:

```text
api/
├── admin/admin.openapi.yaml
├── client/client-control.openapi.yaml
├── internal/relay-control.openapi.yaml
├── internal/usage-ingest.openapi.yaml
└── fixtures/

backend/
├── cmd/control-hub/
├── cmd/runtime-relay/
├── pkg/platformid/
├── internal/hub/
├── internal/relay/
├── ent/
├── migrations/
└── test/system/          # current Go-module system harness

console/
├── src/
└── e2e/                 # browser E2E ownership when/where executable tests land

docs/
```

The S0.1 architecture decision describes the logical `test/system` responsibility for deterministic Adapter, Test Client, system scenarios and reports. The current Go harness is physically under `backend/test/system/` so it can execute inside the backend Go module and reuse permitted test/internal implementation boundaries. Documentation must describe this fact rather than pretend a different directory already exists. If C6/C7 later require a physical repository-level move, make that implementation change explicitly; do not create a documentation-only layout.

## 3. Dependency direction

### Control Hub

`control-hub` owns identity, capability draft/release state, desired runtime control, Admin/Client control APIs, usage ledger and Hub persistence.

Its internal packages may depend on generated Admin/Client/Internal wire types and Hub persistence packages.

### Runtime Relay

`runtime-relay` is a separate binary and failure domain. It must not import Hub domain/service/Ent packages or access `hub.db`.

Permitted shared production code between Hub and Relay is intentionally narrow:

- `backend/pkg/platformid` or equivalent pure identifier utility;
- generated wire types required by their direct protocol;
- generic helpers with no Hub business semantics.

Test-only helpers are allowed where production dependency direction remains unchanged.

### Admin Console

Admin consumes only the Admin OpenAPI surface and same-origin public paths. It does not call Relay internal APIs and does not define a second business-validation model.

## 4. Executable contract ownership

```text
measix-architecture
  semantic / state / error / security requirements
        ↓
api/*.openapi.yaml
  exact executable HTTP/wire shape
        ↓
code generation + canonical fixtures
        ↓
Go / TypeScript / Android consumers
```

Rules:

1. `measix-architecture` remains authoritative for meaning.
2. `api/*.openapi.yaml` is authoritative for exact executable HTTP schema in this repository.
3. generated code is never manually edited.
4. canonical cross-component fixtures live only under `api/fixtures/`.
5. a semantic wire change starts in architecture; a non-semantic schema completion may be implemented locally only when it cannot change client interpretation.

See `docs/api-contracts.md`.

## 5. Persistence ownership

Control Hub persistent application state uses SQLite + Ent with Atlas versioned migrations. Runtime Relay persists only its own durable runtime data such as usage spool and does not share Hub ORM state.

This repository is authoritative for actual Ent schemas, migration SQL/`atlas.sum`, bootstrap/upgrade commands and migration tests. Architecture remains authoritative for durability, ownership, atomicity and recovery invariants.

## 6. Test architecture

Executable tests mirror architecture gates rather than duplicate their semantics:

```text
T0 Static / Contract
T1 Unit / Domain
T2 Component Integration
T3 Cross-component Integration
T4.1 S0.1 pre-Android Product/System E2E
T4 final S0 Android/System RC
```

This repository owns Hub, Relay and Admin component tests, Adapter qualification infrastructure, the deterministic/system harness and S0.1 browser/product evidence. Android component/instrumentation tests remain in `rikkahub_mcp`; final S0 T4 combines pinned commits from both repositories.

Critical scenario semantics/IDs come from architecture, including:

```text
HUB-*   Control Hub
RLY-*   Runtime Relay
ADM-*   Admin Console
CAP-*   S0.1 Capability Delivery
AND-*   S0.2 Android integration
SYS-*   final S0 system/RC
```

Tests here reference those IDs where applicable; this repository must not invent new cross-component product semantics locally.

Default GitHub Actions CI deliberately proves deterministic T0–T3 only. T4.1 real-browser candidate verification and real external Adapter qualification are explicit S0.1 promotion gates; a Green `ci-gate` is not C6/C7/S0.1 Freeze evidence.

See `docs/testing.md` and `docs/tdd.md`.

## 7. Documentation/evidence boundary

- `docs/s0-execution-progress.md` is the living implementation-status document.
- `docs/s0-review-report.md` is historical audit/regression evidence tied to its recorded baseline; it is not current checkpoint authority.
- a valid `docs/s0-freeze-manifest.json` may exist only after the architecture-defined C7 gate actually passes on an exact candidate SHA. Placeholder/stale manifests are not retained as current evidence.

## 8. Change boundary

Update `measix-architecture` first when a change alters:

- platform terminology or stable identifier meaning;
- S0 scope or component ownership;
- cross-component state/lifecycle semantics;
- HTTP/wire/error/idempotency semantics;
- security/admission invariants;
- required Component/System Testing Spec behavior.

Keep the change in this repository when it only changes implementation while preserving those meanings, such as package refactoring, DB indexing, HTTP implementation details, CI optimization, test helpers, local tooling or UI component decomposition.

When implementation reveals architectural ambiguity, do not choose a new semantic locally. Resolve the owning architecture authority first, then implement it here.
