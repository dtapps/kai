# Changelog

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

格式参考 Keep a Changelog；版本号沿用仓库 tag。更早版本见 [GitHub Releases](https://github.com/dtapps/kai/releases)。

## [Unreleased]

### Changed

- 升级 Wails Go 依赖到 `v3.0.0` 正式版；前端 `@wailsio/runtime` 仍为 `3.0.0-beta.34`。
- 升级 `posthog-go` 到 `1.38.0`。

## [v0.2.7] - 2026-10-26

### Added

- 新增 GitCode 发布工作流，GitLab 流水线改为发布同步并移除 Nightly 按钮。
- 新增前端单元测试，前端语句覆盖率提升到 14.38%。
- 新增 CI 镜像说明、Codecov、Coveralls、GitLab 发布同步、Go/Node 覆盖率模板等文档。
- 新增 Coveralls 覆盖率上报工作流。
- 为 Codecov 配置按职责域划分的 6 个组件。

### Changed

- 统一 CI 预装工具链镜像，GitHub 与 CNB 复用同一套 Dockerfile 环境。
- Markdown 格式化从 Prettier 切换为 markdownfmt。
- 移除 goveralls 与 CI 镜像的 arm64 构建。

### Fixed

- 修复 Vitest JSON 转 JUnit 时 `classname` 使用绝对路径的问题，恢复 Codecov 用例关联。

## [v0.2.6] - 2026-10-22

### Added

- 新增 Codecov 覆盖率上报工作流与 `.github/codecov.yml`。
- 新增 Go / 前端单元测试覆盖率检查与增量红线配置。
- Code Wiki 生成从 pull 工作流迁移到 tag 推送触发。
- 为 Go 与 Node 覆盖率报告配置独立数据键。

## [v0.2.5] - 2026-10-22

### Changed

- 升级 Wails、AI SDK 及前端依赖版本。
- 引入 pull 请求工作流配置。
- 修正代码扫描路径规则，使排除项生效。

## [v0.2.4] - 2026-10-19

### Changed

- 升级前后端依赖，调整同步与发布流程。
- Dependabot：`genai`、`openai-go`、`@types/node`、`prettier` 等版本更新。

## [v0.2.3] - 2026-10-10

### Changed

- 升级多处依赖并同步 `go.sum`。

## [v0.2.2] - 2026-10-10

### Added

- 更新器下载接入中转，失败自动回退源站。

### Changed

- 升级构建工具链与前端依赖。

## [v0.2.1] - 2026-10-09

### Changed

- 升级构建工具链、robotgo、OpenAI/Anthropic/GenAI SDK、PostHog、Vite、`@wailsio/runtime` 等依赖。
- 新增 GitHub Actions 依赖更新配置。

## [v0.2.0] - 2026-09-22

### Added

- 构建期注入 PostHog 环境变量。

### Changed

- 升级前端依赖。

## 更早版本

`v0.1.3` ~ `v0.1.9`（2026-08-31 ~ 2026-09-22）及更早的发布说明见 GitHub Releases 页面；本文件从 `v0.2.0` 起维护。
