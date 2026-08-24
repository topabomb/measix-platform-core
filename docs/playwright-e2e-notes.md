# Playwright Browser E2E 调试经验

> Scope：`console/e2e/` + `scripts/e2e-harness.mjs` + `scripts/lib/harness.mjs` 中积累的 Playwright 实战经验。  
> Architecture authority：`measix-s0-capability-delivery-system-testing-spec.md` §13 CAP-C6-001 / CAP-C6-BROWSER-006。  
> 此文档为工程经验备忘，不替代 architecture 的行为/契约/安全语义。

## 1. 三个核心陷阱（按踩坑顺序）

### 1.1 `chromium_headless_shell` 在 Windows 上无法连接本地 HTTP server

**现象**：Playwright `page.goto('http://127.0.0.1:<port>/admin/')` 报 `net::ERR_ABORTED` 或 `net::ERR_CONNECTION_RESET`，但 Node.js HTTP server 日志显示**没有收到任何请求**——请求根本没到达服务端。

**根因**：Playwright 默认使用 `chromium_headless_shell`（一个精简的 headless 二进制），在 Windows 上存在 loopback 网络缺陷，无法正确发起 127.0.0.1 的 TCP 连接。Linux/macOS 上不复现。

**修复**：在 `playwright.config.ts` 中显式指定 `channel: 'chromium'`，使用完整 Chromium 二进制而非 headless shell：

```typescript
projects: [
  {
    name: 'chromium',
    use: {
      ...devices['Desktop Chrome'],
      // 使用完整 Chromium 二进制而非 headless shell
      // headless shell 在 Windows 上有 loopback 网络缺陷
      channel: 'chromium',
    },
  },
],
```

**验证方法**：用 `browser.version()` 打印浏览器版本——headless shell 版本字符串与完整 Chromium 不同，可用来确认切换是否生效。

### 1.2 `execSync` 阻塞 Node.js 事件循环导致 HTTP server 无法响应

**现象**：即使修复了 1.1，通过 `e2e-harness.mjs` 运行 Playwright 时仍然 `ERR_ABORTED`。但如果用独立的 `.mjs` 脚本直接 `chromium.launch()` + `page.goto()` 则能成功。

**根因**：`e2e-harness.mjs` 用 `execSync('npx playwright test', ...)` 运行 Playwright。`execSync` 是**同步阻塞**调用——它会冻结整个 Node.js 事件循环，包括 HTTP server 的 `request` 事件。Chromium 向 SPA proxy 发起请求时，Node.js server 根本无法 `accept` 连接，Chromium 超时后报 `ERR_ABORTED`。

**修复**：将 HTTP server（SPA proxy + deterministic adapter）移到 `worker_threads` 中运行，主线程只负责 `execSync` 调用 Playwright。Worker thread 有独立的事件循环，不受主线程 `execSync` 阻塞：

```javascript
import { Worker } from 'node:worker_threads'

// 在 worker 中运行 HTTP servers
const worker = new Worker(join(ROOT, 'scripts', '_server-worker.mjs'), {
  workerData: { spaPort, spaDir, adapterPort, hubPort },
})
await new Promise((resolve, reject) => {
  worker.on('message', (msg) => { if (msg.ready) resolve() })
  worker.on('error', reject)
})

// 主线程安全地使用 execSync
execSync('npx playwright test', { cwd: join(ROOT, 'console'), stdio: 'inherit', env: e2eEnv })
```

**关键文件**：`scripts/_server-worker.mjs` 包含 SPA proxy 和 adapter 的完整 HTTP server 实现，在 worker thread 中独立运行。

### 1.3 第三方安全软件拦截 loopback 连接

**现象**：1.1 和 1.2 都修复后仍然 `ERR_ABORTED`。Node.js server 没有收到请求。

**根因**：360 安全卫士等软件的 `ZhuDongFangYu.exe`（主动防御服务）会拦截 Chromium 发起的本地 TCP 连接，即使 "关闭" 主程序后该服务仍可能在后台运行。

