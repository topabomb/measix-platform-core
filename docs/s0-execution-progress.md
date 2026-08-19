# S0 Platform Core 开发计划与当前进展

> 架构权威：`topabomb/measix-architecture` 默认分支。  
> 当前架构基线：`c2f517b52d84b1b4de5e5bab46a7a15d936a4e32`（2026-08-19）。  
> 本轮范围：仅 `measix-platform-core`；不修改 `rikkahub_mcp` 与 `weero-agent-space`。

## 执行规则

实现只以当前 `measix-architecture` 默认分支为权威。当前 S0 基线采用 Control Hub / Runtime Relay、SQLite + Ent + Atlas，以及 `PUT /internal/v1/control/state` 的完整 desired-state apply；不实现旧 prepare/barrier/commit 协议。

行为开发执行 Red → Green → Refactor。面向开发者、评审者和运维人员的说明文档优先使用中文；只有明确给 AI/Agent 使用的执行契约（例如 `AGENTS.md`）以及代码/机器配置按工程需要使用英文。

## 阶段计划与状态

| 阶段 | 本仓库实施范围 | 状态 | 验收证据 |
|---|---|---|---|
| I0 工程与可执行契约 | 4 OpenAPI、fixtures、codegen、platformid、Hub/Relay health、SQLite/Ent/Atlas、Quasar shell、T0/T1/T2/CI | 进行中 | Red：commit `28504cef...` / run `32210208203`；生成链 run `32211905037` 已通过 Go module、4×OpenAPI、Ent、TS、Atlas hash |
| I1 Identity & Enrollment | Hub 身份/Enrollment/Session/Admin auth、Admin User/Enrollment/Device | 待执行 | |
| I2 Draft & Snapshot | Upstream/Secret、typed Draft aggregate、validation、deterministic Snapshot/ETag、Admin workflow | 待执行 | |
| I3 Relay & Publish | full-state atomic apply/status、JWT/admission/route/proxy、Activation/reconcile/republish | 待执行 | |
| I4 Relay transport completion | Model/SSE、TTS binary、ASR multipart、MCP Streamable HTTP、cancel/limits；Android 源码不在本轮 | 待执行 | |
| I5 Metering/Admin/Hardening | Relay SQLite spool、Hub ingest/dedupe/pricing、Admin status/usage、migration/backup/restart/security/load harness | 待执行 | |

## 本轮边界与 S0 RC 关系

本轮实现所有不依赖 Android 源码修改或 Agent Space 修改的 platform-core 需求。以下跨仓库/外部输入不修改，也不会伪报通过：Android instrumentation/Emulator、`weero-agent-space`、真实外部 Adapter 凭据、最终固定 Android commit 的 T4。platform-core 保留对应接口、fixture、harness 和 manifest 字段。

## 当前进展记录

- 2026-08-19：重新读取架构仓库当前 `main`，废弃旧 Control Server / Runtime Gateway / barrier-saga 假设。
- 2026-08-19：确认技术栈为 Go 1.26.x、SQLite + Ent + Atlas、Vue 3 + Quasar、OpenAPI 3.0.3。
- 2026-08-19：建立实施分支和 Draft PR #1。
- 2026-08-19：完成真实 TDD Red：GitHub Actions 在 `platformid` 行为断言处失败，而非工具链/配置失败。
- 2026-08-19：I0 executable source 已落库；4 份 OpenAPI 与 canonical fixtures 可解析。
- 2026-08-19：GitHub Actions 已实际完成 `go mod tidy → 4×oapi-codegen → Ent → pnpm/openapi-typescript → Atlas hash`；Quasar 3 HTML 入口已修正。
- 2026-08-19：T0 contract、`platformid`、health、Hub app、generated wire 已进入 Green；当前继续收敛 Ent 生成后依赖与完整 T0/T1/T2 Gate。
