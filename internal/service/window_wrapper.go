package service

import (
	"log/slog"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// WindowWrapper 薄适配层：负责各窗口的呼出（主窗口 / 设置窗口）。
// 窗口句柄由 app 引用按需查询，避免持有过期的 window 实例。
// 仅暴露前端需要的 RPC，不实现 wails 生命周期三件套。
type WindowWrapper struct {
	app *application.App
}

// NewWindowWrapper 构造窗口 Wrapper。app 允许在 app 就绪后通过 SetApp 注入。
func NewWindowWrapper(app *application.App) *WindowWrapper {
	return &WindowWrapper{app: app}
}

// SetApp 在 app 就绪后注入。
func (w *WindowWrapper) SetApp(app *application.App) {
	w.app = app
}

func (w *WindowWrapper) translateWindow() application.Window {
	if w.app == nil {
		return nil
	}
	win, ok := w.app.Window.GetByName(model.WindowTranslate)
	if !ok {
		slog.Error(i18n.T("log.window_handle_failed"), slog.String("window", model.WindowTranslate))
		return nil
	}
	return win
}

func (w *WindowWrapper) settingsWindow() application.Window {
	if w.app == nil {
		return nil
	}
	win, ok := w.app.Window.GetByName(model.WindowSettings)
	if !ok {
		slog.Error(i18n.T("log.window_handle_failed"), slog.String("window", model.WindowSettings))
		return nil
	}
	return win
}

// screenshotWindow 按名取截图翻译窗口句柄（与 translateWindow/settingsWindow 同款）。
func (w *WindowWrapper) screenshotWindow() application.Window {
	if w.app == nil {
		return nil
	}
	win, ok := w.app.Window.GetByName(model.WindowScreenshot)
	if !ok {
		slog.Error(i18n.T("log.window_handle_failed"), slog.String("window", model.WindowScreenshot))
		return nil
	}
	return win
}

// showAndFocus 呼出并确保窗口真正可见。
// 注意：Wails v3 对 Hidden 窗口会延迟创建 webview 实现（impl）。
// 首次调用 Show() 时若 impl 尚为 nil，底层只触发 Run() 创建 webview 而「不会」真正 show，
// 导致第一次快捷键呼出时窗口不出现、需再按一次才生效。
// 因此连续两次 Show()：第一次触发 Run() 创建 impl，第二次 impl 已就绪真正 show。
func showAndFocus(win application.Window) {
	if win == nil {
		return
	}
	win.Show()
	win.Show()
	win.Focus()
}

// ShowTranslateWindow 呼出翻译窗口
func (w *WindowWrapper) ShowTranslateWindow() {
	showAndFocus(w.translateWindow())
}

// ShowSettings 打开设置
func (w *WindowWrapper) ShowSettings() {
	showAndFocus(w.settingsWindow())
}

// ShowScreenshotWindow 呼出截图翻译窗口（与 ShowTranslateWindow 对称）。
// 注：main.go 的 EventWindowShow('screenshot') 当前仍走独立的 showScreenshotWindow()
// 包级函数；后续可统一迁移到本方法，使窗口呼出全部收口到 WindowWrapper。
func (w *WindowWrapper) ShowScreenshotWindow() {
	showAndFocus(w.screenshotWindow())
}
