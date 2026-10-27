# ADR-0006：单实例使用 Wails 内置能力

> status: 已采纳（生效中） · last_verified: 2026-10-27 · 决策日期：2026-08-25

## 背景

Kai 常驻托盘，持有：

- 三个 SQLite 连接（config / history / httplog）；
- 全局快捷键注册；
- 托盘图标与菜单；
- 可能正在进行的截图/OCR/翻译。

如果允许启动多个实例，会争抢数据库、快捷键与托盘，出现「快捷键触发两次」「历史写冲突」「托盘出现多个图标」。

`internal/singleinstance/` 目录曾经准备放自研锁，但当前是空目录（`ls internal/singleinstance` 为空）。

## 决策

使用 Wails v3 内置的单实例能力：

```go
SingleInstance: &application.SingleInstanceOptions{
    UniqueID: "cnb.cool.dtapp.kai",
    OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
        // 把设置窗口唤到前台
        if settingsWindow != nil {
            settingsWindow.Show().Focus()
        }
    },
}
```

不再维护自研单实例包。

## 后果

正面：

- 少一套平台相关锁代码；macOS 上由 Wails 处理 flock + 进程间通知。
- 第二实例启动时由首个实例响应，行为可控。

负面 / 约束：

- `internal/singleinstance/` 目录成为空壳，容易被误认为还有实现；新增代码不要往这里放东西。
- 第二实例不是「静默退出」，而是触发首个实例把设置窗口唤到前台；如果用户期望「无任何反应」，需要另行调整。
- `UniqueID` 改动会让新旧版本互相视为不同应用，升级期间可能短暂允许双开。

## 证据

- `main.go`：`application.Options.SingleInstance` 与 `OnSecondInstanceLaunch`。
- `internal/singleinstance/`：当前为空目录。
