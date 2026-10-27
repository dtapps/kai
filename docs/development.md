# 开发与代码生成

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

面向第一次在本机跑起来、改代码、跑检查的人。架构主干见 `PLAN.md`，测试细节见 `testing.md`，配置字段见 `configuration.md`。

## 1. 工具链

| 工具             | 仓库声明 / 实际使用                               | 依据                                                |
|------------------|---------------------------------------------------|-----------------------------------------------------|
| Go               | `1.27.3`                                          | `go.mod`、两份 CI Dockerfile 的 `FROM`              |
| Node             | `.nvmrc` 为 `25.9.0`；CI 镜像为 `26.12.1`         | `frontend/.nvmrc`、`.github/ci/Dockerfile`          |
| pnpm             | `packageManager: pnpm@12.15.0`；CI 镜像 `12.16.0` | `frontend/package.json`、CI Dockerfile `ARG`        |
| Wails 3          | Go 依赖 `github.com/wailsapp/wails/v3 v3.0.0`     | `go.mod`、`ARG WAILS3_VERSION=v3.0.0`               |
| Wails 前端运行时 | `@wailsio/runtime 3.0.0-beta.34`                  | `frontend/package.json`                             |
| sqlc             | `v1.31.1`                                         | CI Dockerfile `ARG SQLC_VERSION`、生成文件头        |
| golangci-lint    | `v2.15.0`                                         | `.github/ci/Dockerfile`                             |
| govulncheck      | `v1.9.0`                                          | `.github/ci/Dockerfile`                             |
| markdownfmt      | `v3.1.0`                                          | `.github/ci/Dockerfile`、`Makefile:format-markdown` |

> Wails 的 Go 依赖已是正式版 `v3.0.0`，前端运行时仍锁在 `3.0.0-beta.34`。升级时两边要一起看，不要只改一边。

一次性安装本机工具：

```bash
make tool-deps
```

它会 `go install` wails3、sqlc、golangci-lint、govulncheck、gojq、yamlfmt、shfmt、dockerfmt、markdownfmt。已安装的会被覆盖到最新版；CI 使用固定版本，本地版本不必与 CI 完全一致，但排错时以 CI 为准。

## 2. 第一次跑起来

```bash
# 1. Go module + 前端依赖
make deps

# 2. 生成 Wails TypeScript 绑定（frontend/bindings）
make bindings

# 3. 生成 sqlc 代码（三个 store）
make sqlc

# 4. macOS 额外：编译 Swift 桥接 dylib（系统翻译 / 系统 OCR 依赖）
make swift-build

# 5. 开发模式（Wails 热重载，端口默认 9247）
make dev
```

`make setup` 等价于 `deps + bindings + sqlc`，**不包含** `swift-build`、`icons`、`build-assets`、`i18n`。macOS 上如果设置页里「系统翻译 / 系统 OCR」不可用，先补 `make swift-build`。

开发态的数据目录由 `buildinfo.Dev` 决定：`true` 走 `~/.kai.dev/`，`false` 走 `~/.kai/`（`internal/buildinfo/buildinfo.go:38`）。`make dev` 走 `build/config.yml` 的 `dev_mode`，其中执行的是 `wails3 build DEV=true`，因此不会污染正式数据。

## 3. 代码生成链

| 目标         | 命令                 | 产物 / 去向                                                                 | 是否入库           |
|--------------|----------------------|-----------------------------------------------------------------------------|--------------------|
| Wails 绑定   | `make bindings`      | `frontend/bindings/`（按 Go 包分目录）                                      | 是                 |
| 图标         | `make icons`         | `build/appicon.png`、`build/darwin/icons.icns`、`build/windows/icon.ico` 等 | 是                 |
| 构建资源     | `make build-assets`  | `build/config.yml` → 各平台 `Info.plist` / `info.json` 等                   | 是                 |
| sqlc         | `make sqlc`          | 三个 store 的 `db.go` / `models.go` / `querier.go` / `query.sql.go`         | 是                 |
| i18n（后端） | `make i18n-go`       | `internal/i18n/locales/zh-CN.json`、`en-US.json`                            | 是                 |
| i18n（前端） | `make i18n-frontend` | 当前为 no-op，见 `i18n.md` §3                                               | —                  |
| Swift 桥接   | `make swift-build`   | `pkg/swiftbridge/libkai_bridge.dylib` / `.a`                                | 否（`.gitignore`） |

