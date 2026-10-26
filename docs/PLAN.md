# Kai 项目规划与架构现状

> 本文最初是 2026-08 的立项规划，现按当前实现更新（最后核对：2026-10-26）。

- 当前版本：`v0.2.x`（最新 tag `v0.2.6`）
- 技术栈：**Wails 3 (Go) + Svelte 5 + TypeScript 7 + Tailwind 4 + Vite 8**
- 历史决策过程见 `docs/2026-*.md`（开发日志，已冻结）；文档缺口与后续计划见 `docs/docs-plan.md`

---

## 1. 产品定位

常驻系统托盘（菜单栏）的轻量翻译工具，开机自启、失焦可用：

| 能力       | 触发方式（mac / win）            | 说明                                          |
|------------|----------------------------------|-----------------------------------------------|
| 输入翻译   | 全局快捷键 `⌥+A` / `Alt+A`       | 唤起翻译窗口，手动输入或由复制键回填          |
| 截图翻译   | 全局快捷键 `⌥+S`（**仅 macOS**） | 区域截图 → OCR → 翻译，结果在截图窗口展示     |
| 复制键取词 | 执行键 `⌘+C` / `Ctrl+C`          | 模拟系统复制，把当前选区填入输入框并翻译      |
| 自动剪贴板 | 输入翻译窗口内开关               | 按输入翻译键时直接读剪贴板，不模拟复制键      |
| OCR        | 随截图翻译调用                   | macOS 用系统 Vision；tesseract 为可选外部引擎 |

平台边界（以代码为准）：

- **macOS**：完整能力（系统翻译 / 系统 OCR / 截图翻译 / 复制键 / 自动更新）。
- **Windows**：输入翻译、复制键翻译、在线引擎、自动更新；**不支持系统翻译与截图翻译**（区域截图仅 macOS）。
- **Linux**：仅保留构建与纯在线翻译能力，不在支持范围。

参考对象：Bob、Easydict（功能与交互参考，非代码依赖）。

---

## 2. 技术选型

