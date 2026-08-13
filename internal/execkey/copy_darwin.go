//go:build darwin

package execkey

import (
	"log/slog"
	"strings"
	"time"

	"github.com/go-vgo/robotgo"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// parseHotkey 在 macOS 上把用户配置的热键字符串解析成 robotgo.KeyTap 所需的 (key, modifiers)。
// macOS 平台专用别名：Cmd/Command → "cmd", Option → "alt"。
func parseHotkey(s string) (key string, modifiers []string) {
	for part := range strings.SplitSeq(s, "+") {
		p := strings.TrimSpace(part)
		switch strings.ToLower(p) {
		case "cmd", "command":
			modifiers = append(modifiers, "cmd")
		case "ctrl", "control":
			modifiers = append(modifiers, "ctrl")
		case "shift":
			modifiers = append(modifiers, "shift")
		case "alt", "option":
			modifiers = append(modifiers, "alt")
		default:
			key = strings.ToLower(p)
		}
	}
	return key, modifiers
}

// copySelection 在 macOS 上替用户执行 ExecKeyConfig.Copy 配置的键，把目标 app 的选区写入剪贴板，
// 并返回剪贴板中的文本。
//
// 关键约束：robotgo.KeyTap 走 CGEvent，必须在主线程执行，否则 SIGTRAP。Wails3 的
// GlobalShortcut 回调运行在独立 goroutine（非主线程），故需 application.InvokeSyncWithError
// 把 KeyTap 调度回主线程执行（与 Windows 的 makc Combo 同一调用结构，便于统一排查）。
// sleep + readClipboard 仍在当前 goroutine 上执行。
func (e *ExecKeyController) copySelection() string {
	hotkey := e.settingsSvc.Get().ExecKeys.Copy.Key
	if strings.TrimSpace(hotkey) == "" {
		return ""
	}

	key, modifiers := parseHotkey(hotkey)
	if key == "" {
		e.log.Warn("[复制键] 解析热键失败，跳过模拟",
			slog.String("按键", hotkey),
		)
		return ""
	}

	mods := make([]any, len(modifiers))
	for i, m := range modifiers {
		mods[i] = m
	}

	e.log.Debug("[复制键] 解析热键 InvokeSync 内",
		slog.String("按键", hotkey),
		slog.String("key", key),
		slog.Any("modifiers", modifiers),
		slog.Any("mods", mods),
	)
	// 在主线程执行
	comboErr := application.InvokeSyncWithError(func() error {
		return robotgo.KeyTap(key, mods...)
	})
	if comboErr != nil {
		e.log.Warn("[复制键] 发送组合键失败（可能未获辅助功能授权）",
			slog.String("按键", hotkey),
			slog.Any("error", comboErr),
		)
		return ""
	}
	e.log.Debug("[复制键] 执行快捷键完成（robotgo KeyTap 成功）",
		slog.String("按键", hotkey),
	)

	time.Sleep(120 * time.Millisecond)
	text := e.selection.ReadClipboardText()
	if text == "" {
		e.log.Warn("[复制键] 发送组合键成功但剪贴板为空（疑似未获辅助功能授权）",
			slog.String("按键", hotkey),
		)
	}
	return text
}
