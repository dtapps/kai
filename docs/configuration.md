# 配置与数据目录

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

配置分两处：界面偏好写 `settings.json`（viper，可热重载），引擎凭据写 `config.db`（SQLite，凭据加密）。两者不要混。

## 1. 数据目录

| 模式                | 根目录        | 数据库                 | 日志                    |
|---------------------|---------------|------------------------|-------------------------|
| 正式（`Dev=false`） | `~/.kai/`     | `~/.kai/data/*.db`     | `~/.kai/logs/*.log`     |
| 开发（`Dev=true`）  | `~/.kai.dev/` | `~/.kai.dev/data/*.db` | `~/.kai.dev/logs/*.log` |

目录规则集中在 `internal/buildinfo/buildinfo.go`：`DataHome()`、`DataDir()`、`DBDir()`、`LogDir()`。`main.go` 启动时只 `MkdirAll` 根目录与 `data/`，`logs/` 由日志模块按需创建。

| 文件                  | 内容                                           | 生成方                          |
|-----------------------|------------------------------------------------|---------------------------------|
| `settings.json`       | 界面 / 功能偏好                                | `internal/settings`（viper）    |
| `data/config.db`      | 引擎配置与凭据                                 | `internal/configstore`（sqlc）  |
| `data/history.db`     | 翻译历史                                       | `internal/historystore`（sqlc） |
| `data/httplog.db`     | HTTP 请求日志                                  | `internal/httplogstore`（sqlc） |
| `logs/kai.log`        | 主日志（按天归档为 `kai-YYYY-MM-DD.log[.gz]`） | `internal/logutil`              |
| `logs/frontend.log`   | 前端 console / JS 错误转发                     | `internal/logutil`              |
| `logs/kai-bridge.log` | Swift 桥接层日志（仅 macOS）                   | `pkg/swiftbridge`               |

## 2. `settings.json`

默认值来自 `settings.DefaultSettings()`（`internal/settings/service.go:172`）。首次启动时若文件不存在会写入默认配置；每次启动还会重写一次，补齐缺失字段并丢弃未知 key。

### 2.1 字段表

| JSON key              | 类型 / 默认值                                          | 含义                                           | 设置页 UI    |
|-----------------------|--------------------------------------------------------|------------------------------------------------|--------------|
| `language`            | string `auto`                                          | 界面语言模式：`auto` / `zh-CN` / `en-US`       | 有           |
| `theme`               | string `auto`                                          | 主题：`auto` / `light` / `dark`                | 有           |
| `default_to`          | string `zh`                                            | 翻译默认目标语言                               | 翻译窗口     |
| `default_from`        | string `auto`                                          | 翻译默认源语言（`auto` 自动检测）              | 翻译窗口     |
| `hotkeys.input`       | `{key:"Alt+A", enabled:true}`                          | 唤起输入翻译窗口的注册键                       | 有           |
| `hotkeys.screenshot`  | `{key:"Alt+S", enabled:false}`                         | 截图翻译注册键（仅 macOS 可用）                | 有           |
| `execkeys.copy`       | `{key:"Cmd+C"/"Ctrl+C", enabled:true, fallback:false}` | 执行键：程序模拟按下它完成复制                 | 有           |
| `auto_clipboard`      | bool `false`                                           | 按输入翻译键时直接读剪贴板，不模拟复制         | 输入窗口     |
| `copy_key_snapshot`   | object / `null`                                        | 开启自动剪贴板时快照的复制键状态，关闭时恢复   | 无（内部）   |
| `tts`                 | `{engine:"system", speed:1.0}`                         | 语音合成配置；当前没有使用方与 UI              | 无           |
| `http_log`            | `{enabled:true, retention_days:30}`                    | HTTP 请求日志开关与保留天数                    | 无           |
| `log`                 | `{level:"info", retention_days:30, compress:true}`     | 主日志等级、保留天数、归档压缩                 | 无           |
| `proxy`               | `{enabled:false, protocol:"http", port:8080, ...}`     | HTTP/HTTPS/SOCKS5 代理                         | 无           |
| `updater`             | `{prerelease:true, source:""}`                         | 预发布通道；更新源 `""`（自动）/`github`/`cnb` | 预发布在托盘 |
| `analytics_enabled`   | bool `false`                                           | 匿名统计开关（opt-in）                         | 有           |
| `analytics_installed` | bool `false`                                           | 是否已发过 `app_installed`（内部标记）         | 无           |
| `dns_configs`         | array `[]`                                             | 自定义 DNS：`[{enabled, servers:["1.1.1.1"]}]` | 无           |

