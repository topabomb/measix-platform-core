# S0.3 Enterprise Tool Gateway Testing 规格

> 状态：S0.3 System / Component Testing Authority
> 版本：2026-08-31
> 上位合同：`measix-s0-enterprise-tool-gateway-contract-spec.md`
> 组件架构：`measix-s0-enterprise-tool-gateway.md`
> Wire 权威：`measix-s0-control-protocol.md`
> 最终系统测试：`measix-s0-system-testing-spec.md`
> 文档职责：定义 S0.3 Gateway/Admin/Test Client 的 required evidence；不规定测试框架、文件目录、具体 runner 或实现结构。

## 1. 测试目标与分层

S0.3 必须证明：

```text
S0.1/S0.2 frozen baselines remain reproducible
+ Gateway standard MCP surface is constant-size and generation-stable
+ Candidate review/publish produces an immutable governed catalog
+ discovery/ref/schema/authorization fail closed
+ Hub → Gateway → Relay activation converges safely
+ platform tool and external MCP execute through real three-process topology
```

| Layer | Required evidence |
|---|---|
| T0 Contract | Architecture/OpenAPI/fixture/schema/hash/error compatibility |
| T1 Domain | Catalog/description/search/toolRef/schema/authorization rules |
| T2 Component | Hub/Gateway/Relay/Admin persistence, apply, restart and recovery |
| T3 Cross-component | real Hub↔Gateway↔Relay↔Test Client + downstream MCP |
| T4.3 S0.3 Product | production Admin SPA + public Test Client topology + browser review |

Android emulator/device 不属于 S0.3；它是 S0.4 required evidence。

## 2. Contract 与 surface

- `ETG-C0-001` 当前 v4 资源 fixture 可复算，未来 v5 另有 Gateway gate；不保留旧原型兼容；
- `ETG-C0-002` Snapshot v5 adds one optional Gateway resource with stable `twg_*`, runtime path, surfaceVersion and surfaceHash, and contains no catalog/integration/secret/toolRef/internal endpoint；
- `ETG-C0-003` Gateway `tools/list` returns exactly `discover_tools`,`invoke_tool` in that order；
- `ETG-C0-004` same generation emits byte-stable name/description/input/output schema and exact surfaceHash；
- `ETG-C0-005` Snapshot does not duplicate complete Tool Definitions and client validates standard MCP response against surfaceHash；
- `ETG-C0-006` invalid schema/surface/catalog/reference blocks Publish/apply；
- `ETG-C0-007` old client rejects unsupported Snapshot v5 explicitly without interpreting it as v4 or historical v1/v2; reserved v3 is not accepted as the current Gateway profile (Control Protocol §10.10.1)。
- `ETG-C0-008` Snapshot resource presence replaces a second enabled bit and carries exactly one known `clientEnablementPolicy`；unknown policy fails closed；
- `ETG-C0-009` Gateway pair is atomic/default-on: REQUIRED forbids disable, optional policy permits only pair-level disable, and Direct MCP remains Assistant-bound。
- `ETG-C0-010` Hub/Gateway/Test Client canonical surface fixtures match Protocol JCS/SHA-256 across key order, Unicode and numeric serialization; v1/v2 Snapshot hashes are unchanged。
- `ETG-C0-011` platform source uses an allowlisted platformToolName without a fictitious ToolIntegration; downstream source requires a real integration; mixed/missing source fields fail validation。

## 3. Guidance 与 Catalog lifecycle

- `ETG-CAT-001` platform invariant description cannot be removed/overridden by enterprise guidance；
- `ETG-CAT-002` guidance/highlighted tools preview and published surface use the same compiler；
- `ETG-CAT-003` save/validate/preview do not affect active surface；Publish creates a new generation；
- `ETG-CAT-004` highlighted disabled/missing/duplicate reference blocks Publish；
- `ETG-CAT-005` raw source name/description/schema and agent-facing override remain separately auditable；
- `ETG-CAT-006` agent-facing name uniqueness is enforced while rename preserves `gtl_*` identity；
- `ETG-CAT-007` admin cannot change required/type/validation constraints or publish non-READ_ONLY tool；
- `ETG-CAT-008` remote list-changed/manual refresh creates Candidate diff only；active Published Catalog/hash/generation remain unchanged；
- `ETG-CAT-009` same governed source cannot be active through both Direct MCP and Gateway in one Release；
- `ETG-CAT-010` untrusted annotation never alone grants READ_ONLY or authorization。

