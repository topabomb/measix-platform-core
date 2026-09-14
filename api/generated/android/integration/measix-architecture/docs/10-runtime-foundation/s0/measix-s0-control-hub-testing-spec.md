# S0 Control Hub Testing 规格

> 状态：S0 Component Testing Baseline  
> 版本：2026-08-31
> 组件架构：`measix-s0-control-hub.md`  
> Wire 权威：`measix-s0-control-protocol.md`  
> S0.1 Gate：`measix-s0-capability-delivery-system-testing-spec.md`  
> S0.2 Gate：`measix-s0-enterprise-realm-experience-testing-spec.md`
> S0.3 Gate：`measix-s0-enterprise-tool-gateway-testing-spec.md`
> 最终系统测试：`measix-s0-system-testing-spec.md`  
> 文档职责：定义 Control Hub 必须如何被证明；不规定 test file 目录、runner、CI workflow 或实现 package 结构。

## 1. 测试目标

Control Hub 必须同时证明：

```text
Identity authority
Managed Draft / Release / Snapshot authority
Operational Upstream / Secret authority
Runtime Relay / Gateway desired-state authority
Gateway Profile / Integration / Candidate + Published Catalog authority
Usage / Pricing authority
Durable recovery authority
```

HTTP CRUD 成功不是组件完成证据；关键不变量必须跨真实 persistence、network、crash/restart 和 concurrency boundary 验证。

阶段适用性：S0.1 Gate 不要求尚属 S0.2 的 Experience/Portal 或 S0.3 Gateway 实现；相应扩展从所属子阶段开始成为 MUST，并进入 final S0。基础 identity/security/persistence 规则持续适用，不能因阶段拆分跳过。

## 2. 测试分层

| Layer | 目标 |
|---|---|
| Unit / Domain | validation、state transition、hash/canonicalization、pricing、pure security rules |
| Persistence Integration | 真实 SQLite/migration/constraint/backup/reopen |
| HTTP / Contract | generated OpenAPI + canonical fixtures + auth/trust boundary |
| Peer Integration | 受控 Relay/Gateway peer 注入 ACK/timeout/conflict/unavailable，按阶段适用 |
| T3 | real Hub + Relay；S0.3 加 real Gateway |
| T4.1–T4.4/T4 | 由相应子阶段及 Final S0 System Testing Spec 负责 |

纯 service unit test 可以使用窄 fake，但 persistence、migration、HTTP、crypto、reconciliation 等关键边界必须另有真实 integration evidence。

## 3. Identity scenarios

必须覆盖：

- `HUB-ID-001` platform stable ID type/prefix/format validation；
- `HUB-ID-002` normalized username 唯一，rename 不改变 stable user identity；
- `HUB-ID-003` Enrollment single-use / expiry / wrong context；
- `HUB-ID-004` installation metadata 不取代 server Device authorization identity；
- `HUB-ID-005` Session ACTIVE/REVOKED/EXPIRED；
- `HUB-ID-006` Access JWT claims/audience/issuer/device/session linkage；
- `HUB-ID-007` restrictive state 先提交 Hub，Relay 失败不回滚；
- `HUB-ID-008` permissive enable 只有 Relay enforcement 成功后 finalize ACTIVE。
- `HUB-ID-009` Enrollment 初始化七天 idle expiry；只有成功 Refresh 原子轮换凭据并续期，普通请求/后台 heartbeat 不续期；
- `HUB-ID-010` Refresh response-loss/retry behavior 不导致双重续期、旧 credential 重用或客户端永久误锁。

Clock/random 可以注入 deterministic test source，但 production generator/crypto contract 需有独立 compatibility test。

## 4. Draft / Validation / Snapshot scenarios

- `HUB-CAP-001` draftRevision optimistic concurrency；
- `HUB-CAP-002` caller-proposed child ID type/prefix/collision/reference validation；
- `HUB-CAP-003` Provider/Model/TTS/ASR/MCP/Policy reference closure；
- `HUB-CAP-004` runtime binding/upstream/secret/path validation；
- `HUB-CAP-005` Save Draft 不改变 active Release/ManagedState；
- `HUB-CAP-006` canonical Snapshot deterministic compile；
- `HUB-CAP-007` Snapshot 排除 Secret、Upstream URL、runtimeRouteId/server-only binding；
- `HUB-CAP-008` Published Release/Snapshot immutable；
- `HUB-CAP-009` Republish historical content 产生新 release/generation；
- `HUB-CAP-010` warning acknowledgement 不能绕过 hard validation。
- `HUB-CAP-011` 当前 Snapshot v4 包含资源和 Assistant/Seed/Starter 且引用闭合、hash/order deterministic；
- `HUB-CAP-012` Memory Seed 作为 Release content immutable，Enterprise Local Memory 不进入 Snapshot compiler。

