// bridge_common.swift
// Swift 桥接层共享基础设施：C 缓冲区写入 + 源语言自动检测。
// 被各 apple_*.swift 的 @_cdecl 入口复用。
import AppKit
import ApplicationServices
import CoreGraphics
import Foundation
import NaturalLanguage
import Translation
import Vision

// 日志等级数值映射（数值越大越严重，仅当 level >= bridgeLogLevel 才落盘）。
// 定义于 bridge_log.swift 的 BRIDGE_LOG_*，此处仅声明便于本文件引用；
// 实际常量在 bridge_log.swift 中定义（同编译单元可见）。

// 把 Swift String 写入调用方提供的 C 缓冲区（含结尾\0）。
// 返回实际写入长度（不含\0），超容量则返回 -1。
func writeCString(_ str: String, into buf: UnsafeMutablePointer<CChar>?, cap: Int32) -> Int32 {
  guard let buf = buf else { return -1 }
  let capInt = Int(cap)
  guard capInt > 1 else { return -1 }
  let bytes = Array(str.utf8)  // 不含 \0，长度 = 实际内容字节数
  let maxLen = min(bytes.count, capInt - 1)
  for i in 0..<maxLen {
    buf[i] = CChar(bitPattern: bytes[i])
  }
  buf[maxLen] = 0
  return Int32(maxLen)
}

// detectSourceLanguage：用 NaturalLanguage 的 LanguageRecognizer 自动识别文本主导语言，
// 并在本机已安装语言列表中求交集，保证返回的源语言一定是已下载、可直接用于 TranslationSession 的。
// installed：本机已安装语言码集合（小写形式用于匹配）。
// detectedLang：输出参数，返回 NaturalLanguage 识别到的原始主导语言码（无论是否已安装），用于错误提示。
// 返回已安装的匹配源语言码；若识别失败或识别出的语言未安装，则回退到 installed 中的首选（通常第一个，如 en），
// 若 installed 为空则返回 nil。
func detectSourceLanguage(_ text: String, installed: [String], detectedLang: inout String?)
  -> String?
{
  let recognizer = NLLanguageRecognizer()
  recognizer.processString(text)
  guard let detected = recognizer.dominantLanguage?.rawValue else {
    detectedLang = nil
    return installed.first
  }
  detectedLang = detected
  let detectedLower = detected.lowercased()
  if let exact = installed.first(where: { $0.lowercased() == detectedLower }) { return exact }
  if let prefix = installed.first(where: { $0.lowercased().hasPrefix(detectedLower) }) {
    return prefix
  }
  if let rev = installed.first(where: { detectedLower.hasPrefix($0.lowercased()) }) { return rev }
  return installed.first
}
