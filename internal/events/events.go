package events

import "cnb.cool/dtapp/kai/internal/model"

// 应用事件名称常量，防止拼写错误。
const (
	// EventWindowShow 前端呼出窗口，payload: string（"settings" | "main"）
	EventWindowShow = "kai:window:show"

	// EventWindowClosing 窗口关闭（原生红 X / 自定义标题栏关闭）被触发时广播，
	// 各窗口按需清理自身状态（如翻译窗口清空输入与结果）。payload: 窗口名（model.WindowTranslate 等），
	// 各窗口只处理自身关闭，避免关闭别的窗口误清空。
	// 与前端 frontend/src/utils/events.ts 的 EventWindowClosing 对齐，是单一真相源。
	EventWindowClosing = "kai:window:closing"

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

	// EventScreenshotRetranslate 前端改语言后触发：复用最近一次 OCR 原文，
	// 跳过截图/OCR 直接用新语言重新翻译并增量推送结果。payload: ScreenshotRetranslatePayload
	EventScreenshotRetranslate = "kai:screenshot:retranslate"

	// EventEnginesChanged 引擎增删/启停/配置变更后广播，通知所有窗口（尤其翻译窗口）
	// 重新拉取引擎列表。payload: EngineChangedPayload（变更引擎 ID + 启用态）。
	// 与前端 frontend/src/utils/events.ts 的 EventEnginesChanged 对齐，是单一真相源。
	EventEnginesChanged = "kai:engines:changed"

	// EventAutoClipboardChanged 输入翻译窗口「自动读取剪贴板翻译」开关状态变化后广播。
	// payload: bool（开启=true / 关闭=false）。设置页据此实时禁用/恢复复制键两个开关。
	// 与前端 frontend/src/utils/events.ts 的 EventAutoClipboardChanged 对齐，是单一真相源。
	EventAutoClipboardChanged = "kai:auto-clipboard:changed"
)

// 截图/OCR 缓存的 session 标识：区分不同入口，避免互相覆盖。
const (
	// ScreenshotSessionScreenshot 截图翻译窗口（含热键/菜单/重新截图按钮，全部投到 ScreenshotWindow）。
	ScreenshotSessionScreenshot = "screenshot"
	// ScreenshotSessionInput 输入翻译页内的截图 OCR（预留，与截图翻译窗口隔离）。
	ScreenshotSessionInput = "input"
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

// ScreenshotRetranslatePayload 截图翻译改语言重新翻译事件参数。
// Session 标识缓存来源（ScreenshotSessionScreenshot / ScreenshotSessionInput），
// 后端据此取用对应入口最近一次 OCR 原文，避免不同入口互相串。
// From/To 为目标翻译语言组合（From 允许 Auto），后端复用最近一次 OCR 原文重新翻译。
type ScreenshotRetranslatePayload struct {
	Session string         `json:"session"`
	From    model.Language `json:"from"`
	To      model.Language `json:"to"`
}
