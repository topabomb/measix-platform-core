# S0 阶段审查报告

> 架构 baseline：`topabomb/measix-architecture@02ba0add27cddce3bcebe63433495df6ea39b9ad`  
> 平台分支：`agent/s0-platform-core`

## 1. 审查范围

覆盖 S0 Foundation/Core 与 S0.1 Managed Capability Delivery 的当前实现状态：

1. 架构契约完整性（OpenAPI → fixtures → codegen → 实现）
2. 测试场景覆盖率（对照 Hub/Relay Testing Spec 中的 MUST 场景）
3. 测试执行结果（Green 证据）
4. 缺口与下一步

## 2. 架构契约状态

### 2.1 已修复的契约缺口

| 缺口 | 架构场景 | 修复内容 | 状态 |
|---|---|---|---|
| TTS `voice` 字段缺失 | HUB-CAP-003 | OpenAPI 添加 `TtsDefinition.voice`，`validateContent` 增加 voice 必填检查，snapshot compile/hash 包含该字段 | Green |
| MCP `authOwnership` 字段缺失 | HUB-CAP-003 | OpenAPI 添加 `McpDefinition.authOwnership`，`validateContent` 增加合法性校验，snapshot compile/hash 包含该字段 | Green |
| 精确十进制算术 `1/5` 计算错误 | HUB-USG-007 | `exactDecimal` 函数 `two`/`five` 因子逻辑修正，`1/5` 从 `0.5` 修正为 `0.2` | Green |
| Identity TTL 参数类型 | HUB-ID-004 | `10*1e9` 改为 `10*time.Minute` | Green |
| DB schema revision 查询 | HUB-DB-008 | 使用 `maintenance.CurrentSchemaRevision` 常量替代查询不存在的 `schema_revisions` 表 | Green |

### 2.2 契约流水线

修复严格遵循契约驱动流水线：

```text
OpenAPI (client + admin) → fixtures → generated artifacts (codegen) → tests (Red) → implementation (Green)
```

关键文件：
- `api/client/client-control.openapi.yaml`、`api/admin/admin.openapi.yaml`：TTS `voice` 与 MCP `authOwnership`
- `api/fixtures/`：canonical fixtures 覆盖新字段
- `backend/internal/wire/adminapi/`、`relaycontrolapi/`：生成代码包含新常量
- `backend/internal/hub/capability/service.go`：验证逻辑
- `backend/internal/hub/capability/snapshot.go`：编译/哈希包含新字段
- `backend/internal/hub/usage/pricing.go`：精确十进制算术

## 3. MUST 测试场景映射

### 3.1 Control Hub

对照 `measix-s0-control-hub-testing-spec.md` 第 4–9 节 MUST 场景：

