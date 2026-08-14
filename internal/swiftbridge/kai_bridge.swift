//go:build ignore

// kai_bridge.swift
// Swift 桥接层：把 Apple Translation.framework 与 macOS 辅助功能授权暴露为 C 接口，供 cgo 链接。
//
// 编译（见 build.sh，部署目标由 SDK 版本自动决定，最低 26.0）：
//   swiftc -c -parse-as-library -o kai_bridge.o kai_bridge.swift \
//     -framework Translation -framework ApplicationServices -target arm64-apple-macosx$(SDK_MIN)
//   ar rcs libkai_translate.a kai_bridge.o
//
// 暴露的 C 接口：
//   int kai_translate(const char* src, const char* dst, const char* text,
//                     char* out, int out_cap);
//   int kai_available_languages(char* out, int out_cap);
//   void kai_set_log_config(const char* dir, const char* level, int retention_days, bool compress);
//       // 由 Go 在启动时传入日志目录与 LogConfig（等级/保留天数/压缩），桥接层日志统一受其控制
//   int kai_accessibility_enabled(void);     // 辅助功能已授权返回 1，否则 0
//   void kai_accessibility_request(void);    // 弹系统辅助功能授权框（仅首次）+ 打开系统辅助功能设置面板
//   int kai_input_monitoring_enabled(void);  // 输入监控已授权返回 1，否则 0
//   void kai_input_monitoring_request(void); // 打开系统输入监控设置面板
//   int kai_selected_text(char* out, int out_cap);  // 通过辅助功能读取前台 app 当前选区文本
//   int kai_selection_point(char* out, int out_cap); // 取前台 app 窗口锚点，返回 JSON {x,y}
//   int kai_screen_size(char* out, int out_cap);      // 主屏分辨率，返回 JSON {w,h}
//
// 约定：out 由调用方（Go）分配，out_cap 为容量；返回写入字节数（不含结尾\0），
//       失败返回负数。结果均为 JSON 字符串。
//
// 文件日志：所有桥接日志通过 bridgeFileLog 追加写入 dataDir/logs/kai-bridge.log，不带前缀；
// 未设置日志目录（kai_set_log_config 未调用）则不写任何日志。日志等级、保留天数、压缩
// 开关均由 Go 侧 LogConfig 经 kai_set_log_config 传入，与主应用日志（kai.log）保持一致策略。

import Translation
import NaturalLanguage
import ApplicationServices
import AppKit
import Foundation
import CoreGraphics
import Vision

// 日志等级数值映射（数值越大越严重，仅当 level >= bridgeLogLevel 才落盘）。
let BRIDGE_LOG_DEBUG: Int = 0
let BRIDGE_LOG_INFO:  Int = 1
let BRIDGE_LOG_WARN:  Int = 2
let BRIDGE_LOG_ERROR: Int = 3

// 日志目录由 Go 在启动时通过 kai_set_log_config 设置（指向 dataDir/logs）；
// 未设置则为 nil，bridgeFileLog 直接返回。等级/保留天数/压缩同步来自 LogConfig。
var bridgeLogDir: URL? = nil
var bridgeLogLevel: Int = BRIDGE_LOG_INFO
var bridgeRetentionDays: Int = 30
var bridgeCompress: Bool = true

