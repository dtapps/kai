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
	"golang.org/x/sys/windows"
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

// attachToForeground 把当前线程（Kai 的调用线程）附着到前台窗口的线程，
// 让随后经 SendInput 注入的 Ctrl+C 能可靠地交给前台目标 app 处理。
//
// 背景：Windows 有 foreground lock timeout 机制——当一个非前台进程通过
// SendInput 投递输入时，系统可能把输入"排队"而暂不交给前台窗口，导致目标 app
// 偶发没收到复制键、剪贴板读到空（"有时成功有时失败"）。AttachThreadInput 把输入
// 线程与前景线程挂接后，SendInput 的输入会直接进入前台窗口的消息队列，规避该节流。
//
// 调用方必须保证在注入后调用返回的 restore() 解除挂接，否则会破坏输入路由、造成
// 系统级卡顿。若无可附着的前景窗口（如桌面）或调用失败，返回 no-op 的 restore。
//
// golang.org/x/sys/windows 未导出 AttachThreadInput，这里用 LazyProc 直调 user32。
func attachToForeground(log *slog.Logger) (restore func()) {
	noOp := func() {}
	fg := windows.GetForegroundWindow()
	if fg == 0 {
		return noOp
	}
	fgThread, _ := windows.GetWindowThreadProcessId(fg, nil)
	selfThread := windows.GetCurrentThreadId()
	if fgThread == 0 || fgThread == selfThread {
		return noOp
	}

	user32 := windows.NewLazySystemDLL("user32.dll")
	attachProc := user32.NewProc("AttachThreadInput")
	// 仅在尚未附着时挂接，避免重复 Attach 报错。
	r, _, err := attachProc.Call(uintptr(selfThread), uintptr(fgThread), 1)
	if r == 0 {
		// 附着失败（例如已被占用），放弃，不破坏后续流程。
		log.Debug("[复制键] AttachThreadInput 失败，跳过附着",
			slog.String("错误", errNoop(err)))
		return noOp
	}
	return func() {
		attachProc.Call(uintptr(selfThread), uintptr(fgThread), 0)
	}
}

// errNoop 把可能为 nil 的 error 安全转成字符串，方便 Debug 记录。
func errNoop(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// copySelection 在 Windows 上替用户执行 ExecKeyConfig.Copy 配置的键（默认 Ctrl+C），
//
// fallback=true 时：若自定义复制键执行失败（解析失败 / 注入失败 / 剪贴板为空），会自动再用
// 系统默认复制键（Ctrl+C）重试一次——即"自定义键没生效就退回到系统原生的复制键"。
// 该参数由 CopySelection 在开启回退时传入 true（方法内部自行保护/还原用户剪贴板）。
//
// 实现：用 makc 的 Keyboard.Combo 经 user32.SendInput 注入组合键（底层 purego，零 CGO）。
// makc 自带 Windows SendInput 后端，无需 robotgo（robotgo 需 CGO，与 Windows CI 冲突）。
func (e *ExecKeyController) copySelection(fallback bool) string {
	hotkey := e.settingsSvc.Get().ExecKeys.Copy.Key
	text := e.copyWithHotkey(hotkey)

	// 回退：自定义键没拿到内容，改用系统默认复制键（Ctrl+C）再试一次。
	if fallback && text == "" {
		e.log.Warn("[复制键] 自定义复制键未生效，回退使用默认复制键",
			slog.String("自定义键", hotkey),
		)
		text = e.copyDefaultKey()
	}
	return text
}

// copyDefaultKey 直接用 makc 真实枚举键按下系统默认复制键 Ctrl+C，
// 不经过 ParseKey 字符串解析。仅作为 Fallback 回退路径：自定义复制键未生效时
// 退回到系统原生复制键。
func (e *ExecKeyController) copyDefaultKey() string {
	client, err := getCopyClient()
	if err != nil {
		e.log.Error("[复制键] 初始化 makc 失败", slog.String("错误", err.Error()))
		return ""
	}

	parentCtx := context.Background()
	if e.app != nil {
		parentCtx = e.app.Context()
	}

	comboErr := application.InvokeSyncWithError(func() error {
		// 注入前把 Kai 线程附着到前台目标线程，规避 foreground lock 导致的偶发失效。
		defer attachToForeground(e.log)()
		ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
		defer cancel()
		return client.Keyboard.Combo(ctx, makc.KeyControl, makc.KeyC)
	})
	if comboErr != nil {
		e.log.Error("[复制键] 默认复制键(Ctrl+C)发送失败",
			slog.String("错误", comboErr.Error()),
		)
		return ""
	}
	e.log.Debug("[复制键] 执行默认复制键完成（makc Ctrl+C）")

	time.Sleep(120 * time.Millisecond)
	text := e.selection.ReadClipboardText()
	if text == "" {
		e.log.Warn("[复制键] 默认复制键发送成功但剪贴板为空（目标 app 可能未响应复制）")
	}
	return text
}

// copyWithHotkey 按指定热键字符串执行一次"模拟复制 + 读剪贴板"。
// 任一环节失败（解析失败 / 注入失败 / 剪贴板为空）都返回空串，由调用方决定是否回退。
func (e *ExecKeyController) copyWithHotkey(hotkey string) string {
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
		// 注入前把 Kai 线程附着到前台目标线程，规避 foreground lock 导致的偶发失效。
		defer attachToForeground(e.log)()
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
