// apple_accessibility.swift
// 辅助功能 / 屏幕录制授权查询与引导、选区锚点读取、主屏分辨率读取。
// 依赖 bridge_common.swift（writeCString）、bridge_log.swift（bridgeFileLog / bridgeLogText）。
import AppKit
import ApplicationServices
import CoreGraphics
import Foundation
import NaturalLanguage
import Translation
import Vision

// kai_accessibility_enabled：查询辅助功能是否已授权。
// out 接收 "true"/"false"。
@_cdecl("kai_accessibility_enabled")
public func kai_accessibility_enabled(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let enabled = AXIsProcessTrusted()
  bridgeFileLog(bridgeLogText("a11y.query", String(enabled)))
  return writeCString(enabled ? "true" : "false", into: out, cap: out_cap)
}

// kai_accessibility_request：弹出系统授权框，并尝试打开系统设置 > 隐私与安全 > 辅助功能 面板。
// 返回 0 表示成功发起（仅代表"已请求"，不等于已授权）。
@_cdecl("kai_accessibility_request")
public func kai_accessibility_request() -> Int32 {
  bridgeFileLog(bridgeLogText("a11y.request"))
  // 触发系统授权弹窗（异步请求，首次会弹框）。
  let opts = [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: true] as CFDictionary
  _ = AXIsProcessTrustedWithOptions(opts)
  // 额外打开设置面板，便于用户立即勾选。
  if #available(macOS 13.0, *) {
    if let url = URL(
      string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")
    {
      NSWorkspace.shared.open(url)
    }
  }
  bridgeFileLog(bridgeLogText("a11y.request_done"))
  return 0
}

// kai_screenrecording_enabled：查询屏幕录制是否已授权。
// out 接收 "true"/"false"。
@_cdecl("kai_screenrecording_enabled")
public func kai_screenrecording_enabled(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let enabled = CGPreflightScreenCaptureAccess()
  bridgeFileLog(bridgeLogText("screen.query", String(enabled)))
  return writeCString(enabled ? "true" : "false", into: out, cap: out_cap)
}

// kai_screenrecording_request：打开系统设置 > 隐私与安全 > 屏幕录制 面板。
// 返回 0 表示成功打开。
@_cdecl("kai_screenrecording_request")
public func kai_screenrecording_request() -> Int32 {
  bridgeFileLog(bridgeLogText("screen.request"))
  if let url = URL(
    string: "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture")
  {
    NSWorkspace.shared.open(url)
  }
  bridgeFileLog(bridgeLogText("screen.request_done"))
  return 0
}

// kai_selection_point：读取当前鼠标位置，返回 {"x":0,"y":0}（屏幕坐标，左下角为原点）。
@_cdecl("kai_selection_point")
public func kai_selection_point(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let loc = NSEvent.mouseLocation  // 左上角为原点的 Cocoa 坐标
  let h = NSScreen.screens.first?.frame.height ?? 0
  let point = SelectionPoint(x: Double(loc.x), y: Double(h - loc.y))  // 转换为左下角原点
  let json = bridgeEncode(point)
  bridgeFileLog(
    bridgeLogText(
      "selection.point_done", String(format: "%.1f", point.x), String(format: "%.1f", point.y)),
    level: BRIDGE_LOG_DEBUG)
  return writeCString(json, into: out, cap: out_cap)
}

// kai_screen_size：返回主屏分辨率 {"w":0,"h":0}（逻辑点）。
@_cdecl("kai_screen_size")
public func kai_screen_size(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  guard let main = NSScreen.screens.first else {
    let json = bridgeEncode(ScreenSize(w: 0, h: 0))
    return writeCString(json, into: out, cap: out_cap)
  }
  let size = ScreenSize(w: Double(main.frame.width), h: Double(main.frame.height))
  let json = bridgeEncode(size)
  return writeCString(json, into: out, cap: out_cap)
}

// 说明：选区文本读取（AXUIElement 取 AXSelectedText）之前在 kai_selected_text 实现，
// 对多数 App 取值失败（返回 empty / deadlock），实际由 Go 端 engine.CaptureRegion + OCR 完成，
// 故该入口已禁用，保留注释以免误用：
//   // @_cdecl("kai_selected_text")
//   // public func kai_selected_text(...) -> Int32 { ... AXUIElementCopyAttributeValue(AXValue(...)) ... }
//
// 输入监控授权（Input Monitoring）检测：用 EventTap 创建失败判定未授权。
// 之前在 kai_input_monitoring 实现，但事件点击转发由 Go 端 hotkey 处理，Swift 不再负责，
// 仅保留查询能力，逻辑见 detectSourceLanguage 同模块的注释示例。

// kai_input_monitoring_enabled：检测输入监控是否已授权（创建 CGEvent.tap 失败即未授权）。
// out 接收 "true"/"false"。
@_cdecl("kai_input_monitoring_enabled")
public func kai_input_monitoring_enabled(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  var enabled = false
  if let src = CGEventSource(stateID: .combinedSessionState) {
    let tap = CGEvent.tapCreate(
      tap: .cgSessionEventTap,
      place: .headInsertEventTap,
      options: .defaultTap,
      eventsOfInterest: CGEventMask(1 << CGEventType.keyDown.rawValue),
      callback: { _, _, _, _ in return Unmanaged.passRetained(CGEvent(source: nil)!) },
      userInfo: UnsafeMutableRawPointer(Unmanaged.passRetained(src).toOpaque())
    )
    if let tap = tap {
      enabled = true
      CFMachPortInvalidate(tap)
    } else {
      bridgeFileLog(bridgeLogText("input.tap_fail"), level: BRIDGE_LOG_WARN)
    }
  }
  bridgeFileLog(bridgeLogText("input.tap_enabled", String(enabled)), level: BRIDGE_LOG_DEBUG)
  return writeCString(enabled ? "true" : "false", into: out, cap: out_cap)
}
