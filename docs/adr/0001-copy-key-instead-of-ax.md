# ADR-0001：用复制键模拟复制取代 AX 选区读取

> status: 已采纳（生效中） · last_verified: 2026-10-27 · 决策日期：2026-08-08

## 背景

早期取词依赖 macOS Accessibility（AX）读取前台应用选区，并配一个选区浮窗。实际使用中：

- 不同应用对 AX 选区 API 的支持不一致，很多应用返回空或超时；
- 需要辅助功能授权，且授权状态不稳定；
- 浮窗定位、焦点切换与 Ax 查询互相干扰，出现过系统异常反馈；
- Windows 没有等价的稳定 AX 方案。

## 决策

改为「执行键」模型：用户按下输入翻译快捷键时，程序用系统级按键模拟复制当前选区，再从剪贴板读文本。

- macOS：`robotgo.KeyTap("c", "cmd")`。
- Windows：`makc` 走 `user32.dll` 的 `SendInput`，并 `AttachThreadInput` 附着前台线程。
- 复制前备份剪贴板，复制后先清空再写回备份，避免选区内容残留。
- 可选 `fallback`：自定义复制键取不到内容时，回退系统默认复制键再试一次。
- 另设「自动读取剪贴板」模式：用户自己复制，快捷键直接读剪贴板，不模拟复制键。

系统取词路径（Swift `kai_selected_text` / AX）保留实现但禁用，相关分支在 `internal/hotkey/manager.go` 与 `internal/selection/service.go` 中被注释。

## 后果

正面：

- 跨应用一致性显著提高，只要能 `⌘+C` / `Ctrl+C` 就能取词。
- 不再依赖 AX 选区读取，浮窗与定位问题一起消失。
- Windows 可以保持 `CGO_ENABLED=0`（makc 纯 Go）。

负面 / 约束：

- macOS 必须授予辅助功能权限。
- 会短暂改动剪贴板；异常退出时可能来不及还原。
- 需要处理「复制键与自动剪贴板双重触发」：开启自动剪贴板时自动停用复制键，并用 `copy_key_snapshot` 记录原状态。
- 复制键是「模拟按键」，不是系统 API，受目标应用与 foreground lock 影响。

## 证据

- `internal/execkey/controller.go`：备份/清空/复制/还原流程。
- `internal/execkey/copy_darwin.go`、`copy_windows.go`、`copy_other.go`。
- `internal/hotkey/manager.go`：`TriggerInput` 的自动剪贴板 / 复制键分支，以及被注释的系统取词分支。
- `internal/selection/service.go`：`SelectedTextViaSystem` 已禁用说明。
- `frontend/src/components/settings/ShortcutsTab.svelte`：复制键与自动剪贴板联动 UI。