**排查方法**：
1. 用 `netstat -ano | findstr <port>` 确认 Node.js server 确实在监听
2. 用 `curl http://127.0.0.1:<port>/admin/` 从命令行测试——如果 curl 能连但 Chromium 不能，说明是安全软件拦截
3. 在任务管理器中检查 `ZhuDongFangYu.exe` 是否仍在运行
4. 尝试 kill 该进程（可能需要管理员权限）

**注意**：仅在 Windows 环境出现，CI 环境（Linux）不受影响。

## 2. Quasar 组件选择器策略

### 2.1 `q-select` 下拉选项

**问题**：Quasar 的 `q-select` 不使用原生 `<select>`/`<option>`，而是渲染为 `.q-popup` 或 `.q-menu` 中的 `.q-item`。Playwright 的 `page.selectOption()` 不适用。

**解决方案**：编写 `selectQOption` helper：

```typescript
async function selectQOption(page: Page, selectLabel: string, optionText: string): Promise<void> {
  // 通过 label 文本定位 q-select 容器，用 .first() 避免严格模式违规
  const selectContainer = page.locator('.q-card')
    .locator(`label:has-text("${selectLabel}")`)
    .locator('..')
    .first()
  await selectContainer.click()
  await page.waitForTimeout(300) // 等待 popup 动画
  // 弹出菜单可能是 .q-popup 或 .q-menu
  await page.locator('.q-popup, .q-menu')
    .locator(`text=${optionText}`)
    .first()
    .click()
  await page.waitForTimeout(200)
}
```

**要点**：
- `.first()` 必不可少——Quasar 可能在 DOM 中残留多个 popup/menu 实例
- `.q-popup, .q-menu` 双选择器——不同 Quasar 版本和渲染模式下 popup 的 class 名不同
- `waitForTimeout(300)` 在 click 后等待动画完成——Quasar popup 有 CSS transition

### 2.2 `q-input` readonly 字段读取值

**问题**：Quasar `q-input` 的 `readonly` 字段不显示为 `<input value="...">`，而是一个有 `value` 属性但 Playwright `inputValue()` 可能返回空字符串的结构。

**解决方案**：用 `page.evaluate()` 在浏览器上下文中直接读取 DOM：

```typescript
const value = await page.evaluate(() => {
  const el = document.querySelector('[data-cy="enrollment-code-field"]')
  if (!el) return ''
  const input = el.querySelector('input')
  if (input) {
    if (input.value) return input.value
    const attrVal = input.getAttribute('value')
    if (attrVal) return attrVal
  }
  const text = el.textContent || ''
  if (text.trim().length > 5) return text.trim()
  // 最后 fallback：遍历 data-* 属性
  for (const attr of el.attributes) {
    if (attr.value && attr.value.length > 10 && attr.name !== 'data-cy') return attr.value
  }
  return ''
})
```

**要点**：API 返回是异步的，需要先 `expect(field).not.toBeEmpty({ timeout: 10_000 })` 等待值填入后再读取。

### 2.3 `q-chip` 状态等待

**问题**：Upstream Apply 后状态从 "Inactive" 变为 "Active"，但直接用 `text=Active` 会匹配到多处。

**解决方案**：用 `.q-chip` + `filter` 精确定位：

```typescript
await expect(page.locator('.q-chip')
  .filter({ hasText: /active/i })
  .first()
).toBeVisible({ timeout: 30_000 })
```

## 3. SPA 路由与页面加载

### 3.1 SPA fallback 路由

**问题**：`page.goto('/admin/users')` 可能返回 404，因为 Node.js 静态 server 找不到 `users.html` 文件。

**修复**：SPA proxy 必须实现 client-side routing fallback——对于没有文件扩展名的路径，返回 `index.html`：

```javascript
// 没有 extension 且不是 assets/ 下的文件 → 返回 index.html
if (!filePath.startsWith('assets/') && !extname(filePath)) {
  const index = join(spaDir, 'index.html')
  if (existsSync(index)) {
    serveStaticFile(res, index, 'index.html')
    return
  }
}
```

### 3.2 SPA 加载后等待组件渲染

**问题**：`page.goto('/admin/')` 返回 200 但 SPA 尚未完成 Vue 组件渲染，立即 `fill` login 字段会失败。

