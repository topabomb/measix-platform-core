# Implementation Architecture

> Authority boundary: this document defines the implementation structure and dependency rules of `measix-platform-core`. Product semantics and S0 architecture remain authoritative in `topabomb/measix-architecture`.

## 1. Repository role

This repository implements three S0 logical components:

```text
Control Hub     → Go binary
Runtime Relay   → Go binary
Admin Console   → Quasar/Vue SPA build
```

It also owns the executable API contracts, database migrations, test/qualification infrastructure, CI and operational procedures needed to prove those components satisfy the architecture.

This document must not restate Publish semantics, Managed State semantics, stable ID meaning, Runtime admission rules, or S0 Exit requirements. Those belong to `measix-architecture`.

## 2. Target source layout

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
└── migrations/

console/

test/
├── qualification/
└── system/
```

The architecture repository may prescribe important boundaries, but the actual source tree in this repository is the authority for concrete package/file locations once implementation exists.

## 3. Dependency direction

### Control Hub

`control-hub` owns identity, capability draft/release state, desired runtime control, Admin/Client control APIs, usage ledger and persistence.

Its internal packages may depend on generated Admin/Client/Internal wire types and Hub persistence packages.

### Runtime Relay

`runtime-relay` is a separate binary and failure domain. It must not import Hub domain/service/Ent packages or access `hub.db`.

Permitted shared code between Hub and Relay is intentionally narrow:

- `backend/pkg/platformid` or equivalent pure identifier utility;
- generated wire types required by their direct protocol;
- generic helpers with no Hub business semantics;
- test-only helpers where production dependency direction is unchanged.

### Admin Console

The Admin Console consumes only the Admin OpenAPI surface and same-origin public paths. It does not call Relay internal APIs and does not define a second business-validation model.

## 4. Executable contract ownership

Architecture semantics flow into executable contracts as follows:

```text
measix-architecture
  semantic / state / error / security requirements
        ↓
api/*.openapi.yaml
  executable HTTP/wire shape
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
5. a wire-semantic change starts in the architecture repository; a non-semantic schema completion may be made here if it does not contradict architecture.

See `docs/api-contracts.md`.

## 5. Persistence ownership

Control Hub persistent application state uses SQLite + Ent with Atlas versioned migrations. Runtime Relay persists only its own durable runtime data such as the usage spool and does not share Hub ORM state.

The repository is authoritative for:

- actual Ent schemas;
- migration SQL and `atlas.sum`;
- database bootstrap/upgrade commands;
- migration tests and operational procedures.

The architecture repository remains authoritative for persistence invariants such as durability, ownership, atomicity and recovery semantics.

## 6. Test architecture

Tests mirror the S0 Testing Specs rather than duplicating them:

```text
T0 Static / Contract
T1 Unit / Domain
T2 Component Integration
T3 Cross-component Integration
T4 S0 System / E2E
```

This repository owns Hub, Relay and Admin component tests, Adapter qualification infrastructure, and the S0 system harness. Android component/instrumentation tests remain in `rikkahub_mcp`; T4 combines fixed commits from both repositories.

Required scenario semantics and IDs (`HUB-*`, `RLY-*`, `ADM-*`, `SYS-*`) come from `measix-architecture`. Test code here must reference those IDs where applicable.

See `docs/testing.md` and `docs/tdd.md`.

## 7. Change boundary

A change must first update `measix-architecture` when it alters any of these:

- platform terminology or stable identifier meaning;
- S0 scope or component ownership;
- cross-component state or lifecycle semantics;
- HTTP/wire/error/idempotency semantics;
- security/admission invariants;
- required Component/System Testing Spec behavior.

A change stays in this repository when it only changes implementation without changing those meanings, for example package refactoring, DB indexing, HTTP implementation details, CI optimization, test helpers, local tooling or UI component decomposition.

When implementation reveals an architectural ambiguity, do not choose a new semantic locally. Resolve the authority document first, then implement it here.