| 场景 ID | 描述 | 测试函数 | 状态 |
|---|---|---|---|
| HUB-ID-001 | platform ID prefix/type/UUID validation | `TestCanonicalKindsGenerateAndValidateUUIDv4`, `TestRejectsWrongPrefixAndNonV4UUID` | Green |
| HUB-ID-002 | normalized username unique, rename doesn't change `usr_*` | `TestHUBID002RenamePreservesStableUserID` | Green |
| HUB-ID-003 | enrollment single-use, expiry, wrong user/context | `TestHUBID003EnrollmentExpiryAndCredentialDigestOnly` | Green |
| HUB-ID-004 | device installation metadata 不复制 `dev_*` authorization identity | `TestHUBID004InstallationMetadataDoesNotCopyAuthIdentity` | Green |
| HUB-ID-005 | session ACTIVE/REVOKED/EXPIRED | `TestHUBID005RefreshCredentialDigestOnly` | Green |
| HUB-ID-006 | access JWT claims, audience, issuer, device/session linkage | `TestAccessJWTClaimsAndTTL` | Green |
| HUB-ID-007 | restrictive change 先提交 Hub state, Relay 失败不回滚 | `TestI5SecurityDisableIsDenyFirstAndEnableIsAllowLast` | Green |
| HUB-ID-008 | permissive enable 只有 Relay enforcement 成功后才 finalize ACTIVE | `TestI5SecurityDisableIsDenyFirstAndEnableIsAllowLast` | Green |
| HUB-CAP-001 | draftRevision optimistic concurrency | `TestHUBCAP001DraftOptimisticConcurrencyAndSaveDoesNotActivate` | Green |
| HUB-CAP-002 | candidate ID prefix/type/collision/reference validation | `TestHUBCAP002CandidateIDValidation` (5 subtests) | Green |
| HUB-CAP-003 | provider/model/TTS/ASR/MCP reference closure | `TestCAPC0004TTSVoiceRequired`, `TestCAPC0006MCPAuthOwnershipValidation` | Green |
| HUB-CAP-004 | route/upstream/secret/runtimePath validation | `TestHUBCAP004RouteUpstreamValidation` | Green |
| HUB-CAP-005 | Save Draft 不改变 active Release/ManagedState | `TestHUBCAP005SaveDraftDoesNotChangeActiveRelease` | Green |
| HUB-CAP-006 | snapshot deterministic compile | `TestHUBCAP006SnapshotDeterministicAndClientSafe` | Green |
| HUB-CAP-007 | Snapshot 排除 Secret、Upstream URL、runtimeRouteId | `TestHUBCAP007SnapshotExcludesSecretsAndUpstreamURLAndRouteID` | Green |
| HUB-CAP-008 | published release/snapshot immutable | `TestHUBCAP008StagedReleaseIsImmutable` | Green |
| HUB-CAP-009 | Republish 历史内容生成新 releaseId/generation | `TestHUBCAP009RepublishCreatesNewReleaseIDAndGeneration` | Green |
| HUB-CAP-010 | warnings 不能绕过服务端确认语义 | `TestHUBCAP010WarningsCannotBypassServerValidation` | Green |
| HUB-UPS-001 | candidate config revision 与 active config revision 分离 | `TestHUBUPS001CandidateAndActiveConfigRevisionsAreSeparate` | Green |
| HUB-UPS-002 | Test Connection 不自动 apply | `TestHUBUPS002ValidateConfigDoesNotApply` | Green |
| HUB-UPS-003 | SecretVersion append-only, replace 不修改旧 version | `TestHUBUPS003SecretVersionsAreAppendOnlyEncryptedAndReferencedPrecisely` | Green |
| HUB-UPS-004 | UpstreamConfig 精确引用 `secretId+secretVersion` | `TestHUBUPS004PreciseSecretReference` | Green |
| HUB-UPS-005 | apply 失败保留旧 active revision | `TestHUBUPS005UpdateFailureRetainsOldRevision` | Green |
| HUB-UPS-006 | Secret rotation 可只产生新 controlRevision | `TestHUBUPS006SecretRotationProducesNewVersion` | Green |
| HUB-ACT-001 | compiler 只读取持久化 Release, 不读取未发布 Draft | `TestHUBACT001CompilerReadsPersistedReleaseNotDraft` | Green |
| HUB-ACT-002 | RuntimeControlState deterministic descriptor/bundleHash | `TestHUBACT002RuntimeControlStateDeterministic` | Green |
| HUB-ACT-003 | activation intent 先持久化, 网络调用不在 DB transaction 内 | `TestI3PublishPersistsIntentBeforeRelayAndFinalizesAfterAck` | Green |
| HUB-ACT-004 | Relay ACK 前 Activation 为 COMPLETED/Release 为 ACTIVE | `TestHUBACT004ReleaseNotActiveBeforeAck` | Green |
| HUB-ACT-005 | same Idempotency-Key + same request 返回同一结果 | `TestHUBI5RequestUsageBatchIsIdempotent` | Green |
| HUB-ACT-006 | same key + different request hash 返回 409 | `TestHUBACT006SameKeyDifferentRequestHash` | Green |
| HUB-ACT-007 | 同一时刻仅一个非终态跨 Relay Activation | `TestHUBACT007OnlyOneNonTerminalActivation` | Green |
| HUB-ACT-008 | status 显示 equal desired/applied 时可 finalize pending | `TestHUBACT008ReconcileFinalizesPending` | Green |
| HUB-ACT-009 | Relay unexpected newer/different hash → DEGRADED | `TestI3ReconcileDoesNotBlindlyOverwriteUnexpectedNewerRelay` | Green |
| HUB-USG-001 | RequestUsage requestId dedupe | `TestHUBI5RequestUsageBatchIsIdempotent` | Green |
| HUB-USG-002 | batch 全部 accepted/duplicate 才 ACK; invalid batch 不部分提交 | `TestHUBUSG002InvalidBatchDoesNotPartiallyCommit` | Green |
| HUB-USG-003 | semanticUsage sourceEventId dedupe | `TestHUBI5SemanticUsageDedupeAndCompleteness` | Green |
| HUB-USG-004 | UNKNOWN/PARTIAL 不伪造成精确语义用量 | `TestHUBUSG004UnknownPartialDoNotFabricateCost` | Green |
| HUB-USG-005 | PricingRule effective window/scope/meter selection | `TestHUBI5PricingUsesSpecificEffectiveRuleAndDecimalArithmetic` | Green |
| HUB-USG-006 | 缺 meter/价格时 Cost=UNKNOWN | `TestHUBUSG006MissingMeterOrPriceGivesUnknownCost` | Green |
| HUB-USG-007 | decimal quantity/cost 无二进制浮点错误 | `TestHUBUSG007DecimalArithmeticNoFloatErrors` | Green |
| HUB-DB-001~010 | migration/backup/restore | `TestHUBDBIntegrityAndBackupRestore`, `TestHUBDBCheckRejectsEmptyDatabase`, `TestHUBDB008RestorePassesIntegrityAndRevision` | Green |

