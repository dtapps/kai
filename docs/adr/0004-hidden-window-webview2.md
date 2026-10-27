# ADR-0004：窗口创建后立即 Hide，规避 WebView2 controller 竞态

> status: 已采纳（生效中） · last_verified: 2026-10-27 · 决策日期：2026-08-24

## 背景

Wails v3 的 `WebviewWindowOptions.Hidden: true` 会把窗口的 WebView2 controller 创建推迟到首次显示。在 Windows 上，这会导致 `80010108`（`RPC_E_DISCONNECTED`）偶发崩溃：controller 创建回调拿到已断开的对象，进程直接退出。

翻译窗口、截图窗口都是「先隐藏、按快捷键再显示」的形态，正好踩中这个路径。

## 决策

- 不使用 `Hidden: true` 创建窗口。
- 窗口照常创建，创建完成后立即 `Hide()`。这样 controller 在启动阶段就完成初始化，后续 `Show()` 走轻量路径。
- 截图窗口同样「创建即 Hide」，等效于预建；首次 `Show()` 需要连续两次再 `Focus()`，因为 beta 版 Wails 对 Hidden 窗口首次 `Show()` 只同步建 impl 不真正显示（`internal/translate/service.go` 的 `showScreenshotWindow`）。
- 所有 `Show/Focus` 序列放进 `application.InvokeAsync` 主线程闭包，避免后台 goroutine 直接调原生窗口方法触发 AppKit 线程断言。

## 后果

正面：

- Windows 上 `80010108` 的触发频率显著下降（配合 ADR-0005 的 GPU 参数）。
- 截图窗口首次唤起更快，不再卡在 Hidden 状态机。

负面 / 约束：

- 启动时会多创建几个 WebView，内存占用略高。
- 窗口 `Show()` 的调用次数与线程约束变成隐式契约，修改窗口逻辑时要遵守「主线程 + 可能两次 Show」的范式。
- 仍然不是 100% 消除：上游 WebView2/GPU 问题依旧可能存在。

## 证据

- `main.go`：设置/翻译/截图窗口创建段，`translateWindow.Hide()`、`screenshotWindow.Hide()` 与注释。
- `internal/translate/service.go`：`showScreenshotWindow` 的两次 `Show()` + `Focus()` + `InvokeAsync`。
- `internal/service/window_wrapper.go`：`showAndFocus` 的同款范式。
