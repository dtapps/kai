# 平台能力与构建

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

当前正式支持 macOS 与 Windows；Linux 只保留构建与纯在线翻译能力，不在支持范围。平台差异由 build tags 与少量运行时判断实现，没有运行时插件体系。

## 1. 能力矩阵

| 能力                              | macOS | Windows | Linux（未支持） | 依据                             |
|-----------------------------------|-------|---------|-----------------|----------------------------------|
| 输入翻译                          | ✅    | ✅      | ✅（可编译）    | 通用                             |
| 复制键取词                        | ✅    | ✅      | ❌（桩返回空）  | `internal/execkey/copy_*.go`     |
| 自动读剪贴板                      | ✅    | ✅      | ✅              | 通用                             |
| 截图翻译                          | ✅    | ❌      | ❌              | `engine.CaptureRegion` 仅 darwin |
| 系统 OCR（Vision）                | ✅    | ❌      | ❌              | `ocr_vision_darwin.go`           |
| tesseract OCR                     | ✅    | ✅      | ✅（未验证）    | `ocr_tesseract.go` 跨平台        |
| 系统翻译（Translation.framework） | ✅    | ❌      | ❌              | `apple_darwin.go`                |
| 在线翻译引擎                      | ✅    | ✅      | ✅              | 通用                             |
| 自动更新                          | ✅    | ✅      | ❌（不发布）    | 更新器 + Release 矩阵            |
| 原生通知                          | ✅    | ✅      | 取决于 Wails    | `NotificationService`            |
| 托盘 / 开机自启                   | ✅    | ✅      | 取决于 Wails    | `main.go`                        |
| 辅助功能授权检查                  | ✅    | 恒 true | 恒 true         | `accessibility_*.go`             |

## 2. 构建标签与平台文件

| 能力 / 文件    | 平台实现                                                                      | 其它平台                                                           |
|----------------|-------------------------------------------------------------------------------|--------------------------------------------------------------------|
| 系统翻译       | `internal/engine/apple_darwin.go`（`//go:build darwin`）                      | `apple_other.go` 返回不支持                                        |
| 系统 OCR       | `internal/engine/ocr_vision_darwin.go`                                        | `ocr_vision_other.go` 返回不支持                                   |
| 复制键         | `internal/execkey/copy_darwin.go`（robotgo，CGO）                             | `copy_windows.go`（makc，纯 Go） / `copy_other.go` 空实现          |
| 选区坐标       | `internal/selection/point_darwin.go` / `point_windows.go`                     | `point_other.go` 返回 nil / 0                                      |
| 系统取词       | `internal/selection/bridge_darwin.go`（macOS）/ `point_windows.go`（Windows） | 空实现                                                             |
| 辅助功能       | `internal/service/accessibility_darwin.go`                                    | `accessibility_other.go` 恒 true                                   |
| WebView2 参数  | —                                                                             | `internal/webview/webview_windows.go`；其它平台 `webview_other.go` |
| Swift 桥接加载 | `pkg/swiftbridge/load.go`                                                     | `load_other.go` 空操作                                             |
| SQLite 驱动    | `internal/sqlite/sqlite_mattn.go`（`sqlite_mattn`）                           | `sqlite_modernc.go`（默认，纯 Go）                                 |

## 3. CGO 与构建组合

| 组合                             | CGO                                         | SQLite             | 说明                            |
|----------------------------------|---------------------------------------------|--------------------|---------------------------------|
| macOS 本地 `make darwin-package` | 开启（robotgo 需要）                        | `modernc`（默认）  | 没有传 `EXTRA_TAGS`             |
| macOS Release                    | `CGO_ENABLED=1` + `EXTRA_TAGS=sqlite_mattn` | `mattn/go-sqlite3` | `.github/workflows/release.yml` |
| Windows Release                  | `CGO_ENABLED=0`                             | `modernc`          | makc 纯 Go                      |
| Linux 本地                       | 视命令                                      | `modernc` 默认     | 不发布                          |

Windows 保持 `CGO_ENABLED=0` 是硬约束：复制键用 `makc`（purego 调 `user32.dll`），SQLite 用 modernc。不要在 Windows 构建里引入需要 CGO 的依赖。

## 4. macOS

### 4.1 系统版本与 App 形态

