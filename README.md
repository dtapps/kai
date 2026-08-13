# Kai - 跨平台翻译 & OCR 工具

> 参考 [Bob](https://github.com/ripperhe/Bob) 与 [Easydict](https://github.com/tisfeng/Easydict)，打造一款 macOS 为主、兼容 Windows 的划词/截图/输入翻译 + OCR 桌面工具。

[English](./README.en.md)

## 1. 产品定位

常驻系统托盘的菜单栏式工具，提供三类核心能力：

| 能力 | 触发方式 | 说明 |
|------|----------|------|
| 输入翻译 | 全局快捷键呼出窗口（mac: `⌥+A` / win: `Alt+A`） | 手动输入文本翻译 |
| 划词翻译 | 选中文本 + 快捷键（`⌥+D` / `Alt+D`），或鼠标悬停自动弹窗 | 自动识别语种 |
| 截图翻译 | 快捷键截图区域（`⌥+S` / `Alt+S`） | 区域 OCR + 翻译 |
| 静默 OCR | 快捷键（`⌥+⇧+S` / `Alt+Shift+S`） | 截图识别后写入剪贴板 |

## 2. 技术栈

- **框架**：[Wails 3](https://v3.wails.io/)（Beta，Go 后端 + 原生 WebView）
- **后端**：Go 1.22+，采用 Wails 3 `application` 管理器风格 API
  - 多窗口：`translate`（主翻译窗口）/ `settings`（设置）/ `selection`（划词浮窗）/ `screenshot`（截图翻译）
  - `app.SystemTray` 系统托盘、`app.Menu` 原生菜单
  - `app.Event` 内存 IPC（无端口）、`app.Service` 暴露 Go 方法给前端
  - `updater` 自动更新（CNB / GitHub 双源，按界面语言选源）
- **前端**：Svelte 5（runes）+ TypeScript + Tailwind CSS v4 + Vite 6 多入口
- **包管理**：前端 pnpm，Go modules
- **本地存储**：SQLite（`sqlc` 生成，`internal/{historystore,configstore,httplogstore}`），配置与引擎凭据分离

## 3. 目录结构

```
kai/
├── main.go                      # 应用入口：app 初始化、窗口、托盘、菜单、事件、更新器
├── go.mod
├── build/                       # Wails 构建配置与图标资源
│   ├── config.yml
│   └── darwin/                  # macOS Info.plist / dmg 资源
├── Taskfile.yml                 # 构建打包任务（task darwin:package 等）
├── Makefile                     # 开发 / 检查 / 打包封装（make dev / make darwin-package）
├── internal/
│   ├── service/                 # 启动编排门面 + 各能力 Wrapper（AppService/EngineWrapper/...)
│   ├── engine/                  # 翻译引擎（deepl/google/openai/baidu/tencent/youdao）
│   │                            #   + OCR（tesseract / macOS Vision.framework）
│   ├── translate/               # 翻译领域服务（多引擎调度）
│   ├── selection/               # 划词监听（mac Accessibility / 剪贴板）
│   ├── execkey/                 # 执行键（复制键等一键动作）
│   ├── hotkey/                  # 全局快捷键注册（mac/win）
│   ├── configstore/             # SQLite 引擎配置库（凭据 AES-256-GCM 加密）
│   ├── historystore/            # SQLite 翻译历史库
│   ├── httplogstore/            # SQLite HTTP 抓包日志库
│   ├── settings/                # UI 偏好（语言/主题/快捷键/TTS）
│   ├── i18n/                    # Go 端 i18n（zh-CN / en-US）
│   ├── events/                  # 跨端事件常量与 payload
│   ├── updater/                 # CNB / GitHub 双源更新 Provider
│   ├── swiftbridge/             # Swift 桥接静态库（Translation/Vision/辅助功能）
│   ├── network/                 # 全局 HTTP 客户端（UA/代理/DNS 注入）
│   ├── buildinfo/               # 版本/构建时间/Token（ldflags 注入）
│   ├── model/                   # 共享数据结构
│   └── ...
├── frontend/
│   ├── package.json
│   ├── vite.config.ts
│   ├── translate.html / settings.html / selection.html / screenshot.html
│   └── src/
│       ├── windows/             # 各窗口入口：translate/settings/selection/screenshot
│       ├── components/          # 复用 UI 组件（Svelte 5）
│       ├── stores/              # 状态管理（runes）
│       ├── i18n/                # 前端 i18n（zh / en）
│       ├── runtime/             # Wails runtime 封装（事件监听/通知）
│       ├── utils/               # 工具（events 常量等）
│       └── app.css              # Tailwind v4 + 语义 CSS 变量
└── README.md
```

## 4. 功能模块规划

### 4.1 翻译引擎（engine/）
可配置 API Key 的翻译引擎：
- DeepL、Google、OpenAI（含兼容大模型）、百度、腾讯、有道
- 统一接口 `Translate(ctx, req) (*TranslateResult, error)`
- 支持自动语种识别、多引擎结果并排展示

### 4.2 OCR（engine + swiftbridge）
- 截图区域捕获（mac 用 `screencapture` 系统命令，归类「屏幕录制」TCC 权限）
- OCR 引擎：macOS Vision.framework（系统离线 OCR，零安装）、Tesseract（本机依赖）
- 静默模式：识别结果写入剪贴板

### 4.3 划词（selection/）
- mac：监听系统选区（Accessibility 授权 + 剪贴板轮询）
- 浮窗：靠近选区位置弹出翻译卡片

### 4.4 快捷键（hotkey/ + execkey/）
- 后端全局钩子，失焦可用
- 区分「注册键」（RegisteredHotkey）与「执行键」（ExecKey，如复制键）

### 4.5 TTS
- 系统语音（mac `say`） + 在线 TTS

### 4.6 自动更新（updater/）
- 启动静默检查 + 托盘菜单「检查更新」
- CNB / GitHub 双源，按界面语言单一固定选源（英文走 GitHub，中文走 CNB）
- 支持 nightly 预发布通道（需开启设置项）

## 5. 开发计划（里程碑）

1. **M1 脚手架**：Wails3 + Svelte5 + Tailwind v4 跑通，主窗口 + 托盘 + 设置窗口
2. **M2 翻译主流程**：输入翻译 + 多引擎接入 + 结果展示
3. **M3 划词浮窗**：选区监听 + 浮窗翻译（mac 优先）
4. **M4 截图 OCR**：区域截图 + OCR + 静默 OCR 到剪贴板
5. **M5 快捷键 & 设置**：全局快捷键、服务配置持久化、TTS
6. **M6 跨平台 & 打包**：Windows 适配、签名与安装包（dmg / nsis/msi）

## 6. 环境准备

```bash
# 1. 安装 Wails 3 CLI
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
# 校验
wails3 doctor

# 2. 安装 Go 工具链依赖（sqlc / golangci-lint / govulncheck 等）
make tool-deps

# 3. 安装前端依赖
make install        # 等价 cd frontend && pnpm install

# 4. 完整初始化（依赖 + 绑定 + sqlc）
make setup

# 5. 开发运行（热重载，端口固定 9247）
make dev
```

> 注：Wails 3 目前处于 Beta，API 可能变动，以 `v3.wails.io` 最新文档为准。

## 7. 构建与打包

```bash
# [macOS] 编译正式二进制 -> bin/Kai（不打包 .app）
make darwin-build

# [macOS] 正式打包 -> bin/Kai.app
make darwin-package

# [macOS] 打包并生成 bin/Kai.dmg 安装包
make darwin-package-dmg

# 通过同名变量覆盖版本与 Token（CI/手动）：
make darwin-package VERSION=1.2.0 BUILD_TIME=2026-08-12T00:00:00Z
```

版本与构建时间一律由传入变量透传（`build/{darwin,linux,windows}/Taskfile.yml` 的 `VERSION`/`BUILD_TIME`），不写本地回退，调用方必须显式传参。

## 8. 代码规范要点

- **编译验证**：只用 `go build ./...` 检查，禁止 `go build .`（避免根目录生成裸二进制 `kai`）。
- **UI 文案 i18n**：任何用户可见文案必须走 `t()`，禁止硬编码中文/英文。
- **前端 Runtime**：统一 `import { Events, Window } from '@wailsio/runtime'`，事件名走 `frontend/src/utils/events.ts` 常量。
- **存储分层**：引擎配置（凭据加密）与 UI 偏好（`settings.json`）严格分离，禁止混用。
- **i18n 拆分合并**：`internal/i18n/locales/split/**` 为源，跑 `make i18n` 合并到主文件。
- **Swift 桥接**：改动 `internal/swiftbridge/*.swift` 后必须跑 `make swift-build` 重编静态库。
