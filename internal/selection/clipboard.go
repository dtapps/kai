package selection

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// readClipboardText 跨平台读取系统剪贴板文本。app 为 nil 时直接返回空串。
func readClipboardText(app *application.App) string {
	if app == nil {
		return ""
	}
	text, _ := app.Clipboard.Text()
	return text
}