| 层     | 选型                                                                               | 说明                                                                  |
|--------|------------------------------------------------------------------------------------|-----------------------------------------------------------------------|
| 框架   | **Wails 3 `v3.0.0-beta.34`**                                                       | Go 后端 + 原生 WebView（mac: WKWebView / win: WebView2），无 Chromium |
| 后端   | Go `1.27.3`，module `cnb.cool/dtapp/kai`                                           | 领域 Service + Wrapper 门面，事件驱动 IPC                             |
| 前端   | **Svelte 5 + TypeScript 7 + Tailwind 4 + Vite 8**                                  | 多 HTML 入口（translate / settings / screenshot），无前端路由框架     |
| 状态   | Svelte runes + 自研 `stores/`（localStorage 持久化）                               | 无 Pinia / vue-router                                                 |
| 包管理 | pnpm 12（前端）、Go modules（后端）                                                | 前端 `frontend/.nvmrc` 固定 Node 25.9.0                               |
| 存储   | SQLite（默认 modernc 纯 Go；`sqlite_mattn` 标签切 CGO）/ [sqlc](https://sqlc.dev/) | `config.db` / `history.db` / `httplog.db`                             |
| 配置   | viper（`settings.json`，支持热重载）                                               | 引擎凭据独立存 `config.db`，不进 settings                             |
| i18n   | go-i18n/v2（后端）+ 自研前端 i18n                                                  | split JSON，`make i18n` 合并                                          |
| 构建   | `wails3` + Taskfile + Makefile                                                     | `make dev` / `make darwin-package` / `make windows-package`           |
| 可观测 | logutil 按天滚动 + httplogstore + PostHog（opt-in）                                | 日志/HTTP 日志/匿名统计互相独立                                       |

### Wails 3 关键 API（当前用法）

- 入口：`application.New(application.Options{...})` → `app.Run()`
- 多窗口：`app.Window.NewWithOptions(...)`，统一 **创建后立即 `Hide()`**（见 §7）
- 托盘/菜单：`app.SystemTray.New()` + `app.Menu.New()`，语言变更后整棵菜单重建
- 事件：`application.RegisterEvent[T](name)` 注册强类型事件，`app.Event.Emit/On` 通信
- 服务：`application.NewService(...)` 注册领域 Service，前端经 `frontend/bindings/` 调用
- 资源：`//go:embed all:frontend/dist` + `application.AssetFileServerFS(assets)`

---

## 3. 目录结构

```text
kai/
├── main.go                    # 组装：日志/配置/DB/服务/窗口/托盘/事件
├── go.mod                     # module cnb.cool/dtapp/kai
├── Makefile                   # 开发、代码生成、检查、打包、依赖升级
├── Taskfile.yml               # Wails 任务入口
├── build/                     # Wails 构建配置（config.yml / darwin / windows / linux / ios）
├── docs/                      # 规划、CI、覆盖率、开发日志
├── internal/
│   ├── service/               # AppService 及 Config/Engine/History/Translate/Window Wrapper（RPC 门面）
│   ├── translate/             # 翻译/OCR 编排、多引擎流式结果、历史写入
│   ├── engine/                # Translator / OcrEngine 接口、Registry、9 个翻译引擎 + 2 个 OCR 引擎
│   ├── settings/              # settings.json（viper）读写与热重载
│   ├── configstore/           # config.db：引擎配置 + 凭据加密（sqlc）
│   ├── historystore/          # history.db：翻译历史（sqlc）
│   ├── httplogstore/          # httplog.db：HTTP 请求日志（sqlc，append-only）
│   ├── sqlite/                # SQLite 驱动切换（modernc / mattn）
│   ├── hotkey/                # 注册类全局快捷键（input / screenshot）
│   ├── execkey/               # 执行类快捷键（复制键；mac robotgo / win makc）
│   ├── selection/             # 剪贴板与选区辅助、平台坐标
│   ├── events/                # 13 个应用事件常量与 payload
│   ├── model/                 # 共享模型、语言/主题/窗口常量
│   ├── i18n/                  # 后端 i18n（locales/split + 合并产物）
│   ├── logutil/               # 运行日志滚动/压缩/清理
│   ├── network/               # 代理 / 自定义 DNS 的 HTTP client
│   ├── useragent/             # 全局 UA 注入
│   ├── webview/               # Windows WebView2 参数兜底
│   ├── analytics/             # PostHog 匿名统计
│   ├── singleinstance/        # 单实例锁
│   └── buildinfo/             # 构建期变量、数据目录规则
├── pkg/
│   ├── swiftbridge/           # macOS Swift 桥接（开发用 dylib / 打包内嵌）
│   └── wails-updater-providers/ # GitHub / CNB 双更新源 + 内置更新窗口
└── frontend/
    ├── package.json           # Svelte 5 + TS7 + Tailwind4 + Vite8 + Vitest5
    ├── vite.config.ts         # 多入口 + wails 插件 + 条件 Codecov 插件
    ├── vitest.config.ts       # 独立测试配置（不 merge vite.config）
    ├── bindings/              # wails3 生成，随仓库提交
    └── src/
        ├── components/        # TranslateWindow / ScreenshotWindow / Settings + settings/*Tab
        ├── windows/           # 各 HTML 入口的 mount 代码
        ├── stores/            # theme / ui / persisted
        ├── runtime/           # 平台判断、前端日志
        ├── utils/             # events / analytics / style
        ├── i18n/              # zh-CN / en-US
        └── test/              # Vitest setup
```

---

## 4. 功能模块

### 4.1 翻译引擎（`internal/engine`）

- 统一接口：`Translator.Name/Translate`、`OcrEngine.Name/Recognize`；`Registry` 按配置动态注册。
- 已知引擎：`apple`、`google`、`deepl`、`openai`、`anthropic`、`gemini`、`baidu`、`tencent`、`youdao`；OCR：`vision`、`tesseract`。
- 默认启用：`google`（全平台）、`apple`（仅 macOS）、`vision`（仅 macOS）；其余需用户在设置页添加并填 Key。
- 配置存 `config.db` 的 `engines` 表（API Key / Secret 加密），字段 schema 由 `engine.GetEngineSchema` 提供，前端据此渲染表单。
- 多引擎并行：`TranslateMulti` 启动后通过 `kai:translate:result` 事件逐个推送结果。

### 4.2 OCR 与截图翻译

- OCR 单选：同一时刻只注册一个 OCR 引擎，优先用户启用的 `tesseract`，否则 `vision`。
- `vision` 走 macOS Vision 框架（离线、免安装），参数 `correct_text` / `timeout_sec` 存 `Extra` JSON。
- `tesseract` 调本机 `tesseract` 可执行文件，需用户自行安装。
- 截图翻译流程：快捷键 → `CaptureRegion`（macOS `screencapture -i`）→ OCR → 多引擎翻译 → `kai:screenshot:ocr` 投递截图窗口；改语言走 `kai:screenshot:retranslate` 复用 OCR 原文。
- Windows 无区域截图实现，截图翻译不可用。

### 4.3 取词（复制键，不是划词浮窗）

- 历史方案（AX 读取 + `SelectionPopup` 浮窗）已移除；现方案为**执行键模拟复制**：
  - macOS：`robotgo.KeyTap("c","cmd")`（CGO）；
  - Windows：`makc` 调 `user32.dll`（纯 Go，保持 `CGO_ENABLED=0`）；
  - 复制失败可按配置回退系统默认复制键（`execkeys.copy.fallback`）。
- 读取剪贴板后经 `kai:input:fill` 回填输入框并翻译。
- 自动剪贴板模式（`auto_clipboard`）下不模拟复制，直接读剪贴板；开启时复制键自动停用并快照原状态。

### 4.4 快捷键（`internal/hotkey`）

- 注册类：`input`（默认 `Alt+A`，启用）、`screenshot`（默认 `Alt+S`，关闭）；通过 `app.GlobalShortcut` 注册。
- 执行类：`execkeys.copy`（默认 `Cmd+C` / `Ctrl+C`，启用），不注册监听，由程序主动模拟。
- 配置变更后重新注册并广播 `kai:hotkeys:changed`；托盘菜单按启用状态置灰。

### 4.5 窗口与事件

- 常驻窗口：`settings`（设置，主窗口）、`translate`（输入翻译）、`screenshot`（截图翻译）；另有 updater 窗口由 `pkg/wails-updater-providers` 管理。
- 全部使用**原生标题栏**；红 X = `Hide()` 不销毁，并在 `WindowClosing` 钩子里广播 `kai:window:closing` 让前端清空状态。
- 事件（`internal/events/events.go`，13 个）：窗口显隐/关闭、语言/主题/快捷键/引擎变更、输入回填、多引擎结果、截图 OCR / 重截 / 重翻、自动剪贴板变更。
- 前端对应常量在 `frontend/src/utils/events.ts`，两端保持一致。

### 4.6 配置、存储与日志

| 内容          | 位置                                        | 说明                                                            |
|---------------|---------------------------------------------|-----------------------------------------------------------------|
| 界面/功能配置 | `~/.kai/settings.json`（开发 `~/.kai.dev`） | viper 热重载；字段见 `internal/settings`                        |
| 引擎配置      | `~/.kai/data/config.db`                     | 凭据由机器指纹派生密钥加密（`configstore/crypto.go`）           |
| 翻译历史      | `~/.kai/data/history.db`                    | 去重写入，支持搜索/删除/清空                                    |
| HTTP 日志     | `~/.kai/data/httplog.db`                    | append-only，按保留天数清理                                     |
| 运行日志      | `~/.kai/logs/`                              | `kai.log` / `frontend.log` / `kai-bridge.log`，按天滚动、可压缩 |

默认值：界面语言/主题 `auto`、目标语言 `zh`、源语言 `auto`、预发布通道开、匿名统计关、日志 `info` 保留 30 天并压缩、HTTP 日志开、代理关。

### 4.7 自动更新

- `pkg/wails-updater-providers` 提供 GitHub / CNB 双 provider，按界面语言选源（中文 → CNB，英文 → GitHub），可手动指定。
- 预发布通道默认开启；stable 用 tag 比较，预发布用构建时间/commit 比较。
- 启动静默检查 + 托盘「检查更新」；内置更新窗口，下载完成后重启生效。

### 4.8 国际化与主题

- 后端：go-i18n/v2，`internal/i18n/locales/split/<locale>/*.json` 为源，`make i18n-go` 合并。
- 前端：`zh-CN.ts` / `en-US.ts`，split 文件由 `make i18n-frontend` 合并。
- 严格区分 `Locale`（界面语言 `auto/zh-CN/en-US`）与 `Language`（翻译语言 `auto/zh/en/...`），不可混用。
- 主题 `auto/light/dark`，auto 跟随系统；窗口原生外观在启动时设置，运行时由前端 theme store 尝试同步。
- 语言/主题变更分别广播 `kai:locale:changed` / `kai:theme:changed`，托盘菜单随之重建。

### 4.9 匿名统计（PostHog）

- 默认关闭，opt-in；未配置 token 或 dev 构建自动 no-op。
- 事件：`app_installed` / `app_started` / `translate_input` / `translate_screenshot` / `settings_opened` / `feature_toggled` / `error_occurred`。
- 详细字段与隐私边界见 `README.md` §2。

---

## 5. 里程碑与现状

| 阶段             | 状态 | 说明                                                                                   |
|------------------|------|----------------------------------------------------------------------------------------|
| M1 脚手架 & 基建 | ✅   | 多窗口 + 托盘 + 菜单；buildinfo/model/engine/service 骨架；i18n/主题/语言 auto         |
| M2 翻译主流程    | ✅   | Google/DeepL/OpenAI 等引擎；多引擎并排；历史库（sqlc）                                 |
| M3 取词          | ✅   | 原划词浮窗已下线，改为复制键 + 自动剪贴板方案                                          |
| M4 截图 OCR      | ✅   | macOS 区域截图 + Vision/tesseract；截图窗口左图右译                                    |
| M5 快捷键 & 设置 | ✅   | 注册键/执行键拆分；设置页四个 Tab；引擎 schema 化                                      |
| M6 跨平台 & 打包 | ✅   | macOS / Windows 双平台打包（dmg / nsis / msix）；GitHub Release                        |
| 发布后迭代       | ✅   | 双源自动更新、单实例、日志/HTTP 日志、匿名统计、CNB / GitLab / Gitee 镜像同步、CI 体系 |

当前发版：GitHub Release 触发后同步到 CNB / GitLab / Gitee；Gitea / GitCode 仅作代码镜像，不发布 Release。每日 Nightly 已取消，预发布改为手动 `prerelease`。

---

## 6. 环境准备与常用命令

### 6.1 数据目录

- 正式版 `~/.kai/`，开发版 `~/.kai.dev/`，由 `buildinfo.Dev` 切换。
- `data/` 放三个 SQLite；`logs/` 放运行日志。

### 6.2 构建期变量

`internal/buildinfo/buildinfo.go` 暴露可被 `-ldflags -X` 覆盖的变量：

```go
var (
    Version   = "dev"      // 版本号
    BuildTime = "unknown"  // 打包时间
    GitCommit = ""         // 提交短哈希
    Dev       = "true"     // 开发模式：~/.kai.dev
)
```

CI 通过 `-ldflags "-X cnb.cool/dtapp/kai/internal/buildinfo.Version=..."` 注入；本地 `make dev` 默认 `Dev=true`。

### 6.3 常用命令

```bash
# 初始化（Go/前端依赖 + bindings + sqlc）
make setup

# 开发（热重载，走 ~/.kai.dev）
make dev

# 代码生成
make bindings      # Wails TS 绑定
make sqlc          # configstore / historystore / httplogstore
make i18n          # 合并 split i18n
make swift-build   # 编译 Swift 桥接 dylib（仅 macOS）

# 检查与测试
make format
make check         # lint-go + lint-frontend + test-go + fuzz-go + vuln-go
make check-cross   # 交叉编译验证（darwin/arm64 + windows/CGO=0）

# 打包
make darwin-package
make windows-package
```

> Wails 3 仍处 Beta，API 可能变动，以 https://v3.wails.io 为准；升级 Wails 版本时同步改 `go.mod` 与两份 CI Dockerfile 的 `WAILS3_VERSION`。

---

## 7. 当前约束与已知问题

- **Windows WebView2 竞态**：窗口统一「创建后立即 `Hide()`」，避免 `Hidden: true` 延迟创建 controller 触发 `80010108`；同时注入 `--disable-gpu*` 兜底参数。
- **macOS 首次唤起截图窗口可能被遮挡**：accessory 应用从后台抢焦点的平台限制；再次触发或点托盘图标可恢复。
- **截图翻译仅 macOS**：`CaptureRegion` 只实现 darwin；Windows 调用返回 `ErrNoScreenshot`。
- **Swift 桥接**：开发态需本地 `pkg/swiftbridge/libkai_bridge.dylib`（`make swift-build`）；打包态从内嵌字节落地临时文件后 `Dlopen`。
- **CGO / 平台构建**：macOS 依赖 CGO（robotgo、mattn sqlite 可选、Swift 桥）；Windows 保持 `CGO_ENABLED=0`（makc + modernc sqlite）。
- **SQLite 驱动切换**：默认 `modernc.org/sqlite`（纯 Go）；`-tags sqlite_mattn` 切到 `mattn/go-sqlite3`（CGO），DSN 语法不同，见 `internal/sqlite`。
- **主程序构建用 `go build .`**：`go build ./...` 会误扫 `build/ios` 脚手架脚本；CI 检查时需要先建 `frontend/dist` 占位目录满足 `//go:embed`。
- **前端测试**：`frontend/vitest.config.ts` 不 merge `vite.config.ts`（wails 插件依赖构建期 bindings）；`@bindings` 指向 `frontend/bindings`。
- **pnpm peer 规则**：`frontend/.npmrc` 显式放行 `@codecov/vite-plugin` 对 Vite 8 的 peer。

---

## 8. 文档与历史

| 文档                                            | 内容                                        |
|-------------------------------------------------|---------------------------------------------|
| `README.md` / `README_EN.md`                    | 用户向介绍、功能、平台、隐私、发布          |
| `docs/ci-image.md`                              | CNB / GitHub 两份 CI Dockerfile 与镜像分发  |
| `docs/codecov.md`                               | Codecov 覆盖率 / bundle / 测试结果          |
| `docs/coveralls.md`                             | Coveralls 双 flag 上传                      |
| `docs/gitlab-ci.md`                             | GitLab 发布同步（原覆盖率方案已下线）       |
| `docs/coverage-go.md` / `docs/coverage-node.md` | 覆盖率实现模板与当前状态                    |
| `docs/docs-plan.md`                             | 文档缺口分析与后续补写计划                  |
| `docs/2026-*.md`                                | 2026-08 ~ 09 开发日志（历史快照，不再更新） |
