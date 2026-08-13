//go:build windows

package execkey

import (
	"log/slog"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

// Windows INPUT 结构（INPUT_KEYBOARD）。
const (
	INPUT_KEYBOARD = 1
	KEYEVENTF_KEYUP = 0x0002
	VK_CONTROL      = 0x11
)

type inputStruct struct {
	typ uint32
	// 联合体部分：ki（KEYBDINPUT）
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
	// 尾部填充（mouse/keyboard 联合体大小一致）
	pad [8]byte
}

// parseHotkey 在 Windows 上把用户配置的热键字符串解析成 (key, modifiers) 与虚拟键码。
// Windows 平台别名：Ctrl/Control → VK_CONTROL，无 Cmd（Windows 无 Command 键）。
func parseHotkey(s string) (key string, modifiers []string) {
	for part := range strings.SplitSeq(s, "+") {
		p := strings.TrimSpace(part)
		switch strings.ToLower(p) {
		case "ctrl", "control":
			modifiers = append(modifiers, "ctrl")
		case "shift":
			modifiers = append(modifiers, "shift")
		case "alt":
			modifiers = append(modifiers, "alt")
		default:
			key = strings.ToLower(p)
		}
	}
	return key, modifiers
}

// vkFromKey 把常见按键名映射到 Windows 虚拟键码（VK_*）。
// 覆盖本项目复制键常用键；未知键返回 0（调用方跳过）。
func vkFromKey(key string) uint16 {
	switch key {
	case "c":
		return 0x43
	case "x":
		return 0x58
	case "v":
		return 0x56
	case "a":
		return 0x41
	case "z":
		return 0x5A
	case "y":
		return 0x59
	case "s":
		return 0x53
	default:
		return 0
	}
}

// sendKey 经 user32.SendInput 发送一次按键（down 后 up）。
func sendKey(vk uint16) {
	inputs := [2]inputStruct{
		{typ: INPUT_KEYBOARD, wVk: vk},
		{typ: INPUT_KEYBOARD, wVk: vk, dwFlags: KEYEVENTF_KEYUP},
	}
	procSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
}

// copySelection 在 Windows 上替用户执行 ExecKeyConfig.Copy 配置的键（默认 Ctrl+C），
// 把目标 app 的选区写入剪贴板，并返回剪贴板中的文本。
//
// 纯 Go 实现：用 user32.SendInput 模拟组合键，不依赖 robotgo（robotgo 需 CGO，
// 与本项目 Windows CI 的 CGO_ENABLED=0 冲突）。剪贴板读取复用 Wails 的
// application.App.Clipboard.Text()（纯 Go）。
func (e *ExecKeyController) copySelection() string {
	key, modifiers := parseHotkey(e.settingsSvc.Get().ExecKeys.Copy.Key)
	if key == "" {
		return ""
	}
	hotkey := e.settingsSvc.Get().ExecKeys.Copy.Key
	targetVk := vkFromKey(key)
	if targetVk == 0 {
		e.log.Warn("[复制键] Windows 上未知按键，跳过模拟", slog.String("按键", hotkey))
		return ""
	}

	// 按下修饰键（Ctrl/Alt/Shift）
	if has(modifiers, "ctrl") {
		sendKeyPress(VK_CONTROL, true)
	}
	if has(modifiers, "alt") {
		sendKeyPress(0x12, true) // VK_MENU
	}
	if has(modifiers, "shift") {
		sendKeyPress(0x10, true) // VK_SHIFT
	}

	// 按下并松开目标键
	sendKey(targetVk)

	// 松开修饰键（顺序与按下相反）
	if has(modifiers, "shift") {
		sendKeyPress(0x10, false)
	}
	if has(modifiers, "alt") {
		sendKeyPress(0x12, false)
	}
	if has(modifiers, "ctrl") {
		sendKeyPress(VK_CONTROL, false)
	}

	e.log.Info("[复制键] 执行快捷键 SendInput 完成", slog.String("按键", hotkey))

	time.Sleep(120 * time.Millisecond)
	text := e.selection.ReadClipboardText()
	if text == "" {
		e.log.Warn("[复制键] SendInput 成功但剪贴板为空（目标 app 可能未响应复制）",
			slog.String("按键", hotkey))
	}
	return text
}

// sendKeyPress 发送单键的按下(down=true)或松开(down=false)。
func sendKeyPress(vk uint16, down bool) {
	in := inputStruct{typ: INPUT_KEYBOARD, wVk: vk}
	if !down {
		in.dwFlags = KEYEVENTF_KEYUP
	}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

// has 判断 modifiers 切片是否包含指定修饰键。
func has(modifiers []string, name string) bool {
	for _, m := range modifiers {
		if m == name {
			return true
		}
	}
	return false
}
