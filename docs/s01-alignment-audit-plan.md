# S0.1 实现对齐审计与临时执行单 — ✅ 已完成

> 文档性质：临时审计与执行计划，S0.1 Freeze 已完成，本文件中所有偏差已关闭。
> S0.1 状态权威在 `docs/s0-execution-progress.md`。本文件保留作为历史记录。
>
> 审计日期：2026-08-27  
> platform-core：`agent/s0-platform-core` @ `11e2b2efffac3ecb5690f08e3a03303d09d240eb`  
> 架构基线：本地 `measix-architecture` 当前 `main` @ `cc60f8f`，S0.1/S0.2 架构决议已提交。

## 1. 结论先行

`measix-platform-core` 已具备 S0.1 的主要实现骨架：Identity/Enrollment、Draft/Release/Snapshot、Runtime Control、
Relay admission/transport、Usage、Admin Console、deterministic Test Client/Adapter 和系统测试基础均存在。

但 S0.1 仍未完成，当前阻塞集中在 C6/C7 的“可证明闭环”，不是再增加 S0.2 Android 内容。当前新架构下，S0.1
仍冻结 Client Snapshot v1；Managed Assistant、Memory Seed、Starter、ClientRealm 和 Portal 属于 S0.2，不能提前混入
本仓库的 S0.1 修复。

当前可复现结果：

| 检查 | 结果 | 证据/含义 |
|---|---|---|
| `go test -p 1 ./... -count=1` | PASS | 后端全量单测/组件测试通过；并行首次执行的 Windows 临时二进制占用不作为产品失败 |
| `go vet ./...` | PASS | 静态检查通过 |
| Console typecheck + 66 tests + production build | PASS | 前端组件与生产 SPA 可构建 |
| Atlas migration replay/hash gate | NOT_EXECUTED | 本机找不到 `atlas` 可执行文件；不能以其他迁移测试替代该门禁 |
| candidate system scenarios | PASS | 当前 HEAD 的 deterministic C6/C7 前置场景通过 |
| resource baseline | GREEN | 当前 HEAD artifact 记录 Hub/Relay 资源指标完整且无 sanity failure |
| production browser C6 | FAIL | Topology security 通过；Golden Path 在 Publish 返回 `runtime_activation_failed` |
| real Adapter qualification | NOT_EXECUTED | `.artifacts/real-adapter-qualification.json` 明确未执行 |
| Freeze manifest | BLOCKED | 当前架构工作树 dirty、浏览器失败、real Adapter 未执行、既有 artifact 非当前 SHA |

因此：C0–C5 的确定性实现基础可以作为回归基线，C6 不能标 Green，C7 和 S0.1 Freeze 不能开始。

## 2. 已确认的偏差

### P0-1：Browser Golden Path 把 Upstream Apply 误判为成功

`console/e2e/golden-path.spec.ts` 点击 Apply 前没有接受 `window.confirm`；Playwright 默认会关闭该对话框，Apply
 mutation 实际不会执行。随后测试用 `hasText: /active/i` 检查状态，该表达式同时匹配 `INACTIVE`，形成假 Green。
本次真实运行的页面快照显示四条 binding 都是 `upstream_not_active`，最终 Publish 返回 `runtime_activation_failed`。

对齐方案：

1. 测试在点击 Apply 前显式接受确认框，确认 mutation 已发出并等待终态；
2. 用精确的 `ACTIVE`/active revision 等状态断言，禁止用可匹配 `INACTIVE` 的模糊正则；
3. Apply、Publish 两处都断言 active revision 等于 candidate revision，失败时保留响应与页面状态；
4. 保留一个回归测试，证明“未 Apply 不能通过 C6 Golden Path”。

### P0-2：Browser Usage 阶段与 Test Client 流量顺序自相矛盾

`golden-path.spec.ts` 在同一个浏览器测试中先要求 Usage 有四种 runtime 数据，之后才可能结束；但
`e2e-harness.mjs` 只启动 Browser，不在该测试结束前运行 Test Client 四能力流量。`replay-freeze.mjs` 也在整个 Browser
测试结束后才生成四能力流量，所以两条路径都无法满足“Browser 配置/发布 → Test Client → Browser Usage/System”的顺序。

