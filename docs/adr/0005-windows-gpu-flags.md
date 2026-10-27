# ADR-0005：Windows 全局注入 GPU 兜底参数

> status: 已采纳（生效中） · last_verified: 2026-10-27 · 决策日期：2026-08-24

## 背景

除了 ADR-0004 的 controller 竞态，Windows 上还有一条独立的崩溃路径：GPU 进程异常（显卡驱动、硬件加速 GPU 调度、睡眠恢复后的 DComp 状态）导致 WebView2 渲染进程在 controller 创建回调前崩溃，同样表现为 `80010108`。

这类问题无法在业务代码里 catch，必须在 WebView2 启动参数层面降级。

## 决策

在 Windows 平台全局注入：

```text
--disable-gpu
--disable-gpu-compositing
```

实现放在 `internal/webview/webview_windows.go` 的 `BrowserArgs()` / `ApplyOptions()`，由 `main.go` 在 `application.New` 前调用；非 Windows 平台是同签名空实现。

## 后果

正面：

- 把渲染降级到软件路径，掐断 GPU 进程异常这条崩溃链。
- 对所有窗口（含更新窗口）全局生效，不需要逐窗口配置。

负面 / 约束：

- 失去硬件加速；翻译类轻量 UI 影响可接受，但动画/滚动可能略慢。
- 这是兜底，不是根因修复；显卡驱动/WebView2 更新后仍建议回归验证。
- 参数是 Windows 专属字段（`application.Options.Windows.AdditionalBrowserArgs`），只能在 `//go:build windows` 文件里访问。

## 证据

- `internal/webview/webview_windows.go`：参数与注释。
- `internal/webview/webview_other.go`：非 Windows 空实现。
- `main.go`：`webview.ApplyOptions(&appOpts)`。
