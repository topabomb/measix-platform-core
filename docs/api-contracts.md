# API Contracts, Fixtures and Code Generation

This document defines executable-contract ownership in `measix-platform-core`. Semantic meaning remains authoritative in `topabomb/measix-architecture`.

## Direct MCP v5 tool governance

Control Protocol §10.7.1 owns the upgraded, unpublished v5 semantics. The implementation and Android handoff are described in [Direct MCP tool governance](direct-mcp-tool-governance.md). Client v5 requires explicit `mcp[].allowedTools[]` (`name`, full-definition JCS `contractHash`, `approvalPolicy`) and `mcp[].toolAccessMode` and `assistants[].mcpBindings[]` (`mcpServerId`, `toolSelection`, `toolNames`). Both modes are ALL / ALLOWLIST: ALL requires an empty tool array and follows dynamic discovery; ALLOWLIST requires a nonempty list. No server binding means the assistant does not use that server. Missing/null fields and unknown modes are invalid; clearing a restricted list never silently enables ALL. An ALL server requires confirmation for every invocation. v5 no longer emits assistant `mcpServerIds`. v4 has separate generated DTOs and retains its published server-level references and canonical bytes.

Admin retains optional new fields so existing durable drafts can be edited without destructive migration. New servers use ALL with an empty allowlist; new assistants start with no server bindings. Selecting a server defaults its binding to ALL. Save preserves unfinished drafts; Validate/Preview/Publish require the new closed references. Legacy assistants require an explicit conversion that retains the selected servers in ALL mode. Admin stores the reviewed full definition and a private discovery catalog; neither raw definition nor discovery credentials/source identity enters Client Snapshot. Discovery metadata is excluded from release diffs and canonical projection.

`POST /api/admin/v1/draft/mcp/{mcpServerId}:discover` requires an Admin session, CSRF and `expectedDraftRevision`; workspace discovery additionally requires a connected `userId`. It uses the applied upstream configuration or selected user's workspace bearer, runs bounded Streamable HTTP initialize/list pagination outside the database transaction, and atomically saves the complete catalog only after rechecking source identity and draft revision. Discovery does not approve tools. Save rejects forged catalog or newly approved definitions outside the latest verified catalog. Failed discovery preserves existing catalog and draft revision; no upstream response body, URL or credential enters the error.

The generator updates OpenAPI DTOs, v5 fixtures, portal transitive client schemas and the Core-owned Android export. Android source is a separate consumer: Core-only export verification is not proof of native decoder, call admission or confirmation support.

## Portal contract synchronization

[Control Protocol §8](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md) owns Bridge v3 document bootstrap and correlated native operations. Portal Session and Feed data use Core HTTP only; there are no native local-read methods or phone-side Portal data source. Native OpenAPI, shared cases, Android exports and Portal consumers implement this single profile. The fixed Preview identity and evidence-index boundary are in [S0 status](s0-execution-progress.md).

## 1. Current and planned S0 OpenAPI surfaces

The current source owns four HTTP OpenAPI 3.0.3 documents:

```text
api/admin/admin.openapi.yaml
api/client/client-control.openapi.yaml
api/internal/relay-control.openapi.yaml
api/internal/usage-ingest.openapi.yaml
```

They are separated so Admin/Android consumers do not accidentally generate or depend on Relay-internal APIs.

An additional schema-only document, `api/portal/portal-contract.openapi.json`, owns Bridge v3 bootstrap/requests/responses. Its empty `paths` is intentional: it creates no Hub endpoint. Native transport and authorization remain Control Protocol §8 semantics implemented by the Android host. Feed response types remain in Client OpenAPI. The generator derives `client-feed.schemas.json` and its transitive dependencies from that single authority. The Android export manifest records every artifact and source digest; standalone-directory tests verify reference resolution without sibling repositories.

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

The internal Preview pins the four executable HTTP documents in `api/protocol-baseline.json` under the candidate identity recorded in that file. `node scripts/verify-preview-contract.mjs` fails when a document changes without deliberate baseline review, and the ARM64 release manifest records every document hash. Within this baseline, additive optional response fields are allowed only after consumer review; removal, rename, type/meaning changes, enum narrowing, or a new required input require an architecture-approved protocol version and migration plan. Human error text is never a compatibility key; HTTP status plus stable Problem `code` is.

New Draft Preview/Publish uses Snapshot v5; published v4 remains readable and republishable at v4. Starter opening details and executed verification are in [starter-opening-snapshots.md](starter-opening-snapshots.md). All five policy flags are required booleans. Policy exposes ten independent optional defaults: assistant, chat, fast, title, attachment inspection, suggestion, context compaction, Image Generation, TTS and ASR. Omission means unset; no default is inferred from another slot or from resource order. All six model defaults must reference enabled models, and attachment inspection additionally requires IMAGE input. `ManagedPolicy` is intentionally exposed on both Admin and Client surfaces; a contract test requires those two schema definitions to remain byte-equivalent after parsing. Standalone Image Generation remains part of the same v4 profile: `imageGenerators` and `policy.defaultImageGenerationId` may be absent only to represent an empty collection and an unset default; new writers emit the collection explicitly. Known values remain strict, and typed Snapshot consumers reject unknown fields. Shared Android materials, mappings and HTTP/runtime examples are maintained in [android-platform-integration.md](android-platform-integration.md).

Admin `ModelDefinition.publishedModelKey` is an optional enterprise selector; missing/blank means `upstreamModelKey`. Snapshot v4/v5 keeps its existing `upstreamModelKey` wire field but its value is the effective published selector, so Android needs no new field and never receives the real provider key. The internal Relay control carries both selectors and both runtime paths. Current Hub output always emits that mapping; Relay accepts a missing mapping only to keep an already-applied pre-change control state pass-through during an incremental restart. The four model profiles translate only their selector location: top-level JSON `model` for OpenAI Chat/Responses and Anthropic, and the exact `/models/{key}:method` path segment for Google. Unknown fields, query strings and provider payload shape otherwise remain unchanged.

Budget templates are Admin-only configuration. Admin OpenAPI owns `bgt_*` CRUD, one-template user assignment and capability override removal. Client/Portal expose only the five effective capability budgets and generic configured/default source; Managed Snapshot, Relay control and Android exports contain no template identity, revision, assignment or rule metadata. Image Generation uses `IMAGE_GENERATION`, `REQUESTED_IMAGES`, and the typed `OPENAI_IMAGES_GENERATIONS` / `DASHSCOPE_MULTIMODAL_GENERATION` protocols without an API or Snapshot version increment.

Snapshot v5 adds required Starter opening; `ManagedSnapshotV4` preserves the original wire shape and the download endpoint uses explicit versioned alternatives. Admin may save a missing opening as an unfinished draft, but Validate/Preview/Publish rejects it. Discovery/Bootstrap advertises `[4,5]`. Gateway v6 remains planned. Policy requirements and its ten optional defaults are unchanged. Use:

- `measix-s0-capability-delivery-contract-spec.md`;
- `measix-s0-enterprise-realm-experience-contract-spec.md`;
- `measix-s0-enterprise-tool-gateway-contract-spec.md`;
- `measix-s0-control-protocol.md`;
- relevant component/product/testing specs.

