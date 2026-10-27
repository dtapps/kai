# 安全与隐私

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

Kai 是本地桌面应用：没有服务端账号体系，主要风险面是**本机数据**、**第三方 API Key**、**构建/发布 secret** 与**匿名统计**。本文说明边界、已知取舍和报告方式。

## 1. 漏洞披露

优先使用 GitHub 的私密漏洞报告：

```text
https://github.com/dtapps/kai/security/advisories/new
```

如果该入口不可用，邮件联系仓库维护者（git 提交作者邮箱 `li.gc@icloud.com`）。请不要在公开 Issue 里贴可复现的利用代码、API Key、token 或用户数据。

报告里请包含：

- 受影响版本 / commit（`Kai <version>` 或 `git rev-parse HEAD`）。
- 平台与系统版本（macOS / Windows）。
- 复现步骤、期望行为、实际行为。
- 相关日志片段（先删除文本内容、API Key、代理密码、token）。
- 如果是依赖漏洞，给出 `govulncheck` / SCA 的输出。

不承诺固定 SLA；确认后会以修复版本或缓解说明回应。

## 2. 本机数据

| 数据       | 位置                                            | 保护状态                        |
|------------|-------------------------------------------------|---------------------------------|
| 界面偏好   | `~/.kai/settings.json`                          | 明文；**代理密码也是明文**      |
| 引擎凭据   | `~/.kai/data/config.db` 的 `api_key` / `secret` | AES-256-GCM 加密                |
| 翻译历史   | `~/.kai/data/history.db`                        | 明文（原文 + 译文）             |
| HTTP 日志  | `~/.kai/data/httplog.db`                        | 明文，可能含请求头/体           |
| 主日志     | `~/.kai/logs/kai.log`                           | 明文；**info 级会记录取词内容** |
| 前端日志   | `~/.kai/logs/frontend.log`                      | 明文                            |
| Swift 日志 | `~/.kai/logs/kai-bridge.log`                    | 明文（仅 macOS）                |

### 2.1 引擎凭据加密

- 算法：AES-256-GCM；密钥用 HKDF-SHA256（固定 salt + info）从本机设备指纹派生（`internal/configstore/crypto.go`）。
- 设备指纹：macOS 读 `ioreg` 的 `IOPlatformUUID`；Linux 读 `/etc/machine-id`；失败回退 `hostname`。
- 密文带前缀 `kai:cipher:`；旧明文数据在 `MigrateSecrets` 里一次性加密，已加密的跳过。
- 密钥不落库、不进仓库。把 `config.db` 单独复制到另一台机器无法解密，表现为 Key 为空；设备指纹变化（换机、重装、hostname 变化）也会导致同样结果。
- 局限：这是「防单文件泄露」，不是抗有权限的本机攻击者。能读进程内存或以同一用户运行代码的攻击者仍可拿到明文。

### 2.2 明文与日志风险

- `settings.json` 的 `proxy.password` 是明文；不要在共享机器上保存代理密码。
- `history.db` 保存翻译原文与译文；`httplog.db` 可能保存 `Authorization` 头、请求体（翻译文本）与响应体。两者都未加密。
- `kai.log` 在 `info` 级会记录取词来源、长度和**内容**（`internal/hotkey/manager.go` 的 `log.hotkey_read_clipboard` 带 `log.field_content`）。如果介意，把 `settings.json` 的 `log.level` 改为 `warn` 或 `error`。
- 关闭 HTTP 日志：`settings.json` 设 `http_log.enabled=false`，重启应用（`httplogstore.Init` 是 `sync.Once`，运行中改不生效）。
- 删除数据：退出应用后 `rm -rf ~/.kai`；开发态是 `~/.kai.dev`。

## 3. 匿名统计（PostHog）

默认**关闭**（`analytics_enabled: false`），必须在「设置 → 通用」手动开启。

上报门控（全部满足才发）：

1. `buildinfo.PosthogToken` 非空（构建期注入）；
2. 非 dev 构建（`buildinfo.Dev != true`，除非显式 `EnableDevUpload`，仅调试用）；
3. `machineid.ProtectedID("kai")` 可用；
4. 用户开关为 true。

端点：`https://us.i.posthog.com`。SDK 配置显式 `DisableGeoIP=false`、`IsServer=false`，因此 PostHog 可按请求 IP 做粗粒度 GeoIP（国家/地区/城市），不采集精确坐标。

已定义 9 个事件，当前有发射点的是：

| 事件                   | 触发                   | 属性                                                                         |
|------------------------|------------------------|------------------------------------------------------------------------------|
| `app_installed`        | 首装首次启动（仅一次） | `app_version`、`channel`                                                     |
| `app_started`          | 每次启动               | `app_version`、`ui_lang`、`channel`                                          |
| `translate_input`      | 输入翻译               | `engine`（逗号拼接）、`src_lang`、`dst_lang`、`len_bucket`、`trigger=manual` |
| `translate_screenshot` | 截图翻译               | `engine`、`ocr_provider`、`ocr_retried`                                      |
| `settings_opened`      | 前端打开设置页         | `tab`                                                                        |
| `feature_toggled`      | 前端切换开关           | `feature`、`enabled`                                                         |
| `error_occurred`       | 翻译/OCR 失败          | `kind`、`engine` 或 `ocr_provider`                                           |