**修复**：`goto` 后显式等待关键选择器可见：

```typescript
await page.goto('/admin/')
await page.waitForSelector('[data-cy="login-username"]', { state: 'visible' })
await page.fill('[data-cy="login-username"]', 'admin')
```

### 3.3 登录后 URL 不确定性

**问题**：登录后 SPA 可能跳转到 `/admin/`（默认 Overview）或 `/admin/overview/`，取决于路由配置。

**修复**：用正则匹配两种可能：

```typescript
await expect(page).toHaveURL(/\/admin\/(overview)?$/)
```

## 4. HTTP Server 配置

### 4.1 `keepAliveTimeout` 和 `headersTimeout`

**问题**：Chromium headless 使用 HTTP keep-alive 积极复用连接。Node.js 默认 `keepAliveTimeout=5s` 和 `headersTimeout=60s` 可能导致 server 在 Chromium 仍有 pending 请求时关闭连接，引发 `ERR_ABORTED`。

**修复**：

```javascript
server.keepAliveTimeout = 30000  // 30s，比 Chromium 默认 keep-alive 长
server.headersTimeout = 35000   // 35s，比 keepAliveTimeout 多 5s
```

### 4.2 `Content-Length` header

**问题**：缺少 `Content-Length` 时，Chromium 可能等待更多数据，导致页面加载挂起。

**修复**：所有静态文件响应都设置 `Content-Length`：

```javascript
const data = readFileSync(fullPath)
res.setHeader('Content-Length', data.length)
res.writeHead(200)
res.end(data)
```

### 4.3 `Cache-Control` 策略

- `index.html`：`Cache-Control: no-cache`——确保 SPA 始终获取最新入口
- `assets/` 下文件：`Cache-Control: public, max-age=31536000, immutable`——Vite 构建文件名含 hash，可永久缓存

## 5. Windows 特定注意事项

| 问题 | 影响 | 修复 |
|---|---|---|
| `chromium_headless_shell` loopback 缺陷 | Chromium 无法连接本地 server | `channel: 'chromium'` |
| `execSync` 阻塞事件循环 | HTTP server 无法响应 | `worker_threads` 运行 server |
| 安全软件（360等）拦截 | 同 1.1 现象 | 终止 `ZhuDongFangYu.exe` |
| `npx` 在 Windows 上是 `.cmd` | `spawn('npx', ...)` 找不到 | 用 `cmd.exe /c npx.cmd` 或 `shell: true` |
| `wmic` 已弃用 | 进程 metrics 采集失败 | 改用 PowerShell `Get-CimInstance` |

## 6. E2E 测试编写规范

### 6.1 `data-cy` 属性优先

所有可交互元素必须有 `data-cy` 属性，不依赖文本内容或 CSS class：

```html
<q-input data-cy="login-username" />
<q-btn data-cy="login-submit" />
```

```typescript
await page.fill('[data-cy="login-username"]', 'admin')
await page.click('[data-cy="login-submit"]')
```

### 6.2 单测试 + `test.step()`

CAP-C6-001 是一个完整的 UI 工作流，必须用**单个 `test()` + `test.step()`** 而非多个独立 `test()`：

```typescript
test('CAP-C6-001 Browser Golden Path', async ({ page }) => {
  await test.step('login as admin', async () => { ... })
  await test.step('create user + enrollment', async () => { ... })
  await test.step('create secret → upstream → Test → Apply', async () => { ... })
  // ...
})
```

**原因**：整个工作流有状态依赖（后一步需要前一步创建的资源），独立 test 无法保证执行顺序和状态传递。

### 6.3 禁止 mock API

```typescript
// 禁止：不模拟 API
await page.route('/api/**', route => route.fulfill({ ... }))
```

T4.1 Green evidence 必须是 real browser → real Hub → real Relay 路径，不允许 mock。

### 6.4 `fullyParallel: false` + `workers: 1`

```typescript
export default defineConfig({
  fullyParallel: false,
  workers: 1,
  retries: 0,
})
```

