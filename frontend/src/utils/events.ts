// 应用事件名称常量，防止拼写错误（对齐 internal/events/events.go 的 Go 端定义）。

// EventWindowShow 前端呼出窗口，payload: 'settings' | 'main'
export const EventWindowShow = 'kai:window:show';

// EventWindowClosing 窗口关闭按钮（标题栏 X）被点击时广播；各窗口按需清理自身状态
// （如翻译窗口清空结果），再执行关闭。payload 可选窗口名。
export const EventWindowClosing = 'kai:window:closing';

// EventLocaleChanged 界面语言变更后广播，payload: LocaleChangedPayload
export const EventLocaleChanged = 'kai:locale:changed';
/** 截图翻译结果投递（后端 区域截图→OCR→翻译 完成后推送）：payload 为 ScreenshotResult */
export const EventScreenshotOCR = 'kai:screenshot:ocr';
/** 前端请求重新截图（点击「重新截图」按钮）：后端触发一次新的区域截图流程 */
export const EventScreenshotRecapture = 'kai:screenshot:recapture';

// EventThemeChanged 主题变更广播，payload: ThemeChangedPayload
export const EventThemeChanged = 'kai:theme:changed';

// EventHotkeysChanged 快捷键重注册完成后广播，payload: string[]
export const EventHotkeysChanged = 'kai:hotkeys:changed';

// EventInputFill 选区回填主窗口输入框，payload: string（选中文本）
export const EventInputFill = 'kai:input:fill';

// EventTranslateResult 多引擎翻译逐个返回结果，payload: TranslateResult
export const EventTranslateResult = 'kai:translate:result';

// 事件 payload 类型定义（对齐 internal/events/events.go 的 Go 端结构体）

// LocaleChangedPayload 语言变更事件参数。
// mode 为用户配置模式（取自 constants/lang 的 Lang：auto/zh/en）；
// language 为实际生效语言（zh/en，auto 时由系统 locale 派生）。
export interface LocaleChangedPayload {
  mode: string; // Lang: auto | zh | en
  language: string; // zh | en
}

// ThemeChangedPayload 主题变更事件参数。
// mode 为用户配置模式（取自 constants/theme 的 ThemeMode：auto/light/dark）；
// theme 为系统真实外观（dark/light，由 Env.IsDarkMode() 派生）。
export interface ThemeChangedPayload {
  mode: string; // ThemeMode: auto | light | dark
  theme: string; // dark | light
}
