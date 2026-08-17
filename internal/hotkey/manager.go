// Package hotkey 专管全局快捷键的注册与广播，与 AppService（RPC 门面）解耦。
// 它只持有注册热键所必需的最小依赖（app / settings / 执行键控制器 / 少量回调），
// 不掺入翻译、历史、引擎等其它职责。
//
// 复制键(ExecKeyConfig.Copy)是执行键，不在此注册，由 execKeyCtrl 在注册键回调里按需调用。
package hotkey

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/execkey"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/settings"
)

// Manager 全局快捷键管理器。
type Manager struct {
	app         *application.App
	settingsSvc *settings.Service
	log         *slog.Logger

	// execKeyCtrl 执行键控制器：注册键回调按需调用其 CopySelection 模拟复制。
	execKeyCtrl *execkey.ExecKeyController

	// TODO(2026-08-11): selSvc 字段已禁用。系统取词（Swift 桥接 kai_selected_text）分支在
	// Register 内被注释禁用（用户反馈该路径引发电脑异常），故此处不再持有 selSvc。
	// 若日后恢复系统取词路径，取消本行注释并恢复 NewManager 中的 selSvc 赋值即可。
	// selSvc *selection.Service

	// 以下回调由上层注入，桥接窗口/翻译等能力。
	mainWindow          func() application.Window
	screenshotOCR       func() error
	screenshotTranslate func() error              // 截图翻译主流程：区域截图→OCR→翻译→投递截图窗口
	screenshotWindow    func() application.Window // 截图翻译窗口
	emitHotkeysChanged  func([]string)            // 广播当前生效清单给前端
}

// SetApp 在 app 就绪后注入（启动编排阶段）。
func (h *Manager) SetApp(app *application.App) {
	h.app = app
}

// Unregister 逐个注销当前已注册的全局快捷键（绕过 UnregisterAll 内部提前 return 的坑）。
func (h *Manager) Unregister() {
	if h.app == nil {
		return
	}
	mgr := h.app.GlobalShortcut
	if mgr == nil {
		return
	}
	for _, accel := range mgr.GetAll() {
		mgr.Unregister(accel)
	}
}

// NewManager 构造快捷键管理器。
func NewManager(
	app *application.App,
	st *settings.Service,
	execKeyCtrl *execkey.ExecKeyController,
	mainWindow func() application.Window,
	screenshotOCR func() error,
	screenshotTranslate func() error,
	screenshotWindow func() application.Window,
	emitHotkeysChanged func([]string),
) *Manager {
	return &Manager{
		app:         app,
		settingsSvc: st,
		log:         slog.Default(),
		execKeyCtrl: execKeyCtrl,
		// TODO(2026-08-11): selSvc 字段已禁用，参数暂保留以兼容 main.go 调用签名；
		// 恢复系统取词路径时改回 `selSvc: selSvc,`。
		mainWindow:          mainWindow,
		screenshotOCR:       screenshotOCR,
		screenshotTranslate: screenshotTranslate,
		screenshotWindow:    screenshotWindow,
		emitHotkeysChanged:  emitHotkeysChanged,
	}
}

// TriggerInput 等效于按下「输入翻译」快捷键：呼起主窗口，并按配置走复制键模拟复制
// 或（未来）系统取词分支，把选区内容经 EventInputFill 投递到输入框。供托盘菜单点击复用，
// 使菜单点击与真实快捷键行为完全一致。
func (h *Manager) TriggerInput() {
	w := h.mainWindow()
	if w == nil {
		return
	}
	switch {
	case h.settingsSvc.Get().ExecKeys.Copy.Key != "" && h.settingsSvc.Get().ExecKeys.Copy.Enabled:
		// 复制键分支：顺序严格为「先模拟 Cmd+C 复制 → 再 Show/Focus」。
		// 若先 Focus，焦点切到 Kai，模拟的 Cmd+C 落在 Kai 窗口（无选中内容），
		// 剪贴板仍是旧值，导致取到错误内容。
		sel := h.execKeyCtrl.CopySelection()
		h.log.Info(i18n.T("log.hotkey_read_clipboard"), slog.String(i18n.T("log.field_source"), "唤起主窗口模拟复制"), slog.Int(i18n.T("log.field_length"), len(sel)), slog.String(i18n.T("log.field_content"), sel))
		w.Show()
		w.Focus()
		if sel != "" {
			h.app.Event.Emit(events.EventInputFill, sel)
		}
		// TODO(2026-08-11): 暂时禁用「系统取词（macOS Swift 桥接 kai_selected_text）」分支。
		// 原因：用户反馈启用该路径后电脑偶尔出现奇奇怪怪的问题（疑似与全局快捷键回调内
		// 延迟 150ms 的 AX 查询/焦点切换有关）。待定位根因后再决定是否恢复。
		// 恢复方式：取消下面整段注释即可（selSvc 字段及 SelectedTextViaSystem 实现保持不变）。
		//
		// case h.selSvc != nil:
		// 	// 未配置复制键：经系统取词（macOS Swift 桥接 kai_selected_text）。
		// 	// 关键约束：取词时前台 app 必须仍是「目标 app」，否则 AX 读不到其选区。
		// 	// 因此不能先 Focus Kai（那会让前台 app 变成 Kai 自己）。
		// 	// 顺序为「延迟取词（焦点仍在目标 app）→ 再 Show/Focus 拉起 Kai」。
		// 	// 延迟的原因：全局快捷键回调触发瞬间，系统仍在处理该按键事件，此时同步
		// 	// 做 AX 查询会被拒绝（kAXErrorCannotComplete -25212）；等事件分发完毕、
		// 	// 焦点稳定后（此时目标 app 仍是前台）再读，才能拿到选区。
		// 	time.AfterFunc(150*time.Millisecond, func() {
		// 		sel := h.selSvc.SelectedTextViaSystem()
		// 		h.log.Info("系统取词读取选区", slog.String("来源", "Swift桥接取词"), slog.Int("长度", len(sel)))
		// 		w.Show()
		// 		w.Focus()
		// 		if sel != "" {
		// 			h.app.Event.Emit(events.EventInputFill, sel)
		// 		}
		// 		})
	}
}

