# 真机联调真实供应商预设

这是本机开发用的可重复联调组合，不是生产安装包，也不会修改 `.data/access-preview` 或普通 `.data/hub.db`。它使用独立的 `.data/device-real/` 数据库、Relay 用量 spool、进程记录和日志；密钥仅从 Git 忽略的 `.secrets/supplier-keys.env` 读取并写入该数据库的 Secret Store。

## 启动

在 `measix-platform-core` 执行：

```powershell
npm run device:real
```

启动器会选择默认网关所用网卡的 IPv4，并输出手机可访问的局域网地址。网络切换或希望指定网卡时，在启动前明确设置；以下 `192.0.2.20` 仅为文档示例：

```powershell
$env:MEASIX_REAL_DEVICE_ORIGIN = 'http://192.0.2.20:9100'
npm run device:real
```

它会构建 Admin 与 Portal，初始化唯一当前数据库，启动一个本地 `device-demo` 组合进程，保存管理员密码到 `.secrets/device-real-admin-password.txt`，随后创建/应用上游并发布预设。进程直接在同一局域网 HTTP origin 提供 Hub、Portal、Admin 和 Relay 的 `/runtime/v1`；**本预设不启动或依赖 Caddy**。这样仍符合 Android 对 Discovery 相对同源 `clientApiBase`、`runtimeApiBase` 的要求。

已发布资源为：DeepSeek Flash、Qwen 3.8 Flash（本机实验）、百炼 `wan2.7-image` 文生图（本机实验）、MiMo 云端朗读、设备本地朗读、百炼 HTTP 录音转写（本机实验）、Firecrawl Streamable HTTP MCP、两个企业助手和三个常用入口。十项默认值都由预设显式配置：对话、快速、标题、建议和上下文压缩使用 DeepSeek，附件检查使用 Qwen，另配置 `wan2.7-image`、MiMo、百炼 ASR 和企业工作助手。文生图使用 `DASHSCOPE_MULTIMODAL_GENERATION` 原生同步协议，不借用 OpenAI Images 或 Chat Model。预设将四个 Upstream 的 Adapter 资格标为 `LEVEL_0`；这只表示尚无经过资格认证的外部语义用量来源。Relay 的生产协议观察器仍可按资源 `clientProtocol` 解析可证明的指标；Admin 只对可靠 meter 与已配置价格计算费用，未知或部分结果必须保持 UNKNOWN/PARTIAL。

在 Admin 的 **Users** 创建成员并生成接入资料，手机扫描或粘贴该资料即可开始真实 Android 联调。手机与电脑必须在同一可达网络；若 Windows 防火墙提示，请允许该本机开发程序在专用网络接收 9100 端口访问。

## 重置与停止

内部监听优先使用 `127.0.0.1:9101`（Hub）和 `127.0.0.1:9103`（Relay）。未设置覆盖值时，启动器先探测端口；被占用或无法绑定则由系统分配空闲 loopback 端口，并避免两个内部服务选中同一端口。若需固定端口，可在启动前显式指定；覆盖值不会自动改写，公共入口保持不变：

```powershell
$env:MEASIX_REAL_DEVICE_HUB_INTERNAL_LISTEN = '127.0.0.1:19101'
$env:MEASIX_REAL_DEVICE_RELAY_INTERNAL_LISTEN = '127.0.0.1:19103'
npm run device:real
```

2026-10-06 修复 Windows `Start-Process` 参数边界：工作区路径含空格时必须显式引用参数，且保留嵌入引号与末尾反斜线。原生子进程回归测试修复前复现路径截断，修复后通过；内部端口覆盖与默认端口被占用时自动选取可绑定端口也经历 Red/Green。预设同时按既有 Control Protocol §10.7.1 补齐 Firecrawl 的 `toolAccessMode=ALL, allowedTools=[]`，以及两个助手的 `mcpBindings`（显式 ALL 和空 toolNames），移除预设中的旧 v5 `mcpServerIds` 写入；缺少字段的回归测试先失败再通过。`npm run test:tooling` 最新运行 62 项通过。实际在 NekoBox 占用 9103 时，不设置内部端口环境变量运行 `npm run device:real`，启动器选择替代 loopback 端口；已有 generation 1 保持 already-active，ready、Discovery、Admin 均为 HTTP 200，验证后停止服务。本次只修复启动与预设对现有合同的符合性，不改变 wire/state/security 合同；不作为阶段验收或真实模型调用证据。

```powershell
npm run device:real:stop
npm run device:real:reset
```

`stop` 仅停止该预设自己记录的进程。`reset` 先停止该进程，再删除仅属于 `.data/device-real/` 的数据库、spool、日志和预设状态，随后建立新的真机联调环境；它不会触碰任何其他数据库或联调环境。

启动时也会先执行停止检查。记录中的 PID 已不存在时，脚本清理失效进程记录；仍存在时，记录路径和实际进程路径都必须匹配 `.data/device-real/bin/measix-device-demo.exe`。身份不符会显示 PID 和三项路径并拒绝停止；查询失败或停止后仍未退出时，保留进程记录并报错。临时验证实例应使用独立进程记录，不要覆盖标准预设的 `process.json`；不要通过重置数据库处理进程身份冲突。

若启动在初始化数据库时报 `non-current schema/checksum`，说明该目录下的库由当前初始化 SQL 的旧版本建立（当前结构与旧版本之间不做迁移）。这种情况执行 `npm run device:real:reset` 即可：它删除该库并以当前 SQL 重新初始化。同一情形出现在普通开发库 `.data/hub.db` 时，用 `npm run setup`。