字段定义：`Settings`、`HotkeyEntry`、`ExecKeyEntry`、`TTSConfig`、`HttpLogConfig`、`LogConfig`、`ProxyConfig`、`UpdaterConfig`、`DNSConfig`，均在 `internal/settings/service.go`。

### 2.2 热重载与写回

- 文件变更由 fsnotify 监听，**防抖 500ms**；`saving` 标记避免自身写盘触发回调（`internal/settings/service.go:283`）。
- 热重载后的回调只同步：后端 i18n 语言、日志等级/保留/压缩、更新器语言/主题。**不会**重新注册全局快捷键，也**不会**重建引擎 HTTP 客户端或更新器实例。
- `writeConfig` 用全新 viper 实例只写已知字段，因此文件里已废弃的旧 key 会被清掉。
- 前端 `SaveConfig` 只回写固定子集：`language`、`theme`、`default_to`、`default_from`、`hotkeys`、`tts`、`execkeys`、`auto_clipboard`、`copy_key_snapshot`、`analytics_enabled`（`internal/service/config_wrapper.go:79`）。`proxy`、`http_log`、`log`、`updater`、`dns_configs` 不会从前端保存。

### 2.3 生效时机

| 配置                        | 生效方式                                                                          |
|-----------------------------|-----------------------------------------------------------------------------------|
| 语言 / 主题                 | 设置页保存立即生效；手改文件只同步后端 i18n/日志/更新器，前端 UI 与托盘需重启刷新 |
| 快捷键 / 复制键             | 设置页保存后立即 `Register()` 重注册；手改文件需重启                              |
| 代理 / DNS                  | 启动时构建 HTTP client；改后需重启（或触发一次引擎重载）                          |
| HTTP 日志开关               | 启动时 `httplogstore.Init`；改后需重启                                            |
| 日志等级 / 保留 / 压缩      | 热重载立即生效                                                                    |
| 更新源 `updater.source`     | 启动时 `SetSource`；改后需重启                                                    |
| 预发布 `updater.prerelease` | 托盘复选框写配置；更新器 provider 启动时读取，改后需重启                          |
| 匿名统计                    | 设置页开关立即生效；关闭后断开 PostHog 连接                                       |

## 3. 引擎配置（`config.db`）

表结构：

```sql
CREATE TABLE engines (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    engine   TEXT    NOT NULL UNIQUE,
    enabled  INTEGER NOT NULL DEFAULT 0,
    api_key  TEXT    NOT NULL DEFAULT '',
    secret   TEXT    NOT NULL DEFAULT '',
    extra    TEXT    NOT NULL DEFAULT '',
    endpoint TEXT    NOT NULL DEFAULT ''
);
```

- `engine` 有 `UNIQUE` 约束：**每个引擎类型只有一行**，不支持同一引擎多账号/多实例。
- 启动时若表为空，写入默认引擎：`apple`、`google`，macOS 额外 `vision`（`engine.DefaultEngineConfigs()`、`EngineWrapper.loadEngines()`）。
- 敏感字段：`api_key` / `secret` 由 `configstore` 加密后落库，读取时解密；`extra` 是 JSON 字符串（如 OpenAI 的 `model` / `timeout_sec`，OCR 的 `langs` / `correct_text` / `timeout_sec` / `retry_count`）。
- 系统内置 `apple` / `vision` 不可删除，但可以停用（`service.EngineWrapper.RemoveEngine`）。
- OCR 单选：启用一个 OCR 引擎时，会自动停用其它 OCR 引擎（`disableOtherOcrs`）。

字段从哪来：`internal/engine/engine.go` 的 `engineSchemas` 是唯一事实源，前端按 `GetEngineSchema` 动态渲染。具体字段与默认值见 `engines.md`。

## 4. 代理、DNS、日志与更新源

### 4.1 代理

```json
"proxy": {
  "enabled": true,
  "protocol": "http",
  "host": "127.0.0.1",
  "port": 7890,
  "username": "",
  "password": ""
}
```

- 支持 `http` / `https` / `socks5`；`buildProxyURL` 会带 Basic Auth（`internal/network/transport.go`）。
- 引擎 HTTP client 在注册引擎时构建；代理改后重启应用最稳妥。

### 4.2 自定义 DNS

```json
"dns_configs": [
  { "enabled": true, "servers": ["1.1.1.1", "8.8.8.8"] }
]
```

