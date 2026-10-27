# IPC / RPC 与事件

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

Kai 是 Wails 3 桌面应用：前端（Svelte）通过生成的 TypeScript bindings 调 Go 方法，Go 通过应用事件把结果推给前端。本文列出当前 RPC、事件、窗口常量与扩展步骤。业务编排见 `PLAN.md`，引擎字段见 `engines.md`。

## 1. 运行时与窗口

| 窗口名（`model` 常量） | 值               | HTML 入口             | 尺寸 / 约束                     | 启动行为         |
|------------------------|------------------|-----------------------|---------------------------------|------------------|
| `WindowSettings`       | `settings`       | `/settings.html`      | 1280×800                        | 创建即显示、居中 |
| `WindowTranslate`      | `translate`      | `/translate.html`     | 420×560，宽固定 420，最小高 520 | 创建后 `Hide()`  |
| `WindowScreenshot`     | `screenshot`     | `/screenshot.html`    | 900×600，最小 600×400           | 创建后 `Hide()`  |
| 更新窗口（库内）       | `updater-window` | `updater_window.html` | 默认 520×660                    | 创建后 `Hide()`  |

- 窗口名常量在 `internal/model/windows.go`，前端对应 `frontend/src/constants/window.ts`。
- 除更新窗口外，三个窗口都用原生标题栏；红 X 触发 `WindowClosing` hook，`Cancel()` 后 `Hide()`，不销毁窗口（`main.go:397`、`main.go:432`、`main.go:467`）。
- 更新窗口由 `pkg/wails-updater-providers` 管理，关闭复用为 `Hide`。

## 2. Service 与 RPC

所有 Service 在 `main.go` 的 `application.NewService(...)` 注册；生成的前端文件在 `frontend/bindings/cnb.cool/dtapp/kai/internal/service/`。

### 2.1 AppService（`appservice.ts`）

| 方法                          | 签名                                           | 说明                                                    |
|-------------------------------|------------------------------------------------|---------------------------------------------------------|
| `GetVersion`                  | `() string`                                    | 构建注入的版本号                                        |
| `CheckAccessibility`          | `() bool`                                      | macOS 辅助功能授权；非 darwin 恒 `true`                 |
| `OpenAccessibilitySettings`   | `() void`                                      | 打开系统辅助功能面板                                    |
| `CheckScreenRecording`        | `() bool`                                      | macOS 屏幕录制授权；非 darwin 恒 `true`                 |
| `OpenScreenRecordingSettings` | `() void`                                      | 弹屏幕录制授权                                          |
| `SetUserAgent`                | `(ua string) void`                             | 前端把 `navigator.userAgent` 传给后端，作为全局 HTTP UA |
| `TranslateMulti`              | `(req TranslateRequest) *TranslateMultiResult` | 委托 `translate.Service`，结果走事件流                  |
| `ScreenshotOCR`               | `() *OcrResult`                                | 使用默认 OCR 引擎做一次截图 OCR                         |
| `EmitHotkeysChanged`          | `(active []string) void`                       | 内部用：广播当前生效快捷键                              |
| `SetApp`                      | `(app *application.App) void`                  | 启动编排内部注入；**前端不要调用**                      |

### 2.2 ConfigWrapper（`configwrapper.ts`）

| 方法                 | 签名                             | 说明                                                        |
|----------------------|----------------------------------|-------------------------------------------------------------|
| `GetConfig`          | `() *settings.Settings`          | 返回当前配置（含 `Path` 之外的字段）                        |
| `SaveConfig`         | `(cfg *settings.Settings) error` | 只回写固定子集，见 `configuration.md` §2.2                  |
| `GetLanguages`       | `(lang string) []NamedItem`      | 翻译语言下拉项（`value` = 语言码）                          |
| `SystemLanguages`    | `() []string`                    | macOS 系统翻译可用语言（Swift 桥接）；其他平台空            |
| `Theme` / `GetTheme` | `() string`                      | 当前主题模式（`auto`/`light`/`dark`）                       |
| `GetSystemTheme`     | `() string`                      | 系统真实外观（`light`/`dark`），来源 `App.Env.IsDarkMode()` |
| `SetTheme`           | `(theme string) error`           | 持久化并广播 `kai:theme:changed`                            |
| `SetApp`             | `(app *application.App) void`    | 内部注入，前端不要调用                                      |

### 2.3 EngineWrapper（`enginewrapper.ts`）

