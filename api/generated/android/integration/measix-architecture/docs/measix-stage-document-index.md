# MEASIX 阶段文档索引

> 本索引只定义“当前阶段需要读哪些文档”，不记录实现完成状态。
>
> - `measix-architecture/*` 始终以 `measix-architecture@main` 为正式架构权威。
> - `measix-platform-core/*` / `rikkahub_mcp/*` 是实现仓库文档引用。开发阶段读取 active implementation candidate；Freeze/RC 读取 manifest 固定的 exact commit。
> - Testing Spec 是 checkpoint 完成标准之一；历史 Green 不能替代当前 authority 下的 executable evidence。
> - 为避免重复，本索引使用 **Base Reading Set + checkpoint delta**，不在每个 checkpoint 重复整套共同文档。

## 1. 平台总体 Base Reading Set

- `measix-architecture/docs/00-platform/measix-agent-platform-roadmap.md`
- `measix-architecture/docs/00-platform/measix-platform-terminology-and-identifier-contract.md`
- `measix-architecture/docs/00-platform/measix-enterprise-experience-lifecycle-architecture.md`
- `measix-architecture/docs/10-runtime-foundation/measix-runtime-foundation-architecture.md`

## 2. S0 Base Reading Set

在平台总体 Base 上增加：

- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-foundation-contract-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-implementation-decision.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-hub.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-hub-testing-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-runtime-relay.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-runtime-relay-testing-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-upstream-adapter-contract.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-upstream-adapter-qualification-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-system-testing-spec.md`
- `measix-platform-core/ARCHITECTURE.md`
- `measix-platform-core/docs/api-contracts.md`
- `measix-platform-core/docs/testing.md`
- `measix-platform-core/docs/s0-execution-progress.md`

## 3. S0.1 — Managed Capability Delivery Base

在 S0 Base 上重点增加：

- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-capability-delivery-contract-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-capability-delivery-implementation-decision.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-capability-delivery-system-testing-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-admin-console-product-requirements.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-admin-console-testing-spec.md`
- `measix-platform-core/docs/admin-console-implementation.md`
- `measix-platform-core/docs/release.md`

### C0 — Contract Audit & Freeze Preparation

重点追加/重读：

- `measix-s0-capability-delivery-contract-spec.md`
- `measix-s0-control-protocol.md`
- `measix-platform-core/docs/api-contracts.md`
- `measix-platform-core/docs/testing.md`

### C1 — Upstream Operational Completion

重点追加/重读：

- `measix-s0-admin-console-product-requirements.md`
- `measix-s0-control-hub.md`
- `measix-s0-upstream-adapter-contract.md`
- `measix-s0-upstream-adapter-qualification-spec.md`
- `measix-s0-admin-console-testing-spec.md`

### C2 — Managed Resource Editor Completion

重点追加/重读：

- `measix-s0-admin-console-product-requirements.md`
- `measix-s0-capability-delivery-contract-spec.md`
- `measix-s0-control-protocol.md`
- `measix-s0-admin-console-testing-spec.md`

### C3 — Snapshot Projection & Preview

重点追加/重读：

- `measix-s0-capability-delivery-contract-spec.md`
- `measix-s0-control-protocol.md`
- `measix-s0-control-hub.md`
- `measix-s0-admin-console-product-requirements.md`
- `measix-s0-capability-delivery-system-testing-spec.md`

### C4 — Runtime Reference Profile Completion

重点追加/重读：

- `measix-s0-runtime-relay.md`
- `measix-s0-runtime-relay-testing-spec.md`
- `measix-s0-upstream-adapter-contract.md`
- `measix-s0-upstream-adapter-qualification-spec.md`
- `measix-s0-capability-delivery-system-testing-spec.md`

### C5 — Usage / Pricing / Observability Completion

重点追加/重读：

- `measix-s0-control-hub.md`
- `measix-s0-runtime-relay.md`
- `measix-s0-admin-console-product-requirements.md`
- `measix-s0-admin-console-testing-spec.md`
- `measix-s0-capability-delivery-system-testing-spec.md`

### C6 — System E2E

重点追加/重读：

- `measix-s0-capability-delivery-system-testing-spec.md`
- `measix-s0-control-hub-testing-spec.md`
- `measix-s0-runtime-relay-testing-spec.md`
- `measix-s0-admin-console-testing-spec.md`
- `measix-s0-upstream-adapter-qualification-spec.md`
- `measix-platform-core/docs/testing.md`
- `measix-platform-core/docs/release.md`
- `measix-platform-core/docs/s0-execution-progress.md`

