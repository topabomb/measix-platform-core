# API Contracts, Fixtures and Code Generation

This document defines executable-contract ownership in `measix-platform-core`. Semantic meaning remains authoritative in `topabomb/measix-architecture`.

## 1. Current and planned S0 OpenAPI surfaces

The current source owns four OpenAPI 3.0.3 documents:

```text
api/admin/admin.openapi.yaml
api/client/client-control.openapi.yaml
api/internal/relay-control.openapi.yaml
api/internal/usage-ingest.openapi.yaml
```

They are separated so Admin/Android consumers do not accidentally generate or depend on Relay-internal APIs.

S0.3 architecture additionally requires a private Gateway Control surface, expected at:

```text
api/internal/gateway-control.openapi.yaml
```

It does not exist at the current implementation head. Do not generate types, claim S0.3 contract coverage or add ad-hoc structs until the architecture-authorized schema is implemented through OpenAPI, fixtures, generated types and tests.

## 2. Authority boundary

- architecture repository: lifecycle/state/security/error/idempotency meaning, Managed Capability profile, delivery gates and required behavior;
- OpenAPI here: exact executable HTTP shape — method, path, required/optional fields, types, enums and request/response schema;
- generated code: derived representation only.

If an exact schema choice can change client interpretation, resolve architecture first.

## 3. Versioned contract state

The historical `docs/s0-freeze-manifest.json` declares a Snapshot v1 candidate; acceptance requires validating its pinned artifact chain. Current code compiles Snapshot v2, even without Assistant entries. Retaining v1 fixtures/manifest does not make the current server a v1 freeze or validate the current branch.

Snapshot v2 Realm/Experience additions and Snapshot v3 Gateway additions are forward profile extensions with separate product/freeze gates. Do not duplicate the complete required field/enum list in this document; use:

- `measix-s0-capability-delivery-contract-spec.md`;
- `measix-s0-enterprise-realm-experience-contract-spec.md`;
- `measix-s0-enterprise-tool-gateway-contract-spec.md`;
- `measix-s0-control-protocol.md`;
- relevant component/product/testing specs.

Current stage status is maintained only in `docs/s0-execution-progress.md`; the [alignment audit](architecture-alignment-audit.md) is a dated source/evidence snapshot.

## 4. Canonical fixtures

Cross-component fixtures live only under `api/fixtures/` and must cover valid representative payloads, required invalid/strict-decoding cases, forward-compatible response behavior, deterministic Snapshot/RuntimeControl canonicalization and all S0.1 required Managed Capability profiles.

Fixtures change in the same commit as the executable contract they represent. They must never contain production credentials or user data.

Distinguish fixture coverage from complete runtime validation: unmarshalling into generated Go types does not by itself enforce every OpenAPI required/format/enum/additionalProperties rule. Contract tests, HTTP decoding and domain validation must collectively prove each required constraint.

## 5. Code generation

Expected consumers include:

- Go server/client types for Hub/Relay surfaces;
- TypeScript Admin API types;
- deterministic Android Client OpenAPI export and hash manifest under `api/generated/android/`; this repository does not generate/validate the actual Kotlin consumer implementation merely by exporting that input.

Generator configuration/version is repository-controlled and reproducible. Generated files are never manually edited. CI must regenerate or verify from a clean checkout and fail on drift.

## 6. Contract-change workflow

### Semantic wire change

```text
architecture authority
→ OpenAPI
→ fixtures
→ generated artifacts
→ component tests
→ affected T3/T4.1/T4.2/T4.3/T4.4 tests
→ downstream consumer when applicable
```

### Non-semantic completion

```text
OpenAPI + fixture
→ generated artifacts
→ contract tests
→ consumers
```

If implementation discovers that reasonable clients could interpret the detail differently, it is semantic and must return to architecture.

## 7. Versioned Freeze evidence

Freeze is an executable milestone, not a Markdown declaration.

Before a later stage treats S0.1 as an accepted frozen dependency, the exact candidate must have:

- Client/Admin/Internal OpenAPI aligned with the pinned S0.1 architecture baseline;
- canonical fixtures for every Android-visible Snapshot resource/policy behavior;
- deterministic generation/drift checks Green;
- Snapshot Preview compiled from the same canonical projection as Release Snapshot;
- required S0.1 deterministic/product system evidence;
- required real Adapter qualification evidence;
- the architecture-defined machine-readable Freeze manifest.

The complete manifest evidence contract belongs to `measix-s0-capability-delivery-system-testing-spec.md` and `docs/release.md`; this document intentionally does **not** maintain a second partial field list.

A draft produced by the two-phase writer/replay flow is not an accepted Freeze. Current tooling writes to `docs/s0-freeze-manifest.json`, so run it only on an isolated candidate and preserve existing historical evidence. Final acceptance and known validation gaps are defined in [release](release.md); do not infer them from the filename.

After freeze, an incompatible Android-visible change cannot silently mutate frozen v1/v2/v3. Follow architecture compatibility/versioning semantics and create the applicable new candidate. S0.3 additionally pins Gateway Control OpenAPI, Gateway build identity, surface/catalog fixtures and scenario evidence; it cannot reuse the v1 manifest as proof.

## 8. Compatibility rules

S0 contract tests must prove architecture rules including (not a claim that all current tests already do):

- clients tolerate added unknown optional response fields;
- undeclared request fields are rejected where strict request decoding is required;
- programs branch on HTTP status + stable Problem `code`, not human `detail` text;
- stable identifier format/ownership is not redefined by generated DTOs;
- internal APIs never leak into Android/Admin generated clients;
- `runtimeRouteId`, Upstream internal/base URL and Secret material never enter Client Snapshot;
- unsupported/future protocol behavior is explicit rather than silent fallback.

## 9. Review requirements

An OpenAPI change must identify:

1. owning architecture requirement;
2. whether semantics changed;
3. pre-freeze vs frozen-contract impact;
4. fixtures changed;
5. generated consumers changed;
6. affected T0–T3 and stage-specific T4.1/T4.2/T4.3/T4.4/final lanes;
7. Android synchronization impact;
8. backward compatibility.

## 10. T0 contract gate

The T0 gate must include or evolve to include:

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

Freeze identity generation is a candidate/C7 concern and must not be confused with ordinary pre-freeze contract drift checks.