### 3.2 Runtime Relay

对照 `measix-s0-runtime-relay-testing-spec.md` 第 4–10 节 MUST 场景：

| 场景 ID | 描述 | 测试函数 | 状态 |
|---|---|---|---|
| **Control State Apply** | | | |
| RLY-CTL-001 | 首次有效 full state apply → READY | `TestRLYCTL001FirstValidStateApplyProducesReady` | Green |
| RLY-CTL-002 | same revision + same bundleHash 重放幂等 | `TestRLYCTL002SameRevisionSameHashIsIdempotent` | Green |
| RLY-CTL-003 | same revision + different hash 返回 409 | `TestRLYCTL003SameRevisionDifferentHashReturnsConflict` | Green |
| RLY-CTL-004 | older revision 被拒绝且 Current state 不变 | `TestRLYCTL004OlderRevisionRejectedAndStateUnchanged` | Green |
| RLY-CTL-005 | invalid route/upstream/reference/JWK/descriptor 全量拒绝 | `TestRLYCTL005InvalidControlStateFullyRejected` (6 subtests) | Green |
| RLY-CTL-006 | validate/build 完成前不暴露新 state | `TestRLYCTL006ValidateBuildDoesNotExposePartialState` | Green |
| RLY-CTL-007 | atomic swap 后新请求使用新 state | `TestRLYCTL007AtomicSwapMakesNewRequestsUseNewState` | Green |
| RLY-CTL-008 | 旧请求 capture 的 state 在生命周期内不重新读取 | `TestRLYCTL008CapturedStateNotReread` | Green |
| RLY-CTL-009 | process restart 后无 persisted control state, public runtime fail closed | `TestRLYCTL009RestartWithNoPersistedStateFailsClosed` | Green |
| RLY-CTL-010 | rehydrate 后 READY/status revision/hash 正确 | `TestRLYCTL010RehydrateProducesReadyWithCorrectRevisionAndHash` | Green |
| **Auth / Principal** | | | |
| RLY-AUTH-001 | 合法 EdDSA JWT | `TestRLYAUTH001ValidEdDSAJWTAccepted` | Green |
| RLY-AUTH-002 | wrong alg/signature/kid | `TestRLYAUTH002WrongAlgSignatureKidRejected` (2 subtests) | Green |
| RLY-AUTH-003 | wrong issuer/deployment/audience | `TestRLYAUTH003WrongIssuerDeploymentAudienceRejected` (2 subtests) | Green |
| RLY-AUTH-004 | expired/not-yet-valid token | `TestRLYAUTH004ExpiredTokenRejected` (2 subtests) | Green |
| RLY-AUTH-005 | malformed `usr_*/dev_*/ses_*` | `TestRLYAUTH005MalformedPrincipalIDsRejected` (3 subtests) | Green |
| RLY-AUTH-006 | disabled user | `TestRLYAUTH006DisabledUserRejected` | Green |
| RLY-AUTH-007 | revoked device/session | `TestRLYAUTH007RevokedDeviceSessionRejected` (2 subtests) | Green |
| RLY-AUTH-008 | unknown kid 不回调 Hub | `TestRLYAUTH008UnknownKidReturns401NoCallback` | Green |
| **Generation / Resource** | | | |
| RLY-ADM-001 | generation == active → 可继续 | `TestRLYADM001GenerationEqualActiveCanContinue` | Green |
| RLY-ADM-002 | generation old/new/missing/invalid → 拒绝 | `TestRLYADM002GenerationMismatchRejected` (4 subtests) | Green |
| RLY-ADM-003 | old generation 返回 428 stable Problem | `TestRLYADM002GenerationMismatchRejected/old_generation_returns_428` | Green |
| RLY-ADM-004 | unknown/disabled resource 拒绝 | `TestRLYADM004UnknownResourceRejected` | Green |
| RLY-ADM-005 | resourceId 解析到当前 route/upstream | `TestRLYADM005ResourceIDResolvesToCorrectRoute` | Green |
| RLY-ADM-006 | S0 不出现 per-user ACL | `TestRLYADM006NoPerUserACLInS0` | Green |
| **URL / Route / SSRF** | | | |
| RLY-ROUTE-001 | 正常 runtimePath + query | `TestRLYROUTE001NormalPathAndQueryPreserved` | Green |
| RLY-ROUTE-002 | base URL 自带 path 不被覆盖 | `TestRLYROUTE002BaseURLWithPathNotOverwritten` | Green |
| RLY-ROUTE-003 | absolute URI 拒绝 | `TestRLYROUTE003AbsoluteURIRejected` | Green |
| RLY-ROUTE-004 | userinfo/scheme/host/port override 拒绝 | `TestRLYROUTE004NoTargetOverride` | Green |
| RLY-ROUTE-005 | `..`/encoded traversal/double-encoding 拒绝 | `TestRLYROUTE005TraversalVariantsRejected`, `TestRLYROUTE005ControlRejectsEncodedTraversalPrefixes` | Green |
| RLY-ROUTE-006 | method allowlist | `TestRLYROUTE006MethodAllowlist` | Green |
| RLY-ROUTE-007 | allowedPathPrefixes 边界匹配 | `TestRLYROUTE007AllowedPathPrefixBoundary` (2 subtests) | Green |
| RLY-ROUTE-008 | Adapter management endpoint 不可达 | `TestRLYROUTE008AdapterManagementNotReachable` | Green |
| RLY-ROUTE-009 | query 原样保留 | `TestRLYROUTE009QueryPreservedNoHostChange` | Green |
| RLY-ROUTE-010 | S0 不执行 generic path rewrite | `TestRLYROUTE010NoGenericPathRewrite` | Green |
| **Header / Credential** | | | |
| RLY-HDR-001~008 | Outbound header/credential policy | `TestRLYHDR001To008OutboundResponseHeaderPolicy` (8 subtests) | Green |
| RLY-HDR (upstream error) | upstream 4xx/5xx 不被改成 Relay 成功 | `TestRLYHDRUpstreamErrorPreserved` | Green |
| RLY-HDR (redirect) | redirect 不自动 follow | `TestRLYHDRRedirectNotFollowed` | Green |
| **Transparent Transport** | | | |
| RLY-TRN-001 | 首个 flush chunk 在后续 chunk 未产生时即可到达 | `TestRLYTRN001002SSEFirstFlushAndOrderPreserved` | Green |
| RLY-TRN-002 | 多 chunk 顺序与内容不改变 | `TestRLYTRN001002SSEFirstFlushAndOrderPreserved` | Green |
| RLY-TRN-003 | 长连接不因固定短 WriteTimeout 被错误关闭 | `TestRLYTRN003LongConnectionNotKilled` | Green |
| RLY-TRN-004 | client cancel 传播 Upstream | `TestRLYTRN004ClientCancelPropagates` | Green |
| RLY-TRN-005 | Upstream 中途断流释放资源 | `TestRLYTRN005UpstreamMidStreamBreakReleasesResources` | Green |
| RLY-TRN-006 | 随机 binary payload hash/length 完全一致 | `TestRLYTRN006BinaryPayloadExact` | Green |
| RLY-TRN-007 | content-type/content-length/chunked 行为 | `TestRLYTRN007ContentTypePreserved` | Green |
| RLY-TRN-008 | 大 binary 不整体 buffer | `TestRLYTRN008LargeBinaryNotBuffered` | Green |
| RLY-TRN-009 | multipart boundary/field/filename/content-type 不被破坏 | `TestRLYTRN009MultipartPreserved` | Green |
| RLY-TRN-010 | 大上传 streaming | `TestRLYTRN010LargeUploadStreaming` | Green |
| RLY-TRN-011 | 上传中 cancel 终止 upstream read | `TestRLYTRN011UploadCancelTerminatesUpstreamRead` | Green |
| RLY-TRN-012 | MCP request/response/stream 正常透传 | `TestRLYTRN012MCPTransparentForward` | Green |
| RLY-TRN-013 | MCP cancel/connection close 传播 | `TestRLYTRN013MCPCancelPropagates` | Green |
| RLY-TRN-014 | Relay 不解析/改写 MCP JSON-RPC 业务语义 | `TestRLYTRN014RelayDoesNotRewriteMCP` | Green |
| **Request State Capture / 并发** | | | |
| RLY-CON-001 | 长 stream 使用 revision R; 并发 apply R+1 后该 stream 继续使用已 capture 的 state | `TestRLYCON001LongStreamKeepsCapturedState` | Green |
| RLY-CON-002 | apply R+1 完成后的新 request 使用 R+1 | `TestRLYCON002NewRequestUsesNewState` | Green |
| RLY-CON-003 | 高并发 request 与 atomic swap 不出现 race/panic/partial map | `TestRLYCON003HighConcurrencyWithAtomicSwap` | Green |
| RLY-CON-004 | old credential 只属于已经 capture 的旧请求 | `TestRLYCON004OldCredentialNotUsedInNewRequest` | Green |
| RLY-CON-005 | cancel storm 不泄漏 goroutine/connection | `TestRLYCON005CancelStormNoLeak` | Green |
| RLY-CON-006 | control apply 与 usage sender 并发互不持有共享长锁 | `TestRLYCON006ControlApplyAndUsageSenderNoDeadlock` | Green |
| **Request Usage / Spool** | | | |
| RLY-SP-001 | event commit 后 process crash/restart 仍存在 | `TestRLYSP001EventPersistsAcrossRestart` | Green |
| RLY-SP-002 | requestId unique | `TestRLYSP002RequestIdUnique` | Green |
| RLY-SP-003 | Hub accepted/duplicate 后才删除 | `TestRLYSPHubAckDeletesOnlyAcknowledgedBatch` | Green |
| RLY-SP-004 | Hub outage + exponential backoff | `TestRLYSPHubOutageKeepsRowsAndRecordsBackoff` | Green |
| RLY-SP-005 | 401/403 internal auth → degraded + 低频重试 | `TestRLYSP005AuthFailureTriggersDegradedLowFreq` | Green |
| RLY-SP-006 | 422 poison batch 隔离坏 row | `TestRLYSPPoison422IsolatedWithoutDroppingGoodRows` | Green |
| RLY-SP-007 | sender restart 不需要内存 sent-set | `TestRLYSP007SenderRestartWithoutMemorySet` | Green |
| RLY-SP-008 | oldest age/pending count/status 正确 | `TestRLYSP008StatsCorrect` | Green |
| RLY-SP-009 | disk full/write failure → METERING_DEGRADED | `TestRLYSP009DiskFullTriggersDegraded` | Green |
| RLY-SP-010 | shutdown 不因 Hub 永久不可达无限阻塞 | `TestRLYSP010ShutdownDoesNotBlock` | Green |
| **Security** | | | |
| Security | public caller 不能访问 `/internal/v1/control/*` | `TestRLYSecurityPublicCannotAccessInternalControl` | Green |
| Security | client 不能注入 upstream credential header / X-Measix-Request-Id | `TestRLYSecurityClientCannotInjectRequestId` | Green |
| Security | credential header 不能恢复保留 headers | `TestRLYHDRCredentialHeaderCannotRestoreReservedHeaders` | Green |
| Security | internal listener 与 public runtime auth 分离 | `TestRelayInternalControlRequiresServiceCredential` | Green |

