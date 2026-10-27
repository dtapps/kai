# ADR-0010：更新源双 provider，按界面语言路由

> status: 已采纳（生效中） · last_verified: 2026-10-27 · 决策日期：2026-08-15

## 背景

Kai 同时发布到 GitHub 与 CNB。用户分布两边：

- 中文用户访问 GitHub 可能慢/不稳定，CNB 更快；
- 海外用户更习惯 GitHub；
- 只做一个源会让另一半用户下载失败。

同时，Wails 官方 updater 只提供 GitHub provider，CNB 没有现成实现。

## 决策

在 `pkg/wails-updater-providers` 实现双 provider + 路由：

- `githubProvider`：GitHub Releases API + `SHA256SUMS`。
- `cnbProvider`：CNB Releases API + `SHA256SUMS`。
- `MirrorProvider` 作为唯一 `updater.Provider`，每次 `Check` / `Download` 时按包级 `GetSource()` / `GetLocale()` 现算主源。
- `source` 为空/`auto` 时：中文（`zh-CN`）走 CNB，其它走 GitHub；可显式 `github` / `cnb` 强制。
- 资源匹配只认 `updater-` 前缀的升级包，避免把安装包当自更新文件。
- 预发布：稳定版比 tag；预发布比 `BUILD_TIME` / `GIT_COMMIT` 附带文件。
- 运行时语言/主题变更通过 `SetLocale` / `SetTheme` 同步到库全局，更新窗口与 matcher 立即跟随。

## 后果

正面：

- 中英文用户各走就近源；一个源故障时可手动切换。
- 复用官方 `updater.Provider` 接口与资源匹配类型，升级 Wails 时改动可控。
- 更新窗口由库自建（BYO），语言/主题可热更新。

负面 / 约束：

- CNB 源需要 token；匿名请求可能 401。客户端内置的 `CnbToken` 是只读 token，不能放发布权限。
- `source` 覆盖只能在 `settings.json` 改，没有 UI；改完要重启（`SetSource` 只在启动时调用）。
- 预发布必须上传 `BUILD_TIME` / `GIT_COMMIT` 两个附件，否则直接报错；这是发布流程的硬约束。
- 资源命名必须包含平台与架构字符串，否则 matcher 找不到。
- 两个 provider 的 API 字段不同，新增逻辑要两边都测（`pkg/wails-updater-providers/*_test.go`）。

## 证据

- `pkg/wails-updater-providers/mirror_provider.go`：`decideSource`、惰性初始化、`SetSource` / `GetSource`。
- `pkg/wails-updater-providers/github_provider.go`、`cnb_provider.go`：两套 API 与校验。
- `pkg/wails-updater-providers/matcher.go`：`updater-` 前缀匹配。
- `main.go`：`kupdater.SetSource` / `SetLocale` / `SetTheme`、`app.Updater.Init`。
- `internal/settings/service.go`：`UpdaterConfig{Prerelease, Source}`。
