// apple_translate.swift
// 系统翻译相关 @_cdecl 入口：kai_translate / kai_available_languages。
// 依赖 bridge_errors.swift（错误码 / Codable / 编码辅助）、bridge_common.swift（detectSourceLanguage）、
// bridge_log.swift（bridgeFileLog / bridgeLogText）。
import AppKit
import ApplicationServices
import CoreGraphics
import Foundation
import NaturalLanguage
import Translation
import Vision

// kai_translate：同步执行一次系统翻译。
// src/dst 为 BCP-47 语言码（如 "en" / "zh-Hans"）；src 可为空字符串表示自动检测。
// out 接收 JSON：{"result":"...","from":"..."}（TranslateSuccess）；失败 out 写入 {"code":"...","detail":"..."}（BridgeError）。
@_cdecl("kai_translate")
public func kai_translate(
  _ src: UnsafePointer<CChar>?,
  _ dst: UnsafePointer<CChar>?,
  _ text: UnsafePointer<CChar>?,
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let sourceCode = src.flatMap { String(cString: $0) } ?? ""
  let targetCode = dst.flatMap { String(cString: $0) } ?? ""
  let inputText = text.flatMap { String(cString: $0) } ?? ""

  bridgeFileLog(bridgeLogText("translate.call", sourceCode, targetCode, inputText.utf8.count))

  guard !inputText.isEmpty else {
    bridgeFileLog(bridgeLogText("translate.empty"), level: BRIDGE_LOG_WARN)
    // detail: 入参文本长度（已 trim 前），便于 Go 侧排查空传。
    return writeCString(
      bridgeErrorJSON(
        code: BRIDGE_ERR_EMPTY_TEXT,
        detail: "input text is empty (len=0)"), into: out, cap: out_cap)
  }
  guard !targetCode.isEmpty else {
    bridgeFileLog(bridgeLogText("translate.no_target"), level: BRIDGE_LOG_WARN)
    return writeCString(
      bridgeErrorJSON(
        code: BRIDGE_ERR_TARGET_REQUIRED,
        detail: "target language code is empty"), into: out, cap: out_cap)
  }

  let targetLang = Locale.Language(identifier: targetCode)

  let sema = DispatchSemaphore(value: 0)
  var resultJSON = "{}"

  Task {
    let availability = LanguageAvailability()
    let installedAll = await availability.supportedLanguages
    var installed: [String] = []
    for lang in installedAll {
      if await availability.status(from: lang, to: nil) == .installed {
        installed.append(lang.maximalIdentifier)
      }
    }

    var effectiveSource = sourceCode
    var detectedLang: String? = nil
    if sourceCode.isEmpty {
      if let detected = detectSourceLanguage(
        inputText, installed: installed, detectedLang: &detectedLang)
      {
        effectiveSource = detected
        bridgeFileLog(bridgeLogText("translate.detect_src", effectiveSource, targetCode))
      } else {
        bridgeFileLog(bridgeLogText("translate.detect_fail", targetCode), level: BRIDGE_LOG_ERROR)
      }
    }
    guard !effectiveSource.isEmpty else {
      let installedDesc =
        installed.isEmpty
        ? "no installed language pack" : "installed: \(installed.joined(separator: ", "))"
      let detectedDesc = detectedLang.map { "detected source: \($0)" } ?? ""
      let detail =
        "no installed source language for auto-detect\(detectedDesc). \(installedDesc). download the language pack in system settings > general > language & region > translate."
      bridgeFileLog(bridgeLogText("translate.fail", detail), level: BRIDGE_LOG_ERROR)
      // 返回结构化错误码，由 Go 侧 err.apple_no_source_lang 渲染用户可见文案；
      // detail 仅作技术细节附在后面（含 Apple 语言标识符，非可翻译文案）。
      resultJSON = bridgeErrorJSON(code: BRIDGE_ERR_NO_SOURCE_LANG, detail: detail)
      sema.signal()
      return
    }
    let sourceLang = Locale.Language(identifier: effectiveSource)
    let session = TranslationSession(installedSource: sourceLang, target: targetLang)
    do {
      try await session.prepareTranslation()
      let resp = try await session.translate(inputText)
      let from = resp.sourceLanguage.languageCode?.identifier ?? sourceCode
      resultJSON = bridgeEncode(TranslateSuccess(result: resp.targetText, from: from))
      bridgeFileLog(bridgeLogText("translate.done", from, targetCode, resp.targetText.utf8.count))
    } catch {
      // Apple 系统级翻译错误：统一为 {"code":"apple_translate","detail":...} 结构，
      // 由 Go 侧 err.apple_translate_engine 渲染，detail 携带系统 localizedDescription。
      let detail = error.localizedDescription
      resultJSON = bridgeErrorJSON(code: BRIDGE_ERR_APPLE_TRANSLATE, detail: detail)
      bridgeFileLog(bridgeLogText("translate.fail", detail), level: BRIDGE_LOG_ERROR)
    }
    sema.signal()
  }

  _ = sema.wait(timeout: .now() + 20)
  return writeCString(resultJSON, into: out, cap: out_cap)
}

// kai_available_languages：通过 LanguageAvailability 查询本机已下载（已安装、可离线翻译）的语言，
// 返回 {"langs":[...]}。
@_cdecl("kai_available_languages")
public func kai_available_languages(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let sema = DispatchSemaphore(value: 0)
  var resultJSON = "{\"langs\":[]}"

  Task {
    let availability = LanguageAvailability()
    let all = await availability.supportedLanguages
    var installed: [String] = []
    for lang in all {
      let status = await availability.status(from: lang, to: nil)
      if status == .installed {
        installed.append(lang.maximalIdentifier)
      }
    }
    resultJSON = bridgeEncode(AvailableLanguages(langs: installed))
    bridgeFileLog(
      bridgeLogText("lang.query_done", all.count, installed.count), level: BRIDGE_LOG_DEBUG)
    sema.signal()
  }

  _ = sema.wait(timeout: .now() + 30)
  return writeCString(resultJSON, into: out, cap: out_cap)
}