- `workers: 1`：单浏览器实例，避免端口/进程竞争
- `retries: 0`：失败不重试，避免掩盖真实缺陷
- `fullyParallel: false`：按文件顺序执行

## 7. Harness 架构

### 7.1 进程拓扑

```
e2e-harness.mjs (主线程)
├── Go binary: control-hub (spawn)
├── Go binary: runtime-relay (spawn)
├── Worker thread: _server-worker.mjs
│   ├── HTTP server: SPA proxy (dist/spa + API proxy)
│   └── HTTP server: Deterministic Adapter
└── execSync: npx playwright test
```

### 7.2 环境变量传递

```
MEASIX_E2E_BASE_URL        → SPA proxy URL (Playwright baseURL)
MEASIX_E2E_HUB_BASE_URL    → Hub public URL (topology-security test)
MEASIX_E2E_ADAPTER_URL     → Adapter URL (golden-path test)
MEASIX_E2E_ADMIN_PASSWORD  → Bootstrap admin password
```

### 7.3 清理策略

- `SIGINT` / `SIGTERM` / `exit` 事件注册 cleanup handler
- `--keep` 保留 temp 目录用于调试
- 先 `SIGTERM`，3s 后 `SIGKILL`
- temp 目录在 `tmpdir()` 下用 `mkdtempSync` 创建

## 8. 调试技巧

### 8.1 隔离法

当 Playwright 测试失败时，用最小化脚本隔离问题：

1. **Step 1**：直接 `chromium.launch()` + `page.goto()` ——排除 Playwright config 干扰
2. **Step 2**：加 `page.on('requestfailed')` 和 `page.on('response')` 监听——看请求是否发出
3. **Step 3**：用 `curl` 测试同一 URL ——排除 Chromium 因素
4. **Step 4**：如果 curl 成功但 Chromium 失败 → 安全软件或 headless shell 问题

### 8.2 Playwright Report

```typescript
reporter: [
  ['list'],
  ['html', { outputFolder: 'playwright-report' }],
  ['json', { outputFile: '../.artifacts/e2e-playwright.json' }],
],
```

- `list`：终端实时输出
- `html`：失败时截图/视频/trace 可视化
- `json`：机器可读，供 freeze-manifest 消费

### 8.3 `--keep` 保留环境

```bash
node scripts/e2e-harness.mjs --keep
```

保留 temp 目录、Hub DB、进程日志，用于事后排查。

## 9. 相关文件索引

| 文件 | 职责 |
|---|---|
| `console/playwright.config.ts` | Playwright 配置（channel, timeout, reporter） |
| `console/e2e/golden-path.spec.ts` | CAP-C6-001 完整 UI 工作流（13 phases） |
| `console/e2e/topology-security.spec.ts` | CAP-C6-BROWSER-006 拓扑安全验证 |
| `scripts/e2e-harness.mjs` | 一键 T4.1 harness（Hub/Relay/Adapter/SPA/Playwright） |
| `scripts/lib/harness.mjs` | 共享工具函数（环境/端口/进程/SPA proxy/adapter） |
| `scripts/_server-worker.mjs` | Worker thread 中的 HTTP server 实现 |
| `.artifacts/e2e-playwright.json` | Playwright JSON report（freeze evidence） |

## 10. 常见错误速查

| 错误 | 可能原因 | 修复 |
|---|---|---|
| `net::ERR_ABORTED` | headless shell loopback / execSync 阻塞 / 安全软件 | 见 §1.1–1.3 |
| `net::ERR_CONNECTION_RESET` | 同上 | 同上 |
| `net::ERR_CONNECTION_REFUSED` | Server 未启动 / 端口错误 | 检查 `waitFor()` 逻辑 |
| Strict mode violation | 多个元素匹配选择器 | 加 `.first()` 或更精确选择器 |
| `TimeoutError` on `fill` | SPA 未完成渲染 | 加 `waitForSelector` |
| `expect(received).toBeTruthy()` on q-input | Quasar readonly 字段值读取 | 用 `page.evaluate` |
| `page.goto` timeout | 缺少 `Content-Length` 或 keep-alive 超时 | 见 §4.1–4.2 |