// TriggerScreenshot 等效于按下「截图翻译」快捷键：隐藏截图窗口→区域截图→OCR→翻译→
// 呼起截图窗口展示。供托盘菜单点击复用，使菜单点击与真实快捷键行为完全一致。
func (h *Manager) TriggerScreenshot() {
	// 先隐藏自身窗口，避免遮挡用户选区（screencapture 交互选区需要干净的屏幕）。
	if w := h.screenshotWindow(); w != nil {
		w.Hide()
	}
	if err := h.screenshotTranslate(); err != nil {
		h.log.Error(i18n.T("log.hotkey_screenshot_failed"), slog.Any(i18n.T("log.field_error"), err))
		return
	}
	// 流程完成会经 EventScreenshotOCR 投递结果，这里只负责把窗口拉起展示。
	// 注意：screenshot 窗口也是 Hidden 创建的复用浮窗，必须像 showAndFocus 那样
	// 连点两次 Show()——首次 Show 仅触发 Run() 创建 webview impl（不会真正 show），
	// 第二次才能真正 show+聚焦；否则窗口首帧未就绪，用户首次点击标题栏按钮（红绿灯/关闭）
	// 会被尚未就绪的窗口吞掉、需点第二次才有反应。
	if w := h.screenshotWindow(); w != nil {
		w.Show()
		w.Show()
		w.Focus()
	}
}

// Register 注册全局快捷键（仅注册键：Input/Screenshot）。
// 注册前先逐个 Unregister，保证热键配置变更后可实时生效（无需重启）。
func (h *Manager) Register() {
	if h.app == nil {
		return
	}
	mgr := h.app.GlobalShortcut
	if mgr == nil {
		return
	}
	// 逐个注销当前已注册项（绕开 UnregisterAll 内部 hadShortcuts/started 提前 return，
	// 确保运行时任何已注册快捷键都能被真正撤销，避免"清空后仍执行"）。
	for _, accel := range mgr.GetAll() {
		mgr.Unregister(accel)
	}
	cfg := h.settingsSvc.Get()
	if cfg == nil {
		return
	}
	hk := cfg.Hotkeys
	h.log.Info(i18n.T("log.hotkey_start_register"),
		slog.String(i18n.T("log.field_source"), hk.Input.Key),
		slog.Bool(i18n.T("log.field_enabled"), hk.Input.Enabled),
		slog.String(i18n.T("log.field_source"), hk.Screenshot.Key),
		slog.Bool(i18n.T("log.field_enabled"), hk.Screenshot.Enabled),
	)

	// 唤起主窗口（输入框聚焦）
	if hk.Input.Key != "" && hk.Input.Enabled {
		if err := mgr.Register(hk.Input.Key, func() {
			h.log.Info(i18n.T("log.hotkey_trigger"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_main_window")), slog.String(i18n.T("log.field_key"), hk.Input.Key))
			h.TriggerInput()
		}); err != nil {
			h.log.Error(i18n.T("log.hotkey_register_failed"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_main_window")), slog.String(i18n.T("log.field_key"), hk.Input.Key), slog.Any(i18n.T("log.field_error"), err))
		} else {
			h.log.Info(i18n.T("log.hotkey_registered"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_main_window")), slog.String(i18n.T("log.field_key"), hk.Input.Key))
		}
	} else {
		h.log.Info(i18n.T("log.hotkey_skip"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_main_window")), slog.String(i18n.T("log.field_reason"), i18n.T("log.field_value_not_set")))

	}

	// 截图翻译：区域截图→系统 OCR→翻译→投递截图窗口并呼出
	if hk.Screenshot.Key != "" && hk.Screenshot.Enabled {
		if err := mgr.Register(hk.Screenshot.Key, func() {
			h.log.Info(i18n.T("log.hotkey_trigger"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_screenshot")), slog.String(i18n.T("log.field_key"), hk.Screenshot.Key))
			h.TriggerScreenshot()
		}); err != nil {
			h.log.Error(i18n.T("log.hotkey_register_failed"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_screenshot")), slog.String(i18n.T("log.field_key"), hk.Screenshot.Key), slog.Any(i18n.T("log.field_error"), err))
		} else {
			h.log.Info(i18n.T("log.hotkey_registered"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_screenshot")), slog.String(i18n.T("log.field_key"), hk.Screenshot.Key))
		}
	} else {
		h.log.Info(i18n.T("log.hotkey_skip"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_screenshot")), slog.String(i18n.T("log.field_reason"), i18n.T("log.field_value_not_set")))
	}

	// 汇总当前真正生效的注册清单（用于排查"清空后仍执行"）
	active := mgr.GetAll()
	h.log.Info(i18n.T("log.hotkey_reregister_done"), slog.Any(i18n.T("log.field_active"), active))

	// 把当前真正生效的快捷键清单推给前端实时显示。
	h.emitHotkeysChanged(active)
}
