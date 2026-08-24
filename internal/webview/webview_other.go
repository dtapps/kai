//go:build !windows

// Package webview 提供 Windows 平台 WebView2 运行时的启动前自检。
// 非 Windows 平台（macOS / Linux）无需检测，提供空实现。
package webview

// EnsureWebView2 在非 Windows 平台为空操作。
func EnsureWebView2() {}

// BrowserArgs 在非 Windows 平台返回 nil（无 WebView2，无需浏览器参数）。
func BrowserArgs() []string { return nil }

// ApplyOptions 在非 Windows 平台为空操作（AdditionalBrowserArgs 字段不存在）。
func ApplyOptions(opts any) {}