### Enterprise Update

- `HUB-UPD-001` Draft/Publish/Withdraw state 与权限正确；
- `HUB-UPD-002` Publish/Withdraw 只推进 Feed revision/ETag，不推进 managedGeneration；
- `HUB-UPD-003` Client/Portal 与 Gateway private typed read projection 只看到 PUBLISHED 内容且共享一个 authority；
- `HUB-UPD-004` no-date query uses default/explicit limit; optional date filters/timezone/order/invalid range/truncation match Protocol；
- `HUB-UPD-005` Hub 不暴露 MCP initialize/tools/list/tools/call；Gateway private typed endpoint 不经 public ingress；
- `HUB-UPD-006` Gateway-facing projection 保持只读，不能调用 Admin mutation surface；
- `HUB-UPD-007` category/severity/contentFormat closed vocabulary、projection 和 old-record default semantics 与上位合同一致。

### Enterprise Tool Gateway

- `HUB-GTW-001` ToolGatewayProfile/Integration/Candidate/Published Catalog durable ownership and stable IDs；
- `HUB-GTW-002` Test/Refresh only changes operational/Candidate state, never Published generation；
- `HUB-GTW-003` platform invariant guidance cannot be overridden; highlighted refs closed；
- `HUB-GTW-004` source/agent metadata separated; schema constraints immutable; READ_ONLY enforced；
- `HUB-GTW-005` Direct/Gateway duplicate source and invalid surfaceHash block Publish；
- `HUB-GTW-006` Snapshot v5 + GatewayControlState use one canonical surface/catalog compiler；
- `HUB-GTW-007` private Enterprise Update read projection is typed/read-only and does not become a generic Hub proxy。

S0.1 required resource/profile 的更细 scenario 由 Capability Delivery System Testing Spec 维护，本文不复制 CAP-C0/C2/C3 matrix。

## 5. Upstream / Secret scenarios

- `HUB-UPS-001` candidate revision 与 active revision 分离；
- `HUB-UPS-002` Connection Test 不自动 Apply；
- `HUB-UPS-003` SecretVersion append-only / replace 不修改旧 version；
- `HUB-UPS-004` candidate config 精确引用 Secret version；
- `HUB-UPS-005` Apply failure 保留旧 active revision；
- `HUB-UPS-006` Secret operational change 可以只推进 controlRevision，不错误推进 managedGeneration。

必须验证 Secret plaintext 不进入 read response、Problem、普通日志、Snapshot、Usage 或 browser-visible state。

## 6. Runtime + Gateway Control / Activation scenarios

- `HUB-ACT-001` compiler 只读取 persisted Release + active operational state，不读取未发布 Draft；
- `HUB-ACT-002` full desired state descriptor/hash deterministic；
- `HUB-ACT-003` Activation intent 先 durable persist，network call 不在 DB transaction 内；
- `HUB-ACT-004` Relay ACK 前 Activation 不 COMPLETED / Release 不 ACTIVE；
- `HUB-ACT-005` same idempotency identity + same normalized command 返回同一 semantic result；
- `HUB-ACT-006` same key + different command hash → conflict；
- `HUB-ACT-007` 并发跨 Relay command 不产生互相覆盖的 desired state；
- `HUB-ACT-008` observed applied revision/hash == pending desired 时可安全 finalize；
- `HUB-ACT-009` unexpected newer/different Relay state → DEGRADED，不盲目覆盖。
- `HUB-ACT-010` target containing Gateway requires its new generation mapping/ACK before Relay apply, even if catalog bytes are unchanged；
- `HUB-ACT-011` Gateway failure blocks switch/finalize when target contains Gateway; removing Gateway instead follows restrictive Relay removal and nonblocking cleanup under Control Protocol §6；
- `HUB-ACT-012` Gateway ACK + Relay failure leaves new Catalog dormant/public-unreachable；
- `HUB-ACT-013` Hub restart reconciles exact Gateway + Relay revisions/hashes without duplicate generation。

Crash injection 至少覆盖：

```text
A. intent durable commit 前
B. intent commit 后 / Relay apply 前
C. Relay applied 后 / Hub 收 ACK 前
D. Hub 收 ACK 后 / finalize commit 前
E. finalize 后 / Admin response 前
```

断言：无半 Release、无 duplicate generation、restart/retry 可根据 persisted intent + Relay status 收敛。

## 7. Usage / Pricing scenarios

