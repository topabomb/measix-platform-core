# S0 Platform Core 开发计划与当前进展

> 架构权威：`topabomb/measix-architecture` 默认分支。  
> 当前架构基线：`c2f517b52d84b1b4de5e5bab46a7a15d936a4e32`（2026-08-19）。  
> 本轮范围：仅 `measix-platform-core`；不修改 `rikkahub_mcp` 与 `weero-agent-space`。

## 执行规则

实现只以当前 `measix-architecture` 默认分支为权威，不使用历史副本或旧会话记忆替代正式文档。当前 S0 基线采用 Control Hub / Runtime Relay、SQLite + Ent + Atlas，以及完整 desired-state 的 Relay 单次 apply：`PUT /internal/v1/control/state`；不实现已经废弃的 prepare/barrier/commit 协议。

行为开发默认执行 Red → Green → Refactor。当前环境无法直接安装架构锁定的 Go/Node/Atlas 版本时，以 GitHub Actions 作为最终可执行证据；静态审查不能替代“测试已执行并通过”。

面向开发者、评审者和运维人员的说明文档优先使用中文；只有明确给 AI/Agent 使用的执行契约（例如 `AGENTS.md`）以及代码/机器配置按工程需要使用英文。

## 阶段计划与状态

| 阶段 | 本仓库实施范围 | 状态 | 验收证据 |
|---|---|---|---|
| I0 工程与可执行契约 | 4 份 OpenAPI、canonical fixtures、Go/TS/Android wire 生成、`platformid`、Hub/Relay health、SQLite/Ent/Atlas replay、Quasar shell、T0/T1/T2 与 `ci-gate` | 进行中 | 已建立 Red 测试分支与最小 CI 引导 |
| I1 Identity & Enrollment | Hub 身份/Enrollment/Session/Admin auth、Admin User/Enrollment/Device UI、Relay security-state 支持 | 待执行 | |
| I2 Draft & Snapshot | Upstream/Secret、typed Draft aggregate、validation、deterministic Snapshot/ETag、Admin Resource/Upstream workflow | 待执行 | |
| I3 Relay & Publish | full-state atomic apply/status、JWT/admission/route/proxy、Activation/reconcile/republish | 待执行 | |
| I4 Relay transport completion | Model/SSE、TTS binary、ASR multipart、MCP Streamable HTTP、cancel/limits；Android 实现不在本轮修改范围 | 待执行 | |
| I5 Metering/Admin/Hardening | Relay SQLite spool、Hub ingest/dedupe/pricing、完整 Admin status/usage、migration/backup/restart/security/load harness | 待执行 | |

## 本轮边界与 S0 RC 关系

本轮会实现所有不依赖 Android 源码修改或 Agent Space 修改的 `measix-platform-core` 需求，包括服务端、Admin Console、OpenAPI/fixture/codegen、migration、qualification/system harness 和 CI。

以下内容属于跨仓库或外部环境输入，本轮不修改、也不会伪报为已通过：

- `rikkahub_mcp` Android 源码与 instrumentation；
- `weero-agent-space`；
- 真实 Android Emulator E2E；
- 需要真实外部凭据的生产 Adapter qualification；
- 最终跨仓库 T4 中固定的 Android commit。

本仓库会保留对应接口、fixture、测试入口和 manifest 字段，使后续可以在固定外部 commit/credential 后执行完整 S0 RC Gate。

## 当前进展记录

- 2026-08-19：重新读取 `measix-architecture` 当前 `main`，废弃旧的 Control Server / Runtime Gateway / barrier-saga 假设。
- 2026-08-19：确认正式技术栈为 Go 1.26.x、SQLite + Ent + Atlas、Vue 3 + Quasar、OpenAPI 3.0.3。
- 2026-08-19：建立 `agent/s0-platform-core` 实施分支与 Draft PR #1。
- 2026-08-19：建立 `platformid` 首个失败测试作为 TDD Red 起点。
- 2026-08-19：在 `main` 加入最小 `ci-gate` 引导 workflow，使 PR 分支后续 Red/Green 有可执行证据。
