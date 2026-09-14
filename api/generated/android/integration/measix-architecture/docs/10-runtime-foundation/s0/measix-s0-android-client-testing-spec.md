# S0.4 Android Client Testing 规格

> 状态：S0.4 Component Testing Baseline
> 版本：2026-08-31
> Android Contract/Architecture：`measix-s0-android-integration-contract-spec.md`  
> Wire 权威：`measix-s0-control-protocol.md`  
> 最终系统测试：`measix-s0-system-testing-spec.md`  
> 实现仓库：`topabomb/rikkahub_mcp`  
> 文档职责：定义 Android Enterprise Binding、Managed State/Snapshot、Effective Runtime、Direct MCP/Gateway required profile、failure/concurrency/UI/emulator 的 required evidence；不规定 Kotlin package/class/file、具体 DataStore/Keystore wrapper 或 test runner。

## 1. 测试入口

所有 S0.4 Android tests 必须 pin 有效 S0.3 Freeze baseline：

```text
platformCoreCommit
clientControlOpenApiHash
gatewayControlOpenApiHash
canonicalFixtureHash
snapshotSchemaVersion
gatewaySurfaceHash / evaluationCatalogHash
S0.3 scenario/evidence manifest
```

测试不得从 moving `platform-core latest` 隐式读取变化中的 schema。

## 2. 测试目标

证明 Personal/Enterprise 是同一 Android Runtime 上严格隔离的两个 Client Realm，Enterprise 是现有 Local Runtime 的受控增量：

```text
PERSONAL → Built-in + Personal Local only
+
ENTERPRISE → Managed + policy-allowed User Configuration + Enterprise Preferences
+
Enterprise Binding / credential / Applied Managed State independently persisted
+
new Managed top-level Runtime always passes authoritative correctness guard
+
Managed resource read-only without deleting user definitions or realm runtime data
+
required Model/TTS/HTTP-ASR/Direct MCP/Gateway map to existing execution boundaries
+
generation/revoke/auth/network failure does not cause unsafe replay
```

不能只测试 Chat。

## 3. 测试分层

| Layer | 目标 |
|---|---|
| JVM / pure unit | ID/state/resolver/validator/profile mapping/Problem |
| Coroutine/concurrency | refresh/snapshot/preflight single-flight/cancel/race |
| HTTP contract | Client Control + Runtime mapping + 428/auth semantics |
| Persistence/instrumentation | Binding/Managed State/secure credential/LKG/process restart |
| UI/instrumentation | realm switch、Enterprise access、origin/lock/failure states |
| T4.4 Android | real emulator/device + pinned Hub/Gateway/Relay + public platform topology |

Final Android T4 不能用 JVM Test Client 代替。

## 4. Wire / fixture compatibility

必须验证：

- frozen Client wire/generated/synced artifacts reproducible；
- canonical fixture hashes match Freeze；
- unknown optional response tolerated；
- unsupported protocol/schema 不 silent fallback；
- malformed typed ID/reference rejected；
- Snapshot ETag/hash/deployment identity contract；
- Problem logic uses status + code；
- required full Snapshot v5 覆盖 Model/TTS/ASR/Direct MCP/ToolGateway/Policy，且 Gateway 只携带 resource/surfaceHash。

## 5. Installation / Binding

- `AND-BIND-001` first install 生成并稳定保存 installation identity；
- `AND-BIND-002` installation identity != server Device authorization identity；
- `AND-BIND-003` platform URL 只接受允许的 secure origin；
- `AND-BIND-004` Enrollment 持久化 authoritative deployment/user/device/session binding；
- `AND-BIND-005` failed/expired/consumed enrollment 不留下 half binding；
- `AND-BIND-006` Disconnect 只清 Enterprise Binding/Credential/Managed state；
- `AND-BIND-007` REVOKED 不能通过本地 toggle 恢复 ACTIVE；
- `AND-BIND-008` new deployment 不复用旧 credential/snapshot；
- `AND-BIND-009` `BOUND` 不自动等于 active `ClientRealm=ENTERPRISE`；
- `AND-BIND-010` revoke/Disconnect 将 active realm 安全退回 PERSONAL，且不删除 Personal Local 数据。

## 6. Credential / Session

