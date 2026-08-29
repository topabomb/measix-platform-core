# S0 Platform Core 当前实现状态

> Architecture authority：`topabomb/measix-architecture@main` 及阶段阅读清单
> Implementation branch：`agent/s0-platform-core`  
> 本次代码审计基线（docs 修订前）：`b344d394bdd25396c5eeb6cd219a1363154aad3d`
> 状态日期：2026-08-30

## 当前判断

仓库保留了一份有效的**历史 S0.1 Freeze evidence**，但它只证明 manifest 固定的 architecture/core candidate，不证明当前分支 HEAD 或后来扩展后的 architecture：

```text
architectureCommit = cc60f8f540d309f2b73228094c8b9cd1b0b0a60f
platformCoreCommit = a6075bc0afd78fa86d77e1a520f838c954c9adfa
snapshotSchemaVersion = 1
```

当前实现分支已经在该 candidate 之后增加 S0.2 Realm/Experience/Enterprise Update 等代码与合同修订。除非存在与当前 architecture baseline、当前 exact candidate 对应的 T4.2/freeze evidence，不得把这些实现存在报告为“S0.2 Freeze Complete”。

S0.3 尚未开始形成可执行闭环：当前源代码只有 `control-hub`、`runtime-relay` 两个生产 Go entrypoint 和 Admin SPA；不存在 `enterprise-tool-gateway` binary、Gateway Control OpenAPI、Gateway fixture/generated types、三进程 harness、production service units 或统一生产日志实现。因此不得声称 S0.3 Gateway、生产进程监管或日志 Gate 已完成。

## 已实现与历史证据

- `backend/cmd/control-hub`、`backend/cmd/runtime-relay` 和现有 Admin SPA/Hub/Relay contract、migration、test infrastructure 存在；
- S0.1 v1 manifest/clean replay/Adapter qualification 是 pinned candidate 的不可变历史证据，可作为后续 regression baseline；
- 当前分支含 S0.2 Managed Assistant/Starter/Enterprise Update 等实现和测试；其完整 Gate 状态必须由当前 candidate evidence 单独判定；
- Hub/Relay 当前使用 Go `slog.JSONHandler` 输出 JSON，但缺少统一 `service`/`buildVersion`/`event`/correlation 初始化和 production collector/retention package。

## 当前主要缺口

### S0.2 状态复核

- 对当前 exact core/architecture candidate 执行并保存 architecture 要求的 T0–T4.2 evidence；
- 明确 Snapshot v2/Realm/Portal/Enterprise Update 的冻结组成和下游 Android/Portal build identities；
- 在完成前只报告已实现范围与具体测试，不报告阶段 Freeze。

### S0.3 Enterprise Tool Gateway

- 先实现 `api/internal/gateway-control.openapi.yaml`、canonical fixtures/generated types/contract tests；
- 实现独立 Gateway binary/failure domain、Hub desired state、Gateway-first/Relay-second activation/reconcile；
- 实现固定 `discover_tools`/`invoke_tool` surface、Catalog/toolRef/downstream MCP/platform tool 路径；
- 实现 Snapshot v3 `clientEnablementPolicy`：Gateway 两工具原子且默认开启，REQUIRED 时客户端不可关闭；Direct Managed MCP 保持 Assistant-bound；
- 扩展 Admin、deterministic downstream MCP、Test Client 和三进程 T3/T4.3 harness；
- 提供宿主原生 production supervision、graceful lifecycle 和安全结构化日志收集/脱敏的 executable evidence。

## 证据与报告规则

- `docs/s0-freeze-manifest.json` 不因继续开发而重写；它是 exact S0.1 candidate 的历史产物；
- 当前分支 Green CI 只证明对应命令/范围，不自动继承 S0.1 Freeze、S0.2 Freeze、S0.3 Freeze 或 final S0 Exit；
- Architecture requirement 或 candidate SHA 变化后，受影响 Gate 必须重新执行；
- 本文件是当前实现/缺口的唯一 living status。具体命令、服务单元与日志操作事实落地后同步 `docs/development.md`、`docs/operations.md`、`docs/testing.md` 和 `docs/release.md`。