// kai_set_log_config 由 Go 在启动时及配置热更新时调用，统一设置桥接层日志目录与策略。
// dir：日志目录（dataDir/logs）；level：debug/info/warn/error（非法回退 info）；
// retention_days：保留天数（<=0 表示仅按天滚动、不清理）；compress：过期归档是否压缩为 .gz。
@_cdecl("kai_set_log_config")
public func kai_set_log_config(_ dir: UnsafePointer<CChar>?,
                               _ level: UnsafePointer<CChar>?,
                               _ retention_days: Int32,
                               _ compress: Bool) {
    if let dir = dir {
        let path = String(cString: dir)
        let url = URL(fileURLWithPath: path, isDirectory: true)
        try? FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        bridgeLogDir = url
    }
    if let level = level {
        switch String(cString: level).lowercased() {
        case "debug":          bridgeLogLevel = BRIDGE_LOG_DEBUG
        case "warn", "warning": bridgeLogLevel = BRIDGE_LOG_WARN
        case "error":          bridgeLogLevel = BRIDGE_LOG_ERROR
        default:               bridgeLogLevel = BRIDGE_LOG_INFO
        }
    }
    bridgeRetentionDays = Int(retention_days)
    bridgeCompress = compress
    bridgeFileLog("日志配置已应用 dir=\(bridgeLogDir?.path ?? "") level=\(bridgeLogLevel) retention_days=\(bridgeRetentionDays) compress=\(bridgeCompress)", level: BRIDGE_LOG_INFO)
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
       !Calendar.current.isDateInToday(mtime) {
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

// 把 Swift String 写入调用方提供的 C 缓冲区（含结尾\0）。
// 返回实际写入长度（不含\0），超容量则返回 -1。
private func writeCString(_ str: String, into buf: UnsafeMutablePointer<CChar>?, cap: Int32) -> Int32 {
    guard let buf = buf else { return -1 }
    let capInt = Int(cap)
    guard capInt > 1 else { return -1 }
    let bytes = Array(str.utf8) // 不含 \0，长度 = 实际内容字节数
    let maxLen = min(bytes.count, capInt - 1)
    for i in 0..<maxLen {
        buf[i] = CChar(bitPattern: bytes[i])
    }
    buf[maxLen] = 0
    return Int32(maxLen)
}

// detectSourceLanguage：用 NaturalLanguage 的 LanguageRecognizer 自动识别文本主导语言，
// 并在本机已安装语言列表中求交集，保证返回的源语言一定是已下载、可直接用于 TranslationSession 的。
// installed：本机已安装语言码集合（小写形式用于匹配）。返回已安装的匹配源语言码；
// 若识别失败或识别出的语言未安装，则回退到 installed 中的首选（通常第一个，如 en），
// 若 installed 为空则返回 nil。
func detectSourceLanguage(_ text: String, installed: [String]) -> String? {
    let recognizer = NLLanguageRecognizer()
    recognizer.processString(text)
    guard let detected = recognizer.dominantLanguage?.rawValue else {
        return installed.first
    }
    let detectedLower = detected.lowercased()
    if let exact = installed.first(where: { $0.lowercased() == detectedLower }) { return exact }
    if let prefix = installed.first(where: { $0.lowercased().hasPrefix(detectedLower) }) { return prefix }
    if let rev = installed.first(where: { detectedLower.hasPrefix($0.lowercased()) }) { return rev }
    return installed.first
}

// kai_translate：同步执行一次系统翻译。
// src/dst 为 BCP-47 语言码（如 "en" / "zh-Hans"）；src 可为空字符串表示自动检测。
// out 接收 JSON：{"result":"...","from":"..."}；失败 out 写入 {"error":"..."}。
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

    bridgeFileLog("系统翻译调用 src=\(sourceCode) dst=\(targetCode) 文本长度=\(inputText.utf8.count)")

    guard !inputText.isEmpty else {
        bridgeFileLog("系统翻译失败: 文本为空", level: BRIDGE_LOG_WARN)
        return writeCString("{\"error\":\"empty text\"}", into: out, cap: out_cap)
    }
    guard !targetCode.isEmpty else {
        bridgeFileLog("系统翻译失败: 缺少目标语言", level: BRIDGE_LOG_WARN)
        return writeCString("{\"error\":\"target language required\"}", into: out, cap: out_cap)
    }

    let targetLang = Locale.Language(identifier: targetCode)

    let sema = DispatchSemaphore(value: 0)
    var resultJSON = "{\"error\":\"unknown\"}"

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
        if sourceCode.isEmpty {
            if let detected = detectSourceLanguage(inputText, installed: installed) {
                effectiveSource = detected
                bridgeFileLog("系统翻译自动检测源语言=\(effectiveSource) dst=\(targetCode)")
            } else {
                bridgeFileLog("系统翻译自动检测失败且无已安装语言回退 dst=\(targetCode)", level: BRIDGE_LOG_ERROR)
            }
        }
        guard !effectiveSource.isEmpty else {
            resultJSON = "{\"error\":\"no installed source language for auto detect\"}"
            sema.signal()
            return
        }
        let sourceLang = Locale.Language(identifier: effectiveSource)
        let session = TranslationSession(installedSource: sourceLang, target: targetLang)
        do {
            try await session.prepareTranslation()
            let resp = try await session.translate(inputText)
            let from = resp.sourceLanguage.languageCode?.identifier ?? sourceCode
            let payload = ["result": resp.targetText, "from": from]
            if let data = try? JSONSerialization.data(withJSONObject: payload),
               let str = String(data: data, encoding: .utf8) {
                resultJSON = str
                bridgeFileLog("系统翻译完成 from=\(from) dst=\(targetCode) 译文长度=\(resp.targetText.utf8.count)")
            }
        } catch {
            resultJSON = "{\"error\":\"\(error.localizedDescription)\"}"
            bridgeFileLog("系统翻译失败: \(error.localizedDescription)", level: BRIDGE_LOG_ERROR)
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
        let payload: [String: Any] = ["langs": installed]
        if let data = try? JSONSerialization.data(withJSONObject: payload),
           let str = String(data: data, encoding: .utf8) {
            resultJSON = str
        }
        bridgeFileLog("系统已安装语言列表查询完成 总数=\(all.count) 已安装=\(installed.count)", level: BRIDGE_LOG_DEBUG)
        sema.signal()
    }

    _ = sema.wait(timeout: .now() + 30)
    return writeCString(resultJSON, into: out, cap: out_cap)
}

// kai_accessibility_enabled 查询当前二进制是否已获得辅助功能授权。
@_cdecl("kai_accessibility_enabled")
public func kai_accessibility_enabled() -> Int32 {
    let trusted = AXIsProcessTrusted()
    bridgeFileLog("辅助功能授权查询 结果=\(trusted ? "已授权" : "未授权")", level: BRIDGE_LOG_DEBUG)
    return trusted ? 1 : 0
}

// kai_accessibility_request 请求辅助功能授权。
//
// 实现策略：先尝试一次系统弹窗（AXIsProcessTrustedWithOptions 带 prompt，仅首次会真正弹框，
// 之后系统不再弹，调用无害），随后立即打开系统「辅助功能」设置面板，让用户去勾选当前 app。
// 之所以打开设置面板：AX 弹窗只在从未决定过时弹一次，若用户此前已拒绝/忽略，再点按钮只会静默
// 无反应；而系统设置面板始终可打开，能保证"点击授权按钮必有所反馈"。
@_cdecl("kai_accessibility_request")
public func kai_accessibility_request() {
    bridgeFileLog("辅助功能授权请求 弹出系统授权框并尝试打开设置面板")
    // 首次调用才会真正弹系统授权框；已决定过则无副作用。
    let opts = [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: true] as CFDictionary
    _ = AXIsProcessTrustedWithOptions(opts)
    // 始终打开系统设置面板，确保点击后有明确反馈。
    if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility") {
        NSWorkspace.shared.open(url)
        bridgeFileLog("辅助功能授权请求 已打开系统设置面板")
    }
}