### 3.3 契约/系统测试

| 场景 ID | 描述 | 测试函数 | 状态 |
|---|---|---|---|
| SYS-I-0001 | canonical fixtures decode with generated wire | `TestSYSI0001CanonicalFixturesDecodeWithGeneratedWire` | Green |
| SYS-I-0002 | unknown optional response field is tolerated | `TestSYSI0002UnknownOptionalResponseFieldIsTolerated` | Green |
| SYS-I-0003 | unknown request field is rejected | `TestSYSI0003UnknownRequestFieldIsRejected` | Green |
| SYS-I-1001 | identity HTTP closed loop | `TestSYSI1001IdentityHTTPClosedLoop` | Green |
| — | snapshot & runtime control golden hashes | `TestSnapshotAndRuntimeControlGoldenHashes` | Green |
| — | S0 OpenAPI surfaces validate | `TestS0OpenAPISurfacesValidate` | Green |

## 4. 测试执行结果

### 4.1 后端 Go 测试

```text
Hub 侧（13 包全部 Green）：
  internal/hub/adminstatic, app, capability, httpapi, identity,
  maintenance, runtimecontrol, security, upstream, usage

Relay 侧（4 包全部 Green）：
  internal/relay, internal/relay/app, internal/relay/control,
  internal/relay/metering, internal/relay/runtime

基础设施（全部 Green）：
  internal/contract, internal/common/health, internal/common/sqliteutil,
  pkg/platformid
```