对齐方案：建立一个唯一的 candidate orchestrator，顺序固定为：

```text
Browser Admin：login → user/enrollment → upstream test/apply → resources/policy → validate/review/publish
→ 同一 clean environment 的 Test Client：Model/TTS/ASR/MCP
→ 等待 Usage ingestion
→ Browser Admin：Usage/System/refresh/session/candidate-active 审查
→ cleanup
```

Browser 测试拆成 authoring/publish 与 usage/system 两个阶段；一次运行内使用短生命周期、自动删除的 enrollment handoff，
不能把 token、Secret 或完整请求体写进 artifact。Test Client 继续使用 `backend/test/system/client` 的 public topology，
不在前端复制一套 runtime client。

### P0-3：Clean replay 提前关闭 deterministic Adapter

`scripts/replay-freeze.mjs` 在 Browser 阶段结束后立即 `adapterServer.close()`，随后才向同一 Relay 发送四能力流量。
即使 Browser 阶段成功，后续 runtime 请求也会因 Adapter 已关闭而失败。

对齐方案：Adapter 的生命周期覆盖 Browser 配置、Test Client 流量和 Usage 等待；所有阶段放进 `try/finally`，只在最终
Usage/System 断言完成后关闭。`e2e-harness.mjs` 与 `replay-freeze.mjs` 共用同一个 orchestrator/生命周期函数，不再各自维护
不同的闭环顺序。

### P0-4：Freeze 静态证据存在硬编码 PASS

`Makefile` 的 `collect-artifacts` 直接写入 `static-contract.json` 的 `codegenDrift/gofmt/goVet: PASS`，没有执行对应命令；
`freeze-gate` 也没有把完整 static/contract/migration/generated gate 作为前置步骤。这样可能把未执行的证据编译进 manifest。

对齐方案：静态 artifact 必须由实际命令退出码和输出 hash 生成，至少覆盖：OpenAPI/contract、gofmt、go vet、migration replay/hash、
generated drift、Console typecheck/build。任何命令未执行、失败、artifact/meta 缺失或 SHA 不一致都只能是
`NOT_EXECUTED`/`FAIL`，不能写成 PASS；manifest 只消费 exact candidate SHA 的 artifact。

### P0-5：Real Adapter qualification 无法聚合不同 profile

架构允许 Model/TTS/ASR/MCP 使用不同 endpoint/Adapter；当前 `collect-adapter-qualification.mjs --profile` 每次都覆盖同一个
`.artifacts/real-adapter-qualification.json`，而 Freeze 又要求单个 artifact 的四个 profile 都是 `VERIFIED`。因此按 profile
分次执行会丢失之前结果，只能错误地要求一个 endpoint 一次覆盖全部 profile。

对齐方案：每个 profile 生成独立、不可变、带 `adapterName/version + upstreamId/configRevision + profile` 的报告；另生成一个
只包含报告 hash/path 和覆盖状态的 aggregate index。不同 endpoint 可以分别 qualified；Freeze 检查 required profile coverage、
每份报告的 provenance 和对应 known deviations，不接受人工把状态改成 VERIFIED。

### P1-1：Browser 失败时报告进程与临时环境收尾不可靠

Playwright HTML reporter 在失败时启动本地 report server，非交互候选命令会停在报告服务；`e2e-harness.mjs` 的 cleanup 调用
`cleanupEnvironment(..., '')`，没有传入 `env.envRoot`，导致失败/成功后都可能遗留 clean-environment 临时目录。

对齐方案：candidate reporter 使用 `open: 'never'`；harness 用 `try/finally` 保存并传递真实 envRoot，确保 Worker、Hub、Relay、
Adapter、SPA proxy 和临时目录都在终态收敛。Windows 下要验证进程树无 orphan，失败 artifact 只保留安全诊断。

### P1-2：scenario definitions 混入未被架构授权的 CAP ID

