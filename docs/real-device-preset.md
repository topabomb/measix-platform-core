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

已发布资源为：DeepSeek Flash、Qwen 3.8 Flash（本机实验）、百炼 `wan2.7-image` 文生图（本机实验）、MiMo 云端朗读、设备本地朗读、百炼 HTTP 录音转写（本机实验）、Firecrawl Streamable HTTP MCP、两个企业助手和三个常用入口。十项默认值都由预设显式配置：对话、快速、标题、建议和上下文压缩使用 DeepSeek，附件检查使用 Qwen，另配置 `wan2.7-image`、MiMo、百炼 ASR 和企业工作助手。文生图使用 `DASHSCOPE_MULTIMODAL_GENERATION` 原生同步协议，不借用 OpenAI Images 或 Chat Model。所有上游目前为 `LEVEL_0`：管理台可验证转发次数、状态和字节数，但不能把供应商 token 或费用显示为已知。

在 Admin 的 **Users** 创建成员并生成接入资料，手机扫描或粘贴该资料即可开始真实 Android 联调。手机与电脑必须在同一可达网络；若 Windows 防火墙提示，请允许该本机开发程序在专用网络接收 9100 端口访问。

## 重置与停止

```powershell
npm run device:real:stop
npm run device:real:reset
```

`stop` 仅停止该预设自己记录的进程。`reset` 先停止该进程，再删除仅属于 `.data/device-real/` 的数据库、spool、日志和预设状态，随后建立新的真机联调环境；它不会触碰任何其他数据库或联调环境。

若启动在初始化数据库时报 `non-current schema/checksum`，说明该目录下的库由当前初始化 SQL 的旧版本建立（当前结构与旧版本之间不做迁移）。这种情况执行 `npm run device:real:reset` 即可：它删除该库并以当前 SQL 重新初始化。同一情形出现在普通开发库 `.data/hub.db` 时，用 `npm run setup`。