## 4. Discovery quality 与 authorization

- `ETG-DISC-001` 1..5 atomic queries and limit 1..5 validate strictly；
- `ETG-DISC-002` exact name and exact alias deterministically return the intended tool；
- `ETG-DISC-003` each result includes published complete inputSchema and optional outputSchema；
- `ETG-DISC-004` filter-before-rank excludes disabled/unpublished/unauthorized/non-read-only tools without count/order leakage；
- `ETG-DISC-005` same catalog/query yields deterministic order with stable tie-break；
- `ETG-DISC-006` 50+ tool corpus proves required Top-K, aliases, similar names, cross-domain and bilingual cases；
- `ETG-DISC-007` multiple atomic queries retrieve separate capabilities without exposing the whole catalog；
- `ETG-DISC-008` lexical baseline remains available when optional embedding/router dependency is absent。

## 5. toolRef 与 invoke

- `ETG-REF-001` valid discover→invoke resolves exact published tool and validates arguments；
- `ETG-REF-002` random/modified ref fails without downstream call；
- `ETG-REF-003` cross-deployment/user/interaction/generation replay fails no-forward；
- `ETG-REF-003A` cross-device/session replay fails no-forward even for the same user；
- `ETG-REF-004` expired ref requires rediscovery and never falls back to name；
- `ETG-REF-005` schemaHash/catalog mismatch fails no-forward；
- `ETG-REF-006` arguments violating published required/type/format constraints fail before downstream; extra fields are rejected only when that schema forbids them; unsupported validation dialect/remote references block publication；
- `ETG-REF-007` remote drift/missing source tool fails closed and marks drift；
- `ETG-REF-008` cancellation propagates; forwarded timeout/network uncertainty is not automatically retried；
- `ETG-REF-009` new generation causes Relay 428 for old interaction before Gateway, then client starts a new interaction and rediscovers；
- `ETG-REF-010` no generation grace or dormant-catalog bypass exists。

## 6. Identity、security 与 privacy

- `ETG-SEC-001` Gateway runtime is unreachable from public ingress/direct Android bearer；
- `ETG-SEC-002` Relay strips forged principal/internal headers and injects authenticated envelope；
- `ETG-SEC-003` invalid issuer/audience/time/request/deployment/generation envelope fails no-forward；
- `ETG-SEC-004` Android token is never forwarded downstream; correct Gateway credential is injected；
- `ETG-SEC-005` endpoint/path/redirect/cookie/header/SSRF cannot escape allowlisted integration；
- `ETG-SEC-006` logs/status/result never expose secret/internal URL/toolRef claims/raw sensitive payload；
- `ETG-SEC-007` external result/description/annotation remains untrusted and cannot become platform instruction；
- `ETG-SEC-008` client-safe resolved tool metadata exposes business action/status but not private topology。

## 7. Control、restart 与 reconciliation

