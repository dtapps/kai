// 本文件是 Kai Swift 桥接层（cgo）错误码与 JSON 结构的「双端契约真实定义」。
//
// Go 端错误码常量在此唯一定义（导出），供 internal/engine 包引用；
// Swift 端 BRIDGE_ERR_* 常量（internal/swift/bridge_errors.swift）必须是相同的字面量。
// 新增 / 修改任一错误码时，必须同步三处，缺一不可：
//   ① 本文件 BridgeErr* 常量（Go 真实定义）
//   ② kai_bridge.swift 的 BRIDGE_ERR_* 常量（Swift，字面量必须一致）
//   ③ internal/i18n/locales/split/ 的 err.apple_<code> key
//      —— Apple 系统级错误（apple_translate / apple_ocr）复用通用引擎文案，不新增 key。

package swiftbridge

// 错误码常量：字面量必须与 Swift 端 BRIDGE_ERR_* 完全一致。
const (
	BridgeErrEmptyText          = "empty_text"           // Swift: BRIDGE_ERR_EMPTY_TEXT  -> err.apple_empty_text
	BridgeErrTargetRequired     = "target_required"      // Swift: BRIDGE_ERR_TARGET_REQUIRED -> err.apple_target_required
	BridgeErrNullImage          = "null_image"           // Swift: BRIDGE_ERR_NULL_IMAGE  -> err.apple_null_image
	BridgeErrDecodeFailed       = "decode_failed"        // Swift: BRIDGE_ERR_DECODE_FAILED -> err.apple_decode_failed
	BridgeErrEmptyImage         = "empty_image"          // Swift: BRIDGE_ERR_EMPTY_IMAGE -> err.apple_empty_image
	BridgeErrBitmapCtxFailed    = "bitmap_ctx_failed"    // Swift: BRIDGE_ERR_BITMAP_CTX_FAILED -> err.apple_bitmap_ctx_failed
	BridgeErrBitmapRedrawFailed = "bitmap_redraw_failed" // Swift: BRIDGE_ERR_BITMAP_REDRAW_FAILED -> err.apple_bitmap_redraw_failed
	BridgeErrOcrTimeout         = "ocr_timeout"          // Swift: BRIDGE_ERR_OCR_TIMEOUT -> err.apple_ocr_timeout
	BridgeErrNoSourceLang       = "no_source_lang"       // Swift: BRIDGE_ERR_NO_SOURCE_LANG -> err.apple_no_source_lang
	BridgeErrAppleTranslate     = "apple_translate"      // Swift: BRIDGE_ERR_APPLE_TRANSLATE（系统级，复用 err.apple_translate_engine）
	BridgeErrAppleOcr           = "apple_ocr"            // Swift: BRIDGE_ERR_APPLE_OCR（系统级，复用 err.vision_ocr_engine）
)

// Swift 端常量（镜像，真实定义见 internal/swift/bridge_errors.swift）：
//
//	let BRIDGE_ERR_EMPTY_TEXT          = "empty_text"
//	let BRIDGE_ERR_TARGET_REQUIRED     = "target_required"
//	let BRIDGE_ERR_NULL_IMAGE          = "null_image"
//	let BRIDGE_ERR_DECODE_FAILED       = "decode_failed"
//	let BRIDGE_ERR_EMPTY_IMAGE         = "empty_image"
//	let BRIDGE_ERR_BITMAP_CTX_FAILED   = "bitmap_ctx_failed"
//	let BRIDGE_ERR_BITMAP_REDRAW_FAILED= "bitmap_redraw_failed"
//	let BRIDGE_ERR_OCR_TIMEOUT         = "ocr_timeout"
//	let BRIDGE_ERR_NO_SOURCE_LANG      = "no_source_lang"
//	let BRIDGE_ERR_APPLE_TRANSLATE     = "apple_translate"
//	let BRIDGE_ERR_APPLE_OCR           = "apple_ocr"

// ---------------------------------------------------------------------------
// 返回 JSON 的 Go struct 镜像（与 Swift 端 Codable struct 字段一一对应）
// ---------------------------------------------------------------------------
// 每个 Go struct 的 json tag 必须与 Swift 对应 Codable struct 的字段名完全一致；
// 成功 struct 内嵌 BridgeError，使同一份 JSON 既能取成功结果也能在失败时取 code/detail。
// 所有 detail 字段均为「非可翻译技术细节」（尺寸 / 系统错误原文 / 语言标识符），
// 用户可见文案由 Go 端 err.apple_* i18n 渲染，二者拼接为 "用户文案 (技术细节)"。

// BridgeError 镜像 Swift BridgeError：所有函数失败时返回 {"code":...,"detail":...}。
// 成功 JSON 中无此二字段，解析为空字符串（安全）。
type BridgeError struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// TranslateSuccess 镜像 Swift TranslateSuccess：{"result":...,"from":...}。
// 内嵌 BridgeError 以便同一份 JSON 既能取成功结果，也能在失败时取 code/detail。
type TranslateSuccess struct {
	Result string `json:"result"`
	From   string `json:"from"`
	BridgeError
}

// AvailableLanguages 镜像 Swift AvailableLanguages：{"langs":[...]}。
type AvailableLanguages struct {
	Langs []string `json:"langs"`
}

// SelectionPoint 镜像 Swift SelectionPoint：{"x":0,"y":0}。
type SelectionPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ScreenSize 镜像 Swift ScreenSize：{"w":0,"h":0}。
type ScreenSize struct {
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// OCRRegion 镜像 Swift OCRRegion：{"text":...,"conf":0,"box":[x1,y1,x2,y2]}。
type OCRRegion struct {
	Text string  `json:"text"`
	Conf float64 `json:"conf"`
	Box  []int   `json:"box"`
}

// OCRSuccess 镜像 Swift OCRSuccess：{"text":...,"regions":[...]}。
// 内嵌 BridgeError 以便同一份 JSON 既能取识别结果，也能在失败时取 code/detail。
type OCRSuccess struct {
	Text    string      `json:"text"`
	Regions []OCRRegion `json:"regions"`
	BridgeError
}
