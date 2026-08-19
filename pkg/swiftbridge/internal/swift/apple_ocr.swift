// apple_ocr.swift
// Vision OCR 相关 @_cdecl 入口：kai_ocr。
// 依赖 bridge_errors.swift（错误码 / Codable / 编码辅助）、bridge_common.swift（writeCString）、
// bridge_log.swift（bridgeFileLog / bridgeLogText）。
import AppKit
import ApplicationServices
import CoreGraphics
import Foundation
import ImageIO
import NaturalLanguage
import Translation
import UniformTypeIdentifiers
import Vision

// kai_ocr：对传入的图片（base64 PNG/JPEG）执行 Vision 文本识别（默认中文，支持更多语言）。
// 参数顺序/类型必须与 Go 端 cgo 声明一致：
//   int kai_ocr(const char* img, char* out, int out_cap, int correct, int timeout_sec);
// out 接收 {"text":"...","regions":[{"text","conf","box"}]}（OCRSuccess）；失败写入 {"code":"...","detail":"..."}（BridgeError）。
// correct 为 0/1（对应 Go 端 usesLanguageCorrection 开关）；timeout_sec 为识别超时秒数（替代原硬编码 15s）；
// retry 为 Vision OCR 失败兜底重试次数（<=0 用默认 2），仅对 CRImageReaderError 类瞬拒生效。
@_cdecl("kai_ocr")
public func kai_ocr(
  _ base64: UnsafePointer<CChar>?,
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32,
  _ correct: Int32,
  _ timeout_sec: Int32,
  _ retry: Int32
) -> Int32 {
  let b64 = base64.flatMap { String(cString: $0) } ?? ""
  let correctionOn = correct != 0
  let timeout = max(Int(timeout_sec), 1)
  let retryCount = max(Int(retry), 0)
  // 诊断：确认 cgo 调用约定是否生效（out_cap 应收到 1<<20 量级，out 不应为 nil）。
  bridgeFileLog(
    bridgeLogText(
      "ocr.entry", out == nil ? "nil" : "ok", out_cap, correctionOn ? "on" : "off", timeout),
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

  // 像素规范化（CRImageReaderError error 1 根治）：
  // 区域截图直接喂 VNImageRequestHandler 偶发 CRImageReaderError error 1（perform 0.000s 瞬拒）。
  // 根因是 Vision 对带 alpha 通道 / 色彩空间未知 / 半透明像素的图会直接拒识。
  // 双重兜底：
  //  ① CGContext 重绘为「标准 sRGB + 8bit + noneSkipLast（A 置 255 不透明）」，去掉退化色彩空间与半透明语义；
  //     注：本机 macOS 26.2 上 CGImageAlphaInfo.none 的 bitmap context 创建会返回 nil（实测
  //     failed to create bitmap context），故用 noneSkipLast 保底确保 context 可创建，A=255 不透明 Vision 兼容良好。
  //  ② 再经 JPEG 容器重封装读回（JPEG 无 alpha，强制丢弃半透明像素），是真正去半透明的关键一步。
  // 关键：归一化抽成 normalizedVariant(_:)，在失败重试循环里**每次都重新归一化原始 cg**（而非复用同一张），
  // 因为单次归一化在边缘图上可能未完全治愈 error 1，重新走一遍 JPEG 重封装路径有时能成功。
  // noneSkipLast（RGBA，A 不透明）每像素 4 字节 → bytesPerRow 用 w*4（按 16 字节对齐）。
  func normalizedVariant(_ src: CGImage) -> CGImage {
    let rw = src.width
    let rh = src.height
    let bytesPerRow = (rw * 4 + 15) & ~15
    var work: CGImage = src  // 任一步失败都兜底用上一步结果，避免整体失败
    if let ctx = CGContext(
      data: nil, width: rw, height: rh, bitsPerComponent: 8,
      bytesPerRow: bytesPerRow, space: CGColorSpaceCreateDeviceRGB(),
      bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)
    {
      ctx.draw(src, in: CGRect(x: 0, y: 0, width: CGFloat(rw), height: CGFloat(rh)))
      if let img = ctx.makeImage() {
        work = img
      } else {
        bridgeFileLog(bridgeLogText("ocr.redraw_fail"), level: BRIDGE_LOG_WARN)
      }
    } else {
      bridgeFileLog(bridgeLogText("ocr.bitmap_fail"), level: BRIDGE_LOG_WARN)
    }
    if let jpegData = normalizeToJPEG(work) {
      let hint =
        [kCGImageSourceTypeIdentifierHint: UTType.jpeg.identifier as CFString] as CFDictionary
      if let srcImg = CGImageSourceCreateWithData(jpegData as CFData, hint)
        .flatMap({ CGImageSourceCreateImageAtIndex($0, 0, hint) })
      {
        let reBytesPerRow = (srcImg.width * 4 + 15) & ~15
        let reCtx = CGContext(
          data: nil, width: srcImg.width, height: srcImg.height, bitsPerComponent: 8,
          bytesPerRow: reBytesPerRow, space: CGColorSpaceCreateDeviceRGB(),
          bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)
        reCtx?.draw(
          srcImg,
          in: CGRect(x: 0, y: 0, width: CGFloat(srcImg.width), height: CGFloat(srcImg.height)))
        if let reImg = reCtx?.makeImage() {
          work = reImg
          bridgeFileLog(
            bridgeLogText(
              "ocr.normalized", work.bitsPerPixel, work.alphaInfo.rawValue, work.width, work.height),
            level: BRIDGE_LOG_DEBUG)
        } else {
          bridgeFileLog(bridgeLogText("ocr.normalize_redraw_fail"), level: BRIDGE_LOG_WARN)
        }
      } else {
        bridgeFileLog(bridgeLogText("ocr.normalize_fail"), level: BRIDGE_LOG_WARN)
      }
    } else {
      bridgeFileLog(bridgeLogText("ocr.normalize_skip"), level: BRIDGE_LOG_WARN)
    }
    return work
  }

  let rawBpp = cg.bitsPerPixel
  let rawAlpha = cg.alphaInfo.rawValue
  let rawCS = cg.colorSpace?.name as String? ?? "unknown"
  bridgeFileLog(
    bridgeLogText(
      "ocr.redraw_info", rawBpp, rawAlpha, rawCS, cg.bitsPerPixel, cg.alphaInfo.rawValue, w, h),
    level: BRIDGE_LOG_DEBUG)

  // 初始归一化（首试用），重试时按 attempt 重新归一化原始 cg。
  let redrawn: CGImage = normalizedVariant(cg)

  let request = VNRecognizeTextRequest()
  // correctionOn（Go 端 correct 0/1）：开启语言校正时用 accurate 级别；关闭时仍用 accurate
  // 但关闭 usesLanguageCorrection 以换取更快推理（fast 级别在中文密集场景召回更差，故保留 accurate）。
  request.recognitionLevel = .accurate
  request.usesLanguageCorrection = correctionOn
  // 收敛语言候选：cs=unknown 退化图上多语言候选会加大 Vision 内部解码器崩溃概率（error 1）。
  // 主候选集只保留最常用的中/英/繁，避免 ja/ko 这类在退化图上触发拒识。
  let primaryLangs = ["zh-Hans", "zh-Hant", "en"]
  request.recognitionLanguages = primaryLangs

  var resultJSON = "{\"text\":\"\",\"regions\":[]}"
  let sema = DispatchSemaphore(value: 0)
  let start = Date()

  // performOCR 执行一次 Vision 识别；返回 nil 表示成功（resultJSON 已写好），
  // 返回非 nil 表示失败详情（含 error 1 等），供调用方决定重试。img 为本次要识别的（已归一化）像素。
  func performOCR(_ langs: [String], _ img: CGImage) -> String? {
    let req = VNRecognizeTextRequest()
    req.recognitionLevel = .accurate
    req.usesLanguageCorrection = correctionOn
    req.recognitionLanguages = langs
    let h = VNImageRequestHandler(cgImage: img, options: [:])
    do {
      try h.perform([req])
      guard let observations = req.results, !observations.isEmpty else {
        return "Vision perform returned nil/empty results"
      }
      var lines: [String] = []
      var regions: [OCRRegion] = []
      for obs in observations {
        guard let candidate = obs.topCandidates(1).first else { continue }
        let txt = candidate.string
        let conf = candidate.confidence
        let bb = obs.boundingBox  // 归一化坐标，原点左下
        let x1 = Int(bb.origin.x * CGFloat(w))
        let y1 = Int((1 - bb.origin.y - bb.height) * CGFloat(img.height))  // 翻转 y 到左上原点
        let x2 = Int((bb.origin.x + bb.width) * CGFloat(w))
        let y2 = Int((1 - bb.origin.y) * CGFloat(img.height))
        lines.append(txt)
        regions.append(OCRRegion(text: txt, conf: Double(conf), box: [x1, y1, x2, y2]))
      }
      let full = lines.joined(separator: "\n")
      resultJSON = bridgeEncode(OCRSuccess(text: full, regions: regions))
      bridgeFileLog(
        bridgeLogText("ocr.done", regions.count, full.utf8.count), level: BRIDGE_LOG_DEBUG)
      return nil
    } catch {
      return error.localizedDescription
    }
  }

  DispatchQueue.global(qos: .userInitiated).async {
    // 兜底重试：依次尝试不同语言候选集 + 每次重新归一化原始像素，绕过 Vision 在该图上
    // 对多语言候选 / 退化像素格式的崩溃（CRImageReaderError error 1）。
    // 语言候选集优先级：主候选集(中/英/繁) → 单一中文 → 单一英文（用尽后循环复用）。
    // 每轮都从原始 cg 重新归一化（normalizedVariant），单次归一化在边缘图上可能未治愈 error 1，
    // 重新走一遍 JPEG 重封装路径有时能成功——这是 error 1 必复现场景的关键兜底。
    // retry 为用户配置的「额外语言候选重试次数」（不含首试）；但 CRImageReaderError error 1
    // 是像素格式瞬拒，重归一化是关键兜底，不应被用户关掉——故 maxAttempts 下限保底 2
    // （首试 + 至少 1 次重归一化重试），确保 error 1 必复现场景仍有机会自愈。
    let langCandidates: [[String]] = [primaryLangs, ["zh-Hans"], ["en"]]
    let maxAttempts = max(1 + max(retryCount, 0), 2)  // 至少 2 次：首试 + 至少 1 次重归一化
    var detail: String?
    var attempt = 0
    while attempt < maxAttempts {
      attempt += 1
      let langs = langCandidates[(attempt - 1) % langCandidates.count]
      // 第 1 次用初始归一化 redrawn；之后每次从原始 cg 重新归一化，换一条像素路径再试。
      let img = attempt == 1 ? redrawn : normalizedVariant(cg)
      detail = performOCR(langs, img)
      if detail == nil { break }  // 成功，退出重试循环
      // 非 CRImageReaderError（如 timeout）不重试，直接上报
      if !detail!.contains("CRImageReaderError") { break }
      if attempt >= maxAttempts { break }  // 已达重试上限
      bridgeFileLog(
        bridgeLogText("ocr.retry", attempt, langs.joined(separator: ","), detail!),
        level: BRIDGE_LOG_WARN)
    }
    if let d = detail {
      resultJSON = bridgeErrorJSON(code: BRIDGE_ERR_APPLE_OCR, detail: d)
      bridgeFileLog(bridgeLogText("ocr.fail", d), level: BRIDGE_LOG_ERROR)
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

// normalizeToJPEG 把 CGImage 编码为 JPEG（sRGB、8bit、无 alpha），返回规范化后的图片数据。
// 用于根治 VNImageRequestHandler 的 CRImageReaderError error 1：经 JPEG 容器重封装后，
// 像素被强制转为 Vision 兼容性最好的格式，规避源图色彩空间未知 / 半透明边缘导致的拒识。
private func normalizeToJPEG(_ image: CGImage) -> Data? {
  let mutData = NSMutableData()
  guard
    let dest = CGImageDestinationCreateWithData(
      mutData as CFMutableData, UTType.jpeg.identifier as CFString, 1, nil)
  else {
    return nil
  }
  let opts =
    [
      kCGImageDestinationLossyCompressionQuality: 0.92
    ] as CFDictionary
  CGImageDestinationAddImage(dest, image, opts)
  guard CGImageDestinationFinalize(dest) else {
    return nil
  }
  return mutData as Data
}
