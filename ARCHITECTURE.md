# Implementation Architecture

> Authority boundary: this document defines the implementation structure, dependency rules, and documentation governance of `measix-platform-core`. Product semantics and S0 architecture remain authoritative in `topabomb/measix-architecture`.

## 1. Repository role

This repository owns four S0 logical components:

```text
Control Hub     → Go binary
Runtime Relay   → Go binary
Enterprise Tool Gateway → Go binary (S0.3 target; not present yet)
Admin Console   → Quasar/Vue SPA build
```

It also owns executable API contracts, the ordered database migration history, qualification/system-test infrastructure, CI and operational procedures needed to prove those components satisfy architecture.

Current Direct MCP v5 tool governance is implemented in Hub capability discovery/validation/projection and the Admin resource/assistant editors; see [the implementation and Android handoff](docs/direct-mcp-tool-governance.md). The semantic authority is Control Protocol §10.7.1. Relay keeps its opaque MCP transport boundary; planned Gateway remains a separate component.

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
├── cmd/devmigrate/
├── cmd/generate-android-wire/
├── pkg/platformid/
├── internal/common/
├── internal/wire/
├── internal/hub/
├── internal/relay/
├── ent/
├── migrations/
└── test/system/          # current Go-module system harness

console/
├── src/
└── e2e/                 # existing browser E2E

