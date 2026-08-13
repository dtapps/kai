//go:build windows

package execkey

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aiwaki/makc"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Windows 复制键：使用 makc（No-cgo 跨平台输入库，底层走 purego 调 user32.dll），
// 保持 CGO_ENABLED=0 与 Windows CI 一致。macOS 仍走 copy_darwin.go 的 robotgo，
// 本文件仅在 Windows 编译（按后缀分离）。剪贴板读取复用 Wails 的 application.App。

// copyClient 缓存 makc 客户端，避免每次按键都 Open/Close。
var copyClient *makc.Client

func getCopyClient() (*makc.Client, error) {
	if copyClient != nil {
		return copyClient, nil
	}
	c, err := makc.Open()
	if err != nil {
		return nil, err
	}
	copyClient = c
	return copyClient, nil
}

// parseHotkey 在 Windows 上把用户配置的热键字符串解析为 makc.Key 列表。
// 支持 "ctrl+c" / "ctrl+shift+c" / "alt+x" 等形式；Windows 无 Command 键（忽略 cmd）。
// 解析失败（含未知键名）时返回 error，调用方跳过模拟。
func parseHotkey(s string) ([]makc.Key, error) {
	parts := strings.Split(s, "+")
	keys := make([]makc.Key, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(strings.ToLower(p))
		switch name {
		case "cmd", "command", "win", "super":
			// Windows 无对应键，跳过不报错
			continue
		}
		k, err := makc.ParseKey(name)
		if err != nil {
			return nil, fmt.Errorf("未知按键 %q: %w", name, err)
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("热键 %q 未解析出任何有效按键", s)
	}
	return keys, nil
}

// copySelection 在 Windows 上替用户执行 ExecKeyConfig.Copy 配置的键（默认 Ctrl+C），
// 把目标 app 的选区写入剪贴板，并返回剪贴板中的文本。
//
// 实现：用 makc 的 Keyboard.Combo 经 user32.SendInput 注入组合键（底层 purego，零 CGO）。
// makc 自带 Windows SendInput 后端，无需 robotgo（robotgo 需 CGO，与 Windows CI 冲突）。
func (e *ExecKeyController) copySelection() string {
	hotkey := e.settingsSvc.Get().ExecKeys.Copy.Key
	if strings.TrimSpace(hotkey) == "" {
		return ""
	}

	keys, err := parseHotkey(hotkey)
	if err != nil {
		e.log.Warn("[复制键] 解析热键失败，跳过模拟",
			slog.String("按键", hotkey), slog.String("错误", err.Error()))
		return ""
	}

	client, err := getCopyClient()
	if err != nil {
		e.log.Error("[复制键] 初始化 makc 失败", slog.String("错误", err.Error()))
		return ""
	}

	// ctx 用 app 生命周期 context（app 退出即取消），再叠加 2s 超时作保护。
	parentCtx := context.Background()
	if e.app != nil {
		parentCtx = e.app.Context()
	}

	e.log.Debug("[复制键] 解析热键 InvokeSync 内",
		slog.String("按键", hotkey),
		slog.Any("keys", keys),
	)
	// 在主线程执行
	comboErr := application.InvokeSyncWithError(func() error {
		ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
		defer cancel()
		return client.Keyboard.Combo(ctx, keys...)
	})
	if comboErr != nil {
		e.log.Error("[复制键] 发送组合键失败",
			slog.String("按键", hotkey),
			slog.String("错误", comboErr.Error()),
		)
		return ""
	}

	e.log.Debug("[复制键] 执行快捷键完成（makc 注入组合键）", slog.String("按键", hotkey))

	time.Sleep(120 * time.Millisecond)
	text := e.selection.ReadClipboardText()
	if text == "" {
		e.log.Warn("[复制键] 组合键发送成功但剪贴板为空（目标 app 可能未响应复制）",
			slog.String("按键", hotkey))
	}
	return text
}