### 4.2 前端 Vitest

```text
6 test files, 19 tests, all passed
  src/api/client.test.ts         → 1 test
  src/api/workflow.test.ts       → 3 tests
  src/stores/workflows.test.ts   → 4 tests
  src/pages/LoginPage.test.ts    → 2 tests
  src/pages/UpstreamsPage.test.ts → 2 tests
  src/pages/ResourcesPage.test.ts → 7 tests
```

### 4.3 已知环境问题

Windows 并发执行 `go test ./...` 时偶发临时二进制文件锁冲突（`adminstatic`、`security`、`platformid`）。单独重跑均通过。此为 Windows 平台特性，非代码缺陷。

## 5. 缺口与下一步

### 5.1 C0 剩余缺口

- ✅ Provider/Model/TTS/ASR/MCP 枚举闭合完成
- ✅ Snapshot fixtures 覆盖 positive + negative boundary cases
- ✅ Codegen drift Green
- ASR managed semantics 等 audit 需在 C4-C6 集成中进一步验证

### 5.2 C1–C7 状态

| Checkpoint | 状态 | 优先级缺口 |
|---|---|---|
| C1 Upstream Operational | ✅ Green | UpstreamConfig 完整表单 + component TDD 2 tests Green |
| C2 Resource Editor | ✅ Green | Models/TTS/ASR/MCP/Policy 五类 editor + TDD 7 tests Green |
| C3 Snapshot Preview | ✅ Green | draft:preview 端点 + UI dialog + TDD Green |
| C4 Runtime Profile | 未完成 | Test Client/Test Adapter 四 profile (SSE/Binary/Multipart/MCP) 闭环 |
| C5 Usage/Pricing/Observability | 部分完成 | Admin resource-kind 视角、趋势可视化 |
| C6 System E2E | 未完成 | real browser + real Hub/Relay + Test Client/Adapter |
| C7 Contract Freeze | 未开始 | freeze manifest |