- `AND-AUTH-001` Refresh Credential 使用平台 secure storage/Keystore-backed protection；
- `AND-AUTH-002` Access Token memory-only；
- `AND-AUTH-003` secure-storage invalidation/decrypt failure → require safe re-enrollment/recovery；
- `AND-AUTH-004` credential absent from log/UI/normal Local backup/settings；
- `AND-AUTH-005` valid access token reuse；
- `AND-AUTH-006` concurrent refresh single-flight；
- `AND-AUTH-007` refresh 不漂移 authoritative session identity；
- `AND-AUTH-008` revoked/expired refresh maps to correct binding state；
- `AND-AUTH-009` only safe Control request refresh+retry once；
- `AND-AUTH-010` Runtime POST/stream/tool/TTS/ASR/MCP 不自动 replay on auth/network uncertainty；
- `AND-AUTH-011` Android 严格消费 frozen Protocol 定义的七天滚动闲置 Session：只接受服务端权威 expiry/renewal，超时要求重新验证/接入，不自行续期，也不把 Access Token lifetime 错当成七天。

具体 crypto/API wrapper 由 Android implementation tests 决定。

## 7. Managed State / Snapshot

- `AND-SNP-001` READY + same generation → allow；
- `AND-SNP-002` READY + different generation → SYNC_REQUIRED；
- `AND-SNP-003` ACTIVATING/DEGRADED → block new Managed runtime；
- `AND-SNP-004` Hub unavailable → block new Managed runtime，但 Local/history 可用；
- `AND-SNP-005` snapshot sync single-flight；
- `AND-SNP-006` schema/ref/hash/profile validation 完成后才 commit；
- `AND-SNP-007` invalid/corrupt snapshot 不替换 LKG；
- `AND-SNP-008` payload+generation+release/hash/schema whole-state atomic commit；
- `AND-SNP-009` process restart 恢复一致 Applied state；
- `AND-SNP-010` ETag/cache semantic correct；
- `AND-SNP-011` Snapshot 无 Secret/upstream/runtimeRoute/internal URL；
- `AND-SNP-012` LKG 不能绕过 server active-generation barrier；
- `AND-SNP-013` TTS missing voice rejected；
- `AND-SNP-014` unsupported realtime/native profile 不 fallback 到另一 protocol；
- `AND-SNP-015` Direct MCP auth ownership 只接受 frozen supported values；
- `AND-SNP-016` ToolGatewayDefinition 不含 Catalog/Integration/Secret/toolRef/internal URL/full Tool Definitions；
- `AND-SNP-017` standard Gateway `tools/list` canonical hash 必须等于 Snapshot surfaceHash，mismatch fail closed。

## 8. ClientRealm / Effective Runtime / origin / lock

- `AND-EFF-001` PERSONAL deterministic view = Built-ins + Personal Local；
- `AND-EFF-002` ENTERPRISE deterministic content view = Managed + policy-allowed User Configuration + Enterprise Preferences；host safety/navigation functions cannot appear as selectable Built-in content to bypass policy；
- `AND-EFF-003` Managed 内容在 PERSONAL picker/tool registry/runtime resolver/execution 中完全不存在；
- `AND-EFF-004` 五项开关各自允许/禁止/收紧/恢复用户原定义，使用原用户凭据，不注入企业 Session、不创建副本；
- `AND-EFF-005` Managed/User 同 ID 按来源独立寻址，不覆盖原定义；用户助手引用逐项授权，禁止引用显示原因并阻止执行；
- `AND-EFF-006` Managed remove/Disconnect 不破坏用户配置与 Personal 运行数据；企业运行数据与个人备份恢复隔离；
- `AND-EFF-007` Managed Model/TTS/ASR/Direct MCP/Gateway 仅在 Enterprise Realm 出现在对应现有 picker/list/registry；
- `AND-EFF-008` Managed edit/delete disabled in UI；
- `AND-EFF-009` write/commit boundary 仍拒绝 Managed mutation if UI bypassed；
- `AND-EFF-010` 企业策略限制使用而非全局编辑用户原定义；受管定义写入命令拒绝，不能仅测试 UI disabled；
- `AND-EFF-011` Managed projection only contains client-required runtime metadata；
- `AND-EFF-012` realm switch 不改写 Conversation/Assistant/Memory/Attachment/Workspace 的原有 realm ownership。

## 9. Interaction context / concurrency

- `AND-INT-001` each top-level Managed action gets unique interaction identity；
- `AND-INT-002` capture realm + deployment + generation + immutable Effective Runtime context；
- `AND-INT-003` Chat/Tool/MCP loop in same interaction reuses context；
- `AND-INT-004` background sync 或 realm switch 不 mutate active context；
- `AND-INT-005` concurrent interactions 可共享 single-flight network work but contexts distinct；
- `AND-INT-006` cancel propagates execution/HTTP；
- `AND-INT-007` child operation 使用 explicit parent context when semantically same interaction；
- `AND-INT-008` no global mutable interaction cross-talk。

