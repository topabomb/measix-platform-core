# 发布与 Core / Android 版本对应

本文维护具体版本、打包入口和 Android 接入要求。跨端语义见[版本关联规则](../../measix-architecture/docs/00-platform/measix-versioning-and-compatibility-plan.md)；阶段门禁仍按原 Testing Spec；历史包和结果见[状态页](s0-execution-progress.md)。

## 当前版本对应

| Android versionName / versionCode | Core 版本 | 关系与状态 |
| --- | --- | --- |
| 0.0.20 / 20（尚未正式发布） | 0.2.0-preview.23（当前目标，尚未发布） | Android 本次对接目标；实际兼容待验证 |
| 历史固定源码 / Debug APK | 0.2.0-preview.22 | 历史固定组合见状态页，不能写成正式 Android 20 已发布 |

对外只需说：**Android 0.0.20（20）已验证支持 Core〈实际通过测试的版本列表〉**。当前尚未完成真实验证，不能把目标行改称已支持。以后新 Core 与原 Android APK 测试通过，就在新 Core 的发行记录增加这对版本；Android 不必随之改号、改材料或重新发包。未测试表示未知，不等于不兼容。

`coreBaselineVersion` 表示 Android 最初对接哪版 Core；实际支持哪些 Core 看发行记录。`platformContractVersion`、支持集合及材料 hash 是内部追溯字段，不让使用者通过这些数字推算兼容。当前 Core 基准为 preview.23、主合同为 2、支持集合为 `[2]`，唯一源是 `api/protocol-baseline.json`，Client/Portal manifest 自动生成。Core 包版本来自 builder 参数及 binary buildVersion；npm package.version 只是工具版本。

Android 20 未正式发布，当前修订继续使用 `0.0.20 / 20`，由 commit/APK hash 区分候选；正式发布后保留原包和原记录。Snapshot、Bridge、SQL migration 与业务 generation 沿用原有含义，不另设一套模块版本流程。

## Core 验证与打包

日常只做三步：

1. **构建一次固定 Core 包。** 元数据自动随包生成；打包不依赖 Android checkout、APK 或专用报告。
2. **用承诺支持的固定 APK 测试这个包。** 复用现有 CI/真机用例，首次关联覆盖身份、Snapshot、Runtime、Portal 实际边界；后续按变更影响及既有门禁执行，无需另建四套报告。Core 独立升级使用原 APK，不能改旧 decoder 或重编旧包冒充原版。
3. **把对应关系写入本次发行说明或已有 CI 发行记录。** 明确 Android `versionName/versionCode`、APK hash、Core 版本及包 hash、测试范围、结果/日志链接。只声明证据覆盖的组合。记录生成后固定；后补验证另留结果并引用原包，不改原包，也不为补记录重建一次。

```text
# 元数据检查；不证明 APK 兼容
node scripts/verify-preview-contract.mjs --mode core

# 日常打包；--candidate 仍可使用，含义相同
node scripts/build-preview-release.mjs <version>
```

builder 仍要求固定且干净的 Core、Portal、architecture，检查生成漂移，构建 Admin/Portal 和 ARM64 Hub/Relay；占用的输出目录或归档在构建前拒绝，不覆盖历史包。输出为 `.artifacts/releases/measix-core-<version>-linux-arm64-candidate.tar.gz`，包含 `release.json`、`COMPATIBILITY.md`、`SHA256SUMS`。

包内 `UNVERIFIED_CANDIDATE` 和空 `verifiedAndroid` 表示**打包时未附机器校验的 Android 结果**，不表示已知不兼容；后续对应结论以引用该包摘要的发行记录为准。兼容记录与包分开保存，无须将包“转正”或重打；对外发布仍须满足原有发布/阶段要求。正式身份不可复用，未发布修订也须保全不同 commit/hash 的产物。