- `network.resolveHost` 取所有启用配置的服务器，按顺序尝试；全部失败回退系统 DNS。
- 只解析 IPv4（`LookupIP(..., "ip4", ...)`）。
- **已知偏差**：`dns_configs` 在 `Settings` 结构体和读取逻辑中存在，但 `settings.writeConfig()` 没有写回该字段（`internal/settings/service.go:333`），因此通过 `settings.json` 配的自定义 DNS 会在下一次保存/启动重写时被清掉。当前实际可用性以代码为准，不要把它当成稳定配置项。

### 4.3 日志

```json
"log": { "level": "debug", "retention_days": 7, "compress": true }
```

- `level`：`debug` / `info` / `warn` / `error`，非法值回退 `info`。
- `retention_days <= 0` 表示只按天归档、不清理。
- 归档在**启动时**判断：当天不是 `kai.log` 的 mtime 日期时，重命名为 `kai-YYYY-MM-DD.log`，按 `compress` 决定是否 `.gz`。运行中跨天不会立刻换文件。
- 前端 `frontend.log` 使用同一套等级与压缩配置，并在启动时按天归档；但它的过期清理没有独立触发（`FrontendWriter` 不调用 `Rotator.cleanup`），实际长期保留取决于文件大小/手动清理。
- `kai-bridge.log` 由 Swift 桥接层接收同一份 `LogConfig`，等级/保留/压缩由 Swift 侧执行。

### 4.4 更新源

```json
"updater": { "prerelease": true, "source": "" }
```

- `source` 为空：按界面语言自动选源，中文走 CNB，其它走 GitHub（`pkg/wails-updater-providers/mirror_provider.go` 的 `decideSource`）。
- `source: "github"`：只走 GitHub，下载后用 `SHA256SUMS` 校验。
- `source: "cnb"`：只走 CNB；匿名请求可能 401，需内置 `CnbToken`。
- 预发布判断：stable 比 tag，预发布比 `BUILD_TIME` / `GIT_COMMIT` 附带文件。
- 更新资源只匹配 `updater-` 前缀的压缩包（`NewUpdaterAssetMatcher`），安装包不参与自更新。

## 5. 构建期变量

`internal/buildinfo/buildinfo.go` 暴露可被 `-ldflags -X` 覆盖的变量：

| Go 变量            | Makefile / 环境变量  | 说明                                        |
|--------------------|----------------------|---------------------------------------------|
| `Version`          | `VERSION`            | 版本号；默认 `git describe --tags --always` |
| `BuildTime`        | `BUILD_TIME`         | UTC RFC3339；默认当前时间                   |
| `GitCommit`        | `GIT_COMMIT`         | 提交短哈希                                  |
| `Dev`              | `DEV`                | `true` → `~/.kai.dev`；`false` → `~/.kai`   |
| `GithubToken`      | `GITHUB_TOKEN`       | 更新器 GitHub 只读 token                    |
| `CnbToken`         | `CNB_TOKEN`          | 更新器 CNB token                            |
| `PosthogToken`     | `POSTHOG_TOKEN`      | 匿名统计客户端 key；空则不上报              |
| `PosthogProjectID` | `POSTHOG_PROJECT_ID` | PostHog 项目 ID                             |

`Makefile` 顶部会在根目录存在 `.env` 时自动 `include` 并 `export`，所以本地可以放：

```bash
VERSION=v0.2.7
DEV=true
GITHUB_TOKEN=github_pat_xxx
CNB_TOKEN=cnb_xxx
POSTHOG_TOKEN=phc_xxx
POSTHOG_PROJECT_ID=12345
```

`.env` 已被 `.gitignore` 忽略。注意 `buildinfo.GithubToken` / `CnbToken` 会被编进客户端二进制，必须使用最小权限的只读 token；详见 `security-privacy.md`。

## 6. 重置与迁移

```bash
# 只重置引擎配置与历史（保留界面偏好）
rm -f ~/.kai/data/config.db ~/.kai/data/history.db ~/.kai/data/httplog.db
```

```bash
# 全量重置（包含界面偏好、日志、匿名统计标记）
rm -rf ~/.kai
```

- 引擎凭据的加密密钥由本机设备指纹派生（`internal/configstore/crypto.go`）。把 `config.db` 复制到另一台机器，密文无法解密，表现是 API Key 为空，需要重新填写。
- 重置后首次启动会重建 `settings.json` 与默认引擎（`google`，macOS 另加 `apple` / `vision`）。
- 删除数据库前先退出应用，避免 WAL 文件（`*.db-wal` / `*.db-shm`）残留。
