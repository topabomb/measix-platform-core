# S0 Admin Console Testing 规格

> 状态：S0 Component Testing Authority
> 版本：2026-08-31
> 产品/浏览器边界权威：`measix-s0-admin-console-product-requirements.md`  
> Wire 权威：`measix-s0-control-protocol.md`  
> S0.1 System Gate：`measix-s0-capability-delivery-system-testing-spec.md`
> S0.3 Gateway Gate：`measix-s0-enterprise-tool-gateway-testing-spec.md`
> 具体实现/测试组织：`topabomb/measix-platform-core/docs/admin-console-implementation.md`、`measix-platform-core/docs/testing.md`
> 文档职责：证明 Admin 产品任务、state semantics、安全边界和 clean-environment browser path；不固定 Vue 文件、component tree 或具体 runner。

## 1. 测试目标

Admin tests 不只验证“页面能渲染”，而要证明：

- 用户通过 typed UI 提交的 payload 与 Hub/OpenAPI authoritative contract 一致；
- browser 不创造第二份 active state / Snapshot / validation truth；
- Draft/Candidate、Active Runtime、Release 不混淆；
- Secret/internal topology 不泄漏；
- long-running command 可恢复、可诊断、不重复；
- S0.1 required Model/TTS/ASR/MCP/Policy 都能真实 author；
- production SPA 的 Browser T4.1 可以完成完整产品闭环。

## 2. 测试分层

| Layer | 目的 |
|---|---|
| Unit | Problem mapping、derived state、filter/idempotency helpers |
| Component/UI | form、editor、page state、navigation、accessibility/responsive |
| Contract | generated Admin OpenAPI + canonical fixtures |
| Static-host | production SPA + same-origin Hub boundary |
| Browser T4.1 | real browser + real Hub + real Relay + deterministic Adapter/Test Client |

Frontend test 不复制 Hub validator；它验证 UI 发对 typed request、正确显示 authoritative response/state。

## 3. 通用交互

必须覆盖：

- primary navigation / route restore；
- loading / empty / error / session expired；
- desktop + narrow responsive smoke；
- visible keyboard focus for primary workflow；
- collection pagination/filter/search；
- stable ID view/copy；
- destructive confirmation；
- unsaved-edit leave warning；
- dirty state local edit 后出现、authoritative save 后清除；
- Save/Validate/Test/Apply/Publish 语义不混淆；
- Discard/Cancel 恢复最近 server baseline；
- typed enum/capability/binding editor，不要求 raw constant/JSON；
- complex editor 不依赖多层 modal；
- long command 使用 persistent progress/result，不只 toast。

## 4. API / Session / browser security

必须验证：

- same-origin credentials / Admin session；
- mutation CSRF；
- 401 session clear/redirect；
- Problem 使用 stable status + code；
- no generic mutation auto-retry；
- command retry 使用同一 idempotency identity；
- unknown optional response compatibility；
- browser 不调用 `/internal/*` / Relay control；
- HttpOnly cookie 不复制到 application state；
- Secret input 提交后清除且不持久化；
- Usage/detail 不展示 prompt/body/credential；
- browser state 不保存 server-only route/upstream credential。

## 5. Draft / active / release semantics

必须覆盖：

- Draft baseline vs local edits；
- Save 只改变 Draft/Candidate；
- stale conflict 保留 local edits 并显式 recovery；
- candidate Upstream revision != active revision；
- Apply 只有 authoritative Activation terminal success 后显示 active；
- Publish 202 不显示 ACTIVE；
- Published Release immutable；
- refresh 恢复既有 activation，不创建第二 command。

## 6. Upstream / Secret

Editor workflow 必须覆盖：

```text
name/base URL
transport capabilities
auth + SecretRef
correlation mode
usage capability level
timeout defaults
candidate/active revision
Test
Apply
```

断言：

