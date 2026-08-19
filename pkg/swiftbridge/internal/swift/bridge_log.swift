// bridge_log.swift
// Swift 桥接层日志子系统：等级常量、日志配置 cdecl、中英文日志文案、按天滚动与压缩。
// 所有桥接日志通过 bridgeFileLog 追加写入 dataDir/logs/kai-bridge.log，不带前缀；
// 未设置日志目录（kai_set_log_config 未调用）则不写任何日志。日志等级、保留天数、压缩
// 开关均由 Go 侧 LogConfig 经 kai_set_log_config 传入，与主应用日志（kai.log）保持一致策略。
import AppKit
import ApplicationServices
import CoreGraphics
import Foundation
import NaturalLanguage
import Translation
import Vision

// 日志等级数值映射（数值越大越严重，仅当 level >= bridgeLogLevel 才落盘）。
let BRIDGE_LOG_DEBUG: Int = 0
let BRIDGE_LOG_INFO: Int = 1
let BRIDGE_LOG_WARN: Int = 2
let BRIDGE_LOG_ERROR: Int = 3

// 日志目录由 Go 在启动时通过 kai_set_log_config 设置（指向 dataDir/logs）；
// 未设置则为 nil，bridgeFileLog 直接返回。等级/保留天数/压缩同步来自 LogConfig。
var bridgeLogDir: URL? = nil
var bridgeLogLevel: Int = BRIDGE_LOG_INFO
var bridgeRetentionDays: Int = 30
var bridgeCompress: Bool = true

// 当前日志语言：由 Go 经 kai_set_locale 设置；"zh" 输出中文日志，"en" 输出英文日志，默认 zh。
var bridgeLogLocale: String = "zh"

// kai_set_log_config 由 Go 在启动时及配置热更新时调用，统一设置桥接层日志目录与策略。
// dir：日志目录（dataDir/logs）；level：debug/info/warn/error（非法回退 info）；
// retention_days：保留天数（<=0 表示仅按天滚动、不清理）；compress：过期归档是否压缩为 .gz。
@_cdecl("kai_set_log_config")
public func kai_set_log_config(
  _ dir: UnsafePointer<CChar>?,
  _ level: UnsafePointer<CChar>?,
  _ retention_days: Int32,
  _ compress: Bool
) {
  if let dir = dir {
    let path = String(cString: dir)
    let url = URL(fileURLWithPath: path, isDirectory: true)
    try? FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
    bridgeLogDir = url
  }
  if let level = level {
    switch String(cString: level).lowercased() {
    case "debug": bridgeLogLevel = BRIDGE_LOG_DEBUG
    case "warn", "warning": bridgeLogLevel = BRIDGE_LOG_WARN
    case "error": bridgeLogLevel = BRIDGE_LOG_ERROR
    default: bridgeLogLevel = BRIDGE_LOG_INFO
    }
  }
  bridgeRetentionDays = Int(retention_days)
  bridgeCompress = compress
  bridgeFileLog(
    bridgeLogText(
      "log.config_applied", bridgeLogDir?.path ?? "", bridgeLogLevel, bridgeRetentionDays,
      String(bridgeCompress)), level: BRIDGE_LOG_INFO)
}

// kai_set_locale 由 Go 在启动时及语言切换时调用，设置桥接层日志输出语言。
// locale：形如 "zh-CN" / "en-US" 的语言码；以 "en" 开头视为英文，其余按中文处理。
@_cdecl("kai_set_locale")
public func kai_set_locale(_ locale: UnsafePointer<CChar>?) {
  guard let locale = locale else { return }
  let l = String(cString: locale).lowercased()
  bridgeLogLocale = l.hasPrefix("en") ? "en" : "zh"
}