The current Client API requires refresh Idempotency-Key, rotating credentials and sessionIdleExpiresAt; enrollment is 201 and requires deviceName. Missing authentication returns `401 unauthenticated`, while a supplied but invalid credential returns `401 invalid_credential`. Known authorization denials preserve their owner across Client, Refresh and Runtime: `403 user_disabled`, `403 device_revoked` or `403 session_revoked`; an unverifiable Runtime JWT remains `401 invalid_session`. Admin enrollment issuance defaults to one hour when `expiresInSeconds` is omitted and accepts explicit durations from one minute through 24 hours. Enrollment exchange distinguishes an expired code (`401 enrollment_expired`), a consumed one-time code (`409 enrollment_already_used`) and an installation already bound to another user (`409 installation_user_conflict`); the latter does not consume the code. Admin user deletion requires exact username plus reason and exposes its durable terminal state. Once deny-first deletion begins, every old Client/Runtime access credential and refresh credential resolves to the stable `401 enterprise_identity_deleted` Problem; credential tombstones contain only irreversible digests, and no deleted-user private data is retained to implement this distinction. A later user may reuse the username only as a fresh `usr_*` principal. A new enrollment can bind the cleared installation to that principal, but never removes the old principal/credential tombstones or inherits the deleted user's private state. Feed HTTP fields/queries are camelCase; snake_case belongs only to the planned Gateway platform-tool schema. A durable unforwarded admission denial uses the same complete immutable deployment, resource, runtime-route and upstream attribution as a forwarded request; it differs by `forwarded=false`, empty semantic meters, `EXACT` completeness and terminal `SETTLED` state. Client and Admin budget views expose retained cumulative `usageMeters` independently from the current-window `used` and `reserved` values in `limits`, including for unlimited capabilities. Generated Android export carries these changes; actual Android consumers must explicitly adopt and verify them before compatibility/Freeze claims.

Admin reconciliation rows carry their request's safe `RequestUsageView` projection when the request still exists. This projection supplies immutable resource display name plus user/device display context and the same correlation, protocol, transfer result, bytes, semantic meters, completeness, settlement and budget fields used by the ordinary request-detail endpoint. The reconciliation-specific fields remain the reason, observed quantities and uncertain reservation. No prompt/body, Secret, credential header or private endpoint is added; deleted or otherwise unavailable request facts may omit the nested projection and remain resolvable by stable request ID.

The authenticated Admin self-service password endpoint requires Cookie Session plus CSRF, verifies the current password, checks the confirmed replacement, updates the Argon2id hash and revokes all Admin Web Sessions for that user in one transaction. Stable explicit failures are `403 invalid_current_password` and `400 password_confirmation_mismatch`; localized UI copy is not part of the wire compatibility key.

The fixed S0.2 Preview identity and later-stage boundaries are maintained in [S0 status](s0-execution-progress.md).

## 4. Canonical fixtures

Admin `Release` 与 `DraftPreviewResponse` 的必填 `snapshotSchemaVersion` 是只读下发协议诊断：分别来自不可变发布 Snapshot 和当前 canonical 编译结果；它不是发布序号或设备兼容性判定。该投影仅扩展 Admin OpenAPI/generated types，不变更 Client Snapshot 协议和已发布内容。

Portal grant/exchange/restricted Web Session operations are in the Client OpenAPI. Only the two canonical `/api/client/v1/enterprise/updates` GETs accept the additional Portal Cookie scheme; other Client/Admin/runtime operations retain their own authentication. Hub stores ticket/cookie digests in `PortalSession` linked to the parent Android Session; exchange atomically consumes the ticket, and a Portal Cookie never acquires Admin or general Client privileges. The independent Portal generates TypeScript from this same OpenAPI and records its input SHA256. Hub distribution/configuration is documented in [operations](operations.md); Portal UI/build ownership remains in the sibling Portal repository.

`PlatformEnrollmentMaterial` in the Client OpenAPI describes the native scan/paste document, not the HTTP Enrollment request. Its canonical sample is `api/fixtures/enrollment/platform-v1.json`; Android export and Admin's `generated-client.ts` derive from this same source. The Admin generator emits both surface type files; it imports only the native material type from the Client output and does not call Client HTTP APIs. Control Protocol §8 owns trust checks and byte limits. Private configuration-file import, if any, is an Android concern and is not an Enrollment or Portal protocol.

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

