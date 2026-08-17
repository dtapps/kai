//go:build darwin

package execkey

import (
	"log/slog"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
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
// fallback=true 时：若自定义复制键执行失败（解析失败 / 注入失败 / 剪贴板为空），会自动再用
// 系统默认复制键（Cmd+C）重试一次——即"自定义键没生效就退回到系统原生的复制键"。
// 该参数由 CopySelection 在开启回退时传入 true（方法内部自行保护/还原用户剪贴板）。
//
// 关键约束：robotgo.KeyTap 走 CGEvent，必须在主线程执行，否则 SIGTRAP。Wails3 的
// GlobalShortcut 回调运行在独立 goroutine（非主线程），故需 application.InvokeSyncWithError
// 把 KeyTap 调度回主线程执行（与 Windows 的 makc Combo 同一调用结构，便于统一排查）。
// sleep + readClipboard 仍在当前 goroutine 上执行。
func (e *ExecKeyController) copySelection(fallback bool) string {
	hotkey := e.settingsSvc.Get().ExecKeys.Copy.Key
	text := e.copyWithHotkey(hotkey)

	// 回退：自定义键没拿到内容，改用系统默认复制键（Cmd+C）再试一次。
	if fallback && text == "" {
		e.log.Warn(i18n.T("log.copykey_fallback_default"),
			slog.String(i18n.T("log.field_customkey"), hotkey),
		)
		text = e.copyDefaultKey()
	}
	return text
}

// copyDefaultKey 直接用 robotgo 真实 API 按下系统默认复制键 Cmd+C，
// 不经过字符串解析（robotgo 的 KeyTap 接收的就是 key/mods 字符串值）。
// 仅作为 Fallback 回退路径：自定义复制键未生效时退回到系统原生复制键。
func (e *ExecKeyController) copyDefaultKey() string {
	comboErr := application.InvokeSyncWithError(func() error {
		return robotgo.KeyTap("c", "cmd")
	})
	if comboErr != nil {
		e.log.Warn(i18n.T("log.copykey_default_send_failed"),
			slog.Any("error", comboErr),
		)
		return ""
	}
	e.log.Debug(i18n.T("log.copykey_exec_default_done"))

	time.Sleep(120 * time.Millisecond)
	text := e.selection.ReadClipboardText()
	if text == "" {
		e.log.Warn(i18n.T("log.copykey_default_empty"))
	}
	return text
}

// copyWithHotkey 按指定热键字符串执行一次"模拟复制 + 读剪贴板"。
// 任一环节失败（解析失败 / 注入失败 / 剪贴板为空）都返回空串，由调用方决定是否回退。
func (e *ExecKeyController) copyWithHotkey(hotkey string) string {
	if strings.TrimSpace(hotkey) == "" {
		return ""
	}

	key, modifiers := parseHotkey(hotkey)
	if key == "" {
		e.log.Warn(i18n.T("log.copykey_parse_hotkey_failed"),
			slog.String(i18n.T("log.field_key"), hotkey),
		)
		return ""
	}

	mods := make([]any, len(modifiers))
	for i, m := range modifiers {
		mods[i] = m
	}

	e.log.Debug(i18n.T("log.copykey_invoke_parse"),
		slog.String(i18n.T("log.field_key"), hotkey),
		slog.String("key", key),
		slog.Any("modifiers", modifiers),
		slog.Any("mods", mods),
	)
	// 在主线程执行
	comboErr := application.InvokeSyncWithError(func() error {
		return robotgo.KeyTap(key, mods...)
	})
	if comboErr != nil {
		e.log.Warn(i18n.T("log.copykey_send_combo_failed"),
			slog.String(i18n.T("log.field_key"), hotkey),
			slog.Any("error", comboErr),
		)
		return ""
	}
	e.log.Debug(i18n.T("log.copykey_exec_hotkey_done"),
		slog.String(i18n.T("log.field_key"), hotkey),
	)

	time.Sleep(120 * time.Millisecond)
	text := e.selection.ReadClipboardText()
	if text == "" {
		e.log.Warn(i18n.T("log.copykey_send_combo_empty"),
			slog.String(i18n.T("log.field_key"), hotkey),
		)
	}
	return text
}
