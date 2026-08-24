//go:build windows && !server

// Package webview 提供 Windows 平台 WebView2 的浏览器参数兜底。
// 仅保留对 80010108 偶发崩溃可能有效的 GPU 兜底参数注入；Runtime 缺失检测已移除
// （该错误在已安装 Runtime 的机器上偶发，检测层只会误伤正常用户）。
// 非 Windows 平台由 webview_other.go 提供空实现。
package webview

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// BrowserArgs 返回 Windows 平台下注入 WebView2 的全局浏览器启动参数。
//
// 背景：80010108 (RPC_E_DISCONNECTED) 偶发崩溃（"The object invoked has
// disconnected from its clients"）在已安装 WebView2 的机器上偶发，高频诱因是 GPU
// 进程异常（显卡驱动 / 硬件加速 GPU 调度 / 系统从睡眠恢复后的 DComp 状态异常），
// 导致 WebView2 渲染进程在 controller 创建回调前崩溃，回调拿到已断开对象。注入
// --disable-gpu 与 --disable-gpu-compositing 将渲染降级到软件路径，掐断这条
// 偶发崩溃路径。代价是失去硬件加速（对翻译类轻量 UI 影响可忽略）。
// WebView2 共享单一浏览器环境，该参数对所有窗口（含检查更新窗口）全局生效。
// 非 Windows 平台（_other.go）返回 nil。
func BrowserArgs() []string {
	return []string{
		"--disable-gpu",
		"--disable-gpu-compositing",
	}
}

// ApplyOptions 在 Windows 上把 WebView2 浏览器兜底参数写入 application.Options。
// 注意：AdditionalBrowserArgs 位于 Options.Windows（WindowsOptions）子结构内，
// 而非顶层 Options（macOS 结构体无 Windows 字段），故只能在 //go:build windows
// 文件中访问。非 Windows 由 _other.go 提供空实现，main.go 可无条件调用。
func ApplyOptions(opts *application.Options) {
	opts.Windows.AdditionalBrowserArgs = BrowserArgs()
}