- `ETG-CTL-001` full GatewayControlState apply is atomic, same revision/hash idempotent, stale/conflicting revision rejected；
- `ETG-CTL-002` invalid candidate leaves old applied state intact；
- `ETG-CTL-003` restart without rehydrate is fail-closed；
- `ETG-CTL-004` Hub rehydrates exact desired revision/hash after restart；
- `ETG-CTL-005` when target includes Gateway, Gateway apply failure prevents Relay switch and Release ACTIVE; removal exception is covered by ETG-CTL-013；
- `ETG-CTL-006` Gateway ACK + Relay failure leaves new catalog dormant/unreachable；
- `ETG-CTL-007` Hub crash/timeout reconciles exact Gateway/Relay status without duplicate generation；
- `ETG-CTL-008` concurrent request captures one immutable applied view；
- `ETG-CTL-009` restrictive state remains fail-closed even when one process is unavailable。
- `ETG-CTL-010` Gateway-first handoff keeps Relay-current traffic working, next generation dormant until Relay switch, then Relay rejects every new old-generation request while accepted in-flight work may finish；
- `ETG-CTL-011` post-finalize cleanup removes the unreachable old catalog without interrupting captured requests or changing the semantic Release。
- `ETG-CTL-012` Model/Assistant-only Publish with unchanged Gateway bytes still stages generation N and supports N discover/invoke after Relay switch；
- `ETG-CTL-013` Gateway removal preserves old reachable catalog until Relay route removal, then cleans up; unavailable Gateway/cleanup failure cannot reopen the route or prevent safe restrictive finalize；

## 7.1 Production supervision 与 logs

- `ETG-OPS-001` packaged Hub/Gateway/Relay run as three independently supervised production binaries; dev `go run`/SPA server/concurrent script is absent from the production path；
- `ETG-OPS-002` aggregate start/stop reaches bounded readiness, while supervisor active and application ready remain distinguishable；
- `ETG-OPS-003` crash triggers bounded on-failure restart/rate-limit without cascading all daemons; permanent configuration failure is diagnosable and does not spin forever；
- `ETG-OPS-004` SIGTERM performs bounded drain and preserves Hub durable state/Relay spool; Gateway/Relay restart fail closed until rehydrate；
- `ETG-OPS-005` all daemons emit line-delimited JSON with service/build/event and applicable correlation IDs; service manager collection/rotation/retention is executable；
- `ETG-OPS-006` fixtures and failure scenarios prove logs omit auth/secret/credential/private endpoint/toolRef claims/prompts/bodies/tool arguments/results/direct user identity。

## 8. Platform 与 external tool E2E

- `ETG-E2E-001` Admin creates/tests/reviews/publishes a real deterministic downstream MCP integration；
- `ETG-E2E-002` Test Client lists exactly two Gateway tools, discovers an external tool and invokes it through public Relay path；
- `ETG-E2E-003` `get_enterprise_updates` is discovered/invoked through Gateway platform adapter and matches Client/Portal Feed authority/filter semantics；
- `ETG-E2E-004` Hub exposes no MCP initialize/list/call projection and Gateway cannot reach Admin mutation surface；
- `ETG-E2E-005` Relay is proven byte-transparent for Gateway MCP body and does not log/parse real tool name or arguments；
- `ETG-E2E-006` Relay RequestUsage and Gateway tool execution fact correlate by requestId without double-counting；
- `ETG-E2E-007` Admin browser shows candidate/published drift, surface preview, generation, Gateway status and exact failure recovery actions。
- `ETG-E2E-008` Gateway RequestUsage has the real typed Gateway target and no fabricated upstreamId; authenticated no-route denials are observable without fabricated route IDs。

## 9. Resource / performance evidence

Record at least: 50/200/1000-tool catalog index/search latency; concurrent discover/invoke; downstream session reconnect; large schema/result bounds; memory/handle/goroutine trend; cancellation cleanup; Gateway restart/replay time.

No universal QPS target is invented here. Evidence must show bounded result/context size, no full catalog in model context, no unbounded leak and diagnosable degradation under declared deployment profile.

## 10. S0.3 Freeze Gate

Freeze evidence must include：

1. exact architecture/core/build/OpenAPI/fixture/catalog/scenario identities；
2. executable T0/T1/T2/T3/T4.3 results and final exit codes；
3. production Admin SPA Browser review；
4. real three-process Hub/Gateway/Relay topology, deterministic MCP server and public Test Client traffic；
5. Gateway surface/catalog/control/restart/security/performance artifacts；
6. three-daemon production supervision/graceful lifecycle/structured-log collection and redaction evidence；
7. S0.1/S0.2 regression proof；
8. explicit statement that Android S0.4 real integration/device Gate has not yet been proven。
