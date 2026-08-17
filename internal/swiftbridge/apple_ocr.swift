// apple_ocr.swift
// Vision OCR 相关 @_cdecl 入口：kai_ocr。
// 依赖 bridge_errors.swift（错误码 / Codable / 编码辅助）、bridge_common.swift（writeCString）、
// bridge_log.swift（bridgeFileLog / bridgeLogText）。
import AppKit
import ApplicationServices
import CoreGraphics
import Foundation
import NaturalLanguage
import Translation
import Vision

// kai_ocr：对传入的图片（base64 PNG/JPEG）执行 Vision 文本识别（默认中文，支持更多语言）。
// 参数顺序/类型必须与 Go 端 cgo 声明一致：
//   int kai_ocr(const char* img, char* out, int out_cap, int correct, int timeout_sec);
// out 接收 {"text":"...","regions":[{"text","conf","box"}]}（OCRSuccess）；失败写入 {"code":"...","detail":"..."}（BridgeError）。
// correct 为 0/1（对应 Go 端 usesLanguageCorrection 开关）；timeout_sec 为识别超时秒数（替代原硬编码 15s）。
@_cdecl("kai_ocr")
public func kai_ocr(
  _ base64: UnsafePointer<CChar>?,
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32,
  _ correct: Int32,
  _ timeout_sec: Int32
) -> Int32 {
  let b64 = base64.flatMap { String(cString: $0) } ?? ""
  let correctionOn = correct != 0
  let timeout = max(Int(timeout_sec), 1)
  // 诊断：确认 cgo 调用约定是否生效（out_cap 应收到 1<<20 量级，out 不应为 nil）。
  bridgeFileLog(
    bridgeLogText("ocr.entry", out == nil ? "nil" : "ok", out_cap, correctionOn ? "on" : "off", timeout),
    level: BRIDGE_LOG_DEBUG)
  bridgeFileLog(
    bridgeLogText("ocr.config", correctionOn ? "on" : "off", timeout), level: BRIDGE_LOG_DEBUG)

  guard let data = Data(base64Encoded: b64) else {
    bridgeFileLog(bridgeLogText("ocr.base64_fail"), level: BRIDGE_LOG_WARN)
    return writeCString(
      bridgeErrorJSON(
        code: BRIDGE_ERR_DECODE_FAILED,
        detail: "base64 decode failed (len=\(b64.count))"), into: out, cap: out_cap)
  }
  guard let src = CGImageSourceCreateWithData(data as CFData, nil),
    let cg = CGImageSourceCreateImageAtIndex(src, 0, nil)
  else {
    bridgeFileLog(bridgeLogText("ocr.image_fail"), level: BRIDGE_LOG_WARN)
    return writeCString(
      bridgeErrorJSON(
        code: BRIDGE_ERR_DECODE_FAILED,
        detail: "CGImageSource decode failed (input is not a valid image)"), into: out, cap: out_cap
    )
  }

  let w = cg.width
  let h = cg.height
  guard w > 0, h > 0 else {
    bridgeFileLog(bridgeLogText("ocr.empty_size"), level: BRIDGE_LOG_WARN)
    return writeCString(
      bridgeErrorJSON(code: BRIDGE_ERR_EMPTY_IMAGE, detail: "image size is 0x0"),
      into: out, cap: out_cap)
  }

  // 关键修复：区域截图直接喂 VNImageRequestHandler 偶发 CRImageReaderError error 1。
  // 用 CGContext（RGBA8、noneSkipLast、DeviceRGB）重绘一遍，强制转为 Vision 友好像素格式，
  // 彻底规避该随机失败。
  guard
    let ctx = CGContext(
      data: nil, width: w, height: h, bitsPerComponent: 8,
      bytesPerRow: w * 4, space: CGColorSpaceCreateDeviceRGB(),
      bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)
  else {
    bridgeFileLog(bridgeLogText("ocr.bitmap_fail"), level: BRIDGE_LOG_ERROR)
    return writeCString(
      bridgeErrorJSON(
        code: BRIDGE_ERR_BITMAP_CTX_FAILED,
        detail: "failed to create bitmap context \(w)x\(h)"), into: out, cap: out_cap)
  }
  ctx.draw(cg, in: CGRect(x: 0, y: 0, width: CGFloat(w), height: CGFloat(h)))
  guard let redrawn = ctx.makeImage() else {
    bridgeFileLog(bridgeLogText("ocr.redraw_fail"), level: BRIDGE_LOG_ERROR)
    return writeCString(
      bridgeErrorJSON(
        code: BRIDGE_ERR_BITMAP_REDRAW_FAILED,
        detail: "failed to redraw image \(w)x\(h)"), into: out, cap: out_cap)
  }
  let rawBpp = cg.bitsPerPixel
  let rawAlpha = cg.alphaInfo.rawValue
  let rawCS = cg.colorSpace?.name as String? ?? "unknown"
  let newBpp = redrawn.bitsPerPixel
  let newAlpha = redrawn.alphaInfo.rawValue
  bridgeFileLog(
    bridgeLogText(
      "ocr.redraw_info", rawBpp, rawAlpha, rawCS, newBpp, newAlpha, w, h),
    level: BRIDGE_LOG_DEBUG)

  let request = VNRecognizeTextRequest()
  // correctionOn（Go 端 correct 0/1）：开启语言校正时用 accurate 级别；关闭时仍用 accurate
  // 但关闭 usesLanguageCorrection 以换取更快推理（fast 级别在中文密集场景召回更差，故保留 accurate）。
  request.recognitionLevel = .accurate
  request.usesLanguageCorrection = correctionOn
  request.recognitionLanguages = ["zh-Hans", "zh-Hant", "en", "ja", "ko"]

  let handler = VNImageRequestHandler(cgImage: redrawn, options: [:])
  var resultJSON = "{\"text\":\"\",\"regions\":[]}"
  let sema = DispatchSemaphore(value: 0)
  let start = Date()

  DispatchQueue.global(qos: .userInitiated).async {
    do {
      try handler.perform([request])
      guard let observations = request.results else {
        let detail = "Vision perform returned nil results"
        resultJSON = bridgeErrorJSON(code: BRIDGE_ERR_APPLE_OCR, detail: detail)
        bridgeFileLog(bridgeLogText("ocr.fail", detail), level: BRIDGE_LOG_ERROR)
        sema.signal()
        return
      }
      var lines: [String] = []
      var regions: [OCRRegion] = []
      for obs in observations {
        guard let candidate = obs.topCandidates(1).first else { continue }
        let txt = candidate.string
        let conf = candidate.confidence
        let bb = obs.boundingBox  // 归一化坐标，原点左下
        let x1 = Int(bb.origin.x * CGFloat(w))
        let y1 = Int((1 - bb.origin.y - bb.height) * CGFloat(h))  // 翻转 y 到左上原点
        let x2 = Int((bb.origin.x + bb.width) * CGFloat(w))
        let y2 = Int((1 - bb.origin.y) * CGFloat(h))
        lines.append(txt)
        regions.append(OCRRegion(text: txt, conf: Double(conf), box: [x1, y1, x2, y2]))
      }
      let full = lines.joined(separator: "\n")
      resultJSON = bridgeEncode(OCRSuccess(text: full, regions: regions))
      bridgeFileLog(
        bridgeLogText("ocr.done", regions.count, full.utf8.count), level: BRIDGE_LOG_DEBUG)
    } catch {
      let detail = error.localizedDescription
      resultJSON = bridgeErrorJSON(code: BRIDGE_ERR_APPLE_OCR, detail: detail)
      bridgeFileLog(bridgeLogText("ocr.fail", detail), level: BRIDGE_LOG_ERROR)
    }
    sema.signal()
  }

  let cost = Date().timeIntervalSince(start)
  if sema.wait(timeout: .now() + Double(timeout)) == .timedOut {
    let detail = "Vision perform exceeded \(timeout)s, aborted"
    resultJSON = bridgeErrorJSON(code: BRIDGE_ERR_OCR_TIMEOUT, detail: detail)
    bridgeFileLog(bridgeLogText("ocr.timeout", timeout), level: BRIDGE_LOG_ERROR)
  } else {
    bridgeFileLog(
      bridgeLogText("ocr.perform_cost", String(format: "%.3f", cost)), level: BRIDGE_LOG_DEBUG)
  }
  return writeCString(resultJSON, into: out, cap: out_cap)
}
