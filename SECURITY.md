# 安全策略

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

## 报告漏洞

优先使用 GitHub 私密漏洞报告：

```text
https://github.com/dtapps/kai/security/advisories/new
```

如果入口不可用，邮件联系维护者：`li.gc@icloud.com`。

不要在公开 Issue / PR / 讨论区贴：

- 可复现的利用代码或 payload；
- API Key、token、证书、代理密码；
- 用户的翻译原文、剪贴板内容、`httplog.db` 原始行。

报告请包含：

- 版本 / commit、平台与系统版本；
- 复现步骤、期望行为、实际行为；
- 相关日志（先脱敏）；
- 如果是依赖问题，附 `govulncheck` 或 SCA 输出。

## 适用范围

- 当前 master 与最新 Release。
- macOS / Windows 正式构建。
- 本仓库代码与发布工作流；第三方翻译引擎/服务本身的问题请找对应厂商。

## 已知边界（不是漏洞）

- 应用不提供沙箱，以当前用户权限运行。
- `history.db` 与 `httplog.db` 是明文 SQLite；`httplog` 可能记录请求头/体。
- `settings.json` 的代理密码是明文。
- `kai.log` 在 info 级会记录取词内容。
- 客户端内置的 `GithubToken` / `CnbToken` 是只读更新 token，会随二进制公开。
- 历史与 HTTP 日志没有加密；`config.db` 只加密 `api_key` / `secret` 两列。

完整说明见 `docs/security-privacy.md`。

## 依赖与供应链

- 本地：`make vuln-go`。
- PR：CNB 的 SCA 漏洞 / License / SBOM 扫描，以及 `cnb-sechook` 敏感信息扫描。
- 依赖更新：`.github/dependabot.yml`（gomod / npm / github-actions，每日）。

## 发布完整性

- Release 资产带 `SHA256SUMS`；自更新只使用 `updater-` 前缀包并校验。
- Windows Authenticode 与 macOS 签名/公证是可选步骤；未配置 secret 的构建不签名。
- 请优先从 GitHub 或 CNB 官方 Release 下载。