| 方法                  | 签名                                 | 说明                                  |
|-----------------------|--------------------------------------|---------------------------------------|
| `GetEngines`          | `() []EngineListItem`                | 已注册引擎（按 config.db id 顺序）    |
| `GetAllEngines`       | `() []AllEngineItem`                 | 数据库里全部引擎 + 启用态 + `builtin` |
| `GetKnownEngines`     | `() []EngineListItem`                | 代码里「可添加」的引擎目录            |
| `GetEngineSchema`     | `(name string) EngineSchema`         | 动态表单 schema                       |
| `GetEngineConfig`     | `(id int64) *EngineConfig`           | 单条配置回填（含解密后的 key）        |
| `AddEngine`           | `(cfg *EngineConfig) (int64, error)` | 新增并重新注册                        |
| `UpdateEngineConfig`  | `(cfg *EngineConfig) error`          | 更新并重新注册                        |
| `ToggleEngineEnabled` | `(id int64, enabled bool) error`     | 启用前校验必填；OCR 单选              |
| `RemoveEngine`        | `(id int64) error`                   | 内置 `apple`/`vision` 禁止删除        |
| `GetOcrLangs`         | `() []string`                        | OCR 语言码候选                        |
| `CheckTesseract`      | `() TesseractStatus`                 | 本机 tesseract 探测                   |
| `SetApp`              | `(app *application.App) void`        | 内部注入，前端不要调用                |

增删改启停都会广播 `kai:engines:changed`，payload 为 `EngineChangedPayload{id, enabled}`。

### 2.4 HistoryWrapper（`historywrapper.ts`）

| 方法            | 签名                                                | 说明                                         |
|-----------------|-----------------------------------------------------|----------------------------------------------|
| `GetHistory`    | `(keyword string, offset, limit int) []HistoryItem` | `limit<=0` 时后端按 50；按 `created_at DESC` |
| `CountHistory`  | `(keyword string) int64`                            | 分页总数                                     |
| `DeleteHistory` | `(id int64) error`                                  | 删单条                                       |
| `ClearHistory`  | `() error`                                          | 清空                                         |

RPC 返回的 `HistoryItem` 是 `internal/service` 的 DTO，字段为 `from_lang` / `to_lang` / `engine`（引擎名），与 `internal/model.HistoryItem` 的 `from` / `to` 不同。

### 2.5 TranslateWrapper（`translatewrapper.ts`）

| 方法                  | 签名                                           | 说明                                                |
|-----------------------|------------------------------------------------|-----------------------------------------------------|
| `Translate`           | `(req TranslateRequest) *TranslateResult`      | 单引擎；引擎未注册返回错误                          |
| `TranslateMulti`      | `(req TranslateRequest) *TranslateMultiResult` | 启动多个翻译引擎，结果走 `kai:translate:result`     |
| `Ocr`                 | `(req OcrRequest) *OcrResult`                  | 直接 OCR；`ImageData` 是 `json:"-"`，不会从 JS 传入 |
| `ScreenshotOCR`       | `(engineName string) *OcrResult`               | 先截图再 OCR                                        |
| `ScreenshotTranslate` | `(session string) *ScreenshotResult`           | 截图 → OCR → 多引擎翻译 → 事件流                    |

### 2.6 其它

| Service               | 方法                                                                  | 说明                              |
|-----------------------|-----------------------------------------------------------------------|-----------------------------------|
| `WindowWrapper`       | `ShowTranslateWindow()` / `ShowSettings()` / `ShowScreenshotWindow()` | 呼出窗口；`SetApp` 同样是内部注入 |
| `NotificationService` | `Notify(opts notifications.NotificationOptions)`                      | 原生通知，内部做授权检查          |
| `FrontendLogService`  | `FrontendLog(level, msg string)` / `SetLevel(level slog.Level)`       | 前端日志写入 `frontend.log`       |

## 3. 事件

### 3.1 事件表

| 事件名                       | 方向        | payload                                         | 发射方 / 消费方                                                       |
|------------------------------|-------------|-------------------------------------------------|-----------------------------------------------------------------------|
| `kai:window:show`            | 前端 → 后端 | `string`                                        | 前端 emit；`main.go:327` 处理 `translate` / `settings` / `screenshot` |
| `kai:window:closing`         | 后端 → 前端 | `string`（窗口名）                              | 窗口 hook emit；各窗口按名过滤后清空状态                              |
| `kai:locale:changed`         | 后端 → 前端 | `LocaleChangedPayload{mode,language}`           | `ConfigWrapper.SaveConfig`（`OnChange` 只同步后端 i18n，不 emit）     |
| `kai:theme:changed`          | 后端 → 前端 | `ThemeChangedPayload{mode,theme}`               | 配置保存、系统主题变更                                                |
| `kai:hotkeys:changed`        | 后端 → 前端 | `[]string`                                      | 快捷键注册完成后 `EmitHotkeysChanged`                                 |
| `kai:input:fill`             | 后端 → 前端 | `string`                                        | 复制键/剪贴板取词后回填翻译窗口                                       |
| `kai:translate:result`       | 后端 → 前端 | `model.TranslateResult`                         | 多引擎翻译逐条返回                                                    |
| `kai:window:screenshot`      | 保留        | `struct{}`                                      | 仅 `RegisterEvent`，当前没有 emit 点                                  |
| `kai:screenshot:ocr`         | 后端 → 前端 | `model.ScreenshotResult`                        | 截图翻译阶段性结果（图片/原文/译文）                                  |
| `kai:screenshot:recapture`   | 前端 → 后端 | `struct{}`                                      | 截图窗口「重新截图」                                                  |
| `kai:screenshot:retranslate` | 前端 → 后端 | `ScreenshotRetranslatePayload{session,from,to}` | 截图窗口改语言重翻                                                    |
| `kai:engines:changed`        | 后端 → 前端 | `EngineChangedPayload{id,enabled}`              | 引擎增删改启停；前端只当作刷新信号                                    |
| `kai:auto-clipboard:changed` | 前端 → 前端 | `bool`                                          | 翻译窗口 emit，设置页监听，实时禁用/恢复复制键                        |