组合目标：

```bash
make wails3-generate   # bindings + icons + build-assets
make code-generate     # sqlc + i18n + swift-build
```

CI 的检查任务会先跑 `bindings / icons / build-assets / sqlc / i18n`，再跑格式化与检查（`.cnb/workflows/push.yml`、`.github/workflows/push.yml`），所以提交前最好本地也跑一遍生成链，避免 CI 里生成出与提交内容不同的文件。

**生成文件不要手改**：`frontend/bindings/`、三个 store 的 `*.sql.go`、`internal/i18n/locales/*.json` 都是生成物。改源头（Go 方法、`query.sql`、`split/*.json`）后重新生成。

## 4. 日常校验

```bash
make format            # 全部格式化：Go / Swift / 前端 / i18n JSON / JSON / YAML / Markdown / Shell / Dockerfile
make lint-go           # golangci-lint run ./...
make lint-frontend     # tsc --noEmit + vite build --mode development
make test-go           # go test -vet=off -v ./internal/... -count=1
make fuzz-go           # 当前仓库没有 Fuzz 目标，实际只跑普通测试
make vuln-go           # govulncheck -show verbose ./...
make check             # lint-go + lint-go-fix + lint-frontend + test-go + fuzz-go + vuln-go
make check-cross       # darwin/arm64（CGO）+ windows/amd64（CGO=0）交叉编译
```

几个容易误会的点：

- `make test-go` **只跑 `./internal/...`**，不跑 `./pkg/...`。`pkg/wails-updater-providers` 的测试只在 CI 覆盖率任务里跑（`-coverpkg` 命令显式包含 `./pkg/...`）。
- `make lint-frontend` 不做 `svelte-check`；`frontend/package.json` 里有 `check` 脚本，但 Makefile 没调用。
- `make check` 不含 `format`、`check-cross`，也不生成 bindings/sqlc/i18n。
- `make fuzz-go FUZZ=FuzzXxx TIME=30s` 只有在仓库里存在对应 `FuzzXxx` 函数时才有意义；当前没有 Fuzz 测试。
- `make format-markdown` / `format-yaml` / `format-json` 在工具缺失时会打印提示并**以成功退出**，不要把它当成「已格式化」。

## 5. 打包命令

```bash
make darwin-build          # bin/Kai（不打包 .app）
make darwin-package        # bin/Kai.app
make darwin-package-dmg    # bin/Kai.dmg（本地用；Release 工作流不用 dmg）
make windows-build         # bin/Kai.exe
make windows-package       # bin/Kai.exe + NSIS 安装包
make linux-build           # 仅本地联调，Linux 不在支持范围
```

本地 `make darwin-package` 没有传 `EXTRA_TAGS=sqlite_mattn`，默认用 `modernc.org/sqlite`（纯 Go）。Release 工作流在非 Windows 平台显式设置 `EXTRA_TAGS=sqlite_mattn` 和 `CGO_ENABLED=1`，两份产物链接的 SQLite 驱动不同（`internal/sqlite/`、`.github/workflows/release.yml` 的「配置 SQLite 驱动」步骤）。

## 6. 已知坑

每条按「现象 → 原因 → 解法」写。

### 6.1 `go build` / `go test` 报 `pattern all:frontend/dist: no matching files found`