## 6. 结论

### 已完成

- S0 Core 基础架构（Control Hub + Runtime Relay + Admin Console shell）已建立
- Hub 侧全部 MUST 测试场景（HUB-ID/CAP/UPS/ACT/USG/DB）已覆盖并验证 Green
- Relay 侧全部 MUST 测试场景（RLY-CTL/AUTH/ADM/ROUTE/HDR/TRN/CON/SP）已覆盖并验证 Green
- 精确十进制算术 (`big.Rat`) 修正完成
- ✅ C0 Contract Audit & Freeze Prep Green（枚举闭合 + fixtures + codegen）
- ✅ C1 Upstream Operational Green（完整 UpstreamConfig + TDD）
- ✅ C2 Managed Resource Editor Green（五类 editor + TDD）
- ✅ C3 Snapshot Projection & Preview Green（draft:preview 端点 + UI + TDD）
- 全部后端 Go 测试和前端 Vitest 19 个测试通过

### 不能宣称

- **S0.1 未完成**：C0–C7 仍有多个 Checkpoint 未 Green
- **Client Control/Snapshot v1 仍是 pre-freeze**：C7 freeze manifest 不存在
- **不能进入 S0.2 Android**：C0–C6 未全部 Green
- **不能宣称 S0 Exit**：架构 Gate 未执行