- `HUB-USG-001` RequestUsage requestId dedupe；
- `HUB-USG-002` batch accepted/duplicate 语义完整，invalid input 不制造静默部分成功；
- `HUB-USG-003` SemanticUsage source dedupe；
- `HUB-USG-004` UNKNOWN/PARTIAL 不伪造成精确 semantic usage；
- `HUB-USG-005` PricingRule effective window/scope/meter selection；
- `HUB-USG-006` 缺可靠 meter/price → Cost UNKNOWN/PARTIAL；
- `HUB-USG-007` quantity/cost arithmetic 不产生不可接受的累计精度错误。

历史 usage 必须保留发生时的 identity/generation/resource/upstream facts，不能被当前 state 重新解释。

## 8. Persistence / Migration / Backup

真实 persistence 必须验证：

### Schema / invariant

- stable IDs、关键 FK/uniqueness/monotonicity invariants；
- active Release / generation / SecretVersion / request usage / idempotency 等 architecture 约束；
- foreign-key/integrity validation；
- immutable history 不被 update path 修改。

### Migration

- `HUB-DB-001` empty DB applies the single current initialization schema；
- `HUB-DB-003` 当前数据库与初始化记录不被普通 restart 改写；
- `HUB-DB-004` incompatible schema revision startup fail-fast；
- `HUB-DB-005` 当前 schema 和数据在备份后保持完整；

### Backup / Restore

- `HUB-DB-007` backup 产生一致 durable image；
- `HUB-DB-008` 当前版本 restore 后 integrity + schema 校验 Green；
- `HUB-DB-009` master/signing/service credential 不被当普通 DB content 备份；
- `HUB-DB-010` 缺失/错误 secure key material 时 fail closed，而不是把 Secret 当空值继续运行。

具体 table/column/index/SQLite pragma/backup command 由 core 实现测试决定，不属于本 Architecture Testing Spec。

## 9. HTTP / Contract / Security

必须覆盖：

- Admin cookie/CSRF、Client bearer/session、internal service identity 不混用；
- request unknown-field rejection / response unknown optional compatibility；
- stable Problem `status + code`；
- pagination/ETag/idempotency/202 Activation 等 Control Protocol 语义；
- secret create/replace response 永不回显 plaintext；
- `/internal/*` 不接受 User/Admin credential；
- Admin role enforcement；
- Client principal 从 token/session authority 推导；
- JWT algorithm/signature/kid/audience/issuer/expiry；
- password/credential/secret encryption/tamper failure；
- bootstrap/maintenance boundary 不变成 public management endpoint；
- logs/reports 不出现 Secret/token/prompt/body。

具体密码 hash 参数和 crypto library version 归 implementation security baseline；改变跨组件 token/credential semantic 时才修改 architecture。

## 10. Concurrency

至少证明：

- 同 draftRevision 并发 save 只有一个 authoritative successor；
- same idempotency identity 并发 command 只有一个 semantic Activation；
- reconciliation 与 Admin command 不产生 same revision/different bundle；
- session refresh/revoke race 最终不能恢复 revoked authority；
- duplicate RequestUsage 并发 ingest 只保留一个 request fact；
- DB busy/transient failure 不破坏 durable invariants。

测试使用可控 barrier/failure injection，不依赖概率性 timing。

## 11. Real Gateway / Relay T3 requirement

以下不能只靠 Gateway/Relay fake：

1. Publish → real Gateway atomic apply → real Relay atomic apply → Hub finalize；
2. apply timeout/unknown result → real status reconciliation；
3. User/Device restrictive change 到 real Relay；
4. Secret/Upstream operational apply；
5. Relay restart → Hub rehydrate；
6. Hub restart → persisted desired state reconcile；
7. RequestUsage real batch ingest/dedupe。
8. Candidate refresh/publish、platform Update query 与 external MCP Catalog through real Gateway。

跨组件最终断言由对应 System Testing Spec 拥有；不要在本文复制 SYS/CAP matrix。

## 12. Resource / performance evidence

关注真实 S0 风险：

- idle Hub resource baseline；
- Admin CRUD/Publish/usage batch latency；
- Snapshot/GatewayControl/RuntimeControl compile 随规模增长；
- reconciliation idle overhead；
- usage history pagination；
- DB growth / backup behavior。

不规定脱离目标部署环境的虚假峰值 QPS。

## 13. Component Exit

Control Hub component 可以进入相关 System Gate，仅当：

1. 本文 required scenarios 对当前 architecture baseline 有 executable evidence；
2. migration/backup/security/recovery/concurrency critical paths Green；
3. real Gateway/Relay T3 required paths Green；
4. 无长期 skipped critical scenario；
5. evidence 对应 exact implementation candidate，而不是历史/移动 `latest`。
