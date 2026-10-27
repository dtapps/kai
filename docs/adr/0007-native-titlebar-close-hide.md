# ADR-0007：原生标题栏 + 关闭即隐藏 + 事件清理

> status: 已采纳（生效中） · last_verified: 2026-10-27 · 决策日期：2026-08-20

## 背景

Kai 的窗口是「用完即走」的工具窗：

- 设置窗口需要常驻，关闭后还要能从托盘唤回；
- 翻译窗口关闭后应清空输入/结果，但保留 WebView 实例，下次秒开；
- 截图窗口关闭后应清空截图与译文，但不要销毁窗口（否则重新创建会踩 ADR-0004 的竞态）。

如果红 X 直接销毁窗口，会带来重建成本、状态残留与 Windows 竞态。

## 决策

- 使用原生标题栏（不使用自绘标题栏；`frontend/src/components/TitleBar.svelte` 存在但当前三个窗口没有引用）。
- 所有常驻窗口注册 `WindowClosing` hook：`event.Cancel()` 后 `Hide()`，不销毁。
- 在隐藏前广播 `kai:window:closing`，payload 是窗口名；各窗口只处理自己的名字，清空输入、结果、截图、译文。
- 退出应用只走托盘菜单「退出」→ `app.Quit()`，不依赖「关闭最后一个窗口」。

## 后果

正面：

- 窗口二次打开快，状态由前端显式清空，不依赖销毁重建。
- Windows 上避免反复创建 WebView controller。
- 关闭行为统一，设置窗口与翻译/截图窗口一致。

负面 / 约束：

- 窗口常驻内存，三个 WebView 不会释放。
- 关闭不是退出，用户可能误以为已退出；托盘菜单与文档需要说明。
- 事件清理必须按窗口名过滤，否则关闭一个窗口会清空另一个窗口的状态。
- 原生标题栏的最小化/最大化按钮由窗口选项控制（例如翻译窗口隐藏最大化），改窗口样式要去 `main.go` 改，不是改前端组件。

## 证据

- `main.go`：三个窗口的 `RegisterHook(events.Common.WindowClosing, ...)`、`event.Cancel()`、`Hide()`、`EventWindowClosing` emit。
- `internal/events/events.go`：`EventWindowClosing`。
- `frontend/src/components/TranslateWindow.svelte` / `ScreenshotWindow.svelte`：按窗口名过滤并清空状态。
- 托盘「退出」：`main.go` 的 `menu.quit` → `app.Quit()`。
