# API Contracts, Fixtures and Code Generation

This document defines executable-contract ownership in `measix-platform-core`. Semantic meaning remains authoritative in `topabomb/measix-architecture`.

## 1. S0 OpenAPI surfaces

The repository owns four separate OpenAPI 3.0.3 documents:

```text
api/admin/admin.openapi.yaml
api/client/client-control.openapi.yaml
api/internal/relay-control.openapi.yaml
api/internal/usage-ingest.openapi.yaml
```

They are separated deliberately so Admin/Android consumers do not accidentally generate or depend on Relay-internal APIs.

## 2. Authority boundary

Use this rule when a Markdown statement and OpenAPI detail appear to overlap:

- architecture repository: **meaning** — lifecycle, state semantics, Managed Capability profile, security, error meaning, idempotency, delivery/Exit gates and required behavior;
- OpenAPI here: **exact executable shape** — method, path, required/optional fields, types, enum values, request/response schema;
- generated code: derived consumer representation only.

If an exact schema choice would change architecture meaning, it is not a local OpenAPI detail; resolve architecture first.

## 3. Current S0.1 contract state

The current delivery target is S0.1 Managed Capability Delivery. Until the architecture-defined S0.1 Client Contract Freeze Gate passes, the Client Control OpenAPI and Snapshot v1 are **pre-freeze executable contracts**.

This has two practical consequences:

1. current code/tests may prove earlier S0 Core semantics without proving the final S0.1 Android-facing contract is complete;
2. architecture-approved S0.1 schema corrections must be implemented here before Android S0.2 begins, together with fixtures, generated artifacts and executable tests.

The exact required S0.1 Managed Capability profiles and fields are owned by:

- `measix-s0-capability-delivery-contract-spec.md`;
- `measix-s0-control-protocol.md`;
- relevant component specs in `topabomb/measix-architecture`.

Do not duplicate their enum/field lists in this document. `docs/s0-execution-progress.md` records the current implementation gaps against that authority.

## 4. Canonical fixtures

Cross-component fixtures live only under:

```text
api/fixtures/
├── problem/
├── identity/
├── managed-state/
├── draft/
├── snapshot/
├── runtime-control/
└── usage/
```

The fixture taxonomy may grow with implemented S0 contracts, but consumers must not create divergent private copies of the same wire truth.

Required fixture qualities:

- valid minimal and representative payloads;
- invalid request unknown-field samples where required by contract;
- response samples containing unknown optional fields for forward compatibility;
- deterministic Snapshot/RuntimeControl canonicalization/hash golden data;
- S0.1 representative fixtures for every required Managed Capability profile before freeze;
- no production credential or user data.

Fixtures change in the same commit as the OpenAPI change they represent.

## 5. Code generation

Expected consumers:

- Go server/client types for Hub/Relay surfaces;
- TypeScript Admin API types;
- deterministic Android Client wire generation/export from `client-control.openapi.yaml`.

Generator configuration/version is repository-controlled and reproducible. Generated files contain a clear generated marker and are not manually edited.

CI must be able to regenerate from a clean checkout and fail on unexpected drift.

## 6. Contract-change workflow

### Semantic wire change

```text
architecture contract change
→ OpenAPI
→ fixtures
→ generated artifacts
→ component tests
→ affected S0.1/S0.2/T3/T4 tests
→ affected external consumer repository
```

### Non-semantic completion

For a detail that architecture intentionally leaves to executable schema and that does not change meaning:

```text
OpenAPI + fixture
→ generated artifacts
→ contract tests
→ consumers
```

If implementation discovers that clients could reasonably interpret the detail in incompatible ways, treat it as semantic and return to architecture.

## 7. S0.1 Client Contract Freeze

The S0.1 freeze is an executable milestone, not a Markdown declaration.

Before S0.2 Android implementation starts, this repository must have:

- Client/Admin/Internal OpenAPI aligned with the merged S0.1 architecture baseline;
- canonical fixtures for all Android-visible Snapshot resource kinds and policy behavior;
- deterministic generation/drift checks Green;
- Snapshot preview compiled from the same canonical projection as Release Snapshot;
- deterministic S0.1 system-gate evidence;
- required real Adapter qualification evidence;
- a freeze manifest that pins the architecture commit, platform-core commit, Client OpenAPI hash, canonical fixture hash and Snapshot schema version.

After that manifest is produced, the pinned Client OpenAPI/fixture set is the S0.2 handoff input. An incompatible Android-visible change cannot silently mutate the frozen v1 contract; follow the compatibility/versioning semantics defined by architecture and create a new freeze candidate.

## 8. Compatibility rules

S0 contract tests must preserve architecture rules including:

- clients tolerate newly added unknown optional response fields;
- undeclared request fields are rejected where the S0 protocol requires strict request decoding;
- programs branch on HTTP status + stable Problem `code`, not human `detail` text;
- stable identifier format/ownership is not redefined by generated DTOs;
- internal APIs never leak into Android/Admin generated clients;
- `runtimeRouteId`, Upstream internal URL and Secret material never enter the Client Snapshot;
- unsupported/future client protocol behavior is explicit rather than silent fallback.

## 9. Review requirements

An OpenAPI PR must answer:

1. which architecture requirement it implements;
2. whether semantics changed;
3. whether the change is pre-freeze S0.1 work or modifies an already frozen handoff;
4. which fixtures changed;
5. which generated consumers changed;
6. which T0/T1/T2/T3/T4 or `CAP-*` lanes are affected;
7. whether Android synchronization is required;
8. whether the change is backward-compatible for existing/frozen S0 clients.

## 10. CI gate

The T0 contract gate must include or evolve to include:

```text
OpenAPI parse/validation
codegen reproducibility
generated-code drift
canonical fixture validation
invalid fixture rejection
Snapshot/RuntimeControl hash golden verification
Admin production typecheck against generated API types
Android export/generation compatibility for client-control contract
```

S0.1 adds a freeze check that computes and records the Client OpenAPI/fixture identities used by the handoff manifest. The implementation tooling may split these into jobs, but the required PR aggregate gate must always report a result for every PR.
