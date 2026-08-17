package events

// 应用事件名称常量，防止拼写错误。
const (
	// EventWindowShow 前端呼出窗口，payload: string（"settings" | "main"）
	EventWindowShow = "kai:window:show"

	// EventLocaleChanged 界面语言变更后广播，payload: LocaleChangedPayload
	EventLocaleChanged = "kai:locale:changed"

	// EventThemeChanged 主题变更广播，payload: ThemeChangedPayload
	EventThemeChanged = "kai:theme:changed"

	// EventHotkeysChanged 快捷键重注册完成后广播，payload: []string
	EventHotkeysChanged = "kai:hotkeys:changed"

	// EventInputFill 选区回填主窗口输入框，payload: string（选中文本）
	EventInputFill = "kai:input:fill"

	// EventTranslateResult 多引擎翻译逐个返回结果，payload: model.TranslateResult
	EventTranslateResult = "kai:translate:result"

	// EventWindowScreenshot 快捷键触发截图翻译：后端落盘区域截图后呼出截图窗口准备接收结果。
	EventWindowScreenshot = "kai:window:screenshot"

	// EventScreenshotOCR 截图翻译流程完成：后端抓取区域→OCR→翻译后，把结果投递给截图窗口。
	// payload: ScreenshotResult{Image, Text, Translations, To}
	EventScreenshotOCR = "kai:screenshot:ocr"

	// EventScreenshotRecapture 前端「重新截图」按钮触发：后端隐藏窗口并重新走一次截图翻译流程。
	EventScreenshotRecapture = "kai:screenshot:recapture"
)

// LocaleChangedPayload 界面语言变更事件参数。
// 注意：这里是界面显示语言，与翻译语言（model.Language：auto/zh/en/...）完全是两套体系，不可混用。
// Mode 为用户配置的界面语言模式（auto / zh-CN / en-US）；
// Language 为实际生效的界面语言（zh-CN / en-US，auto 时由系统 locale 派生）。
type LocaleChangedPayload struct {
	Mode     string `json:"mode"`     // 界面语言模式：auto | zh-CN | en-US
	Language string `json:"language"` // 实际生效界面语言：zh-CN | en-US
}

// ThemeChangedPayload 主题变更事件参数。
// Mode 为用户配置模式（取自 model 的 ThemeAuto/ThemeLight/ThemeDark：auto/light/dark）；
// Theme 为系统真实外观（dark/light，由 Env.IsDarkMode() 派生）。
type ThemeChangedPayload struct {
	Mode  string `json:"mode"`  // settings: auto | light | dark
	Theme string `json:"theme"` // settings: dark | light
}