// kai_screenrecording_enabled 查询当前二进制是否已获得「屏幕录制」授权（截图 OCR 依赖）。
// 使用 ScreenCaptureKit 时代的 TCC 查询 API：CGPreflightScreenCaptureAccess。
@_cdecl("kai_screenrecording_enabled")
public func kai_screenrecording_enabled() -> Int32 {
    // CGPreflightScreenCaptureAccess 在 macOS 11+ 可用，返回是否已授权。
    let granted = CGPreflightScreenCaptureAccess()
    bridgeFileLog("屏幕录制授权查询 结果=\(granted ? "已授权" : "未授权")", level: BRIDGE_LOG_DEBUG)
    return granted ? 1 : 0
}

// kai_screenrecording_request 打开系统「屏幕录制」设置面板，让用户勾选当前 app。
//
// 注意：此前用的 CGRequestScreenCaptureAccess() 只在应用首次请求、且用户尚未决定时弹一次系统
// 授权框；若用户此前已拒绝/忽略，再调用只会静默返回 false，点击授权按钮毫无反馈。
// 正确做法是打开系统设置面板（x-apple.systempreferences URL scheme 的 Privacy_ScreenCapture 锚点），
// 这样无论授权状态如何，点击按钮都能把用户带到「隐私与安全性 > 屏幕录制」页去勾选/确认。
@_cdecl("kai_screenrecording_request")
public func kai_screenrecording_request() {
    bridgeFileLog("屏幕录制授权请求 打开系统设置面板")
    if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture") {
        NSWorkspace.shared.open(url)
        bridgeFileLog("屏幕录制授权请求 已打开系统设置面板")
    }
}