## 10. Managed Model

- `AND-MDL-001` uses frozen `OPENAI_CHAT_COMPLETIONS` compatible execution path；
- `AND-MDL-002` Snapshot upstream model selector maps correctly；
- `AND-MDL-003` TEXT/IMAGE + TOOL/REASONING capabilities project correctly；
- `AND-MDL-004` network endpoint is platform Relay resource URL, not provider base URL；
- `AND-MDL-005` Local provider key/custom header cannot override Enterprise Runtime auth；
- `AND-MDL-006` streaming reaches existing rendering/tool flow；
- `AND-MDL-007` Local model paths regress Green when UNBOUND/local-selected。

## 11. Managed TTS

- `AND-TTS-001` independent Managed TTS action passes interaction guard；
- `AND-TTS-002` payload uses Snapshot upstream model selector；
- `AND-TTS-003` payload uses Snapshot `voice` exactly；
- `AND-TTS-004` no implicit local/default voice fallback；
- `AND-TTS-005` MP3 binary enters existing playback/save pipeline unchanged；
- `AND-TTS-006` cancel propagates；
- `AND-TTS-007` Local TTS paths regress Green。

## 12. Managed HTTP ASR

- `AND-ASR-001` Managed ASR action passes guard before upload；
- `AND-ASR-002` uses HTTP multipart `OPENAI_AUDIO_TRANSCRIPTIONS`；
- `AND-ASR-003` multipart file/model/optional language correct；
- `AND-ASR-004` transcription result maps to existing consumer/UI；
- `AND-ASR-005` upload cancel 不 retry/replay；
- `AND-ASR-006` Managed ASR 不要求 WebSocket；
- `AND-ASR-007` Local realtime ASR regress Green；
- `AND-ASR-008` realtime VAD/sample-rate/WebSocket settings 不从 Managed Snapshot fabricated。

## 13. Direct Managed MCP

- `AND-MCP-001` Managed MCP uses existing compatible Streamable HTTP stack；
- `AND-MCP-002` connect/call passes guard + generation context；
- `AND-MCP-003` ENTERPRISE_MANAGED only sends platform Runtime auth; upstream Secret never on Android；
- `AND-MCP-004` NONE works without fabricated credential；
- `AND-MCP-005` Local OAuth/header MCP remains Local/regress Green；
- `AND-MCP-006` reconnect cannot bypass preflight/revoke/generation；
- `AND-MCP-007` unsupported user-managed Managed auth is rejected/not offered。

## 14. Enterprise Tool Gateway

- `AND-GTW-001` standard MCP initialize/`tools/list` only exposes `discover_tools`,`invoke_tool` in fixed order；
- `AND-GTW-002` surface canonical bytes/hash match frozen Snapshot and no local tool-definition builder exists；
- `AND-GTW-003` Gateway is automatically registered only in Enterprise Realm and never appears in ordinary MCP settings/editor；
- `AND-GTW-004` Direct Managed MCP remains Assistant-bound and is not used as Gateway fallback；
- `AND-GTW-005` discover result complete schema/toolRef is scoped to captured interaction；
- `AND-GTW-006` invoke sends only toolRef + arguments through platform Runtime auth；
- `AND-GTW-007` 428/new generation terminates old interaction; new interaction rediscovers and never reuses ref；
- `AND-GTW-008` resolved business action/status updates existing tool card/diagnostics without exposing endpoint/credential；
- `AND-GTW-009` Personal Realm has no Gateway tools/cache/ref/execution path；
- `AND-GTW-010` Gateway unavailable/surface mismatch is explicit and cannot silently degrade to Direct/Local MCP。
- `AND-GTW-011` `REQUIRED` always registers both tools, exposes a managed lock state and rejects client disable writes；
- `AND-GTW-012` `USER_CONTROLLABLE_DEFAULT_ON` defaults both tools on, and one Enterprise capability toggle atomically removes/restores the pair without affecting Direct MCP；
- `AND-GTW-013` local preference is isolated by `(deploymentId, toolGatewayId)`, survives process restart, is ignored-but-preserved under REQUIRED, and reappears when policy becomes optional；
- `AND-GTW-014` preference never changes Snapshot/hash/generation, never enters ordinary MCP backup/settings and cannot produce a one-tool-only registry state。
- `AND-GTW-015` pair preference changes affect only the next interaction capture; in-flight tools stay immutable, explicit cancel stops immediately, and server policy changes retain the generation barrier。

