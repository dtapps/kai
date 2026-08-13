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
// GlobalShortcut 回调运行在独立 goroutine（非主线程），故需 application.InvokeSync 把
// KeyTap 调度回主线程执行（官方文档推荐方式：自定义主线程任务用 InvokeSync 包裹）。
// sleep + readClipboard 仍在当前 goroutine 上执行。
func (e *ExecKeyController) copySelection() string {
	key, modifiers := parseHotkey(e.settingsSvc.Get().ExecKeys.Copy.Key)
	if key == "" {
		return ""
	}
	hotkey := e.settingsSvc.Get().ExecKeys.Copy.Key

	var keyTapErr error
	application.InvokeSync(func() {
		e.log.Info("执行快捷键 解析热键 InvokeSync内", slog.String("按键", hotkey),
			slog.String("key", key), slog.Any("modifiers", modifiers))
		mods := make([]any, len(modifiers))
		for i, m := range modifiers {
			mods[i] = m
		}
		keyTapErr = robotgo.KeyTap(key, mods...)
	})
	if keyTapErr != nil {
		e.log.Warn("[复制键] 复制键执行失败（可能未获得辅助功能授权）",
			slog.String("按键", hotkey), slog.Any("error", keyTapErr))
		return ""
	}
	e.log.Info("[复制键] 执行快捷键 KeyTap 返回成功（无 error）", slog.String("按键", hotkey))

	time.Sleep(120 * time.Millisecond)
	text := e.selection.ReadClipboardText()
	if text == "" {
		// KeyTap 返回成功但剪贴板为空：典型是辅助功能（Accessibility）未授权，
		// 系统静默丢弃了 CGEvent，Cmd+C 没有真正送达目标 app。
		e.log.Warn("[复制键] 唤起主窗口模拟复制 KeyTap 成功但剪贴板为空（疑似未获辅助功能授权）",
			slog.String("按键", hotkey))
	}
	return text
}
