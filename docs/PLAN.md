# Kai 项目规划文档

> 参考 [Bob](https://github.com/ripperhe/Bob) 与 [Easydict](https://github.com/tisfeng/Easydict)
> 一款 macOS 为主、兼容 Windows 的划词 / 截图 / 输入翻译 + OCR 桌面工具。
> 技术栈：**Wails 3 (Go) + Vue 3 + TypeScript 7 + Naive UI**

---

## 1. 产品定位

常驻系统托盘（菜单栏）的轻量工具，开机自启、失焦可用，提供四类核心能力：

| 能力 | 触发方式（mac / win） | 说明 |
|------|----------|------|
| 输入翻译 | 全局快捷键呼窗 `⌥+A` / `Alt+A` | 手动输入文本翻译 |
| 划词翻译 | 选中 + 快捷键 `⌥+D` / `Alt+D`，或鼠标悬停自动弹窗 | 自动识别语种 |
| 截图翻译 | 快捷键截图区域 `⌥+S` / `Alt+S` | 区域 OCR + 翻译 |
| 静默 OCR | 快捷键 `⌥+⇧+S` / `Alt+Shift+S` | 截图识别后写入剪贴板 |

参考对象能力对照：
- **Bob**：划词/截图/输入翻译、OCR（离线/腾讯/百度/Google）、TTS、驼峰拆分、AppleScript/PopClip 调用、自定义插件。
- **Easydict**：20+ 翻译服务（OpenAI/Gemini/DeepL/Google/有道/腾讯/Bing/百度…）、48 种语言、OCR 截图、安静截图 OCR、TTS、鼠标划词 + 快捷键划词。

---

## 2. 技术选型

| 层 | 选型 | 说明 |
|----|------|------|
| 框架 | **Wails 3**（Beta） | Go 后端 + 原生 WebView（mac: WKWebView / win: WebView2），无内嵌 Chromium |
| 后端 | Go（module `cnb.cool/dtapp/kai`） | `application` 包管理器风格 API |
| 前端 | **Vue 3 + TypeScript 7 + Naive UI** | SPA，bindings 自动生成 |
| 状态/路由 | Pinia + vue-router | 配置状态 / 多窗口路由 |
| 包管理 | **pnpm**（前端）、Go modules（后端） | |
| 构建 | `wails3 dev` / `wails3 build` | 热重载 + 类型安全 IPC |

### Wails 3 关键 API（设计依据）
- 应用入口：`application.New(application.Options{...})` → `app.Run()`
- 多窗口：`app.Window.NewWithOptions(application.WebviewWindowOptions{...})`，支持 `Show/Hide/Focus/Center/RegisterHook`
- 系统托盘：`app.SystemTray.New().SetIcon(...).SetMenu(...).AttachWindow(w)`
- 原生菜单：`app.Menu.New()` → `AddSubmenu` / `Add` / `SetAccelerator` / `OnClick`
- 事件 IPC：`app.Event.On(name, fn)` / `app.Event.Emit(name, data)`（内存通信，无端口）
- 服务暴露：`application.NewService(&MyService{})` 注入 `Services`，前端经 `frontend/bindings/` 调用
- 资源：`//go:embed all:frontend/dist` + `application.AssetFileServerFS(assets)`

---

## 3. 目录结构

```
kai/
├── main.go                      # app 初始化、主窗/设置窗、托盘、菜单、事件
├── go.mod                       # module cnb.cool/dtapp/kai
├── build/                       # Wails 构建配置（config.yml / darwin / windows ...）
├── Taskfile.yml                 # dev/build 任务
├── PLAN.md                      # 本规划文档
├── README.md
├── internal/
│   ├── service/
│   │   ├── app.go               # AppService：暴露给前端的统一入口
│   │   ├── engines.go           # 按配置注册启用引擎
│   │   ├── shortcut.go          # 全局快捷键注册（mac/win）
│   │   ├── selection.go         # 划词监听
│   │   ├── screenshot.go        # 截图模块
│   │   ├── ocr.go               # OCR 调度
│   │   └── tts.go               # 语音合成
│   ├── engine/                  # 翻译/OCR 引擎实现（统一接口）
│   │   ├── engine.go            # Translator / OcrEngine 接口 + Registry
│   │   ├── deepl.go
│   │   ├── google.go
│   │   ├── openai.go
│   │   ├── baidu.go
│   │   ├── tencent.go
│   │   └── youdao.go
│   ├── config/
│   │   └── config.go            # JSON 配置（~/.kai/config.json）
│   ├── i18n/
│   │   └── i18n.go              # 后端 i18n（zh/en 映射 + 按 config.Language 取文案）
│   ├── historystore/
│   │   ├── db.go                # sqlc 生成的 DB 访问代码（SQLite）
│   │   ├── query.sql            # sqlc 查询定义
│   │   └── sqlc.yaml            # sqlc 配置
│   ├── configstore/
│   │   ├── db.go                # 配置类数据 SQLite 存储（引擎等）
│   │   ├── query.sql            # sqlc 查询定义
│   │   └── sqlc.yaml            # sqlc 配置
│   └── system/                  # 系统级能力封装
│       ├── translate_darwin.go  # macOS Translation framework（CGO 桥接 Swift/ObjC）
│       ├── detect_darwin.go     # macOS 语种识别（LanguageIdentification）
│       └── translate_windows.go # Windows：无系统翻译，回退在线引擎
│   └── model/
│       └── model.go             # TranslateRequest/Result、OcrRequest/Result、HistoryItem 等
└── frontend/
    ├── package.json             # TS7 + naive-ui + pinia + vue-router + vite
    ├── vite.config.ts
    ├── translate.html
    ├── tsconfig.json
    └── src/
        ├── main.ts              # 接入 pinia + naive-ui + router + i18n
        ├── App.vue
        ├── bindings/            # wails3 自动生成（勿手改）
        ├── i18n/
        │   ├── index.ts         # vue-i18n 初始化（locale 取自后端 GetConfig 或系统）
        │   ├── en.ts            # 英文文案
        │   └── zh.ts            # 中文文案
        ├── router/index.ts      # 主窗/设置窗 路由
        ├── views/
        │   ├── TranslateWindow.vue   # 主翻译窗口
        │   ├── SelectionPopup.vue    # 划词浮窗
        │   └── Settings.vue          # 设置窗口
        ├── composables/
        │   ├── useTranslate.ts
        │   └── useHotkey.ts
        ├── stores/
        │   └── config.ts        # Pinia 配置状态
        └── styles/
```

---

## 4. 功能模块设计

### 4.1 翻译引擎（engine/）
- 统一接口：
  - `Translator`: `Name() string` + `Translate(ctx, req) (*TranslateResult, error)`
  - `OcrEngine`: `Name() string` + `Recognize(ctx, req) (*OcrResult, error)`
- 首版接入：DeepL、Google、OpenAI（兼容大模型）、百度、腾讯、有道
- 能力：自动语种识别、多引擎结果并排展示、`Registry` 按配置动态注册
- 请求/结果模型（`internal/model/model.go`）：`TranslateRequest/Result`、`OcrRequest/Result`、`DictItem`、`OcrRegion`

### 4.2 OCR（ocr.go + engine）
- 区域截图捕获：mac 用 `screencapture` / 自建选区；win 用 `PrintWindow` + 选区
- OCR 引擎：腾讯 / 百度 / Google OCR，后续可接离线
- 静默模式：识别结果直接写入系统剪贴板

### 4.3 划词（selection.go）
- mac：Accessibility 权限 + 选区监听（或轮询 `pbpaste` / CGEvent 兜底）
- win：UI Automation 取选中文本
- 浮窗：贴近选区坐标弹出 Naive UI 卡片

### 4.4 快捷键（shortcut.go）
- 优先后端全局钩子（失焦可用）：mac 走 CGO/Carbon 或成熟库；win 注册系统热键
- 配置化：键位存于 `Config.Hotkeys`，前端可编辑

### 4.5 TTS（tts.go）
- 系统语音（mac `say` / win SAPI）+ 在线 TTS（火山 / 腾讯 / Google）

### 4.6 配置（config/）
- 数据目录（按构建模式切换，由 `version` 包变量 `Dev` 决定）：
  - **正式版**：`~/.kai/`（config.json、history.db 等）
  - **开发版**：`~/.kai.dev/`（避免污染正式数据，便于联调）
- 字段：默认引擎、默认目标语言、各引擎凭证（APIKey/Secret/Endpoint）、快捷键、TTS 设置
- 前端 `GetConfig / SaveConfig` 读写并热更新引擎注册

### 4.7 翻译历史（historystore/）
- 存储：`~/.kai/data/history.db`（SQLite 单文件）
- **DB 交互使用 [sqlc](https://sqlc.dev/)**：在 `internal/historystore/sqlc.yaml` 配置 SQLite 方言，`query.sql` 定义 `insert_history` / `query_history` / `delete_history` / `clear_history` 等查询，由 `sqlc generate` 产出类型安全 Go 代码（`db.go`），业务层只调用生成的方法，不手写 SQL 拼接
- 记录字段（见 `model.HistoryItem`）：原文、译文、源语言、目标语言、引擎名、时间戳、OCR 来源标记
- 能力：
  - 每次翻译/OCR 完成后自动入库（去重：相同原文+引擎+方向不重复记）
  - 前端历史面板：列表、搜索（按原文/译文/语言）、按时间倒序、分页加载
  - 点击历史项可回填到翻译框重新翻译
  - 支持单条删除 / 清空全部
  - 上限保护（如保留最近 5000 条，超出按时间淘汰），避免无限增长
- 对外接口（AppService 暴露给前端）：`AddHistory` / `QueryHistory(keyword, offset, limit)` / `DeleteHistory(id)` / `ClearHistory`

### 4.8 系统级翻译能力（system/）
不同平台系统是否提供翻译接口，方案不同：

- **macOS：提供系统翻译**
  - 框架：**Translation framework**（macOS 14 Sonoma+），可应用内翻译，离线、无需 API Key
  - 语种识别：**NaturalLanguage `LanguageIdentification`**（或 Translation 自带识别）
  - 接入方式：Go 通过 CGO 桥接 Swift/Objective-C（编译期链接 `Translation.framework` / `NaturalLanguage.framework`），封装为 `system.TranslateDarwin` / `system.DetectDarwin`
  - 注意：首次使用需下载语言模型 + 系统授权弹窗；应作为可选引擎「系统翻译（macOS）」出现在引擎列表，默认启用（mac 用户零配置即用）
- **Windows：无系统翻译 API**
  - Windows 没有类似 macOS Translation 的公开离线系统翻译接口（剪贴板/分享用的 `Windows.ApplicationModel.DataTransfer` 不是翻译）
  - 方案：Windows 下「系统翻译」引擎不存在，自动隐藏该选项；翻译走在线引擎（Google/DeepL/OpenAI/腾讯/百度/有道）
  - 语种识别：Windows 可用 `Windows.Globalization` 的 `LanguageRecognizer`（WinRT），同样经 CGO/WinRT 桥接，作为可选能力
- **统一处理**：`system` 包按 `runtime.GOOS` 编译不同文件（build tag `darwin` / `windows`），对外暴露统一 `SystemTranslator` 接口；不可用时引擎列表自动剔除，前端无需感知平台差异

### 4.9 国际化（i18n，前端 + 后端）
前后端均需支持多语言（至少 **中文 / 英文**），翻译工具本身面向多语言用户，i18n 是刚需。

- **前端（vue-i18n）**
  - 依赖：`vue-i18n`（与 Vue3 配套）
  - 文案文件：`src/i18n/zh.ts`、`src/i18n/en.ts`，由 `src/i18n/index.ts` 初始化
  - 所有 UI 文案（按钮、菜单、设置项、提示、空状态）走 `t('key')`，**不硬编码字符串**
  - 当前语言来源：`GetConfig().Language`；**支持 `auto`（见 4.11），非 auto 时直接用配置值**，可在设置里切换并持久化
  - 与 Naive UI 集成：Naive UI 自身有 `n-config-provider` 的 locale，需与 vue-i18n 的 locale 联动（如中文用 `zhCN`、英文用 `enUS`）
- **后端（Go，使用 go-i18n/v2）**
  - 范围：对外返回的错误信息、状态提示、引擎/语言显示名等需可本地化
  - 方案：`internal/i18n` 包，使用 **`github.com/nicksnyder/go-i18n/v2`** 管理多语言文案；文案文件 `active.en.json`（source/fallback）/ `active.zh.json` 经 `//go:embed` 内嵌，构建后独立运行无需外部文件
  - 对外函数保持 `T(lang, key)` / `EngineName(lang, engine)` / `LanguageName(lang, code)`，未知语言回退英文 source，再缺省返回 key
  - 该库原生支持 ICU MessageFormat 与复数规则，后续如需「N 条记录」类本地化文案可直接用，无需自行拼装
  - 错误包装：业务错误（如 `ErrNoEngine`、`ErrAPIKey`）返回时附带 i18n key，前端按当前 locale 渲染，避免后端硬编码中文
  - 引擎/语言列表的「展示名」由后端提供（如 `google → "Google"`、`ZH → "中文"`），前端直接用，减少前端维护成本
- **语言枚举**：中（zh）/ 英（en）为首批；架构预留扩展（ja/ko 等），新增只需补两份文案 + 映射

### 4.10 主题（Theme，含自动）
UI 主题需支持切换，并**默认「自动」跟随系统外观**（亮/暗），与 Bob/Easydict 一致。

- **主题枚举**：`auto`（默认）/ `light` / `dark`
- **自动模式（auto）实现**：
  - 前端经 Wails 暴露的接口读取**系统外观**（mac 用 `window.matchMedia('(prefers-color-scheme: dark)')`；Wails3 WebView 内可直接用 `matchMedia`，无需后端桥接；如需后端兜底可加 `system.Appearance()` CGO 取 `NSAppearance`）
  - 监听系统外观变化：`matchMedia(...).addEventListener('change', ...)`，切到暗色则前端切 `dark` 主题、亮色切 `light`
  - `auto` 下不写死主题，实时跟随系统；用户切到 `light`/`dark` 后停止跟随
- **与 Naive UI 集成**：`n-config-provider` 的 `theme` 属性在 dark 时传 `darkTheme`、light 时传 `lightTheme`；同时 `n-config-provider` 的 `theme-overrides` 提供 Kai 品牌色（主色等）
- **持久化**：`Config.Theme` 存于 `config.json`，默认 `auto`；前端 `SaveConfig` 写回，重启后恢复。AppService 新增 `GetTheme()/SetTheme()`（或直接复用 GetConfig/SaveConfig）
- **托盘/窗口外观**：深色模式下窗口背景、托盘图标（可准备 light/dark 两套 `ic_template`/`ic_mask`）随主题切换

### 4.11 语言（Language，含自动）
界面语言同样支持「自动」，**跟随系统语言**，与主题自动模式对称。

- **语言枚举**：`auto`（默认）/ `zh` / `en`（架构预留 ja/ko 等）
- **自动模式（auto）实现**：
  - 前端首次启动（配置为 `auto`）：取 `navigator.language`（或 `window.language`）→ 映射 `zh-CN/zh / → zh`，`en-* → en`，其余回退 `en`
  - 后端兜底：AppService 提供 `DetectSystemLang()`，经 `runtime.GOOS`/`os` 环境变量（`LANG`/`LC_ALL` 或 mac 的 `defaults read -g AppleLocale`）推断，供前端在 WebView 不可用时的回退
  - `auto` 下实际生效语言（resolved locale）由前端解析后用于 vue-i18n 的 `locale` + Naive UI 的 `n-config-provider` locale；用户手动选 `zh`/`en` 后停止跟随
- **持久化**：`Config.Language` 存于 `config.json`，默认 `auto`；切换后 `SaveConfig` 写回。`auto` 与具体 `zh/en` 均视为合法值，后端 `i18n.T` 在收到 `auto` 时应回退到推断值（或前端解析后传确定值给后端）
- **与主题联动**：主题 auto 跟随系统外观、语言 auto 跟随系统语言，二者独立配置、互不干扰

---

## 5. 开发里程碑（M1–M6）

| 阶段 | 目标 | 关键任务 |
|------|------|----------|
| **M1 脚手架 & 基建** | 跑通 Wails3+Vue3+Naive UI | ①`pnpm install` ②`main.go` 改多窗口+托盘+菜单 ③`AppService` 替换示例 Service ④前端 `main.ts` 接 pinia+naive-ui+router ⑤`wails3 dev` 验证空壳可运行 |
| **M2 翻译主流程** | 输入翻译闭环 | ①`model`/`config`/`engine`/`history` 接口 ②接 Google（免 key）验证链路 ③叠加 DeepL/OpenAI（带 key）④前端 `TranslateWindow.vue` + `useTranslate` 结果展示 ⑤翻译完成自动写历史 |
| **M3 划词浮窗** | 选中即译 | ①`selection.go` 选区监听（mac 优先）②`SelectionPopup.vue` 浮窗 ③贴近选区坐标弹出 |
| **M4 截图 OCR** | 截图识别 | ①`screenshot.go` 区域截图 ②`ocr.go` + OCR 引擎 ③静默 OCR 写剪贴板 |
| **M5 快捷键 & 设置** | 体验完善 | ①`shortcut.go` 全局快捷键 ②`Settings.vue` 引擎配置持久化 ③`tts.go` 语音合成 |
| **M6 跨平台 & 打包** | 发布 | ①Windows 适配 ②签名 ③dmg / msi 安装包 |

> 引擎接入优先级建议：M2 先通 **Google（免 key）** 验证链路，再叠加 **DeepL / OpenAI**；百度/腾讯/有道随配置可用。

---

## 6. 待确认事项（开工前）

1. 首版是否只做 **macOS**，Windows 放到 M6 再适配？（或脚手架即双平台）
2. M2 引擎接入顺序偏好（Google 先通 vs DeepL/OpenAI 优先）
3. 划词首版方案：接受「选中 + 快捷键 `Alt+D`」先跑通，还是直接做「鼠标悬停自动弹窗」（需 Accessibility 授权，较重）

---

## 7. 环境准备 & 常用命令

### 7.1 数据目录规则
- 正式版使用 `~/.kai/`，开发版使用 `~/.kai.dev/`，由编译期变量切换（见 7.3）。

### 7.2 版本与打包信息注入
- 在 `internal/buildinfo/version.go` 定义可被 `-ldflags -X` 覆盖的变量：
  ```go
  package buildinfo
  var (
      Version   = "dev"        // 版本号
      BuildTime = "unknown"    // 打包时间
      Dev       = "true"       // 是否开发模式（决定 ~/.kai 还是 ~/.kai.dev）
  )
  ```
- 打包时通过 `-ldflags` 覆盖：
  ```bash
  go build -ldflags "\
    -X cnb.cool/dtapp/kai/internal/buildinfo.Version=1.0.0 \
    -X cnb.cool/dtapp/kai/internal/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
    -X cnb.cool/dtapp/kai/internal/buildinfo.Dev=false"
  ```
- 开发运行（`wails3 dev`）默认 `Dev=true`，走 `~/.kai.dev/`；正式构建 `Dev=false`，走 `~/.kai/`。

### 7.3 命令
```bash
# 安装 Wails 3 CLI
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
wails3 doctor

# 前端依赖（pnpm）
cd frontend && pnpm install

# 开发运行（热重载，默认开发目录 ~/.kai.dev）
wails3 dev

# 正式构建（注入版本号 + 打包时间 + 正式目录）
wails3 build -ldflags "\
  -X cnb.cool/dtapp/kai/internal/buildinfo.Version=1.0.0 \
  -X cnb.cool/dtapp/kai/internal/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -X cnb.cool/dtapp/kai/internal/buildinfo.Dev=false"
```

> 注：Wails 3 目前处于 Beta，API 可能变动，以 https://v3.wails.io 最新文档为准。

---

## 8. 进度追踪

| 里程碑 | 状态 | 说明 |
|--------|------|------|
| **M1 脚手架 & 基建** | ✅ 完成 | 多窗口(主/设置)+托盘+菜单；`buildinfo`/`model`/`config`/`engine`/`service` 骨架；前端 pinia+naive-ui+router + i18n 初始化；**主题/语言 auto 基建已落地**（config 加 `Theme`+`Language=auto`、service 加 `DetectSystemLang`/`DetectSystemAppearance`/`GetTheme`/`SetTheme`、`resolveLang` 修正 GetEngines/GetLanguages lang 来源；前端 App.vue 接 ConfigProvider + matchMedia 自动主题 + navigator.language 自动语言）；后端 go-i18n/v2 切换完成；`go build .` + `pnpm typecheck` 通过 |
| **M2 翻译主流程** | ✅ 完成 | ①`model`/`config`/`engine`/`history`/`service` 接口落地 ②Google（免 key gtx 端点，标准库 net/http）③DeepL（需 APIKey，免费版默认端点）+ OpenAI（Chat Completions 兼容，Secret=base_url/Extra=model）④前端 `TranslateWindow.vue` 引擎/语言下拉 + 结果展示 ⑤翻译完成自动写历史（去重）；`internal/historystore` 纯 Go SQLite（modernc.org/sqlite，无 CGO），API 手写实现与 sqlc 生成一致（`query.sql`+`sqlc.yaml` 为可重新生成源头）；前端历史面板（搜索/删除/清空/回填）；`go build` 后端零错误、前端 typecheck 零错误 |
| **M3 划词浮窗** | ✅ 完成 | ①`selection.go`（带 darwin/windows build tag）轮询 `pbpaste` 检测选中文本变化，命中后 `app.Event.Emit("kai:selection", {text,x,y})` 推前端（zero 额外授权，mac 优先；非 darwin 回退空实现）②`SelectionPopup.vue` 浮窗监听 `kai:selection`，贴近选区坐标弹出并即时翻译 ③`main.go` 注册 `selection` 隐藏窗口、`StartSelectionMonitor` 在 Startup 启动 ④i18n 补 `selection.*`；`go build` 后端零错误、前端 typecheck 零错误（仅 TranslateWindow.vue 预存 `copyText` 未读 HINT） |
| **M4 截图 OCR** | ✅ 完成 | ①`internal/engine/ocr_tesseract.go`（纯 Go exec 调系统 `tesseract`，无 CGO）：`TesseractOCR.Recognize` 写临时 PNG→tesseract→读 txt；`CaptureScreenshot` 用 `screencapture`(mac)/`import`(linux) 截全屏，windows 回退 `ErrNoScreenshot` ②`engines.go` 注册 `tesseract` OCR 引擎 ③`AppService.Ocr(req)`（取已注册 OCR，默认 tesseract）+ `ScreenshotOCR()`（截图→OCR 一站式，返回文本）④`TranslateWindow.vue` 加「截图翻译」按钮，回填并自动翻译（`from_ocr=true`），i18n 补 `ocr.*`；后端零 error、前端仅 HINT（未用变量） |
| **M5 快捷键 & 设置** | ✅ 完成 | ①`internal/service/hotkey.go`（新建）用 `app.GlobalShortcut.Register(accel, cb)` 注册 4 个全局快捷键（input/selection/screenshot/silent_ocr，来自 `cfg.Hotkeys`），回调只做窗口显隐/事件推送：截图→`ScreenshotOCR()`→`kai:screenshot:result` 事件；静默 OCR→`writeToClipboard`（mac pbcopy）②`selection_darwin.go`/`selection_other.go` 加 `writeToClipboard` 平台实现 ③`Startup` 末尾调 `RegisterHotkeys` ④前端 `TranslateWindow.vue` 监听 `kai:screenshot:result`(回填并翻译)/`kai:hotkey:selection`(提示先选文本)；`Settings.vue` 增强默认引擎/目标语言/TTS/快捷键展示 ⑤i18n 补 `settings.engine/default_to/tts/hotkeys/hotkey_hint`、`hotkey.*`、`selection.hotkey_hint`；后端零 error、前端仅预存 HINT |
| **M6 跨平台 & 打包** | ⏳ 待开始 | |

### 已知技术约束（已验证）
- Wails 3 beta.3 无 `UpdateService`；Service 经 `application.NewService(svc)` 注册，窗口引用由 `svc.SetApp(app)` 注入，Service 内用 `app.Window.GetByName("main")` 取窗口。
- **TypeScript 7 与 vue-tsc 不兼容**（TS7 移除 `typescript/lib/tsc` 子路径）。构建脚本用纯 `vite build`，类型检查用 `tsc --noEmit`；vue-tsc 暂不参与构建。
- pnpm 11 供应链策略：`vue-demi` 须 `pnpm approve-builds --all` 批准 postinstall；`package.json` 的 `pnpm.onlyBuiltDependencies` 已废弃，改用 `frontend/.npmrc` 的 `dangerously-allow-all-builds=true`。
- 主程序构建用 `go build .`（`go build ./...` 会误报 `build/ios` 脚手架辅助脚本）。
- 数据目录：正式 `~/.kai/`、开发 `~/.kai.dev/`（由 `buildinfo.Dev` 切换）；打包经 `-ldflags -X` 注入版本号/时间/Dev。