// TODO(2026-08-11): kai_selected_text 及其私有实现 readSelectedText 已禁用。
// 原因：用户反馈启用该 Swift 取词路径后电脑偶发奇奇怪怪的问题（与 AX 后台线程查询/焦点切换相关）。
// Go 侧 bridge_darwin.go 的 selectedTextViaBridge 与 point_darwin.go 的 currentSelectionOSA
// 已同步注释/改为返回空串；cgo 声明与导出符号一并移除，故此处注释后 libkai_bridge.a 不再提供该符号，
// 链接不会缺失。若日后定位根因需恢复：取消下方整段注释 + 恢复 Go 侧三处即可。
//
// kai_selected_text 通过 macOS 辅助功能（AXUIElement）读取当前前台应用的选区文本。
// 不依赖 AppleScript，授权失败或无选区时返回空串（写入 out 为空字符串）。
// out 接收 UTF-8 选区文本；返回写入字节数（不含\0），失败返回负数。
//
// 注意：不在此处用 AXIsProcessTrusted() 做前置拦截。原因：ad-hoc（临时自签名）打包的 app
// 在 macOS 上 AXIsProcessTrusted() 恒返回 false（TCC 授权查询绑定签名身份，ad-hoc 无 Team ID），
// 若前置拦截会导致真实选区读取被误杀、功能完全不可用。AX API 本身在未授权时会返回错误码，
// readSelectedText 已据此安全返回空串，无需主动 guard。
// @_cdecl("kai_selected_text")
// public func kai_selected_text(
//     _ out: UnsafeMutablePointer<CChar>?,
//     _ out_cap: Int32
// ) -> Int32 {
//     // AX 调用必须在主线程（run loop）执行。Wails 全局快捷键回调在后台 goroutine，
//     // 直接调用 AXUIElementCopyAttributeValue 会偶发 kAXErrorCannotComplete(-25212)，
//     // 故整体派发到主队列同步执行。
//     var result: Int32 = 0
//     if Thread.isMainThread {
//         result = readSelectedText(out: out, cap: out_cap)
//     } else {
//         DispatchQueue.main.sync {
//             result = readSelectedText(out: out, cap: out_cap)
//         }
//     }
//     return result
// }
//
// readSelectedText 实际取词逻辑（须在主线程调用）。
// private func readSelectedText(
//     out: UnsafeMutablePointer<CChar>?,
//     cap out_cap: Int32
// ) -> Int32 {
//     let systemWide = AXUIElementCreateSystemWide()
//
//     // 读取目标：前台应用的聚焦元素（能拿选区）。systemWide 本身不支持 kAXSelectedTextAttribute
//     // （调用会返回 -25205 kAXErrorAttributeUnsupported），因此不回退读 systemWide。
//     // 取不到前台 app 的聚焦元素时直接返回空，交由调用方（快捷键延迟后重试）处理。
//     var focusedApp: CFTypeRef?
//     let errApp = AXUIElementCopyAttributeValue(systemWide, kAXFocusedApplicationAttribute as CFString, &focusedApp)
//     guard errApp == .success, let app = focusedApp else {
//         bridgeFileLog("选区读取 无法获取前台应用 axErr=\(errApp.rawValue)")
//         return writeCString("", into: out, cap: out_cap)
//     }
//
//     var target: AXUIElement = app as! AXUIElement
//     var focusedElem: CFTypeRef?
//     let errElem = AXUIElementCopyAttributeValue(target, kAXFocusedUIElementAttribute as CFString, &focusedElem)
//     if errElem == .success, let elem = focusedElem {
//         target = elem as! AXUIElement
//     }
//
//     var selVal: CFTypeRef?
//     let errSel = AXUIElementCopyAttributeValue(target, kAXSelectedTextAttribute as CFString, &selVal)
//     guard errSel == .success, let strRef = selVal else {
//         bridgeFileLog("选区读取结果为空 axErr=\(errSel.rawValue)")
//         return writeCString("", into: out, cap: out_cap)
//     }
//
//     let text = (strRef as! String) as NSString
//     bridgeFileLog("选区读取完成 长度=\(text.length)")
//     return writeCString(text as String, into: out, cap: out_cap)
// }