// bridgeLogText 按当前 bridgeLogLocale 返回中/英日志文案。
// key 为稳定英文标识（与 Go i18n 表解耦）；Swift 仅维护这一张轻量小表。
// 文案中的 %@ / %d 占位符与 String(format:) 对应。
func bridgeLogText(_ key: String, _ args: CVarArg...) -> String {
  let zh: [String: String] = [
    "log.config_applied": "日志配置已应用 dir=%@ level=%d retention_days=%d compress=%@",
    "translate.call": "系统翻译调用 src=%@ dst=%@ 文本长度=%d",
    "translate.empty": "系统翻译失败: 文本为空",
    "translate.no_target": "系统翻译失败: 缺少目标语言",
    "translate.detect_src": "系统翻译自动检测源语言=%@ dst=%@",
    "translate.detect_fail": "系统翻译自动检测失败且无已安装语言回退 dst=%@",
    "translate.fail": "系统翻译失败: %@",
    "translate.done": "系统翻译完成 from=%@ dst=%@ 译文长度=%d",
    "lang.query_done": "系统已安装语言列表查询完成 总数=%d 已安装=%d",
    "a11y.query": "辅助功能授权查询 结果=%@",
    "a11y.request": "辅助功能授权请求 弹出系统授权框并尝试打开设置面板",
    "a11y.request_done": "辅助功能授权请求 已打开系统设置面板",
    "screen.query": "屏幕录制授权查询 结果=%@",
    "screen.request": "屏幕录制授权请求 打开系统设置面板",
    "screen.request_done": "屏幕录制授权请求 已打开系统设置面板",
    "selection.point_done": "选区坐标读取完成 x=%@ y=%@",
    "ocr.base64_fail": "系统 OCR 失败: base64 解码失败",
    "ocr.image_fail": "系统 OCR 失败: 图片解码失败",
    "ocr.empty_size": "系统 OCR 失败: 图片尺寸为空",
    "ocr.bitmap_fail": "系统 OCR 失败: 位图上下文创建失败",
    "ocr.redraw_fail": "系统 OCR 失败: 位图重绘失败",
    "ocr.redraw_info": "系统 OCR 重绘前 raw bpp=%d alpha=%d cs=%@ | 重绘后 bpp=%d alpha=%d w=%d h=%d",
    "ocr.fail": "系统 OCR 失败: %@",
    "ocr.done": "系统 OCR 完成 行数=%d 首行长度=%d",
    "ocr.entry": "系统 OCR 入口 out=%@ cap=%d correct=%@ timeout=%d",
    "ocr.config": "系统 OCR 配置 correction=%@ level=%d",
    "ocr.perform_cost": "系统 OCR perform 实际耗时=%@s",
    "ocr.timeout": "系统 OCR 超时: perform 超过 %d s 未完成",
    "selection.fail_app": "选区读取 无法获取前台应用 axErr=%d",
    "selection.empty": "选区读取结果为空 axErr=%d",
    "selection.done": "选区读取完成 长度=%d",
    "input.tap_fail": "输入监控检测 创建 EventTap 失败（未授权或被策略拒绝）",
    "input.tap_enabled": "输入监控检测 tapIsEnabled=%@",
    "input.settings": "输入监控 打开系统设置面板",
  ]
  let en: [String: String] = [
    "log.config_applied": "log config applied dir=%@ level=%d retention_days=%d compress=%@",
    "translate.call": "system translate call src=%@ dst=%@ text_len=%d",
    "translate.empty": "translate failed: empty text",
    "translate.no_target": "translate failed: missing target language",
    "translate.detect_src": "system translate auto-detected source=%@ dst=%@",
    "translate.detect_fail":
      "system translate auto-detect failed and no installed language fallback dst=%@",
    "translate.fail": "translate failed: %@",
    "translate.done": "system translate done from=%@ dst=%@ result_len=%d",
    "lang.query_done": "installed language list query done total=%d installed=%d",
    "a11y.query": "accessibility authorization query result=%@",
    "a11y.request":
      "accessibility authorization request: popup system dialog and try opening settings panel",
    "a11y.request_done": "accessibility authorization request: settings panel opened",
    "screen.query": "screen recording authorization query result=%@",
    "screen.request": "screen recording authorization request: open settings panel",
    "screen.request_done": "screen recording authorization request: settings panel opened",
    "selection.point_done": "selection point read done x=%@ y=%@",
    "ocr.base64_fail": "system OCR failed: base64 decode failed",
    "ocr.image_fail": "system OCR failed: image decode failed",
    "ocr.empty_size": "system OCR failed: image size empty",
    "ocr.bitmap_fail": "system OCR failed: bitmap context creation failed",
    "ocr.redraw_fail": "system OCR failed: bitmap redraw failed",
    "ocr.redraw_info":
      "system OCR before redraw raw bpp=%d alpha=%d cs=%@ | after redraw bpp=%d alpha=%d w=%d h=%d",
    "ocr.fail": "system OCR failed: %@",
    "ocr.done": "system OCR done lines=%d first_len=%d",
    "ocr.entry": "system OCR entry out=%@ cap=%d correct=%@ timeout=%d",
    "ocr.config": "system OCR config correction=%@ level=%d",
    "ocr.perform_cost": "system OCR perform actual cost=%@s",
    "ocr.timeout": "system OCR timeout: perform exceeded %d s",
    "selection.fail_app": "selection read failed: cannot get front app axErr=%d",
    "selection.empty": "selection read result empty axErr=%d",
    "selection.done": "selection read done length=%d",
    "input.tap_fail":
      "input monitoring check: create EventTap failed (not authorized or blocked by policy)",
    "input.tap_enabled": "input monitoring tapIsEnabled=%@",
    "input.settings": "input monitoring: open system settings panel",
  ]
  guard let tmpl = (bridgeLogLocale == "en" ? en : zh)[key] else {
    // 缺翻译时直接透传 key，便于发现遗漏
    return key
  }
  return String(format: tmpl, arguments: args)
}