Core/Android 协议升级的执行参考与本次 v5 收敛计划见 [第 11 节](#11-coreandroid-协议升级参考与-v5-收敛计划)。该节区分生产支持、候选开发、基础功能降级和历史数据保全，不表示计划已实施或验收。

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

## 7. Freeze candidate evidence

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

New draft evidence writes exclusively to `.artifacts/s0-freeze-candidate.json` (or an explicit new output), without overwriting an existing candidate. Current CAP tooling pins Snapshot v5 and requires CAP-C0-010 evidence for defaults, shared v4/v5 wire cases and strict Starter opening/version isolation; these schema checks do not prove Starter authoring, Android execution or the S0.2 ERX gate; C7 requires independent clean-source rebuild/replay and separate validated finalization. Final acceptance and unimplemented later-stage gates are defined in [release](release.md); do not infer them from the filename.

Current Starter output is Snapshot v5; supported immutable v4 releases retain their bytes and semantics. Bridge v3 is unchanged. Contract changes regenerate every consumer/export and explicitly preserve the supported version boundary; an old candidate report cannot certify current work. S0.3 additionally pins Gateway Control OpenAPI, Gateway build identity, surface/catalog fixtures and scenario evidence; current S0.2 evidence cannot prove those later capabilities.

## 8. Current contract strictness and extensibility

S0 contract tests must prove architecture rules including (not a claim that all current tests already do):

- clients tolerate unknown optional fields only where the response contract permits extension; closed Snapshot objects reject undeclared fields and require coordinated producer/consumer contract changes;
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
8. impact on every current consumer and generated export.

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
Android export/generation consistency for the client-control contract
```

Freeze identity generation is a candidate/C7 concern and must not be confused with ordinary pre-freeze contract drift checks.


## 11. Core/Android 协议升级参考与 v5 收敛计划

### 11.1 适用目标与状态

本节是跨仓库升级工作的执行参考。跨组件语义仍由 [Control Protocol §10.10](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md#1010-managedpolicy) 拥有；滚动支持策略、基础控制兼容承诺和本地配置退役边界已同步到该 authority 及对应 Android 合同/测试要求。后续升级仍须先修改所属 authority，再实施。本文不另行定义 DTO 或复制完整架构测试矩阵。

2026-10-02 用户确认生产仍使用 Snapshot v4，v5 尚未正式发布。本次目标是完成 v5 配置闭环，同时保留 v4 消费；面对未来版本，企业配置整体拒绝应用并明确提示升级，但有效身份下的企业历史访问、空间切换、个人完整功能和本地退出持续可用。恢复和导航不以配置解析成功为前提。身份真实过期、撤销或删除仍遵循原有权限合同。

实施基线为 Core `c63e6af`、Architecture `79408bd`、Android `50398b087`；实现和验证记录见 §11.14。基线不是生产部署声明；生产实际构建及可回退发布仍应在执行上线前核对。

### 11.2 滚动支持与退役

Android 明确列举支持的 Snapshot 格式，不从最大版本、连续范围或 App 显示版本推断。开发候选默认支持“当前生产格式 + 本轮目标格式”；它是有限支持窗口，不累积全部历史 decoder。

| 阶段 | Android 候选消费集合 | 处理规则 |
| --- | --- | --- |
| 当前生产 v4，开发 v5 | `{4,5}` | 生产 v4 正常消费；验证 v5；未知 v6/v7 进入不兼容状态 |
| v5 已上线，仍在 v4 回退窗口 | `{4,5}` | 不因 v5 上线立即删除 v4；保留约定回退路径 |
| v5 稳定，v4 回退窗口关闭，开发 v6 | `{5,6}` | 经数据保全验证后退役 v4 网络 decoder；v4 内容给出明确不兼容结果 |
| 后续迭代 | 当前生产与目标格式 | 按同一流程滚动，不能机械按日历或版本号删除支持 |

若生产或承诺的回退路径仍依赖旧格式，就尚未满足退役条件；应延后退役或显式调整支持窗口。停止消费旧网络协议不等于删除本地历史、身份迁移、旧发布查询或历史数据保全测试。

Core 的新发布格式、可读取的历史发布格式、Android 可执行的格式是三件事。Core 保留不可变旧发布，不代表未来 Android 永远能应用它。历史重新发布仍按原合同生成新身份/generation；实际回退前必须核对目标 APK 的支持范围。

当前不引入按设备自动降级、删字段转换、多 active generation 或客户端能力上报系统。固定构建支持矩阵即可支撑本轮；以后有独立产品需求再设计。Core 激活较新格式时，支持窗口外的 Android 可以停止企业新执行，但必须满足基础功能持续可用的承诺；不要求每台设备都升级才允许发布。

### 11.3 各端修订职责

| 责任方 | 本轮修订 | 完成依据 |
| --- | --- | --- |
| Architecture | 在原协议 authority 明确有限滚动支持、稳定基础控制边界、配置不兼容与身份失效的区别；统一恢复/导航/退出规则 | 对应 Android 合同及 testing 要求引用同一规则，无平行语义 |
| Core | 固定 v1 基础控制 profile 和 Snapshot 外层版本识别约定；继续提供实际版本及不可变 bytes/hash；共享用例覆盖未来版本与控制面组合 | 受支持真实 Android 构建能完成基础控制；版本/hash/304 与发布行为正确 |
| Android | 保留 `{4,5}`；集中版本预检与同步结果；从恢复、历史、导航、退出路径解除配置加载依赖；明确本地配置退役策略 | 不兼容/配置加载失败前后及重启后的完整基础功能测试 |
| Admin/发布工具 | 沿现有发布审查展示目标格式，记录准确构建支持集合、基础控制兼容证据和回退目标 | 不以 App 名称或双方支持集合交集冒充目标发布兼容 |
| 派生消费者 | 有关联 OpenAPI 变化时同步 Portal、导出、fixtures 与生成摘要 | 干净工作树上漂移校验通过；不因相同 Snapshot 版本跳过接口影响检查 |

基础控制 profile 至少覆盖 Discovery、Enrollment、Bootstrap、Refresh、Logout、Managed State 和既有身份 Problem 语义。Snapshot 升版不能顺带使旧客户端无法解析 Bootstrap 或续期；嵌套 Managed State 的必需字段和枚举同样属于边界。当前严格 DTO 不能将任意新增 optional 字段视为安全扩展。

默认保持已发布基础响应的结构与解释稳定。新增业务数据优先放独立接口；已有明确可扩展对象才允许经过消费验证的 optional 扩展。确需破坏性改变基础控制时，另行版本化并保留受支持旧客户端所需的控制路径。它有独立生命周期，不要求保留旧 Snapshot 执行能力，也不要求机械跟随 Snapshot 编号升级。

Snapshot 继续使用现有外层 `schemaVersion`，不为本轮新增包装协议。有界读取并检查 JSON 语法、重复键和合法版本后，不支持的版本直接形成兼容性结果；不能先解码其未知内部结构。受支持版本继续严格验证类型、引用、身份、generation、hash 和 ETag，并整体提交。版本缺失/错型、已知格式损坏、网络失败与身份失效分别处理，不全部提示升级。

### 11.4 Android 具体收敛入口

沿现有 owner 修订，不另建 Session、配置存储或恢复协调器：

1. `PlatformSnapshotCompatibility`、`PlatformWireCodec`、`PlatformControlClient` 保持单一版本预检入口，保留 v4/v5 的精确语义。未知格式不产生候选、Applied 回执或部分配置；不能用全局忽略未知字段替代版本分流。
2. `EnterpriseAppliedStore`、`EnterpriseSessionController` 将身份/manifest 的恢复与配置正文读取结果分开。修订前，`load()` 和 `commit()` 在 manifest 有 Applied 时读取 candidate，`recover()` 可将读取失败发布为整个 `EnterpriseState.Failed`。修订后有效身份可独立恢复，配置不可读只关闭配置依赖能力；保留原 Applied 的身份提交与发布新配置的严格提交分别处理。未知网络版本不应造成文件损坏状态；真正的身份或凭据损坏仍单独处理。
3. 本地 Applied 是本地数据合同，不等于网络 wire。移除旧 wire decoder 时，已有 canonical 配置要么通过现有有序迁移保持可读，要么明确标为不可用于执行并保全原应用事实；两者都不能阻断身份、历史或退出。不能通过删除 Applied 事实、清库或伪造同步成功来消除不兼容。
4. `EnterpriseSynchronizationService` 负责有限、可行动的同步结果，UI 只消费该结果。进入页面、恢复和切域不引入循环下载或隐式重放；同步检查与实际下载/应用的触发规则需和当前手动同步策略统一。不兼容状态不构成全局错误弹窗，不在切到个人域后继续打断用户。
5. 历史列表、会话详情、历史消息及开场详情按原企业主体授权读取，不要求当前模型/助手/MCP 定义存在。配置相关补充信息不可用时展示历史已保存内容；新发送、续写及依赖企业配置的操作继续执行准入检查。不得把企业会话静默转为个人执行。
6. `EnterpriseExitService` 的本地撤权和资源收口不依赖 Snapshot 解码或重新同步。已有远端 logout 失败捕获应保留；验证 Core 不可达或返回未知业务数据时，退出可以终结而不被卡住。真实资源清理失败必须保留可重试状态，不能冒充清理完成。
7. `ApplicationRecoveryCoordinator` 保留原唯一恢复流程。已有企业错误局部化保护继续保留，检查后续历史投影、运行恢复、待退出收口不会重新加载失效配置而升级为全应用恢复失败。Room/个人文件等真正的数据恢复故障不在本轮容错范围内。

无需为展示升级提示先引入最低 App 版本服务、自动下载或永久后台轮询。诊断可显示实际目标格式及本 APK 支持集合；没有可信发布信息时不猜下载地址或最低 App 版本。

### 11.5 每次升级的执行顺序

1. **登记基线。** 固定生产 Core/Android 构建、active release 的真实格式、当前支持集合、本轮目标和仍承诺的回退格式。核对实际发布记录，不只读取开发编译常量。
2. **分类变更。** 列出 Snapshot、基础控制、独立功能 API、Runtime/Relay、Portal 和本地存储的影响；分别判断是否改变 required、枚举、执行语义、默认值或权限。修订未发布候选可沿用候选版本，但需新的 Git/合同摘要；既有发布记录不能原地改写。
3. **先定语义。** 更新 Architecture 的所属 authority 与验收要求，再更新 Core OpenAPI、共享正反例、生成物和实现。Android 从固定导出消费；实现变化和本地迁移留在各自仓库。
4. **完成两条验证线。** 一条验证当前生产/目标格式的真实配置闭环；另一条用未来格式及异常接口验证基础功能持续可用。不能以“成功提示升级”替代实际退出、历史、导航和个人功能验证。
5. **形成候选证据。** 按现有 release owner 固定各仓库提交、合同摘要、APK/服务构建及场景结果；漂移检查、定向测试和实际设备证据均针对同一候选。普通生成一致不代表设备验收。
6. **部署与激活分开。** 可先部署保持当前 active 发布的 Core，再交付滚动支持 Android，最后显式激活新格式。若旧 APK 未具备本轮基础功能隔离保证，应先交付修订 Android；不能声称服务端能修补已经安装的旧 APK。已证明能受控降级的旧客户端遇到新格式可提示升级，无须为它保持旧执行通道。
7. **核对恢复路径。** 配置回退使用正常历史重新发布，先核对所有目标客户端仍支持该格式；App/数据库降级不是配置回退，不默认允许。验证升级客户端后原 Session 可同步恢复，不要求用户清数据或重新接入。
8. **关闭窗口再退役。** 生产稳定且旧格式回退承诺关闭后，下一轮才调整 Android 支持集合；移除旧 wire 路径时保留基础控制、历史数据迁移和仍有效的保全测试。

### 11.6 本轮 v5 验收与后续复用

完整语义矩阵归 Architecture 测试规范；本轮执行至少记录以下用户路径，后续按实际生产/目标版本替换输入，不累加全部历史 decoder：

| 输入/操作 | 验收结果 |
| --- | --- |
| 当前生产 v4；候选 v5，各自有/无 Starter | 修订 Android 正确消费，保持各自开场语义，成功后才上报 Applied |
| v6/v7 外层版本，内部含未知字段/枚举/嵌套结构 | 只形成版本不支持结果；不要求实现 v6/v7 业务功能 |
| 支持集合完全无交集；有交集但 active 格式不支持 | 有效身份可以接入/恢复，企业配置明确不可用 |
| 已有企业历史及 Applied；首次接入无 Applied | 两类状态都可切个人并完整使用个人聊天、设置与本地功能；已有企业历史仍按身份权限读取 |
| 不兼容后重启、后台恢复、反复切域、主动重试 | 无崩溃、无永久 Loading、无重复自动同步或阻断弹窗；个人操作不触发企业同步 |
| 不兼容/本地旧配置不可用时退出，Core 正常或不可达 | 本地退出可终结，资源按原协议收口；远端未确认单独呈现 |
| 同步期间切域、换 Session 或退出 | 取消和迟到结果保持原归属，不污染个人或新 Session |
| 在同一安装中升级到支持目标格式的 APK | 身份和历史保全，原 Session 同步后恢复配置能力，无需重接入 |
| 未来退役旧 wire 后保留其磁盘配置/历史 | 本地迁移或配置不可用路径明确，基础恢复、历史和退出正常 |
| 身份撤销/删除，以及已知版本坏内容或网络失败 | 保留真实权限限制和错误分类，不能借不兼容容错开放已撤销访问 |

共享未来版本 fixture 必须通过真实下载/解码链使用；设备验证应在真实企业历史和个人数据存在时操作 UI。第一次建立隔离保证使用本轮修订 APK；以后协议升级还须用未修改的上一生产 APK 验证 Core 的基础控制仍兼容。测试专用未来内容不冒充正式发布的 v6/v7 合同或业务验收。

### 11.7 本轮执行顺序与阶段交付

以下阶段是本次 v5 里程碑的执行计划，不是新的 S0 子阶段编号。各阶段进展见 §11.14；先补能证明缺口的失败用例，再实现并验证，已有正确行为只补回归证据。各仓库保持独立变更；最终验收固定同一组源码/合同/构建，不能拼接不同候选的通过结果。

| 顺序 | 责任方与工作包 | 前置条件 | 交付物及进入下一阶段的条件 |
| --- | --- | --- | --- |
| P0 | 固定生产与开发基线 | 已确认本轮范围 | 准确生产 Core/APK 身份、真实 v4 发布、候选提交、备份及回退目标；准备独立测试数据与专用设备，不能用开发 HEAD 代替生产 |
| P1 | Architecture：固定语义和场景 | P0 | §11.8 所列 authority 无冲突；明确 `{4,5}`、基础控制 profile、状态/访问表和同步触发规则；形成可供 Core/Android 共同实现的版本 |
| P2 | Core：控制面边界、共享用例与导出 | P1 | §11.9 的合同/HTTP/发布测试通过，交付固定来源摘要的 Android 合同和接收 cases；正常生产代码仍只发布真实支持格式 |
| P3a | Android：配置读取与身份恢复隔离 | P2 固定输入 | §11.10 A1/A2 的存储、恢复、同步修复及原子提交测试通过；无须 UI 即可证明配置失败不损害有效 Session |
| P3b | Android：历史、导航、执行和退出接线 | P3a | §11.10 A3–A5 的查询/命令/竞态测试通过，页面消费统一状态；全量构建及设备组件测试通过 |
| P4 | Core/Admin/Portal：发布说明和派生同步 | P2；最终对齐 P3b | §11.11 所列生成物、摘要及发布检查通过；发布页面不虚构设备兼容性；跨仓库基线检查无漂移 |
| P5 | Core + Android 联调与里程碑验收 | P3b、P4 | §11.13 的真实 v4/v5 闭环、未来版本降级、本地恢复和安装升级全部取得当前候选证据；未执行不计通过 |
| P6 | 发布准备及分步上线 | P5 | 固定发布包、兼容说明和回退计划；按 §11.5 部署 Core/交付 Android/激活配置；本计划不直接执行生产部署 |

P4 的派生生成可在 P2 后开展，最终门禁必须等待 P3b；不存在 Android 尚未消费最终合同就冻结发布的捷径。空间管理查看 MCP 工具是独立功能，不作为本轮版本隔离工作的前提。

### 11.8 Architecture 实现方案

在既有 authority 内修改，不新增第二份协议规格：

- `measix-s0-control-protocol.md` §2.1、§8、§10.10.1–3：固定基础控制 profile 的扩展限制、滚动支持与退役、版本预检、有效身份和配置可用性的独立判断。保留现有已发布 v4 数据规则；明确旧 release 保全不强制所有未来 APK 保留旧 wire 消费。
- `measix-s0-android-integration-contract-spec.md` 的 Managed State、同步、UI、持久化章节：明确本地配置不可读时仍能恢复身份和历史；把前台自动检查/自动应用的旧文字与当前手动同步策略统一。本轮选择保留现有首次接入一次初始化、显式同步/重试；恢复/切域不自动下载，新的企业执行仍做状态复验，已知不兼容持续显示。重启后未检查不能显示已兼容，也不进入无限 Loading。
- `measix-s0-enterprise-realm-experience-contract-spec.md`：如现有空间/历史访问规则隐含配置可用条件，在其所属条目修正并引用 Control Protocol，不复制 wire 表。
- `measix-s0-android-client-testing-spec.md` 和 `measix-s0-control-hub-testing-spec.md`：增加本轮基础控制与基础功能隔离场景，沿现有场景编号分配；本计划中的 P/A 编号只用于工作包，不冒充正式测试 ID。跨组件证据在现有 Realm/Experience testing 文档引用；当前计划不宣称 S0.3/S0.4 Gateway 验收。
- `measix-stage-document-index.md` 只按必要变更更新入口。完整升级操作步骤仍由本文维护，Android 本地参考记录实际实现和运行命令。

P1 审查必须逐项回答：配置版本不支持、配置正文坏数据、基础控制无法解析、身份失效分别能做什么；哪些行为需要联网；未知版本是否会触发自动同步；旧 decoder 何时可以删除；生产激活和客户端发布顺序如何保证。本轮不改变身份过期/撤销规则，不新增离线永久企业授权。

### 11.9 Core 实现方案

**C1 基础控制合同。** 在 `api/client/client-control.openapi.yaml` 现有 schema 和 endpoint 描述中标明 P1 确定的稳定 profile。以实际生产构建的合同为比较基线，逐项检查 Discovery、Enrollment、Bootstrap（含 Managed State）、Refresh、Logout 和身份 Problem。默认不新增 Client 字段、不改路径或枚举；发现破坏旧基础消费者的差异，先修正差异，不能仅更新摘要放行。Admin 独立展示字段无需搬到 Client。

**C2 接收用例。** 扩展 canonical `api/fixtures/client-integration/snapshot-reception-cases.json` 及生成/导出 owner，给每项明确预期分类：v4/v5 接受、未来合法版本不支持、版本字段错误、已知格式坏内容、304 条件非法。未来版本含未知内部对象/枚举；重复键、过大正文等仍按输入限制拒绝。未来用例属于测试输入，不加入 `SupportedSnapshotSchemaVersions()`，也不能被当作合法 v5 fixture 验证。

**C3 服务端验证。** 在 `backend/internal/contract` 增补基础控制 profile 和接收分类测试，扩展现有 `snapshot_versions_test.go`；在 `hub/httpapi` 扩展 `old_client_release_test.go`、`client_integration_test.go`，证明同一真实 Session 的 Bootstrap/Refresh/Logout 不依赖客户端能否应用目标 Snapshot；在 `hub/runtimecontrol/snapshot_version_test.go` 保留历史 bytes/hash、版本与重新发布行为。未知格式 HTTP 注入只放测试 adapter/代理，不给正式编译器增加 v6/v7 开关。

**C4 原有运行边界。** `capability/snapshot.go`、`httpapi/client_snapshot.go`、Relay generation barrier 默认保持不变；仅在前述测试证明实际缺口时修改。继续拒绝过期企业执行，不允许客户端用旧 Applied 绕过 active generation，也不自动重放被中断工具/模型调用。

**C5 固定导出。** 经原 `scripts/checks.mjs generate` 更新 Core 生成物与 fixtures/export manifest，再由 Android 显式导入固定产物并运行其生成检查。`protocol-baseline.json` 只在差异已审查后更新；保留可复算合同与构建身份，不把一次 hash 对齐当成运行兼容证明。

### 11.10 Android 实现方案

下述状态名称表达拟定职责，具体 Kotlin 命名可沿现有类型统一；不创建平行配置服务或额外持久化数据库。

**A1 在原状态中表达配置加载结果。** 扩展 `LoadedEnterpriseState` / `EnterpriseState.Available`，明确区分未应用、已读可用、已应用但不可读取（含诊断及原 revision）三种结果。`manifest.applied` 仍表示上次提交事实，不能因加载失败清空；`configuration == null` 不再同时充当所有失败原因。网络同步结果继续归 `EnterpriseSynchronizationService`，不复制为第二套同步状态。`EnterpriseState.Failed` 保留给无法恢复可信身份/manifest 的情况。

`EnterpriseAppliedStore.load()` 先验证 manifest 和必要凭据，再在配置边界返回加载结果；取消不吞掉，配置加载异常保留原因，只关闭配置依赖能力。沿现有持久化 owner 区分两种提交：身份/选择/凭据更新在 Applied 引用不变时不要求重新解码正文；替换 Applied 必须完整验证新 candidate、staging bytes/hash 并原子提交 manifest。不能把新配置发布也改成宽松提交。配置文件保留到原回收流程允许删除时，不新建失败副本目录。

本轮优先只增加派生状态，不为内存错误增加 manifest/Room 版本。若实际落盘形状确需改变，必须先列出有序迁移和升级样例；不能把 Snapshot 5 当成本地 manifest 版本。未来取消 v4 网络支持后，本地 canonical 仍能解析时可用于只读历史补充；执行读取仍要验证实际支持范围，不能绕过退役。

**A2 同步和执行采用不同读取要求。** 调整 `platformConfiguration()` 的同步输入，使其可以返回有效 Session 和可选的可用缓存，不被 `store.appliedCandidate()` 的失败阻断。缓存不可读取、实际版本未知或不在本次 Bootstrap/客户端支持集合时，不发条件 ETag；请求完整配置。`readExecution()`、`captureExecution()`、`confirmPlatformExecution()` 则明确要求可用配置和当前准入，沿同一检查收口。

同步修复还要覆盖当前 `applyValidated()` 的同 generation 冲突判断：正常可读旧配置继续严格比较；旧配置不可读时，经真实 Core 身份、目标 generation、完整 Snapshot hash/ETag、引用校验后，允许原 owner 写入新的本地 revision 修复。不得接受低于原 Applied 的 generation；仍可读取的旧 release/hash 等事实若冲突必须报错，不能以修复为由忽略。无法恢复旧内容的部分由权威完整下载重建，过程保全原 revision，提交失败保持原 manifest。该分支是显式修复路径，不放宽正常相同 generation 的冲突检查。成功后才上报 Applied；304 无可用缓存必须失败并允许显式重新获取。

未知版本由现有 `PlatformSnapshotCompatibility` 预检和 `EnterpriseSynchronizationService` 形成可行动结果；不调用 mapper、不产生新 Applied、不触发退出。启动恢复时配置加载状态可由磁盘重建，网络兼容状态未检查就明确未检查；不以提示内存被清空代表兼容或执行已获准。

**A3 历史读模型不等待有效配置。** 保留 `allowsDataAccess()` / `withRealmAccess()` / `RealmSelection` 的原主体与时效校验。检查 `SelectedRealmPagingSource`、`ConversationQueryService`、历史 VM、聊天详情与附件预览：它们读取已持久化数据只需原数据访问权，不能隐式调用执行配置检查。

`ConversationQueryService` 当前把消息投影与 `settings.observeConfiguration()` 合并；在该 query owner 内将当前配置装饰设为可不可用的附属结果，先保持正文/历史开场/附件投影可读。当前助手或模型不可解析时使用已保存身份/名称或明确“当前配置不可用”，不能套入个人默认模型。只读页面与新发送/续写动作分别判断；新聊天、发送、重试、工具、企业语音/图片等入口统一经过原执行准入，不能遗漏旁路。

**A4 退出与恢复沿已有机制验证。** `manifestForExit()` 已能独立读取 manifest，`beginClosing()` 已先撤权，`EnterpriseExitService` 已捕获远端 logout 失败；保留这些路径，优先补行为测试，仅修实际断点。验证刷新凭据待恢复、配置正文不可读、同步在途和远端不可达时仍能收口。资源取消、租约归还必须真实完成；退出后个人页面不得等待远端成功。检查 `ApplicationRecoveryCoordinator` 的后续投影、Turn 恢复和 pending exit，确保不把已局部化配置错误重新升级为全局失败。

**A5 UI 与触发收敛。** `EnterpriseApplicationService`/企业 VM 统一输出身份、最后应用事实、配置加载结果和同步问题。企业页显示一处提示与“同步/查看诊断”动作；未知较新版本提示升级客户端。配置错误不指向清库、不反复弹窗、不自动切域。保留现有首次接入一次同步、手动同步/重试和执行前检查；个人操作不触发企业同步。历史页仍可打开、空间入口和退出可操作；切域/退出后的旧同步结果按原 Session 丢弃。

不把“任意网络失败后是否继续旧企业执行”扩大为本轮策略重设计；本轮保证基础功能独立，企业执行继续原准入/显式恢复边界。独立远程文件能力按自己的身份和接口检查，不从 Snapshot 失败推导文件服务失败。

### 11.11 Admin、Portal 与发布工具方案

- Admin 已有 `snapshotSchemaVersion` 投影，先复用原预览/发布详情。仅在缺少必要说明时补“目标配置格式”与升级影响文案；不新增设备能力数据库，不按 `appVersion` 宣称所有设备兼容，不要求所有设备已消费新格式才能发布。
- `scripts/verify-preview-contract.mjs` 保持原跨仓库漂移检查，修复 Portal 既有来源摘要漂移须通过 `pnpm generate:api` 重新生成并审查实际差异，不能手改 hash。Android 使用固定 Core 导出，不从 sibling HEAD 隐式生成不同合同。
- `scripts/build-preview-release.mjs` 和 `docs/release.md` 复用现有产物/evidence owner，补实际 Android 消费集合、基础控制兼容结果及配置回退目标的记录和校验。只有既有 release schema 无合适字段时才增加明确字段并补工具测试，不产生第二份发布身份文件。
- Portal 业务默认不改；合同变化影响到它才同步类型和验证器。Portal 不是企业退出/个人导航的必经路径，不能用 Portal 初始化成功替代 Android 基础功能验收。

### 11.12 验证分层、入口和通过条件

| 工作包 | 优先扩展的现有测试 | 必须新增/确认的断言 |
| --- | --- | --- |
| C1–C3 | Core `internal/contract`、`httpapi/old_client_release_test.go`、`client_integration_test.go`、`runtimecontrol/snapshot_version_test.go` | 基础控制在不兼容配置下仍正确；历史 v4 保全；目标版本与 hash 真实；未来输入只用于测试 |
| A1 | `RealmAccessTest`、`EnterpriseStarterAppliedStoreTest`、`ApplicationRecoveryCoordinatorTest` | manifest/credential 有效而正文不可读时身份和原 Applied 引用保留；选择/身份更新可提交；新配置提交失败原子回滚 |
| A2 | `PlatformSnapshotVersionCompatibilityTest`、`PlatformWireTest`、`EnterpriseSynchronizationServiceTest` | 未知版本在 DTO 前拒绝；无缓存不发 ETag；同 generation 修复成功；冲突/回退拒绝；不伪造 Applied |
| A3 | `SelectedRealmPagingSourceTest`、`ScopedConversationQueryTest`、`ConversationQueryServiceTest`、`HistoryVMTest` | 非空历史正文/开场/附件可读；配置流失败不吞正文；切主体后旧分页/结果失效；发送依然被拒绝 |
| A4/A5 | `EnterpriseExitServiceTest`、`EnterpriseApplicationServiceTest`、`EnterpriseVMTest`、`EnterpriseSynchronizationTriggersTest` | 退出不读 Snapshot；远端失败不阻止本地终结；个人与切域不触发同步；真实撤权仍限制历史 |
| 设备 | `ApplicationRecoveryAndroidTest`、`EnterprisePageAndroidTest`、`PlatformSnapshotCompatibilityLiveAndroidTest` | 实际恢复/退出/导航/UI 和本地历史操作，见下一节 |

现有测试文件不足时，在相同 owner 测试目录增加聚焦行为测试；不为通过计划而把独立故障全部塞进一个大型测试。不新增源码字符串扫描来代替运行行为。

命令入口如下，按阶段执行；实际执行结果与尚未完成项见 §11.14：

| 工作目录 | 命令/操作 | 用途 |
| --- | --- | --- |
| Core `backend` | `go test -p 1 ./internal/contract ./internal/hub/httpapi ./internal/hub/capability ./internal/hub/runtimecontrol ./internal/relay -count=1` | 定向合同、HTTP、发布及运行准入回归 |
| Core 根目录 | `node scripts/checks.mjs generate`，随后审查生成 diff | 更新唯一派生产物；只在实施环境运行 |
| Core 干净候选 | `node scripts/checks.mjs static`、`node scripts/verify-preview-contract.mjs`、`npm run test:tooling` | 生成/格式/vet/跨仓库摘要/发布工具检查；static 会执行生成，不是只读检查 |
| Core `backend` | `go test -p 1 ./... -count=1` | 本轮服务端候选整体回归，带特殊环境的场景另记 |
| Core 根目录（Admin 有修改） | `pnpm -C console test`、`pnpm -C console typecheck`、`pnpm -C console build`，以及 `node scripts/e2e-harness.mjs` | 原发布 UI 和真实 Hub 浏览器路径 |
| Portal 根目录（派生同步） | `pnpm generate:api`、`pnpm typecheck`、`pnpm test`、`pnpm build` | 从权威重生成，验证派生类型/验证器与构建 |
| Android 根目录 | `python tools/generate-enterprise-wire.py --check` | 固定导出与 Kotlin 生成一致性；先准备可导入 `yaml` 的仓库验证环境 |
| Android 根目录 | `.\gradlew.bat :app:testDebugUnitTest --tests "<上表测试完整类名>" --no-parallel --max-workers=1` | 每个工作包先 Red 后 Green；可在同一次串行 Gradle 调用追加多个 `--tests` |
| Android 根目录 | `.\gradlew.bat test assembleDebug lintDebug assembleRelease --no-parallel --max-workers=1` | 跨模块/持久化边界候选全量门禁 |
| Android 根目录、专用设备 | `.\gradlew.bat connectedDebugAndroidTest --no-parallel --max-workers=1` | 先固定 `ANDROID_SERIAL`，另按 `tools/compatibility/README.md` 执行 opt-in 真实 Core 探针及 UI 实操 |

所有进程/测试用独立数据库、端口和专用测试身份；不重置生产或日常演示数据。构建工具自动安装/卸载可能影响 Debug 数据，持久化升级与 force-stop 场景须使用明确保留数据的安装顺序，不能把重装后新数据当作升级保全。

### 11.13 跨端验收步骤及证据

在现有 `tools/compatibility/README.md`、真实 Core probe 和 Core 测试环境上扩展，不启动新的长期测试服务。每一步都固定请求所属主体、原 Applied、历史正文/附件摘要、客户端版本和网络请求计数，保留失败日志；未经修复不得靠自动重试计为通过。

1. **建立生产兼容基线。** 隔离 Core 使用真实历史 v4 发布，通过原接入流程绑定；Android 建立非空企业聊天历史和个人聊天/设置/本地 MCP 数据。修订 APK 覆盖安装升级后先验证 v4、身份、历史及个人功能，不能清数据。
2. **验证 v5 闭环。** 通过真实 Admin/API 保存、校验、发布 v5；修订 Android 显式同步，验证 Starter 首发、已保存开场、重开和实际 Provider 请求。确定性 adapter 能证明集成内容；真实外部供应商如未运行须另记，不冒充验证。
3. **未来版本注入。** 使用隔离 HTTP 代理转发真实 Core 控制请求，只对声明过的 Discovery/Bootstrap 支持集合和 Snapshot 响应注入 v6/v7 及未知内部结构，分别测试有交集/无交集。标记为故障注入证据，不伪造正式 Core 发布，也不修改正式 compiler。需要同步触发时由用户动作触发；不能暗中让 Android 实现 v6/v7 decoder。
4. **在同一安装操作基础功能。** 企业页升级提示可读，企业历史列表/正文/开场/附件可打开；切个人后发送消息、使用个人 MCP、修改并重开设置、访问本地文件等原有能力可用，且请求不携带企业凭据。检查企业同步请求不会随个人操作增加。企业新发送拒绝且保留输入，无 Provider/工具转发。
5. **进程与竞态。** force-stop 后重开，测试初始选中企业和个人两种情况；前后台及双向切空间不持续请求、不弹循环错误、不永久 Loading。同步下载中切域/退出/换 Session，确认取消和迟到结果不污染新状态。
6. **独立异常分支。** 用复制的测试数据破坏配置正文或提供不支持的本地配置，保留有效 manifest/credential，验证 A1–A4；另测已知版本坏内容、网络失败、身份撤销及过期。上述原因不可互相冒充，不借身份无效继续读取企业历史。
7. **恢复与退出分开建场景。** 一组撤销注入后同 Session 完整同步，确认不兼容提示清除、历史不变和执行恢复；另一组在不兼容状态分别对可达/不可达 Core 执行退出，确认本地退出终结和个人继续可用。未来新 APK 恢复路径在真正 v6 开发时用实际 v6 APK 复测；本轮不能用伪 v6 decoder 宣称该版本业务验收。
8. **发布回退演练。** 在仍保留 `{4,5}` 的候选上重新发布历史 v4，验证新 generation 正常同步、旧发布不变。该演练不做数据库倒退或 APK 降级；v4 支持退役前再次核对承诺的回退范围。

已有 `PlatformSnapshotCompatibilityLiveAndroidTest` 主要验证控制链、状态往返和两节点历史摘要，需补上述 UI、个人实际功能、配置不可读和退出情形，不能仅重复运行旧 probe。未来版本注入本轮验证 v5 APK 的降级；下一轮 v6 开发必须拿本轮未修改的已发布 APK 对新 Core 再跑基础控制/基础功能场景。

P5 通过需同时具备：固定候选的合同/生成检查、Core 定向及整体回归、Android JVM/Debug/Release/lint、设备组件与实操、真实 v4/v5 接入执行和未来版本降级证据。记录实际通过/失败/未执行、请求次数与关键 UI 截图；生产签名包如与设备包不同，安装升级验证及签名/优化差异单独说明。缺失关键路径只报告未完成，不用构建成功代替。

### 11.14 实现与验证记录

2026-10-02，以下为本地候选的实际结果，不代表生产部署。尚未完成的验收项单独列出，不以历史通过记录替代最终候选。

**已实现的收敛边界**

- Architecture 固定有限支持窗口、稳定基础控制、身份与配置失败隔离及同步修复要求。
- Android 在原 Store/Session owner 内恢复有效身份，保留原 Applied 的身份提交可容纳正文读取失败；新配置仍严格验证并原子提交。不可用配置不能获得执行准入。企业页统一显示配置诊断，长用户名与状态分行。
- 企业空间保留直接历史入口；历史 VM 按有效原 RealmSelection 读取企业根会话，不依赖当前助手目录，个人仍按个人助手筛选。切域/重入使旧查询失效，不增加第二份历史存储。
- Core 保持 Client 基础 DTO 结构，补 Refresh/Bootstrap/Logout 不依赖配置应用的回归。共享接收 cases 包含 v4/v5、未来 v6/v7 与坏版本输入；发布工具分别记录新发布格式及 Android 消费集合。未增加旧 v5 候选兼容、转换或迁移。
- Portal 只重生成派生产物；Admin E2E 修复选择器、菜单关闭等待和折叠诊断操作，未放宽业务断言。

**已执行门禁**

| 范围 | 结果 |
| --- | --- |
| Core | 完整 `go test -p 1 ./... -count=1`、42 项工具测试通过 |
| Admin / Portal | Admin 244 项测试、类型检查和完整浏览器 E2E 通过；Portal 94 项测试、类型检查、构建通过 |
| 派生产物 | 原生成链、Android wire `--check`、跨仓库 `verify-preview-contract.mjs` 通过 |
| Android | 历史修复后的 `test assembleDebug lintDebug assembleRelease --no-parallel --max-workers=1` 通过；app Debug JVM 报告 2523 项、零失败；历史 VM/作用域查询 8 项定向测试及企业页 27 项设备测试通过 |
| 安装升级探针 | 历史和当前 APK 使用同一覆盖安装探针；两次实际执行与新增测试源的 lint 通过 |
| 设备组件 | 同一候选分模块完成：app 389 项、17 项显式跳过、零失败；speech 18 项、2 项跳过、零失败；workspace 12 项、1 项跳过、零失败。联网完整命令两次在系统 TTS 首次合成超时；离线完整命令在 ADB 短暂断连时中止。后续离线 speech/workspace 完整运行通过，冷启动 TTS 单测 3.5 秒通过。保留全部失败，不宣称一次完整命令全绿 |

**真实服务与界面证据**

| 场景 | 已观察结果与范围 |
| --- | --- |
| v5 闭环 | `npm run device:real` 的 Admin 实际保存、审查并发布工作区 MCP 与助手绑定；active 为 v5 generation 11。Android 接入、无预同步重开、三条 Starter 预填、两节点历史及真实模型流式/辅助调用通过，release/hash 与 Core 一致 |
| v5 Starter 首发 | 当前已提交 Core/Android 运行带 Android lane 的隔离 E2E：真实 Admin 发布 v5 generation 2，设备点击预填、首次发送、开场/Room 回读及重开通过；确定性供应商实际请求核验通过。该独立数据库及发布身份与 `device:real` generation 11 分开记录 |
| 未来格式 | 隔离代理注入 v6/v7 Snapshot 和 Bootstrap 仅声明 `{6,7}`；身份、Applied、非空历史保全，执行准备阶段拒绝，企业页显示升级提示。属于故障注入，不是正式 v6/v7 发布 |
| 个人使用 | 不兼容期间实际切个人、修改设置、个人聊天及 MCP `tools/call` 成功。请求无企业执行头，个人 MCP 无 Authorization，未触发企业 Snapshot 下载 |
| 退出 | 不兼容时真实 Core logout 返回 204，本地 SIGNED_OUT；另对 logout 注入 503，本地仍终结。两者均清除 Session、Applied 与 pending exit；离线退出后个人模型/MCP 保留且实际聊天和工具调用成功 |
| 正文损坏恢复 | 保留 manifest/凭据而破坏配置正文，重启仍可导航。实查暴露历史入口及空助手筛选缺口，修复后同一数据的历史列表/正文可读；同 Session、同 generation 11 完整同步写入新 revision 修复，历史 digest 不变，执行恢复 |
| v4 覆盖安装 | 固定旧 Android `29ecc109335c536c6f2b60841e3d4aace35b0dd3`；归档 1690 文件审计确认生产源码未改。历史 v4 经隔离库副本正式迁移及重新发布成为 generation 24。旧 APK 保存企业/个人各两节点历史、主题与 MCP；当前 APK `install -r` 后在同步前核验身份、Applied、节点身份/正文和个人设置，再同步恢复执行，均通过。两个空间的历史正文已实际查看 |
| Agent Space | Core/Admin 和 Android 浏览真实文件及 Markdown 内嵌图片；Android 在独立目录编辑、保存、重开，真实模型经工作区 MCP `read` 返回相同内容。追加真实探针通过 64 MiB 往返哈希、双编辑冲突/另存、条件复制移动、取消下载、切域取消、异空间拒绝、PDF/图片/Markdown 原生预览；未操作共享工作区的断开或替换 |

首次无 Applied 接入未来 v7、进程重开后再次拒绝、撤销注入后同 Session 恢复 v5 和恢复后重开均通过；界面已查看升级提示及独立历史/个人入口。个人本地文本经系统选择器导入草稿，落盘 82 字节与源文件一致；选中个人后重启仍进入个人，期间企业 Snapshot 下载增量为零。该设备未配置个人模型，个人模型/MCP 实际调用证据来自上表已配置的另一专用设备。

隔离 Core 的当前 APK 同 Session 完成 v4 generation 24 → v5 generation 25 → 重新发布历史 v4 generation 26 → 重开，均可执行；两节点历史 digest 不变，原始 v4 release/hash/generation 未改写。

v4 验证使用专用模拟器、独立数据库副本和重建 Debug APK，不声称验证了当前生产签名包或 OEM 真机。历史发布 bytes/hash 保留，重新发布使用新 generation；没有修改旧 v4 内容来伪造样例。升级顺序和输入方式见 Android `tools/compatibility/README.md`。

测试过程中发生过代理连接中断、测试地址未放行、lint 分析器崩溃及新模拟器 System UI 无响应；相应失败未计为通过。后续最终构建未禁用 lint。工作区 MCP 首次相对路径不存在明确返回错误，随后定位并以绝对路径读取成功。

§11.13 的取消、迟到 Session、撤权/过期、已知坏内容及网络错误由现有 owner 的 JVM/设备行为测试覆盖；实际界面和 HTTP 注入覆盖上表组合，不声称每一种组合均已人工操作。完整个人功能的隔离由统一 Realm/配置/执行路径及行为回归共同约束，个人聊天、MCP、设置与本地文件另有实操。真实麦克风、未准备的 PRoot 镜像和 OEM/生产签名安装不属于本次已执行证据，显式跳过不计通过。

验收材料已归档到本机工作区外 `%USERPROFILE%\Documents\MeasixValidation\2026-10-02-v5`：`core/v5-protocol/` 保存未来版本、退出、回退、个人文件和真实工作区证据；`core/starter-e2e/` 保存当前候选的真实 Core Starter 首发及供应商请求证据；`android/v5-protocol/` 保存覆盖安装、构建、设备运行和失败日志。`binaries/` 保留实际测试的 v4/v5 Debug 与测试 APK，并逐个核对原 SHA-256。264 个归档文件与原件哈希一致；公开结果 JSON 不包含接入凭据。

实现候选已本地提交：Architecture `07fdf4e`、Core `34decd0`、Portal `6b96d10`、Android `3d47418de`；本节后续提交只补验收记录。Core 干净候选的 gofmt、go vet、生成漂移门禁通过，跨仓库合同检查与 Android wire 检查通过；修改文档的 34 个本地链接有效。上述实现与协议专项验证完成，设备整条命令的环境失败和未覆盖场景保留为明确边界，不声称生产发布或所有环境均通过。

收尾采用可恢复归档：自动审批拒绝永久删除，未绕过该限制。项目内本次测试材料移出到上述归档，运行副本、旧源码构建和已停止的专用 AVD 归集到 `%TEMP%\measix-v5-retained-temporary-20261002`，保留原件而非永久删除；运行副本不作为公开验收材料。临时代理、个人测试服务和隔离 v4 Core 已停止；仅删除明确属于本次验证的两个远程目录，其他根目录条目保全。现有 `device:real`、Agent Space 及其原数据保留，活动服务日志仍留在原路径。

## 远程工作区合同扩展

运行控制显式 v2 和 MCP workspaceTarget、独立 WorkspaceProjection v1，以及 Admin/Client 文件接口已加入相应 OpenAPI。新目标不填造假 upstreamId；旧字段缺省序列化和历史 Snapshot 保持原语义。共享正反例在 `api/fixtures/workspace/`，执行测试为 `workspace_contract_test.go`。

详见 [远程工作区实现参考](remote-workspace-implementation.md) 与 [当前联调记录](remote-workspace-verification.md)。

未发布的工作区合同直接同步修订：WorkspaceProjection v1 必填 `serviceState`，全部文件请求必填 `agentSpaceId`，文件响应明确条件、Range 与 Problem 错误语义。Snapshot 仍为 v5，不增加工作区兼容探测或回退。两份 Client 投影样例随 Android 对接包导出，并由 Admin/Client 合同共同校验。