- human-readable status + candidate/active revision 可见；
- transport/auth/correlation/usage 使用 typed controls；
- auth mode 只显示对应字段；
- Secret metadata 不回显 plaintext；
- Test result 结构化显示 connection/auth/capability/error context；
- Test PASS 不自动 Apply/VERIFIED；
- candidate != active 持续可见；
- Apply progress 不只 toast；
- failed Apply 保留旧 active runtime 和 candidate edits。

## 7. Resources / Policy

分别验证 Models、TTS、ASR、MCP、Policy。

### Shared editor

每类 capability 至少证明：

- collection + Add + selected editor/detail；
- row 可见 Display Name、logical ID、Upstream/runtime、Enabled、validation/binding state；
- logical ID 是 secondary identity，不要求管理员手写；
- Upstream 使用 human-readable typed picker；
- missing binding 在 collection/editor 都是明确 error；
- runtimePath 有 label/validation；
- validation issue 可定位 resource/tab/field；
- unsupported/unverified profile 不作为 normal selectable option；
- narrow layout 保留核心 authoring actions。

### Model

- Provider picker；
- logical model ID 与 Upstream Model Key 不混淆；
- TEXT/IMAGE typed multi-select；
- TOOL/REASONING typed boolean；
- TEXT output/profile summary；
- Upstream + runtimePath + transport summary；
- missing Provider/Binding blocks Publish。

### TTS

- Display Name、Model Key、Voice、Upstream、Runtime Path、Enabled 完整；
- voice required 且为主字段；
- `OPENAI_AUDIO_SPEECH` profile；
- MP3 current baseline；
- binary transport summary；
- 没有 authoritative Test API 时不伪造 preview success。

### ASR

- Display Name、Model Key、optional Language、Upstream、Runtime Path、Enabled；
- `OPENAI_AUDIO_TRANSCRIPTIONS`；
- 明确 HTTP multipart transcription；
- 不出现 realtime/WebSocket/VAD/sample-rate future controls。

### MCP

- `MCP_STREAMABLE_HTTP`；
- Auth Ownership 仅 `ENTERPRISE_MANAGED|NONE`；
- ENTERPRISE_MANAGED 明确 server-side credential；
- NONE 不制造 Secret requirement；
- USER_MANAGED/custom-header DSL 不出现。

### Policy

- 四个 Allow Local policy 独立且语义明确；
- Default Model/TTS/ASR 使用对应 picker；
- picker 只允许存在且 enabled 的同 kind resource；
- invalid default 形成可导航 error；
- policy dirty/diff 独立可见。

## 8. Relationship View

必须证明管理员能理解：

```text
Resource → Upstream → candidate/active state → runtime path/transport
```

至少覆盖 kind filter、missing binding、disabled resource、unverified/degraded Upstream、candidate!=active、click-through 到 Resource Execution/Upstream detail。

不要求固定 graph library；table/list/graph 均可，只要产品语义成立。

## 9. Validate / Review / Snapshot Preview

- Errors block Publish；
- Warnings require explicit review；
- stable code/severity drives UI；
- Review 显示 Hub 基于最新不可变 Release 返回的 Added/Changed/Removed + Policy/runtime impact；保存 Draft 后差异不能归零，无 Release 时明确显示空基线；
- issue 可跳转到具体 resource/field；
- Snapshot Preview 来自 Hub canonical compiler；
- Preview 无 Release/generation/runtime side effect；
- Preview 不含 Upstream/base URL/Secret/runtimeRoute/Binding/Pricing；
- Preview 能展示完整 required client-facing fields。

## 10. Publish / Releases

必须覆盖：

- Publish command idempotency；
- authoritative activation phases/progress；
- refresh recovery same activation；
- failure/degraded 显示 stable error/recovery context；
- active generation 只在成功 finalize 后增长；
- release list/detail 提供 generation/hash/source revision/diff/history；
- republish 产生新 generation，不倒退。

## 11. Pricing / Usage / Overview / System

Pricing：resource/upstream scope、meter/unit/price/currency/effective time/revision、KNOWN/PARTIAL/UNKNOWN。

Usage：MODEL/TTS/ASR/MCP、Time/User/Resource/Kind/Upstream/Status/Completeness filters、semantic/cost completeness、request detail safety。