`scenario-definitions.json` 中的 `CAP-SEC-001..022`、`CAP-C6-006` 以及若干 `RLY-*` 标识在当前架构 S0.1/S0 基础文档中没有
对应定义；本地治理规则明确禁止实现仓库自行发明跨组件 `CAP-*` 语义。

对齐方案：先按架构 Testing Spec 的章节/既有 ID 重新映射；若确实需要稳定 scenario ID，先回架构仓库登记，再同步 OpenAPI/
fixtures/tests/manifest。未获架构授权的检查可保留为 implementation regression name，但不能作为 S0.1 required manifest ID。

### P1-3：当前证据与进展描述不同步

既有 `.artifacts` 多数来自 `ad4f1bcc…` 或更早 SHA；本次 Browser 失败 artifact 来自当前 HEAD 且 meta 未完成正常收尾。
进展页仍保留“Phase 7/core 修复已完成”的历史叙述，容易把修复代码存在误读为 C6 已证明。

对齐方案：进展页只保留当前结论和可追溯证据；本文件中的细节只用于本轮修复，候选 SHA 变化后必须重新收集 artifact。没有
当前 SHA 的 exact evidence，不得写 Green。

### P2-1：Freeze 脚本仍硬编码旧架构提交的 warning

`freeze-manifest.mjs` 将 `dbb56952…` 写成 expected warning。S0.1 语义冻结后架构提交应由 candidate input/manifest pin 决定，不能
依赖旧提交常量。当前脚本已拒绝 dirty architecture，但下一次架构决议提交后需要同步 candidate identity，而不是仅接受 warning。

## 3. 对齐执行计划

按依赖顺序执行，不并行制造多个“几乎完成”的证据集：

1. **锁定架构输入**：提交并标识 S0.1/S0.2 架构工作树；更新 progress 的 architecture commit；确认 S0.1 v1 wire 未被
   S0.2 内容改写；两仓库 clean 后才进入 candidate。
2. **修复 Browser 真实前置**：补 Apply confirm/精确终态断言；先让 Browser authoring/publish 阶段在 clean environment Green。
3. **重组 C6 闭环**：拆 Browser 阶段，接入同环境 Test Client 四能力和 bounded Usage wait，再运行 Browser Usage/System 阶段。
4. **统一 replay/cleanup**：复用唯一 orchestrator；Adapter 覆盖全链路；关闭 HTML auto-open；修复 finally、Worker、进程树和
   temp root 收尾，并增加失败路径的清理验证。
5. **修复证据编译**：移除 static PASS 常量；每个 gate 真实执行并写 provenance；清理/隔离旧 artifact；按架构授权重映射
   scenario definitions；修复 architecture identity 输入。
6. **完成外部资格**：为实际 release 声称的每个 required profile 提供真实 endpoint/Adapter 报告；按 profile 聚合，不在日志或
   artifact 中保存凭据。没有真实 endpoint/key 时保持 `NOT_EXECUTED`，不能用 deterministic Adapter 替代。
7. **候选验证与 Freeze**：在 exact clean SHA 运行 C0–C6、resource baseline、browser/product gate 和 real qualification；生成
   candidate manifest；从同一 manifest clean replay；只有所有 required scenarios、无 critical skip、C7-002 PASS 后才更新
   S0.1 状态并删除本临时文档。

## 4. 完成判据

本临时单关闭前必须同时具备：

- 架构仓库和 platform-core 候选均 clean，manifest pin 的 SHA 与所有 artifact/meta 一致；
- Browser Golden Path 的 Apply、Publish、refresh/recovery、Usage/System 全链路真实通过；
- 同一 clean environment 的 Test Client 四能力、Usage ingestion 和 generation/no-forward 场景真实通过；
- C0–C5 static/contract/component evidence 由当前候选重新生成，未执行项不再伪装 PASS；
- real Adapter qualification 覆盖实际 release 声称的 profile，包含 correlation/usage/findings；
- resource baseline、security/no-forward/no-secret-leak、backup/recovery 和 clean replay 均有 exact evidence；
- 生成的 Freeze manifest 可验证、可重放，之后才允许进入 S0.2 Android。

