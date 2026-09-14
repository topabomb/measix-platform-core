# S0 Runtime Relay Testing 规格

> 状态：S0 Component Testing Baseline  
> 版本：2026-08-29
> 组件架构：`measix-s0-runtime-relay.md`  
> Wire 权威：`measix-s0-control-protocol.md`  
> S0.1 Gate：`measix-s0-capability-delivery-system-testing-spec.md`
> S0.3 Gate：`measix-s0-enterprise-tool-gateway-testing-spec.md`
> 最终系统测试：`measix-s0-system-testing-spec.md`  
> 文档职责：定义 Runtime Relay admission、control apply、透明传输、security、durable usage、concurrency/restart/resource 的 required evidence；不规定 test 目录、具体 proxy/SQLite implementation 或 CI runner。

## 1. 测试目标

必须证明完整请求链：

```text
auth/principal
→ generation barrier
→ resource/route/path policy
→ header/credential policy
→ transparent transport/cancellation
→ durable request usage
```

两个最高优先级不变量：

1. admission/security failure 在 Upstream/Gateway 收到业务 body 前完成；
2. control update 以完整 immutable state 原子切换，在途 request 不看到 half-old/half-new state。

## 2. 测试分层

| Layer | 目标 |
|---|---|
| Unit / Domain | state validation、JWT claims、generation/resource/path/header pure rules |
| HTTP Integration | 真实 TCP/HTTP streaming/binary/multipart/cancel/redirect/error |
| Persistence Integration | 真实 durable usage spool、restart/outage/poison/write failure |
| Peer Integration | 受控 Hub peer 做 control/usage failure injection |
| T3 | real Runtime Relay + real Control Hub |
| T4.1/T4 | 对应 System Testing Spec |

真实 transport 行为不能用内存 handler/buffer test 替代；streaming test 必须能观察 progressive flush 和 cancellation。

## 3. Control State scenarios

- `RLY-CTL-001` first valid full state apply → READY；
- `RLY-CTL-002` same revision + same bundle identity 幂等；
- `RLY-CTL-003` same revision + different hash/conflicting descriptor 拒绝；
- `RLY-CTL-004` stale revision 拒绝且 Current State 不变；
- `RLY-CTL-005` invalid reference/path/auth/JWK/control descriptor 全量拒绝；
- `RLY-CTL-006` validation/build 完成前不暴露 candidate state；
- `RLY-CTL-007` atomic apply 后新 request 使用新 state；
- `RLY-CTL-008` old in-flight request 保持已 capture state；
- `RLY-CTL-009` process restart 后无 persisted current control，public runtime fail closed；
- `RLY-CTL-010` Hub rehydrate 后 READY/status revision/hash 正确。

必须验证 status surface，而不只验证 apply response。

## 4. Auth / Principal scenarios

- `RLY-AUTH-001` valid Hub-issued runtime token；
- `RLY-AUTH-002` wrong algorithm/signature/key；
- `RLY-AUTH-003` wrong issuer/deployment/audience；
- `RLY-AUTH-004` expired/not-yet-valid token；
- `RLY-AUTH-005` malformed platform identity claims；
- `RLY-AUTH-006` disabled user；
- `RLY-AUTH-007` revoked device/session；
- `RLY-AUTH-008` unknown key 不触发 request-path Hub introspection/fetch。
- `RLY-AUTH-009` Gateway route strips forged internal principal headers and injects integrity-protected audience-bound envelope；
- `RLY-AUTH-010` Relay→Gateway service credential is distinct from Android/Admin/Hub control credentials。

任何 failure 必须断言 Upstream/Gateway 未收到业务 body。

## 5. Generation / Resource admission

- `RLY-ADM-001` request generation == active → continue；
- `RLY-ADM-002` old/new/missing/invalid generation 按 Control Protocol 拒绝；
- `RLY-ADM-003` stale generation 返回 stable 428 managed-update semantic；
- `RLY-ADM-004` unknown/disabled/non-active resource 拒绝；
- `RLY-ADM-005` active resource 精确解析当前 route/upstream；
- `RLY-ADM-006` S0 不出现隐式 per-user capability ACL 第二套授权。

对 deny path 必须验证 `forwarded=false` / Adapter no-body evidence，而不是只看最终 4xx。

## 6. URL / Route / SSRF

- `RLY-ROUTE-001` normal runtimePath + query；
- `RLY-ROUTE-002` Upstream base path 与 runtime path 组合不丢失/越界；
- `RLY-ROUTE-003` absolute URI 拒绝；
- `RLY-ROUTE-004` userinfo/scheme/host/port override 拒绝；
- `RLY-ROUTE-005` traversal / encoded / double-encoding variants 拒绝；
- `RLY-ROUTE-006` method allowlist；
- `RLY-ROUTE-007` path-prefix boundary 不可被近似前缀绕过；
- `RLY-ROUTE-008` Adapter/Relay management endpoint 不可由业务 resource route 到达；
- `RLY-ROUTE-009` query 保留但不能改变 target host；
- `RLY-ROUTE-010` 不执行 generic path rewrite。

## 7. Header / Credential / response boundary

必须覆盖：

- client enterprise `Authorization` 不透传到 Upstream；
- Host/hop-by-hop/internal platform headers 正确清理；
- client 不能覆盖 Relay request correlation；
- active Upstream credential 只由 server-side state 注入；
- credential 不因 connection reuse 泄漏到另一个 route/upstream；
- response `Set-Cookie` 不建立 provider cookie 到 Client；
- redirect 不能把 Client 带出 Relay boundary；
- upstream response/error 不泄露 internal credential。

可以在 implementation tests 内用 table-driven cases 组织，不要求 architecture 为每个 case 固定函数名。

