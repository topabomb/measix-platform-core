# API Contracts, Fixtures and Code Generation

This document defines executable-contract ownership in `measix-platform-core`. Semantic meaning remains authoritative in `topabomb/measix-architecture`.

## Portal contract synchronization (2026-09-08)

[Control Protocol §8](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md) now targets Bridge v3 / localReadVersion=2: document bootstrap and correlated message replies, plus three typed local reads. Native OpenAPI, shared cases, Android exports and Portal consumers implement this profile. Older v2 bundles are rejected by the delivery checks. Reuse Client Feed DTOs and existing date fixtures; remote Hub HTTP remains unchanged. The [implementation status](s0-execution-progress.md) and [Android handoff](../../measix-enterprise-portal/docs/android-alignment-handoff.md) track the remaining work.

## 1. Current and planned S0 OpenAPI surfaces

The current source owns four HTTP OpenAPI 3.0.3 documents:

```text
api/admin/admin.openapi.yaml
api/client/client-control.openapi.yaml
api/internal/relay-control.openapi.yaml
api/internal/usage-ingest.openapi.yaml
```

They are separated so Admin/Android consumers do not accidentally generate or depend on Relay-internal APIs.

An additional schema-only document, `api/portal/portal-contract.openapi.json`, owns Bridge v3 bootstrap/requests/responses and local read v2 results; enrollment/context formatVersion remains 1. Its empty `paths` is intentional: it creates no Hub endpoint. Local transport and authorization remain Control Protocol §8 semantics implemented by the native host. Feed response types remain in Client OpenAPI. The generator derives client-feed.schemas.json and its transitive schema dependencies from that single authority; the native schema references this adjacent generated file. Both files must travel together. The export manifest includes eight artifacts; a standalone-directory test verifies reference resolution without sibling repositories, and the generated dependency records the Client source SHA256.

`api/fixtures/portal/native-vectors.json` contains named valid/invalid wire cases consumed by Go and Portal. `api/fixtures/enrollment/cases.json` preserves raw text, duplicate keys, UTF-8 byte limits, origin/expiry/source checks under a fixed clock. The Go reference oracle does not prove Android parser adoption. Admin produces the canonical platform fixture; native scan/paste must consume both material kinds using the same parser.

Enrollment input accepts lowercase t/z and UTC +00:00, with at most nine fractional digits and no leap seconds; producers emit uppercase T/Z. The two material schemas and raw cases enforce the Control Protocol subset. Native contract tests use scoped calendar-date and date-time validators because kin-openapi's default regex rejects RFC3339 lowercase t/z; no global validator or HTTP behavior is relaxed. `api/fixtures/portal/feed-vectors.json` contains shared calendar/query expectations, including DST, consumed by the real Go Feed service and exported for the native implementation.

`node scripts/checks.mjs generate` exports these inputs and SHA256 manifest to `api/generated/android/portal/`, entirely inside core. It never writes to Android. Portal generates TypeScript and CSP-safe standalone validators from the same native schema, and copies shared vectors with input hashes. Schema validation is followed by method/result correlation, URL trust, chunk progression and lifecycle checks in each actual consumer.

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

Only the current unpublished Snapshot v4 is supported. All five policy flags are required booleans. Old snapshots, policy adoption and incremental database migrations are removed; obsolete development data/configuration can be deleted and recreated. Shared Android materials, mappings and HTTP/runtime examples are maintained in [android-platform-integration.md](android-platform-integration.md).

Snapshot v4 is the only current profile; Gateway v5 remains planned. Policy has five required booleans and new policies deny all five. No old draft/release adoption or optional policy compatibility DTO remains. Discovery/Bootstrap advertises the current compiler version. Use:

- `measix-s0-capability-delivery-contract-spec.md`;
- `measix-s0-enterprise-realm-experience-contract-spec.md`;
- `measix-s0-enterprise-tool-gateway-contract-spec.md`;
- `measix-s0-control-protocol.md`;
- relevant component/product/testing specs.

The current Client API requires refresh Idempotency-Key, rotating credentials and sessionIdleExpiresAt; enrollment is 201 and requires deviceName. Feed HTTP fields/queries are camelCase; snake_case belongs only to the planned Gateway platform-tool schema. Ingest allows missing resource/route/upstream for authenticated unforwarded denials, never for forwarded requests. Generated Android export carries these changes; actual Android consumers must explicitly adopt and verify them before compatibility/Freeze claims.

Current implementation, verification results and remaining stage gates are maintained in [current status](s0-execution-progress.md).

## 4. Canonical fixtures

Portal grant/exchange/restricted Web Session operations are in the Client OpenAPI. Only the two canonical `/api/client/v1/enterprise/updates` GETs accept the additional Portal Cookie scheme; other Client/Admin/runtime operations retain their own authentication. The independent Portal generates TypeScript from this same file and records its input SHA256. See [Portal implementation](portal-implementation.md) for source/config/test ownership.

`PlatformEnrollmentMaterial` in the Client OpenAPI describes the native scan/paste document, not the HTTP Enrollment request. Its canonical sample is `api/fixtures/enrollment/platform-v1.json`; Android export and Admin's `generated-client.ts` derive from this same source. The Admin generator emits both surface type files; it imports only the native material type from the Client output and does not call Client HTTP APIs. Control Protocol §8 owns source selection, trust checks, byte limits and the separate local example material. The local example consumer and private configuration-file format remain Android implementation responsibilities.

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

New draft evidence writes exclusively to `.artifacts/s0-freeze-candidate.json` (or an explicit new output), without overwriting an existing candidate. Current CAP tooling validates the current v4 resource baseline; runtime-only replay cannot finalize C7. Final acceptance and unimplemented later-stage gates are defined in [release](release.md); do not infer them from the filename.

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
