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
/** 前端改语言后触发：复用上次 OCR 原文，跳过截图/OCR 直接按新语言重新翻译。payload: ScreenshotRetranslatePayload */
export const EventScreenshotRetranslate = 'kai:screenshot:retranslate';

// 截图/OCR 缓存的 session 标识：区分不同入口，避免互相覆盖（对齐 internal/events/events.go）。
/** 截图翻译窗口（含热键/菜单/重新截图按钮，全部投到 ScreenshotWindow） */
export const ScreenshotSessionScreenshot = 'screenshot';
/** 输入翻译页内的截图 OCR（预留，与截图翻译窗口隔离） */
export const ScreenshotSessionInput = 'input';

// EventThemeChanged 主题变更广播，payload: ThemeChangedPayload
export const EventThemeChanged = 'kai:theme:changed';

// EventHotkeysChanged 快捷键重注册完成后广播，payload: string[]
export const EventHotkeysChanged = 'kai:hotkeys:changed';

// EventInputFill 选区回填主窗口输入框，payload: string（选中文本）
export const EventInputFill = 'kai:input:fill';

// EventTranslateResult 多引擎翻译逐个返回结果，payload: TranslateResult
export const EventTranslateResult = 'kai:translate:result';

// EventEnginesChanged 设置里增删/启停翻译引擎后广播，通知翻译窗口等刷新引擎列表。
// 无 payload（纯前端窗口间通知，各窗口自行重新拉取 GetEngines）。
export const EventEnginesChanged = 'kai:engines:changed';

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

// ScreenshotRetranslatePayload 截图翻译改语言重新翻译事件参数。
// session 标识缓存来源（ScreenshotSessionScreenshot / ScreenshotSessionInput），
// 后端据此取用对应入口最近一次 OCR 原文，避免不同入口互相串。
// from/to 为目标翻译语言组合（from 允许 Auto），后端复用上次 OCR 原文重新翻译。
export interface ScreenshotRetranslatePayload {
  session: string; // ScreenshotSessionScreenshot | ScreenshotSessionInput
  from: string; // TranslateLang: auto | zh | en | ...
  to: string; // TranslateLang
}
