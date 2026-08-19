# API Contracts, Fixtures and Code Generation

This document defines executable-contract ownership in `measix-platform-core`. Semantic meaning remains authoritative in `topabomb/measix-architecture`.

## 1. S0 OpenAPI surfaces

I0 owns four separate OpenAPI 3.0.3 documents:

```text
api/admin/admin.openapi.yaml
api/client/client-control.openapi.yaml
api/internal/relay-control.openapi.yaml
api/internal/usage-ingest.openapi.yaml
```

They are separated deliberately so Admin/Android consumers do not accidentally generate or depend on Relay-internal APIs.

## 2. Authority boundary

Use this rule when a Markdown statement and OpenAPI detail appear to overlap:

- architecture repository: **meaning** — lifecycle, state semantics, security, error meaning, idempotency, required behavior;
- OpenAPI here: **exact executable shape** — method, path, required/optional fields, types, enum values, request/response schema;
- generated code: derived consumer representation only.

If an exact schema choice would change architecture meaning, it is not a local OpenAPI detail; resolve architecture first.

## 3. Canonical fixtures

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
- no production credential or user data.

Fixtures change in the same commit as the OpenAPI change they represent.

## 4. Code generation

Expected consumers:

- Go server/client types for Hub/Relay surfaces;
- TypeScript Admin API types;
- deterministic Android Client wire generation/export from `client-control.openapi.yaml`.

Generator configuration/version is repository-controlled and reproducible. Generated files contain a clear generated marker and are not manually edited.

CI must be able to regenerate from a clean checkout and fail on unexpected drift.

## 5. Contract-change workflow

### Semantic wire change

```text
architecture contract change
→ OpenAPI
→ fixtures
→ generated artifacts
→ component tests
→ affected T3/T4 tests
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

## 6. Compatibility rules

S0 contract tests must preserve architecture rules including:

- clients tolerate newly added unknown optional response fields;
- undeclared request fields are rejected where the S0 protocol requires strict request decoding;
- programs branch on HTTP status + stable Problem `code`, not human `detail` text;
- stable identifier format/ownership is not redefined by generated DTOs;
- internal APIs never leak into Android/Admin generated clients.

## 7. Review requirements

An OpenAPI PR must answer:

1. which architecture requirement it implements;
2. whether semantics changed;
3. which fixtures changed;
4. which generated consumers changed;
5. which T0/T1/T2/T3/T4 lanes are affected;
6. whether Android synchronization is required;
7. whether the change is backward-compatible for existing S0 clients.

## 8. CI gate

The T0 contract gate must eventually include:

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

The implementation tooling may split these into jobs, but the required PR aggregate gate must always report a result for every PR.