## 8. Transparent Transport scenarios

### Request/response

- method/query/content-type/body/status 保真；
- large body 不整体 buffer；
- upstream early error 正确结束 request。

### Streaming / SSE

- `RLY-TRN-001` first flush 在后续 chunk 未产生前已到 Client；
- `RLY-TRN-002` chunk order/content 保持；
- `RLY-TRN-003` long stream 不被统一短 timeout 错误终止；
- `RLY-TRN-004` client cancel 传播 Upstream；
- `RLY-TRN-005` upstream mid-stream failure 释放资源并形成正确 usage/error fact。

### Binary / TTS

- `RLY-TRN-006` binary bytes/hash/length 保真；
- `RLY-TRN-007` content type/HTTP framing 合法；
- `RLY-TRN-008` large binary 不整体 buffer。

### Multipart / ASR

- `RLY-TRN-009` multipart boundary/field/file/content type preserved；
- `RLY-TRN-010` large upload streaming-friendly；
- `RLY-TRN-011` upload cancel 终止 upstream read/work。

### MCP Streamable HTTP

- `RLY-TRN-012` MCP request/response/stream transparent；
- `RLY-TRN-013` cancel/connection close propagation；
- `RLY-TRN-014` Relay 不解析/改写 MCP JSON-RPC business semantic。
- `RLY-TRN-015` `twg_*` route is byte-transparent and Relay cannot observe/log discover query、toolRef、real tool name or arguments；
- `RLY-TRN-016` Gateway tool result/error/client metadata is not rewritten into Relay business semantics。

## 9. Request capture / concurrency

- `RLY-CON-001` stream capture revision R；并发 apply R+1 后仍使用 R 的 route/credential/state；
- `RLY-CON-002` apply 完成后的新 request 使用 R+1；
- `RLY-CON-003` concurrent request + state swap 无 race/panic/partial view；
- `RLY-CON-004` old credential 只属于已 capture 的旧 request；
- `RLY-CON-005` cancel storm 后 goroutine/connection/resource 回落；
- `RLY-CON-006` control apply 与 usage sender 不通过长共享锁互相阻塞。

实现仓库对关键并发路径使用合适 race/leak/resource test，但具体工具不由 architecture 固定。

## 10. Request Usage / durable spool

必须验证 request fact 捕获的是**发生时**的 state，而非 sender 时的 current state。

- `RLY-SP-001` durable write 后 process crash/restart 仍存在；
- `RLY-SP-002` request identity uniqueness/dedup boundary；
- `RLY-SP-003` Hub accepted/duplicate ACK 后才删除；
- `RLY-SP-004` Hub outage 后 backlog 保留并恢复 delivery；
- `RLY-SP-005` internal auth failure → degraded/observable，不能静默 drop；
- `RLY-SP-006` poison item 隔离，不永久阻断所有正常 events；
- `RLY-SP-007` sender restart 不依赖易失内存 sent-set；
- `RLY-SP-008` backlog count/age/status accurate；
- `RLY-SP-009` disk/write failure → explicit metering degraded；
- `RLY-SP-010` shutdown 不因 Hub 永久不可达无限阻塞。

Relay 不测试/实现 provider-specific semantic token/audio calculation；该能力属于 Hub/Adapter qualification boundary。

## 11. Restart / shutdown

必须证明：

- stop accepting new requests 后 bounded drain；
- drain 超时会 cancel remaining work，而不是永久阻塞 upgrade；
- durable spool 保留；
- restart 时 runtime readiness false 直到 valid control rehydrate；
- rehydrate 后 status/revision/hash/generation 恢复正确；
- Hub unavailable during restart 不会错误启用 stale runtime state。

## 12. Security

除 auth/route/header suites 外至少覆盖：

- public caller 不能访问 internal control surface；
- internal service identity 与 User/Admin identity 分离；
- malformed control state 不触发 partial apply/panic；
- client 不能注入 upstream credential/internal correlation；
- log/status/problem 不出现 runtime credential/secret；
- compression/decompression/size limit 不形成绕过；
- oversized header/path/body 按 runtime policy 拒绝；
- requestId 不接受 client override。

## 13. Real Hub / Gateway T3 requirement

以下必须有 real Control Hub integration：

1. Publish full desired state → Relay apply/status；
2. apply/status/reconcile unknown-result path；
3. security revoke/disable；
4. Upstream/Secret operational apply；
5. request usage accepted/duplicate；
6. Hub outage → spool → recovery；
7. Relay restart → Hub rehydrate。
8. public Gateway resource → Relay admission/envelope → real Gateway MCP surface；
9. new generation blocks old Gateway interaction before Gateway no-forward。

跨组件最终断言由 S0.1/S0.3/Final System Testing Spec 拥有。

## 14. Load / resource evidence

关注真实 S0 风险：

- 1/10/50/目标并发 long streams；
- first-byte overhead；
- RSS/goroutine/open-connections 随并发增长和 cancel 后回落；
- binary/multipart 大 body memory behavior；
- control apply during streams；
- Hub usage outage backlog + drain；
- spool DB growth。

不规定脱离目标 VM 的虚假峰值 QPS；明显非线性 memory/resource growth 必须调查。

## 15. Component Exit

Runtime Relay 进入相关 System Gate 前必须满足：

1. required control/auth/admission/route/transport/spool/security scenarios Green；
2. no-forward 由真实 peer/Adapter body-observation proof；
3. restart/cancel/concurrency/resource critical paths 有 evidence；
4. real Hub/Gateway T3 required paths Green；
5. 无长期 skipped critical scenario；
6. evidence 对应 exact implementation candidate，而不是历史/移动 `latest`。
