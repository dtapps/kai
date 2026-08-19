package model

// 应用窗口名称常量，集中管理防止裸字符串拼写错误。
// 与 internal/events（事件名）分离，避免职责混淆。
const (
	// WindowTranslate 输入翻译窗口（mainWindow，/translate.html）
	WindowTranslate = "translate"
	// WindowSettings 设置窗口（/settings.html）
	WindowSettings = "settings"
	// WindowScreenshot 截图翻译窗口（/screenshot.html）
	WindowScreenshot = "screenshot"
)