`engine_configured` / `update_installed` 只在常量与测试里出现，当前没有发射点。

边界：

- 不采集文本内容、译文、API Key、账号、剪贴板原文。
- 文本长度只上报分桶（`0` / `1-20` / `21-100` / `101-500` / `501-2000` / `2000+`），见 `analytics.LenBucket`。
- 设备 ID 是 `machineid.ProtectedID("kai")`，按应用名 HMAC 的机器指纹；**它不保存在 `~/.kai` 里，删除应用数据不会重置它**。README §2 的「删除应用数据即可重置」与实现不一致，以代码为准。
- 关闭开关后 `analytics.Close()` 关闭 client，后续 `Track` 直接 no-op。
- PostHog 可能从请求上下文推断 OS/地区；代码本身不显式发送操作系统字段。

## 4. 构建与发布 secret

完整清单见 `ci/README.md` §6。权限原则：

- **编进客户端的 token 视为公开**：`buildinfo.GithubToken`（来自 `UPDATER_TOKEN`）与 `buildinfo.CnbToken`（来自 `CNB_TOKEN`）会写进二进制。只能使用只读、最小范围、可随时吊销的 token，绝不能放 Actions 写权限 token。
- **发布 token 与客户端 token 分开**：`SYNC_CNB_TOKEN` 只用于触发 CNB 构建；`SYNC_GITLAB_TRIGGER_TOKEN` 只用于触发 GitLab 流水线；`SYNC_GITCODE_TOKEN` 只用于 dispatch。不要复用。
- **签名证书**：`WINDOWS_SIGN_CERTIFICATE` 是 .pfx 的 base64（含私钥），只能放 secret；`APPLE_*` 同理。
- **GitHub API 限流 token**：`KAI_GITHUB_TOKEN` 在 CNB / GitLab / Gitee 各配一份（GitCode 保留，当前不发布），源头是 `cnb.cool/dtapp/envs` 的 `github.yml`；只读即可。
- **平台 token 来源不同名同权**：GitHub secret `CNB_TOKEN` 与 CNB 平台自带的 `CNB_TOKEN` 不是同一份配置；确认各自的权限边界，客户端侧不要使用 `repo-release:rw` 这类写权限。
- 本地 `.env` 被 `.gitignore` 忽略；不要把 `.env`、`*.pfx`、`*.p12`、dylib 提交进仓库。

## 5. 依赖与供应链

| 检查              | 入口                                                             | 触发                |
|-------------------|------------------------------------------------------------------|---------------------|
| Go 漏洞           | `make vuln-go`（`govulncheck -show verbose ./...`）              | 本地 / `make check` |
| 依赖更新          | `.github/dependabot.yml`（gomod / npm / github-actions，每日）   | GitHub              |
| 漏洞扫描          | `.cnb/workflows/sca-vulnerability.yml`（`failOnSeverity: high`） | PR                  |
| License 扫描      | `.cnb/workflows/sca-license.yml`（`failOnSeverity: high`）       | PR                  |
| SBOM Delta        | `.cnb/workflows/sca-sbom.yml`（输出 `.cnb-sca/sbom-delta.json`） | PR                  |
| 敏感信息/代码扫描 | `.cnb/workflows/cnb-sechook.yml`                                 | PR                  |
| 代码评审          | `.cnb/workflows/code-review.yml`（AI，`deepseek-v4.1-flash`）    | PR                  |

没有接入 CodeQL、Snyk、Socket 之类的额外扫描；不要假设它们存在。

## 6. 发布完整性

- Release 资产带 `SHA256SUMS`；更新器下载后校验（`pkg/wails-updater-providers`）。
- 自更新只匹配 `updater-` 前缀的压缩包，安装包不参与替换。
- Windows Authenticode 与 macOS 签名/公证都是**可选**的；未配置 secret 的构建不签名。用户应优先从 GitHub / CNB 官方 Release 下载。
- 没有可复现构建承诺；构建产物依赖 runner、Go/Node 版本与签名步骤。

## 7. 权限与最小化

- macOS 只申请两项 TCC：辅助功能（模拟复制键）、屏幕录制（截图）。没有申请输入监控（相关方法已注释）。
- 截图只保存在内存/临时文件，用于 OCR 与截图窗口展示；不会上传到第三方，除非你启用了会把图片发给该引擎的 OCR/翻译服务。
- 翻译请求会发给你配置的引擎（Google / DeepL / OpenAI / ...），其隐私政策由对应服务商决定；Kai 不做二次转发。
- 代理/DNS 只影响出站请求；不会绕过引擎自身的 TLS。

## 8. 非目标 / 已知边界

- 不是沙箱：应用以当前用户权限运行，能读用户可读的文件。
- 不提供多用户隔离；`~/.kai` 的权限是当前用户。
- 历史与 HTTP 日志不加密；需要强隐私时关闭 HTTP 日志并定期清理历史。
- 不隐藏「本机已被入侵」场景下的凭据；设备指纹派生只能防离线复制。
- 不保证第三方引擎的可用性、内容审核或数据保留策略。