## 15. Runtime endpoint / security

- `AND-RT-001` URL = platform origin + `/runtime/v1/resources/{resourceId}{runtimePath}`；
- `AND-RT-002` required platform auth/generation/interaction correlation present；
- `AND-RT-003` Snapshot cannot introduce arbitrary host/base URL；
- `AND-RT-004` Enterprise auth never written to normal Provider apiKey/custom credential；
- `AND-RT-005` every public top-level Managed entrypoint passes Enterprise interaction guard；
- `AND-RT-006` lower-level helper is not exposed as bypassing alternate top-level path。

## 16. 428 / failure / revoke

- `AND-428-001` only authoritative managed-update problem with no-forward drives sync；
- `AND-428-002` current interaction terminates；
- `AND-428-003` sync target generation completes before READY；
- `AND-428-004` original request/action not replayed；
- `AND-428-005` new action gets new interaction + generation；
- `AND-428-006` other 4xx/5xx not misclassified；
- `AND-428-007` 401/403/network uncertainty with possible side effect never automatically replays runtime request。

Revoke/control outage 还必须验证 Local settings/history browsing 不被无关阻断。

## 17. UI

Enterprise access 覆盖：扫码/粘贴 Enrollment、UNBOUND/ENROLLING/BOUND/REVOKED、Generation、READY/SYNCING/SYNC_FAILED/CONTROL_UNAVAILABLE、Sync/Retry、Disconnect confirmation。

Realm UI 覆盖：

- `Personal` / `<Enterprise display name>` 明确切换；
- Personal Realm 不出现 Managed 资源、工具、助手或场景入口；
- Enterprise Realm 显示企业来源、受管/强制启用状态及用户配置准入和不可用原因；
- Assistant-bound Direct Managed MCP 与 verified Gateway two-tool surface 在 Enterprise Realm 进入可用工具集合，切回 Personal 后完全移除；
- Gateway 不进入 MCP Server settings/URL/Header/OAuth editor；invoke 工具卡显示 resolved 真实业务动作，而不是永久只显示 `invoke_tool`。

Resource UI 覆盖：

- Managed origin/lock label；
- 获准用户原配置 + Managed coexistence；
- no second Enterprise picker；
- unavailable/update/revoked state；
- Managed mutation UI disabled + lower write boundary deny；
- control outage 不阻断 Local/history browsing。

## 18. Persistence / process lifecycle

Instrumentation/process tests 至少验证：

- Binding + active ClientRealm durable roundtrip；
- secure credential isolation/invalidation；
- Applied Managed State whole-state commit；
- crash/restart 不出现 generation/payload split brain；
- corrupt state 保持 LKG/Local；
- Disconnect 清理范围准确；
- no enterprise credential in normal backup/export/log/crash report。

## 19. Emulator T4.4 Gate

Final Android lane 必须使用：

```text
real emulator/device
pinned frozen Client contract/fixtures
real Control Hub
real Runtime Relay
real Enterprise Tool Gateway
real deterministic downstream MCP
client-facing public platformUrl
no pre-injected enterprise session/snapshot
```

从 Personal Realm 启动并确认无 Managed 内容；完成 Enrollment 后进入 Enterprise Realm，至少完成 Model streaming、TTS binary、HTTP ASR multipart、Direct MCP Streamable HTTP、Gateway discover→platform tool + external tool→invoke，并回到 Relay/Gateway Usage/Audit correlation；再切回 Personal 验证 Managed 内容从所有选择、注册和执行边界消失。

JVM Test Client 或 direct internal API 不能替代 Android T4。

## 20. Component Exit

Android S0.4 可以进入 Final S0 System/RC Gate，仅当：

1. S0.3 Freeze pin/Snapshot v5/Gateway wire/surface/catalog compatibility 与 S0.1–S0.3 regression Green；
2. Binding/Credential/Snapshot persistence + failure/restart Green；
3. Personal/Enterprise 运行数据隔离、五项用户配置准入及完整迁移/个人恢复保全企业数据 Green；
4. interaction guard/context/concurrency Green；
5. Model/TTS/ASR/Direct MCP/Gateway required profile + resolved tool UI Green；
6. 428/revoke/auth/network/cancel no-unsafe-replay Green；
7. UI/security endpoint boundary Green；
8. real emulator/device T4 required path Green；
9. existing Local runtime regression Green；
10. evidence 对应 exact Android/core/architecture baseline。
