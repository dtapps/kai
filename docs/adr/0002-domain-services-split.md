# ADR-0002：AppService 拆分为领域 Service + 薄 Wrapper

> status: 已采纳（生效中） · last_verified: 2026-10-27 · 决策日期：2026-08-09

## 背景

最初的 `AppService` 是一个大泥球：翻译、OCR、历史、配置、引擎、窗口、快捷键、辅助功能全部挂在一个结构体上。问题：

- 任一改动都要动同一个文件，冲突频繁；
- 无法单独测试领域逻辑，wails 生命周期与应用对象到处渗透；
- 前端一次 RPC 调用会跨越多层职责，边界不清。

## 决策

按「领域服务 + 薄 Wrapper」拆分：

- `internal/translate`、`settings`、`hotkey`、`execkey`、`selection`、`engine` 等是纯业务包，不实现 wails 生命周期。
- `internal/service` 下每个领域一个 Wrapper（`ConfigWrapper`、`EngineWrapper`、`HistoryWrapper`、`TranslateWrapper`、`WindowWrapper`），只做 RPC 透传与 DTO 转换。
- `AppService` 只保留：版本、辅助功能、`SetUserAgent`、`TranslateMulti` / `ScreenshotOCR` 委托，以及 wails 生命周期三件套（`ServiceStartup` / `ServiceShutdown` / `ServiceName`）。
- 依赖在 `main.go` 显式构造并注入，取消共享 `ServiceContext` 容器。
- `app` 在 `application.New` 之前为 `nil`，之后通过各 `SetApp` 注入。

## 后果

正面：

- 领域包可单测（`translate` / `engine` / `settings` 都有测试）。
- 前端按领域调用，bindings 也按 Wrapper 分文件。
- 启动顺序集中在一个地方，容易看清依赖。

负面 / 约束：

- 每个 Wrapper 都需要 `SetApp`，漏注入会导致事件发不出去或剪贴板读不到（`execkey.SetApp` 还会传导给 `selection`）。
- `AppService` 仍持有各 Wrapper 的引用，`main.go` 侧又 Bind 了另一批 Wrapper 实例；两批实例必须都注入 `app`。
- 新增领域时要同时改 `main.go` 的构造、注册与 `SetApp` 链，步骤比单结构体多。

## 证据

- `internal/service/app.go`：`AppService` 字段与生命周期。
- `internal/service/*_wrapper.go`：各领域 Wrapper。
- `main.go`：`NewAppService` / `NewConfigWrapper` / `NewEngineWrapper` / `NewHistoryWrapper` / `NewTranslateWrapper` / `NewWindowWrapper` 的构造与 `SetApp`。
- `frontend/bindings/cnb.cool/dtapp/kai/internal/service/`：按 Wrapper 生成的 TS 文件。