内部未正式发布的 Preview 同版本更新仍固定新的源码 commit 和包 hash：在新的空输出位置构建，保留原包；Spark 可使用 `releases/<version>-<commit>` 区分部署目录，包内版本和平台基线不随目录后缀变化。切换与备份按[部署手册](s02-preview-deployment.md#11-升级)执行，现场记录必须区分同版本的具体构建。

最小发行记录示意（尖括号必须换成真实值）：

```text
Android：0.0.20（versionCode 20，candidate/released 按实际）
APK：<文件/ABI/变体及 SHA-256>
Core：0.2.0-preview.23，<包 SHA-256 或 release.json 的 buildHash>
结果：待验证 / 已验证 / 不兼容；<实际测试范围与 CI 报告/日志链接>
```

这个对应条目直接写在现有发行记录里，不新增专用文件或另一份手工台账。不同 ABI/APK 分别绑定实际测试身份，不把一个 APK 的结果自动外推到全部变体。具体部署、备份和恢复见 [Spark runbook](s02-preview-deployment.md)。

## Android 端调整方案

**已在实施的字段和文件格式继续保留；本次只收窄强制流程。** Android 在自身仓库修改。保持现有网络、Room/Settings/Applied、备份及 Bridge 安全边界；普通构建不访问 Core/网络，不增加在线产品版本握手或版本关联专用数据库迁移。

### 1. 固定材料与构建身份：继续完成

- `app/src/test/resources/contracts/platform/` 导入 Core 的 `client-control.openapi.yaml`、`manifest.json`、`protocol-baseline.json`；按原流程同步 Snapshot reception cases/fixtures。`contracts/portal/` 导入完整 Portal manifest 和列出的材料，只更新本次采用的固定版本。
- `tools/generate-enterprise-wire.py` 核对 Client/Portal/基线身份、Client LF sourceHash 和 Portal artifact 摘要，再生成 `PlatformWire.kt`，保留 Core source SHA256 header；缺失或不一致应失败。
- `app/build.gradle.kts` 唯一维护 `versionName/versionCode`；从本地固定材料生成下列 BuildConfig，并检查一致性。导入/重新生成仍是单独开发步骤，普通 assemble 不强制运行 Python。

| BuildConfig | 来源 |
| --- | --- |
| `PLATFORM_CONTRACT_VERSION`（int） | 固定 manifest 的 platformContractVersion |
| `CORE_BASELINE_VERSION`（String） | 固定 manifest 的 coreBaselineVersion |
| `PLATFORM_CONTRACT_BASELINE_HASH`（String） | 基线 LF SHA-256，与两份 manifest 核对 |
| `SUPPORTED_PLATFORM_CONTRACT_VERSIONS`（int[]） | Android 自己的明确支持集合；当前 `[2]`，不能复制 Core 集合 |

支持集合为排序、去重的正整数且包含主合同；支持历史合同须有相应实现和消费验证，保留 Snapshot v4 不等于承诺整个平台历史合同。关于页展示基准 Core/合同属于可选诊断，不阻塞本次版本关联，不能从服务器动态改写 APK 的构建身份。

### 2. 最终 APK 身份与版本对应：继续完成

沿用 `.github/workflows/release.yml` 和现有发布记录。从最终 APK 核对 applicationId、versionName/versionCode、文件 SHA-256 和签名；可使用 SDK `apkanalyzer`、`apksigner`。正式 tag 与版本匹配、versionCode 满足安装升级要求，不覆盖已发布 APK；Debug 标为候选。已有 version.json 读取同一实际版本源。

记录采用的 Core 基准及固定材料来源，保留原 APK/材料；执行既有真实消费测试，在发行记录写明支持的具体 Core 版本和结果链接。身份、版本/材料不一致须有失败测试；按受影响边界执行回归。只有测试通过的组合才能登记“已验证”，不能把工具测试 fixture 或历史 Green 当真机结果。

### 3. 可以延后，不作为本轮完成条件

专用 `android-release.json` producer、四类 `evidence.json` runner、Core 自动证据打包，以及新增关于页诊断展示都可延后；已实现则保留使用，无需删掉重做。本轮 Android 完成条件是前两步正确并验证目标组合，不要求为了满足 Core 自定义报告格式重构 CI。阶段完整 Gate 仍按原架构执行。

## 可选：已有自动证据校验

这一节保留已交给 Android 的接口，供已经接入的流水线继续使用。**它是可选自动化格式，不是日常打包或版本关联的唯一办法。** 不新增 schema、不扩展 runner；无需因本次收窄返工已完成的 producer。

```text
# 仅在联合导入时比较 Android checkout 与当前 Core 材料
node scripts/verify-preview-contract.mjs --mode joint

# 可选：核验原 APK/材料/报告，不需要 Android checkout
node scripts/verify-preview-contract.mjs --mode independent --version <version> --android-release <android-release.json> --evidence <evidence.json>

# 可选：把已有机器证据装入包；提供两项即启用，默认 independent
node scripts/build-preview-release.mjs <version> --android-release <android-release.json> --evidence <evidence.json>
```

显式 `--mode joint` 还比较当前 Android checkout 的材料及 commit，使用 `MEASIX_RELEASE_ANDROID_ROOT`（默认 `../../rikkahub_mcp`）。独立验证保留原 APK/原材料，不要求它等于新 Core 导出。只传 record 或 evidence 一项会失败，不静默降级。

可选 `android-release.json` 的既有格式不变：

| 字段 | 内容 |
| --- | --- |
| `formatVersion/product/status` | `1` / `MEASIX Android` / `candidate` 或 `released`；正式另有 `tag=v<versionName>` |
| `sourceCommit/sourceDirty` | 固定 Android commit / `false` |
| `applicationId/versionName/versionCode` | 最终 APK 身份 |
| `platformContractVersion/supportedPlatformContractVersions/coreBaselineVersion/baselineHash` | APK 固定构建身份，与原导出关联 |
| `snapshotSchemaVersions` | Android 明确支持的集合 |
| `contracts.directory` | 原材料目录，含 platform/portal 子目录 |
| `contracts.platformManifest/portalManifest/wire` | 各 `{path,sha256}`，指向原两份 manifest 和生成 Kotlin |
| `artifacts[]` | `{path,sha256,abi,variant,signingCertificateSha256}`；variant 为 debug/release |

路径相对记录目录且不能逃出；摘要为 `sha256:<64位小写hex>`。每个 APK 固定来源和签名。Core 校验文件及记录，不替 Android 解析 APK 或验证签名。

可选 `evidence.json` 保持 `formatVersion=1`、`suite=android-core-consumer`、`result=PASS`、`sourceDirty=false`；`core` 为 `{version,sourceCommit,architectureCommit,portalCommit,baselineHash,buildHash}`，取自实际测试的 Core 包；`android` 为 `{releaseHash,sourceCommit,apkSha256}`。`checks` 保持 identity/snapshot/runtime/portal 四项，各 `{id,command,exitCode,report:{path,sha256},log:{path,sha256}}`。该严格工具仍要求非空 JUnit、明确计数、无失败/错误/跳过及 exitCode=0；identity 项含真实 APK/签名/构建身份核验。只有选择本工具才需要适配此格式。

自动证据打包会重新构建并比对已测 `buildHash`，字节不同则拒绝。成功后输出无 `-candidate` 后缀的包，`VERIFIED_PREVIEW` 及 `verifiedAndroid` 仅证明所列固定组合，包内保存原记录和报告/日志副本；APK/材料另行保全。**日常流程不要求这次重建**，直接保存所测原包和已有测试记录即可。工具 PASS 也不代表正式发行或 S0 阶段完成。

## 配置、持久化与独立升级

配置格式启用遵循 [API 升级流程](api-contracts.md#合同变更与独立演进)：消费者先具备能力，发送方后启用。Core 可先部署并保持原 active Release；恢复记录实际 active release/generation/hash，目标必须在所选 APK 的支持范围内。

Hub SQL、Relay spool、部署配置/密钥及 Android Room/Settings/Applied/备份沿用现有 owner，不为总合同另建迁移链。Core `schemaMigrationIdentity` 固定原有 append-only SQL 历史；保全 durable drafts、immutable release bytes/hash，禁止清库/重建升级。旧 binary 不能读取升级数据时按软件+数据+配置+密钥一致恢复，不能只替换 binary。具体迁移和恢复见[数据库迁移](database-migrations.md)与 Spark runbook。

## S0.1 Candidate/Freeze

S0.1 为 pre-Android CAP gate；候选可在验证不完整时存在，accepted Freeze 必须满足完整 C6/C7。manifest 精确字段与场景由 architecture Spec 和 executable harness schema 拥有，Markdown 不维护第二份 schema。现有 CAP compiler pins Snapshot v5、资源证据和 CAP-C0-010 shared v4/v5/default/strict opening，不生成 S0.2 ERX 设备结果。

默认候选输出为 `.artifacts/s0-freeze-candidate.json`，独占写入；新候选使用新路径。CAP-C7-002=NOT_EXECUTED 的 draft 不是 Freeze。

在所冻结的干净、固定组合收集实际 evidence 和各自 `*.meta.json`：

```text
make collect-artifacts
make collect-candidate
make collect-static-contract
make collect-baseline
make collect-adapter-qualification ENDPOINT=... KEY=...
make s01-browser-candidate
```

producer 元数据绑定 source/architecture、tree cleanliness、exit 和 artifact hash。缺失、stale source、失败或 hash mismatch 拒绝；resource baseline 必须 GREEN，真实资格须在同一 run 验证全部四个 profile 和 required Adapter/upstream/config/transport/forwarded Usage。未知身份、未执行项、LEVEL_0/header echo 或拼接诊断都不是合格语义资格。

按顺序执行：

```text
node scripts/freeze-manifest.mjs --validate --candidate --manifest <candidate.json>
node scripts/replay-freeze.mjs --manifest <candidate.json>
node scripts/freeze-manifest.mjs --finalize --manifest <candidate.json> --output <new-final.json>
node scripts/freeze-manifest.mjs --validate --manifest <new-final.json>
```

replay 从固定 Core/architecture commits 建立独立 sibling checkouts，排除工作区修改、秘密和旧产物；恢复 locked dependencies、重新生成和构建 SPA，核对 source/contract/fixture/build pins，再执行 contract/vet/backend/smoke/system/candidate/console/browser。没有仅运行旧 binary 的捷径，生成或测试使 checkout 变脏也失败。

完整日志位于 `.artifacts/replay/`，与证据一同保留。`.artifacts/replay-artifact.json` 独占写入并绑定原 candidate bytes、重建事实和有序命令/日志。finalize 核验报告与日志 hash，写新 manifest，原候选不改。runner fixture tests 通过不是一次候选 replay 已执行。

## CI 与后续阶段晋级

PR 执行受影响测试和当前 ci-gate；完整浏览器、真实外部资格和 candidate replay 是固定 SHA 的晋级 gate。具体执行见[测试说明](testing.md)。正常 CI Green 不能替代 C6/C7。

| 阶段 | 新增的实际证据边界 |
| --- | --- |
| S0.2 | 固定 server/Android/Portal、Realm/Experience 与 ERX 场景；保留 v4/v5 数据与真实 consumer 证据 |
| S0.3 | Gateway binary/build、Gateway Control hash、Snapshot v6/catalog fixtures、真实三 daemon/MCP、生产浏览器、监督/graceful lifecycle/结构化日志脱敏 |
| S0.4 | 固定 S0.3 基线上的完整真实 Android profile/device |
| Final S0 | 合成有效 S0.1–S0.4 pins，执行适用 CAP/ERX/ETG/AND/SYS、备份恢复、spool/replay、资源/负载和最终系统 gate |

后续阶段存在未交付能力；不能用早期 manifest、页面截图或本地 preset 宣告已完成。最终 schema/场景仍以各架构 Testing Spec 为准。

## 证据保全与失败处理

每份结果能追到固定 commits、build 配置/摘要、协议/fixtures、实际命令/场景/时间及适用 browser/device/APK。不得包含生产 credentials、Secret、私人会话或原始正文。deterministic lane 与真实 external qualification 各自标明，不能互相替代。

失败保留原始日志和产物，最小回归复现后按 Red → Green 修复；修复产生新 source/build/contract 组合，再跑受影响检查与完整 required candidate gate。不能修改失败报告使之显示成功。首次正式 tag/release 命名落实前，commit 和 manifest/package hash 仍是可复核身份。
