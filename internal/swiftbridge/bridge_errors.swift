// bridge_errors.swift
// Swift 桥接层「双端契约」：错误码常量 + 对外 JSON 的 Codable 模型。
// 与 Go 侧 internal/swiftbridge/bridge_errors.go 一一对应（字面量 / 字段名必须一致）。
//
// 新增 / 修改任一错误码时，必须同步三处，缺一不可：
//   ① Go 侧 bridge_errors.go 的 BridgeErr* 常量（真实定义）
//   ② 本文件的 BRIDGE_ERR_* 常量（Swift 字面量必须一致）
//   ③ internal/i18n/locales/split/ 的 err.apple_<code> key
//      —— Apple 系统级错误（apple_translate / apple_ocr）复用通用引擎文案，不新增 key。
import Foundation

// Swift 桥接层自定义错误码（与 Go 侧 BridgeErr* 常量字面量保持一致）。
let BRIDGE_ERR_EMPTY_TEXT: String = "empty_text"
let BRIDGE_ERR_TARGET_REQUIRED: String = "target_required"
let BRIDGE_ERR_NULL_IMAGE: String = "null_image"
let BRIDGE_ERR_DECODE_FAILED: String = "decode_failed"
let BRIDGE_ERR_EMPTY_IMAGE: String = "empty_image"
let BRIDGE_ERR_BITMAP_CTX_FAILED: String = "bitmap_ctx_failed"
let BRIDGE_ERR_BITMAP_REDRAW_FAILED: String = "bitmap_redraw_failed"
let BRIDGE_ERR_OCR_TIMEOUT: String = "ocr_timeout"
let BRIDGE_ERR_NO_SOURCE_LANG: String = "no_source_lang"
// Apple 系统级翻译/识别错误（如 Translation.framework / Vision 抛出的本地化错误）。
// detail 携带系统 localizedDescription，非可翻译文案，仅作技术细节。
let BRIDGE_ERR_APPLE_TRANSLATE: String = "apple_translate"
let BRIDGE_ERR_APPLE_OCR: String = "apple_ocr"

// MARK: - 桥接层返回 JSON 的 Codable 模型
// 所有对外（cgo）返回的 JSON 统一用 Codable struct + JSONEncoder 编码，
// 字段名必须与 Go 端对应 struct 的 json tag 完全一致（见 bridge_errors.go）。
// 注意：cgo 边界只能传 C 类型，故 Swift 内部用 struct 组织，再编码成 String 写回 out 缓冲区。

/// 错误返回：{"code":"...","detail":"..."}（detail 为非可翻译的技术细节）。
struct BridgeError: Codable {
  let code: String
  let detail: String
}

/// 翻译成功：{"result":"...","from":"..."}
struct TranslateSuccess: Codable {
  let result: String
  let from: String
}

/// 可用语言列表：{"langs":["...","..."]}
struct AvailableLanguages: Codable {
  let langs: [String]
}

/// 选区锚点：{"x":0,"y":0}
struct SelectionPoint: Codable {
  let x: Double
  let y: Double
}

/// 主屏分辨率：{"w":0,"h":0}
struct ScreenSize: Codable {
  let w: Double
  let h: Double
}

/// OCR 单区域：{"text":"...","conf":0,"box":[x1,y1,x2,y2]}
struct OCRRegion: Codable {
  let text: String
  let conf: Double
  let box: [Int]
}

/// OCR 成功：{"text":"...","regions":[...]}
struct OCRSuccess: Codable {
  let text: String
  let regions: [OCRRegion]
}

/// 把任意 Codable 编码为紧凑 JSON 字符串；失败回退空对象 "{}"（Go 端会按默认错误文案处理）。
func bridgeEncode<T: Codable>(_ value: T) -> String {
  guard let data = try? JSONEncoder().encode(value),
    let str = String(data: data, encoding: .utf8)
  else {
    return "{}"
  }
  return str
}

/// 便捷构造错误 JSON 字符串。
func bridgeErrorJSON(code: String, detail: String) -> String {
  bridgeEncode(BridgeError(code: code, detail: detail))
}