- `build/darwin/Info.plist`：`LSMinimumSystemVersion` 为 `12.0.0`，`LSUIElement` 为 `true`。
- 应用是 Accessory（无 Dock 图标、无菜单栏），只常驻托盘（`main.go` 的 `ActivationPolicyAccessory`）。
- 窗口使用原生标题栏；设置窗口启动即显示，翻译/截图窗口创建后 `Hide()`。
- 系统翻译依赖 Translation.framework；Swift 桥接脚本按当前 SDK 编译，低于 26 时兜底 `26.0`，注释说明该 API 要求 macOS 26+（`pkg/swiftbridge/scripts/build.sh`）。因此：
  - 普通 Go 功能（输入翻译、在线引擎、复制键）不受影响；
  - 系统翻译 / 系统 OCR 是否可用，最终看 `libkai_bridge.dylib` 能否加载、符号是否齐全。

### 4.2 Swift 桥接

- 源文件：`pkg/swiftbridge/internal/swift/*.swift`，按单编译单元编译成 `.o`，再打 `.a` 与 `.dylib`。
- Go 侧用 `purego` 运行时 `Dlopen`，零 CGO 静态链接。
- 开发态优先加载 `pkg/swiftbridge/libkai_bridge.dylib`；打包态从 `go:embed` 的字节落地临时文件加载（`pkg/swiftbridge/load.go`）。
- dylib 是构建产物，**不入库**；干净检出后要先 `make swift-build`。
- 加载失败不致命：`Available()` 返回 false，相关功能报「Swift 桥接不可用」，其它功能照常。

### 4.3 权限（TCC）

| 权限     | 用途                 | 检查 API                          | 打开设置                      |
|----------|----------------------|-----------------------------------|-------------------------------|
| 辅助功能 | 模拟复制键           | `AppService.CheckAccessibility`   | `OpenAccessibilitySettings`   |
| 屏幕录制 | `screencapture` 截图 | `AppService.CheckScreenRecording` | `OpenScreenRecordingSettings` |

非 darwin 平台两个检查恒返回 true，打开设置是空操作。

### 4.4 已知窗口问题

- 首次用快捷键唤起截图窗口可能被原前台应用遮挡；再触发一次或点托盘图标恢复。原因是 accessory 应用从后台抢焦点的系统限制，纯 Wails 方案下首次存在竞态（`README.md` §7）。
- 运行时切换窗口主题在 Wails beta API 下不完整：启动时按应用内主题设置原生外观，运行中由前端 theme store 尽量同步（`main.go` 窗口创建段注释）。

## 5. Windows

- WebView2：需要 Runtime；窗口统一「创建后立即 `Hide()`」，规避 `Hidden: true` 延迟创建 controller 的 `80010108` 竞态。
- GPU 兜底：注入 `--disable-gpu` 与 `--disable-gpu-compositing`（`internal/webview/webview_windows.go`），代价是失去硬件加速；对翻译类轻量 UI 影响可接受。
- 不支持系统翻译与系统 OCR；截图翻译没有入口（`CaptureRegion` 返回 `ErrNoScreenshot`）。
- 复制键用 `makc`，并通过 `AttachThreadInput` 附着前台线程，减少 `SendInput` 被 foreground lock 节流导致的偶发失败（`copy_windows.go`）。
- 安装包默认 NSIS；Authenticode 签名可选（见 `release.md` §4.2）。

## 6. Linux

- 可以编译（`make linux-build` / `linux-package`），但不在支持范围，不发布产物。
- `execkey.copy` 是空实现，复制键不可用；`selection` 坐标返回 nil；无截图翻译；无系统翻译/OCR。
- 在线引擎、输入翻译、历史等纯 Go 功能理论上可用，但未做发布验证。

## 7. 平台差异的代码入口

| 关注点         | 文件                                                                           |
|----------------|--------------------------------------------------------------------------------|
| 引擎平台可用性 | `internal/engine/engine.go` 的 `EngineSupported`（`apple`/`vision` 仅 darwin） |
| 窗口与原生外观 | `main.go` 的窗口创建段、`internal/service/window_wrapper.go`                   |
| WebView2       | `internal/webview/webview_windows.go`                                          |
| 复制键         | `internal/execkey/copy_darwin.go` / `copy_windows.go` / `copy_other.go`        |
| 选区与坐标     | `internal/selection/point_*.go`、`bridge_darwin.go`、`osa_windows.go`          |
| 权限           | `internal/service/accessibility_*.go`                                          |
| Swift 桥       | `pkg/swiftbridge/`、`pkg/swiftbridge/scripts/build.sh`                         |
| SQLite         | `internal/sqlite/sqlite_*.go`                                                  |