// kai_selection_point 通过 AX 读取前台 app 窗口坐标与尺寸，返回 JSON：
// {"x":<屏幕中心x>, "y":<窗口底y>}，用于浮窗定位。未授权或无窗口返回 {"x":0,"y":0}。
//
// 不做 AXIsProcessTrusted() 前置拦截（原因见 kai_selected_text 注释），避免 ad-hoc 下误杀。
@_cdecl("kai_selection_point")
public func kai_selection_point(
    _ out: UnsafeMutablePointer<CChar>?,
    _ out_cap: Int32
) -> Int32 {
    let systemWide = AXUIElementCreateSystemWide()
    var focusedApp: CFTypeRef?
    guard AXUIElementCopyAttributeValue(systemWide, kAXFocusedApplicationAttribute as CFString, &focusedApp) == .success,
          let app = focusedApp else {
        return writeCString("{\"x\":0,\"y\":0}", into: out, cap: out_cap)
    }

    var posVal: CFTypeRef?
    var sizeVal: CFTypeRef?
    _ = AXUIElementCopyAttributeValue(app as! AXUIElement, kAXPositionAttribute as CFString, &posVal)
    _ = AXUIElementCopyAttributeValue(app as! AXUIElement, kAXSizeAttribute as CFString, &sizeVal)

    var x: CGFloat = 0, y: CGFloat = 0, w: CGFloat = 0, h: CGFloat = 0
    if let pos = posVal {
        var cgPoint = CGPoint.zero
        AXValueGetValue(pos as! AXValue, .cgPoint, &cgPoint)
        x = cgPoint.x; y = cgPoint.y
    }
    if let size = sizeVal {
        var cgSize = CGSize.zero
        AXValueGetValue(size as! AXValue, .cgSize, &cgSize)
        w = cgSize.width; h = cgSize.height
    }
    let anchorX = x + w / 2
    let anchorY = y + h
    let payload: [String: Any] = ["x": anchorX, "y": anchorY]
    var result = "{\"x\":0,\"y\":0}"
    if let data = try? JSONSerialization.data(withJSONObject: payload),
       let str = String(data: data, encoding: .utf8) {
        result = str
    }
    bridgeFileLog("选区坐标读取完成 x=\(anchorX) y=\(anchorY)", level: BRIDGE_LOG_DEBUG)
    return writeCString(result, into: out, cap: out_cap)
}

