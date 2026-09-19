# 公共接入与配置应用交付计划

范围：architecture、Core、Portal；Android 只读。用户要求 HTTP/HTTPS、IP/域名均可正式使用。当前唯一协议和初始化 SQL，不保留旧配置别名、迁移或回退。

- [x] 统一 `public-origin` 配置，替换 Portal 独立 origin；返回权威接入资料和管理端展示。
- [x] HTTP/IP 地址校验、Admin/Portal Cookie、Origin/CSRF、ws/wss 与共享样例同步。
- [x] 客户端原子应用后的 authenticated report；当前 Session 隔离、重复报告、过时报告拒绝、发布/hash 校验。
- [x] 当前 SQL/Ent、OpenAPI/生成类型、设备列表及发布后的设备状态。
- [x] 管理端公共地址、部署问题、发布影响与操作指引；Portal 同步恢复复验。
- [x] 清除旧地址配置、过时限制和对应无意义测试/文档；保留有效状态/发布/构建身份。
- [x] HTTP/IP 与 HTTPS 实际接入、管理员实际配置/发布/报告验证，完整包与 Android 交接。

状态、交付包路径和最新摘要统一记入 [当前状态](s0-execution-progress.md)。本清单对应本轮七项上游交付，不代表 S0 阶段 Freeze 或 Android 真机验收。

## 完成审计

| 要求 | 现行实现与证据入口 |
| --- | --- |
| 地址唯一来源 | `backend/internal/hub/config/config.go`、`identity/portal.go`；Enrollment/Portal 返回配置 origin，System 展示该值。HTTP/IP 页面实际生成、复制资料并接入成功；共享 Enrollment 正反例覆盖域名/IP/IPv6/端口及非法 origin |
| HTTP/HTTPS | `httpapi/public_transport_test.go` 使用真实连接和受信测试证书，验证认证、下载、报告、Portal 换票与 Cookie/CSRF；`relay/websocket_test.go` 验证 ws/wss。HTTP/IP 浏览器暴露的 UUID/复制问题已修复并回归 |
| 更新和恢复 | Control Protocol 与 Android Integration Contract 明确前台、进入企业、手动同步和交互前检查；`httpapi/client_integration_test.go` 及 Runtime generation barrier；Portal `App.test.ts` 覆盖同步失败保留已应用状态、超时后检查状态、迟到结果隔离，生产包浏览器验证同步/恢复 |
| 应用报告 | `identity/applied.go`、当前 Session schema/初始化 SQL、Admin/Client OpenAPI；同一环境两次页面发布观察未知→已应用→待更新→已应用，Hub 重启后报告仍在；自动回归覆盖不续期、重复、错误 hash、回退、撤销、新 Session 和过期 |
| 管理界面 | HTTP/IP 真实页面创建成员、生成材料、创建/测试/应用上游、编辑全部资源类别/助手/种子/入口/策略/默认项，再验证、审查、发布；System/Users/Releases 组件测试及重建后页面复验 |
| 唯一实现 | 旧 Portal origin 配置和命令别名已移除；只有一份当前初始化 SQL，无增量转换。旧错误生成目录无源码；清理过时 Android 状态/包记录与 wss-only 交接描述；源码搜索无历史兼容路径，框架 `legacy: false` 不是业务兼容 |
| 交接与构建 | 全量 Go/vet、Admin、Portal 单元与生产浏览器回归；生成代码/正反例与当前源同步。交接包逐文件摘要校验与接入说明本地链接检查通过（文件清单以 `api/generated/android/integration/manifest.json` 为准，不在本文固定数量）；Android 顺序、字段、报告和网络要求见 `android-platform-integration.md` |

验证范围：管理员人工浏览器操作使用普通 HTTP/IP；HTTPS 使用真实 TLS 协议测试，未绕过证书验证。上游资源执行使用确定性的合成服务，不表示真实厂商生成质量；Android 保持只读，其原子提交、自动同步和硬件执行由维护方完成设备验收。正式阶段门禁中要求的 HTTPS 浏览器人工证据及 clean-source/Freeze 仍按各自门禁收集，不由本轮协议测试冒充。
