# S0.1 Platform Core 进展

> Architecture authority：`topabomb/measix-architecture@cc60f8f`  
> Implementation branch：`agent/s0-platform-core`  
> 当前阶段：**S0.1 Managed Capability Delivery — ✅ Freeze Complete**  
> 阶段阅读清单：`topabomb/measix-architecture/docs/measix-stage-document-index.md`

## 当前判断

S0.1 Managed Capability Delivery 已完成 Freeze Gate。所有 C0–C7 checkpoint 全部 Green，CAP-C7-002 Clean Replay 通过。Client Control/Snapshot v1 已冻结。

`docs/s0-freeze-manifest.json` 为有效 S0.1 Freeze Manifest，包含 exact architecture/core/build/contract/adapter/scenario identities。可以进入 S0.2。

## C0–C7 当前状态

| Checkpoint | 状态 | 当前结论 |
|---|---|---|
| C0 Contract Audit & Freeze Preparation | ✅ Green | required profile OpenAPI/fixtures/generated types/backend validation 已闭合；Client contract v1 frozen |
| C1 Upstream Operational | ✅ Green | candidate edit、Secret replace、Test/Apply、candidate/active revision、409/recovery 已实现 |
| C2 Managed Resource Editor | ✅ Green | Provider/Model/TTS/ASR/MCP/Policy typed authoring、binding、validation navigation、relationship view 已实现 |
| C3 Snapshot Projection & Preview | ✅ Green | canonical projection、Review、Client Snapshot Preview、Publish progress 已实现 |
| C4 Runtime Reference Profile | ✅ Green | deterministic T2/T3 已证明 Chat/SSE、TTS binary、ASR multipart、MCP 与关键 Relay admission/transport；real Adapter qualification VERIFIED |
| C5 Usage / Pricing / Observability | ✅ Green | filters、request detail、pricing、summary、Overview/System 产品闭环已验证 |
| C6 Browser + Hub + Relay Product/System E2E | ✅ Green | Browser Golden Path (authoring/publish + usage/system)、Test Client 四能力、Usage ingestion、topology security 全链路在 clean replay 中通过 |
| C7 Client Contract Freeze Gate | ✅ Green | CAP-C7-001 Freeze Manifest 生成；CAP-C7-002 Clean Replay PASS。Snapshot schemaVersion=1 frozen |

## 当前有效证据

- `docs/s0-freeze-manifest.json` — 有效 S0.1 Freeze Manifest，包含 exact SHA 和所有 scenario 结果。
- `.artifacts/replay-artifact.json` — CAP-C7-002 Clean Replay artifact，记录了 fresh environment 全链路验证结果。
- `scripts/freeze-manifest.mjs` 从真实 artifact 编译结果而非硬编码；artifact 缺失或 SHA 不匹配时 hard fail。
- `scripts/replay-freeze.mjs` 执行真实 clean-environment 重放：fresh Hub/Relay/Adapter + Playwright Browser Golden Path + four-capability traffic + Usage + topology security。
- `scripts/e2e-harness.mjs` 提供同链路的 E2E harness（使用 Worker 线程避免 execSync 阻塞 HTTP 事件循环）。
- `scripts/collect-adapter-qualification.mjs` 已完成 real Adapter qualification（VERIFIED）。
- `scripts/collect-baseline.mjs` / `baseline_test.go` 采集 architecture §17 指标，baseline GREEN。
- `console/e2e/golden-path-authoring.spec.ts` + `console/e2e/golden-path-usage.spec.ts` 拆分浏览器 E2E 阶段。

## Freeze 状态

**S0.1 Freeze 已完成。** `docs/s0-freeze-manifest.json` 是有效冻结清单：

- `platformCoreCommit`: exact candidate SHA
- `architectureCommit`: `cc60f8f`
- `snapshotSchemaVersion`: 1 (frozen)
- `realAdapterQualificationStatus`: VERIFIED
- `resourceBaselineStatus`: GREEN
- `CAP-C7-001`: PASS
- `CAP-C7-002`: PASS
- 所有 required scenarios: PASS

后续变化（如 S0.2 新增 schemaVersion=2）必须作为向后兼容 profile extension，不破坏 frozen Snapshot v1 语义。

## 状态维护规则

本文件只维护**当前实现状态、可用证据和剩余阻塞项**，不复制 architecture requirement 细节。

Architecture 变化后必须重新审查受影响 checkpoint；历史测试结果不能自动继承为新的 Green。Checkpoint completion 必须能够指向当前 architecture baseline 与 exact implementation candidate 对应的 executable evidence。