Overview/System：runtime health、active generation、desired/applied revision/hash、Relay ready、Upstream health、usage backlog、activation failure、semantic unknown/orphan。

所有 visualization 必须：状态不只靠颜色；unknown 不显示成 0；精确数据可 drill-down。

## 12. Browser T4.1

Browser E2E 必须使用：

```text
production Admin SPA
real browser
real Control Hub
real Runtime Relay
deterministic Adapter/Test Client system environment
public client/admin/runtime topology only
```

不得使用 `page.route()`/mock API、manual DB/JSON/internal endpoint 替代产品路径。

完整 CAP-C6 scenario 和 recovery/security assertions 由 `measix-s0-capability-delivery-system-testing-spec.md` 唯一维护；Admin spec 不复制其 scenario matrix。

## 12.1 S0.2 Experience / Enterprise Updates

复用 Realm/Experience Testing Spec 的 `ERX-C-001..003`、`ERX-UPD-001..004`，以实际浏览器证明 Assistant/Seed/Starter 的 typed authoring→Validate→Snapshot Preview→Publish，以及 Enterprise Update 的独立 Draft→Publish→Withdraw。测试断言引用闭合、Markdown 安全子集、明确的双发布状态与 Feed 更新不推进 managedGeneration；不以手工 API/DB 构造代替 authoring proof。

## 13. S0.3 Enterprise Tools

- `ADM-GTW-001` Enterprise Tools 是独立一级导航，Direct MCP remains under Resources；
- `ADM-GTW-002` Guidance 显示 platform invariant read-only segment + editable enterprise segment + highlighted compiler output；
- `ADM-GTW-003` final two-tool description/schema preview and surfaceHash match canonical Gateway surface；
- `ADM-GTW-004` Integration Test / Refresh / Save Candidate / Validate / Preview / Publish actions and states remain distinct；
- `ADM-GTW-005` Refresh creates drift Candidate only and does not change active generation/catalog；
- `ADM-GTW-006` Catalog shows source/agent metadata, aliases, input/output schema, hashes, risk and candidate/published state；
- `ADM-GTW-007` schema validation constraints are read-only and non-READ_ONLY cannot publish；
- `ADM-GTW-008` untrusted annotation/source text is explicit and Direct/Gateway duplicate source blocks Publish；
- `ADM-GTW-009` publish progress shows Gateway apply before Relay apply and recovers exact Activation after refresh；
- `ADM-GTW-010` status shows desired/applied gateway revision/hash, ready/last-seen/catalog/drift without credential/internal URL/toolRef/payload；
- `ADM-GTW-011` production Browser T4.3 uses real Hub/Gateway/Relay/downstream MCP/Test Client, no mocked/internal shortcut；
- `ADM-GTW-012` accessibility/responsive/error recovery applies to all Enterprise Tools workflows。
- `ADM-GTW-013` Guidance configures Gateway inclusion and exactly one client policy, previews REQUIRED vs default-on/user-controllable semantics, and never offers per-tool toggles for the Meta Tool pair；
- `ADM-GTW-014` Gateway Status distinguishes supervisor process-active from application ready and exposes only safe event correlation, not raw logs or forbidden payloads。

## 14. Component Exit

Admin component 可以进入 S0.1 System Gate，仅当：

1. typed authoring、state semantics、security、responsive/accessibility critical requirements 有 component evidence；
2. production static build boundary Green；
3. browser 不使用 internal shortcut；
4. no Secret/browser-persistence leak；
5. required Product Requirements 可由 real-browser T4.1 完整执行；
6. evidence 对应 exact candidate，而不是历史/移动 `latest`。

Admin component 可以进入 S0.3 Gateway Freeze，仅当上述 S0.1 baseline 继续 Green，`ADM-GTW-*` 全部 Green，production Browser T4.3 完成真实 Integration→Catalog→Guidance→Publish→discover/invoke/diagnostics 路径，并且 evidence 固定 exact architecture/core/admin/gateway/contract/catalog identities。