### C7 — Client Contract Freeze Gate

重点追加/重读：

- `measix-s0-capability-delivery-contract-spec.md`
- `measix-s0-capability-delivery-system-testing-spec.md`
- `measix-platform-core/docs/api-contracts.md`
- `measix-platform-core/docs/release.md`
- `measix-platform-core/docs/s0-execution-progress.md`

## 4. S0.2 — Enterprise Realm & Experience Foundation

当前未发布平台只交付 Snapshot v4 / Bridge v3 / local-read v2；版本与唯一当前数据库语义见 Control Protocol §10.10.1–2。旧原型不作为兼容或 Freeze 前置。

在 S0 Base 上增加：

- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-enterprise-realm-experience-contract-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-enterprise-portal-product-requirements.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-enterprise-realm-experience-testing-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md`
- `measix-platform-core/docs/api-contracts.md`
- `measix-platform-core/docs/release.md`

实现时还必须读取 `rikkahub_mcp` 当前 implementation candidate 中与 Assistant、Memory、QuickMessage、MCP、Realm、Session、WebView Host 直接相关的实现文档/代码。Enterprise Portal implementation entry 已登记为同级 `measix-enterprise-portal`，读取其 `ARCHITECTURE.md`/`README.md`；候选 build identity 由该仓库 production build 生成，Freeze 必须固定其 exact commit/build，不能使用仓库名代替证据。

## 5. S0.3 — Enterprise Tool Gateway & Governed Tool Integration

在 S0 Base 与 S0.2 Freeze 上增加：

- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-enterprise-tool-gateway-contract-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-enterprise-tool-gateway.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-enterprise-tool-gateway-testing-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-admin-console-product-requirements.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-admin-console-testing-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-hub.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-runtime-relay.md`
- `measix-platform-core/ARCHITECTURE.md`
- `measix-platform-core/docs/api-contracts.md`
- `measix-platform-core/docs/admin-console-implementation.md`
- `measix-platform-core/docs/operations.md`
- `measix-platform-core/docs/development.md`
- `measix-platform-core/docs/testing.md`
- `measix-platform-core/docs/release.md`

实现时必须登记真实 deterministic downstream MCP、Test Client、Gateway daemon/build identity；S0.3 Freeze 不读取 Android device evidence 代替 Gateway server evidence。

## 6. S0.4 — Android Managed Runtime Integration

在 S0 Base 与 S0.3 Freeze 上增加：

- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-android-integration-contract-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-android-client-testing-spec.md`
- `measix-architecture/docs/10-runtime-foundation/s0/measix-s0-system-testing-spec.md`
- `measix-platform-core/docs/api-contracts.md`
- `measix-platform-core/docs/release.md`

实现时还必须读取 `rikkahub_mcp` 当前 implementation candidate 中与 Enterprise Binding、Managed State、runtime integration、testing 直接相关的实现文档/代码；具体文件名由 Android 仓库自身维护，本索引不复制其 source tree。

## 7. S0 Exit

使用 S0 Base，并重点重读：

- `measix-s0-foundation-contract-spec.md`
- `measix-s0-enterprise-realm-experience-contract-spec.md`
- `measix-s0-enterprise-portal-product-requirements.md`
- `measix-s0-enterprise-tool-gateway-contract-spec.md`
- `measix-s0-enterprise-tool-gateway-testing-spec.md`
- `measix-s0-system-testing-spec.md`
- `measix-s0-control-hub-testing-spec.md`
- `measix-s0-runtime-relay-testing-spec.md`
- `measix-s0-admin-console-testing-spec.md`
- `measix-s0-enterprise-realm-experience-testing-spec.md`
- `measix-s0-android-client-testing-spec.md`
- `measix-s0-upstream-adapter-qualification-spec.md`
- `measix-platform-core/docs/release.md`

## 8. 后续 Stage

### S1 Agent Space

- `measix-architecture/docs/00-platform/measix-agent-platform-roadmap.md`
- `measix-architecture/docs/10-runtime-foundation/measix-runtime-foundation-architecture.md`

### S2 Agent Runtime & Remote Delegation

- `measix-architecture/docs/00-platform/measix-agent-platform-roadmap.md`
- `measix-architecture/docs/10-runtime-foundation/measix-runtime-foundation-architecture.md`

### S3 Runtime Hook

- `measix-architecture/docs/00-platform/measix-agent-platform-roadmap.md`
- `measix-architecture/docs/10-runtime-foundation/measix-runtime-foundation-architecture.md`