### 3.2 payload 结构

```go
// internal/events/events.go
type LocaleChangedPayload struct {
    Mode     string // auto | zh-CN | en-US
    Language string // 实际生效：zh-CN | en-US
}

type ThemeChangedPayload struct {
    Mode  string // auto | light | dark
    Theme string // dark | light（系统真实外观）
}

type ScreenshotRetranslatePayload struct {
    Session string         // "screenshot" | "input"
    From    model.Language // auto | zh | en | ...
    To      model.Language
}
```

注意 `LocaleChangedPayload` 的结构注释与当前发射值不完全一致：`ConfigWrapper.SaveConfig` 把 `Mode` 和 `Language` 都填成配置值（可能是 `auto`），并没有在发射前解析成实际语言（`internal/service/config_wrapper.go`）。前端要用实际语言时应调用 `resolveLang`，不要假设 `language` 一定是 `zh-CN`/`en-US`。

`EngineChangedPayload` 定义在 `internal/service/engine_wrapper.go`，不在 `internal/events`。

### 3.3 注册约定

`main.go:init()` 用 `application.RegisterEvent[T](name)` 注册类型，`T` 必须与 `Emit` 的数据类型严格一致，否则 Wails 会 panic：

```go
application.RegisterEvent[string](kevents.EventWindowShow)
application.RegisterEvent[model.ScreenshotResult](kevents.EventScreenshotOCR)
```

当前 13 个事件里有 12 个在 `main.go:84-95` 注册；**`kai:engines:changed` 没有注册**（由 `EngineWrapper` 直接 `Emit`）。新增事件时不要照抄这个例外，按 §5 的步骤补齐注册。

前端事件名与 payload 类型在 `frontend/src/utils/events.ts`，必须与 Go 端逐字一致。

## 4. Bindings 生成

```bash
make bindings
# 等价于
wails3 generate bindings -clean=true -ts -i
```

- 输出到 `frontend/bindings/`，按 Go 包分目录；`internal/service` 下每个 Wrapper 一个 `.ts`。
- 生成文件已入库，CI 也会重新生成；不要手改。
- 前端通过 `@bindings/...` 别名导入（`frontend/vite.config.ts`、`frontend/vitest.config.ts`、`frontend/tsconfig.json`）。
- 生成的 `$Call.ByID(<hash>, args...)` 里的 ID 是 Wails 方法 ID，改动 Go 方法签名后会变。

## 5. 新增 RPC / 事件

### 5.1 新增 RPC

1. 在对应 Service/Wrapper 上加导出方法，参数/返回值必须是可 JSON 序列化的类型。
2. 如果新增的是全新 Service，在 `main.go` 的 `application.NewService(...)` 里注册。
3. `make bindings` 重新生成，前端从 `@bindings/...` 导入。
4. 更新本文 §2；涉及配置/引擎/历史的，同步 `configuration.md` / `engines.md` / `data-storage.md`。
5. 前端错误按 Promise reject 处理；Go 返回 `error` 时不要假设一定有 i18n 信封。

### 5.2 新增事件

1. 在 `internal/events/events.go` 加常量与 payload 类型。
2. 在 `main.go:init()` 加 `application.RegisterEvent[T](...)`。
3. 在 `frontend/src/utils/events.ts` 加同名常量与 TS 类型。
4. 发射方 `app.Event.Emit(name, payload)`；接收方 `app.Event.On(name, ...)`（Go）或 `onEvent(name, ...)`（前端）。
5. 更新本文 §3；必要时在 Go 侧加一个事件名稳定性测试。
6. 事件名属于跨端契约，改名要同时改 Go、前端与文档，不能只改一边。

## 6. 错误与序列化

- Go 方法的 `error` 会让前端 Promise reject；错误文本来自 `Error()`。
- `internal/i18n.Error` 的 `Error()` 会附带不可见信封：`已翻译文本 + U+001F + key + U+001F + JSON 参数`。Go 侧可用 `i18n.ParseError` 解析；日志/落库应使用 `i18n.TranslateError` 去掉信封。
- 当前前端没有调用 `ParseError` 的代码，实际展示的是后端按当前界面语言生成的文本。要改成前端按 key 翻译，需要在前端统一错误处理里接入。
- 事件 payload 注册类型不匹配会在 `Emit` 时 panic（Wails `validateCustomEvent`），这是最常见的“功能突然崩”原因。