scripts/                # Node browser/candidate/evidence orchestration
docs/
```

The Go harness is under `backend/test/system/` to run inside the Go module and reuse permitted internal test boundaries. Node browser/candidate orchestration also exists under `scripts/`; these are two concrete environments, not a single physical harness. Same-environment product closure and shared contract/evidence rules remain required. Planned S0.3 paths are `backend/cmd/enterprise-tool-gateway/` and `api/internal/gateway-control.openapi.yaml`; neither is current source.

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

### Enterprise Tool Gateway

`enterprise-tool-gateway` is an S0.3 separate binary/failure domain. It consumes only Hub-compiled Gateway control, accepts runtime traffic only from Relay private service identity, owns applied catalog/search/toolRef/downstream MCP execution, and never reads Hub persistence or validates Android bearer tokens directly.

Permitted shared production code remains narrow: generated direct wire types, pure identifier/canonicalization helpers and generic server/logging helpers with no Hub durable-domain semantics. The Gateway binary and `gateway-control.openapi.yaml` do not currently exist; documentation must preserve that implementation gap until source and executable contracts land.

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

### Ownership table

| Subject | Authority |
|---|---|
| Phase/Stage/sub-stage scope | `measix-architecture` |
| Product/UX requirements | `measix-architecture` |
| Terminology / stable IDs | `measix-architecture` |
| Cross-component state/wire/error/security semantics | `measix-architecture` |
| Required component/system scenarios | `measix-architecture` |
| Stage reading lists | `measix-architecture/docs/measix-stage-document-index.md` |
| Exact executable HTTP schema | `api/*.openapi.yaml` |
| Canonical fixtures | `api/fixtures/` |
| Source/package/UI structure | source tree + local implementation docs |
| Concrete frontend/Go dependencies | package manifests/lockfiles |
| Ent schema / append-only migration history | repository source |
| Build/test/CI/operations | repository tooling + local docs |
| Freeze/RC evidence | exact candidate artifacts generated by repository tooling |

Do not create local copies of architecture contracts, stage reading lists, protocol documents or identifier tables.

### Contract-change workflow

Rules:

1. `measix-architecture` remains authoritative for meaning.
2. `api/*.openapi.yaml` is authoritative for exact executable HTTP schema in this repository.
3. generated code is never manually edited.
4. canonical cross-component fixtures live only under `api/fixtures/`.
5. a semantic wire change starts in architecture; a non-semantic schema completion may be implemented locally only when it cannot change client interpretation.

A semantic wire/state/ID/security change requires architecture authority first. Then update, as applicable:

```text
OpenAPI → fixtures → generated artifacts → tests → implementation
```

See `docs/api-contracts.md` for the full contract/codegen/freeze workflow.

## 5. Persistence ownership

Control Hub persistent application state uses SQLite + Ent with an ordered, append-only migration history owned by `backend/migrations`. Runtime Relay persists only its own durable runtime data such as usage spool and does not share Hub ORM state.

This repository is authoritative for actual Ent schemas, immutable migration SQL, data-preserving upgrades, clean bootstrap and schema-identity tests. Architecture remains authoritative for durability, ownership, atomicity and recovery invariants.

## 6. Test architecture

Executable tests mirror architecture gates rather than duplicate their semantics:

```text
T0 Static / Contract
T1 Unit / Domain
T2 Component Integration
T3 Cross-component Integration
T4.1 S0.1 pre-Android Product/System E2E
T4.2 S0.2 Realm/Experience Product
T4.3 S0.3 Gateway Product
T4.4 S0.4 Android Integration
T4 final S0 System RC
```

This repository owns Hub/Relay/Admin tests and future Gateway tests, qualification infrastructure, deterministic/system harnesses and server-side product evidence. Android component/instrumentation tests remain in `rikkahub_mcp`; Portal has its own product implementation. Final S0 T4 combines all applicable pinned repositories and builds.

Critical scenario semantics/IDs come from architecture, including:

```text
HUB-*   Control Hub
RLY-*   Runtime Relay
ADM-*   Admin Console
CAP-*   S0.1 Capability Delivery
ERX-*   S0.2 Realm/Experience
ETG-*   S0.3 Enterprise Tool Gateway
AND-*   S0.4 Android integration
SYS-*   final S0 system/RC
```

Tests here reference those IDs where applicable; this repository must not invent new cross-component product semantics locally.

Default GitHub Actions CI deliberately proves deterministic T0–T3 only. T4.1 real-browser candidate verification and real external Adapter qualification are explicit S0.1 promotion gates; a Green `ci-gate` is not C6/C7/S0.1 Freeze evidence.

See `docs/testing.md` for executable test organization, CI design, and the TDD cycle.

## 7. Documentation governance

### Local documents

[README](README.md#文档导航) 是唯一完整导航。本文维护依赖/源码边界；docs/ 的各文档分别维护合同、实现、运行、测试和发布事实。固定 Preview 组合、当前源码范围与历史证据只在 [status index](docs/s0-execution-progress.md)维护，不重复抄到功能参考。

### Documentation rule

A local document contains only information needed to implement, run, test or operate this repository. If a paragraph merely re-explains an architecture requirement without adding a local implementation consequence, replace it with a reference.

Do not maintain the same current-state claim in multiple documents. `docs/s0-execution-progress.md` owns the implementation/seal summary and historical evidence index; release manifests and actual runs carry the reproducible evidence. After implementation, merge durable mechanisms and commands into their existing references and remove the completed plan. Keep unresolved risks and actual evidence pointers, not repeated test counts or repair diaries.

Stage-specific reading order is maintained only in `topabomb/measix-architecture/docs/measix-stage-document-index.md`.

### Active implementation candidate vs default branch

During active development, architecture may reference local implementation documents that exist on the current S0.1 implementation candidate before they are merged to `main`. In that situation:

- architecture semantics still come only from `measix-architecture@main`;
- concrete implementation/docs are read from the active core candidate branch/commit identified by the current PR/status;
- completion claims always cite an exact implementation SHA;
- Freeze/RC never uses a moving branch head — the manifest pins exact commits/builds.

### Synchronization

**Architecture semantic change:**

```text
architecture authority
→ OpenAPI/fixtures/tests
→ implementation
→ downstream consumers when affected
→ current progress re-evaluation
```

An architecture change that adds or strengthens a required scenario automatically reopens any previously Green checkpoint whose evidence did not prove the new requirement. Historical Green remains regression evidence only.

**Pure implementation change:**

Code layout, component decomposition, dependency choice, DB index, build tooling and UI internals stay in this repository unless they change an architectural boundary.

### S0.1 Client Contract Freeze

MEASIX has an internal Preview and no formal public release. Supported Snapshot versions and data-preservation boundaries follow Control Protocol §10.10.1–2. Current Starter v5 retains published v4 semantics and immutable data; database changes follow the existing append-only migration owner. Current candidate acceptance still requires the actual source/build/contract/artifact chain; retained old reports never certify current work. See [release procedures](docs/release.md).

Core release association follows the adopted [version association rules](../measix-architecture/docs/00-platform/measix-versioning-and-compatibility-plan.md). The identity source is `api/protocol-baseline.json`; Client and Portal exports share it. [Release procedures](docs/release.md) own packaging and Android handoff. Ordinary packaging does not require Android evidence: test the fixed package with the original APK, then record the actual product-version pair, artifact hashes and result links in the existing release record. The stricter evidence-bundling tools remain optional. Version metadata does not change runtime protocols, migration ownership or stage gates, and cannot alone certify compatibility.

## 8. Change boundary

S0.2 的生产协议计量与用户额度见 [当前实现说明](docs/usage-budget.md)，阶段语义由其引用的架构合同拥有。Relay 以隔离的只读协议观察器和 durable spool 形成请求事实，Hub 通过原子预算准入、幂等结算及 Admin/Client/Portal 投影拥有额度权威；Relay 不依赖 Hub domain/Ent，Hub 不承载 Runtime body。当前封版组合与后续阶段边界见 [S0 状态](docs/s0-execution-progress.md)。

Update `measix-architecture` first when a change alters:

- platform terminology or stable identifier meaning;
- S0 scope or component ownership;
- cross-component state/lifecycle semantics;
- HTTP/wire/error/idempotency semantics;
- security/admission invariants;
- required Component/System Testing Spec behavior.

Keep the change in this repository when it only changes implementation while preserving those meanings, such as package refactoring, DB indexing, HTTP implementation details, CI optimization, test helpers, local tooling or UI component decomposition.

When implementation reveals architectural ambiguity, do not choose a new semantic locally. Resolve the owning architecture authority first, then implement it here.
