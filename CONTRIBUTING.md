# 贡献指南

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

先读 `docs/README.md` 的文档地图，再按你要改的部分找对应文档。本文只给最短路径和 PR 约定。

## 1. 本地跑起来

```bash
make tool-deps      # wails3 / sqlc / golangci-lint / govulncheck / markdownfmt 等
make setup          # deps + bindings + sqlc
make swift-build    # 仅 macOS：系统翻译 / 系统 OCR
make dev
```

详细工具链、代码生成链、已知坑见 `docs/development.md`。

## 2. 改代码前

- 业务代码在 `internal/`，入口在 `main.go`；前端在 `frontend/`。
- 不要手改生成物：`frontend/bindings/`、`internal/*store/*.sql.go`、`internal/i18n/locales/*.json`。
- 配置字段改 `internal/settings`，引擎字段改 `internal/engine/engine.go` 的 `engineSchemas`，事件改 `internal/events` + 前端 `frontend/src/utils/events.ts`。
- 新增/修改 RPC、事件、引擎、配置、CI 时，按 `docs/README.md` §4 的表格同步文档。

## 3. 提交前检查

```bash
make format
make lint-go
make vuln-go
make test-go
go test -vet=off ./internal/... ./pkg/...   # make test-go 不含 pkg
make lint-frontend
```

涉及生成链的改动再跑：

```bash
make bindings
make sqlc
make i18n
```

前端测试：

```bash
pnpm --dir frontend test
pnpm --dir frontend test:coverage
```

macOS 上涉及 Swift 的改动：`make swift-build` 后确认 `pkg/swiftbridge/libkai_bridge.dylib` 重新生成，并跑一次系统翻译/系统 OCR。

## 4. PR 约定

- 一个 PR 只做一件事；混入无关格式化会让评审无法聚焦。
- 提交信息用仓库现有风格：`feat(scope): ...`、`fix(scope): ...`、`chore(ci): ...`、`docs: ...`、`test: ...`、`build: ...`。中文描述可以，scope 用英文。
- PR 描述写：改了什么、为什么、怎么验证、影响的平台（macOS / Windows / Linux）。
- 改了以下内容必须在 PR 里说明：
  - 生成文件重新生成（bindings / sqlc / i18n）；
  - `settings.json` 字段或默认值变化；
  - 事件名/payload 变化；
  - 新增/删除 CI workflow 或 secret；
  - 平台能力变化（尤其 macOS 权限、Windows WebView2）。
- 不要提交 `.env`、证书、token、`*.dylib`、`frontend/dist`、`coverage.out`、`frontend/coverage`。

## 5. 安全

漏洞不要开公开 Issue，按 `SECURITY.md` 的渠道报告。CI secret 的权限边界见 `docs/security-privacy.md`。

## 6. 文档

- 文档与代码同 PR 更新；新增文档登记到 `docs/README.md`。
- Markdown 用 `make format-markdown`（markdownfmt），不要用 Prettier。
- 中文是事实源；英文只维护 `README_EN.md` 和用户手册英文版（如新增）。