- 原因：`main.go` 用 `//go:embed all:frontend/dist` 嵌入前端产物，该目录被 `.gitignore` 忽略，干净检出时不存在。
- 解法：

```bash
mkdir -p frontend/dist && touch frontend/dist/index.html
```

CI 也是这么做的（`.cnb/workflows/push.yml`「创建 frontend/dist 占位」）。要真正构建前端用 `make darwin-build` / `make windows-package`。

### 6.2 `wails3: command not found`

- 原因：没装 Wails CLI，或 `$GOPATH/bin` 不在 `PATH`。
- 解法：`make tool-deps`，然后确认 `go env GOPATH` 的 `bin` 在 `PATH`。

### 6.3 macOS 上系统翻译 / 系统 OCR 不可用，日志提示 Swift 桥接加载失败

- 原因：`pkg/swiftbridge/libkai_bridge.dylib` 是构建产物且不入库；开发态需要本地编译。
- 解法：`make swift-build`，确认 `pkg/swiftbridge/libkai_bridge.dylib` 存在。加载路径与降级逻辑见 `pkg/swiftbridge/load.go`。

### 6.4 复制键不生效 / 模拟按键没反应

- 原因：macOS 未授予「辅助功能」权限；或复制键被「自动读取剪贴板」自动停用。
- 解法：设置 → 快捷键 → 授权卡片里检查辅助功能；关闭「自动读取剪贴板」后复制键会恢复原状态（`frontend/src/components/settings/ShortcutsTab.svelte`、`internal/execkey/controller.go`）。

### 6.5 截图翻译没反应 / 截出来是黑图

- 原因：macOS 未授予「屏幕录制」；或快捷键未启用。
- 解法：设置 → 快捷键 → 屏幕录制授权；确认截图快捷键已启用。截图实现是 `screencapture -i`（`internal/engine/ocr_tesseract.go:208`），Windows 没有该实现。

### 6.6 Windows 窗口偶发 `80010108` / WebView2 崩溃

- 原因：WebView2 controller 创建竞态 + GPU 进程异常。
- 现状：窗口改为「创建后立即 `Hide()`」，并注入 `--disable-gpu` / `--disable-gpu-compositing`（`main.go` 窗口创建段、`internal/webview/webview_windows.go`）。
- 解法：先确认 WebView2 Runtime 已安装；仍复现时收集 `~/.kai/logs/kai.log` 与 Windows 事件查看器记录。

### 6.7 `make test-go` 通过但 CI 挂了

- 原因：CI 还跑 `./pkg/...`、`govulncheck`、`golangci-lint`、前端 `tsc` + `vite build`，以及生成链一致性。
- 解法：本地至少补跑：

```bash
go test -vet=off ./internal/... ./pkg/...
make lint-go
make vuln-go
make lint-frontend
```

### 6.8 改了 `settings.json` 字段但前端读不到

- 原因：`ConfigWrapper.SaveConfig` 只同步固定字段（语言、主题、默认语言、快捷键、TTS、执行键、自动剪贴板、统计开关），不含 `proxy` / `http_log` / `log` / `updater` / `dns_configs`（`internal/service/config_wrapper.go:79`）。
- 解法：这些字段直接编辑 `settings.json`，保存后由 viper 热重载；不要在设置页里找开关。

### 6.9 改了 i18n split 文件但界面没变

- 原因：改的是 `split/` 源文件，没合并；或改的是生成物。
- 解法：`make i18n-go` 重新合并；前端当前直接改 `frontend/src/i18n/zh-CN.ts` / `en-US.ts`（详见 `i18n.md`）。

### 6.10 前端测试在 CI 能跑、本地 `pnpm test` 报模块找不到

- 原因：`frontend/vitest.config.ts` 不 merge `vite.config.ts`，只保留 `@bindings` 别名；`frontend/bindings` 必须先由 `make bindings` 生成。
- 解法：先 `make bindings`，再在 `frontend/` 下 `pnpm test`。