// kai_ocr：用 Vision.framework 的 VNRecognizeTextRequest 对传入图片做离线 OCR，
// 返回 JSON：{"text":"全部文字(按行\\n连接)","regions":[{"text":"...","conf":0.98,"box":[x,y,w,h]}]}。
// img 为 PNG/JPEG 图片的 base64 字符串（由 Go 侧把截图字节 base64 后传入）；
// regions.box 采用 [x, y, w, h]（左上原点，单位像素，与图片像素尺寸一致）。
// 失败 out 写入 {"error":"..."}。Vision 文本识别为异步 API，内部用信号量同步。
@_cdecl("kai_ocr")
public func kai_ocr(
    _ img: UnsafePointer<CChar>?,
    _ out: UnsafeMutablePointer<CChar>?,
    _ out_cap: Int32,
    _ correct: Int32,
    _ timeout_sec: Int32
) -> Int32 {
    guard let img = img else {
        return writeCString("{\"error\":\"null image\"}", into: out, cap: out_cap)
    }
    let b64 = String(cString: img)
    guard let data = Data(base64Encoded: b64) else {
        bridgeFileLog("系统 OCR 失败: base64 解码失败", level: BRIDGE_LOG_ERROR)
        return writeCString("{\"error\":\"decode image failed\"}", into: out, cap: out_cap)
    }
    // 用 CGImageSource 直接从 PNG/JPEG 字节解码为 CGImage，避免 NSImage 的分辨率/方向适配层
    // 在 Vision 下偶发触发 CRImageReaderError error 1（区域截图更常见）。
    guard let src = CGImageSourceCreateWithData(data as CFData, nil),
          let rawImage = CGImageSourceCreateImageAtIndex(src, 0, nil) else {
        bridgeFileLog("系统 OCR 失败: 图片解码失败", level: BRIDGE_LOG_ERROR)
        return writeCString("{\"error\":\"decode image failed\"}", into: out, cap: out_cap)
    }
    // 把原始 cgImage 重绘到一个「标准格式」位图上下文（RGBA8、无 Alpha、sRGB 色彩空间），
    // 再用重绘后的 cgImage 喂给 Vision。区域截图偶发的
    // TextRecognition.CRImageReaderError error 1 是 Vision 内部 CRImageReader 对
    // 特殊像素格式/Alpha 通道/色彩空间拒绝解码所致；强制重绘为标准格式可彻底规避
    // （Vision 的 VNImageRequestHandler 内部仍会用 CRImageReader 重新解码，无法靠外层
    // CGImageSource 解码绕过，必须在喂入前把像素格式归一化）。
    let w = rawImage.width
    let h = rawImage.height
    guard w > 0, h > 0 else {
        bridgeFileLog("系统 OCR 失败: 图片尺寸为空", level: BRIDGE_LOG_ERROR)
        return writeCString("{\"error\":\"empty image\"}", into: out, cap: out_cap)
    }
    let colorSpace = CGColorSpaceCreateDeviceRGB()
    let bytesPerRow = w * 4 // 显式计算每行字节数（RGBA8），避免系统自动对齐在某些尺寸下触发 CRImageReader
    guard let ctx = CGContext(data: nil,
                              width: w,
                              height: h,
                              bitsPerComponent: 8,
                              bytesPerRow: bytesPerRow,
                              space: colorSpace,
                              bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue) else {
        bridgeFileLog("系统 OCR 失败: 位图上下文创建失败", level: BRIDGE_LOG_ERROR)
        return writeCString("{\"error\":\"bitmap ctx failed\"}", into: out, cap: out_cap)
    }
    ctx.draw(rawImage, in: CGRect(x: 0, y: 0, width: w, height: h))
    guard let cgImage = ctx.makeImage() else {
        bridgeFileLog("系统 OCR 失败: 位图重绘失败", level: BRIDGE_LOG_ERROR)
        return writeCString("{\"error\":\"bitmap redraw failed\"}", into: out, cap: out_cap)
    }
    // 诊断：打印原始图与重绘图的格式，确认 CRImageReaderError 是否因格式未归一化导致。
    let rawBPP = rawImage.bitsPerPixel
    let rawAlpha = rawImage.alphaInfo.rawValue
    let rawCS = rawImage.colorSpace?.name.map({ $0 as String }) ?? "nil"
    let outBPP = cgImage.bitsPerPixel
    let outAlpha = cgImage.alphaInfo.rawValue
    bridgeFileLog("系统 OCR 重绘前 raw bpp=\(rawBPP) alpha=\(rawAlpha) cs=\(rawCS) | 重绘后 bpp=\(outBPP) alpha=\(outAlpha) w=\(w) h=\(h)", level: BRIDGE_LOG_DEBUG)

    // 初始占位非真实错误：若 perform 抛错才会被改写；正常同步完成后必为识别结果。
    var resultJSON = "{\"error\":\"ocr pending\"}"
    let imgW = CGFloat(cgImage.width)
    let imgH = CGFloat(cgImage.height)

    // Vision 的 perform 是同步阻塞调用，若不设超时，遇到异常图片会一直卡在 cgo 线程上，
    // 导致 Go 侧 30s 超时之后 Swift 仍在后台跑（实测可达 69s），既浪费线程又让用户干等。
    // 因此改为：perform 放到后台队列异步执行，cgo 线程用信号量带超时等待其完成；
    // 超时则立即返回 ocr timeout，后台 perform 晚到的结果只写入局部变量，不再拖住 Go 侧。
    // 超时阈值由调用方传入（默认 60s），保证 Go 侧 ctx.Done() 不会提前假触发。
    let ocrTimeout: TimeInterval = timeout_sec > 0 ? TimeInterval(timeout_sec) : 60
    let request = VNRecognizeTextRequest { req, err in
        if let err = err {
            resultJSON = "{\"error\":\"\(err.localizedDescription)\"}"
            bridgeFileLog("系统 OCR 失败: \(err.localizedDescription)", level: BRIDGE_LOG_ERROR)
            return
        }
        guard let observations = req.results as? [VNRecognizedTextObservation] else {
            resultJSON = "{\"text\":\"\",\"regions\":[]}"
            return
        }
        var lines: [String] = []
        var regions: [[String: Any]] = []
        for obs in observations {
            guard let candidate = obs.topCandidates(1).first else { continue }
            let text = candidate.string
            let conf = candidate.confidence
            // Vision 的 boundingBox 以左下原点、归一化 [0,1]；转成左上原点像素 [x,y,w,h]。
            let b = obs.boundingBox
            let x = b.origin.x * imgW
            let y = (1 - b.origin.y - b.size.height) * imgH
            let w = b.size.width * imgW
            let h = b.size.height * imgH
            lines.append(text)
            regions.append([
                "text": text,
                "conf": Double(conf),
                "box": [Int(x), Int(y), Int(w), Int(h)],
            ])
        }
        let payload: [String: Any] = [
            "text": lines.joined(separator: "\n"),
            "regions": regions,
        ]
        if let d = try? JSONSerialization.data(withJSONObject: payload),
           let str = String(data: d, encoding: .utf8) {
            resultJSON = str
            bridgeFileLog("系统 OCR 完成 行数=\(lines.count) 首行长度=\(lines.first?.utf8.count ?? 0)", level: BRIDGE_LOG_INFO)
        }
    }
    request.recognitionLanguages = ["zh-Hans", "zh-Hant", "en"]
    // usesLanguageCorrection=true 会让 Vision 对每个候选做语言模型校正打分，
    // 在特定像素/文字密度下显著放大 perform 内部耗时，是偶发 60s+ 卡死的主因。
    // 截图翻译多为中英文短句。校正开关由调用方传入（correct=1 开启 / 0 关闭）：
    // 开启（默认）更准确，关闭更快、偶发卡死概率更低。
    request.usesLanguageCorrection = correct != 0
    // 显式用 .fast 模式（默认 .accurate 更重），配合关闭校正，正常图仍是秒回。
    request.recognitionLevel = .fast
    bridgeFileLog("系统 OCR 配置 correction=\(request.usesLanguageCorrection) level=\(request.recognitionLevel.rawValue)", level: BRIDGE_LOG_DEBUG)

    let handler = VNImageRequestHandler(cgImage: cgImage, options: [:])

    let sig = DispatchSemaphore(value: 0)
    let performStart = Date()
    // 后台队列异步执行同步 perform，避免阻塞 cgo 调用线程；完成（或失败）时释放信号量。
    DispatchQueue.global(qos: .userInitiated).async {
        do {
            try handler.perform([request]) // 同步：perform 返回即表示识别已完成（completion 已触发）
        } catch {
            resultJSON = "{\"error\":\"\(error.localizedDescription)\"}"
            bridgeFileLog("系统 OCR 失败: \(error.localizedDescription)", level: BRIDGE_LOG_ERROR)
        }
        let elapsed = Date().timeIntervalSince(performStart)
        bridgeFileLog("系统 OCR perform 实际耗时=\(String(format: "%.2f", elapsed))s", level: BRIDGE_LOG_DEBUG)
        sig.signal()
    }
    // 带超时等待：ocrTimeout 内未完成则视为 OCR 超时，立即返回，不拖住 Go 侧 30s 超时逻辑。
    let waitResult = sig.wait(timeout: .now() + ocrTimeout)
    if waitResult == .timedOut {
        bridgeFileLog("系统 OCR 超时: perform 超过 \(Int(ocrTimeout))s 未完成", level: BRIDGE_LOG_ERROR)
        return writeCString("{\"error\":\"ocr timeout\"}", into: out, cap: out_cap)
    }
    return writeCString(resultJSON, into: out, cap: out_cap)
}

