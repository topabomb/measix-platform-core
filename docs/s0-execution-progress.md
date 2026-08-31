# S0 Platform Core 当前实现状态

> 状态日期：2026-08-31
> Architecture authority：[阶段阅读清单](../../measix-architecture/docs/measix-stage-document-index.md)及 owning contracts
> 本轮审查前基线：architecture `d11ea643f2326db43c5a495a73490d09f2d966db`；core `agent/s0-platform-core@bb7a689e20b3325cd6ade4bae0d477acd3cbd183`
> 本轮是文档修正，不含运行代码/可执行 schema/历史 manifest 修改。

## 当前判断

Hub、Relay、Admin 和四份 OpenAPI/fixtures/持久化/测试工具已存在；当前 Snapshot compiler 输出 v2。S0.2 后端包含 Assistant/Starter、Enterprise Update 等部分实现，但缺完整 Admin 作者流程、Feed 原子 revision/时区/wire 对齐，以及本轮没有执行的真实 consumer/ERX 证据。不能声明 S0.2 Freeze。

S0.3 尚无 Gateway binary、Gateway Control OpenAPI、v3 surface/catalog、三进程真实 harness、production service units 或统一日志 collector。文档明确了默认开启且可强制的 Gateway 两工具和原生进程监管方案，但这些不等于已实现。Direct Managed MCP 仍是 Assistant-bound 路径。

本轮发现的现有基础合同缺口（会话刷新、Discovery、拒绝计量等）与运维/证据工具问题也须修复；不得以历史 S0.1 manifest 或局部测试通过覆盖它们。源码定位、风险分级与实施顺序见 [2026-08-31 alignment audit](architecture-alignment-audit.md)。

## 历史 evidence 的边界

仓库保留的 manifest 声明：

```text
architectureCommit = cc60f8f540d309f2b73228094c8b9cd1b0b0a60f
platformCoreCommit = a6075bc0afd78fa86d77e1a520f838c954c9adfa
snapshotSchemaVersion = 1
```

本轮未重放或完整验证其外部 artifact 链，故只作为保留的历史声明，不称“当前已验证有效 Freeze”。它既不覆盖当前 v2 compiler，也不覆盖 S0.3/S0.4。历史 JSON 不因继续开发而重写。

## 推荐执行次序

1. 基础合同与证据可靠性：会话/Discovery/拒绝计量、schema/migration/backup 事实、采集器失败传播与 source/build/contract/profile 校验。
2. S0.2：canonical ordering、typed Assistant/Seed/Starter 作者流程、Feed 事务/时区/HTTP 投影、Portal 协议及真实 consumer gate。
3. S0.3：OpenAPI/fixtures → Gateway/Hub/Relay 发布与执行 → Admin/Test Client → 原生 supervisor、有限 drain/cancel、日志收集/脱敏及三进程验收。
4. S0.4/final：在有效的前序 exact candidates 上补真实 Android/Portal 和最终 SYS 证据。

S0.3 可以继续设计，但不能跳过其依赖合同和前序验收输入。禁止以伪造 Integration/Upstream、长期双路径或虚假 service unit 占位制造完成状态。

## 本轮验证与未执行

- `go test -p 1 ./... -count=1` 退出码 0；之前无缓存并行运行有两个 test.exe 因 Windows 文件占用未能启动，原始失败记录见 audit。未包含 smoke/candidate build tags。
- Console typecheck 通过；Vitest 14 文件、66 测试通过。
- 未执行 Atlas/race 全 gate、真实浏览器、真实 Adapter、Android/Portal、production supervisor 或 Freeze/replay。
- 文档一致性和 Git diff 检查不构成阶段验收；后续状态更新须引用具体运行、范围、skip 原因与 exact candidate。

本文件是唯一 living implementation/stage status。audit 是 dated evidence，开发/运维/测试/release 文档维护具体方法与限制，不另立阶段完成声明。
