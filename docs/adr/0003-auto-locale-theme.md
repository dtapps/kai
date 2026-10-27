# ADR-0003：语言与主题的 auto 以系统为唯一来源

> status: 已采纳（部分落地） · last_verified: 2026-10-27 · 决策日期：2026-08-10

## 背景

早期实现里，`auto` 有多处「兜底」：前端自己猜、后端再猜、某些路径写死中文。结果：

- 系统切语言后，界面部分中文、部分英文；
- 主题 `auto` 在 macOS 上依赖 `matchMedia`，而 Wails webview 的 `prefers-color-scheme` 不可靠；
- 语言/主题变更事件里的「实际值」与配置值混在一起，前端无法区分。

## 决策

- 配置只保存用户模式：`language: auto|zh-CN|en-US`、`theme: auto|light|dark`。
- 实际生效值由系统派生，并作为「唯一来源」通过事件下发：
  - 主题实际值：后端 `App.Env.IsDarkMode()` → `light|dark`。
  - 界面语言实际值：前端 `navigator.language`（`resolveLang`）。
- 去掉各处 naive 兜底：不再在组件里写死中文/英文，不再用 `matchMedia` 当主题真值。
- 事件 payload 同时带 `mode`（用户模式）与 `theme`/`language`（实际值），前端一个事件处理 auto 跟随。

## 后果

正面：

- 主题 auto 在 macOS 上稳定；系统主题变更由 Wails 原生事件转发。
- 前端只认后端下发的实际主题，不再两套判断。
- 窗口启动时的原生外观也按同一解析结果设置。

负面 / 约束：

- **后端 i18n 的 auto 没有真正跟随系统**：`i18n.SetLocale` 只认 `en-US`，其余（含 `auto`）落到 `zh-CN`。所以英文系统 + auto 时，前端界面是英文，托盘/通知/后端错误仍可能是中文。
- 语言实际值有两套解析（前端 `navigator.language`、后端白名单），新增语言时必须同时改。
- 更新器不接受 `auto`，启动时用 `resolveUpdaterLocale` 把 auto 解析为 `i18n.GetLocale()`，继承上述后端限制。

## 证据

- `internal/settings/service.go`：`Language` / `Theme` 字段与默认 `auto`。
- `internal/service/config_wrapper.go`：`GetSystemTheme`、`SetTheme`、`SaveConfig` 的事件 payload。
- `main.go`：`resolveUpdaterTheme`、`resolveUpdaterLocale`、系统主题事件监听、窗口启动外观。
- `frontend/src/i18n/index.svelte.ts`：`resolveLang` / `getSystemLocale`。
- `internal/i18n/i18n.go`：`SetLocale` 的白名单（`EN_US` 之外一律 `ZH_CN`）。