// kai_screen_size 返回主屏幕分辨率（逻辑像素），返回 JSON：{"w":<宽>, "h":<高>}。
@_cdecl("kai_screen_size")
public func kai_screen_size(
    _ out: UnsafeMutablePointer<CChar>?,
    _ out_cap: Int32
) -> Int32 {
    let screen = NSScreen.main
    let frame = screen?.visibleFrame ?? CGRect.zero
    let payload: [String: Any] = ["w": frame.width, "h": frame.height]
    var result = "{\"w\":0,\"h\":0}"
    if let data = try? JSONSerialization.data(withJSONObject: payload),
       let str = String(data: data, encoding: .utf8) {
        result = str
    }
    return writeCString(result, into: out, cap: out_cap)
}

// TODO: 输入监控相关（kai_input_monitoring_enabled / kai_input_monitoring_request 两个 @_cdecl 接口）当前未使用，已注释。需 robotgo 模拟复制键时恢复（注意保留 ARC 自动释放、回调原样回传，避免全局键盘失控）。
// // kai_input_monitoring_enabled 检测 macOS「输入监控」授权状态。
// // 输入监控（TCC kTCCServiceListenEvent）没有公开的直接查询 API，常规做法是尝试在
// // kCGHIDEventTap 上创建 CGEventTap：创建失败或被策略禁用即未授权。复制键（robotgo
// // 模拟 Cmd+C，底层 CGEventPost）依赖此授权，未授权时按键事件被系统静默丢弃。
// //
// // 注意：本函数只用于“探测授权”，创建 tap 后必须立即释放，且回调不得吞事件——
// // 否则会在全局键盘事件流最前留下一个常驻 tap，导致所有 app 的按键被拦截/吞掉/乱序，
// // 表现为“鼠标或键盘偶尔不受控制”。
// @_cdecl("kai_input_monitoring_enabled")
// public func kai_input_monitoring_enabled() -> Int32 {
//     let tap = CGEvent.tapCreate(
//         tap: .cghidEventTap,
//         place: .headInsertEventTap,
//         options: .defaultTap,
//         eventsOfInterest: CGEventMask(1 << CGEventType.keyDown.rawValue),
//         callback: { _, _, event, _ in return Unmanaged.passRetained(event) },  // 重要：原样回传，绝不吞事件
//         userInfo: nil
//     )
//     guard let t = tap else {
//         bridgeFileLog("输入监控检测 创建 EventTap 失败（未授权或被策略拒绝）")
//         return 0
//     }
//     // 创建成功但被策略禁用（CGEventTapIsEnabled == false）也视为未授权。
//     let enabled = CGEvent.tapIsEnabled(tap: t)
//     bridgeFileLog("输入监控检测 tapIsEnabled=\(enabled)")
//     // t 为 CFMachPort，受 Swift ARC 管理；函数返回后自动释放，内核 event tap 随之失效，
//     // 不会留下常驻全局键盘拦截器（之前手动 CFRelease 在 ARC 下不可用，且旧代码从不释放才导致失控）。
//     return enabled ? 1 : 0
// }
//
// // kai_input_monitoring_request 打开系统「安全性与隐私 > 输入监控」设置面板。
// // 输入监控没有类似辅助功能的系统弹窗请求 API（AXIsProcessTrustedWithOptions），
// // 只能通过打开设置面板让用户手动勾选当前 app。
// @_cdecl("kai_input_monitoring_request")
// public func kai_input_monitoring_request() {
//     if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent") {
//         NSWorkspace.shared.open(url)
//         bridgeFileLog("输入监控 打开系统设置面板")
//     }
// }
