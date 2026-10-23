# DAV 验收依赖

这些目录只用于 [WebDAV 专项验证](../../docs/testing.md#webdav实际旧库与大文件验证)，不作为产品组件部署。

- `workspace-dav-server`：独立 Go module，文件协议由固定 `golang.org/x/net/webdav v0.61.0` 实现；管理响应是确定性替身，不证明真实 Agent Space 的生命周期或镜像资格。
- `dav-client`：在该目录运行 `npm ci`，然后 `node verify.mjs <environment.private.json>`。固定 `webdav@5.11.0`，真实标准库通过平台 Bearer 进行文件操作；只创建、删除自己的 UUID 目录。

环境和凭据文件来自 `scripts/webdav-acceptance.mjs --keep` 的隔离输出，不提交。端到端入口及真实服务配置见测试文档。依赖锁文件需保留；`node_modules` 不入库。