// bridgeDayString 返回日期的 2006-01-02 形式，用于按天滚动的归档文件名。
private func bridgeDayString(_ d: Date) -> String {
  let f = DateFormatter()
  f.timeZone = TimeZone.current
  f.dateFormat = "yyyy-MM-dd"
  return f.string(from: d)
}

// bridgeGzip 调用系统 gzip 压缩文件，成功则删除原文件；失败静默返回（保留原文件）。
private func bridgeGzip(_ path: String) {
  let proc = Process()
  proc.executableURL = URL(fileURLWithPath: "/usr/bin/gzip")
  proc.arguments = [path]
  do {
    try proc.run()
    proc.waitUntilExit()
  } catch {
    return
  }
}

// 追加一行到 kai-bridge.log（ISO8601 时间戳，本地时区，无前缀）。
// level 决定等级过滤：默认 info；低于当前 bridgeLogLevel 的日志直接丢弃。
// 写入前按天滚动：若 kai-bridge.log 的 mtime 非今天，则归档为 kai-bridge-YYYY-MM-DD.log
// （bridgeCompress 为真时压缩为 .gz），归档文件名沿用 kai- 前缀，由 Go 侧统一清理策略覆盖。
func bridgeFileLog(_ message: String, level: Int = BRIDGE_LOG_INFO) {
  guard level >= bridgeLogLevel else { return }
  guard let dir = bridgeLogDir else { return }
  let url = dir.appendingPathComponent("kai-bridge.log")

  // 按天滚动：当前文件 mtime 非今天则归档（先压缩再保留，供 Go cleanup 删除过期）。
  if let attrs = try? FileManager.default.attributesOfItem(atPath: url.path),
    let mtime = attrs[.modificationDate] as? Date,
    !Calendar.current.isDateInToday(mtime)
  {
    let archiveName = "kai-bridge-\(bridgeDayString(mtime)).log"
    let archivePath = dir.appendingPathComponent(archiveName).path
    let uniqueArchive = uniqueBridgePath(archivePath)
    if (try? FileManager.default.moveItem(atPath: url.path, toPath: uniqueArchive)) != nil {
      if bridgeCompress {
        bridgeGzip(uniqueArchive)
      }
    }
  }

  let fmt = ISO8601DateFormatter()
  fmt.timeZone = TimeZone.current
  let ts = fmt.string(from: Date())
  let line = "\(ts) \(message)\n"
  if let data = line.data(using: .utf8) {
    let fd = open(url.path, O_WRONLY | O_CREAT | O_APPEND, 0o644)
    if fd >= 0 {
      _ = data.withUnsafeBytes { write(fd, $0.baseAddress, $0.count) }
      close(fd)
    }
  }
}

// uniqueBridgePath 若 path 已存在则在中段插入 .N 避免覆盖。
private func uniqueBridgePath(_ path: String) -> String {
  if !FileManager.default.fileExists(atPath: path) { return path }
  let url = URL(fileURLWithPath: path)
  let ext = url.pathExtension
  let base = url.deletingPathExtension().path
  var i = 1
  while true {
    let cand = "\(base).\(i).\(ext)"
    if !FileManager.default.fileExists(atPath: cand) { return cand }
    i += 1
  }
}
